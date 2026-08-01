package migrations

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var migrationNamespaceName = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

type namespaceMigrationRecord struct {
	ID          string              `gorm:"type:uuid;primaryKey"`
	Name        string              `gorm:"type:varchar(64);not null;uniqueIndex:uq_namespaces_name;check:ck_namespaces_name,name ~ '^[a-z0-9]+([._-][a-z0-9]+)*$'"`
	OwnerUserID string              `gorm:"type:uuid;not null;uniqueIndex:uq_namespaces_owner_user_id"`
	OwnerUser   userMigrationRecord `gorm:"foreignKey:OwnerUserID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	CreatedAt   time.Time           `gorm:"type:timestamptz(6);not null"`
}

func (namespaceMigrationRecord) TableName() string { return "namespaces" }

func personalNamespacesMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "000003_personal_namespaces",
		Migrate: func(database *gorm.DB) error {
			if err := database.Migrator().CreateTable(&namespaceMigrationRecord{}); err != nil {
				return err
			}
			var users []userMigrationRecord
			if err := database.Find(&users).Error; err != nil {
				return err
			}
			for _, user := range users {
				name := strings.ToLower(user.Username)
				if len(name) < 1 || len(name) > 64 || !migrationNamespaceName.MatchString(name) {
					return errors.New("existing username cannot become a personal namespace")
				}
				if err := database.Create(&namespaceMigrationRecord{
					ID: user.ID, Name: name, OwnerUserID: user.ID, CreatedAt: user.CreatedAt.UTC(),
				}).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(database *gorm.DB) error {
			return database.Migrator().DropTable(&namespaceMigrationRecord{})
		},
	}
}
