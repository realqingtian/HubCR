// Command m4trustseed creates one immutable trust-policy version for the isolated
// M4 acceptance runner. It is not a product policy-management entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/securitystore"
	"hubcr.io/hubcr/migrations"
)

func main() {
	keyPath := os.Getenv("HUBCR_E2E_COSIGN_PUBLIC_KEY_FILE")
	keyName := os.Getenv("HUBCR_E2E_COSIGN_KEY_NAME")
	if keyPath == "" || keyName == "" {
		fail("HUBCR_E2E_COSIGN_PUBLIC_KEY_FILE and HUBCR_E2E_COSIGN_KEY_NAME are required")
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		fail("read public key: %v", err)
	}
	key, err := security.NewPublicKeyTrust(keyName, keyPEM)
	if err != nil {
		fail("validate public key: %v", err)
	}
	databaseConfig, err := config.LoadDatabase()
	if err != nil {
		fail("load database configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseConfig.URL, ConnectTimeout: databaseConfig.ConnectTimeout,
		MaxConnections: databaseConfig.MaxConnections,
	})
	if err != nil {
		fail("open PostgreSQL: %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		fail("apply migrations: %v", err)
	}
	var owner struct{ ID string }
	if err := pool.ORM().WithContext(ctx).Raw(
		"SELECT id FROM users WHERE username = 'm2-e2e-owner'",
	).Scan(&owner).Error; err != nil || owner.ID == "" {
		fail("resolve fixture owner: %v", err)
	}
	var namespace struct{ ID string }
	if err := pool.ORM().WithContext(ctx).Raw(
		"SELECT id FROM namespaces WHERE name = 'm2-e2e-team'",
	).Scan(&namespace).Error; err != nil || namespace.ID == "" {
		fail("resolve fixture namespace: %v", err)
	}
	service, err := security.NewTrustService(securitystore.New(pool.ORM()), time.Now)
	if err != nil {
		fail("initialize trust service: %v", err)
	}
	policy, err := service.CreatePolicy(
		ctx, namespace.ID, owner.ID, []security.PublicKeyTrust{key}, nil,
	)
	if err != nil {
		fail("create trust policy: %v", err)
	}
	fmt.Printf("policy_version=%d fingerprint=%s\n", policy.Version, key.Fingerprint)
}

func fail(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
