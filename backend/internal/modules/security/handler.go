package security

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
)

type RegistryPullTokenIssuer interface {
	IssuePull(context.Context, string) (string, error)
}

type Scanner interface {
	Scan(context.Context, string, string) (ToolVersion, []Finding, error)
	GenerateSBOM(context.Context, string, string) (string, json.RawMessage, error)
}

type WorkflowService interface {
	ResolveJob(context.Context, jobs.Job) (Workflow, Target, error)
	SaveScanResult(context.Context, Workflow, ScanResult) error
	SaveSBOMResult(context.Context, Workflow, SBOMResult) error
}

type HandlerOptions struct {
	RegistryHost string
	Clock        func() time.Time
}

type Handlers struct {
	service WorkflowService
	scanner Scanner
	tokens  RegistryPullTokenIssuer
	options HandlerOptions
}

func NewHandlers(
	service WorkflowService,
	scanner Scanner,
	tokens RegistryPullTokenIssuer,
	options HandlerOptions,
) (*Handlers, error) {
	if service == nil || scanner == nil || tokens == nil || options.RegistryHost == "" ||
		options.Clock == nil {
		return nil, errors.New("security handler dependencies must be configured")
	}
	return &Handlers{service: service, scanner: scanner, tokens: tokens, options: options}, nil
}

func (h *Handlers) JobHandlers() map[jobs.Kind]jobs.Handler {
	return map[jobs.Kind]jobs.Handler{
		jobs.Kind(ScanJobKind): jobs.HandlerFunc(h.handleScan),
		jobs.Kind(SBOMJobKind): jobs.HandlerFunc(h.handleSBOM),
	}
}

func (h *Handlers) handleScan(ctx context.Context, job jobs.Job) error {
	workflow, target, err := h.service.ResolveJob(ctx, job)
	if err != nil {
		return classifyHandlerDependency("SECURITY_JOB_INVALID", "SECURITY_STATE_UNAVAILABLE", err)
	}
	reference, err := target.ImageReference(h.options.RegistryHost)
	if err != nil {
		return jobs.Permanent("SECURITY_JOB_INVALID", err)
	}
	token, err := h.tokens.IssuePull(ctx, target.RepositoryPath())
	if err != nil {
		return jobs.Retryable("REGISTRY_TOKEN_UNAVAILABLE", err)
	}
	version, findings, err := h.scanner.Scan(ctx, reference, token)
	if err != nil {
		return classifyToolError(err)
	}
	result := ScanResult{
		Target: target, ToolVersion: version, Findings: findings,
		CompletedAt: h.options.Clock().UTC().Round(time.Microsecond),
	}
	if err := h.service.SaveScanResult(ctx, workflow, result); err != nil {
		return classifyHandlerDependency("SCAN_RESULT_INVALID", "SECURITY_STATE_UNAVAILABLE", err)
	}
	return nil
}

func (h *Handlers) handleSBOM(ctx context.Context, job jobs.Job) error {
	workflow, target, err := h.service.ResolveJob(ctx, job)
	if err != nil {
		return classifyHandlerDependency("SECURITY_JOB_INVALID", "SECURITY_STATE_UNAVAILABLE", err)
	}
	reference, err := target.ImageReference(h.options.RegistryHost)
	if err != nil {
		return jobs.Permanent("SECURITY_JOB_INVALID", err)
	}
	token, err := h.tokens.IssuePull(ctx, target.RepositoryPath())
	if err != nil {
		return jobs.Retryable("REGISTRY_TOKEN_UNAVAILABLE", err)
	}
	generatorVersion, document, err := h.scanner.GenerateSBOM(ctx, reference, token)
	if err != nil {
		return classifyToolError(err)
	}
	result := SBOMResult{
		Target: target, GeneratorVersion: generatorVersion, Format: CycloneDXFormat,
		Document: document, CompletedAt: h.options.Clock().UTC().Round(time.Microsecond),
	}
	if err := h.service.SaveSBOMResult(ctx, workflow, result); err != nil {
		return classifyHandlerDependency("SBOM_RESULT_INVALID", "SECURITY_STATE_UNAVAILABLE", err)
	}
	return nil
}

func classifyToolError(err error) error {
	if errors.Is(err, ErrInvalidOutput) {
		return jobs.Permanent("SCANNER_OUTPUT_INVALID", err)
	}
	if errors.Is(err, ErrInvalid) {
		return jobs.Permanent("SCANNER_REQUEST_INVALID", err)
	}
	return jobs.Retryable("SCANNER_UNAVAILABLE", err)
}

func classifyHandlerDependency(invalidCode, unavailableCode string, err error) error {
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
		return jobs.Permanent(invalidCode, err)
	}
	return jobs.Retryable(unavailableCode, err)
}
