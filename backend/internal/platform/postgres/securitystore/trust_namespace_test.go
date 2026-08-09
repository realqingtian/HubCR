package securitystore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/migrations"
)

// TestTrustStoreRepairsSingleNamespaceAndReadsCurrentPolicy covers the M5-01 namespace-
// scoped repair and the current-policy read used by the management API. It seeds two
// namespaces each with a policy and an artifact, confirms the namespace-scoped repair
// enqueues a workflow only for the target namespace, that it is idempotent, and that
// CurrentTrustPolicy returns the highest version (and ErrNotFound when none exists).
func TestTrustStoreRepairsSingleNamespaceAndReadsCurrentPolicy(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 8,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 9, 4, 0, 0, 0, time.UTC)
	target := createFixture(t, ctx, pool, now)
	defer target.cleanup(t, ctx, pool)
	other := createFixture(t, ctx, pool, now)
	defer other.cleanup(t, ctx, pool)
	store := New(pool.ORM())

	targetKey := trustTestPublicKey(t, "target")
	otherKey := trustTestPublicKey(t, "other")
	targetPolicy, err := store.CreateTrustPolicy(
		ctx, target.namespaceID, target.userID, []security.PublicKeyTrust{targetKey}, nil, now,
	)
	if err != nil {
		t.Fatalf("CreateTrustPolicy(target) error = %v", err)
	}
	otherPolicy, err := store.CreateTrustPolicy(
		ctx, other.namespaceID, other.userID, []security.PublicKeyTrust{otherKey}, nil, now,
	)
	if err != nil {
		t.Fatalf("CreateTrustPolicy(other) error = %v", err)
	}

	// No policy for a never-used namespace.
	missing, err := store.CurrentTrustPolicy(ctx, target.namespaceID+"x")
	if !errors.Is(err, security.ErrNotFound) {
		t.Fatalf("CurrentTrustPolicy(unknown) = %#v, %v; want ErrNotFound", missing, err)
	}

	// Current policy reflects the highest version.
	current, err := store.CurrentTrustPolicy(ctx, target.namespaceID)
	if err != nil || current.ID != targetPolicy.ID || current.Version != 1 {
		t.Fatalf("CurrentTrustPolicy(target) = %#v, %v", current, err)
	}

	// Namespace-scoped repair enqueues only the target namespace artifact.
	repaired, err := store.RepairMissingVerificationWorkflowsForNamespace(
		ctx, target.namespaceID, 100, now.Add(500*time.Millisecond),
	)
	if err != nil || repaired != 1 {
		t.Fatalf("RepairMissingVerificationWorkflowsForNamespace(target) = %d, %v; want 1", repaired, err)
	}
	// The other namespace must remain unrepaired.
	if repairedOther, err := countWorkflows(t, ctx, pool, other.repositoryID); err != nil || repairedOther != 0 {
		t.Fatalf("other namespace workflow count = %d, %v; want 0", repairedOther, err)
	}
	if repairedTarget, err := countWorkflows(t, ctx, pool, target.repositoryID); err != nil || repairedTarget != 1 {
		t.Fatalf("target namespace workflow count = %d, %v; want 1", repairedTarget, err)
	}

	// Idempotent: running again enqueues nothing new.
	repairedAgain, err := store.RepairMissingVerificationWorkflowsForNamespace(
		ctx, target.namespaceID, 100, now.Add(time.Second),
	)
	if err != nil || repairedAgain != 0 {
		t.Fatalf("repeat namespace repair = %d, %v; want 0", repairedAgain, err)
	}

	// A new policy version becomes current; the namespace-scoped repair picks it up.
	newPolicy, err := store.CreateTrustPolicy(
		ctx, target.namespaceID, target.userID, []security.PublicKeyTrust{trustTestPublicKey(t, "v2")}, nil, now.Add(2*time.Second),
	)
	if err != nil || newPolicy.Version != 2 {
		t.Fatalf("CreateTrustPolicy(v2) = %#v, %v", newPolicy, err)
	}
	currentAfterNew, err := store.CurrentTrustPolicy(ctx, target.namespaceID)
	if err != nil || currentAfterNew.ID != newPolicy.ID || currentAfterNew.Version != 2 {
		t.Fatalf("CurrentTrustPolicy after v2 = %#v, %v", currentAfterNew, err)
	}
	repairedForNew, err := store.RepairMissingVerificationWorkflowsForNamespace(
		ctx, target.namespaceID, 100, now.Add(3*time.Second),
	)
	if err != nil || repairedForNew != 1 {
		t.Fatalf("namespace repair after new policy = %d, %v; want 1", repairedForNew, err)
	}

	// Invalid arguments are rejected.
	if n, err := store.RepairMissingVerificationWorkflowsForNamespace(ctx, "", 10, now); !errors.Is(err, security.ErrInvalid) {
		t.Fatalf("repair empty namespace = %d, %v; want ErrInvalid", n, err)
	}
	if n, err := store.RepairMissingVerificationWorkflowsForNamespace(ctx, target.namespaceID, 0, now); !errors.Is(err, security.ErrInvalid) {
		t.Fatalf("repair zero limit = %d, %v; want ErrInvalid", n, err)
	}
	_ = otherPolicy
}

func countWorkflows(t *testing.T, ctx context.Context, pool *postgres.Pool, repositoryID string) (int64, error) {
	t.Helper()
	var count int64
	if err := pool.ORM().WithContext(ctx).Table("signature_workflows").
		Where("repository_id = ?", repositoryID).Count(&count).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}
