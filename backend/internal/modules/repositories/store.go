package repositories

import (
	"context"

	"hubcr.io/hubcr/internal/modules/organizations"
)

type NamespaceKind string

const (
	NamespacePersonal     NamespaceKind = "PERSONAL"
	NamespaceOrganization NamespaceKind = "ORGANIZATION"
)

type NamespaceAccess struct {
	NamespaceID      string
	NamespaceName    string
	Kind             NamespaceKind
	IsPersonalOwner  bool
	OrganizationID   string
	OrganizationRole organizations.Role
}

type Store interface {
	Create(context.Context, Repository) error
	ByID(context.Context, string) (Repository, error)
	ByNamespaceAndName(context.Context, string, string) (Repository, error)
	ListByNamespace(context.Context, string, bool, int, string) ([]Repository, error)
	Update(context.Context, string, PersistedUpdate) (Repository, error)
	NamespaceAccessByName(context.Context, string, string) (NamespaceAccess, error)
}
