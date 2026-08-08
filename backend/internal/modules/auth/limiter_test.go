package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryLoginLimiterAppliesAccountAndClientWindows(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	limiter, err := NewMemoryLoginLimiter(MemoryLoginLimiterOptions{
		Window: time.Minute, AttemptsPerAccount: 2, AttemptsPerClient: 3,
		MaxEntries: 20, Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewMemoryLoginLimiter() error = %v", err)
	}

	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "Owner", Key: "client-a"}); err != nil {
		t.Fatalf("first Allow() error = %v", err)
	}
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "owner", Key: "client-b"}); err != nil {
		t.Fatalf("second Allow() error = %v", err)
	}
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "OWNER", Key: "client-c"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("account Allow() error = %v, want ErrRateLimited", err)
	}
	limiter.Succeeded(LoginAttempt{Username: "owner"})
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "owner", Key: "client-c"}); err != nil {
		t.Fatalf("account Allow() after success error = %v", err)
	}

	for _, username := range []string{"one", "two", "three"} {
		if err := limiter.Allow(context.Background(), LoginAttempt{Username: username, Key: "shared-client"}); err != nil {
			t.Fatalf("client Allow(%q) error = %v", username, err)
		}
	}
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "four", Key: "shared-client"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("client Allow() error = %v, want ErrRateLimited", err)
	}

	now = now.Add(time.Minute)
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "owner", Key: "shared-client"}); err != nil {
		t.Fatalf("Allow() after window error = %v", err)
	}
}

func TestMemoryLoginLimiterFailsClosedWhenStateIsFull(t *testing.T) {
	limiter, err := NewMemoryLoginLimiter(MemoryLoginLimiterOptions{
		Window: time.Hour, AttemptsPerAccount: 10, AttemptsPerClient: 10,
		MaxEntries: 2, Clock: func() time.Time { return time.Unix(0, 0) },
	})
	if err != nil {
		t.Fatalf("NewMemoryLoginLimiter() error = %v", err)
	}
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "one", Key: "client-one"}); err != nil {
		t.Fatalf("first Allow() error = %v", err)
	}
	if err := limiter.Allow(context.Background(), LoginAttempt{Username: "two", Key: "client-two"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("full Allow() error = %v, want ErrRateLimited", err)
	}
}
