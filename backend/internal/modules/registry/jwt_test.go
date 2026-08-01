package registry

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRS256SignerAndVerifierContract(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	key := testRSAKey(t)
	signer := testSigner(t, key)
	claims := validTestClaims(now)

	token, err := signer.Sign(context.Background(), claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header JWTHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("decode header JSON: %v", err)
	}
	if header != (JWTHeader{Type: "JWT", Algorithm: "RS256", KeyID: signer.KeyID()}) {
		t.Fatalf("header = %#v", header)
	}
	if signer.KeyID() == "" || signer.PublicJWK().KeyID != signer.KeyID() {
		t.Fatalf("signer key ID/JWK = %q %#v", signer.KeyID(), signer.PublicJWK())
	}

	verifier := testVerifier(t, map[string]*rsa.PublicKey{
		signer.KeyID(): &key.PublicKey,
	}, now)
	verified, err := verifier.Verify(token)
	if err != nil || !claimsEqual(verified, claims) {
		t.Fatalf("Verify() = %#v, %v; want %#v", verified, err, claims)
	}
}

func TestRS256VerifierRejectsTamperingAndInvalidClaims(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	key := testRSAKey(t)
	signer := testSigner(t, key)
	verifier := testVerifier(t, map[string]*rsa.PublicKey{
		signer.KeyID(): &key.PublicKey,
	}, now)

	tests := []struct {
		name   string
		claims Claims
		mutate func(string) string
	}{
		{name: "expired", claims: func() Claims {
			value := validTestClaims(now)
			value.IssuedAt = now.Add(-5 * time.Minute).Unix()
			value.NotBefore = now.Add(-5*time.Minute - 30*time.Second).Unix()
			value.ExpiresAt = now.Add(-time.Minute).Unix()
			return value
		}()},
		{name: "wrong audience", claims: func() Claims {
			value := validTestClaims(now)
			value.Audience = "other-registry"
			return value
		}()},
		{name: "future not before", claims: func() Claims {
			value := validTestClaims(now)
			value.NotBefore = now.Add(31 * time.Second).Unix()
			value.IssuedAt = value.NotBefore
			value.ExpiresAt = now.Add(time.Minute).Unix()
			return value
		}()},
		{name: "tampered signature", claims: validTestClaims(now), mutate: func(token string) string {
			parts := strings.Split(token, ".")
			if strings.HasPrefix(parts[2], "A") {
				parts[2] = "B" + parts[2][1:]
			} else {
				parts[2] = "A" + parts[2][1:]
			}
			return strings.Join(parts, ".")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := signer.Sign(context.Background(), test.claims)
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}
			if test.mutate != nil {
				token = test.mutate(token)
			}
			if _, err := verifier.Verify(token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestRS256VerifierSupportsKeyRotationOverlap(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	oldKey := testRSAKey(t)
	newKey := testRSAKey(t)
	oldSigner := testSigner(t, oldKey)
	newSigner := testSigner(t, newKey)
	oldToken, err := oldSigner.Sign(context.Background(), validTestClaims(now))
	if err != nil {
		t.Fatalf("old Sign() error = %v", err)
	}
	newToken, err := newSigner.Sign(context.Background(), validTestClaims(now))
	if err != nil {
		t.Fatalf("new Sign() error = %v", err)
	}

	overlap := testVerifier(t, map[string]*rsa.PublicKey{
		oldSigner.KeyID(): &oldKey.PublicKey,
		newSigner.KeyID(): &newKey.PublicKey,
	}, now)
	for label, token := range map[string]string{"old": oldToken, "new": newToken} {
		if _, err := overlap.Verify(token); err != nil {
			t.Fatalf("overlap Verify(%s) error = %v", label, err)
		}
	}
	retired := testVerifier(t, map[string]*rsa.PublicKey{
		newSigner.KeyID(): &newKey.PublicKey,
	}, now)
	if _, err := retired.Verify(oldToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("retired old token error = %v, want ErrInvalidToken", err)
	}
	if _, err := retired.Verify(newToken); err != nil {
		t.Fatalf("retired new token error = %v", err)
	}
}

func TestParseRSAPrivateKeyPEMAndMinimumSize(t *testing.T) {
	key := testRSAKey(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	parsed, err := ParseRSAPrivateKeyPEM(encoded)
	if err != nil || parsed.N.Cmp(key.N) != 0 {
		t.Fatalf("ParseRSAPrivateKeyPEM() = %#v, %v", parsed, err)
	}

	smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey(1024) error = %v", err)
	}
	smallPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(smallKey),
	})
	if _, err := ParseRSAPrivateKeyPEM(smallPEM); err == nil {
		t.Fatal("ParseRSAPrivateKeyPEM(1024-bit) error = nil")
	}
}

func TestParseRS256JWKSSupportsRotationAndRejectsMismatchedKeyID(t *testing.T) {
	first := testSigner(t, testRSAKey(t))
	second := testSigner(t, testRSAKey(t))
	encoded, err := json.Marshal(JWKSet{Keys: []JWK{
		first.PublicJWK(), second.PublicJWK(),
	}})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	keys, err := ParseRS256JWKS(encoded)
	if err != nil {
		t.Fatalf("ParseRS256JWKS() error = %v", err)
	}
	if len(keys) != 2 || keys[first.KeyID()] == nil || keys[second.KeyID()] == nil {
		t.Fatalf("ParseRS256JWKS() keys = %#v", keys)
	}

	mismatched := first.PublicJWK()
	mismatched.KeyID = second.KeyID()
	encoded, err = json.Marshal(JWKSet{Keys: []JWK{mismatched}})
	if err != nil {
		t.Fatalf("json.Marshal(mismatched) error = %v", err)
	}
	if _, err := ParseRS256JWKS(encoded); err == nil {
		t.Fatal("ParseRS256JWKS(mismatched kid) error = nil")
	}
}

func validTestClaims(now time.Time) Claims {
	return Claims{
		Issuer: "hubcr-token-service", Subject: "", Audience: "hubcr-registry",
		ExpiresAt: now.Add(5 * time.Minute).Unix(),
		NotBefore: now.Add(-30 * time.Second).Unix(),
		IssuedAt:  now.Unix(), ID: "test-token-id",
		Access: []Access{
			{Type: ResourceRepository, Name: "team/image", Actions: []Action{ActionPull}},
			{Type: ResourceRepository, Name: "team/private", Actions: []Action{}},
		},
	}
}

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, minimumRSAKeyBits)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	return key
}

func testSigner(t *testing.T, key *rsa.PrivateKey) *RS256Signer {
	t.Helper()
	signer, err := NewRS256Signer(key, rand.Reader)
	if err != nil {
		t.Fatalf("NewRS256Signer() error = %v", err)
	}
	return signer
}

func testVerifier(
	t *testing.T,
	keys map[string]*rsa.PublicKey,
	now time.Time,
) *RS256Verifier {
	t.Helper()
	verifier, err := NewRS256Verifier(
		keys, "hubcr-token-service", "hubcr-registry",
		func() time.Time { return now }, 30*time.Second,
	)
	if err != nil {
		t.Fatalf("NewRS256Verifier() error = %v", err)
	}
	return verifier
}

func claimsEqual(left, right Claims) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
