// Command m2seed creates deterministic local identities and repositories for the M2
// Registry full-stack test. It is not a product bootstrap entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/migrations"
)

const (
	ownerID    auth.ID = "a1a1a1a1-a1a1-41a1-81a1-a1a1a1a1a1a1"
	readerID   auth.ID = "a2a2a2a2-a2a2-42a2-82a2-a2a2a2a2a2a2"
	outsiderID auth.ID = "a3a3a3a3-a3a3-43a3-83a3-a3a3a3a3a3a3"

	ownerUsername    = "m2-e2e-owner"
	readerUsername   = "m2-e2e-reader"
	outsiderUsername = "m2-e2e-outsider"
	organizationName = "m2-e2e-team"
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

	hasher := auth.NewPasswordHasher()
	identityStore := authstore.New(pool.ORM())
	now := time.Now().UTC()
	for _, input := range []struct {
		id       auth.ID
		username string
	}{
		{id: ownerID, username: ownerUsername},
		{id: readerID, username: readerUsername},
		{id: outsiderID, username: outsiderUsername},
	} {
		passwordHash, err := hasher.Hash(password)
		if err != nil {
			fail("hash test identity password: %v", err)
		}
		if err := identityStore.CreateIdentity(ctx, auth.Identity{
			User: auth.User{
				ID: input.id, Username: input.username, CreatedAt: now, UpdatedAt: now,
			},
			Credential: auth.LocalCredential{
				UserID: input.id, PasswordHash: passwordHash, PasswordChangedAt: now,
				CreatedAt: now, UpdatedAt: now,
			},
			PersonalNamespace: auth.PersonalNamespace{
				ID: input.id, Name: input.username,
			},
		}); err != nil {
			fail("create test identity %s: %v", input.username, err)
		}
	}

	policy := authorization.NewPolicy()
	organizationService, err := organizations.NewService(
		organizationstore.New(pool.ORM()), namespaces.NormalizeName, time.Now, policy,
	)
	if err != nil {
		fail("initialize organization service: %v", err)
	}
	organization, err := organizationService.Create(
		ctx, string(ownerID), organizationName, "M2 Registry end-to-end fixture",
	)
	if err != nil {
		fail("create test organization: %v", err)
	}
	if err := organizationService.AddMember(
		ctx, organization.ID, string(ownerID), string(readerID), organizations.RoleReader,
	); err != nil {
		fail("add test reader: %v", err)
	}
	repositoryService, err := repositories.NewService(
		repositorystore.New(pool.ORM()), policy, time.Now,
	)
	if err != nil {
		fail("initialize repository service: %v", err)
	}
	for _, input := range []struct {
		name       string
		visibility repositories.Visibility
	}{
		{name: "public-image", visibility: repositories.VisibilityPublic},
		{name: "private-image", visibility: repositories.VisibilityPrivate},
	} {
		if _, err := repositoryService.Create(
			ctx, string(ownerID), organization.NamespaceName, input.name,
			input.visibility, "M2 Registry end-to-end fixture",
		); err != nil {
			fail("create test repository %s: %v", input.name, err)
		}
	}
}

func fail(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
