package security

import (
	"context"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/jobs"
)

func TestTrustHandlerPersistsSeparateCryptographicAndPolicyTrust(t *testing.T) {
	now := time.Date(2026, 8, 9, 5, 0, 0, 0, time.UTC)
	input := trustHandlerInput(t, now)
	service := &trustHandlerService{input: input}
	verifier := &trustHandlerVerifier{version: "v3.0.6", evidence: []CryptographicEvidence{
		trustHandlerEvidence(t, "a", SignerPublicKey, input.Policy.PublicKeys[0].Fingerprint, CryptoValid),
		trustHandlerEvidence(t, "b", SignerPublicKey, input.CandidateKeys[1].Fingerprint, CryptoValid),
		trustHandlerEvidence(t, "c", SignerUnknown, "", CryptoUnverified),
	}}
	tokens := &handlerTokens{token: "pull-token"}
	handlers, err := NewTrustHandlers(
		service, verifier, tokens,
		HandlerOptions{RegistryHost: "registry:5000", Clock: func() time.Time { return now }},
	)
	if err != nil {
		t.Fatalf("NewTrustHandlers() error = %v", err)
	}
	job := jobs.Job{
		ID: input.Workflow.JobID, Kind: jobs.Kind(VerificationJobKind), IntentKey: "intent",
		Payload: []byte(`{"workflow_version":"signature-v1","repository_id":"repository","namespace":"team","repository":"app","digest":"sha256:` + strings.Repeat("a", 64) + `","policy_id":"policy","policy_version":2}`),
		State:   jobs.StateRunning, MaxAttempts: 3, Attempts: 1,
		CreatedAt: now, UpdatedAt: now, AvailableAt: now,
		LeaseOwner: "worker", LeaseExpiresAt: trustTimePointer(now.Add(time.Minute)),
		StartedAt: trustTimePointer(now),
	}
	if err := handlers.JobHandlers()[jobs.Kind(VerificationJobKind)].Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if service.saved.CosignVersion != "v3.0.6" || len(service.saved.Evidence) != 3 ||
		service.saved.Evidence[0].TrustState != TrustTrusted ||
		service.saved.Evidence[1].TrustState != TrustUntrusted ||
		service.saved.Evidence[2].TrustState != TrustNotEvaluated {
		t.Fatalf("saved result = %#v", service.saved)
	}
	if tokens.repository != "team/app" || verifier.token != "pull-token" ||
		!strings.Contains(verifier.reference, "@sha256:") {
		t.Fatalf("token/reference = %q, %q, %q", tokens.repository, verifier.token, verifier.reference)
	}
}

func TestTrustHandlerClassifiesCosignDependencyUnavailableAsRetryable(t *testing.T) {
	now := time.Now().UTC()
	input := trustHandlerInput(t, now)
	handlers, err := NewTrustHandlers(
		&trustHandlerService{input: input},
		&trustHandlerVerifier{err: ErrToolFailure},
		&handlerTokens{token: "pull-token"},
		HandlerOptions{RegistryHost: "registry:5000", Clock: func() time.Time { return now }},
	)
	if err != nil {
		t.Fatalf("NewTrustHandlers() error = %v", err)
	}
	job := jobs.Job{
		ID: input.Workflow.JobID, Kind: jobs.Kind(VerificationJobKind), IntentKey: "intent",
		Payload: []byte(`{"workflow_version":"signature-v1","repository_id":"repository","namespace":"team","repository":"app","digest":"sha256:` + strings.Repeat("a", 64) + `","policy_id":"policy","policy_version":2}`),
		State:   jobs.StateRunning, MaxAttempts: 3, Attempts: 1,
		CreatedAt: now, UpdatedAt: now, AvailableAt: now,
		LeaseOwner: "worker", LeaseExpiresAt: trustTimePointer(now.Add(time.Minute)),
		StartedAt: trustTimePointer(now),
	}
	err = handlers.JobHandlers()[jobs.Kind(VerificationJobKind)].Handle(context.Background(), job)
	code, terminal := jobs.ClassifyHandlerError(err)
	if code != "COSIGN_UNAVAILABLE" || terminal {
		t.Fatalf("classified error = %q terminal=%t", code, terminal)
	}
}

type trustHandlerService struct {
	input VerificationInput
	saved VerificationResult
}

func (s *trustHandlerService) ResolveVerificationJob(context.Context, jobs.Job) (VerificationInput, error) {
	return s.input, nil
}

func (s *trustHandlerService) SaveVerificationResult(_ context.Context, result VerificationResult) error {
	s.saved = result
	return nil
}

type trustHandlerVerifier struct {
	version   string
	evidence  []CryptographicEvidence
	reference string
	token     string
	err       error
}

func (v *trustHandlerVerifier) Verify(
	_ context.Context,
	reference, token string,
	_ VerificationInput,
) (string, []CryptographicEvidence, error) {
	v.reference, v.token = reference, token
	return v.version, v.evidence, v.err
}

func trustHandlerInput(t *testing.T, now time.Time) VerificationInput {
	t.Helper()
	target, _ := NewTarget("repository", "team", "app", "sha256:"+strings.Repeat("a", 64))
	current := testPublicKey(t, "current")
	old := testPublicKey(t, "old")
	policy := TrustPolicy{
		ID: "policy", NamespaceID: "namespace", Version: 2, CreatedByUserID: "user",
		PublicKeys: []PublicKeyTrust{current}, CreatedAt: now,
	}
	return VerificationInput{
		Workflow: VerificationWorkflow{
			ID: "workflow", Target: target, PolicyID: policy.ID,
			PolicyVersion: policy.Version, JobID: "job", CreatedAt: now,
		},
		Policy: policy, CandidateKeys: []PublicKeyTrust{current, old},
	}
}

func trustHandlerEvidence(
	t *testing.T,
	digit string,
	signer SignerType,
	fingerprint string,
	state CryptographicState,
) CryptographicEvidence {
	t.Helper()
	digest, err := artifacts.ParseDigest("sha256:" + strings.Repeat(digit, 64))
	if err != nil {
		t.Fatalf("ParseDigest() error = %v", err)
	}
	return CryptographicEvidence{
		SignatureDigest: digest, Kind: SignatureKindSignature,
		SignerType: signer, KeyFingerprint: fingerprint, State: state,
	}
}

func trustTimePointer(value time.Time) *time.Time { return &value }
