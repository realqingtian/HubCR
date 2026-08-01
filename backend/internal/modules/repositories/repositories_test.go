package repositories

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{name: "lowercase", value: "backend", want: "backend", ok: true},
		{name: "normalizes case", value: "Team.API_V2", want: "team.api_v2", ok: true},
		{name: "empty", value: ""},
		{name: "whitespace", value: " backend"},
		{name: "unicode", value: "镜像"},
		{name: "path separator", value: "team/backend"},
		{name: "repeated separator", value: "team--backend"},
		{name: "leading separator", value: "-backend"},
		{name: "trailing separator", value: "backend-"},
		{name: "too long", value: strings.Repeat("a", MaxNameLength+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeName(test.value)
			if test.ok {
				if err != nil || got != test.want {
					t.Fatalf("NormalizeName(%q) = %q, %v; want %q", test.value, got, err, test.want)
				}
				return
			}
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("NormalizeName(%q) error = %v, want ErrInvalidName", test.value, err)
			}
		})
	}
}

func TestParseVisibilityRequiresExplicitCanonicalValue(t *testing.T) {
	t.Parallel()
	for _, value := range []Visibility{VisibilityPublic, VisibilityPrivate} {
		got, err := ParseVisibility(string(value))
		if err != nil || got != value {
			t.Fatalf("ParseVisibility(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"", "public", "UNKNOWN"} {
		if _, err := ParseVisibility(value); !errors.Is(err, ErrInvalidVisibility) {
			t.Fatalf("ParseVisibility(%q) error = %v, want ErrInvalidVisibility", value, err)
		}
	}
}

func TestNewNormalizesIdentityAndRecordsInitialVisibilityActor(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 1, 20, 30, 0, 123, time.FixedZone("test", 8*60*60))
	repository, err := New(NewRepository{
		NamespaceID: "namespace-id", RequestedName: "Backend.API", Visibility: VisibilityPrivate,
		Description: "service image", CreatedByUserID: "user-id",
	}, at)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if repository.ID == "" || repository.Name != "backend.api" || repository.Visibility != VisibilityPrivate {
		t.Fatalf("New() repository = %#v", repository)
	}
	if repository.CreatedByUserID != "user-id" || repository.VisibilityUpdatedByUserID != "user-id" {
		t.Fatalf("New() actors = %#v", repository)
	}
	wantTime := at.UTC()
	if !repository.CreatedAt.Equal(wantTime) || !repository.UpdatedAt.Equal(wantTime) ||
		!repository.VisibilityUpdatedAt.Equal(wantTime) {
		t.Fatalf("New() timestamps = %#v, want %v", repository, wantTime)
	}
}

func TestNewRejectsIncompleteOrInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input NewRepository
		want  error
	}{
		{name: "missing namespace", input: NewRepository{RequestedName: "backend", Visibility: VisibilityPrivate, CreatedByUserID: "user-id"}, want: ErrInvalidRepository},
		{name: "missing actor", input: NewRepository{NamespaceID: "namespace-id", RequestedName: "backend", Visibility: VisibilityPrivate}, want: ErrInvalidRepository},
		{name: "invalid name", input: NewRepository{NamespaceID: "namespace-id", RequestedName: "bad/name", Visibility: VisibilityPrivate, CreatedByUserID: "user-id"}, want: ErrInvalidName},
		{name: "missing visibility", input: NewRepository{NamespaceID: "namespace-id", RequestedName: "backend", CreatedByUserID: "user-id"}, want: ErrInvalidVisibility},
		{name: "invalid visibility", input: NewRepository{NamespaceID: "namespace-id", RequestedName: "backend", Visibility: "INTERNAL", CreatedByUserID: "user-id"}, want: ErrInvalidVisibility},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.input, time.Now()); !errors.Is(err, test.want) {
				t.Fatalf("New() error = %v, want %v", err, test.want)
			}
		})
	}
}
