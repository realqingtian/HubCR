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

func TestSignatureTrustMigrationIsCurrent(t *testing.T) {
	migrations := all()
	if got := migrations[len(migrations)-1].ID; got != "000009_signature_trust" {
		t.Fatalf("last migration ID = %q, want 000009_signature_trust", got)
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
	assertJobFoundationSchema(t, pool)
	assertSignatureTrustSchema(t, pool)

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

func assertSignatureTrustSchema(t *testing.T, pool *postgres.Pool) {
	t.Helper()
	migrator := pool.ORM().Migrator()
	for name, record := range map[string]any{
		"trust_policies":           &trustPolicyMigrationRecord{},
		"trust_policy_public_keys": &trustPolicyPublicKeyMigrationRecord{},
		"trust_policy_identities":  &trustPolicyIdentityMigrationRecord{},
		"signature_workflows":      &signatureWorkflowMigrationRecord{},
		"signature_verifications":  &signatureVerificationMigrationRecord{},
		"signature_evidence":       &signatureEvidenceMigrationRecord{},
		"cosign_tool_state":        &cosignToolStateMigrationRecord{},
	} {
		if !migrator.HasTable(record) {
			t.Fatalf("signature trust table %q is missing", name)
		}
	}
	for record, names := range map[any][]string{
		&trustPolicyMigrationRecord{}:       {"ck_trust_policies_version", "fk_trust_policies_namespace", "fk_trust_policies_creator"},
		&signatureWorkflowMigrationRecord{}: {"ck_signature_workflows_policy_version", "fk_signature_workflows_artifact", "fk_signature_workflows_policy", "fk_signature_workflows_job"},
		&signatureEvidenceMigrationRecord{}: {"ck_signature_evidence_digest", "ck_signature_evidence_kind", "ck_signature_evidence_signer", "ck_signature_evidence_state"},
	} {
		for _, name := range names {
			if !migrator.HasConstraint(record, name) {
				t.Fatalf("signature trust constraint %q is missing", name)
			}
		}
	}
}

func assertJobFoundationSchema(t *testing.T, pool *postgres.Pool) {
	t.Helper()
	migrator := pool.ORM().Migrator()
	if !migrator.HasTable(&jobMigrationRecord{}) {
		t.Fatal("job foundation table is missing")
	}
	for _, name := range []string{
		"ck_jobs_kind", "ck_jobs_intent_key", "ck_jobs_payload", "ck_jobs_state",
		"ck_jobs_attempts", "ck_jobs_error_code", "ck_jobs_timestamps", "ck_jobs_lifecycle",
	} {
		if !migrator.HasConstraint(&jobMigrationRecord{}, name) {
			t.Fatalf("job foundation constraint %q is missing", name)
		}
	}
	for _, name := range []string{"uq_jobs_intent_key", "idx_jobs_claim_queued", "idx_jobs_claim_expired"} {
		if !migrator.HasIndex(&jobMigrationRecord{}, name) {
			t.Fatalf("job foundation index %q is missing", name)
		}
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
