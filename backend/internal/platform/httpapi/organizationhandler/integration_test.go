package organizationhandler

import (
	"bytes"
	"context"
	"encoding/json"
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
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/organizationstore"
	"hubcr.io/hubcr/migrations"
)

func TestOrganizationHTTPFlowWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{URL: databaseURL, ConnectTimeout: 3 * time.Second, MaxConnections: 4})
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool.ORM()); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}

	now := time.Date(2026, 8, 1, 20, 0, 0, 0, time.UTC)
	hasher := auth.NewPasswordHasher()
	passwordHash, err := hasher.Hash([]byte("organization integration password"))
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	owner := httpFlowIdentity("34343434-3434-4434-8434-343434343434", "http-owner", passwordHash, now)
	member := httpFlowIdentity("45454545-4545-4454-8454-454545454545", "http-member", passwordHash, now)
	identityStore := authstore.New(pool.ORM())
	for _, identity := range []auth.Identity{owner, member} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}
	authService, err := auth.NewService(identityStore, hasher, auth.ServiceOptions{
		SessionTTL: time.Hour, Random: bytes.NewReader(bytes.Repeat([]byte{8}, 32)),
		Clock: func() time.Time { return now }, Limiter: auth.AllowAllLoginLimiter{},
	})
	if err != nil {
		t.Fatalf("auth.NewService() error = %v", err)
	}
	organizationService, err := organizations.NewService(
		organizationstore.New(pool.ORM()), namespaces.NormalizeName, func() time.Time { return now }, authorization.NewPolicy(),
	)
	if err != nil {
		t.Fatalf("organizations.NewService() error = %v", err)
	}
	router := httpapi.NewRouter()
	authhandler.RegisterRoutes(router, authhandler.New(authService, false))
	RegisterRoutes(router, New(authService, organizationService))
	handler := httpapi.WithRequestID(httpapi.Recover(router))

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
		`{"username":"http-owner","password":"organization integration password"}`,
	))
	login.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK || len(loginRecorder.Result().Cookies()) != 1 {
		t.Fatalf("login status/body = %d %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	sessionCookie := loginRecorder.Result().Cookies()[0]

	create := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", strings.NewReader(
		`{"name":"HTTP-Team","description":"integration"}`,
	))
	create.Header.Set("Content-Type", "application/json")
	create.AddCookie(sessionCookie)
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status/body = %d %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("decode created organization: %#v, %v", created, err)
	}

	add := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+created.ID+"/members", strings.NewReader(
		`{"user_id":"45454545-4545-4454-8454-454545454545","role":"READER"}`,
	))
	add.Header.Set("Content-Type", "application/json")
	add.AddCookie(sessionCookie)
	addRecorder := httptest.NewRecorder()
	handler.ServeHTTP(addRecorder, add)
	if addRecorder.Code != http.StatusNoContent {
		t.Fatalf("add member status/body = %d %s", addRecorder.Code, addRecorder.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+created.ID+"/members?limit=1", nil)
	list.AddCookie(sessionCookie)
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"next_cursor"`) {
		t.Fatalf("member list status/body = %d %s", listRecorder.Code, listRecorder.Body.String())
	}
}

func httpFlowIdentity(id auth.ID, username, passwordHash string, now time.Time) auth.Identity {
	return auth.Identity{
		User: auth.User{ID: id, Username: username, CreatedAt: now, UpdatedAt: now},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: passwordHash, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: username},
	}
}
