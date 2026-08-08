package cosign

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/security"
)

func TestVerifierUsesPrivateDockerConfigAndSeparatesVerifiedFromUnknown(t *testing.T) {
	payloadTrusted := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`)
	payloadUnknown := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`)
	input, privateKey := trustInputWithPrivateKey(t)
	digest := sha256.Sum256(payloadTrusted)
	trustedSignature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatalf("ecdsa.SignASN1() error = %v", err)
	}
	download := attachmentJSON(trustedSignature, payloadTrusted) + "\n" +
		attachmentJSON([]byte("unknown-signature"), payloadUnknown) + "\n"
	runner := &cosignRunner{
		responses: [][]byte{
			[]byte(`{"gitVersion":"v3.0.6"}`),
			[]byte(download), nil,
		},
		errors: []error{nil, nil, errNoAttachment},
	}
	verifier, err := NewWithRunner(Options{
		Binary: "/usr/local/bin/cosign", ScratchDir: t.TempDir(), Insecure: true,
	}, runner)
	if err != nil {
		t.Fatalf("NewWithRunner() error = %v", err)
	}
	version, evidence, err := verifier.Verify(
		context.Background(),
		"registry:5000/team/app@sha256:"+strings.Repeat("a", 64),
		"private-registry-token",
		input,
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if version != "v3.0.6" || len(evidence) != 2 ||
		evidence[0].State != security.CryptoValid ||
		evidence[0].KeyFingerprint != input.CandidateKeys[0].Fingerprint ||
		evidence[1].State != security.CryptoUnverified ||
		evidence[1].SignerType != security.SignerUnknown {
		t.Fatalf("Verify() = %q, %#v", version, evidence)
	}
	for _, call := range runner.calls {
		for _, argument := range call.arguments {
			if strings.Contains(argument, "private-registry-token") {
				t.Fatalf("token leaked into argument %q", argument)
			}
		}
	}
	if !runner.sawProtectedToken || !slices.Contains(runner.calls[1].arguments, "--allow-http-registry") {
		t.Fatalf("runner did not observe protected token/config: %#v", runner.calls)
	}
}

func TestVerifierRejectsMalformedDownloadedMaterial(t *testing.T) {
	runner := &cosignRunner{
		responses: [][]byte{[]byte(`{"gitVersion":"v3.0.6"}`), []byte(`{"Payload":"not-base64"}`)},
		errors:    []error{nil, nil},
	}
	verifier, _ := NewWithRunner(Options{
		Binary: "cosign", ScratchDir: t.TempDir(),
	}, runner)
	_, _, err := verifier.Verify(
		context.Background(), "registry/team/app@sha256:"+strings.Repeat("a", 64),
		"token", trustInput(t),
	)
	if !errors.Is(err, security.ErrInvalidOutput) {
		t.Fatalf("Verify() error = %v, want ErrInvalidOutput", err)
	}
}

func TestParseAttachmentsAcceptsCosignV3BundlesAndDeduplicatesDownloads(t *testing.T) {
	signaturePayload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`)
	attestationPayload := []byte(`{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1","subject":[]}`)
	raw := v3BundleJSON("signature", signaturePayload) + "\n" +
		v3BundleJSON("attestation", attestationPayload) + "\n"

	first, err := parseAttachments([]byte(raw), security.SignatureKindSignature)
	if err != nil {
		t.Fatalf("parseAttachments() error = %v", err)
	}
	second, err := parseAttachments([]byte(raw), security.SignatureKindAttestation)
	if err != nil {
		t.Fatalf("parseAttachments() second error = %v", err)
	}
	attachments := uniqueAttachments(append(first, second...))
	if len(attachments) != 2 ||
		attachments[0].kind != security.SignatureKindSignature ||
		attachments[1].kind != security.SignatureKindAttestation ||
		string(attachments[0].payload) != string(signaturePayload) ||
		string(attachments[1].payload) != string(attestationPayload) {
		t.Fatalf("parseAttachments() = %#v", attachments)
	}
}

func TestParseAttachmentsPreservesSignedPayloadBytes(t *testing.T) {
	payload := []byte("{\n  \"critical\": {\"image\": {\"docker-manifest-digest\": \"sha256:" + strings.Repeat("a", 64) + "\"}}\n}")
	attachments, err := parseAttachments(
		[]byte(attachmentJSON([]byte("signature"), payload)), security.SignatureKindSignature,
	)
	if err != nil {
		t.Fatalf("parseAttachments() error = %v", err)
	}
	if len(attachments) != 1 || string(attachments[0].payload) != string(payload) {
		t.Fatalf("signed payload bytes changed: %q", attachments[0].payload)
	}
}

func TestVerifierClassifiesV3BundleValidityFromMatchingPublicKeyHint(t *testing.T) {
	payload := []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[],"predicateType":"https://cosign.sigstore.dev/attestation/v1","predicate":{}}`)
	input, privateKey := trustInputWithPrivateKey(t)
	sum := sha256.Sum256(dssePAE("application/vnd.in-toto+json", payload))
	validSignature, err := ecdsa.SignASN1(rand.Reader, privateKey, sum[:])
	if err != nil {
		t.Fatalf("ecdsa.SignASN1() error = %v", err)
	}
	invalidSignature := append([]byte(nil), validSignature...)
	invalidSignature[len(invalidSignature)-1] ^= 1

	for _, test := range []struct {
		name      string
		signature []byte
		state     security.CryptographicState
	}{
		{name: "valid", signature: validSignature, state: security.CryptoValid},
		{name: "invalid", signature: invalidSignature, state: security.CryptoInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &cosignRunner{
				responses: [][]byte{
					[]byte(`{"gitVersion":"v3.0.6"}`),
					[]byte(v3KeyBundleJSON(test.signature, payload, publicKeyHint(input.CandidateKeys[0].Fingerprint))),
					nil,
				},
				errors: []error{nil, nil, errNoAttachment},
			}
			verifier, _ := NewWithRunner(Options{
				Binary: "cosign", ScratchDir: t.TempDir(),
			}, runner)
			_, evidence, err := verifier.Verify(
				context.Background(), "registry/team/app@sha256:"+strings.Repeat("a", 64),
				"token", input,
			)
			if err != nil || len(evidence) != 1 || evidence[0].Kind != security.SignatureKindSignature ||
				evidence[0].State != test.state || evidence[0].KeyFingerprint != input.CandidateKeys[0].Fingerprint {
				t.Fatalf("Verify() = %#v, %v", evidence, err)
			}
		})
	}
}

func TestVerifierClassifiesAnnotatedLegacyValidityFromMatchingPublicKeyHint(t *testing.T) {
	input, privateKey := trustInputWithPrivateKey(t)
	payload, err := json.Marshal(map[string]any{
		"critical": map[string]any{"image": map[string]string{
			"docker-manifest-digest": "sha256:" + strings.Repeat("a", 64),
		}},
		"optional": map[string]string{
			legacyKeyFingerprintKey: input.CandidateKeys[0].Fingerprint,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	sum := sha256.Sum256(payload)
	validSignature, err := ecdsa.SignASN1(rand.Reader, privateKey, sum[:])
	if err != nil {
		t.Fatalf("ecdsa.SignASN1() error = %v", err)
	}
	invalidSignature := append([]byte(nil), validSignature...)
	invalidSignature[len(invalidSignature)-1] ^= 1

	for _, test := range []struct {
		name      string
		signature []byte
		state     security.CryptographicState
	}{
		{name: "valid", signature: validSignature, state: security.CryptoValid},
		{name: "invalid", signature: invalidSignature, state: security.CryptoInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &cosignRunner{
				responses: [][]byte{
					[]byte(`{"gitVersion":"v3.0.6"}`),
					[]byte(attachmentJSON(test.signature, payload)), nil,
				},
				errors: []error{nil, nil, errNoAttachment},
			}
			verifier, _ := NewWithRunner(Options{Binary: "cosign", ScratchDir: t.TempDir()}, runner)
			_, evidence, err := verifier.Verify(
				context.Background(), "registry/team/app@sha256:"+strings.Repeat("a", 64),
				"token", input,
			)
			if err != nil || len(evidence) != 1 || evidence[0].State != test.state ||
				evidence[0].KeyFingerprint != input.CandidateKeys[0].Fingerprint {
				t.Fatalf("Verify() = %#v, %v", evidence, err)
			}
		})
	}
}

func TestVerifierValidatesLegacyDSSEAttestation(t *testing.T) {
	payload := []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[],"predicateType":"https://example.com/provenance/v1","predicate":{}}`)
	input, privateKey := trustInputWithPrivateKey(t)
	sum := sha256.Sum256(dssePAE("application/vnd.in-toto+json", payload))
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, sum[:])
	if err != nil {
		t.Fatalf("ecdsa.SignASN1() error = %v", err)
	}
	runner := &cosignRunner{
		responses: [][]byte{
			[]byte(`{"gitVersion":"v3.0.6"}`), nil,
			[]byte(legacyAttestationJSON(signature, payload)),
		},
		errors: []error{nil, errNoAttachment, nil},
	}
	verifier, _ := NewWithRunner(Options{Binary: "cosign", ScratchDir: t.TempDir()}, runner)
	_, evidence, err := verifier.Verify(
		context.Background(), "registry/team/app@sha256:"+strings.Repeat("a", 64), "token", input,
	)
	if err != nil || len(evidence) != 1 || evidence[0].Kind != security.SignatureKindAttestation ||
		evidence[0].State != security.CryptoValid {
		t.Fatalf("Verify() = %#v, %v", evidence, err)
	}
}

func TestVerifierUsesExactKeylessIdentityFlags(t *testing.T) {
	payload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`)
	input := trustInput(t)
	identity, _ := security.NewKeylessIdentity("https://issuer.example", "release@example.com")
	input.Policy.PublicKeys = nil
	input.Policy.KeylessIdentities = []security.KeylessIdentity{identity}
	input.CandidateKeys = nil
	input.CandidateIdentities = []security.KeylessIdentity{identity}
	runner := &cosignRunner{
		responses: [][]byte{
			[]byte(`{"gitVersion":"v3.0.6"}`),
			[]byte(attachmentJSON([]byte("keyless-signature"), payload)), nil,
			[]byte("[" + string(payload) + "]"),
		},
		errors: []error{nil, nil, errNoAttachment, nil},
	}
	verifier, _ := NewWithRunner(Options{Binary: "cosign", ScratchDir: t.TempDir()}, runner)
	_, evidence, err := verifier.Verify(
		context.Background(), "registry/team/app@sha256:"+strings.Repeat("a", 64), "token", input,
	)
	if err != nil || len(evidence) != 1 || evidence[0].SignerType != security.SignerKeyless ||
		evidence[0].OIDCIssuer != identity.Issuer || evidence[0].Subject != identity.Subject ||
		evidence[0].State != security.CryptoValid {
		t.Fatalf("Verify() = %#v, %v", evidence, err)
	}
	arguments := runner.calls[len(runner.calls)-1].arguments
	issuerIndex := slices.Index(arguments, "--certificate-oidc-issuer")
	subjectIndex := slices.Index(arguments, "--certificate-identity")
	if issuerIndex < 0 || issuerIndex+1 >= len(arguments) || arguments[issuerIndex+1] != identity.Issuer ||
		subjectIndex < 0 || subjectIndex+1 >= len(arguments) || arguments[subjectIndex+1] != identity.Subject {
		t.Fatalf("keyless arguments = %#v", arguments)
	}
}

func TestNoAttachmentMessageAcceptsCosignLegacyAndBundleErrors(t *testing.T) {
	for _, message := range []string{
		"error: no signatures associated with this image",
		"error: no attestations associated with this image",
		"error: found no attestations",
		"no matching attestations",
		"no valid bundles exist in registry",
	} {
		if !noAttachmentMessage(message) {
			t.Fatalf("noAttachmentMessage(%q) = false", message)
		}
	}
	if noAttachmentMessage("dial tcp: connection refused") {
		t.Fatal("dependency failure was classified as an absent attachment")
	}
}

type cosignCall struct {
	arguments   []string
	environment []string
}

type cosignRunner struct {
	responses         [][]byte
	errors            []error
	calls             []cosignCall
	sawProtectedToken bool
}

func (r *cosignRunner) Run(
	_ context.Context,
	_ string,
	arguments []string,
	environment []string,
	_ int,
) ([]byte, error) {
	r.calls = append(r.calls, cosignCall{
		arguments:   append([]string(nil), arguments...),
		environment: append([]string(nil), environment...),
	})
	for _, value := range environment {
		if strings.HasPrefix(value, "DOCKER_CONFIG=") {
			content, err := os.ReadFile(filepath.Join(strings.TrimPrefix(value, "DOCKER_CONFIG="), "config.json"))
			var config struct {
				Auths map[string]struct {
					RegistryToken string `json:"registrytoken"`
					IdentityToken string `json:"identitytoken"`
				} `json:"auths"`
			}
			decodeErr := json.Unmarshal(content, &config)
			auth := config.Auths["registry:5000"]
			if err == nil && decodeErr == nil && auth.RegistryToken == "private-registry-token" && auth.IdentityToken == "" {
				info, statErr := os.Stat(filepath.Join(strings.TrimPrefix(value, "DOCKER_CONFIG="), "config.json"))
				r.sawProtectedToken = statErr == nil && info.Mode().Perm() == 0o600
			}
		}
	}
	if len(r.responses) == 0 {
		return nil, errors.New("missing response")
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	var err error
	if len(r.errors) > 0 {
		err = r.errors[0]
		r.errors = r.errors[1:]
	}
	return response, err
}

func attachmentJSON(signature, payload []byte) string {
	encoded, _ := json.Marshal(map[string]string{
		"Base64Signature": base64.StdEncoding.EncodeToString(signature),
		"Payload":         base64.StdEncoding.EncodeToString(payload),
	})
	return string(encoded)
}

func v3BundleJSON(signature string, payload []byte) string {
	encoded, _ := json.Marshal(map[string]any{
		"mediaType": "application/vnd.dev.sigstore.bundle.v0.3+json",
		"verificationMaterial": map[string]any{
			"publicKey": map[string]string{"hint": "test"},
		},
		"dsseEnvelope": map[string]any{
			"payload":     base64.StdEncoding.EncodeToString(payload),
			"payloadType": "application/vnd.in-toto+json",
			"signatures": []map[string]string{{
				"sig": base64.StdEncoding.EncodeToString([]byte(signature)),
			}},
		},
	})
	return string(encoded)
}

func v3KeyBundleJSON(signature, payload []byte, hint string) string {
	encoded, _ := json.Marshal(map[string]any{
		"mediaType": "application/vnd.dev.sigstore.bundle.v0.3+json",
		"verificationMaterial": map[string]any{
			"publicKey": map[string]string{"hint": hint},
		},
		"dsseEnvelope": map[string]any{
			"payload":     base64.StdEncoding.EncodeToString(payload),
			"payloadType": "application/vnd.in-toto+json",
			"signatures": []map[string]string{{
				"sig": base64.StdEncoding.EncodeToString(signature),
			}},
		},
	})
	return string(encoded)
}

func legacyAttestationJSON(signature, payload []byte) string {
	encoded, _ := json.Marshal(map[string]any{
		"payloadType": "application/vnd.in-toto+json",
		"payload":     base64.StdEncoding.EncodeToString(payload),
		"signatures": []map[string]string{{
			"keyid": "", "sig": base64.StdEncoding.EncodeToString(signature),
		}},
	})
	return string(encoded)
}

func trustInput(t *testing.T) security.VerificationInput {
	input, _ := trustInputWithPrivateKey(t)
	return input
}

func trustInputWithPrivateKey(t *testing.T) (security.VerificationInput, *ecdsa.PrivateKey) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	key, err := security.NewPublicKeyTrust(
		"release", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
	)
	if err != nil {
		t.Fatalf("NewPublicKeyTrust() error = %v", err)
	}
	now := time.Now().UTC()
	target, _ := security.NewTarget(
		"repository", "team", "app", "sha256:"+strings.Repeat("a", 64),
	)
	policy := security.TrustPolicy{
		ID: "policy", NamespaceID: "namespace", Version: 1, CreatedByUserID: "user",
		PublicKeys: []security.PublicKeyTrust{key}, CreatedAt: now,
	}
	return security.VerificationInput{
		Workflow: security.VerificationWorkflow{
			ID: "workflow", Target: target, PolicyID: policy.ID,
			PolicyVersion: policy.Version, JobID: "job", CreatedAt: now,
		},
		Policy: policy, CandidateKeys: []security.PublicKeyTrust{key},
	}, privateKey
}
