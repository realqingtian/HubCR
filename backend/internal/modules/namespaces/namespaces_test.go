package namespaces

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNormalizeName(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "Sunny", want: "sunny"},
		{input: "team-one", want: "team-one"},
		{input: "team_one.example", want: "team_one.example"},
	} {
		got, err := NormalizeName(test.input)
		if err != nil || got != test.want {
			t.Fatalf("NormalizeName(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
}

func TestNormalizeNameRejectsNonCanonicalPaths(t *testing.T) {
	for _, input := range []string{"", " two ", "two//parts", "two--parts", "-leading", "trailing_", "团队"} {
		t.Run(input, func(t *testing.T) {
			if _, err := NormalizeName(input); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("NormalizeName(%q) error = %v, want ErrInvalidName", input, err)
			}
		})
	}
}

func TestCreatePersonalNormalizesAndPersists(t *testing.T) {
	now := time.Date(2026, 8, 1, 15, 0, 0, 0, time.UTC)
	store := &namespaceTestStore{}
	service, err := NewService(store, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	namespace, err := service.CreatePersonal(context.Background(), "user-id", "Sunny")
	if err != nil {
		t.Fatalf("CreatePersonal() error = %v", err)
	}
	if namespace.Name != "sunny" || namespace.OwnerUserID != "user-id" || namespace.ID == "" || store.created != namespace {
		t.Fatalf("CreatePersonal() = %#v, stored %#v", namespace, store.created)
	}
}

type namespaceTestStore struct{ created Namespace }

func (s *namespaceTestStore) CreatePersonal(_ context.Context, namespace Namespace) error {
	s.created = namespace
	return nil
}
func (s *namespaceTestStore) ByName(context.Context, string) (Namespace, error) {
	return s.created, nil
}
func (s *namespaceTestStore) ByOwnerUserID(context.Context, string) (Namespace, error) {
	return s.created, nil
}
