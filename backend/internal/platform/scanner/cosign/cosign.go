package cosign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/security"
)

const (
	maxOutputBytes          = 32 * 1024 * 1024
	maxStderrBytes          = 64 * 1024
	maxAttachments          = 512
	cosignSignPredicateType = "https://cosign.sigstore.dev/attestation/v1"
	legacyKeyFingerprintKey = "hubcr.io/key-fingerprint"
)

var errNoAttachment = errors.New("Cosign attachment not found")

type Options struct {
	Binary     string
	ScratchDir string
	Insecure   bool
}

type Runner interface {
	Run(context.Context, string, []string, []string, int) ([]byte, error)
}

type Verifier struct {
	options Options
	runner  Runner
}

func New(options Options) (*Verifier, error) {
	return NewWithRunner(options, commandRunner{})
}

func NewWithRunner(options Options, runner Runner) (*Verifier, error) {
	if runner == nil || options.Binary == "" || strings.TrimSpace(options.Binary) != options.Binary ||
		options.ScratchDir == "" || !filepath.IsAbs(options.ScratchDir) {
		return nil, errors.New("Cosign dependencies and options must be configured")
	}
	return &Verifier{options: options, runner: runner}, nil
}

func (v *Verifier) Verify(
	ctx context.Context,
	reference, token string,
	input security.VerificationInput,
) (string, []security.CryptographicEvidence, error) {
	if !validReference(reference) || token == "" || input.Validate() != nil {
		return "", nil, security.ErrInvalid
	}
	temporary, err := os.MkdirTemp(v.options.ScratchDir, "hubcr-cosign-")
	if err != nil {
		return "", nil, security.ErrToolFailure
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil {
		return "", nil, security.ErrToolFailure
	}
	environment, err := dockerEnvironment(reference, token, temporary)
	if err != nil {
		return "", nil, err
	}
	version, err := v.version(ctx)
	if err != nil {
		return "", nil, err
	}

	attachments := make([]attachment, 0)
	for _, operation := range []struct {
		kind     security.SignatureKind
		download []string
	}{
		{kind: security.SignatureKindSignature, download: []string{"download", "signature"}},
		{kind: security.SignatureKindAttestation, download: []string{"download", "attestation"}},
	} {
		arguments := append([]string(nil), operation.download...)
		arguments = append(arguments, v.registryArguments()...)
		arguments = append(arguments, reference)
		output, runErr := v.runner.Run(ctx, v.options.Binary, arguments, environment, maxOutputBytes)
		if errors.Is(runErr, errNoAttachment) {
			continue
		}
		if runErr != nil {
			return "", nil, fmt.Errorf("%w: discover Cosign material", security.ErrToolFailure)
		}
		parsed, err := parseAttachments(output, operation.kind)
		if err != nil {
			return "", nil, err
		}
		attachments = append(attachments, parsed...)
	}
	attachments = uniqueAttachments(attachments)
	if len(attachments) > maxAttachments {
		return "", nil, security.ErrInvalidOutput
	}

	for _, key := range input.CandidateKeys {
		publicKey, err := parsePublicKey(key.PublicKeyPEM)
		if err != nil {
			return "", nil, err
		}
		v.verifyCandidate(ctx, reference, environment, attachments, candidate{
			typeValue: security.SignerPublicKey, fingerprint: key.Fingerprint, publicKey: publicKey,
		})
	}
	for _, identity := range input.CandidateIdentities {
		v.verifyCandidate(ctx, reference, environment, attachments, candidate{
			typeValue: security.SignerKeyless, issuer: identity.Issuer, subject: identity.Subject,
		})
	}

	evidence := make([]security.CryptographicEvidence, 0, len(attachments))
	for _, item := range attachments {
		value := security.CryptographicEvidence{
			SignatureDigest: item.digest, Kind: item.kind, SignerType: security.SignerUnknown,
			State: security.CryptoUnverified,
		}
		if item.verifiedBy != nil {
			value.SignerType = item.verifiedBy.typeValue
			value.KeyFingerprint = item.verifiedBy.fingerprint
			value.OIDCIssuer = item.verifiedBy.issuer
			value.Subject = item.verifiedBy.subject
			value.State = security.CryptoValid
		} else if item.invalidBy != nil {
			value.SignerType = item.invalidBy.typeValue
			value.KeyFingerprint = item.invalidBy.fingerprint
			value.State = security.CryptoInvalid
		}
		if value.Validate() != nil {
			return "", nil, security.ErrInvalidOutput
		}
		evidence = append(evidence, value)
	}
	return version, evidence, nil
}

type candidate struct {
	typeValue   security.SignerType
	fingerprint string
	publicKey   crypto.PublicKey
	issuer      string
	subject     string
}

func (v *Verifier) verifyCandidate(
	ctx context.Context,
	reference string,
	environment []string,
	attachments []attachment,
	value candidate,
) {
	if value.typeValue == security.SignerPublicKey {
		for index := range attachments {
			item := &attachments[index]
			if item.verifiedBy != nil || item.invalidBy != nil {
				continue
			}
			if !verifyPublicKeySignature(value.publicKey, item.signedPayload, item.signature) {
				if item.keyHint == publicKeyHint(value.fingerprint) {
					copy := value
					item.invalidBy = &copy
				}
				continue
			}
			copy := value
			item.verifiedBy = &copy
		}
		return
	}
	for _, kind := range []security.SignatureKind{
		security.SignatureKindSignature, security.SignatureKindAttestation,
	} {
		command := "verify"
		if kind == security.SignatureKindAttestation {
			command = "verify-attestation"
		}
		for index := range attachments {
			item := &attachments[index]
			if item.kind != kind || item.verifiedBy != nil {
				continue
			}
			arguments := []string{command, "--output", "json"}
			arguments = append(arguments, v.registryArguments()...)
			arguments = append(
				arguments,
				"--certificate-identity", value.subject,
				"--certificate-oidc-issuer", value.issuer,
			)
			arguments = append(arguments, reference)
			output, err := v.runner.Run(ctx, v.options.Binary, arguments, environment, maxOutputBytes)
			if err != nil {
				continue
			}
			payloads, err := parseVerifiedPayloads(output)
			if err != nil || !uniquelyMatchesJSON(attachments, index, payloads) {
				continue
			}
			copy := value
			item.verifiedBy = &copy
		}
	}
}

func (v *Verifier) version(ctx context.Context) (string, error) {
	output, err := v.runner.Run(
		ctx, v.options.Binary, []string{"version", "--json"}, nil, maxStderrBytes,
	)
	if err != nil {
		return "", fmt.Errorf("%w: read Cosign version", security.ErrToolFailure)
	}
	var value struct {
		GitVersion string `json:"gitVersion"`
	}
	if err := decodeOne(output, &value); err != nil || value.GitVersion == "" {
		return "", security.ErrInvalidOutput
	}
	return value.GitVersion, nil
}

func (v *Verifier) registryArguments() []string {
	if v.options.Insecure {
		return []string{"--allow-http-registry"}
	}
	return nil
}

type attachment struct {
	kind          security.SignatureKind
	digest        artifacts.Digest
	payload       []byte
	signedPayload []byte
	signature     []byte
	keyHint       string
	verifiedBy    *candidate
	invalidBy     *candidate
}

func parseAttachments(raw []byte, kind security.SignatureKind) ([]attachment, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	result := make([]attachment, 0)
	for {
		var record struct {
			Base64Signature string `json:"Base64Signature"`
			Payload         string `json:"Payload"`
			PayloadType     string `json:"payloadType"`
			Signatures      []struct {
				Signature string `json:"sig"`
			} `json:"signatures"`
			MediaType            string `json:"mediaType"`
			VerificationMaterial *struct {
				PublicKey *struct {
					Hint string `json:"hint"`
				} `json:"publicKey"`
			} `json:"verificationMaterial"`
			DSSEEnvelope *struct {
				Payload     string `json:"payload"`
				PayloadType string `json:"payloadType"`
				Signatures  []struct {
					Signature string `json:"sig"`
				} `json:"signatures"`
			} `json:"dsseEnvelope"`
		}
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, security.ErrInvalidOutput
		}
		payloadValue := record.Payload
		signatures := []string{record.Base64Signature}
		itemKind := kind
		payloadType := ""
		keyHint := ""
		if len(record.Signatures) > 0 {
			signatures = make([]string, 0, len(record.Signatures))
			for _, value := range record.Signatures {
				signatures = append(signatures, value.Signature)
			}
			payloadType = record.PayloadType
		}
		if record.DSSEEnvelope != nil {
			if record.MediaType != "application/vnd.dev.sigstore.bundle.v0.3+json" {
				return nil, security.ErrInvalidOutput
			}
			payloadValue = record.DSSEEnvelope.Payload
			payloadType = record.DSSEEnvelope.PayloadType
			if record.VerificationMaterial != nil && record.VerificationMaterial.PublicKey != nil {
				keyHint = record.VerificationMaterial.PublicKey.Hint
			}
			signatures = make([]string, 0, len(record.DSSEEnvelope.Signatures))
			for _, value := range record.DSSEEnvelope.Signatures {
				signatures = append(signatures, value.Signature)
			}
		}
		if payloadValue == "" || len(signatures) == 0 ||
			(len(record.Signatures) > 0 && payloadType == "") {
			return nil, security.ErrInvalidOutput
		}
		payload, err := base64.StdEncoding.DecodeString(payloadValue)
		if err != nil {
			return nil, security.ErrInvalidOutput
		}
		if !json.Valid(payload) {
			return nil, security.ErrInvalidOutput
		}
		if record.DSSEEnvelope != nil {
			itemKind, err = bundleKind(payload)
			if err != nil {
				return nil, err
			}
		} else {
			keyHint = legacyPublicKeyHint(payload)
		}
		for _, encodedSignature := range signatures {
			signature, err := base64.StdEncoding.DecodeString(encodedSignature)
			if err != nil || len(signature) == 0 {
				return nil, security.ErrInvalidOutput
			}
			sum := sha256.Sum256(signature)
			digest, err := artifacts.ParseDigest("sha256:" + hex.EncodeToString(sum[:]))
			if err != nil {
				return nil, security.ErrInvalidOutput
			}
			signedPayload := bytes.Clone(payload)
			if record.DSSEEnvelope != nil || len(record.Signatures) > 0 {
				signedPayload = dssePAE(payloadType, payload)
			}
			result = append(result, attachment{
				kind: itemKind, digest: digest, payload: bytes.Clone(payload),
				signedPayload: signedPayload, signature: bytes.Clone(signature), keyHint: keyHint,
			})
			if len(result) > maxAttachments {
				return nil, security.ErrInvalidOutput
			}
		}
	}
	return result, nil
}

func legacyPublicKeyHint(payload []byte) string {
	var value struct {
		Optional map[string]string `json:"optional"`
	}
	if json.Unmarshal(payload, &value) != nil {
		return ""
	}
	fingerprint := value.Optional[legacyKeyFingerprintKey]
	if _, err := artifacts.ParseDigest(fingerprint); err != nil {
		return ""
	}
	return publicKeyHint(fingerprint)
}

func uniquelyMatchesJSON(attachments []attachment, index int, values [][]byte) bool {
	if index < 0 || index >= len(attachments) || !containsJSON(values, attachments[index].payload) {
		return false
	}
	matches := 0
	for candidateIndex := range attachments {
		if attachments[candidateIndex].verifiedBy == nil &&
			attachments[candidateIndex].kind == attachments[index].kind &&
			containsJSON([][]byte{attachments[candidateIndex].payload}, attachments[index].payload) {
			matches++
		}
	}
	return matches == 1
}

func containsJSON(values [][]byte, target []byte) bool {
	var targetValue any
	if json.Unmarshal(target, &targetValue) != nil {
		return false
	}
	for _, value := range values {
		var candidateValue any
		if json.Unmarshal(value, &candidateValue) == nil && reflect.DeepEqual(candidateValue, targetValue) {
			return true
		}
	}
	return false
}

func parsePublicKey(value string) (crypto.PublicKey, error) {
	block, trailing := pem.Decode([]byte(value))
	if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(trailing)) != 0 {
		return nil, security.ErrInvalid
	}
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, security.ErrInvalid
	}
	return publicKey, nil
}

func verifyPublicKeySignature(publicKey crypto.PublicKey, payload, signature []byte) bool {
	if len(payload) == 0 || len(signature) == 0 {
		return false
	}
	sum := sha256.Sum256(payload)
	switch key := publicKey.(type) {
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(key, sum[:], signature)
	case ed25519.PublicKey:
		return ed25519.Verify(key, payload, signature)
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature) == nil ||
			rsa.VerifyPSS(key, crypto.SHA256, sum[:], signature, nil) == nil
	default:
		return false
	}
}

func publicKeyHint(fingerprint string) string {
	raw, err := hex.DecodeString(strings.TrimPrefix(fingerprint, "sha256:"))
	if err != nil || len(raw) != sha256.Size {
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func dssePAE(payloadType string, payload []byte) []byte {
	return []byte(fmt.Sprintf("DSSEv1 %d %s %d %s", len(payloadType), payloadType, len(payload), payload))
}

func bundleKind(payload []byte) (security.SignatureKind, error) {
	var value struct {
		StatementType string          `json:"_type"`
		PredicateType string          `json:"predicateType"`
		Critical      json.RawMessage `json:"critical"`
	}
	if json.Unmarshal(payload, &value) != nil {
		return "", security.ErrInvalidOutput
	}
	if value.StatementType != "" && value.PredicateType != "" {
		if value.PredicateType == cosignSignPredicateType {
			return security.SignatureKindSignature, nil
		}
		return security.SignatureKindAttestation, nil
	}
	if len(value.Critical) > 0 && string(value.Critical) != "null" {
		return security.SignatureKindSignature, nil
	}
	return "", security.ErrInvalidOutput
}

func uniqueAttachments(values []attachment) []attachment {
	result := make([]attachment, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := string(value.kind) + "\x00" + value.digest.String()
		if _, found := seen[key]; found {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseVerifiedPayloads(raw []byte) ([][]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	result := make([][]byte, 0)
	for {
		var value json.RawMessage
		if err := decoder.Decode(&value); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, security.ErrInvalidOutput
		}
		var values []json.RawMessage
		if len(value) > 0 && value[0] == '[' {
			if err := json.Unmarshal(value, &values); err != nil {
				return nil, security.ErrInvalidOutput
			}
		} else {
			values = []json.RawMessage{value}
		}
		for _, item := range values {
			compact, err := compactJSON(item)
			if err != nil {
				return nil, security.ErrInvalidOutput
			}
			result = append(result, compact)
		}
	}
	return result, nil
}

func compactJSON(raw []byte) ([]byte, error) {
	var compact bytes.Buffer
	if !json.Valid(raw) || json.Compact(&compact, raw) != nil {
		return nil, security.ErrInvalidOutput
	}
	return bytes.Clone(compact.Bytes()), nil
}

func dockerEnvironment(reference, token, directory string) ([]string, error) {
	host, _, found := strings.Cut(reference, "/")
	if !found || host == "" || strings.ContainsAny(host, " \t\r\n") {
		return nil, security.ErrInvalid
	}
	configDirectory := filepath.Join(directory, "docker")
	if err := os.Mkdir(configDirectory, 0o700); err != nil {
		return nil, security.ErrToolFailure
	}
	config, err := json.Marshal(map[string]any{
		"auths": map[string]any{host: map[string]string{"registrytoken": token}},
	})
	if err != nil || os.WriteFile(filepath.Join(configDirectory, "config.json"), config, 0o600) != nil {
		return nil, security.ErrToolFailure
	}
	environment := os.Environ()
	filtered := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		if !strings.HasPrefix(value, "DOCKER_CONFIG=") {
			filtered = append(filtered, value)
		}
	}
	return append(filtered, "DOCKER_CONFIG="+configDirectory), nil
}

func decodeOne(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func validReference(value string) bool {
	return value != "" && len(value) <= 512 && !strings.ContainsAny(value, " \t\r\n") &&
		strings.Contains(value, "@sha256:")
}

type commandRunner struct{}

func (commandRunner) Run(
	ctx context.Context,
	binary string,
	arguments []string,
	environment []string,
	maximum int,
) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, arguments...)
	if environment != nil {
		command.Env = environment
	}
	stdout := &limitedBuffer{maximum: maximum}
	stderr := &limitedBuffer{maximum: maxStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		message := strings.ToLower(string(stderr.Bytes()))
		if noAttachmentMessage(message) {
			return nil, errNoAttachment
		}
		return nil, errors.New("Cosign command failed")
	}
	return bytes.Clone(stdout.Bytes()), nil
}

func noAttachmentMessage(message string) bool {
	for _, marker := range []string{
		"no signatures found",
		"no signatures associated",
		"no attestations found",
		"no attestations associated",
		"found no attestations",
		"no matching attestations",
		"no valid bundles exist",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

type limitedBuffer struct {
	buffer  bytes.Buffer
	maximum int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	remaining := b.maximum - b.buffer.Len()
	if remaining <= 0 {
		return 0, errors.New("Cosign output limit exceeded")
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		return remaining, errors.New("Cosign output limit exceeded")
	}
	return b.buffer.Write(value)
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }
