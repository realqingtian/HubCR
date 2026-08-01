package organizations

import (
	"errors"
	"time"
)

type Role string

const (
	RoleOwner  Role = "OWNER"
	RoleAdmin  Role = "ADMIN"
	RoleWriter Role = "WRITER"
	RoleReader Role = "READER"
)

var (
	ErrInvalidRole   = errors.New("invalid organization role")
	ErrNotFound      = errors.New("organization record not found")
	ErrConflict      = errors.New("organization record conflicts with existing data")
	ErrLastOwner     = errors.New("organization must retain at least one owner")
	ErrForbidden     = errors.New("organization action is forbidden")
	ErrInvalidMember = errors.New("invalid organization member")
)

func ParseRole(value string) (Role, error) {
	role := Role(value)
	switch role {
	case RoleOwner, RoleAdmin, RoleWriter, RoleReader:
		return role, nil
	default:
		return "", ErrInvalidRole
	}
}

type Organization struct {
	ID              string
	NamespaceID     string
	NamespaceName   string
	Description     string
	CreatedByUserID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Membership struct {
	OrganizationID string
	UserID         string
	Role           Role
	AddedByUserID  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type NewOrganization struct {
	Organization Organization
	Owner        Membership
}

type PageRequest struct {
	Limit int
	After string
}

type OrganizationPage struct {
	Items     []Organization
	NextAfter string
}

type MembershipPage struct {
	Items     []Membership
	NextAfter string
}
