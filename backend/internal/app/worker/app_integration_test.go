package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/jobstore"
	"hubcr.io/hubcr/migrations"
)

func TestWorkerRestartReclaimsPersistedLease(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 6,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}
	clock := &mutableClock{now: time.Date(2026, 8, 8, 3, 0, 0, 0, time.UTC)}
	service, err := jobs.NewService(jobstore.New(pool.ORM()), clock.Now)
	if err != nil {
		t.Fatalf("jobs.NewService() error = %v", err)
	}
	queued, inserted, err := service.Enqueue(
		ctx, "RESTART_TEST", "restart:test:"+time.Now().UTC().Format("20060102150405.000000000"),
		json.RawMessage(`{}`), 3, clock.Now(),
	)
	if err != nil || !inserted {
		t.Fatalf("Enqueue() = %#v, %v, %v", queued, inserted, err)
	}
	observed := &observedQueue{Service: service, completed: make(chan jobs.Job, 1)}
	cfg := integrationWorkerConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	started := make(chan struct{}, 1)
	firstHandler := jobs.HandlerFunc(func(handlerContext context.Context, _ jobs.Job) error {
		started <- struct{}{}
		<-handlerContext.Done()
		return handlerContext.Err()
	})
	first, err := newApp(
		cfg, logger, nil, observed, "worker-before-restart",
		map[jobs.Kind]jobs.Handler{"RESTART_TEST": firstHandler},
	)
	if err != nil {
		t.Fatalf("newApp(first) error = %v", err)
	}
	firstContext, stopFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.Run(firstContext) }()
	receive(t, started)
	stopFirst()
	if err := receive(t, firstDone); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	assertPersistedJob(t, ctx, pool, queued.ID, "RUNNING", 1, "")

	clock.Advance(cfg.LeaseDuration + time.Microsecond)
	secondHandler := jobs.HandlerFunc(func(context.Context, jobs.Job) error { return nil })
	second, err := newApp(
		cfg, logger, nil, observed, "worker-after-restart",
		map[jobs.Kind]jobs.Handler{"RESTART_TEST": secondHandler},
	)
	if err != nil {
		t.Fatalf("newApp(second) error = %v", err)
	}
	secondContext, stopSecond := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Run(secondContext) }()
	completed := receive(t, observed.completed)
	if completed.ID != queued.ID || completed.Attempts != 2 {
		t.Fatalf("reclaimed job = %#v", completed)
	}
	stopSecond()
	if err := receive(t, secondDone); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	assertPersistedJob(t, ctx, pool, queued.ID, "SUCCEEDED", 2, "LEASE_EXPIRED")
}

type observedQueue struct {
	*jobs.Service
	completed chan jobs.Job
}

func (q *observedQueue) Complete(ctx context.Context, job jobs.Job, workerID string) error {
	if err := q.Service.Complete(ctx, job, workerID); err != nil {
		return err
	}
	q.completed <- job
	return nil
}

type mutableClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.now
}

func (c *mutableClock) Advance(duration time.Duration) {
	c.mutex.Lock()
	c.now = c.now.Add(duration)
	c.mutex.Unlock()
}

func integrationWorkerConfig() config.Worker {
	return config.Worker{
		Database:     config.Database{ConnectTimeout: 3 * time.Second},
		PollInterval: time.Hour, LeaseDuration: 10 * time.Minute, JobTimeout: 5 * time.Minute,
		ShutdownTimeout: time.Second, RetryBase: time.Second, RetryMax: time.Minute,
		MaxConcurrency: 1,
	}
}

func assertPersistedJob(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	jobID string,
	state string,
	attempts int,
	errorCode string,
) {
	t.Helper()
	var record struct {
		State         string
		AttemptCount  int
		LastErrorCode *string
	}
	if err := pool.ORM().WithContext(ctx).Table("jobs").Where("id = ?", jobID).First(&record).Error; err != nil {
		t.Fatalf("read persisted job: %v", err)
	}
	if record.State != state || record.AttemptCount != attempts {
		t.Fatalf("persisted job = %#v", record)
	}
	if errorCode == "" && record.LastErrorCode != nil ||
		errorCode != "" && (record.LastErrorCode == nil || *record.LastErrorCode != errorCode) {
		t.Fatalf("LastErrorCode = %#v, want %q", record.LastErrorCode, errorCode)
	}
}
