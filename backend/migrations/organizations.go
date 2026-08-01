package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type organizationMigrationRecord struct {
	ID              string                   `gorm:"type:uuid;primaryKey"`
	NamespaceID     string                   `gorm:"type:uuid;not null;uniqueIndex:uq_organizations_namespace_id"`
	Namespace       namespaceMigrationRecord `gorm:"foreignKey:NamespaceID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Description     string                   `gorm:"type:text;not null;default:''"`
	CreatedByUserID string                   `gorm:"type:uuid;not null;index:idx_organizations_created_by_user_id"`
	CreatedByUser   userMigrationRecord      `gorm:"foreignKey:CreatedByUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	CreatedAt       time.Time                `gorm:"type:timestamptz(6);not null"`
	UpdatedAt       time.Time                `gorm:"type:timestamptz(6);not null"`
}

func (organizationMigrationRecord) TableName() string { return "organizations" }

type organizationMembershipMigrationRecord struct {
	OrganizationID string                      `gorm:"type:uuid;primaryKey"`
	Organization   organizationMigrationRecord `gorm:"foreignKey:OrganizationID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:CASCADE"`
	UserID         string                      `gorm:"type:uuid;primaryKey;index:idx_organization_memberships_user_id"`
	User           userMigrationRecord         `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Role           string                      `gorm:"type:varchar(16);not null;check:ck_organization_memberships_role,role IN ('OWNER','ADMIN','WRITER','READER')"`
	AddedByUserID  string                      `gorm:"type:uuid;not null;index:idx_organization_memberships_added_by_user_id"`
	AddedByUser    userMigrationRecord         `gorm:"foreignKey:AddedByUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	CreatedAt      time.Time                   `gorm:"type:timestamptz(6);not null"`
	UpdatedAt      time.Time                   `gorm:"type:timestamptz(6);not null"`
}

func (organizationMembershipMigrationRecord) TableName() string {
	return "organization_memberships"
}

func organizationsMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000004_organizations",
		Migrate: func(database *gorm.DB) error {
			if err := database.Exec(
				"ALTER TABLE namespaces ADD COLUMN owner_kind varchar(16) NOT NULL DEFAULT 'PERSONAL'",
			).Error; err != nil {
				return err
			}
			if err := database.Exec(
				"ALTER TABLE namespaces ALTER COLUMN owner_user_id DROP NOT NULL",
			).Error; err != nil {
				return err
			}
			if err := database.Exec(
				"ALTER TABLE namespaces ADD CONSTRAINT ck_namespaces_owner CHECK ((owner_kind = 'PERSONAL' AND owner_user_id IS NOT NULL) OR (owner_kind = 'ORGANIZATION' AND owner_user_id IS NULL))",
			).Error; err != nil {
				return err
			}
			return database.Migrator().CreateTable(
				&organizationMigrationRecord{},
				&organizationMembershipMigrationRecord{},
			)
		},
		Rollback: func(database *gorm.DB) error {
			if err := database.Migrator().DropTable(
				&organizationMembershipMigrationRecord{},
				&organizationMigrationRecord{},
			); err != nil {
				return err
			}
			if err := database.Exec("ALTER TABLE namespaces DROP CONSTRAINT ck_namespaces_owner").Error; err != nil {
				return err
			}
			if err := database.Exec("ALTER TABLE namespaces ALTER COLUMN owner_user_id SET NOT NULL").Error; err != nil {
				return err
			}
			return database.Exec("ALTER TABLE namespaces DROP COLUMN owner_kind").Error
		},
	}
}
