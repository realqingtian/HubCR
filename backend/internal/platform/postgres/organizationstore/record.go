package organizationstore

import (
	"time"

	"hubcr.io/hubcr/internal/modules/organizations"
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
	ID              string
	NamespaceID     string
	Description     string
	CreatedByUserID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (organizationRecord) TableName() string { return "organizations" }

type membershipRecord struct {
	OrganizationID string
	UserID         string
	Role           string
	AddedByUserID  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (membershipRecord) TableName() string { return "organization_memberships" }

func organizationFromRecords(record organizationRecord, namespace namespaceRecord) organizations.Organization {
	return organizations.Organization{
		ID: record.ID, NamespaceID: record.NamespaceID, NamespaceName: namespace.Name,
		Description: record.Description, CreatedByUserID: record.CreatedByUserID,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
}

func membershipToRecord(membership organizations.Membership) membershipRecord {
	return membershipRecord{
		OrganizationID: membership.OrganizationID, UserID: membership.UserID,
		Role: string(membership.Role), AddedByUserID: membership.AddedByUserID,
		CreatedAt: membership.CreatedAt.UTC(), UpdatedAt: membership.UpdatedAt.UTC(),
	}
}

func membershipFromRecord(record membershipRecord) organizations.Membership {
	return organizations.Membership{
		OrganizationID: record.OrganizationID, UserID: record.UserID,
		Role: organizations.Role(record.Role), AddedByUserID: record.AddedByUserID,
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
}
