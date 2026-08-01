package registry

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseScopesCanonicalizesOrderAndDuplicates(t *testing.T) {
	scopes, err := ParseScopes([]string{
		"repository:team/zeta:push,pull,push",
		"repository:team/alpha:delete,pull",
		"repository:team/zeta:delete",
	})
	if err != nil {
		t.Fatalf("ParseScopes() error = %v", err)
	}
	want := []Scope{
		{
			Type: ResourceRepository, Name: "team/alpha",
			Namespace: "team", Repository: "alpha",
			Actions: []Action{ActionPull, ActionDelete},
		},
		{
			Type: ResourceRepository, Name: "team/zeta",
			Namespace: "team", Repository: "zeta",
			Actions: []Action{ActionPull, ActionPush, ActionDelete},
		},
	}
	if !reflect.DeepEqual(scopes, want) {
		t.Fatalf("ParseScopes() = %#v, want %#v", scopes, want)
	}
}

func TestParseScopesAllowsAbsentScope(t *testing.T) {
	scopes, err := ParseScopes(nil)
	if err != nil || len(scopes) != 0 {
		t.Fatalf("ParseScopes(nil) = %#v, %v", scopes, err)
	}
}

func TestParseScopeRejectsNonCanonicalOrUnsupportedInput(t *testing.T) {
	tests := []string{
		"",
		"repository:team/image:",
		"repository:team/image:pull,",
		"repository:Team/image:pull",
		"repository:team/IMAGE:pull",
		"repository:team/image/extra:pull",
		"repository:host:5000/team/image:pull",
		"repository(plugin):team/image:pull",
		"registry:catalog:*",
		"repository:team/image:mount",
		"repository:team//image:pull",
		"repository:team/../image:pull",
		"repository:团队/image:pull",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseScope(value); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("ParseScope(%q) error = %v, want ErrInvalidRequest", value, err)
			}
		})
	}
	if _, err := ParseScope("repository:team/" + strings.Repeat("a", 65) + ":pull"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("oversized repository error = %v", err)
	}
}

func TestParseScopesRejectsTooManyValues(t *testing.T) {
	values := make([]string, MaxScopes+1)
	for index := range values {
		values[index] = "repository:team/image:pull"
	}
	if _, err := ParseScopes(values); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("ParseScopes() error = %v, want ErrInvalidRequest", err)
	}
}
