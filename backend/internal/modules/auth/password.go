package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordSaltLength = 16
	passwordKeyLength  = 32
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

type passwordParameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultPasswordParameters = passwordParameters{
	memory:      19 * 1024,
	iterations:  2,
	parallelism: 1,
	saltLength:  passwordSaltLength,
	keyLength:   passwordKeyLength,
}

// PasswordHasher creates self-describing Argon2id hashes with a unique random salt.
type PasswordHasher struct {
	random io.Reader
	params passwordParameters
}

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{random: rand.Reader, params: defaultPasswordParameters}
}

func (h *PasswordHasher) Hash(password []byte) (string, error) {
	salt := make([]byte, h.params.saltLength)
	if _, err := io.ReadFull(h.random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	derived := argon2.IDKey(
		password,
		salt,
		h.params.iterations,
		h.params.memory,
		h.params.parallelism,
		h.params.keyLength,
	)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.memory,
		h.params.iterations,
		h.params.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(derived),
	), nil
}

func (h *PasswordHasher) Verify(password []byte, encoded string) (bool, error) {
	params, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey(
		password,
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		params.keyLength,
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func (h *PasswordHasher) NeedsRehash(encoded string) bool {
	params, _, _, err := parsePasswordHash(encoded)
	return err != nil || params != h.params
}

func parsePasswordHash(encoded string) (passwordParameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return passwordParameters{}, nil, nil, ErrInvalidPasswordHash
	}

	params, err := parsePasswordParameters(parts[3])
	if err != nil {
		return passwordParameters{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || uint32(len(salt)) != params.saltLength {
		return passwordParameters{}, nil, nil, ErrInvalidPasswordHash
	}
	derived, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || uint32(len(derived)) != params.keyLength {
		return passwordParameters{}, nil, nil, ErrInvalidPasswordHash
	}
	return params, salt, derived, nil
}

func parsePasswordParameters(value string) (passwordParameters, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return passwordParameters{}, ErrInvalidPasswordHash
	}
	memory, err := parseUintParameter(parts[0], "m=", 1024, 1024*1024, 32)
	if err != nil {
		return passwordParameters{}, err
	}
	iterations, err := parseUintParameter(parts[1], "t=", 1, 10, 32)
	if err != nil {
		return passwordParameters{}, err
	}
	parallelism, err := parseUintParameter(parts[2], "p=", 1, 16, 8)
	if err != nil {
		return passwordParameters{}, err
	}
	return passwordParameters{
		memory:      uint32(memory),
		iterations:  uint32(iterations),
		parallelism: uint8(parallelism),
		saltLength:  passwordSaltLength,
		keyLength:   passwordKeyLength,
	}, nil
}

func parseUintParameter(value, prefix string, minimum, maximum uint64, bits int) (uint64, error) {
	if !strings.HasPrefix(value, prefix) {
		return 0, ErrInvalidPasswordHash
	}
	parsed, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, bits)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, ErrInvalidPasswordHash
	}
	return parsed, nil
}
