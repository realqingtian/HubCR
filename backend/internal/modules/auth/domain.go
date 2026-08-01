package auth

import "time"

// User is the domain representation of an interactive HubCR user.
type User struct {
	ID                ID
	Username          string
	PersonalNamespace string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// LocalCredential contains only the derived password representation. Plaintext
// passwords must never cross the persistence boundary.
type LocalCredential struct {
	UserID            ID
	PasswordHash      string
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Identity groups the records that must be created atomically for a local user.
type Identity struct {
	User              User
	Credential        LocalCredential
	PersonalNamespace PersonalNamespace
}

type PersonalNamespace struct {
	ID   ID
	Name string
}

// Session is a revocable server-side web session. TokenDigest is derived from the
// caller-held secret; the raw secret is never persisted.
type Session struct {
	ID          ID
	UserID      ID
	TokenDigest SecretDigest
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

// Invitation is a single-use, expiring administrator invitation. IssuedByUserID is
// nil only for an instance bootstrap invitation created outside the public API.
type Invitation struct {
	ID               ID
	IssuedByUserID   *ID
	TokenDigest      SecretDigest
	ExpiresAt        time.Time
	RedeemedAt       *time.Time
	RedeemedByUserID *ID
	RevokedAt        *time.Time
	CreatedAt        time.Time
}
