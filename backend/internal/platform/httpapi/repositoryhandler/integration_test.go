package repositoryhandler

import (
	"context"
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

func TestRepositoryHTTPAuthorizationAndVisibilityFlowWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 6,
	})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)
	owner := repositoryHTTPIdentity("61616161-6161-4616-8616-616161616161", "repository-http-owner", now)
	writer := repositoryHTTPIdentity("62626262-6262-4626-8626-626262626262", "repository-http-writer", now)
	outsider := repositoryHTTPIdentity("63636363-6363-4636-8636-636363636363", "repository-http-outsider", now)
	identityStore := authstore.New(pool.ORM())
	for _, identity := range []auth.Identity{owner, writer, outsider} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}

	policy := authorization.NewPolicy()
	organizationService, err := organizations.NewService(
		organizationstore.New(pool.ORM()), namespaces.NormalizeName, func() time.Time { return now }, policy,
	)
	if err != nil {
		t.Fatalf("organizations.NewService() error = %v", err)
	}
	organization, err := organizationService.Create(
		ctx, string(owner.User.ID), "Repository-HTTP-Team", "integration repositories",
	)
	if err != nil {
		t.Fatalf("organizationService.Create() error = %v", err)
	}
	if err := organizationService.AddMember(
		ctx, organization.ID, string(owner.User.ID), string(writer.User.ID), organizations.RoleWriter,
	); err != nil {
		t.Fatalf("organizationService.AddMember(writer) error = %v", err)
	}
	repositoryService, err := repositories.NewService(
		repositorystore.New(pool.ORM()), policy, func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("repositories.NewService() error = %v", err)
	}
	router := httpapi.NewRouter()
	authenticator := repositoryHTTPAuthenticator{users: map[string]auth.User{
		"owner-token": owner.User, "writer-token": writer.User, "outsider-token": outsider.User,
	}}
	RegisterRoutes(router, New(authenticator, repositoryService))
	handler := httpapi.WithRequestID(httpapi.Recover(router))

	ownerCookie := &http.Cookie{Name: authhandler.SessionCookieName, Value: "owner-token"}
	writerCookie := &http.Cookie{Name: authhandler.SessionCookieName, Value: "writer-token"}
	outsiderCookie := &http.Cookie{Name: authhandler.SessionCookieName, Value: "outsider-token"}
	basePath := "/api/v1/namespaces/" + organization.NamespaceName + "/repositories"

	create := repositoryRequest(
		http.MethodPost, basePath, `{"name":"Backend","visibility":"PRIVATE","description":"private images"}`, writerCookie,
	)
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusCreated || !strings.Contains(createRecorder.Body.String(), `"visibility":"PRIVATE"`) {
		t.Fatalf("writer create status/body = %d %s", createRecorder.Code, createRecorder.Body.String())
	}

	privateList := repositoryRequest(http.MethodGet, basePath, "", outsiderCookie)
	privateListRecorder := httptest.NewRecorder()
	handler.ServeHTTP(privateListRecorder, privateList)
	if privateListRecorder.Code != http.StatusOK || !strings.Contains(privateListRecorder.Body.String(), `"items":[]`) {
		t.Fatalf("outsider private list status/body = %d %s", privateListRecorder.Code, privateListRecorder.Body.String())
	}
	privateDetail := repositoryRequest(http.MethodGet, basePath+"/backend", "", outsiderCookie)
	privateDetailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(privateDetailRecorder, privateDetail)
	if privateDetailRecorder.Code != http.StatusNotFound {
		t.Fatalf("outsider private detail status/body = %d %s", privateDetailRecorder.Code, privateDetailRecorder.Body.String())
	}
	writerDetail := repositoryRequest(http.MethodGet, basePath+"/backend", "", writerCookie)
	writerDetailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(writerDetailRecorder, writerDetail)
	if writerDetailRecorder.Code != http.StatusOK ||
		!strings.Contains(writerDetailRecorder.Body.String(), `"capabilities":{"can_pull":true,"can_push":true}`) {
		t.Fatalf("writer private detail status/body = %d %s", writerDetailRecorder.Code, writerDetailRecorder.Body.String())
	}

	writerDescription := repositoryRequest(
		http.MethodPatch, basePath+"/backend", `{"description":"writer updated"}`, writerCookie,
	)
	writerDescriptionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(writerDescriptionRecorder, writerDescription)
	if writerDescriptionRecorder.Code != http.StatusOK || !strings.Contains(writerDescriptionRecorder.Body.String(), `"description":"writer updated"`) {
		t.Fatalf("writer description status/body = %d %s", writerDescriptionRecorder.Code, writerDescriptionRecorder.Body.String())
	}
	writerVisibility := repositoryRequest(
		http.MethodPatch, basePath+"/backend", `{"visibility":"PUBLIC"}`, writerCookie,
	)
	writerVisibilityRecorder := httptest.NewRecorder()
	handler.ServeHTTP(writerVisibilityRecorder, writerVisibility)
	if writerVisibilityRecorder.Code != http.StatusForbidden {
		t.Fatalf("writer visibility status/body = %d %s", writerVisibilityRecorder.Code, writerVisibilityRecorder.Body.String())
	}

	ownerVisibility := repositoryRequest(
		http.MethodPatch, basePath+"/backend", `{"visibility":"PUBLIC"}`, ownerCookie,
	)
	ownerVisibilityRecorder := httptest.NewRecorder()
	handler.ServeHTTP(ownerVisibilityRecorder, ownerVisibility)
	if ownerVisibilityRecorder.Code != http.StatusOK ||
		!strings.Contains(ownerVisibilityRecorder.Body.String(), `"visibility":"PUBLIC"`) ||
		!strings.Contains(ownerVisibilityRecorder.Body.String(), `"visibility_updated_by_user_id":"`+string(owner.User.ID)+`"`) {
		t.Fatalf("owner visibility status/body = %d %s", ownerVisibilityRecorder.Code, ownerVisibilityRecorder.Body.String())
	}

	publicList := repositoryRequest(http.MethodGet, basePath, "", outsiderCookie)
	publicListRecorder := httptest.NewRecorder()
	handler.ServeHTTP(publicListRecorder, publicList)
	if publicListRecorder.Code != http.StatusOK || !strings.Contains(publicListRecorder.Body.String(), `"name":"backend"`) {
		t.Fatalf("outsider public list status/body = %d %s", publicListRecorder.Code, publicListRecorder.Body.String())
	}
	publicDetail := repositoryRequest(http.MethodGet, basePath+"/backend", "", outsiderCookie)
	publicDetailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(publicDetailRecorder, publicDetail)
	if publicDetailRecorder.Code != http.StatusOK ||
		!strings.Contains(publicDetailRecorder.Body.String(), `"capabilities":{"can_pull":true,"can_push":false}`) {
		t.Fatalf("outsider public detail status/body = %d %s", publicDetailRecorder.Code, publicDetailRecorder.Body.String())
	}
	outsiderUpdate := repositoryRequest(
		http.MethodPatch, basePath+"/backend", `{"description":"forbidden"}`, outsiderCookie,
	)
	outsiderUpdateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(outsiderUpdateRecorder, outsiderUpdate)
	if outsiderUpdateRecorder.Code != http.StatusForbidden {
		t.Fatalf("outsider update status/body = %d %s", outsiderUpdateRecorder.Code, outsiderUpdateRecorder.Body.String())
	}
}

func repositoryRequest(method, target, body string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(cookie)
	return request
}

type repositoryHTTPAuthenticator struct {
	users map[string]auth.User
}

func (a repositoryHTTPAuthenticator) Authenticate(_ context.Context, token string) (auth.User, error) {
	user, exists := a.users[token]
	if !exists {
		return auth.User{}, auth.ErrUnauthenticated
	}
	return user, nil
}

func repositoryHTTPIdentity(id auth.ID, username string, now time.Time) auth.Identity {
	return auth.Identity{
		User: auth.User{ID: id, Username: username, CreatedAt: now, UpdatedAt: now},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: "test-hash", PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: username},
	}
}
