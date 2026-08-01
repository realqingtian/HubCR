package authstore

import (
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
)

type userRecord struct {
	ID        string
	Username  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (userRecord) TableName() string { return "users" }

type localCredentialRecord struct {
	UserID            string
	PasswordHash      string
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (localCredentialRecord) TableName() string { return "local_credentials" }

type personalNamespaceRecord struct {
	ID          string
	Name        string
	OwnerUserID string
	CreatedAt   time.Time
}

func (personalNamespaceRecord) TableName() string { return "namespaces" }

type webSessionRecord struct {
	ID          string
	UserID      string
	TokenDigest []byte
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

func (webSessionRecord) TableName() string { return "web_sessions" }

type userInvitationRecord struct {
	ID               string
	IssuedByUserID   *string
	TokenDigest      []byte
	ExpiresAt        time.Time
	RedeemedAt       *time.Time
	RedeemedByUserID *string
	RevokedAt        *time.Time
	CreatedAt        time.Time
}

func (userInvitationRecord) TableName() string { return "user_invitations" }

func userToRecord(user auth.User) userRecord {
	return userRecord{
		ID:        string(user.ID),
		Username:  user.Username,
		CreatedAt: user.CreatedAt.UTC(),
		UpdatedAt: user.UpdatedAt.UTC(),
	}
}

func credentialToRecord(credential auth.LocalCredential) localCredentialRecord {
	return localCredentialRecord{
		UserID:            string(credential.UserID),
		PasswordHash:      credential.PasswordHash,
		PasswordChangedAt: credential.PasswordChangedAt.UTC(),
		CreatedAt:         credential.CreatedAt.UTC(),
		UpdatedAt:         credential.UpdatedAt.UTC(),
	}
}

func identityFromRecords(user userRecord, credential localCredentialRecord) auth.Identity {
	return auth.Identity{
		User: userFromRecord(user),
		Credential: auth.LocalCredential{
			UserID:            auth.ID(credential.UserID),
			PasswordHash:      credential.PasswordHash,
			PasswordChangedAt: credential.PasswordChangedAt.UTC(),
			CreatedAt:         credential.CreatedAt.UTC(),
			UpdatedAt:         credential.UpdatedAt.UTC(),
		},
		PersonalNamespace: auth.PersonalNamespace{},
	}
}

func userFromRecord(user userRecord) auth.User {
	return auth.User{
		ID:        auth.ID(user.ID),
		Username:  user.Username,
		CreatedAt: user.CreatedAt.UTC(),
		UpdatedAt: user.UpdatedAt.UTC(),
	}
}

func sessionToRecord(session auth.Session) webSessionRecord {
	return webSessionRecord{
		ID:          string(session.ID),
		UserID:      string(session.UserID),
		TokenDigest: session.TokenDigest.Bytes(),
		ExpiresAt:   session.ExpiresAt.UTC(),
		RevokedAt:   utcTimePointer(session.RevokedAt),
		CreatedAt:   session.CreatedAt.UTC(),
	}
}

func sessionFromRecord(record webSessionRecord) (auth.Session, error) {
	digest, err := auth.SecretDigestFromBytes(record.TokenDigest)
	if err != nil {
		return auth.Session{}, fmtInvalidRecord("session token digest", err)
	}
	return auth.Session{
		ID:          auth.ID(record.ID),
		UserID:      auth.ID(record.UserID),
		TokenDigest: digest,
		ExpiresAt:   record.ExpiresAt.UTC(),
		RevokedAt:   utcTimePointer(record.RevokedAt),
		CreatedAt:   record.CreatedAt.UTC(),
	}, nil
}

func invitationToRecord(invitation auth.Invitation) userInvitationRecord {
	return userInvitationRecord{
		ID:               string(invitation.ID),
		IssuedByUserID:   idPointerToString(invitation.IssuedByUserID),
		TokenDigest:      invitation.TokenDigest.Bytes(),
		ExpiresAt:        invitation.ExpiresAt.UTC(),
		RedeemedAt:       utcTimePointer(invitation.RedeemedAt),
		RedeemedByUserID: idPointerToString(invitation.RedeemedByUserID),
		RevokedAt:        utcTimePointer(invitation.RevokedAt),
		CreatedAt:        invitation.CreatedAt.UTC(),
	}
}

func invitationFromRecord(record userInvitationRecord) (auth.Invitation, error) {
	digest, err := auth.SecretDigestFromBytes(record.TokenDigest)
	if err != nil {
		return auth.Invitation{}, fmtInvalidRecord("invitation token digest", err)
	}
	return auth.Invitation{
		ID:               auth.ID(record.ID),
		IssuedByUserID:   stringPointerToID(record.IssuedByUserID),
		TokenDigest:      digest,
		ExpiresAt:        record.ExpiresAt.UTC(),
		RedeemedAt:       utcTimePointer(record.RedeemedAt),
		RedeemedByUserID: stringPointerToID(record.RedeemedByUserID),
		RevokedAt:        utcTimePointer(record.RevokedAt),
		CreatedAt:        record.CreatedAt.UTC(),
	}, nil
}

func idPointerToString(value *auth.ID) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

func stringPointerToID(value *string) *auth.ID {
	if value == nil {
		return nil
	}
	result := auth.ID(*value)
	return &result
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func fmtInvalidRecord(field string, err error) error {
	return &invalidRecordError{field: field, cause: err}
}

type invalidRecordError struct {
	field string
	cause error
}

func (e *invalidRecordError) Error() string { return "invalid persisted " + e.field }
func (e *invalidRecordError) Unwrap() error { return e.cause }
