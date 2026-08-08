package jobstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/migrations"
)

func TestStoreLifecycleLeaseRecoveryAndDeduplication(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 12,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}
	if err := pool.ORM().WithContext(ctx).Exec("DELETE FROM jobs").Error; err != nil {
		t.Fatalf("clear jobs: %v", err)
	}

	store := New(pool.ORM())
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	intent := mustIntent(t, "SCAN", "scan:repository:digest:policy-1", `{"digest":"sha256:a"}`, 3, now)
	created, inserted, err := store.Enqueue(ctx, intent, now)
	if err != nil || !inserted || created.State != jobs.StateQueued || created.Attempts != 0 {
		t.Fatalf("Enqueue() = %#v, %v, %v", created, inserted, err)
	}
	replayed, inserted, err := store.Enqueue(ctx, intent, now.Add(time.Second))
	if err != nil || inserted || replayed.ID != created.ID {
		t.Fatalf("Enqueue(replay) = %#v, %v, %v", replayed, inserted, err)
	}
	conflict := mustIntent(t, "SCAN", intent.Key, `{"digest":"sha256:b"}`, 3, now)
	if _, _, err := store.Enqueue(ctx, conflict, now); !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("Enqueue(conflict) error = %v, want ErrConflict", err)
	}

	lease := 30 * time.Second
	first := mustClaim(t, ctx, store, "worker-1", now, lease)
	if first.ID != created.ID || first.Attempts != 1 || first.State != jobs.StateRunning {
		t.Fatalf("first Claim() = %#v", first)
	}
	wrongOwner, _ := jobs.NewCompletion(first.ID, "worker-2", now.Add(time.Second))
	if err := store.Complete(ctx, wrongOwner); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("Complete(wrong owner) error = %v, want ErrLeaseLost", err)
	}

	failure, err := jobs.NewFailure(
		first.ID, "worker-1", "SCANNER_UNAVAILABLE", now.Add(time.Second), now.Add(11*time.Second), false,
	)
	if err != nil {
		t.Fatalf("NewFailure() error = %v", err)
	}
	if err := store.Fail(ctx, failure); err != nil {
		t.Fatalf("Fail(retryable) error = %v", err)
	}
	if _, err := store.Claim(ctx, mustClaimInput(t, "worker-2", now.Add(10*time.Second), lease)); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatalf("Claim(before retry) error = %v, want ErrNoJob", err)
	}
	second := mustClaim(t, ctx, store, "worker-2", now.Add(11*time.Second), lease)
	if second.Attempts != 2 || second.LastErrorCode == nil || *second.LastErrorCode != "SCANNER_UNAVAILABLE" {
		t.Fatalf("second Claim() = %#v", second)
	}
	if _, err := store.Claim(ctx, mustClaimInput(t, "worker-3", now.Add(40*time.Second), lease)); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatalf("Claim(before lease expiry) error = %v, want ErrNoJob", err)
	}
	third := mustClaim(t, ctx, store, "worker-3", now.Add(41*time.Second), lease)
	if third.Attempts != 3 || third.LastErrorCode == nil || *third.LastErrorCode != "LEASE_EXPIRED" {
		t.Fatalf("reclaimed Claim() = %#v", third)
	}
	exhausted, err := jobs.NewFailure(
		third.ID, "worker-3", "TRANSIENT_FAILURE", now.Add(42*time.Second), now.Add(52*time.Second), false,
	)
	if err != nil {
		t.Fatalf("NewFailure(exhausted) error = %v", err)
	}
	if err := store.Fail(ctx, exhausted); err != nil {
		t.Fatalf("Fail(exhausted) error = %v", err)
	}
	assertState(t, ctx, pool, third.ID, jobs.StateDead, 3, "TRANSIENT_FAILURE")

	successIntent := mustIntent(t, "SBOM", "sbom:repository:digest:policy-1", `{}`, 2, now)
	successJob, _, err := store.Enqueue(ctx, successIntent, now)
	if err != nil {
		t.Fatalf("Enqueue(success job) error = %v", err)
	}
	claimed := mustClaim(t, ctx, store, "worker-success", now.Add(time.Minute), lease)
	if claimed.ID != successJob.ID {
		t.Fatalf("Claim(success job).ID = %q, want %q", claimed.ID, successJob.ID)
	}
	completion, _ := jobs.NewCompletion(claimed.ID, "worker-success", now.Add(61*time.Second))
	if err := store.Complete(ctx, completion); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	assertState(t, ctx, pool, claimed.ID, jobs.StateSucceeded, 1, "")

	assertConcurrentSingleClaim(t, ctx, pool, store, now.Add(2*time.Minute), lease)
	assertConcurrentEnqueue(t, ctx, pool, store, now.Add(3*time.Minute))
}

func assertConcurrentEnqueue(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	store *Store,
	now time.Time,
) {
	t.Helper()
	if err := pool.ORM().WithContext(ctx).Exec("DELETE FROM jobs").Error; err != nil {
		t.Fatalf("clear jobs for concurrent enqueue: %v", err)
	}
	intent := mustIntent(t, "SCAN", "scan:concurrent", `{"digest":"sha256:c"}`, 3, now)
	start := make(chan struct{})
	type result struct {
		job      jobs.Job
		inserted bool
		err      error
	}
	results := make(chan result, 6)
	var waitGroup sync.WaitGroup
	for range 6 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			job, inserted, err := store.Enqueue(ctx, intent, now)
			results <- result{job: job, inserted: inserted, err: err}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	insertedCount := 0
	jobID := ""
	for current := range results {
		if current.err != nil {
			t.Fatalf("concurrent Enqueue() error = %v", current.err)
		}
		if current.inserted {
			insertedCount++
		}
		if jobID == "" {
			jobID = current.job.ID
		} else if current.job.ID != jobID {
			t.Fatalf("concurrent Enqueue() job ID = %q, want %q", current.job.ID, jobID)
		}
	}
	var count int64
	if err := pool.ORM().WithContext(ctx).Table("jobs").Where("intent_key = ?", intent.Key).Count(&count).Error; err != nil {
		t.Fatalf("count concurrent intents: %v", err)
	}
	if insertedCount != 1 || count != 1 {
		t.Fatalf("concurrent enqueue = %d inserts, %d rows", insertedCount, count)
	}
}

func assertConcurrentSingleClaim(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	store *Store,
	now time.Time,
	lease time.Duration,
) {
	t.Helper()
	if err := pool.ORM().WithContext(ctx).Exec("DELETE FROM jobs").Error; err != nil {
		t.Fatalf("clear jobs for concurrent claim: %v", err)
	}
	intent := mustIntent(t, "VERIFY", "verify:one", `{}`, 2, now)
	if _, _, err := store.Enqueue(ctx, intent, now); err != nil {
		t.Fatalf("Enqueue(concurrent job) error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 6)
	var waitGroup sync.WaitGroup
	for number := range 6 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			_, err := store.Claim(ctx, mustClaimInput(t, "worker-"+string(rune('a'+number)), now, lease))
			results <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	claimed := 0
	empty := 0
	for err := range results {
		switch {
		case err == nil:
			claimed++
		case errors.Is(err, jobs.ErrNoJob):
			empty++
		default:
			t.Fatalf("concurrent Claim() error = %v", err)
		}
	}
	if claimed != 1 || empty != 5 {
		t.Fatalf("concurrent claims = %d successful, %d empty", claimed, empty)
	}
}

func mustIntent(
	t *testing.T,
	kind string,
	key string,
	payload string,
	maxAttempts int,
	availableAt time.Time,
) jobs.Intent {
	t.Helper()
	intent, err := jobs.NewIntent(kind, key, json.RawMessage(payload), maxAttempts, availableAt)
	if err != nil {
		t.Fatalf("jobs.NewIntent() error = %v", err)
	}
	return intent
}

func mustClaim(
	t *testing.T,
	ctx context.Context,
	store *Store,
	workerID string,
	now time.Time,
	lease time.Duration,
) jobs.Job {
	t.Helper()
	job, err := store.Claim(ctx, mustClaimInput(t, workerID, now, lease))
	if err != nil {
		t.Fatalf("Claim(%s) error = %v", workerID, err)
	}
	return job
}

func mustClaimInput(t *testing.T, workerID string, now time.Time, lease time.Duration) jobs.Claim {
	t.Helper()
	claim, err := jobs.NewClaim(workerID, now, lease)
	if err != nil {
		t.Fatalf("jobs.NewClaim() error = %v", err)
	}
	return claim
}

func assertState(
	t *testing.T,
	ctx context.Context,
	pool *postgres.Pool,
	jobID string,
	state jobs.State,
	attempts int,
	errorCode string,
) {
	t.Helper()
	var record jobRecord
	if err := pool.ORM().WithContext(ctx).Where("id = ?", jobID).First(&record).Error; err != nil {
		t.Fatalf("read job state: %v", err)
	}
	if record.State != string(state) || record.AttemptCount != attempts {
		t.Fatalf("job record = %#v", record)
	}
	if errorCode == "" && record.LastErrorCode != nil ||
		errorCode != "" && (record.LastErrorCode == nil || *record.LastErrorCode != errorCode) {
		t.Fatalf("LastErrorCode = %#v, want %q", record.LastErrorCode, errorCode)
	}
}
