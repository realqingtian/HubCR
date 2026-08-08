package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	DefaultLoginLimitWindow        = time.Minute
	DefaultLoginAttemptsPerAccount = 10
	DefaultLoginAttemptsPerClient  = 60
	DefaultLoginLimitEntries       = 10_000
)

type MemoryLoginLimiterOptions struct {
	Window             time.Duration
	AttemptsPerAccount int
	AttemptsPerClient  int
	MaxEntries         int
	Clock              func() time.Time
}

type loginLimitEntry struct {
	windowStarted time.Time
	attempts      int
}

// MemoryLoginLimiter is the single-process MVP admission control. A shared
// Redis-backed implementation remains necessary before horizontal scaling.
type MemoryLoginLimiter struct {
	window             time.Duration
	attemptsPerAccount int
	attemptsPerClient  int
	maxEntries         int
	clock              func() time.Time
	mu                 sync.Mutex
	entries            map[string]loginLimitEntry
}

func NewMemoryLoginLimiter(options MemoryLoginLimiterOptions) (*MemoryLoginLimiter, error) {
	if options.Window <= 0 || options.AttemptsPerAccount <= 0 ||
		options.AttemptsPerClient <= 0 || options.MaxEntries < 2 || options.Clock == nil {
		return nil, errors.New("login limiter options must be configured")
	}
	return &MemoryLoginLimiter{
		window:             options.Window,
		attemptsPerAccount: options.AttemptsPerAccount,
		attemptsPerClient:  options.AttemptsPerClient,
		maxEntries:         options.MaxEntries,
		clock:              options.Clock,
		entries:            make(map[string]loginLimitEntry),
	}, nil
}

func (l *MemoryLoginLimiter) Allow(_ context.Context, attempt LoginAttempt) error {
	now := l.clock().UTC()
	keys := []struct {
		value string
		limit int
	}{
		{value: "account\x00" + strings.ToLower(strings.TrimSpace(attempt.Username)), limit: l.attemptsPerAccount},
		{value: "client\x00" + attempt.Key, limit: l.attemptsPerClient},
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	newEntries := 0
	for _, key := range keys {
		entry, exists := l.entries[key.value]
		if exists && !now.Before(entry.windowStarted.Add(l.window)) {
			delete(l.entries, key.value)
			exists = false
		}
		if !exists {
			newEntries++
			continue
		}
		if entry.attempts >= key.limit {
			return ErrRateLimited
		}
	}
	if len(l.entries)+newEntries > l.maxEntries {
		l.removeExpired(now)
		if len(l.entries)+newEntries > l.maxEntries {
			return ErrRateLimited
		}
	}
	for _, key := range keys {
		entry, exists := l.entries[key.value]
		if !exists {
			entry.windowStarted = now
		}
		entry.attempts++
		l.entries[key.value] = entry
	}
	return nil
}

// Succeeded clears only the account bucket so legitimate Registry token refreshes do
// not consume the failed-password budget. The client bucket remains to bound work.
func (l *MemoryLoginLimiter) Succeeded(attempt LoginAttempt) {
	l.mu.Lock()
	delete(l.entries, "account\x00"+strings.ToLower(strings.TrimSpace(attempt.Username)))
	l.mu.Unlock()
}

func (l *MemoryLoginLimiter) removeExpired(now time.Time) {
	for key, entry := range l.entries {
		if !now.Before(entry.windowStarted.Add(l.window)) {
			delete(l.entries, key)
		}
	}
}

var _ LoginLimiter = (*MemoryLoginLimiter)(nil)
