package security

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
)

type Store interface {
	EnsureWorkflow(context.Context, Target, time.Time) (Workflow, bool, error)
	RepairMissingWorkflows(context.Context, int, time.Time) (int, error)
	ResolveJob(context.Context, jobs.Job) (Workflow, Target, error)
	SaveScanResult(context.Context, Workflow, ScanResult) error
	SaveSBOMResult(context.Context, Workflow, SBOMResult) error
	Detail(context.Context, string, string) (Detail, error)
}

type Service struct {
	store Store
	clock func() time.Time
}

func NewService(store Store, clock func() time.Time) (*Service, error) {
	if store == nil || clock == nil {
		return nil, errors.New("security service dependencies must be configured")
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) EnsureWorkflow(ctx context.Context, target Target) (Workflow, bool, error) {
	validated, err := NewTarget(
		target.RepositoryID, target.Namespace, target.Repository, target.Digest.String(),
	)
	if err != nil {
		return Workflow{}, false, err
	}
	return s.store.EnsureWorkflow(ctx, validated, s.now())
}

func (s *Service) RepairMissingWorkflows(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > MaxRepairBatch {
		return 0, ErrInvalid
	}
	return s.store.RepairMissingWorkflows(ctx, limit, s.now())
}

func (s *Service) ResolveJob(ctx context.Context, job jobs.Job) (Workflow, Target, error) {
	if job.Kind != jobs.Kind(ScanJobKind) && job.Kind != jobs.Kind(SBOMJobKind) {
		return Workflow{}, Target{}, ErrInvalid
	}
	if _, err := ParseJobPayload(job.Payload); err != nil {
		return Workflow{}, Target{}, err
	}
	return s.store.ResolveJob(ctx, job)
}

func (s *Service) SaveScanResult(ctx context.Context, workflow Workflow, result ScanResult) error {
	if err := workflow.Validate(); err != nil || result.Validate() != nil ||
		workflow.Target != result.Target {
		return ErrInvalid
	}
	return s.store.SaveScanResult(ctx, workflow, result)
}

func (s *Service) SaveSBOMResult(ctx context.Context, workflow Workflow, result SBOMResult) error {
	if err := workflow.Validate(); err != nil || result.Validate() != nil ||
		workflow.Target != result.Target {
		return ErrInvalid
	}
	return s.store.SaveSBOMResult(ctx, workflow, result)
}

func (s *Service) Detail(ctx context.Context, repositoryID, rawDigest string) (Detail, error) {
	target, err := NewTarget(repositoryID, "placeholder", "placeholder", rawDigest)
	if err != nil {
		return Detail{}, err
	}
	return s.store.Detail(ctx, target.RepositoryID, target.Digest.String())
}

func MarshalJobPayload(target Target) (json.RawMessage, error) {
	payload, err := NewJobPayload(target)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func (s *Service) now() time.Time { return s.clock().UTC().Round(time.Microsecond) }
