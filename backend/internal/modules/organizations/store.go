package organizations

import (
	"context"
	"time"
)

type Store interface {
	CreateWithOwner(context.Context, NewOrganization) error
	ByID(context.Context, string) (Organization, error)
	ListForUser(context.Context, string, int, string) ([]Organization, error)
	Membership(context.Context, string, string) (Membership, error)
	ListMembers(context.Context, string, int, string) ([]Membership, error)
	AddMember(context.Context, Membership) error
	ChangeMemberRole(context.Context, string, string, Role, time.Time) error
	RemoveMember(context.Context, string, string) error
}
