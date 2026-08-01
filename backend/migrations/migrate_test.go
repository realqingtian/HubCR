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

func TestArtifactMetadataMigrationIsCurrent(t *testing.T) {
	migrations := all()
	if got := migrations[len(migrations)-1].ID; got != "000006_artifact_metadata" {
		t.Fatalf("last migration ID = %q, want 000006_artifact_metadata", got)
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
	assertArtifactMetadataSchema(t, pool)

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

func assertArtifactMetadataSchema(t *testing.T, pool *postgres.Pool) {
	t.Helper()
	migrator := pool.ORM().Migrator()
	for name, record := range map[string]any{
		"artifacts":            &artifactMigrationRecord{},
		"tags":                 &tagMigrationRecord{},
		"manifest_descriptors": &manifestDescriptorMigrationRecord{},
	} {
		if !migrator.HasTable(record) {
			t.Fatalf("artifact metadata table %q is missing", name)
		}
	}
	for record, names := range map[any][]string{
		&artifactMigrationRecord{}: {
			"ck_artifacts_digest", "ck_artifacts_kind", "ck_artifacts_size",
			"ck_artifacts_descriptor_kind", "ck_artifacts_timestamps",
		},
		&tagMigrationRecord{}: {
			"ck_tags_name", "ck_tags_timestamps", "fk_tags_artifact",
		},
		&manifestDescriptorMigrationRecord{}: {
			"ck_manifest_descriptors_distinct", "ck_manifest_descriptors_position",
			"ck_manifest_descriptors_platform", "fk_manifest_descriptors_index",
			"fk_manifest_descriptors_child",
		},
	} {
		for _, name := range names {
			if !migrator.HasConstraint(record, name) {
				t.Fatalf("artifact metadata constraint %q is missing", name)
			}
		}
	}
	for _, name := range []string{"uq_artifacts_repository_digest", "uq_artifacts_repository_id_id"} {
		if !migrator.HasIndex(&artifactMigrationRecord{}, name) {
			t.Fatalf("artifact metadata index %q is missing", name)
		}
	}
}
