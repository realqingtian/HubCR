package migrations

import (
	"context"
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migrationLockID int64 = 6842399173202401

var options = &gormigrate.Options{
	TableName:                 "hubcr_schema_migrations",
	IDColumnName:              "id",
	IDColumnSize:              190,
	UseTransaction:            false,
	ValidateUnknownMigrations: true,
}

func Apply(ctx context.Context, database *gorm.DB) error {
	return database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Exec("SELECT pg_advisory_xact_lock(?)", migrationLockID).Error; err != nil {
			return fmt.Errorf("lock migrations: %w", err)
		}

		migrator := gormigrate.New(transaction, options, all())
		if err := migrator.Migrate(); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		return nil
	})
}

func all() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		{
			ID: "000001_foundation",
			Migrate: func(*gorm.DB) error {
				// Product tables begin only after G-01 and G-02 are approved.
				return nil
			},
		},
		identityPersistenceMigration(),
		personalNamespacesMigration(),
		organizationsMigration(),
		repositoriesMigration(),
	}
}
