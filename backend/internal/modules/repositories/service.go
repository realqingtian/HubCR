package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
)

type Policy interface {
	AllowsOrganization(organizations.Role, authorization.Capability) bool
	AllowsPersonalNamespace(bool, authorization.Capability) bool
	AllowsRepositoryDiscovery(bool, bool) bool
}

type Service struct {
	store  Store
	policy Policy
	clock  func() time.Time
}

func NewService(store Store, policy Policy, clock func() time.Time) (*Service, error) {
	if store == nil || policy == nil || clock == nil {
		return nil, errors.New("repository service dependencies must be configured")
	}
	return &Service{store: store, policy: policy, clock: clock}, nil
}

func (s *Service) Create(
	ctx context.Context,
	actorUserID, namespaceName, requestedName string,
	visibility Visibility,
	description string,
) (Repository, error) {
	access, err := s.access(ctx, namespaceName, actorUserID)
	if err != nil {
		return Repository{}, err
	}
	if !s.allows(access, authorization.CreateRepositories) {
		return Repository{}, ErrForbidden
	}
	if len(description) > MaxDescriptionLength {
		return Repository{}, ErrInvalidRepository
	}
	repository, err := New(NewRepository{
		NamespaceID: access.NamespaceID, RequestedName: requestedName,
		Visibility: visibility, Description: description, CreatedByUserID: actorUserID,
	}, s.clock())
	if err != nil {
		return Repository{}, err
	}
	if err := s.store.Create(ctx, repository); err != nil {
		return Repository{}, fmt.Errorf("create repository: %w", err)
	}
	return repository, nil
}

func (s *Service) List(
	ctx context.Context,
	actorUserID, namespaceName string,
	page PageRequest,
) (RepositoryPage, error) {
	access, err := s.access(ctx, namespaceName, actorUserID)
	if err != nil {
		return RepositoryPage{}, err
	}
	canPullPrivate := s.allows(access, authorization.PullPrivateRepositories)
	includePrivate := s.policy.AllowsRepositoryDiscovery(false, canPullPrivate)
	items, err := s.store.ListByNamespace(ctx, access.NamespaceID, includePrivate, page.Limit+1, page.After)
	if err != nil {
		return RepositoryPage{}, fmt.Errorf("list repositories: %w", err)
	}
	for _, repository := range items {
		if repository.NamespaceID != access.NamespaceID {
			return RepositoryPage{}, ErrInvalidRepository
		}
		if _, err := ParseVisibility(string(repository.Visibility)); err != nil {
			return RepositoryPage{}, ErrInvalidRepository
		}
	}
	result := RepositoryPage{Items: items}
	if len(items) > page.Limit {
		result.Items = items[:page.Limit]
		result.NextAfter = result.Items[len(result.Items)-1].ID
	}
	return result, nil
}

func (s *Service) Detail(
	ctx context.Context,
	actorUserID, namespaceName, repositoryName string,
) (Repository, error) {
	access, err := s.access(ctx, namespaceName, actorUserID)
	if err != nil {
		return Repository{}, err
	}
	name, err := NormalizeName(repositoryName)
	if err != nil {
		return Repository{}, err
	}
	repository, err := s.store.ByNamespaceAndName(ctx, access.NamespaceID, name)
	if err != nil {
		return Repository{}, err
	}
	visibility, err := ParseVisibility(string(repository.Visibility))
	if err != nil || repository.NamespaceID != access.NamespaceID {
		return Repository{}, ErrInvalidRepository
	}
	canPullPrivate := s.allows(access, authorization.PullPrivateRepositories)
	if !s.policy.AllowsRepositoryDiscovery(visibility == VisibilityPublic, canPullPrivate) {
		return Repository{}, ErrNotFound
	}
	return repository, nil
}

func (s *Service) Update(
	ctx context.Context,
	actorUserID, namespaceName, repositoryName string,
	input UpdateRepository,
) (Repository, error) {
	if input.Description == nil && input.Visibility == nil {
		return Repository{}, ErrInvalidUpdate
	}
	if input.Description != nil && len(*input.Description) > MaxDescriptionLength {
		return Repository{}, ErrInvalidUpdate
	}
	var requestedVisibility *Visibility
	if input.Visibility != nil {
		visibility, err := ParseVisibility(string(*input.Visibility))
		if err != nil {
			return Repository{}, err
		}
		requestedVisibility = &visibility
	}
	access, err := s.access(ctx, namespaceName, actorUserID)
	if err != nil {
		return Repository{}, err
	}
	if input.Description != nil && !s.allows(access, authorization.EditRepositoryDescription) {
		return Repository{}, ErrForbidden
	}
	if requestedVisibility != nil && !s.allows(access, authorization.ChangeRepositoryVisibility) {
		return Repository{}, ErrForbidden
	}
	name, err := NormalizeName(repositoryName)
	if err != nil {
		return Repository{}, err
	}
	repository, err := s.store.ByNamespaceAndName(ctx, access.NamespaceID, name)
	if err != nil {
		return Repository{}, err
	}
	if _, err := ParseVisibility(string(repository.Visibility)); err != nil || repository.NamespaceID != access.NamespaceID {
		return Repository{}, ErrInvalidRepository
	}

	update := PersistedUpdate{ActorUserID: actorUserID, At: s.clock().UTC()}
	if input.Description != nil {
		if *input.Description != repository.Description {
			update.Description = input.Description
		}
	}
	if requestedVisibility != nil {
		if *requestedVisibility != repository.Visibility {
			update.Visibility = requestedVisibility
		}
	}
	if update.Description == nil && update.Visibility == nil {
		return repository, nil
	}
	updated, err := s.store.Update(ctx, repository.ID, update)
	if err != nil {
		return Repository{}, fmt.Errorf("update repository: %w", err)
	}
	return updated, nil
}

func (s *Service) access(ctx context.Context, namespaceName, actorUserID string) (NamespaceAccess, error) {
	name, err := namespaces.NormalizeName(namespaceName)
	if err != nil {
		return NamespaceAccess{}, err
	}
	access, err := s.store.NamespaceAccessByName(ctx, name, actorUserID)
	if err != nil {
		return NamespaceAccess{}, err
	}
	return access, nil
}

func (s *Service) allows(access NamespaceAccess, capability authorization.Capability) bool {
	switch access.Kind {
	case NamespacePersonal:
		return s.policy.AllowsPersonalNamespace(access.IsPersonalOwner, capability)
	case NamespaceOrganization:
		return s.policy.AllowsOrganization(access.OrganizationRole, capability)
	default:
		return false
	}
}
