package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type userMigrationRecord struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	Username  string    `gorm:"type:varchar(64);not null;uniqueIndex:uq_users_username;check:ck_users_username_not_empty,char_length(username) > 0"`
	CreatedAt time.Time `gorm:"type:timestamptz(6);not null"`
	UpdatedAt time.Time `gorm:"type:timestamptz(6);not null"`
}

func (userMigrationRecord) TableName() string { return "users" }

type localCredentialMigrationRecord struct {
	UserID            string              `gorm:"type:uuid;primaryKey"`
	User              userMigrationRecord `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:CASCADE"`
	PasswordHash      string              `gorm:"type:text;not null;check:ck_local_credentials_password_hash_not_empty,char_length(password_hash) > 0"`
	PasswordChangedAt time.Time           `gorm:"type:timestamptz(6);not null"`
	CreatedAt         time.Time           `gorm:"type:timestamptz(6);not null"`
	UpdatedAt         time.Time           `gorm:"type:timestamptz(6);not null"`
}

func (localCredentialMigrationRecord) TableName() string { return "local_credentials" }

type webSessionMigrationRecord struct {
	ID          string              `gorm:"type:uuid;primaryKey"`
	UserID      string              `gorm:"type:uuid;not null;index:idx_web_sessions_user_id"`
	User        userMigrationRecord `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:CASCADE"`
	TokenDigest []byte              `gorm:"type:bytea;not null;uniqueIndex:uq_web_sessions_token_digest;check:ck_web_sessions_token_digest_length,octet_length(token_digest) = 32"`
	ExpiresAt   time.Time           `gorm:"type:timestamptz(6);not null;index:idx_web_sessions_expires_at;check:ck_web_sessions_expiry,expires_at > created_at"`
	RevokedAt   *time.Time          `gorm:"type:timestamptz(6)"`
	CreatedAt   time.Time           `gorm:"type:timestamptz(6);not null"`
}

func (webSessionMigrationRecord) TableName() string { return "web_sessions" }

type userInvitationMigrationRecord struct {
	ID               string               `gorm:"type:uuid;primaryKey"`
	IssuedByUserID   *string              `gorm:"type:uuid;index:idx_user_invitations_issued_by_user_id"`
	IssuedByUser     *userMigrationRecord `gorm:"foreignKey:IssuedByUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	TokenDigest      []byte               `gorm:"type:bytea;not null;uniqueIndex:uq_user_invitations_token_digest;check:ck_user_invitations_token_digest_length,octet_length(token_digest) = 32"`
	ExpiresAt        time.Time            `gorm:"type:timestamptz(6);not null;index:idx_user_invitations_expires_at;check:ck_user_invitations_expiry,expires_at > created_at"`
	RedeemedAt       *time.Time           `gorm:"type:timestamptz(6);check:ck_user_invitations_redemption_pair,(redeemed_at IS NULL) = (redeemed_by_user_id IS NULL);check:ck_user_invitations_terminal_state,NOT (redeemed_at IS NOT NULL AND revoked_at IS NOT NULL)"`
	RedeemedByUserID *string              `gorm:"type:uuid;index:idx_user_invitations_redeemed_by_user_id"`
	RedeemedByUser   *userMigrationRecord `gorm:"foreignKey:RedeemedByUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	RevokedAt        *time.Time           `gorm:"type:timestamptz(6)"`
	CreatedAt        time.Time            `gorm:"type:timestamptz(6);not null"`
}

func (userInvitationMigrationRecord) TableName() string { return "user_invitations" }

func identityPersistenceMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000002_identity_persistence",
		Migrate: func(database *gorm.DB) error {
			return database.Migrator().CreateTable(
				&userMigrationRecord{},
				&localCredentialMigrationRecord{},
				&webSessionMigrationRecord{},
				&userInvitationMigrationRecord{},
			)
		},
		Rollback: func(database *gorm.DB) error {
			return database.Migrator().DropTable(
				&userInvitationMigrationRecord{},
				&webSessionMigrationRecord{},
				&localCredentialMigrationRecord{},
				&userMigrationRecord{},
			)
		},
	}
}
