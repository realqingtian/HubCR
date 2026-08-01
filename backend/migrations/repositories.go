package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type repositoryMigrationRecord struct {
	ID          string                   `gorm:"type:uuid;primaryKey"`
	NamespaceID string                   `gorm:"type:uuid;not null;uniqueIndex:uq_repositories_namespace_name,priority:1"`
	Namespace   namespaceMigrationRecord `gorm:"foreignKey:NamespaceID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Name        string                   `gorm:"type:varchar(64);not null;uniqueIndex:uq_repositories_namespace_name,priority:2;check:ck_repositories_name,name ~ '^[a-z0-9]+([._-][a-z0-9]+)*$'"`
	Visibility  string                   `gorm:"type:varchar(16);not null;check:ck_repositories_visibility,visibility IN ('PUBLIC','PRIVATE')"`
	Description string                   `gorm:"type:text;not null;default:''"`

	CreatedByUserID string              `gorm:"type:uuid;not null;index:idx_repositories_created_by_user_id"`
	CreatedByUser   userMigrationRecord `gorm:"foreignKey:CreatedByUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`

	VisibilityUpdatedByUserID string              `gorm:"type:uuid;not null;index:idx_repositories_visibility_updated_by_user_id"`
	VisibilityUpdatedByUser   userMigrationRecord `gorm:"foreignKey:VisibilityUpdatedByUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	VisibilityUpdatedAt       time.Time           `gorm:"type:timestamptz(6);not null"`
	CreatedAt                 time.Time           `gorm:"type:timestamptz(6);not null"`
	UpdatedAt                 time.Time           `gorm:"type:timestamptz(6);not null"`
}

func (repositoryMigrationRecord) TableName() string { return "repositories" }

func repositoriesMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000005_repositories",
		Migrate: func(database *gorm.DB) error {
			return database.Migrator().CreateTable(&repositoryMigrationRecord{})
		},
		Rollback: func(database *gorm.DB) error {
			return database.Migrator().DropTable(&repositoryMigrationRecord{})
		},
	}
}
