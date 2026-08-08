package security

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"hubcr.io/hubcr/internal/modules/artifacts"
)

const (
	VerificationJobKind        = "COSIGN_VERIFY"
	VerificationPayloadV1      = "signature-v1"
	CosignName                 = "COSIGN"
	MaxPublicKeyBytes          = 16 * 1024
	MaxTrustSubjectBytes       = 2048
	MaxTrustPolicySubjects     = 128
	SignatureReasonTrustedKey  = "TRUSTED_PUBLIC_KEY"
	SignatureReasonTrustedID   = "TRUSTED_KEYLESS_IDENTITY"
	SignatureReasonUntrusted   = "VALID_UNTRUSTED_SIGNER"
	SignatureReasonInvalid     = "SIGNATURE_INVALID"
	SignatureReasonUnknown     = "SIGNER_UNVERIFIED"
	SignatureReasonUnavailable = "VERIFICATION_DEPENDENCY_UNAVAILABLE"
)

type PublicKeyTrust struct {
	Fingerprint  string
	Name         string
	PublicKeyPEM string
}

func NewPublicKeyTrust(name string, raw []byte) (PublicKeyTrust, error) {
	if !validTrustText(name, 128) || len(raw) == 0 || len(raw) > MaxPublicKeyBytes {
		return PublicKeyTrust{}, ErrInvalid
	}
	block, trailing := pem.Decode(raw)
	if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(trailing)) != 0 {
		return PublicKeyTrust{}, ErrInvalid
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return PublicKeyTrust{}, ErrInvalid
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return PublicKeyTrust{}, ErrInvalid
	}
	sum := sha256.Sum256(der)
	canonical := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return PublicKeyTrust{
		Fingerprint:  "sha256:" + hex.EncodeToString(sum[:]),
		Name:         name,
		PublicKeyPEM: string(canonical),
	}, nil
}

func (k PublicKeyTrust) Validate() error {
	canonical, err := NewPublicKeyTrust(k.Name, []byte(k.PublicKeyPEM))
	if err != nil || canonical.Fingerprint != k.Fingerprint || canonical.PublicKeyPEM != k.PublicKeyPEM {
		return ErrInvalid
	}
	return nil
}

type KeylessIdentity struct {
	Issuer  string
	Subject string
}

func NewKeylessIdentity(issuer, subject string) (KeylessIdentity, error) {
	if !validExactIssuer(issuer) || !validTrustText(subject, MaxTrustSubjectBytes) ||
		strings.ContainsAny(issuer+subject, "*") {
		return KeylessIdentity{}, ErrInvalid
	}
	return KeylessIdentity{Issuer: issuer, Subject: subject}, nil
}

func (i KeylessIdentity) Validate() error {
	_, err := NewKeylessIdentity(i.Issuer, i.Subject)
	return err
}

type TrustPolicy struct {
	ID                string
	NamespaceID       string
	Version           int64
	CreatedByUserID   string
	PublicKeys        []PublicKeyTrust
	KeylessIdentities []KeylessIdentity
	CreatedAt         time.Time
}

func (p TrustPolicy) Validate() error {
	if p.ID == "" || p.NamespaceID == "" || p.CreatedByUserID == "" || p.Version < 1 ||
		p.CreatedAt.IsZero() || len(p.PublicKeys)+len(p.KeylessIdentities) < 1 ||
		len(p.PublicKeys)+len(p.KeylessIdentities) > MaxTrustPolicySubjects {
		return ErrInvalid
	}
	keys := map[string]struct{}{}
	for _, key := range p.PublicKeys {
		if key.Validate() != nil {
			return ErrInvalid
		}
		if _, exists := keys[key.Fingerprint]; exists {
			return ErrInvalid
		}
		keys[key.Fingerprint] = struct{}{}
	}
	identities := map[string]struct{}{}
	for _, identity := range p.KeylessIdentities {
		if identity.Validate() != nil {
			return ErrInvalid
		}
		key := identity.Issuer + "\x00" + identity.Subject
		if _, exists := identities[key]; exists {
			return ErrInvalid
		}
		identities[key] = struct{}{}
	}
	return nil
}

type VerificationJobPayload struct {
	WorkflowVersion string `json:"workflow_version"`
	RepositoryID    string `json:"repository_id"`
	Namespace       string `json:"namespace"`
	Repository      string `json:"repository"`
	Digest          string `json:"digest"`
	PolicyID        string `json:"policy_id"`
	PolicyVersion   int64  `json:"policy_version"`
}

func MarshalVerificationPayload(target Target, policy TrustPolicy) (json.RawMessage, error) {
	if _, err := NewTarget(target.RepositoryID, target.Namespace, target.Repository, target.Digest.String()); err != nil || policy.Validate() != nil {
		return nil, ErrInvalid
	}
	payload := VerificationJobPayload{
		WorkflowVersion: VerificationPayloadV1,
		RepositoryID:    target.RepositoryID,
		Namespace:       target.Namespace,
		Repository:      target.Repository,
		Digest:          target.Digest.String(),
		PolicyID:        policy.ID,
		PolicyVersion:   policy.Version,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func ParseVerificationPayload(raw json.RawMessage) (Target, string, int64, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload VerificationJobPayload
	if err := decoder.Decode(&payload); err != nil {
		return Target{}, "", 0, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Target{}, "", 0, ErrInvalid
	}
	target, err := NewTarget(payload.RepositoryID, payload.Namespace, payload.Repository, payload.Digest)
	if err != nil || payload.WorkflowVersion != VerificationPayloadV1 || payload.PolicyID == "" || payload.PolicyVersion < 1 {
		return Target{}, "", 0, ErrInvalid
	}
	return target, payload.PolicyID, payload.PolicyVersion, nil
}

func VerificationIntentKey(target Target, policy TrustPolicy) (string, error) {
	if _, err := NewTarget(target.RepositoryID, target.Namespace, target.Repository, target.Digest.String()); err != nil || policy.Validate() != nil {
		return "", ErrInvalid
	}
	return "security-signature:" + target.RepositoryID + ":" + target.Digest.String() + ":policy:" + policy.ID, nil
}

type VerificationWorkflow struct {
	ID            string
	Target        Target
	PolicyID      string
	PolicyVersion int64
	JobID         string
	CreatedAt     time.Time
}

type VerificationInput struct {
	Workflow            VerificationWorkflow
	Policy              TrustPolicy
	CandidateKeys       []PublicKeyTrust
	CandidateIdentities []KeylessIdentity
}

func (i VerificationInput) Validate() error {
	if i.Workflow.Validate() != nil || i.Policy.Validate() != nil ||
		i.Workflow.PolicyID != i.Policy.ID || i.Workflow.PolicyVersion != i.Policy.Version ||
		i.Workflow.Target.Namespace == "" {
		return ErrInvalid
	}
	for _, key := range i.CandidateKeys {
		if key.Validate() != nil {
			return ErrInvalid
		}
	}
	for _, identity := range i.CandidateIdentities {
		if identity.Validate() != nil {
			return ErrInvalid
		}
	}
	return nil
}

func (w VerificationWorkflow) Validate() error {
	if w.ID == "" || w.PolicyID == "" || w.PolicyVersion < 1 || w.JobID == "" || w.CreatedAt.IsZero() {
		return ErrInvalid
	}
	_, err := NewTarget(w.Target.RepositoryID, w.Target.Namespace, w.Target.Repository, w.Target.Digest.String())
	return err
}

type SignatureKind string
type SignerType string
type CryptographicState string
type PolicyTrustState string

const (
	SignatureKindSignature   SignatureKind      = "SIGNATURE"
	SignatureKindAttestation SignatureKind      = "ATTESTATION"
	SignerPublicKey          SignerType         = "PUBLIC_KEY"
	SignerKeyless            SignerType         = "KEYLESS"
	SignerUnknown            SignerType         = "UNKNOWN"
	CryptoValid              CryptographicState = "VALID"
	CryptoInvalid            CryptographicState = "INVALID"
	CryptoUnverified         CryptographicState = "UNVERIFIED"
	CryptoUnavailable        CryptographicState = "UNAVAILABLE"
	TrustTrusted             PolicyTrustState   = "TRUSTED"
	TrustUntrusted           PolicyTrustState   = "UNTRUSTED"
	TrustNotEvaluated        PolicyTrustState   = "NOT_EVALUATED"
)

type CryptographicEvidence struct {
	SignatureDigest artifacts.Digest
	Kind            SignatureKind
	SignerType      SignerType
	KeyFingerprint  string
	OIDCIssuer      string
	Subject         string
	State           CryptographicState
}

func (e CryptographicEvidence) Validate() error {
	if _, err := artifacts.ParseDigest(e.SignatureDigest.String()); err != nil {
		return ErrInvalid
	}
	if e.Kind != SignatureKindSignature && e.Kind != SignatureKindAttestation {
		return ErrInvalid
	}
	switch e.State {
	case CryptoValid, CryptoInvalid, CryptoUnverified, CryptoUnavailable:
	default:
		return ErrInvalid
	}
	switch e.SignerType {
	case SignerPublicKey:
		if !validSHA256(e.KeyFingerprint) || e.OIDCIssuer != "" || e.Subject != "" {
			return ErrInvalid
		}
	case SignerKeyless:
		if _, err := NewKeylessIdentity(e.OIDCIssuer, e.Subject); err != nil || e.KeyFingerprint != "" {
			return ErrInvalid
		}
	case SignerUnknown:
		if e.KeyFingerprint != "" || e.OIDCIssuer != "" || e.Subject != "" || e.State == CryptoValid {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type SignatureEvidence struct {
	CryptographicEvidence
	TrustState PolicyTrustState
	Reason     string
}

func EvaluateTrust(policy TrustPolicy, evidence []CryptographicEvidence) ([]SignatureEvidence, error) {
	if policy.Validate() != nil {
		return nil, ErrInvalid
	}
	result := make([]SignatureEvidence, 0, len(evidence))
	seen := map[string]struct{}{}
	for _, item := range evidence {
		if item.Validate() != nil {
			return nil, ErrInvalid
		}
		key := string(item.Kind) + "\x00" + item.SignatureDigest.String()
		if _, exists := seen[key]; exists {
			return nil, ErrInvalid
		}
		seen[key] = struct{}{}
		value := SignatureEvidence{CryptographicEvidence: item, TrustState: TrustNotEvaluated}
		switch item.State {
		case CryptoValid:
			if policyTrusts(policy, item) {
				value.TrustState = TrustTrusted
				value.Reason = SignatureReasonTrustedID
				if item.SignerType == SignerPublicKey {
					value.Reason = SignatureReasonTrustedKey
				}
			} else {
				value.TrustState = TrustUntrusted
				value.Reason = SignatureReasonUntrusted
			}
		case CryptoInvalid:
			value.Reason = SignatureReasonInvalid
		case CryptoUnverified:
			value.Reason = SignatureReasonUnknown
		case CryptoUnavailable:
			value.Reason = SignatureReasonUnavailable
		}
		result = append(result, value)
	}
	return result, nil
}

type VerificationResult struct {
	Workflow      VerificationWorkflow
	CosignVersion string
	Evidence      []SignatureEvidence
	CompletedAt   time.Time
}

type VerificationDetail struct {
	Workflow      VerificationWorkflow
	Status        ResultStatus
	CosignVersion string
	Evidence      []SignatureEvidence
	CompletedAt   *time.Time
}

func (r VerificationResult) Validate() error {
	if r.Workflow.Validate() != nil || !versionPattern.MatchString(strings.TrimPrefix(r.CosignVersion, "v")) || r.CompletedAt.IsZero() {
		return ErrInvalid
	}
	for _, evidence := range r.Evidence {
		if evidence.Validate() != nil || evidence.Reason == "" {
			return ErrInvalid
		}
		switch evidence.TrustState {
		case TrustTrusted, TrustUntrusted, TrustNotEvaluated:
		default:
			return ErrInvalid
		}
	}
	return nil
}

func (e SignatureEvidence) Validate() error {
	if e.CryptographicEvidence.Validate() != nil || !validTrustText(e.Reason, 128) {
		return ErrInvalid
	}
	if e.State == CryptoValid && e.TrustState != TrustTrusted && e.TrustState != TrustUntrusted {
		return ErrInvalid
	}
	if e.State != CryptoValid && e.TrustState != TrustNotEvaluated {
		return ErrInvalid
	}
	return nil
}

func policyTrusts(policy TrustPolicy, evidence CryptographicEvidence) bool {
	if evidence.SignerType == SignerPublicKey {
		for _, key := range policy.PublicKeys {
			if key.Fingerprint == evidence.KeyFingerprint {
				return true
			}
		}
	}
	if evidence.SignerType == SignerKeyless {
		for _, identity := range policy.KeylessIdentities {
			if identity.Issuer == evidence.OIDCIssuer && identity.Subject == evidence.Subject {
				return true
			}
		}
	}
	return false
}

func validExactIssuer(value string) bool {
	if len(value) < 9 || len(value) > MaxTrustSubjectBytes || strings.TrimSpace(value) != value {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Path == "" && parsed.String() == value
}

func validTrustText(value string, maximum int) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
