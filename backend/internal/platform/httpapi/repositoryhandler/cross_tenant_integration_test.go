package repositoryhandler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/migrations"
)

func TestCrossTenantRepositoryIsolationWithPostgres(t *testing.T) {
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

	now := time.Date(2026, 8, 1, 23, 30, 0, 0, time.UTC)
	owner := repositoryHTTPIdentity("71717171-7171-4717-8717-717171717171", "isolation-owner", now)
	admin := repositoryHTTPIdentity("72727272-7272-4727-8727-727272727272", "isolation-admin", now)
	writer := repositoryHTTPIdentity("73737373-7373-4737-8737-737373737373", "isolation-writer", now)
	reader := repositoryHTTPIdentity("74747474-7474-4747-8747-747474747474", "isolation-reader", now)
	outsider := repositoryHTTPIdentity("75757575-7575-4757-8757-757575757575", "isolation-outsider", now)
	secondOwner := repositoryHTTPIdentity("76767676-7676-4767-8767-767676767676", "isolation-second-owner", now)

	identityStore := authstore.New(pool.ORM())
	for _, identity := range []auth.Identity{owner, admin, writer, reader, outsider, secondOwner} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}

	activeSessions := map[string]auth.Identity{
		"isolation-owner-token": owner, "isolation-admin-token": admin,
		"isolation-writer-token": writer, "isolation-reader-token": reader,
		"isolation-outsider-token": outsider, "isolation-second-owner-token": secondOwner,
	}
	sessionNumber := byte(0x81)
	for token, identity := range activeSessions {
		createIsolationSession(t, ctx, identityStore, sessionNumber, token, identity.User.ID, now.Add(time.Hour), nil, now)
		sessionNumber++
	}
	expiredAt := now.Add(-time.Minute)
	revokedAt := now.Add(-2 * time.Minute)
	createIsolationSession(
		t, ctx, identityStore, 0x91, "isolation-expired-token", outsider.User.ID, expiredAt, nil, now.Add(-2*time.Hour),
	)
	createIsolationSession(
		t, ctx, identityStore, 0x92, "isolation-revoked-token", outsider.User.ID, now.Add(time.Hour), &revokedAt, now.Add(-3*time.Minute),
	)

	authService, err := auth.NewService(identityStore, auth.NewPasswordHasher(), auth.ServiceOptions{
		SessionTTL: time.Hour,
		Random:     bytes.NewReader(bytes.Repeat([]byte{0x6a}, 32)),
		Clock:      func() time.Time { return now },
		Limiter:    auth.AllowAllLoginLimiter{},
	})
	if err != nil {
		t.Fatalf("auth.NewService() error = %v", err)
	}
	policy := authorization.NewPolicy()
	organizationService, err := organizations.NewService(
		organizationstore.New(pool.ORM()), namespaces.NormalizeName, func() time.Time { return now }, policy,
	)
	if err != nil {
		t.Fatalf("organizations.NewService() error = %v", err)
	}
	primary, err := organizationService.Create(ctx, string(owner.User.ID), "Isolation-Primary", "primary tenant")
	if err != nil {
		t.Fatalf("create primary organization: %v", err)
	}
	for _, member := range []struct {
		identity auth.Identity
		role     organizations.Role
	}{{admin, organizations.RoleAdmin}, {writer, organizations.RoleWriter}, {reader, organizations.RoleReader}} {
		if err := organizationService.AddMember(
			ctx, primary.ID, string(owner.User.ID), string(member.identity.User.ID), member.role,
		); err != nil {
			t.Fatalf("add %s member: %v", member.role, err)
		}
	}
	secondary, err := organizationService.Create(
		ctx, string(secondOwner.User.ID), "Isolation-Secondary", "second tenant",
	)
	if err != nil {
		t.Fatalf("create secondary organization: %v", err)
	}

	repositoryService, err := repositories.NewService(
		repositorystore.New(pool.ORM()), policy, func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("repositories.NewService() error = %v", err)
	}
	router := httpapi.NewRouter()
	RegisterRoutes(router, New(authService, repositoryService))
	handler := httpapi.WithRequestID(httpapi.Recover(router))

	t.Run("personal namespace never inherits another users authority", func(t *testing.T) {
		basePath := "/api/v1/namespaces/" + owner.PersonalNamespace.Name + "/repositories"
		assertRepositoryStatus(t, handler, http.MethodPost, basePath,
			`{"name":"private-app","visibility":"PRIVATE"}`, "isolation-owner-token", http.StatusCreated, "")
		assertRepositoryStatus(t, handler, http.MethodPost, basePath,
			`{"name":"foreign-app","visibility":"PRIVATE"}`, "isolation-outsider-token", http.StatusForbidden, "")
		assertRepositoryStatus(t, handler, http.MethodGet, basePath, "", "isolation-outsider-token", http.StatusOK, `"items":[]`)
		assertRepositoryStatus(t, handler, http.MethodGet, basePath+"/private-app", "", "isolation-outsider-token", http.StatusNotFound, "")
		assertRepositoryStatus(t, handler, http.MethodPatch, basePath+"/private-app",
			`{"visibility":"PUBLIC"}`, "isolation-owner-token", http.StatusOK, `"visibility":"PUBLIC"`)
		assertRepositoryStatus(t, handler, http.MethodGet, basePath+"/private-app", "", "isolation-outsider-token", http.StatusOK, `"name":"private-app"`)
	})

	t.Run("organization roles and tenant boundaries fail closed", func(t *testing.T) {
		primaryPath := "/api/v1/namespaces/" + primary.NamespaceName + "/repositories"
		assertRepositoryStatus(t, handler, http.MethodPost, primaryPath,
			`{"name":"shared","visibility":"PRIVATE","description":"initial"}`, "isolation-writer-token", http.StatusCreated, "")
		assertRepositoryStatus(t, handler, http.MethodGet, primaryPath+"/shared", "", "isolation-reader-token", http.StatusOK, `"visibility":"PRIVATE"`)
		assertRepositoryStatus(t, handler, http.MethodPatch, primaryPath+"/shared",
			`{"description":"reader edit"}`, "isolation-reader-token", http.StatusForbidden, "")
		assertRepositoryStatus(t, handler, http.MethodPatch, primaryPath+"/shared",
			`{"description":"writer edit"}`, "isolation-writer-token", http.StatusOK, `"description":"writer edit"`)
		assertRepositoryStatus(t, handler, http.MethodPatch, primaryPath+"/shared",
			`{"visibility":"PUBLIC"}`, "isolation-writer-token", http.StatusForbidden, "")
		assertRepositoryStatus(t, handler, http.MethodPatch, primaryPath+"/shared",
			`{"visibility":"PUBLIC"}`, "isolation-admin-token", http.StatusOK, `"visibility":"PUBLIC"`)
		assertRepositoryStatus(t, handler, http.MethodGet, primaryPath, "", "isolation-outsider-token", http.StatusOK, `"name":"shared"`)
		assertRepositoryStatus(t, handler, http.MethodPatch, primaryPath+"/shared",
			`{"description":"outsider edit"}`, "isolation-outsider-token", http.StatusForbidden, "")

		secondaryPath := "/api/v1/namespaces/" + secondary.NamespaceName + "/repositories"
		assertRepositoryStatus(t, handler, http.MethodPost, secondaryPath,
			`{"name":"private-second","visibility":"PRIVATE"}`, "isolation-second-owner-token", http.StatusCreated, "")
		assertRepositoryStatus(t, handler, http.MethodGet, secondaryPath+"/private-second", "", "isolation-owner-token", http.StatusNotFound, "")
		assertRepositoryStatus(t, handler, http.MethodPatch, secondaryPath+"/private-second",
			`{"description":"cross-tenant"}`, "isolation-owner-token", http.StatusForbidden, "")
		assertRepositoryStatus(t, handler, http.MethodGet, "/api/v1/namespaces/isolation-missing/repositories", "", "isolation-owner-token", http.StatusNotFound, "")
	})

	t.Run("missing invalid expired and revoked sessions are denied", func(t *testing.T) {
		path := "/api/v1/namespaces/" + primary.NamespaceName + "/repositories"
		assertRepositoryStatus(t, handler, http.MethodGet, path, "", "", http.StatusUnauthorized, "")
		assertRepositoryStatus(t, handler, http.MethodGet, path, "", "isolation-invalid-token", http.StatusUnauthorized, "")
		assertRepositoryStatus(t, handler, http.MethodGet, path, "", "isolation-expired-token", http.StatusUnauthorized, "")
		assertRepositoryStatus(t, handler, http.MethodGet, path, "", "isolation-revoked-token", http.StatusUnauthorized, "")
	})
}

func createIsolationSession(
	t *testing.T,
	ctx context.Context,
	store *authstore.Store,
	suffix byte,
	token string,
	userID auth.ID,
	expiresAt time.Time,
	revokedAt *time.Time,
	createdAt time.Time,
) {
	t.Helper()
	// Use deterministic UUIDs while keeping the token secret outside persistence.
	id := auth.ID(fmt.Sprintf("81818181-8181-4818-8181-8181818181%02x", suffix))
	if err := store.CreateSession(ctx, auth.Session{
		ID: id, UserID: userID, TokenDigest: auth.DigestSecret([]byte(token)),
		ExpiresAt: expiresAt, RevokedAt: revokedAt, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateSession(%s) error = %v", token, err)
	}
}

func assertRepositoryStatus(
	t *testing.T,
	handler http.Handler,
	method, target, body, token string,
	wantStatus int,
	wantBody string,
) {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: authhandler.SessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus || (wantBody != "" && !strings.Contains(recorder.Body.String(), wantBody)) {
		t.Fatalf("%s %s status/body = %d %s, want %d containing %q", method, target, recorder.Code, recorder.Body.String(), wantStatus, wantBody)
	}
}
