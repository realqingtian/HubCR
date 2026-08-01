package organizations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseRole(t *testing.T) {
	for _, role := range []Role{RoleOwner, RoleAdmin, RoleWriter, RoleReader} {
		if parsed, err := ParseRole(string(role)); err != nil || parsed != role {
			t.Fatalf("ParseRole(%q) = %q, %v", role, parsed, err)
		}
	}
	if _, err := ParseRole("MEMBER"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("ParseRole(MEMBER) error = %v, want ErrInvalidRole", err)
	}
}

func TestCreateStartsWithOwnerAndNormalizedNamespace(t *testing.T) {
	now := time.Date(2026, 8, 1, 17, 0, 0, 0, time.UTC)
	store := &organizationTestStore{}
	service, err := NewService(store, func(value string) (string, error) {
		return strings.ToLower(value), nil
	}, func() time.Time { return now }, testMembershipPolicy{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	organization, err := service.Create(context.Background(), "owner-id", "My-Team", "description")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if organization.NamespaceName != "my-team" || organization.ID == "" || organization.NamespaceID == "" {
		t.Fatalf("Create() = %#v", organization)
	}
	if store.created.Owner.Role != RoleOwner || store.created.Owner.UserID != "owner-id" ||
		store.created.Owner.OrganizationID != organization.ID {
		t.Fatalf("initial owner = %#v", store.created.Owner)
	}
}

func TestMemberManagementPolicy(t *testing.T) {
	now := time.Date(2026, 8, 1, 17, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		actorRole Role
		target    *Role
		desired   Role
		operation string
		wantError error
	}{
		{name: "owner adds admin", actorRole: RoleOwner, desired: RoleAdmin, operation: "add"},
		{name: "admin adds writer", actorRole: RoleAdmin, desired: RoleWriter, operation: "add"},
		{name: "admin cannot add admin", actorRole: RoleAdmin, desired: RoleAdmin, operation: "add", wantError: ErrForbidden},
		{name: "writer cannot add reader", actorRole: RoleWriter, desired: RoleReader, operation: "add", wantError: ErrForbidden},
		{name: "admin changes writer to reader", actorRole: RoleAdmin, target: rolePointer(RoleWriter), desired: RoleReader, operation: "change"},
		{name: "admin cannot change admin", actorRole: RoleAdmin, target: rolePointer(RoleAdmin), desired: RoleReader, operation: "change", wantError: ErrForbidden},
		{name: "admin removes reader", actorRole: RoleAdmin, target: rolePointer(RoleReader), operation: "remove"},
		{name: "admin cannot remove owner", actorRole: RoleAdmin, target: rolePointer(RoleOwner), operation: "remove", wantError: ErrForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &policyTestStore{members: map[string]Membership{
				"actor": {OrganizationID: "org", UserID: "actor", Role: test.actorRole},
			}}
			if test.target != nil {
				store.members["target"] = Membership{OrganizationID: "org", UserID: "target", Role: *test.target}
			}
			service, err := NewService(store, func(value string) (string, error) {
				return strings.ToLower(value), nil
			}, func() time.Time { return now }, testMembershipPolicy{})
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}
			switch test.operation {
			case "add":
				err = service.AddMember(context.Background(), "org", "actor", "target", test.desired)
			case "change":
				err = service.ChangeMemberRole(context.Background(), "org", "actor", "target", test.desired)
			case "remove":
				err = service.RemoveMember(context.Background(), "org", "actor", "target")
			}
			if !errors.Is(err, test.wantError) || (test.wantError == nil && err != nil) {
				t.Fatalf("operation error = %v, want %v", err, test.wantError)
			}
		})
	}
}

func rolePointer(role Role) *Role { return &role }

type policyTestStore struct{ members map[string]Membership }

type testMembershipPolicy struct{}

func (testMembershipPolicy) CanAssignMember(actor, desired Role) bool {
	return actor == RoleOwner || actor == RoleAdmin && (desired == RoleWriter || desired == RoleReader)
}
func (testMembershipPolicy) CanChangeMember(actor, current, desired Role) bool {
	return actor == RoleOwner || actor == RoleAdmin &&
		(current == RoleWriter || current == RoleReader) && (desired == RoleWriter || desired == RoleReader)
}
func (testMembershipPolicy) CanRemoveMember(actor, target Role) bool {
	return actor == RoleOwner || actor == RoleAdmin && (target == RoleWriter || target == RoleReader)
}

func (*policyTestStore) CreateWithOwner(context.Context, NewOrganization) error { return nil }
func (*policyTestStore) ByID(context.Context, string) (Organization, error) {
	return Organization{}, nil
}
func (*policyTestStore) ListForUser(context.Context, string, int, string) ([]Organization, error) {
	return nil, nil
}
func (s *policyTestStore) Membership(_ context.Context, _, userID string) (Membership, error) {
	membership, exists := s.members[userID]
	if !exists {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}
func (*policyTestStore) ListMembers(context.Context, string, int, string) ([]Membership, error) {
	return nil, nil
}
func (*policyTestStore) AddMember(context.Context, Membership) error { return nil }
func (*policyTestStore) ChangeMemberRole(context.Context, string, string, Role, time.Time) error {
	return nil
}
func (*policyTestStore) RemoveMember(context.Context, string, string) error { return nil }

type organizationTestStore struct{ created NewOrganization }

func (s *organizationTestStore) CreateWithOwner(_ context.Context, value NewOrganization) error {
	s.created = value
	return nil
}
func (*organizationTestStore) ByID(context.Context, string) (Organization, error) {
	return Organization{}, ErrNotFound
}
func (*organizationTestStore) ListForUser(context.Context, string, int, string) ([]Organization, error) {
	return nil, nil
}
func (*organizationTestStore) Membership(context.Context, string, string) (Membership, error) {
	return Membership{}, ErrNotFound
}
func (*organizationTestStore) ListMembers(context.Context, string, int, string) ([]Membership, error) {
	return nil, nil
}
func (*organizationTestStore) AddMember(context.Context, Membership) error { return nil }
func (*organizationTestStore) ChangeMemberRole(context.Context, string, string, Role, time.Time) error {
	return nil
}
func (*organizationTestStore) RemoveMember(context.Context, string, string) error { return nil }
