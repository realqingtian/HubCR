package namespacestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"hubcr.io/hubcr/internal/modules/namespaces"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (s *Store) CreatePersonal(ctx context.Context, namespace namespaces.Namespace) error {
	record := toRecord(namespace)
	if err := s.database.WithContext(ctx).Create(&record).Error; err != nil {
		return classify("create personal namespace", err)
	}
	return nil
}

func (s *Store) ByName(ctx context.Context, name string) (namespaces.Namespace, error) {
	var record namespaceRecord
	if err := s.database.WithContext(ctx).Where("name = ?", name).First(&record).Error; err != nil {
		return namespaces.Namespace{}, classify("find namespace by name", err)
	}
	return fromRecord(record), nil
}

func (s *Store) ByOwnerUserID(ctx context.Context, ownerUserID string) (namespaces.Namespace, error) {
	var record namespaceRecord
	if err := s.database.WithContext(ctx).Where("owner_user_id = ?", ownerUserID).First(&record).Error; err != nil {
		return namespaces.Namespace{}, classify("find personal namespace", err)
	}
	return fromRecord(record), nil
}

type namespaceRecord struct {
	ID          string
	Name        string
	OwnerUserID string
	CreatedAt   time.Time
}

func (namespaceRecord) TableName() string { return "namespaces" }

func toRecord(namespace namespaces.Namespace) namespaceRecord {
	return namespaceRecord{
		ID: namespace.ID, Name: namespace.Name, OwnerUserID: namespace.OwnerUserID, CreatedAt: namespace.CreatedAt.UTC(),
	}
}

func fromRecord(record namespaceRecord) namespaces.Namespace {
	return namespaces.Namespace{
		ID: record.ID, Name: record.Name, OwnerUserID: record.OwnerUserID, CreatedAt: record.CreatedAt.UTC(),
	}
}

func classify(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, namespaces.ErrNotFound)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, namespaces.ErrConflict)
		case "23502", "23503", "23514", "22P02":
			return fmt.Errorf("%s: %w", operation, namespaces.ErrInvalidName)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

var _ namespaces.Store = (*Store)(nil)
