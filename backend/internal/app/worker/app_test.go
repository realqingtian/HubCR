package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/platform/config"
)

func TestAppBoundsConcurrencyAndCompletesJobs(t *testing.T) {
	queue := newFakeQueue([]jobs.Job{
		{ID: "job-1", Kind: "TEST", Attempts: 1},
		{ID: "job-2", Kind: "TEST", Attempts: 1},
		{ID: "job-3", Kind: "TEST", Attempts: 1},
	})
	started := make(chan string, 3)
	release := make(chan struct{}, 3)
	handler := jobs.HandlerFunc(func(_ context.Context, job jobs.Job) error {
		started <- job.ID
		<-release
		return nil
	})
	app := mustTestApp(t, queue, 2, map[jobs.Kind]jobs.Handler{"TEST": handler})
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- app.Run(ctx) }()

	first := receive(t, started)
	second := receive(t, started)
	if first == second {
		t.Fatalf("first two started jobs = %q, %q", first, second)
	}
	select {
	case third := <-started:
		t.Fatalf("third job %q started before a concurrency slot was released", third)
	default:
	}
	release <- struct{}{}
	third := receive(t, started)
	if third == first || third == second {
		t.Fatalf("third started job = %q", third)
	}
	release <- struct{}{}
	release <- struct{}{}
	for range 3 {
		receive(t, queue.completed)
	}
	cancel()
	if err := receive(t, finished); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if queue.failureCount() != 0 {
		t.Fatalf("failure count = %d, want 0", queue.failureCount())
	}
}

func TestAppPersistsClassifiedFailureAndDefersCanceledWork(t *testing.T) {
	t.Run("classified permanent failure", func(t *testing.T) {
		queue := newFakeQueue([]jobs.Job{{ID: "job-failure", Kind: "FAIL", Attempts: 2}})
		handler := jobs.HandlerFunc(func(context.Context, jobs.Job) error {
			return jobs.Permanent("INVALID_ARTIFACT", errors.New("untrusted handler detail"))
		})
		app := mustTestApp(t, queue, 1, map[jobs.Kind]jobs.Handler{"FAIL": handler})
		ctx, cancel := context.WithCancel(context.Background())
		finished := make(chan error, 1)
		go func() { finished <- app.Run(ctx) }()
		failure := receive(t, queue.failed)
		if failure.code != "INVALID_ARTIFACT" || !failure.terminal || failure.retryAfter != 2*time.Second {
			t.Fatalf("failure = %#v", failure)
		}
		cancel()
		if err := receive(t, finished); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("shutdown leaves lease for recovery", func(t *testing.T) {
		queue := newFakeQueue([]jobs.Job{{ID: "job-canceled", Kind: "BLOCK", Attempts: 1}})
		started := make(chan struct{}, 1)
		handler := jobs.HandlerFunc(func(ctx context.Context, _ jobs.Job) error {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
		app := mustTestApp(t, queue, 1, map[jobs.Kind]jobs.Handler{"BLOCK": handler})
		ctx, cancel := context.WithCancel(context.Background())
		finished := make(chan error, 1)
		go func() { finished <- app.Run(ctx) }()
		receive(t, started)
		cancel()
		if err := receive(t, finished); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if queue.completionCount() != 0 || queue.failureCount() != 0 {
			t.Fatalf("canceled work mutated queue: completed=%d failed=%d", queue.completionCount(), queue.failureCount())
		}
	})
}

type recordedFailure struct {
	code       string
	retryAfter time.Duration
	terminal   bool
}

type fakeQueue struct {
	mutex       sync.Mutex
	jobs        []jobs.Job
	completed   chan string
	failed      chan recordedFailure
	completions int
	failures    int
}

func newFakeQueue(items []jobs.Job) *fakeQueue {
	return &fakeQueue{
		jobs:      append([]jobs.Job(nil), items...),
		completed: make(chan string, len(items)), failed: make(chan recordedFailure, len(items)),
	}
}

func (q *fakeQueue) ClaimKinds(context.Context, string, time.Duration, []jobs.Kind) (jobs.Job, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.jobs) == 0 {
		return jobs.Job{}, jobs.ErrNoJob
	}
	job := q.jobs[0]
	q.jobs = q.jobs[1:]
	return job, nil
}

func (q *fakeQueue) Complete(_ context.Context, job jobs.Job, _ string) error {
	q.mutex.Lock()
	q.completions++
	q.mutex.Unlock()
	q.completed <- job.ID
	return nil
}

func (q *fakeQueue) Fail(
	_ context.Context,
	_ jobs.Job,
	_ string,
	code string,
	retryAfter time.Duration,
	terminal bool,
) error {
	q.mutex.Lock()
	q.failures++
	q.mutex.Unlock()
	failure := recordedFailure{code: code, retryAfter: retryAfter, terminal: terminal}
	q.failed <- failure
	return nil
}

func (q *fakeQueue) completionCount() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.completions
}

func (q *fakeQueue) failureCount() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.failures
}

func mustTestApp(
	t *testing.T,
	queue Queue,
	maxConcurrency int32,
	handlers map[jobs.Kind]jobs.Handler,
) *App {
	t.Helper()
	app, err := newApp(config.Worker{
		Database:     config.Database{ConnectTimeout: time.Second},
		PollInterval: time.Hour, LeaseDuration: 2 * time.Hour, JobTimeout: time.Hour,
		ShutdownTimeout: time.Second, RetryBase: time.Second, RetryMax: time.Minute,
		MaxConcurrency: maxConcurrency,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, queue, "worker-test", handlers)
	if err != nil {
		t.Fatalf("newApp() error = %v", err)
	}
	return app
}

func receive[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	select {
	case value := <-channel:
		return value
	case <-ctx.Done():
		t.Fatal("timed out waiting for test signal")
		var zero T
		return zero
	}
}
