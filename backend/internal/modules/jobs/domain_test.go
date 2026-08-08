package jobs

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewIntentNormalizesAndValidates(t *testing.T) {
	now := time.Date(2026, 8, 8, 1, 2, 3, 456789123, time.UTC)
	intent, err := NewIntent(
		"TRIVY_SCAN", "scan:repository:digest:policy-1", json.RawMessage(`{ "digest": "sha256:a" }`),
		5, now,
	)
	if err != nil {
		t.Fatalf("NewIntent() error = %v", err)
	}
	if string(intent.Payload) != `{"digest":"sha256:a"}` ||
		intent.AvailableAt.Nanosecond() != 456789000 {
		t.Fatalf("intent = %#v", intent)
	}

	for _, test := range []struct {
		name      string
		kind      string
		key       string
		payload   json.RawMessage
		attempts  int
		available time.Time
	}{
		{name: "lowercase kind", kind: "scan", key: "key", payload: json.RawMessage(`{}`), attempts: 1, available: now},
		{name: "space in key", kind: "SCAN", key: "bad key", payload: json.RawMessage(`{}`), attempts: 1, available: now},
		{name: "array payload", kind: "SCAN", key: "key", payload: json.RawMessage(`[]`), attempts: 1, available: now},
		{name: "invalid JSON", kind: "SCAN", key: "key", payload: json.RawMessage(`{`), attempts: 1, available: now},
		{name: "no attempts", kind: "SCAN", key: "key", payload: json.RawMessage(`{}`), attempts: 0, available: now},
		{name: "zero time", kind: "SCAN", key: "key", payload: json.RawMessage(`{}`), attempts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewIntent(test.kind, test.key, test.payload, test.attempts, test.available); !errors.Is(err, ErrInvalidJob) {
				t.Fatalf("NewIntent() error = %v, want ErrInvalidJob", err)
			}
		})
	}
}

func TestJobValidateStateInvariants(t *testing.T) {
	now := time.Date(2026, 8, 8, 1, 0, 0, 0, time.UTC)
	expires := now.Add(time.Minute)
	started := now
	valid := Job{
		ID: "job-id", Kind: Kind("SCAN"), IntentKey: "scan:key", Payload: json.RawMessage(`{}`),
		State: StateRunning, Attempts: 1, MaxAttempts: 3, AvailableAt: now,
		LeaseOwner: "worker-1", LeaseExpiresAt: &expires, CreatedAt: now, UpdatedAt: now,
		StartedAt: &started,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid running job) error = %v", err)
	}

	invalid := valid
	invalid.LeaseOwner = ""
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("Validate(invalid running job) error = %v", err)
	}

	completed := now.Add(time.Second)
	valid.State = StateSucceeded
	valid.LeaseOwner = ""
	valid.LeaseExpiresAt = nil
	valid.CompletedAt = &completed
	valid.UpdatedAt = completed
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid completed job) error = %v", err)
	}
}

func TestNewClaimAndFailureValidation(t *testing.T) {
	now := time.Date(2026, 8, 8, 1, 0, 0, 0, time.UTC)
	if _, err := NewClaim("worker-1", now, time.Minute); err != nil {
		t.Fatalf("NewClaim() error = %v", err)
	}
	if _, err := NewClaim("bad worker", now, time.Minute); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("NewClaim(invalid worker) error = %v", err)
	}
	if _, err := NewFailure("job", "worker-1", "TRANSIENT_FAILURE", now, now.Add(time.Second), false); err != nil {
		t.Fatalf("NewFailure() error = %v", err)
	}
	if _, err := NewFailure("job", "worker-1", "bad-code", now, now.Add(time.Second), false); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("NewFailure(invalid code) error = %v", err)
	}
}
