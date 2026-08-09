package security

import (
	"context"
	"errors"
	"time"

	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/modules/organizations"
)

type TrustStore interface {
	CreateTrustPolicy(context.Context, string, string, []PublicKeyTrust, []KeylessIdentity, time.Time) (TrustPolicy, error)
	CurrentTrustPolicy(context.Context, string) (TrustPolicy, error)
	EnsureCurrentVerification(context.Context, Target, time.Time) (VerificationWorkflow, bool, error)
	RepairMissingVerificationWorkflows(context.Context, int, time.Time) (int, error)
	RepairMissingVerificationWorkflowsForNamespace(context.Context, string, int, time.Time) (int, error)
	ResolveVerificationJob(context.Context, jobs.Job) (VerificationInput, error)
	SaveVerificationResult(context.Context, VerificationResult) error
}

// TrustAuthorization is the narrow authorization decider TrustService uses to evaluate a
// resolved namespace access value against a capability. authorization.Policy satisfies it.
type TrustAuthorization interface {
	AllowsOrganization(organizations.Role, authorization.Capability) bool
	AllowsPersonalNamespace(bool, authorization.Capability) bool
}

// NamespaceAccess describes the actor's relationship to a namespace, resolved by the
// caller (the HTTP layer) from the canonical membership/ownership stores. Keeping it as a
// value type here lets TrustService enforce trust-policy authorization without importing
// the repositories module.
type NamespaceAccess struct {
	NamespaceID      string
	Kind             NamespaceKind
	IsPersonalOwner  bool
	OrganizationRole organizations.Role
}

// NamespaceKind mirrors repositories.NamespaceKind for the access value used by the
// trust-policy authorization path.
type NamespaceKind string

const (
	NamespacePersonal     NamespaceKind = "PERSONAL"
	NamespaceOrganization NamespaceKind = "ORGANIZATION"
)

// NamespaceAccessResolver resolves a namespace name and actor into a NamespaceAccess
// value. Implementations return ErrNotFound when the namespace does not exist so callers
// can map that to 404 without leaking existence.
type NamespaceAccessResolver func(ctx context.Context, namespaceName, actorUserID string) (NamespaceAccess, error)

// NamespaceRepairer eagerly enqueues re-verification for one namespace. The
// TrustService.RepairTrustVerificationForNamespace method satisfies it.
type NamespaceRepairer func(ctx context.Context, namespaceID string, limit int) (int, error)

type TrustService struct {
	store   TrustStore
	auth    TrustAuthorization
	resolve NamespaceAccessResolver
	clock   func() time.Time
}

// NewTrustService constructs a TrustService for worker-side use. Only the repair and
// verification paths are exercised by the worker, so the authorization dependencies are
// not required; CreatePolicy and CurrentPolicy are unavailable on this instance.
func NewTrustService(store TrustStore, clock func() time.Time) (*TrustService, error) {
	if store == nil || clock == nil {
		return nil, errors.New("trust service dependencies must be configured")
	}
	return &TrustService{store: store, clock: clock}, nil
}

// NewManagingTrustService constructs a TrustService with the authorization dependencies
// required to create and read trust policies through the HTTP management API. The worker
// must keep using NewTrustService.
func NewManagingTrustService(
	store TrustStore,
	clock func() time.Time,
	auth TrustAuthorization,
	resolve NamespaceAccessResolver,
) (*TrustService, error) {
	if store == nil || clock == nil {
		return nil, errors.New("trust service dependencies must be configured")
	}
	if auth == nil || resolve == nil {
		return nil, errors.New("trust service authorization dependencies must be configured")
	}
	return &TrustService{store: store, auth: auth, resolve: resolve, clock: clock}, nil
}

// CreatePolicy creates a new append-only trust-policy version after authorizing the actor
// as the namespace owner (ManageTrustPolicy). After the commit succeeds it eagerly
// triggers namespace-scoped re-verification; a repair failure is non-fatal because the
// periodic worker loop backfills any remainder.
func (s *TrustService) CreatePolicy(
	ctx context.Context,
	namespaceName, actorID string,
	keys []PublicKeyTrust,
	identities []KeylessIdentity,
) (TrustPolicy, error) {
	if !s.manageable() {
		return TrustPolicy{}, ErrForbidden
	}
	if actorID == "" {
		return TrustPolicy{}, ErrForbidden
	}
	access, err := s.resolve(ctx, namespaceName, actorID)
	if err != nil {
		return TrustPolicy{}, err
	}
	if !s.allows(access, authorization.ManageTrustPolicy) {
		return TrustPolicy{}, ErrForbidden
	}
	if len(keys)+len(identities) < 1 || len(keys)+len(identities) > MaxTrustPolicySubjects {
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
	policy, err := s.store.CreateTrustPolicy(ctx, access.NamespaceID, actorID, keys, identities, s.now())
	if err != nil {
		return TrustPolicy{}, err
	}
	_, _ = s.RepairTrustVerificationForNamespace(ctx, access.NamespaceID, MaxRepairBatch)
	return policy, nil
}

// CurrentPolicy returns the highest-version trust policy for the namespace, or ErrNotFound
// when none exists. Read access is gated by ViewOrganization; non-members get ErrForbidden
// (which the HTTP layer maps to 404 to avoid leaking namespace existence).
func (s *TrustService) CurrentPolicy(ctx context.Context, namespaceName, actorID string) (TrustPolicy, error) {
	if !s.manageable() {
		return TrustPolicy{}, ErrForbidden
	}
	access, err := s.resolve(ctx, namespaceName, actorID)
	if err != nil {
		return TrustPolicy{}, err
	}
	if !s.allows(access, authorization.ViewOrganization) {
		return TrustPolicy{}, ErrForbidden
	}
	return s.store.CurrentTrustPolicy(ctx, access.NamespaceID)
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

// RepairTrustVerificationForNamespace eagerly enqueues re-verification for the artifacts
// in a single namespace that are missing a workflow for the namespace's current policy.
// It is the eager trigger invoked after CreatePolicy commits and is also exposed so the
// HTTP layer can wire it as a NamespaceRepairer.
func (s *TrustService) RepairTrustVerificationForNamespace(ctx context.Context, namespaceID string, limit int) (int, error) {
	if namespaceID == "" || limit < 1 || limit > MaxRepairBatch {
		return 0, ErrInvalid
	}
	return s.store.RepairMissingVerificationWorkflowsForNamespace(ctx, namespaceID, limit, s.now())
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

func (s *TrustService) allows(access NamespaceAccess, capability authorization.Capability) bool {
	switch access.Kind {
	case NamespacePersonal:
		return s.auth.AllowsPersonalNamespace(access.IsPersonalOwner, capability)
	case NamespaceOrganization:
		return s.auth.AllowsOrganization(access.OrganizationRole, capability)
	default:
		return false
	}
}

// manageable reports whether this instance was constructed with the authorization
// dependencies required for the management (CreatePolicy / CurrentPolicy) paths.
func (s *TrustService) manageable() bool { return s.auth != nil && s.resolve != nil }

func (s *TrustService) now() time.Time { return s.clock().UTC().Round(time.Microsecond) }
