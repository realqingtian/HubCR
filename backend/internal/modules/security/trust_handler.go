package security

import (
	"context"
	"errors"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
)

type SignatureVerifier interface {
	Verify(context.Context, string, string, VerificationInput) (string, []CryptographicEvidence, error)
}

type VerificationService interface {
	ResolveVerificationJob(context.Context, jobs.Job) (VerificationInput, error)
	SaveVerificationResult(context.Context, VerificationResult) error
}

type TrustHandlers struct {
	service  VerificationService
	verifier SignatureVerifier
	tokens   RegistryPullTokenIssuer
	options  HandlerOptions
}

func NewTrustHandlers(
	service VerificationService,
	verifier SignatureVerifier,
	tokens RegistryPullTokenIssuer,
	options HandlerOptions,
) (*TrustHandlers, error) {
	if service == nil || verifier == nil || tokens == nil || options.RegistryHost == "" ||
		options.Clock == nil {
		return nil, errors.New("trust handler dependencies must be configured")
	}
	return &TrustHandlers{service: service, verifier: verifier, tokens: tokens, options: options}, nil
}

func (h *TrustHandlers) JobHandlers() map[jobs.Kind]jobs.Handler {
	return map[jobs.Kind]jobs.Handler{
		jobs.Kind(VerificationJobKind): jobs.HandlerFunc(h.handleVerification),
	}
}

func (h *TrustHandlers) handleVerification(ctx context.Context, job jobs.Job) error {
	input, err := h.service.ResolveVerificationJob(ctx, job)
	if err != nil {
		return classifyHandlerDependency("SIGNATURE_JOB_INVALID", "SECURITY_STATE_UNAVAILABLE", err)
	}
	reference, err := input.Workflow.Target.ImageReference(h.options.RegistryHost)
	if err != nil {
		return jobs.Permanent("SIGNATURE_JOB_INVALID", err)
	}
	token, err := h.tokens.IssuePull(ctx, input.Workflow.Target.RepositoryPath())
	if err != nil {
		return jobs.Retryable("REGISTRY_TOKEN_UNAVAILABLE", err)
	}
	version, cryptographicEvidence, err := h.verifier.Verify(ctx, reference, token, input)
	if err != nil {
		if errors.Is(err, ErrInvalidOutput) || errors.Is(err, ErrInvalid) {
			return jobs.Permanent("COSIGN_OUTPUT_INVALID", err)
		}
		return jobs.Retryable("COSIGN_UNAVAILABLE", err)
	}
	evidence, err := EvaluateTrust(input.Policy, cryptographicEvidence)
	if err != nil {
		return jobs.Permanent("SIGNATURE_EVIDENCE_INVALID", err)
	}
	result := VerificationResult{
		Workflow: input.Workflow, CosignVersion: version, Evidence: evidence,
		CompletedAt: h.options.Clock().UTC().Round(time.Microsecond),
	}
	if err := h.service.SaveVerificationResult(ctx, result); err != nil {
		return classifyHandlerDependency("SIGNATURE_RESULT_INVALID", "SECURITY_STATE_UNAVAILABLE", err)
	}
	return nil
}
