package registry

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
)

const registryTestSubject = "11111111-1111-4111-8111-111111111111"

func TestServiceRegistryAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		credentials *Credentials
		context     repositories.AuthorizationContext
		wantActions []Action
	}{
		{
			name: "anonymous public pull",
			context: registryContext(
				repositories.VisibilityPublic, repositories.NamespacePersonal, false, "",
			),
			wantActions: []Action{ActionPull},
		},
		{
			name: "anonymous private denied",
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespacePersonal, false, "",
			),
			wantActions: []Action{},
		},
		{
			name:        "personal owner pull and push",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespacePersonal, true, "",
			),
			wantActions: []Action{ActionPull, ActionPush},
		},
		{
			name:        "personal non-owner public pull only",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPublic, repositories.NamespacePersonal, false, "",
			),
			wantActions: []Action{ActionPull},
		},
		{
			name:        "organization owner pull and push",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespaceOrganization, false,
				organizations.RoleOwner,
			),
			wantActions: []Action{ActionPull, ActionPush},
		},
		{
			name:        "organization admin pull and push",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespaceOrganization, false,
				organizations.RoleAdmin,
			),
			wantActions: []Action{ActionPull, ActionPush},
		},
		{
			name:        "organization writer pull and push",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespaceOrganization, false,
				organizations.RoleWriter,
			),
			wantActions: []Action{ActionPull, ActionPush},
		},
		{
			name:        "organization reader pull only",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespaceOrganization, false,
				organizations.RoleReader,
			),
			wantActions: []Action{ActionPull},
		},
		{
			name:        "wrong organization member denied",
			credentials: validRegistryCredentials(),
			context: registryContext(
				repositories.VisibilityPrivate, repositories.NamespaceOrganization, false, "",
			),
			wantActions: []Action{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &serviceRepositoryResolver{contexts: map[string]repositories.AuthorizationContext{
				"team/image": test.context,
			}}
			signer := &serviceSigner{}
			service := newRegistryService(t, resolver, signer, nil)
			result, err := service.Issue(context.Background(), IssueRequest{
				Service: "hubcr-registry",
				RawScopes: []string{
					"repository:team/image:delete,push,pull",
				},
				Credentials: test.credentials,
			})
			if err != nil {
				t.Fatalf("Issue() error = %v", err)
			}
			if len(result.Access) != 1 ||
				!reflect.DeepEqual(result.Access[0].Actions, test.wantActions) {
				t.Fatalf("Issue() access = %#v, want actions %#v", result.Access, test.wantActions)
			}
			if !reflect.DeepEqual(signer.claims.Access, result.Access) {
				t.Fatalf("signed access = %#v, result %#v", signer.claims.Access, result.Access)
			}
			wantActor := ""
			if test.credentials != nil {
				wantActor = registryTestSubject
			}
			if resolver.actor != wantActor {
				t.Fatalf("resolver actor = %q, want %q", resolver.actor, wantActor)
			}
		})
	}
}

func TestServiceIssuesDeterministicMultipleExactScopes(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 500, time.UTC)
	resolver := &serviceRepositoryResolver{contexts: map[string]repositories.AuthorizationContext{
		"team/alpha": registryContext(
			repositories.VisibilityPrivate, repositories.NamespaceOrganization, false,
			organizations.RoleReader,
		),
		"team/zeta": registryContext(
			repositories.VisibilityPublic, repositories.NamespaceOrganization, false, "",
		),
	}}
	signer := &serviceSigner{}
	service := newRegistryService(t, resolver, signer, func() time.Time { return now })

	result, err := service.Issue(context.Background(), IssueRequest{
		Service: "hubcr-registry", ClientID: "docker",
		RawScopes: []string{
			"repository:team/zeta:push,pull",
			"repository:team/alpha:push,pull",
		},
		Credentials: validRegistryCredentials(),
	})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	wantAccess := []Access{
		{Type: ResourceRepository, Name: "team/alpha", Actions: []Action{ActionPull}},
		{Type: ResourceRepository, Name: "team/zeta", Actions: []Action{ActionPull}},
	}
	if !reflect.DeepEqual(result.Access, wantAccess) {
		t.Fatalf("Issue() access = %#v, want %#v", result.Access, wantAccess)
	}
	if result.ExpiresIn != 300 || !result.IssuedAt.Equal(now.Truncate(time.Second)) {
		t.Fatalf("Issue() lifetime = %d %v", result.ExpiresIn, result.IssuedAt)
	}
	if signer.claims.Subject != registryTestSubject ||
		signer.claims.Issuer != "hubcr-token-service" ||
		signer.claims.Audience != "hubcr-registry" ||
		signer.claims.NotBefore != now.Truncate(time.Second).Add(-30*time.Second).Unix() ||
		signer.claims.ExpiresAt != now.Truncate(time.Second).Add(5*time.Minute).Unix() ||
		len(signer.claims.ID) != 22 {
		t.Fatalf("signed claims = %#v", signer.claims)
	}
}

func TestServiceReturnsEmptyAccessForMissingRepositoryWithoutExistenceSignal(t *testing.T) {
	resolver := &serviceRepositoryResolver{
		errors: map[string]error{"team/missing": repositories.ErrNotFound},
	}
	service := newRegistryService(t, resolver, &serviceSigner{}, nil)
	result, err := service.Issue(context.Background(), IssueRequest{
		Service:   "hubcr-registry",
		RawScopes: []string{"repository:team/missing:pull,push"},
	})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if len(result.Access) != 1 || len(result.Access[0].Actions) != 0 {
		t.Fatalf("Issue() access = %#v, want exact repository with empty actions", result.Access)
	}
}

func TestServiceClassifiesCredentialsAndDependencyFailures(t *testing.T) {
	tests := []struct {
		name          string
		authenticator CredentialAuthenticator
		resolver      *serviceRepositoryResolver
		credentials   *Credentials
		want          error
	}{
		{
			name: "invalid credentials",
			authenticator: serviceAuthenticator{
				err: ErrInvalidCredentials,
			},
			resolver:    &serviceRepositoryResolver{},
			credentials: validRegistryCredentials(),
			want:        ErrInvalidCredentials,
		},
		{
			name: "authentication dependency",
			authenticator: serviceAuthenticator{
				err: errors.New("database unavailable"),
			},
			resolver:    &serviceRepositoryResolver{},
			credentials: validRegistryCredentials(),
			want:        ErrUnavailable,
		},
		{
			name:          "repository dependency",
			authenticator: serviceAuthenticator{subject: Subject{ID: registryTestSubject}},
			resolver: &serviceRepositoryResolver{
				errors: map[string]error{"team/image": errors.New("database unavailable")},
			},
			credentials: validRegistryCredentials(),
			want:        ErrUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newRegistryServiceWithAuthenticator(
				t, test.authenticator, test.resolver, &serviceSigner{}, nil,
			)
			_, err := service.Issue(context.Background(), IssueRequest{
				Service:     "hubcr-registry",
				RawScopes:   []string{"repository:team/image:pull"},
				Credentials: test.credentials,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Issue() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestServiceRejectsProtocolInputBeforeAuthentication(t *testing.T) {
	authenticator := &countingAuthenticator{}
	service := newRegistryServiceWithAuthenticator(
		t, authenticator, &serviceRepositoryResolver{}, &serviceSigner{}, nil,
	)
	tests := []IssueRequest{
		{Service: "wrong"},
		{Service: "hubcr-registry", ClientID: string([]byte{0x1f})},
		{Service: "hubcr-registry", RawScopes: []string{"bad"}},
	}
	for _, request := range tests {
		if _, err := service.Issue(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("Issue(%#v) error = %v, want ErrInvalidRequest", request, err)
		}
	}
	if authenticator.calls != 0 {
		t.Fatalf("authentication calls = %d, want 0", authenticator.calls)
	}
}

func newRegistryService(
	t *testing.T,
	resolver *serviceRepositoryResolver,
	signer TokenSigner,
	clock func() time.Time,
) *Service {
	t.Helper()
	return newRegistryServiceWithAuthenticator(
		t,
		serviceAuthenticator{subject: Subject{ID: registryTestSubject}},
		resolver,
		signer,
		clock,
	)
}

func newRegistryServiceWithAuthenticator(
	t *testing.T,
	authenticator CredentialAuthenticator,
	resolver RepositoryResolver,
	signer TokenSigner,
	clock func() time.Time,
) *Service {
	t.Helper()
	if clock == nil {
		clock = func() time.Time {
			return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
		}
	}
	service, err := NewService(
		authenticator, resolver, authorization.NewPolicy(), signer,
		ServiceOptions{
			Service: "hubcr-registry", Issuer: "hubcr-token-service",
			TokenTTL: 5 * time.Minute, ClockSkew: 30 * time.Second,
			Clock: clock, Random: bytes.NewReader(bytes.Repeat([]byte{9}, jtiBytes*8)),
		},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func validRegistryCredentials() *Credentials {
	return &Credentials{Username: "owner", Password: []byte("correct")}
}

func registryContext(
	visibility repositories.Visibility,
	kind repositories.NamespaceKind,
	personalOwner bool,
	role organizations.Role,
) repositories.AuthorizationContext {
	return repositories.AuthorizationContext{
		Repository: repositories.Repository{
			ID: "repository-id", NamespaceID: "namespace-id", Name: "image",
			Visibility: visibility,
		},
		Namespace: repositories.NamespaceAccess{
			NamespaceID: "namespace-id", NamespaceName: "team", Kind: kind,
			IsPersonalOwner: personalOwner, OrganizationID: "organization-id",
			OrganizationRole: role,
		},
	}
}

type serviceAuthenticator struct {
	subject Subject
	err     error
}

func (a serviceAuthenticator) Authenticate(context.Context, string, []byte) (Subject, error) {
	return a.subject, a.err
}

type countingAuthenticator struct{ calls int }

func (a *countingAuthenticator) Authenticate(context.Context, string, []byte) (Subject, error) {
	a.calls++
	return Subject{ID: registryTestSubject}, nil
}

type serviceRepositoryResolver struct {
	contexts map[string]repositories.AuthorizationContext
	errors   map[string]error
	actor    string
}

func (r *serviceRepositoryResolver) AuthorizationContext(
	_ context.Context,
	actor, namespace, repository string,
) (repositories.AuthorizationContext, error) {
	r.actor = actor
	name := namespace + "/" + repository
	if err := r.errors[name]; err != nil {
		return repositories.AuthorizationContext{}, err
	}
	return r.contexts[name], nil
}

type serviceSigner struct {
	claims Claims
	err    error
}

func (*serviceSigner) KeyID() string { return "test-key" }
func (s *serviceSigner) Sign(_ context.Context, claims Claims) (string, error) {
	s.claims = claims
	return "signed-token", s.err
}
