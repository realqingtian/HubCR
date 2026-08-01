package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/organizations"
)

const (
	serviceUserID      = "11111111-1111-4111-8111-111111111111"
	serviceNamespaceID = "22222222-2222-4222-8222-222222222222"
)

func TestServiceCreateUsesNamespaceCapabilityMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		access  NamespaceAccess
		allowed bool
	}{
		{name: "personal owner", access: personalAccess(true), allowed: true},
		{name: "personal non-owner", access: personalAccess(false)},
		{name: "organization owner", access: organizationAccess(organizations.RoleOwner), allowed: true},
		{name: "organization admin", access: organizationAccess(organizations.RoleAdmin), allowed: true},
		{name: "organization writer", access: organizationAccess(organizations.RoleWriter), allowed: true},
		{name: "organization reader", access: organizationAccess(organizations.RoleReader)},
		{name: "missing membership", access: organizationAccess("")},
		{name: "missing owner kind", access: NamespaceAccess{NamespaceID: serviceNamespaceID}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &serviceStore{access: test.access}
			service := newTestService(t, store)
			repository, err := service.Create(
				context.Background(), serviceUserID, "Platform-Team", "Backend",
				VisibilityPrivate, "images",
			)
			if test.allowed {
				if err != nil || store.created.ID == "" || repository.Name != "backend" {
					t.Fatalf("Create() = %#v, %v; stored %#v", repository, err, store.created)
				}
				return
			}
			if !errors.Is(err, ErrForbidden) || store.created.ID != "" {
				t.Fatalf("Create() error = %v, stored %#v; want forbidden without write", err, store.created)
			}
		})
	}
}

func TestServiceDiscoveryFiltersPrivateRepositories(t *testing.T) {
	t.Parallel()
	private := serviceRepository(VisibilityPrivate)
	public := serviceRepository(VisibilityPublic)
	tests := []struct {
		name           string
		access         NamespaceAccess
		wantPrivate    bool
		privateDetail  error
		publicDetailOK bool
	}{
		{name: "personal owner", access: personalAccess(true), wantPrivate: true, publicDetailOK: true},
		{name: "personal outsider", access: personalAccess(false), privateDetail: ErrNotFound, publicDetailOK: true},
		{name: "organization reader", access: organizationAccess(organizations.RoleReader), wantPrivate: true, publicDetailOK: true},
		{name: "organization outsider", access: organizationAccess(""), privateDetail: ErrNotFound, publicDetailOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &serviceStore{access: test.access, repositories: []Repository{public, private}, byName: private}
			service := newTestService(t, store)
			if _, err := service.List(context.Background(), serviceUserID, "platform-team", PageRequest{Limit: 20}); err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if store.includePrivate != test.wantPrivate {
				t.Fatalf("List() includePrivate = %v, want %v", store.includePrivate, test.wantPrivate)
			}
			if _, err := service.Detail(context.Background(), serviceUserID, "platform-team", "backend"); !errors.Is(err, test.privateDetail) {
				t.Fatalf("private Detail() error = %v, want %v", err, test.privateDetail)
			}
			store.byName = public
			if _, err := service.Detail(context.Background(), serviceUserID, "platform-team", "backend"); (err == nil) != test.publicDetailOK {
				t.Fatalf("public Detail() error = %v, want success %v", err, test.publicDetailOK)
			}
		})
	}
}

func TestServiceUpdateChecksEachCapabilityAndRecordsVisibilityActor(t *testing.T) {
	t.Parallel()
	newDescription := "new description"
	public := VisibilityPublic
	tests := []struct {
		name    string
		role    organizations.Role
		input   UpdateRepository
		allowed bool
	}{
		{name: "owner changes both", role: organizations.RoleOwner, input: UpdateRepository{Description: &newDescription, Visibility: &public}, allowed: true},
		{name: "admin changes both", role: organizations.RoleAdmin, input: UpdateRepository{Description: &newDescription, Visibility: &public}, allowed: true},
		{name: "writer edits description", role: organizations.RoleWriter, input: UpdateRepository{Description: &newDescription}, allowed: true},
		{name: "writer cannot change visibility", role: organizations.RoleWriter, input: UpdateRepository{Visibility: &public}},
		{name: "writer combined update fails", role: organizations.RoleWriter, input: UpdateRepository{Description: &newDescription, Visibility: &public}},
		{name: "reader cannot edit", role: organizations.RoleReader, input: UpdateRepository{Description: &newDescription}},
		{name: "missing member fails closed", input: UpdateRepository{Description: &newDescription}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &serviceStore{access: organizationAccess(test.role), byName: serviceRepository(VisibilityPrivate)}
			service := newTestService(t, store)
			_, err := service.Update(context.Background(), serviceUserID, "platform-team", "backend", test.input)
			if test.allowed {
				if err != nil || store.lookupCalls != 1 || store.update.ActorUserID != serviceUserID || store.update.At.IsZero() {
					t.Fatalf("Update() error/update = %v, %#v", err, store.update)
				}
				return
			}
			if !errors.Is(err, ErrForbidden) || store.lookupCalls != 0 || store.update.ActorUserID != "" {
				t.Fatalf("Update() error/lookups/update = %v, %d, %#v; want forbidden before lookup or write", err, store.lookupCalls, store.update)
			}
		})
	}
}

func TestServiceRejectsEmptyAndInvalidUpdates(t *testing.T) {
	t.Parallel()
	store := &serviceStore{access: personalAccess(true), byName: serviceRepository(VisibilityPrivate)}
	service := newTestService(t, store)
	if _, err := service.Update(
		context.Background(), serviceUserID, "platform-team", "backend", UpdateRepository{},
	); !errors.Is(err, ErrInvalidUpdate) {
		t.Fatalf("empty Update() error = %v, want ErrInvalidUpdate", err)
	}
	invalid := Visibility("INTERNAL")
	if _, err := service.Update(
		context.Background(), serviceUserID, "platform-team", "backend", UpdateRepository{Visibility: &invalid},
	); !errors.Is(err, ErrInvalidVisibility) {
		t.Fatalf("invalid visibility Update() error = %v, want ErrInvalidVisibility", err)
	}
}

func TestServiceFailsClosedOnMissingRepositoryVisibility(t *testing.T) {
	t.Parallel()
	repository := serviceRepository("")
	store := &serviceStore{
		access: personalAccess(true), byName: repository, repositories: []Repository{repository},
	}
	service := newTestService(t, store)
	if _, err := service.Detail(
		context.Background(), serviceUserID, "platform-team", "backend",
	); !errors.Is(err, ErrInvalidRepository) {
		t.Fatalf("Detail() missing visibility error = %v, want ErrInvalidRepository", err)
	}
	if _, err := service.List(
		context.Background(), serviceUserID, "platform-team", PageRequest{Limit: 20},
	); !errors.Is(err, ErrInvalidRepository) {
		t.Fatalf("List() missing visibility error = %v, want ErrInvalidRepository", err)
	}
}

func newTestService(t *testing.T, store *serviceStore) *Service {
	t.Helper()
	service, err := NewService(store, authorization.NewPolicy(), func() time.Time {
		return time.Date(2026, 8, 1, 22, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func personalAccess(owner bool) NamespaceAccess {
	return NamespaceAccess{
		NamespaceID: serviceNamespaceID, NamespaceName: "platform-team",
		Kind: NamespacePersonal, IsPersonalOwner: owner,
	}
}

func organizationAccess(role organizations.Role) NamespaceAccess {
	return NamespaceAccess{
		NamespaceID: serviceNamespaceID, NamespaceName: "platform-team",
		Kind: NamespaceOrganization, OrganizationID: "33333333-3333-4333-8333-333333333333",
		OrganizationRole: role,
	}
}

func serviceRepository(visibility Visibility) Repository {
	now := time.Date(2026, 8, 1, 21, 0, 0, 0, time.UTC)
	return Repository{
		ID: "44444444-4444-4444-8444-444444444444", NamespaceID: serviceNamespaceID,
		Name: "backend", Visibility: visibility, Description: "images",
		CreatedByUserID: serviceUserID, VisibilityUpdatedByUserID: serviceUserID,
		VisibilityUpdatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

type serviceStore struct {
	access         NamespaceAccess
	created        Repository
	byName         Repository
	repositories   []Repository
	includePrivate bool
	lookupCalls    int
	update         PersistedUpdate
}

func (s *serviceStore) Create(_ context.Context, repository Repository) error {
	s.created = repository
	return nil
}
func (s *serviceStore) ByID(context.Context, string) (Repository, error) {
	return s.byName, nil
}
func (s *serviceStore) ByNamespaceAndName(context.Context, string, string) (Repository, error) {
	s.lookupCalls++
	return s.byName, nil
}
func (s *serviceStore) ListByNamespace(_ context.Context, _ string, includePrivate bool, _ int, _ string) ([]Repository, error) {
	s.includePrivate = includePrivate
	return s.repositories, nil
}
func (s *serviceStore) Update(_ context.Context, _ string, update PersistedUpdate) (Repository, error) {
	s.update = update
	repository := s.byName
	if update.Description != nil {
		repository.Description = *update.Description
	}
	if update.Visibility != nil {
		repository.Visibility = *update.Visibility
		repository.VisibilityUpdatedByUserID = update.ActorUserID
		repository.VisibilityUpdatedAt = update.At
	}
	repository.UpdatedAt = update.At
	return repository, nil
}
func (s *serviceStore) NamespaceAccessByName(context.Context, string, string) (NamespaceAccess, error) {
	return s.access, nil
}
