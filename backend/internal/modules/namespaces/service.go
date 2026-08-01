package namespaces

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound = errors.New("namespace not found")
	ErrConflict = errors.New("namespace conflicts with existing data")
)

type Namespace struct {
	ID          string
	Name        string
	OwnerUserID string
	CreatedAt   time.Time
}

type Store interface {
	CreatePersonal(context.Context, Namespace) error
	ByName(context.Context, string) (Namespace, error)
	ByOwnerUserID(context.Context, string) (Namespace, error)
}

type Service struct {
	store Store
	clock func() time.Time
}

func NewService(store Store, clock func() time.Time) (*Service, error) {
	if store == nil || clock == nil {
		return nil, errors.New("namespace service dependencies must be configured")
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) CreatePersonal(ctx context.Context, ownerUserID, requestedName string) (Namespace, error) {
	name, err := NormalizeName(requestedName)
	if err != nil {
		return Namespace{}, err
	}
	id, err := newID()
	if err != nil {
		return Namespace{}, err
	}
	namespace := Namespace{ID: id, Name: name, OwnerUserID: ownerUserID, CreatedAt: s.clock().UTC()}
	if err := s.store.CreatePersonal(ctx, namespace); err != nil {
		return Namespace{}, fmt.Errorf("create personal namespace: %w", err)
	}
	return namespace, nil
}

func (s *Service) PersonalForUser(ctx context.Context, ownerUserID string) (Namespace, error) {
	return s.store.ByOwnerUserID(ctx, ownerUserID)
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate namespace ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
