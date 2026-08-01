package repositorystore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hubcr.io/hubcr/internal/modules/repositories"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (s *Store) Create(ctx context.Context, repository repositories.Repository) error {
	record := toRecord(repository)
	if err := s.database.WithContext(ctx).Create(&record).Error; err != nil {
		return classify("create repository", err)
	}
	return nil
}

func (s *Store) ByID(ctx context.Context, id string) (repositories.Repository, error) {
	var record repositoryRecord
	if err := s.database.WithContext(ctx).Where("id = ?", id).First(&record).Error; err != nil {
		return repositories.Repository{}, classify("find repository", err)
	}
	return fromRecord(record), nil
}

func (s *Store) ByNamespaceAndName(ctx context.Context, namespaceID, name string) (repositories.Repository, error) {
	var record repositoryRecord
	if err := s.database.WithContext(ctx).
		Where("namespace_id = ? AND name = ?", namespaceID, name).
		First(&record).Error; err != nil {
		return repositories.Repository{}, classify("find repository by namespace and name", err)
	}
	return fromRecord(record), nil
}

func (s *Store) ListByNamespace(
	ctx context.Context,
	namespaceID string,
	includePrivate bool,
	limit int,
	after string,
) ([]repositories.Repository, error) {
	query := s.database.WithContext(ctx).Where("namespace_id = ?", namespaceID)
	if !includePrivate {
		query = query.Where("visibility = ?", string(repositories.VisibilityPublic))
	}
	if after != "" {
		query = query.Where("id > ?", after)
	}
	var records []repositoryRecord
	if err := query.Order("id ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, classify("list repositories", err)
	}
	items := make([]repositories.Repository, 0, len(records))
	for _, record := range records {
		items = append(items, fromRecord(record))
	}
	return items, nil
}

func (s *Store) Update(
	ctx context.Context,
	id string,
	update repositories.PersistedUpdate,
) (repositories.Repository, error) {
	if update.ActorUserID == "" || update.At.IsZero() || (update.Description == nil && update.Visibility == nil) {
		return repositories.Repository{}, repositories.ErrInvalidUpdate
	}
	var updated repositoryRecord
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var current repositoryRecord
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&current).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": update.At.UTC()}
		if update.Description != nil {
			updates["description"] = *update.Description
		}
		if update.Visibility != nil {
			if _, err := repositories.ParseVisibility(string(*update.Visibility)); err != nil {
				return err
			}
			updates["visibility"] = string(*update.Visibility)
			updates["visibility_updated_by_user_id"] = update.ActorUserID
			updates["visibility_updated_at"] = update.At.UTC()
		}
		if err := transaction.Model(&repositoryRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return transaction.Where("id = ?", id).First(&updated).Error
	})
	if err != nil {
		return repositories.Repository{}, classify("update repository", err)
	}
	return fromRecord(updated), nil
}

func (s *Store) NamespaceAccessByName(
	ctx context.Context,
	name, actorUserID string,
) (repositories.NamespaceAccess, error) {
	var namespace namespaceRecord
	if err := s.database.WithContext(ctx).Where("name = ?", name).First(&namespace).Error; err != nil {
		return repositories.NamespaceAccess{}, classify("find repository namespace", err)
	}
	switch repositories.NamespaceKind(namespace.OwnerKind) {
	case repositories.NamespacePersonal:
		if namespace.OwnerUserID == nil {
			return repositories.NamespaceAccess{}, repositories.ErrInvalidRepository
		}
		return namespaceAccessFromRecords(namespace, organizationRecord{}, membershipRecord{}, actorUserID), nil
	case repositories.NamespaceOrganization:
		if namespace.OwnerUserID != nil {
			return repositories.NamespaceAccess{}, repositories.ErrInvalidRepository
		}
		var organization organizationRecord
		if err := s.database.WithContext(ctx).Where("namespace_id = ?", namespace.ID).First(&organization).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repositories.NamespaceAccess{}, repositories.ErrInvalidRepository
			}
			return repositories.NamespaceAccess{}, classify("find namespace organization", err)
		}
		var membership membershipRecord
		if actorUserID == "" {
			return namespaceAccessFromRecords(namespace, organization, membership, actorUserID), nil
		}
		err := s.database.WithContext(ctx).
			Where("organization_id = ? AND user_id = ?", organization.ID, actorUserID).
			First(&membership).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return repositories.NamespaceAccess{}, classify("find repository namespace membership", err)
		}
		return namespaceAccessFromRecords(namespace, organization, membership, actorUserID), nil
	default:
		return repositories.NamespaceAccess{}, repositories.ErrInvalidRepository
	}
}

func classify(operation string, err error) error {
	if errors.Is(err, repositories.ErrInvalidName) ||
		errors.Is(err, repositories.ErrInvalidVisibility) ||
		errors.Is(err, repositories.ErrInvalidUpdate) ||
		errors.Is(err, repositories.ErrInvalidRepository) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, repositories.ErrNotFound)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.ConstraintName {
		case "ck_repositories_name":
			return fmt.Errorf("%s: %w", operation, repositories.ErrInvalidName)
		case "ck_repositories_visibility":
			return fmt.Errorf("%s: %w", operation, repositories.ErrInvalidVisibility)
		}
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, repositories.ErrConflict)
		case "23502", "23503", "23514", "22P02":
			return fmt.Errorf("%s: %w", operation, repositories.ErrInvalidRepository)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

var _ repositories.Store = (*Store)(nil)
