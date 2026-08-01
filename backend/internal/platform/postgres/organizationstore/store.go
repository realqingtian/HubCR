package organizationstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hubcr.io/hubcr/internal/modules/organizations"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (s *Store) CreateWithOwner(ctx context.Context, input organizations.NewOrganization) error {
	organization := input.Organization
	owner := input.Owner
	if owner.Role != organizations.RoleOwner || owner.OrganizationID != organization.ID ||
		owner.UserID != organization.CreatedByUserID || organization.NamespaceID == "" ||
		organization.NamespaceName == "" {
		return organizations.ErrInvalidRole
	}

	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		namespace := namespaceRecord{
			ID: organization.NamespaceID, Name: organization.NamespaceName,
			OwnerKind: "ORGANIZATION", CreatedAt: organization.CreatedAt.UTC(),
		}
		if err := transaction.Create(&namespace).Error; err != nil {
			return err
		}
		record := organizationRecord{
			ID: organization.ID, NamespaceID: organization.NamespaceID,
			Description: organization.Description, CreatedByUserID: organization.CreatedByUserID,
			CreatedAt: organization.CreatedAt.UTC(), UpdatedAt: organization.UpdatedAt.UTC(),
		}
		if err := transaction.Create(&record).Error; err != nil {
			return err
		}
		membership := membershipToRecord(owner)
		return transaction.Create(&membership).Error
	})
	if err != nil {
		return classify("create organization with owner", err)
	}
	return nil
}

func (s *Store) ByID(ctx context.Context, id string) (organizations.Organization, error) {
	var record organizationRecord
	if err := s.database.WithContext(ctx).Where("id = ?", id).First(&record).Error; err != nil {
		return organizations.Organization{}, classify("find organization", err)
	}
	var namespace namespaceRecord
	if err := s.database.WithContext(ctx).Where("id = ?", record.NamespaceID).First(&namespace).Error; err != nil {
		return organizations.Organization{}, classify("find organization namespace", err)
	}
	return organizationFromRecords(record, namespace), nil
}

func (s *Store) ListForUser(ctx context.Context, userID string, limit int, after string) ([]organizations.Organization, error) {
	query := s.database.WithContext(ctx).
		Model(&organizationRecord{}).
		Joins("JOIN organization_memberships ON organization_memberships.organization_id = organizations.id").
		Where("organization_memberships.user_id = ?", userID)
	if after != "" {
		query = query.Where("organizations.id > ?", after)
	}
	var records []organizationRecord
	if err := query.Order("organizations.id ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, classify("list user organizations", err)
	}
	result := make([]organizations.Organization, 0, len(records))
	for _, record := range records {
		var namespace namespaceRecord
		if err := s.database.WithContext(ctx).Where("id = ?", record.NamespaceID).First(&namespace).Error; err != nil {
			return nil, classify("find listed organization namespace", err)
		}
		result = append(result, organizationFromRecords(record, namespace))
	}
	return result, nil
}

func (s *Store) Membership(ctx context.Context, organizationID, userID string) (organizations.Membership, error) {
	var record membershipRecord
	if err := s.database.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		First(&record).Error; err != nil {
		return organizations.Membership{}, classify("find organization membership", err)
	}
	return membershipFromRecord(record), nil
}

func (s *Store) ListMembers(ctx context.Context, organizationID string, limit int, after string) ([]organizations.Membership, error) {
	query := s.database.WithContext(ctx).Where("organization_id = ?", organizationID)
	if after != "" {
		query = query.Where("user_id > ?", after)
	}
	var records []membershipRecord
	if err := query.Order("user_id ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, classify("list organization memberships", err)
	}
	members := make([]organizations.Membership, 0, len(records))
	for _, record := range records {
		members = append(members, membershipFromRecord(record))
	}
	return members, nil
}

func (s *Store) AddMember(ctx context.Context, membership organizations.Membership) error {
	if _, err := organizations.ParseRole(string(membership.Role)); err != nil {
		return err
	}
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := lockOrganization(transaction, membership.OrganizationID); err != nil {
			return err
		}
		record := membershipToRecord(membership)
		return transaction.Create(&record).Error
	})
	if err != nil {
		return classify("add organization member", err)
	}
	return nil
}

func (s *Store) ChangeMemberRole(
	ctx context.Context,
	organizationID, userID string,
	role organizations.Role,
	at time.Time,
) error {
	if _, err := organizations.ParseRole(string(role)); err != nil {
		return err
	}
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := lockOrganization(transaction, organizationID); err != nil {
			return err
		}
		var membership membershipRecord
		if err := transaction.Where(
			"organization_id = ? AND user_id = ?", organizationID, userID,
		).First(&membership).Error; err != nil {
			return err
		}
		if membership.Role == string(organizations.RoleOwner) && role != organizations.RoleOwner {
			if err := requireAnotherOwner(transaction, organizationID); err != nil {
				return err
			}
		}
		return transaction.Model(&membershipRecord{}).
			Where("organization_id = ? AND user_id = ?", organizationID, userID).
			Updates(map[string]any{"role": string(role), "updated_at": at.UTC()}).Error
	})
	if err != nil {
		return classify("change organization member role", err)
	}
	return nil
}

func (s *Store) RemoveMember(ctx context.Context, organizationID, userID string) error {
	err := s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := lockOrganization(transaction, organizationID); err != nil {
			return err
		}
		var membership membershipRecord
		if err := transaction.Where(
			"organization_id = ? AND user_id = ?", organizationID, userID,
		).First(&membership).Error; err != nil {
			return err
		}
		if membership.Role == string(organizations.RoleOwner) {
			if err := requireAnotherOwner(transaction, organizationID); err != nil {
				return err
			}
		}
		return transaction.Where(
			"organization_id = ? AND user_id = ?", organizationID, userID,
		).Delete(&membershipRecord{}).Error
	})
	if err != nil {
		return classify("remove organization member", err)
	}
	return nil
}

func lockOrganization(database *gorm.DB, organizationID string) error {
	var organization organizationRecord
	return database.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").Where("id = ?", organizationID).First(&organization).Error
}

func requireAnotherOwner(database *gorm.DB, organizationID string) error {
	var owners int64
	if err := database.Model(&membershipRecord{}).
		Where("organization_id = ? AND role = ?", organizationID, string(organizations.RoleOwner)).
		Count(&owners).Error; err != nil {
		return err
	}
	if owners <= 1 {
		return organizations.ErrLastOwner
	}
	return nil
}

func classify(operation string, err error) error {
	if errors.Is(err, organizations.ErrLastOwner) || errors.Is(err, organizations.ErrInvalidRole) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, organizations.ErrNotFound)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, organizations.ErrConflict)
		case "23503", "22P02":
			return fmt.Errorf("%s: %w", operation, organizations.ErrInvalidMember)
		case "23502", "23514":
			return fmt.Errorf("%s: %w", operation, organizations.ErrInvalidRole)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

var _ organizations.Store = (*Store)(nil)
