package authorization

import (
	"testing"

	"hubcr.io/hubcr/internal/modules/organizations"
)

func TestApprovedOrganizationCapabilityMatrix(t *testing.T) {
	policy := NewPolicy()
	roles := []organizations.Role{
		organizations.RoleOwner, organizations.RoleAdmin,
		organizations.RoleWriter, organizations.RoleReader,
	}
	tests := []struct {
		capability Capability
		allowed    map[organizations.Role]bool
	}{
		{ViewOrganization, allow(roles...)},
		{ChangeOrganizationSettings, allow(organizations.RoleOwner)},
		{ManageElevatedMembers, allow(organizations.RoleOwner)},
		{ManageBasicMembers, allow(organizations.RoleOwner, organizations.RoleAdmin)},
		{CreateRepositories, allow(organizations.RoleOwner, organizations.RoleAdmin, organizations.RoleWriter)},
		{ChangeRepositoryVisibility, allow(organizations.RoleOwner, organizations.RoleAdmin)},
		{EditRepositoryDescription, allow(organizations.RoleOwner, organizations.RoleAdmin, organizations.RoleWriter)},
		{PushRepositories, allow(organizations.RoleOwner, organizations.RoleAdmin, organizations.RoleWriter)},
		{PullPrivateRepositories, allow(roles...)},
	}
	for _, test := range tests {
		for _, role := range roles {
			if got := policy.AllowsOrganization(role, test.capability); got != test.allowed[role] {
				t.Fatalf("AllowsOrganization(%s, %s) = %v, want %v", role, test.capability, got, test.allowed[role])
			}
		}
	}
}

func TestUnknownRoleCapabilityAndMissingMembershipDeny(t *testing.T) {
	policy := NewPolicy()
	if policy.AllowsOrganization("", ViewOrganization) ||
		policy.AllowsOrganization(organizations.RoleOwner, "UNKNOWN") ||
		policy.AllowsOrganization("SUPERADMIN", ViewOrganization) {
		t.Fatal("policy allowed missing or unknown authorization data")
	}
}

func TestPersonalNamespaceOwnerCapabilities(t *testing.T) {
	policy := NewPolicy()
	for _, capability := range []Capability{
		CreateRepositories, ChangeRepositoryVisibility, EditRepositoryDescription,
		PushRepositories, PullPrivateRepositories,
	} {
		if !policy.AllowsPersonalNamespace(true, capability) {
			t.Fatalf("personal owner denied %s", capability)
		}
		if policy.AllowsPersonalNamespace(false, capability) {
			t.Fatalf("non-owner allowed %s", capability)
		}
	}
	if policy.AllowsPersonalNamespace(true, ManageBasicMembers) {
		t.Fatal("personal namespace owner allowed an organization-only capability")
	}
}

func allow(roles ...organizations.Role) map[organizations.Role]bool {
	result := make(map[organizations.Role]bool, len(roles))
	for _, role := range roles {
		result[role] = true
	}
	return result
}
