package auth

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestIDRoundTrip(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	parsed, err := ParseID(string(id))
	if err != nil {
		t.Fatalf("ParseID() error = %v", err)
	}
	if parsed != id {
		t.Fatalf("ParseID() = %q, want %q", parsed, id)
	}
}

func TestParseIDRejectsMalformedOrNonV4Values(t *testing.T) {
	for _, value := range []string{
		"",
		"not-a-uuid",
		"00000000-0000-0000-0000-000000000000",
		"00000000-0000-1000-8000-000000000000",
		"00000000-0000-4000-0000-000000000000",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseID(value); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("ParseID(%q) error = %v, want ErrInvalidID", value, err)
			}
		})
	}
}

func TestPasswordHasherUsesUniqueSaltAndVerifies(t *testing.T) {
	hasher := &PasswordHasher{
		random: bytes.NewReader(append(
			bytes.Repeat([]byte{1}, passwordSaltLength),
			bytes.Repeat([]byte{2}, passwordSaltLength)...,
		)),
		params: defaultPasswordParameters,
	}
	password := []byte("correct horse battery staple")

	first, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("first Hash() error = %v", err)
	}
	second, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("second Hash() error = %v", err)
	}
	if first == second {
		t.Fatal("Hash() reused a salt")
	}
	if strings.Contains(first, string(password)) {
		t.Fatal("Hash() contains the plaintext password")
	}

	verified, err := hasher.Verify(password, first)
	if err != nil || !verified {
		t.Fatalf("Verify(correct) = %v, %v; want true, nil", verified, err)
	}
	verified, err = hasher.Verify([]byte("wrong password"), first)
	if err != nil || verified {
		t.Fatalf("Verify(wrong) = %v, %v; want false, nil", verified, err)
	}
	if hasher.NeedsRehash(first) {
		t.Fatal("NeedsRehash(current hash) = true")
	}
}

func TestPasswordHasherRejectsUntrustedParametersBeforeAllocation(t *testing.T) {
	hasher := NewPasswordHasher()
	malformed := "$argon2id$v=19$m=4294967295,t=2,p=1$AQIDBAUGBwgJCgsMDQ4PEA$AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"
	if _, err := hasher.Verify([]byte("password"), malformed); !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("Verify() error = %v, want ErrInvalidPasswordHash", err)
	}
}

func TestSecretDigestRoundTripUsesFixedSize(t *testing.T) {
	digest := DigestSecret([]byte("random-high-entropy-secret"))
	parsed, err := SecretDigestFromBytes(digest.Bytes())
	if err != nil {
		t.Fatalf("SecretDigestFromBytes() error = %v", err)
	}
	if parsed != digest {
		t.Fatalf("SecretDigestFromBytes() = %x, want %x", parsed, digest)
	}
	if _, err := SecretDigestFromBytes([]byte("short")); !errors.Is(err, ErrInvalidSecretDigest) {
		t.Fatalf("SecretDigestFromBytes(short) error = %v, want ErrInvalidSecretDigest", err)
	}
}
