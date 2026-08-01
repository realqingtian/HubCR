package repositories

import (
	"errors"
	"regexp"
	"strings"
)

const MaxNameLength = 64

var (
	ErrInvalidName = errors.New("invalid repository name")
	validName      = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

// NormalizeName lowercases one ASCII OCI path component. Whitespace, Unicode,
// path separators, and repeated separators are rejected rather than rewritten.
func NormalizeName(value string) (string, error) {
	normalized := strings.ToLower(value)
	if len(normalized) < 1 || len(normalized) > MaxNameLength || !validName.MatchString(normalized) {
		return "", ErrInvalidName
	}
	return normalized, nil
}
