package registryhandler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/internal/platform/registryauth"
	"hubcr.io/hubcr/migrations"
)

func TestRegistryTokenFlowWithGORMStoresAndPostgres(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 8,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 14, 30, 0, 0, time.UTC)
	passwords := auth.NewPasswordHasher()
	owner := registryIdentity(
		t, passwords, "91919191-9191-4919-8919-919191919191",
		"registry-owner", "owner-password", now,
	)
	reader := registryIdentity(
		t, passwords, "92929292-9292-4929-8929-929292929292",
		"registry-reader", "reader-password", now,
	)
	outsider := registryIdentity(
		t, passwords, "93939393-9393-4939-8939-939393939393",
		"registry-outsider", "outsider-password", now,
	)
	identityStore := authstore.New(pool.ORM())
	for _, identity := range []auth.Identity{owner, reader, outsider} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}
	authService, err := auth.NewService(identityStore, passwords, auth.ServiceOptions{
		SessionTTL: time.Hour,
		Random:     bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
		Clock:      func() time.Time { return now },
		Limiter:    auth.AllowAllLoginLimiter{},
	})
	if err != nil {
		t.Fatalf("auth.NewService() error = %v", err)
	}
	policy := authorization.NewPolicy()
	organizationService, err := organizations.NewService(
		organizationstore.New(pool.ORM()), namespaces.NormalizeName,
		func() time.Time { return now }, policy,
	)
	if err != nil {
		t.Fatalf("organizations.NewService() error = %v", err)
	}
	organization, err := organizationService.Create(
		ctx, string(owner.User.ID), "Registry-Token-Team", "token integration",
	)
	if err != nil {
		t.Fatalf("organizationService.Create() error = %v", err)
	}
	if err := organizationService.AddMember(
		ctx, organization.ID, string(owner.User.ID), string(reader.User.ID),
		organizations.RoleReader,
	); err != nil {
		t.Fatalf("organizationService.AddMember() error = %v", err)
	}
	repositoryService, err := repositories.NewService(
		repositorystore.New(pool.ORM()), policy, func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("repositories.NewService() error = %v", err)
	}
	for _, input := range []struct {
		name       string
		visibility repositories.Visibility
	}{
		{name: "public-image", visibility: repositories.VisibilityPublic},
		{name: "private-image", visibility: repositories.VisibilityPrivate},
	} {
		if _, err := repositoryService.Create(
			ctx, string(owner.User.ID), organization.NamespaceName,
			input.name, input.visibility, "",
		); err != nil {
			t.Fatalf("repositoryService.Create(%s) error = %v", input.name, err)
		}
	}
	publicContext, err := repositoryService.AuthorizationContext(
		ctx, "", organization.NamespaceName, "public-image",
	)
	if err != nil {
		t.Fatalf("anonymous public AuthorizationContext() error = %v", err)
	}
	if publicContext.Repository.Visibility != repositories.VisibilityPublic ||
		publicContext.Namespace.Kind != repositories.NamespaceOrganization {
		t.Fatalf("anonymous public AuthorizationContext() = %#v", publicContext)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
	if err != nil {
		t.Fatalf("registry.NewRS256Signer() error = %v", err)
	}
	credentialAuthenticator, err := registryauth.New(authService)
	if err != nil {
		t.Fatalf("registryauth.New() error = %v", err)
	}
	tokenService, err := registry.NewService(
		credentialAuthenticator, repositoryService, policy, signer,
		registry.ServiceOptions{
			Service: "hubcr-registry", Issuer: "hubcr-token-service",
			TokenTTL: 5 * time.Minute, ClockSkew: 30 * time.Second,
			Clock:  func() time.Time { return now },
			Random: bytes.NewReader(bytes.Repeat([]byte{8}, 256)),
		},
	)
	if err != nil {
		t.Fatalf("registry.NewService() error = %v", err)
	}
	tokenHandler, err := New(tokenService, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("registryhandler.New() error = %v", err)
	}
	router := httpapi.NewRouter()
	RegisterRoutes(router, tokenHandler)
	handler := httpapi.WithRequestID(httpapi.Recover(router))
	verifier, err := registry.NewRS256Verifier(
		map[string]*rsa.PublicKey{signer.KeyID(): &privateKey.PublicKey},
		"hubcr-token-service", "hubcr-registry",
		func() time.Time { return now }, 30*time.Second,
	)
	if err != nil {
		t.Fatalf("registry.NewRS256Verifier() error = %v", err)
	}

	assertRegistryAccess(t, handler, verifier, tokenRequest{
		scope: "repository:" + organization.NamespaceName + "/public-image:pull,push",
	}, "", []registry.Action{registry.ActionPull})
	assertRegistryAccess(t, handler, verifier, tokenRequest{
		scope: "repository:" + organization.NamespaceName + "/private-image:pull,push",
	}, "", []registry.Action{})
	assertRegistryAccess(t, handler, verifier, tokenRequest{
		scope: "repository:" + organization.NamespaceName + "/private-image:pull,push",
		user:  "registry-reader", password: "reader-password",
	}, string(reader.User.ID), []registry.Action{registry.ActionPull})
	assertRegistryAccess(t, handler, verifier, tokenRequest{
		scope: "repository:" + organization.NamespaceName + "/private-image:pull,push,delete",
		user:  "registry-owner", password: "owner-password",
	}, string(owner.User.ID), []registry.Action{registry.ActionPull, registry.ActionPush})
	assertRegistryAccess(t, handler, verifier, tokenRequest{
		scope: "repository:" + organization.NamespaceName + "/private-image:pull,push",
		user:  "registry-outsider", password: "outsider-password",
	}, string(outsider.User.ID), []registry.Action{})
	assertRegistryAccess(t, handler, verifier, tokenRequest{
		scope: "repository:" + organization.NamespaceName + "/missing-image:pull,push",
	}, "", []registry.Action{})

	t.Run("web cookie is ignored", func(t *testing.T) {
		request := tokenRequest{
			scope:  "repository:" + organization.NamespaceName + "/private-image:pull",
			cookie: &http.Cookie{Name: "hubcr_session", Value: "browser-session"},
		}
		claims := requestRegistryClaims(t, handler, verifier, request)
		if claims.Subject != "" || len(claims.Access[0].Actions) != 0 {
			t.Fatalf("cookie-authenticated claims = %#v", claims)
		}
	})
	t.Run("invalid Basic credential is unauthorized", func(t *testing.T) {
		request := httptest.NewRequest(
			http.MethodGet,
			"/token?service=hubcr-registry&scope=repository%3A"+
				organization.NamespaceName+"%2Fprivate-image%3Apull",
			nil,
		)
		request.SetBasicAuth("registry-owner", "wrong-password")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
		}
	})
}

type tokenRequest struct {
	scope    string
	user     string
	password string
	cookie   *http.Cookie
}

func assertRegistryAccess(
	t *testing.T,
	handler http.Handler,
	verifier *registry.RS256Verifier,
	request tokenRequest,
	wantSubject string,
	wantActions []registry.Action,
) {
	t.Helper()
	claims := requestRegistryClaims(t, handler, verifier, request)
	if claims.Subject != wantSubject || len(claims.Access) != 1 ||
		!actionsEqual(claims.Access[0].Actions, wantActions) {
		t.Fatalf("claims = %#v, want subject %q actions %#v", claims, wantSubject, wantActions)
	}
}

func requestRegistryClaims(
	t *testing.T,
	handler http.Handler,
	verifier *registry.RS256Verifier,
	input tokenRequest,
) registry.Claims {
	t.Helper()
	target := "/token?service=hubcr-registry&scope=" + url.QueryEscape(input.scope)
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if input.user != "" {
		request.SetBasicAuth(input.user, input.password)
	}
	if input.cookie != nil {
		request.AddCookie(input.cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	var response tokenResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if response.Token == "" || response.Token != response.AccessToken ||
		response.ExpiresIn != 300 {
		t.Fatalf("token response = %#v", response)
	}
	claims, err := verifier.Verify(response.Token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	return claims
}

func registryIdentity(
	t *testing.T,
	passwords auth.PasswordCodec,
	id auth.ID,
	username, password string,
	now time.Time,
) auth.Identity {
	t.Helper()
	passwordHash, err := passwords.Hash([]byte(password))
	if err != nil {
		t.Fatalf("Hash(%s) error = %v", username, err)
	}
	return auth.Identity{
		User: auth.User{ID: id, Username: username, CreatedAt: now, UpdatedAt: now},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: passwordHash,
			PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: username},
	}
}

func actionsEqual(left, right []registry.Action) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
