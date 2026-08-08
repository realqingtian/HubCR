package registry

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
)

const jtiBytes = 16

type CredentialAuthenticator interface {
	Authenticate(context.Context, string, []byte, string) (Subject, error)
}

type RepositoryResolver interface {
	AuthorizationContext(
		context.Context,
		string,
		string,
		string,
	) (repositories.AuthorizationContext, error)
}

type Policy interface {
	AllowsOrganization(organizations.Role, authorization.Capability) bool
	AllowsPersonalNamespace(bool, authorization.Capability) bool
	AllowsPublicRepositoryPull(bool) bool
}

type ServiceOptions struct {
	Service   string
	Issuer    string
	TokenTTL  time.Duration
	ClockSkew time.Duration
	Clock     func() time.Time
	Random    io.Reader
}

type Service struct {
	authenticator CredentialAuthenticator
	repositories  RepositoryResolver
	policy        Policy
	signer        TokenSigner
	service       string
	issuer        string
	tokenTTL      time.Duration
	clockSkew     time.Duration
	clock         func() time.Time
	random        io.Reader
}

func NewService(
	authenticator CredentialAuthenticator,
	repositoryResolver RepositoryResolver,
	policy Policy,
	signer TokenSigner,
	options ServiceOptions,
) (*Service, error) {
	if authenticator == nil || repositoryResolver == nil || policy == nil ||
		signer == nil || signer.KeyID() == "" || options.Service == "" ||
		options.Issuer == "" || options.Clock == nil || options.Random == nil ||
		options.TokenTTL < time.Minute || options.TokenTTL > 15*time.Minute ||
		options.TokenTTL%time.Second != 0 ||
		options.ClockSkew < 0 || options.ClockSkew > time.Minute {
		return nil, errors.New("Registry token service dependencies and options must be configured")
	}
	return &Service{
		authenticator: authenticator,
		repositories:  repositoryResolver,
		policy:        policy,
		signer:        signer,
		service:       options.Service,
		issuer:        options.Issuer,
		tokenTTL:      options.TokenTTL,
		clockSkew:     options.ClockSkew,
		clock:         options.Clock,
		random:        options.Random,
	}, nil
}

func (s *Service) Issue(ctx context.Context, request IssueRequest) (IssueResult, error) {
	if request.Service != s.service || !validClientID(request.ClientID) {
		return IssueResult{}, ErrInvalidRequest
	}
	scopes, err := ParseScopes(request.RawScopes)
	if err != nil {
		return IssueResult{}, err
	}
	subject, err := s.authenticate(ctx, request.Credentials, request.RateLimitKey)
	if err != nil {
		return IssueResult{}, err
	}
	access := make([]Access, 0, len(scopes))
	for _, scope := range scopes {
		actions, err := s.authorize(ctx, subject, scope)
		if err != nil {
			return IssueResult{}, err
		}
		access = append(access, Access{
			Type: scope.Type, Name: scope.Name, Actions: actions,
		})
	}

	now := s.clock().UTC().Truncate(time.Second)
	tokenID := make([]byte, jtiBytes)
	if _, err := io.ReadFull(s.random, tokenID); err != nil {
		return IssueResult{}, fmt.Errorf("%w: generate token ID", ErrUnavailable)
	}
	claims := Claims{
		Issuer: s.issuer, Subject: subject.ID, Audience: s.service,
		ExpiresAt: now.Add(s.tokenTTL).Unix(),
		NotBefore: now.Add(-s.clockSkew).Unix(),
		IssuedAt:  now.Unix(),
		ID:        base64.RawURLEncoding.EncodeToString(tokenID),
		Access:    access,
	}
	token, err := s.signer.Sign(ctx, claims)
	if err != nil {
		return IssueResult{}, fmt.Errorf("%w: sign token", ErrUnavailable)
	}
	return IssueResult{
		Token: token, ExpiresIn: int(s.tokenTTL / time.Second), IssuedAt: now,
		Subject: subject, Access: access, KeyID: s.signer.KeyID(),
	}, nil
}

func (s *Service) authenticate(
	ctx context.Context,
	credentials *Credentials,
	rateLimitKey string,
) (Subject, error) {
	if credentials == nil {
		return Subject{}, nil
	}
	if credentials.Username == "" {
		return Subject{}, ErrInvalidCredentials
	}
	subject, err := s.authenticator.Authenticate(
		ctx, credentials.Username, credentials.Password, rateLimitKey,
	)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return Subject{}, ErrInvalidCredentials
		}
		if errors.Is(err, ErrRateLimited) {
			return Subject{}, ErrRateLimited
		}
		return Subject{}, fmt.Errorf("%w: authenticate credentials", ErrUnavailable)
	}
	if subject.ID == "" {
		return Subject{}, fmt.Errorf("%w: authenticator returned empty subject", ErrUnavailable)
	}
	return subject, nil
}

func (s *Service) authorize(ctx context.Context, subject Subject, scope Scope) ([]Action, error) {
	repositoryContext, err := s.repositories.AuthorizationContext(
		ctx, subject.ID, scope.Namespace, scope.Repository,
	)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return []Action{}, nil
		}
		return nil, fmt.Errorf("%w: resolve repository policy", ErrUnavailable)
	}

	allowed := make(map[Action]struct{}, 2)
	isPublic := repositoryContext.Repository.Visibility == repositories.VisibilityPublic
	if s.policy.AllowsPublicRepositoryPull(isPublic) {
		allowed[ActionPull] = struct{}{}
	}
	if !subject.Anonymous() {
		switch repositoryContext.Namespace.Kind {
		case repositories.NamespacePersonal:
			if s.policy.AllowsPersonalNamespace(
				repositoryContext.Namespace.IsPersonalOwner,
				authorization.PullPrivateRepositories,
			) {
				allowed[ActionPull] = struct{}{}
			}
			if s.policy.AllowsPersonalNamespace(
				repositoryContext.Namespace.IsPersonalOwner,
				authorization.PushRepositories,
			) {
				allowed[ActionPush] = struct{}{}
			}
		case repositories.NamespaceOrganization:
			if s.policy.AllowsOrganization(
				repositoryContext.Namespace.OrganizationRole,
				authorization.PullPrivateRepositories,
			) {
				allowed[ActionPull] = struct{}{}
			}
			if s.policy.AllowsOrganization(
				repositoryContext.Namespace.OrganizationRole,
				authorization.PushRepositories,
			) {
				allowed[ActionPush] = struct{}{}
			}
		default:
			return nil, fmt.Errorf("%w: unknown namespace ownership", ErrUnavailable)
		}
	}

	requested := make(map[Action]struct{}, len(scope.Actions))
	for _, action := range scope.Actions {
		requested[action] = struct{}{}
	}
	result := make([]Action, 0, len(scope.Actions))
	for _, action := range actionOrder {
		if _, wanted := requested[action]; !wanted {
			continue
		}
		if _, granted := allowed[action]; granted {
			result = append(result, action)
		}
	}
	return result, nil
}

func validClientID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > MaxClientIDBytes {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}
