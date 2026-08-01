// Command m1seed creates deterministic local identities for the M1 full-stack test.
// It is test support only and is not a product registration or bootstrap entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/migrations"
)

const (
	ownerID        auth.ID = "91919191-9191-4919-8919-919191919191"
	memberID       auth.ID = "92929292-9292-4929-8929-929292929292"
	ownerUsername          = "m1-e2e-owner"
	memberUsername         = "m1-e2e-member"
)

func main() {
	password := []byte(os.Getenv("HUBCR_E2E_PASSWORD"))
	if len(password) < 12 {
		fail("HUBCR_E2E_PASSWORD must contain at least 12 bytes")
	}
	defer clear(password)

	databaseConfig, err := config.LoadDatabase()
	if err != nil {
		fail("load database configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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

	hasher := auth.NewPasswordHasher()
	store := authstore.New(pool.ORM())
	now := time.Now().UTC()
	for _, input := range []struct {
		id       auth.ID
		username string
	}{{ownerID, ownerUsername}, {memberID, memberUsername}} {
		hash, err := hasher.Hash(password)
		if err != nil {
			fail("hash test identity password: %v", err)
		}
		identity := auth.Identity{
			User: auth.User{ID: input.id, Username: input.username, CreatedAt: now, UpdatedAt: now},
			Credential: auth.LocalCredential{
				UserID: input.id, PasswordHash: hash, PasswordChangedAt: now,
				CreatedAt: now, UpdatedAt: now,
			},
			PersonalNamespace: auth.PersonalNamespace{ID: input.id, Name: input.username},
		}
		if err := store.CreateIdentity(ctx, identity); err != nil {
			fail("create test identity %s: %v", input.username, err)
		}
	}
}

func fail(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
