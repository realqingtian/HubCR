package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Store interface {
	Enqueue(context.Context, Intent, time.Time) (Job, bool, error)
	Claim(context.Context, Claim) (Job, error)
	ClaimKinds(context.Context, Claim, []Kind) (Job, error)
	Complete(context.Context, Completion) error
	Fail(context.Context, Failure) error
}

type Service struct {
	store Store
	clock func() time.Time
}

func NewService(store Store, clock func() time.Time) (*Service, error) {
	if store == nil || clock == nil {
		return nil, errors.New("job service dependencies must be configured")
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Enqueue(
	ctx context.Context,
	kind string,
	intentKey string,
	payload json.RawMessage,
	maxAttempts int,
	availableAt time.Time,
) (Job, bool, error) {
	intent, err := NewIntent(kind, intentKey, payload, maxAttempts, availableAt)
	if err != nil {
		return Job{}, false, err
	}
	return s.store.Enqueue(ctx, intent, s.now())
}

func (s *Service) Claim(ctx context.Context, workerID string, leaseDuration time.Duration) (Job, error) {
	claim, err := NewClaim(workerID, s.now(), leaseDuration)
	if err != nil {
		return Job{}, err
	}
	return s.store.Claim(ctx, claim)
}

func (s *Service) ClaimKinds(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
	kinds []Kind,
) (Job, error) {
	if len(kinds) == 0 {
		return Job{}, ErrNoJob
	}
	validatedKinds := make([]Kind, len(kinds))
	seen := make(map[Kind]struct{}, len(kinds))
	for index, kind := range kinds {
		validated, err := ParseKind(string(kind))
		if err != nil {
			return Job{}, err
		}
		if _, exists := seen[validated]; exists {
			return Job{}, ErrInvalidJob
		}
		seen[validated] = struct{}{}
		validatedKinds[index] = validated
	}
	claim, err := NewClaim(workerID, s.now(), leaseDuration)
	if err != nil {
		return Job{}, err
	}
	return s.store.ClaimKinds(ctx, claim, validatedKinds)
}

func (s *Service) Complete(ctx context.Context, job Job, workerID string) error {
	completion, err := NewCompletion(job.ID, workerID, s.now())
	if err != nil {
		return err
	}
	return s.store.Complete(ctx, completion)
}

func (s *Service) Fail(
	ctx context.Context,
	job Job,
	workerID string,
	code string,
	retryAfter time.Duration,
	terminal bool,
) error {
	if retryAfter < 0 {
		return ErrInvalidJob
	}
	now := s.now()
	failure, err := NewFailure(job.ID, workerID, code, now, now.Add(retryAfter), terminal)
	if err != nil {
		return err
	}
	return s.store.Fail(ctx, failure)
}

func (s *Service) now() time.Time {
	return s.clock().UTC().Round(time.Microsecond)
}
