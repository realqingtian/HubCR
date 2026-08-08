package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
)

func TestTrustPolicyRequiresCanonicalKeysAndExactIdentities(t *testing.T) {
	key := testPublicKey(t, "release")
	identity, err := NewKeylessIdentity(
		"https://token.actions.githubusercontent.com",
		"https://github.com/acme/app/.github/workflows/release.yml@refs/heads/main",
	)
	if err != nil {
		t.Fatalf("NewKeylessIdentity() error = %v", err)
	}
	policy := TrustPolicy{
		ID: "policy-id", NamespaceID: "namespace-id", Version: 1,
		CreatedByUserID: "user-id", PublicKeys: []PublicKeyTrust{key},
		KeylessIdentities: []KeylessIdentity{identity}, CreatedAt: time.Now().UTC(),
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !strings.HasPrefix(key.Fingerprint, "sha256:") || len(key.Fingerprint) != 71 {
		t.Fatalf("fingerprint = %q", key.Fingerprint)
	}
	for _, candidate := range []KeylessIdentity{
		{Issuer: identity.Issuer, Subject: "*"},
		{Issuer: "http://issuer.example", Subject: identity.Subject},
		{Issuer: identity.Issuer + "/", Subject: identity.Subject},
	} {
		if candidate.Validate() == nil {
			t.Fatalf("identity %#v unexpectedly valid", candidate)
		}
	}
}

func TestEvaluateTrustSeparatesValidityFromCurrentPolicyTrust(t *testing.T) {
	now := time.Now().UTC()
	currentKey := testPublicKey(t, "current")
	oldKey := testPublicKey(t, "old")
	identity, _ := NewKeylessIdentity("https://issuer.example", "subject@example.com")
	policy := TrustPolicy{
		ID: "policy", NamespaceID: "namespace", Version: 2, CreatedByUserID: "user",
		PublicKeys: []PublicKeyTrust{currentKey}, KeylessIdentities: []KeylessIdentity{identity},
		CreatedAt: now,
	}
	evidence := []CryptographicEvidence{
		testCryptoEvidence(t, "a", SignerPublicKey, currentKey.Fingerprint, "", "", CryptoValid),
		testCryptoEvidence(t, "b", SignerPublicKey, oldKey.Fingerprint, "", "", CryptoValid),
		testCryptoEvidence(t, "c", SignerKeyless, "", identity.Issuer, identity.Subject, CryptoValid),
		testCryptoEvidence(t, "d", SignerUnknown, "", "", "", CryptoUnverified),
		testCryptoEvidence(t, "e", SignerPublicKey, currentKey.Fingerprint, "", "", CryptoInvalid),
		testCryptoEvidence(t, "f", SignerUnknown, "", "", "", CryptoUnavailable),
	}
	result, err := EvaluateTrust(policy, evidence)
	if err != nil {
		t.Fatalf("EvaluateTrust() error = %v", err)
	}
	if result[0].TrustState != TrustTrusted || result[1].TrustState != TrustUntrusted ||
		result[2].TrustState != TrustTrusted || result[3].TrustState != TrustNotEvaluated ||
		result[4].TrustState != TrustNotEvaluated || result[4].Reason != SignatureReasonInvalid ||
		result[5].TrustState != TrustNotEvaluated || result[5].Reason != SignatureReasonUnavailable {
		t.Fatalf("EvaluateTrust() = %#v", result)
	}
}

func TestVerificationPayloadBindsPolicyVersionAndDigest(t *testing.T) {
	now := time.Now().UTC()
	target, _ := NewTarget("repository", "team", "app", "sha256:"+strings.Repeat("a", 64))
	policy := TrustPolicy{
		ID: "policy", NamespaceID: "namespace", Version: 7, CreatedByUserID: "user",
		PublicKeys: []PublicKeyTrust{testPublicKey(t, "release")}, CreatedAt: now,
	}
	payload, err := MarshalVerificationPayload(target, policy)
	if err != nil {
		t.Fatalf("MarshalVerificationPayload() error = %v", err)
	}
	decoded, policyID, version, err := ParseVerificationPayload(payload)
	if err != nil || decoded != target || policyID != policy.ID || version != policy.Version {
		t.Fatalf("ParseVerificationPayload() = %#v, %q, %d, %v", decoded, policyID, version, err)
	}
	intent, err := VerificationIntentKey(target, policy)
	if err != nil || !strings.Contains(intent, target.Digest.String()) || !strings.Contains(intent, policy.ID) {
		t.Fatalf("VerificationIntentKey() = %q, %v", intent, err)
	}
}

func testPublicKey(t *testing.T, name string) PublicKeyTrust {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	key, err := NewPublicKeyTrust(name, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	if err != nil {
		t.Fatalf("NewPublicKeyTrust() error = %v", err)
	}
	return key
}

func testCryptoEvidence(
	t *testing.T,
	digit string,
	signer SignerType,
	fingerprint, issuer, subject string,
	state CryptographicState,
) CryptographicEvidence {
	t.Helper()
	digest, err := artifacts.ParseDigest("sha256:" + strings.Repeat(digit, 64))
	if err != nil {
		t.Fatalf("ParseDigest() error = %v", err)
	}
	return CryptographicEvidence{
		SignatureDigest: digest, Kind: SignatureKindSignature, SignerType: signer,
		KeyFingerprint: fingerprint, OIDCIssuer: issuer, Subject: subject, State: state,
	}
}
