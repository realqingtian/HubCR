package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("auth record not found")
	ErrConflict     = errors.New("auth record conflicts with existing data")
	ErrInvalidState = errors.New("auth record is not in a valid state")
)

// Store is consumed by the auth application service. Implementations must preserve
// the atomicity described by each method.
type Store interface {
	CreateIdentity(context.Context, Identity) error
	CredentialByUsername(context.Context, string) (Identity, error)
	UserByID(context.Context, ID) (User, error)

	CreateSession(context.Context, Session) error
	SessionByTokenDigest(context.Context, SecretDigest) (Session, error)
	RevokeSession(context.Context, ID, time.Time) error

	CreateInvitation(context.Context, Invitation) error
	InvitationByTokenDigest(context.Context, SecretDigest) (Invitation, error)
	RedeemInvitationWithIdentity(context.Context, ID, time.Time, Identity) error
	RevokeInvitation(context.Context, ID, time.Time) error
}
