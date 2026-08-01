package repositorystore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
	"hubcr.io/hubcr/migrations"
)

func TestRepositoryPersistenceConstraintsAndNamespaceScope(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 6,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 21, 0, 0, 0, time.UTC)
	owner := repositoryIdentity("31313131-3131-4313-8313-313131313131", "repository-owner", now)
	outsider := repositoryIdentity("32323232-3232-4323-8323-323232323232", "repository-outsider", now)
	identityStore := authstore.New(pool.ORM())
	for _, identity := range []auth.Identity{owner, outsider} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}

	organizationService, err := organizations.NewService(
		organizationstore.New(pool.ORM()), namespaces.NormalizeName,
		func() time.Time { return now }, authorization.NewPolicy(),
	)
	if err != nil {
		t.Fatalf("organizations.NewService() error = %v", err)
	}
	organization, err := organizationService.Create(
		ctx, string(owner.User.ID), "Repository-Team", "repository owners",
	)
	if err != nil {
		t.Fatalf("organizationService.Create() error = %v", err)
	}

	store := New(pool.ORM())
	personal := newTestRepository(
		t, string(owner.User.ID), string(owner.User.ID), "Backend.API", repositories.VisibilityPrivate, now,
	)
	organizationOwned := newTestRepository(
		t, organization.NamespaceID, string(owner.User.ID), "BACKEND.API", repositories.VisibilityPublic, now,
	)
	organizationPrivate := newTestRepository(
		t, organization.NamespaceID, string(owner.User.ID), "private-api", repositories.VisibilityPrivate, now,
	)
	for _, repository := range []repositories.Repository{personal, organizationOwned, organizationPrivate} {
		if err := store.Create(ctx, repository); err != nil {
			t.Fatalf("Create(%s) error = %v", repository.ID, err)
		}
	}

	storedPersonal, err := store.ByID(ctx, personal.ID)
	if err != nil || storedPersonal != personal {
		t.Fatalf("ByID() = %#v, %v; want %#v", storedPersonal, err, personal)
	}
	storedOrganization, err := store.ByNamespaceAndName(ctx, organization.NamespaceID, "backend.api")
	if err != nil || storedOrganization != organizationOwned {
		t.Fatalf("ByNamespaceAndName() = %#v, %v; want %#v", storedOrganization, err, organizationOwned)
	}
	if _, err := store.ByNamespaceAndName(ctx, organization.NamespaceID, "missing"); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("missing repository error = %v, want ErrNotFound", err)
	}

	personalOwner, err := store.NamespaceAccessByName(ctx, owner.PersonalNamespace.Name, string(owner.User.ID))
	if err != nil || personalOwner.Kind != repositories.NamespacePersonal || !personalOwner.IsPersonalOwner {
		t.Fatalf("personal owner access = %#v, %v", personalOwner, err)
	}
	personalOutsider, err := store.NamespaceAccessByName(ctx, owner.PersonalNamespace.Name, string(outsider.User.ID))
	if err != nil || personalOutsider.IsPersonalOwner {
		t.Fatalf("personal outsider access = %#v, %v", personalOutsider, err)
	}
	organizationOwner, err := store.NamespaceAccessByName(ctx, organization.NamespaceName, string(owner.User.ID))
	if err != nil || organizationOwner.OrganizationRole != organizations.RoleOwner {
		t.Fatalf("organization owner access = %#v, %v", organizationOwner, err)
	}
	organizationOutsider, err := store.NamespaceAccessByName(ctx, organization.NamespaceName, string(outsider.User.ID))
	if err != nil || organizationOutsider.OrganizationRole != "" {
		t.Fatalf("organization outsider access = %#v, %v", organizationOutsider, err)
	}

	publicOnly, err := store.ListByNamespace(ctx, organization.NamespaceID, false, 10, "")
	if err != nil || len(publicOnly) != 1 || publicOnly[0].ID != organizationOwned.ID {
		t.Fatalf("public ListByNamespace() = %#v, %v", publicOnly, err)
	}
	allRepositories, err := store.ListByNamespace(ctx, organization.NamespaceID, true, 10, "")
	if err != nil || len(allRepositories) != 2 {
		t.Fatalf("private ListByNamespace() count = %d, %v; want 2", len(allRepositories), err)
	}

	newDescription := "updated description"
	descriptionAt := now.Add(time.Minute)
	descriptionUpdated, err := store.Update(ctx, organizationOwned.ID, repositories.PersistedUpdate{
		Description: &newDescription, ActorUserID: string(owner.User.ID), At: descriptionAt,
	})
	if err != nil || descriptionUpdated.Description != newDescription ||
		!descriptionUpdated.VisibilityUpdatedAt.Equal(now) ||
		descriptionUpdated.VisibilityUpdatedByUserID != string(owner.User.ID) {
		t.Fatalf("description Update() = %#v, %v", descriptionUpdated, err)
	}
	privateVisibility := repositories.VisibilityPrivate
	visibilityAt := now.Add(2 * time.Minute)
	visibilityUpdated, err := store.Update(ctx, organizationOwned.ID, repositories.PersistedUpdate{
		Visibility: &privateVisibility, ActorUserID: string(outsider.User.ID), At: visibilityAt,
	})
	if err != nil || visibilityUpdated.Visibility != repositories.VisibilityPrivate ||
		visibilityUpdated.VisibilityUpdatedByUserID != string(outsider.User.ID) ||
		!visibilityUpdated.VisibilityUpdatedAt.Equal(visibilityAt) || !visibilityUpdated.UpdatedAt.Equal(visibilityAt) {
		t.Fatalf("visibility Update() = %#v, %v", visibilityUpdated, err)
	}

	collision := newTestRepository(
		t, string(owner.User.ID), string(owner.User.ID), "backend.api", repositories.VisibilityPublic, now,
	)
	if err := store.Create(ctx, collision); !errors.Is(err, repositories.ErrConflict) {
		t.Fatalf("normalized name collision error = %v, want ErrConflict", err)
	}

	invalidVisibility := newTestRepository(
		t, string(owner.User.ID), string(owner.User.ID), "invalid-visibility", repositories.VisibilityPrivate, now,
	)
	invalidVisibility.Visibility = ""
	if err := store.Create(ctx, invalidVisibility); !errors.Is(err, repositories.ErrInvalidVisibility) {
		t.Fatalf("missing visibility error = %v, want ErrInvalidVisibility", err)
	}

	invalidName := newTestRepository(
		t, string(owner.User.ID), string(owner.User.ID), "valid-first", repositories.VisibilityPrivate, now,
	)
	invalidName.Name = "Bad/Name"
	if err := store.Create(ctx, invalidName); !errors.Is(err, repositories.ErrInvalidName) {
		t.Fatalf("invalid database name error = %v, want ErrInvalidName", err)
	}

	assertVisibilityHasNoDefault(t, ctx, pool)
	assertConcurrentNamespaceNameUniqueness(t, ctx, store, string(owner.User.ID), now)
}

func assertVisibilityHasNoDefault(t *testing.T, ctx context.Context, pool *postgres.Pool) {
	t.Helper()
	type columnMetadata struct {
		IsNullable    string  `gorm:"column:is_nullable"`
		ColumnDefault *string `gorm:"column:column_default"`
	}
	var metadata columnMetadata
	if err := pool.ORM().WithContext(ctx).Raw(
		"SELECT is_nullable, column_default FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'repositories' AND column_name = 'visibility'",
	).Scan(&metadata).Error; err != nil {
		t.Fatalf("inspect repositories.visibility: %v", err)
	}
	if metadata.IsNullable != "NO" || metadata.ColumnDefault != nil {
		t.Fatalf("repositories.visibility metadata = %#v, want NOT NULL without default", metadata)
	}
}

func assertConcurrentNamespaceNameUniqueness(
	t *testing.T,
	ctx context.Context,
	store *Store,
	namespaceID string,
	at time.Time,
) {
	t.Helper()
	first := newTestRepository(t, namespaceID, namespaceID, "concurrent", repositories.VisibilityPrivate, at)
	second := newTestRepository(t, namespaceID, namespaceID, "CONCURRENT", repositories.VisibilityPublic, at)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, repository := range []repositories.Repository{first, second} {
		repository := repository
		go func() {
			<-start
			results <- store.Create(ctx, repository)
		}()
	}
	close(start)
	successes := 0
	conflicts := 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, repositories.ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Create() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results: successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}
}

func newTestRepository(
	t *testing.T,
	namespaceID, actorUserID, name string,
	visibility repositories.Visibility,
	at time.Time,
) repositories.Repository {
	t.Helper()
	repository, err := repositories.New(repositories.NewRepository{
		NamespaceID: namespaceID, RequestedName: name, Visibility: visibility,
		Description: "test repository", CreatedByUserID: actorUserID,
	}, at)
	if err != nil {
		t.Fatalf("repositories.New(%q) error = %v", name, err)
	}
	return repository
}

func repositoryIdentity(id auth.ID, username string, now time.Time) auth.Identity {
	return auth.Identity{
		User: auth.User{ID: id, Username: username, CreatedAt: now, UpdatedAt: now},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: "test-hash", PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: username},
	}
}
