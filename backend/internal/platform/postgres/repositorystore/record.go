package repositorystore

import (
	"time"

	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
)

type namespaceRecord struct {
	ID          string
	Name        string
	OwnerKind   string
	OwnerUserID *string
	CreatedAt   time.Time
}

func (namespaceRecord) TableName() string { return "namespaces" }

type organizationRecord struct {
	ID          string
	NamespaceID string
}

func (organizationRecord) TableName() string { return "organizations" }

type membershipRecord struct {
	OrganizationID string
	UserID         string
	Role           string
}

func (membershipRecord) TableName() string { return "organization_memberships" }

type repositoryRecord struct {
	ID                        string
	NamespaceID               string
	Name                      string
	Visibility                string
	Description               string
	CreatedByUserID           string
	VisibilityUpdatedByUserID string
	VisibilityUpdatedAt       time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

func (repositoryRecord) TableName() string { return "repositories" }

func toRecord(repository repositories.Repository) repositoryRecord {
	return repositoryRecord{
		ID: repository.ID, NamespaceID: repository.NamespaceID, Name: repository.Name,
		Visibility: string(repository.Visibility), Description: repository.Description,
		CreatedByUserID:           repository.CreatedByUserID,
		VisibilityUpdatedByUserID: repository.VisibilityUpdatedByUserID,
		VisibilityUpdatedAt:       repository.VisibilityUpdatedAt.UTC(),
		CreatedAt:                 repository.CreatedAt.UTC(), UpdatedAt: repository.UpdatedAt.UTC(),
	}
}

func fromRecord(record repositoryRecord) repositories.Repository {
	return repositories.Repository{
		ID: record.ID, NamespaceID: record.NamespaceID, Name: record.Name,
		Visibility: repositories.Visibility(record.Visibility), Description: record.Description,
		CreatedByUserID:           record.CreatedByUserID,
		VisibilityUpdatedByUserID: record.VisibilityUpdatedByUserID,
		VisibilityUpdatedAt:       record.VisibilityUpdatedAt.UTC(),
		CreatedAt:                 record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
}

func namespaceAccessFromRecords(
	namespace namespaceRecord,
	organization organizationRecord,
	membership membershipRecord,
	actorUserID string,
) repositories.NamespaceAccess {
	access := repositories.NamespaceAccess{
		NamespaceID: namespace.ID, NamespaceName: namespace.Name,
		Kind: repositories.NamespaceKind(namespace.OwnerKind),
	}
	if access.Kind == repositories.NamespacePersonal && namespace.OwnerUserID != nil {
		access.IsPersonalOwner = *namespace.OwnerUserID == actorUserID
	}
	if access.Kind == repositories.NamespaceOrganization {
		access.OrganizationID = organization.ID
		access.OrganizationRole = organizations.Role(membership.Role)
	}
	return access
}
