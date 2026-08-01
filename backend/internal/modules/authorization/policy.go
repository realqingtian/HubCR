package authorization

import "hubcr.io/hubcr/internal/modules/organizations"

type Capability string

const (
	ViewOrganization           Capability = "VIEW_ORGANIZATION"
	ChangeOrganizationSettings Capability = "CHANGE_ORGANIZATION_SETTINGS"
	ManageElevatedMembers      Capability = "MANAGE_ELEVATED_MEMBERS"
	ManageBasicMembers         Capability = "MANAGE_BASIC_MEMBERS"
	CreateRepositories         Capability = "CREATE_REPOSITORIES"
	ChangeRepositoryVisibility Capability = "CHANGE_REPOSITORY_VISIBILITY"
	EditRepositoryDescription  Capability = "EDIT_REPOSITORY_DESCRIPTION"
	PushRepositories           Capability = "PUSH_REPOSITORIES"
	PullPrivateRepositories    Capability = "PULL_PRIVATE_REPOSITORIES"
)

type Policy struct{}

func NewPolicy() Policy { return Policy{} }

func (Policy) AllowsOrganization(role organizations.Role, capability Capability) bool {
	allowed, exists := organizationCapabilities[capability]
	return exists && allowed[role]
}

func (Policy) AllowsPersonalNamespace(isOwner bool, capability Capability) bool {
	if !isOwner {
		return false
	}
	switch capability {
	case CreateRepositories, ChangeRepositoryVisibility, EditRepositoryDescription,
		PushRepositories, PullPrivateRepositories:
		return true
	default:
		return false
	}
}

// AllowsRepositoryDiscovery keeps public/private discovery inside the centralized
// policy boundary. Missing visibility must be represented as neither public nor a
// valid private-pull capability, which denies discovery.
func (Policy) AllowsRepositoryDiscovery(isPublic, canPullPrivate bool) bool {
	return isPublic || canPullPrivate
}

// AllowsPublicRepositoryPull implements D-006. The caller must pass true only
// after repository visibility has been validated as explicitly PUBLIC.
func (Policy) AllowsPublicRepositoryPull(isExplicitlyPublic bool) bool {
	return isExplicitlyPublic
}

func (p Policy) CanAssignMember(actor, desired organizations.Role) bool {
	if desired == organizations.RoleOwner || desired == organizations.RoleAdmin {
		return p.AllowsOrganization(actor, ManageElevatedMembers)
	}
	return p.AllowsOrganization(actor, ManageBasicMembers)
}

func (p Policy) CanChangeMember(actor, current, desired organizations.Role) bool {
	if current == organizations.RoleOwner || current == organizations.RoleAdmin ||
		desired == organizations.RoleOwner || desired == organizations.RoleAdmin {
		return p.AllowsOrganization(actor, ManageElevatedMembers)
	}
	return p.AllowsOrganization(actor, ManageBasicMembers)
}

func (p Policy) CanRemoveMember(actor, target organizations.Role) bool {
	if target == organizations.RoleOwner || target == organizations.RoleAdmin {
		return p.AllowsOrganization(actor, ManageElevatedMembers)
	}
	return p.AllowsOrganization(actor, ManageBasicMembers)
}

var organizationCapabilities = map[Capability]map[organizations.Role]bool{
	ViewOrganization: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true,
		organizations.RoleWriter: true, organizations.RoleReader: true,
	},
	ChangeOrganizationSettings: {organizations.RoleOwner: true},
	ManageElevatedMembers:      {organizations.RoleOwner: true},
	ManageBasicMembers: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true,
	},
	CreateRepositories: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true, organizations.RoleWriter: true,
	},
	ChangeRepositoryVisibility: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true,
	},
	EditRepositoryDescription: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true, organizations.RoleWriter: true,
	},
	PushRepositories: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true, organizations.RoleWriter: true,
	},
	PullPrivateRepositories: {
		organizations.RoleOwner: true, organizations.RoleAdmin: true,
		organizations.RoleWriter: true, organizations.RoleReader: true,
	},
}
