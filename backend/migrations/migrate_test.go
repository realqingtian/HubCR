package migrations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"

	"hubcr.io/hubcr/internal/platform/postgres"
)

func TestMigrationsHaveUniqueOrderedIDs(t *testing.T) {
	seen := make(map[string]struct{})
	previous := ""
	for _, migration := range all() {
		if migration.ID <= previous {
			t.Fatalf("migration ID %q is not after %q", migration.ID, previous)
		}
		if _, exists := seen[migration.ID]; exists {
			t.Fatalf("duplicate migration ID %q", migration.ID)
		}
		seen[migration.ID] = struct{}{}
		previous = migration.ID
	}
}

func TestApplyM0UpgradeRepeatAndUnknownMigrationDetection(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL:            databaseURL,
		ConnectTimeout: 3 * time.Second,
		MaxConnections: 2,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()

	foundation := gormigrate.New(pool.ORM().WithContext(ctx), options, all()[:1])
	if err := foundation.Migrate(); err != nil {
		t.Fatalf("apply M0 foundation migration: %v", err)
	}
	var foundationCount int64
	if err := pool.ORM().Table(options.TableName).Count(&foundationCount).Error; err != nil {
		t.Fatalf("count M0 migration records: %v", err)
	}
	if foundationCount != 1 {
		t.Fatalf("M0 migration record count = %d, want 1", foundationCount)
	}

	if err := Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("M0 to M1 Apply() error = %v", err)
	}
	if err := Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	var count int64
	if err := pool.ORM().Table(options.TableName).Count(&count).Error; err != nil {
		t.Fatalf("count migration records: %v", err)
	}
	if count != int64(len(all())) {
		t.Fatalf("migration record count = %d, want %d", count, len(all()))
	}

	type migrationRecord struct {
		ID string `gorm:"column:id;primaryKey"`
	}
	testTransaction := pool.ORM().Begin()
	if testTransaction.Error != nil {
		t.Fatalf("begin unknown-migration test transaction: %v", testTransaction.Error)
	}
	defer testTransaction.Rollback()
	if err := testTransaction.Table(options.TableName).Create(&migrationRecord{ID: "999999_unknown"}).Error; err != nil {
		t.Fatalf("insert unknown migration: %v", err)
	}
	if err := Apply(ctx, testTransaction); err == nil {
		t.Fatal("Apply() with unknown migration error = nil, want an error")
	}
}
