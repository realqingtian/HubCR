package organizationstore

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
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/migrations"
)

func TestOrganizationMembershipLifecycleAndLastOwnerInvariant(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 4,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)
	identityStore := authstore.New(pool.ORM())
	owner := organizationIdentity("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", "org-owner", now)
	secondOwner := organizationIdentity("ffffffff-ffff-4fff-8fff-ffffffffffff", "second-owner", now)
	reader := organizationIdentity("12121212-1212-4212-8212-121212121212", "org-reader", now)
	for _, identity := range []auth.Identity{owner, secondOwner, reader} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}

	store := New(pool.ORM())
	service, err := organizations.NewService(
		store, namespaces.NormalizeName, func() time.Time { return now }, authorization.NewPolicy(),
	)
	if err != nil {
		t.Fatalf("organizations.NewService() error = %v", err)
	}
	organization, err := service.Create(ctx, string(owner.User.ID), "Platform-Team", "platform images")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if organization.NamespaceName != "platform-team" {
		t.Fatalf("namespace name = %q, want platform-team", organization.NamespaceName)
	}
	stored, err := store.ByID(ctx, organization.ID)
	if err != nil || stored != organization {
		t.Fatalf("ByID() = %#v, %v; want %#v", stored, err, organization)
	}
	initialOwner, err := store.Membership(ctx, organization.ID, string(owner.User.ID))
	if err != nil || initialOwner.Role != organizations.RoleOwner {
		t.Fatalf("initial owner = %#v, %v", initialOwner, err)
	}

	if err := service.AddMember(
		ctx, organization.ID, string(owner.User.ID), string(reader.User.ID), organizations.RoleReader,
	); err != nil {
		t.Fatalf("AddMember(reader) error = %v", err)
	}
	if err := service.ChangeMemberRole(
		ctx, organization.ID, string(owner.User.ID), string(owner.User.ID), organizations.RoleAdmin,
	); !errors.Is(err, organizations.ErrLastOwner) {
		t.Fatalf("demote last owner error = %v, want ErrLastOwner", err)
	}
	if err := service.RemoveMember(ctx, organization.ID, string(owner.User.ID), string(owner.User.ID)); !errors.Is(err, organizations.ErrLastOwner) {
		t.Fatalf("remove last owner error = %v, want ErrLastOwner", err)
	}

	if err := service.AddMember(
		ctx, organization.ID, string(owner.User.ID), string(secondOwner.User.ID), organizations.RoleOwner,
	); err != nil {
		t.Fatalf("AddMember(second owner) error = %v", err)
	}
	if err := service.ChangeMemberRole(
		ctx, organization.ID, string(secondOwner.User.ID), string(owner.User.ID), organizations.RoleAdmin,
	); err != nil {
		t.Fatalf("demote owner with replacement error = %v", err)
	}
	if err := service.RemoveMember(ctx, organization.ID, string(secondOwner.User.ID), string(secondOwner.User.ID)); !errors.Is(err, organizations.ErrLastOwner) {
		t.Fatalf("remove replacement last owner error = %v, want ErrLastOwner", err)
	}
	if err := service.ChangeMemberRole(
		ctx, organization.ID, string(secondOwner.User.ID), string(owner.User.ID), organizations.RoleOwner,
	); err != nil {
		t.Fatalf("promote original owner error = %v", err)
	}
	if err := service.RemoveMember(ctx, organization.ID, string(owner.User.ID), string(secondOwner.User.ID)); err != nil {
		t.Fatalf("remove non-last owner error = %v", err)
	}

	members, err := store.ListMembers(ctx, organization.ID, 100, "")
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("member count = %d, want 2", len(members))
	}
	if _, err := service.Create(ctx, string(owner.User.ID), "org-owner", "collision"); !errors.Is(err, organizations.ErrConflict) {
		t.Fatalf("Create(personal namespace collision) error = %v, want ErrConflict", err)
	}
}

func organizationIdentity(id auth.ID, username string, now time.Time) auth.Identity {
	return auth.Identity{
		User: auth.User{ID: id, Username: username, CreatedAt: now, UpdatedAt: now},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: "test-hash", PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: username},
	}
}
