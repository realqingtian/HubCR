package authstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"hubcr.io/hubcr/internal/modules/auth"
)

type Store struct {
	database *gorm.DB
}

func New(database *gorm.DB) *Store {
	return &Store{database: database}
}

func (s *Store) CreateIdentity(ctx context.Context, identity auth.Identity) error {
	if err := validateIdentity(identity); err != nil {
		return err
	}
	if err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return createIdentity(transaction, identity)
	}); err != nil {
		return classify("create identity", err)
	}
	return nil
}

func (s *Store) CredentialByUsername(ctx context.Context, username string) (auth.Identity, error) {
	var user userRecord
	if err := s.database.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return auth.Identity{}, classify("find user by username", err)
	}

	var credential localCredentialRecord
	if err := s.database.WithContext(ctx).Where("user_id = ?", user.ID).First(&credential).Error; err != nil {
		return auth.Identity{}, classify("find local credential", err)
	}
	var namespace personalNamespaceRecord
	if err := s.database.WithContext(ctx).Where("owner_user_id = ?", user.ID).First(&namespace).Error; err != nil {
		return auth.Identity{}, classify("find personal namespace", err)
	}
	identity := identityFromRecords(user, credential)
	identity.PersonalNamespace = auth.PersonalNamespace{ID: auth.ID(namespace.ID), Name: namespace.Name}
	identity.User.PersonalNamespace = namespace.Name
	return identity, nil
}

func (s *Store) UserByID(ctx context.Context, id auth.ID) (auth.User, error) {
	var record userRecord
	if err := s.database.WithContext(ctx).Where("id = ?", string(id)).First(&record).Error; err != nil {
		return auth.User{}, classify("find user by ID", err)
	}
	var namespace personalNamespaceRecord
	if err := s.database.WithContext(ctx).Where("owner_user_id = ?", string(id)).First(&namespace).Error; err != nil {
		return auth.User{}, classify("find user personal namespace", err)
	}
	user := userFromRecord(record)
	user.PersonalNamespace = namespace.Name
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, session auth.Session) error {
	record := sessionToRecord(session)
	if err := s.database.WithContext(ctx).Create(&record).Error; err != nil {
		return classify("create session", err)
	}
	return nil
}

func (s *Store) SessionByTokenDigest(ctx context.Context, digest auth.SecretDigest) (auth.Session, error) {
	var record webSessionRecord
	if err := s.database.WithContext(ctx).Where("token_digest = ?", digest.Bytes()).First(&record).Error; err != nil {
		return auth.Session{}, classify("find session", err)
	}
	return sessionFromRecord(record)
}

func (s *Store) RevokeSession(ctx context.Context, id auth.ID, at time.Time) error {
	database := s.database.WithContext(ctx)
	result := database.Model(&webSessionRecord{}).
		Where("id = ? AND revoked_at IS NULL", string(id)).
		Update("revoked_at", at.UTC())
	if result.Error != nil {
		return classify("revoke session", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	var record webSessionRecord
	if err := database.Select("id", "revoked_at").Where("id = ?", string(id)).First(&record).Error; err != nil {
		return classify("find session for revocation", err)
	}
	return nil
}

func (s *Store) CreateInvitation(ctx context.Context, invitation auth.Invitation) error {
	record := invitationToRecord(invitation)
	if err := s.database.WithContext(ctx).Create(&record).Error; err != nil {
		return classify("create invitation", err)
	}
	return nil
}

func (s *Store) InvitationByTokenDigest(ctx context.Context, digest auth.SecretDigest) (auth.Invitation, error) {
	var record userInvitationRecord
	if err := s.database.WithContext(ctx).Where("token_digest = ?", digest.Bytes()).First(&record).Error; err != nil {
		return auth.Invitation{}, classify("find invitation", err)
	}
	return invitationFromRecord(record)
}

func (s *Store) RedeemInvitationWithIdentity(
	ctx context.Context,
	invitationID auth.ID,
	at time.Time,
	identity auth.Identity,
) error {
	if err := validateIdentity(identity); err != nil {
		return err
	}
	if identity.User.ID == "" || invitationID == "" {
		return auth.ErrInvalidState
	}

	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := createIdentity(transaction, identity); err != nil {
			return err
		}

		result := transaction.Model(&userInvitationRecord{}).
			Where(
				"id = ? AND redeemed_at IS NULL AND revoked_at IS NULL AND expires_at > ?",
				string(invitationID),
				at.UTC(),
			).
			Updates(map[string]any{
				"redeemed_at":         at.UTC(),
				"redeemed_by_user_id": string(identity.User.ID),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var invitation userInvitationRecord
			if err := transaction.Select("id").Where("id = ?", string(invitationID)).First(&invitation).Error; err != nil {
				return err
			}
			return auth.ErrInvalidState
		}
		return nil
	})
	if err != nil {
		return classify("redeem invitation", err)
	}
	return nil
}

func (s *Store) RevokeInvitation(ctx context.Context, id auth.ID, at time.Time) error {
	database := s.database.WithContext(ctx)
	result := database.Model(&userInvitationRecord{}).
		Where("id = ? AND redeemed_at IS NULL AND revoked_at IS NULL", string(id)).
		Update("revoked_at", at.UTC())
	if result.Error != nil {
		return classify("revoke invitation", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	var record userInvitationRecord
	if err := database.Select("id", "redeemed_at", "revoked_at").Where("id = ?", string(id)).First(&record).Error; err != nil {
		return classify("find invitation for revocation", err)
	}
	if record.RevokedAt != nil {
		return nil
	}
	return auth.ErrInvalidState
}

func validateIdentity(identity auth.Identity) error {
	if identity.User.ID == "" || identity.Credential.UserID != identity.User.ID || identity.Credential.PasswordHash == "" {
		return auth.ErrInvalidState
	}
	if identity.PersonalNamespace.ID == "" || identity.PersonalNamespace.Name == "" {
		return auth.ErrInvalidState
	}
	return nil
}

func createIdentity(database *gorm.DB, identity auth.Identity) error {
	user := userToRecord(identity.User)
	if err := database.Create(&user).Error; err != nil {
		return err
	}
	credential := credentialToRecord(identity.Credential)
	if err := database.Create(&credential).Error; err != nil {
		return err
	}
	namespace := personalNamespaceRecord{
		ID:          string(identity.PersonalNamespace.ID),
		Name:        identity.PersonalNamespace.Name,
		OwnerUserID: string(identity.User.ID),
		CreatedAt:   identity.User.CreatedAt.UTC(),
	}
	if err := database.Create(&namespace).Error; err != nil {
		return err
	}
	return nil
}

func classify(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, auth.ErrInvalidState) {
		return fmt.Errorf("%s: %w", operation, auth.ErrInvalidState)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, auth.ErrNotFound)
	}

	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, auth.ErrConflict)
		case "23502", "23503", "23514", "22P02":
			return fmt.Errorf("%s: %w", operation, auth.ErrInvalidState)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

var _ auth.Store = (*Store)(nil)
