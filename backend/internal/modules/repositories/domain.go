package repositories

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type Visibility string

const (
	VisibilityPublic  Visibility = "PUBLIC"
	VisibilityPrivate Visibility = "PRIVATE"
)

var (
	ErrInvalidVisibility = errors.New("invalid repository visibility")
	ErrInvalidRepository = errors.New("invalid repository")
	ErrInvalidUpdate     = errors.New("invalid repository update")
	ErrNotFound          = errors.New("repository not found")
	ErrConflict          = errors.New("repository conflicts with existing data")
	ErrForbidden         = errors.New("repository action is forbidden")
)

const MaxDescriptionLength = 1024

func ParseVisibility(value string) (Visibility, error) {
	visibility := Visibility(value)
	switch visibility {
	case VisibilityPublic, VisibilityPrivate:
		return visibility, nil
	default:
		return "", ErrInvalidVisibility
	}
}

type Repository struct {
	ID                        string
	NamespaceID               string
	Name                      string
	Visibility                Visibility
	Description               string
	CreatedByUserID           string
	VisibilityUpdatedByUserID string
	VisibilityUpdatedAt       time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type NewRepository struct {
	NamespaceID     string
	RequestedName   string
	Visibility      Visibility
	Description     string
	CreatedByUserID string
}

type UpdateRepository struct {
	Description *string
	Visibility  *Visibility
}

type PersistedUpdate struct {
	Description *string
	Visibility  *Visibility
	ActorUserID string
	At          time.Time
}

type PageRequest struct {
	Limit int
	After string
}

type RepositoryPage struct {
	Items     []Repository
	NextAfter string
}

// AuthorizationContext contains validated repository and namespace state for
// consumers such as Registry token authorization. It deliberately does not decide
// whether a caller may perform an action.
type AuthorizationContext struct {
	Repository Repository
	Namespace  NamespaceAccess
}

// New constructs repository state after authorization has been decided by the
// caller. Repository creation APIs must perform their capability check before
// persisting the returned value.
func New(input NewRepository, at time.Time) (Repository, error) {
	if input.NamespaceID == "" || input.CreatedByUserID == "" {
		return Repository{}, ErrInvalidRepository
	}
	if len(input.Description) > MaxDescriptionLength {
		return Repository{}, ErrInvalidRepository
	}
	name, err := NormalizeName(input.RequestedName)
	if err != nil {
		return Repository{}, err
	}
	visibility, err := ParseVisibility(string(input.Visibility))
	if err != nil {
		return Repository{}, err
	}
	id, err := newID()
	if err != nil {
		return Repository{}, err
	}
	now := at.UTC()
	return Repository{
		ID: id, NamespaceID: input.NamespaceID, Name: name,
		Visibility: visibility, Description: input.Description,
		CreatedByUserID: input.CreatedByUserID, VisibilityUpdatedByUserID: input.CreatedByUserID,
		VisibilityUpdatedAt: now, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate repository ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
