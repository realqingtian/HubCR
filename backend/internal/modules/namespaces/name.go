package namespaces

import (
	"errors"
	"regexp"
	"strings"
)

const MaxNameLength = 64

var (
	ErrInvalidName = errors.New("invalid namespace name")
	validName      = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

// NormalizeName lowercases an ASCII OCI path component and rejects values that need
// any other lossy transformation. HubCR intentionally uses a strict subset of the
// Distribution repository-name grammar.
func NormalizeName(value string) (string, error) {
	normalized := strings.ToLower(value)
	if len(normalized) < 1 || len(normalized) > MaxNameLength || !validName.MatchString(normalized) {
		return "", ErrInvalidName
	}
	return normalized, nil
}
