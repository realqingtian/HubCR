package namespacestore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/migrations"
)

func TestPersonalNamespacePersistenceAndUniqueness(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 3,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC)
	users := authstore.New(pool.ORM())
	firstUser := persistenceIdentity("cccccccc-cccc-4ccc-8ccc-cccccccccccc", "NamespaceOwner", now)
	secondUser := persistenceIdentity("dddddddd-dddd-4ddd-8ddd-dddddddddddd", "OtherOwner", now)
	if err := users.CreateIdentity(ctx, firstUser); err != nil {
		t.Fatalf("CreateIdentity(first) error = %v", err)
	}
	if err := users.CreateIdentity(ctx, secondUser); err != nil {
		t.Fatalf("CreateIdentity(second) error = %v", err)
	}

	store := New(pool.ORM())
	created := namespaces.Namespace{
		ID: string(firstUser.PersonalNamespace.ID), Name: firstUser.PersonalNamespace.Name,
		OwnerUserID: string(firstUser.User.ID), CreatedAt: now,
	}
	byName, err := store.ByName(ctx, firstUser.PersonalNamespace.Name)
	if err != nil || byName != created {
		t.Fatalf("ByName() = %#v, %v", byName, err)
	}
	byOwner, err := store.ByOwnerUserID(ctx, string(firstUser.User.ID))
	if err != nil || byOwner != created {
		t.Fatalf("ByOwnerUserID() = %#v, %v", byOwner, err)
	}

	service, err := namespaces.NewService(store, func() time.Time { return now })
	if err != nil {
		t.Fatalf("namespaces.NewService() error = %v", err)
	}
	if _, err := service.CreatePersonal(ctx, string(firstUser.User.ID), "another-name"); !errors.Is(err, namespaces.ErrConflict) {
		t.Fatalf("duplicate owner error = %v, want ErrConflict", err)
	}
	if _, err := service.CreatePersonal(ctx, string(secondUser.User.ID), firstUser.PersonalNamespace.Name); !errors.Is(err, namespaces.ErrConflict) {
		t.Fatalf("duplicate normalized name error = %v, want ErrConflict", err)
	}
}

func persistenceIdentity(id auth.ID, username string, now time.Time) auth.Identity {
	namespaceName, err := namespaces.NormalizeName(username)
	if err != nil {
		panic(err)
	}
	return auth.Identity{
		User: auth.User{ID: id, Username: username, CreatedAt: now, UpdatedAt: now},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: "test-hash", PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: namespaceName},
	}
}
