package security

import (
	"context"
	"errors"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
)

type TrustStore interface {
	CreateTrustPolicy(context.Context, string, string, []PublicKeyTrust, []KeylessIdentity, time.Time) (TrustPolicy, error)
	EnsureCurrentVerification(context.Context, Target, time.Time) (VerificationWorkflow, bool, error)
	RepairMissingVerificationWorkflows(context.Context, int, time.Time) (int, error)
	ResolveVerificationJob(context.Context, jobs.Job) (VerificationInput, error)
	SaveVerificationResult(context.Context, VerificationResult) error
}

type TrustService struct {
	store TrustStore
	clock func() time.Time
}

func NewTrustService(store TrustStore, clock func() time.Time) (*TrustService, error) {
	if store == nil || clock == nil {
		return nil, errors.New("trust service dependencies must be configured")
	}
	return &TrustService{store: store, clock: clock}, nil
}

func (s *TrustService) CreatePolicy(
	ctx context.Context,
	namespaceID, actorID string,
	keys []PublicKeyTrust,
	identities []KeylessIdentity,
) (TrustPolicy, error) {
	if namespaceID == "" || actorID == "" || len(keys)+len(identities) < 1 ||
		len(keys)+len(identities) > MaxTrustPolicySubjects {
		return TrustPolicy{}, ErrInvalid
	}
	for _, key := range keys {
		if key.Validate() != nil {
			return TrustPolicy{}, ErrInvalid
		}
	}
	for _, identity := range identities {
		if identity.Validate() != nil {
			return TrustPolicy{}, ErrInvalid
		}
	}
	return s.store.CreateTrustPolicy(ctx, namespaceID, actorID, keys, identities, s.now())
}

func (s *TrustService) EnsureCurrentVerification(
	ctx context.Context,
	target Target,
) (VerificationWorkflow, bool, error) {
	validated, err := NewTarget(target.RepositoryID, target.Namespace, target.Repository, target.Digest.String())
	if err != nil {
		return VerificationWorkflow{}, false, err
	}
	return s.store.EnsureCurrentVerification(ctx, validated, s.now())
}

func (s *TrustService) RepairMissingVerificationWorkflows(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > MaxRepairBatch {
		return 0, ErrInvalid
	}
	return s.store.RepairMissingVerificationWorkflows(ctx, limit, s.now())
}

func (s *TrustService) ResolveVerificationJob(ctx context.Context, job jobs.Job) (VerificationInput, error) {
	if job.Kind != jobs.Kind(VerificationJobKind) {
		return VerificationInput{}, ErrInvalid
	}
	if _, _, _, err := ParseVerificationPayload(job.Payload); err != nil {
		return VerificationInput{}, err
	}
	return s.store.ResolveVerificationJob(ctx, job)
}

func (s *TrustService) SaveVerificationResult(ctx context.Context, result VerificationResult) error {
	if result.Validate() != nil {
		return ErrInvalid
	}
	return s.store.SaveVerificationResult(ctx, result)
}

func (s *TrustService) now() time.Time { return s.clock().UTC().Round(time.Microsecond) }
