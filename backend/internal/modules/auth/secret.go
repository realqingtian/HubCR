package auth

import (
	"crypto/sha256"
	"errors"
)

const SecretDigestSize = sha256.Size

var ErrInvalidSecretDigest = errors.New("invalid secret digest")

// SecretDigest is the one-way database representation of a high-entropy session or
// invitation secret. SHA-256 is appropriate here because these secrets are randomly
// generated, unlike human-selected passwords.
type SecretDigest [SecretDigestSize]byte

func DigestSecret(secret []byte) SecretDigest {
	return sha256.Sum256(secret)
}

func SecretDigestFromBytes(value []byte) (SecretDigest, error) {
	var digest SecretDigest
	if len(value) != SecretDigestSize {
		return digest, ErrInvalidSecretDigest
	}
	copy(digest[:], value)
	return digest, nil
}

func (d SecretDigest) Bytes() []byte {
	result := make([]byte, len(d))
	copy(result, d[:])
	return result
}
