package organizations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type NormalizeName func(string) (string, error)

type MembershipPolicy interface {
	CanAssignMember(Role, Role) bool
	CanChangeMember(Role, Role, Role) bool
	CanRemoveMember(Role, Role) bool
}

type Service struct {
	store         Store
	normalizeName NormalizeName
	clock         func() time.Time
	policy        MembershipPolicy
}

func NewService(store Store, normalizeName NormalizeName, clock func() time.Time, policy MembershipPolicy) (*Service, error) {
	if store == nil || normalizeName == nil || clock == nil || policy == nil {
		return nil, errors.New("organization service dependencies must be configured")
	}
	return &Service{store: store, normalizeName: normalizeName, clock: clock, policy: policy}, nil
}

func (s *Service) Create(ctx context.Context, ownerUserID, requestedName, description string) (Organization, error) {
	name, err := s.normalizeName(requestedName)
	if err != nil {
		return Organization{}, err
	}
	organizationID, err := newID()
	if err != nil {
		return Organization{}, err
	}
	namespaceID, err := newID()
	if err != nil {
		return Organization{}, err
	}
	now := s.clock().UTC()
	organization := Organization{
		ID: organizationID, NamespaceID: namespaceID, NamespaceName: name,
		Description: description, CreatedByUserID: ownerUserID, CreatedAt: now, UpdatedAt: now,
	}
	owner := Membership{
		OrganizationID: organizationID, UserID: ownerUserID, Role: RoleOwner,
		AddedByUserID: ownerUserID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateWithOwner(ctx, NewOrganization{Organization: organization, Owner: owner}); err != nil {
		return Organization{}, fmt.Errorf("create organization: %w", err)
	}
	return organization, nil
}

func (s *Service) ListForUser(ctx context.Context, userID string, page PageRequest) (OrganizationPage, error) {
	items, err := s.store.ListForUser(ctx, userID, page.Limit+1, page.After)
	if err != nil {
		return OrganizationPage{}, err
	}
	result := OrganizationPage{Items: items}
	if len(items) > page.Limit {
		result.Items = items[:page.Limit]
		result.NextAfter = result.Items[len(result.Items)-1].ID
	}
	return result, nil
}

func (s *Service) ForMember(ctx context.Context, organizationID, userID string) (Organization, error) {
	if _, err := s.store.Membership(ctx, organizationID, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Organization{}, ErrForbidden
		}
		return Organization{}, err
	}
	return s.store.ByID(ctx, organizationID)
}

func (s *Service) Members(ctx context.Context, organizationID, actorUserID string, page PageRequest) (MembershipPage, error) {
	if _, err := s.store.Membership(ctx, organizationID, actorUserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return MembershipPage{}, ErrForbidden
		}
		return MembershipPage{}, err
	}
	items, err := s.store.ListMembers(ctx, organizationID, page.Limit+1, page.After)
	if err != nil {
		return MembershipPage{}, err
	}
	result := MembershipPage{Items: items}
	if len(items) > page.Limit {
		result.Items = items[:page.Limit]
		result.NextAfter = result.Items[len(result.Items)-1].UserID
	}
	return result, nil
}

func (s *Service) AddMember(ctx context.Context, organizationID, actorUserID, userID string, role Role) error {
	if _, err := ParseRole(string(role)); err != nil {
		return err
	}
	actor, err := s.store.Membership(ctx, organizationID, actorUserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrForbidden
		}
		return err
	}
	if !s.policy.CanAssignMember(actor.Role, role) {
		return ErrForbidden
	}
	now := s.clock().UTC()
	return s.store.AddMember(ctx, Membership{
		OrganizationID: organizationID, UserID: userID, Role: role,
		AddedByUserID: actorUserID, CreatedAt: now, UpdatedAt: now,
	})
}

func (s *Service) ChangeMemberRole(ctx context.Context, organizationID, actorUserID, userID string, role Role) error {
	if _, err := ParseRole(string(role)); err != nil {
		return err
	}
	actor, target, err := s.actorAndTarget(ctx, organizationID, actorUserID, userID)
	if err != nil {
		return err
	}
	if !s.policy.CanChangeMember(actor.Role, target.Role, role) {
		return ErrForbidden
	}
	return s.store.ChangeMemberRole(ctx, organizationID, userID, role, s.clock().UTC())
}

func (s *Service) RemoveMember(ctx context.Context, organizationID, actorUserID, userID string) error {
	actor, target, err := s.actorAndTarget(ctx, organizationID, actorUserID, userID)
	if err != nil {
		return err
	}
	if !s.policy.CanRemoveMember(actor.Role, target.Role) {
		return ErrForbidden
	}
	return s.store.RemoveMember(ctx, organizationID, userID)
}

func (s *Service) actorAndTarget(ctx context.Context, organizationID, actorUserID, targetUserID string) (Membership, Membership, error) {
	actor, err := s.store.Membership(ctx, organizationID, actorUserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Membership{}, Membership{}, ErrForbidden
		}
		return Membership{}, Membership{}, err
	}
	target, err := s.store.Membership(ctx, organizationID, targetUserID)
	if err != nil {
		return Membership{}, Membership{}, err
	}
	return actor, target, nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate organization ID: %w", err)
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
