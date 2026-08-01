package artifacthandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/artifactstore"
	"hubcr.io/hubcr/internal/platform/postgres/authstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
	"hubcr.io/hubcr/migrations"
)

func TestArtifactHTTPReadFlowWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("HUBCR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUBCR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
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

	now := time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC)
	owner := artifactHTTPIdentity("e2e2e2e2-e2e2-42e2-82e2-e2e2e2e2e2e2", "artifact-http-owner", now)
	outsider := artifactHTTPIdentity("e3e3e3e3-e3e3-43e3-83e3-e3e3e3e3e3e3", "artifact-http-outsider", now)
	identityStore := authstore.New(pool.ORM())
	for _, identity := range []auth.Identity{owner, outsider} {
		if err := identityStore.CreateIdentity(ctx, identity); err != nil {
			t.Fatalf("CreateIdentity(%s) error = %v", identity.User.Username, err)
		}
	}

	repositoryStore := repositorystore.New(pool.ORM())
	privateRepository := createArtifactHTTPRepository(
		t, ctx, repositoryStore, string(owner.User.ID), "private-image", repositories.VisibilityPrivate, now,
	)
	publicRepository := createArtifactHTTPRepository(
		t, ctx, repositoryStore, string(owner.User.ID), "public-image", repositories.VisibilityPublic, now,
	)
	policy := authorization.NewPolicy()
	repositoryService, err := repositories.NewService(repositoryStore, policy, func() time.Time { return now })
	if err != nil {
		t.Fatalf("repositories.NewService() error = %v", err)
	}
	artifactStore := artifactstore.New(pool.ORM())
	writeService, err := artifacts.NewService(artifactStore)
	if err != nil {
		t.Fatalf("artifacts.NewService() error = %v", err)
	}
	queryService, err := artifacts.NewQueryService(artifactStore)
	if err != nil {
		t.Fatalf("artifacts.NewQueryService() error = %v", err)
	}

	manifestMediaType := "application/vnd.oci.image.manifest.v1+json"
	indexMediaType := "application/vnd.oci.image.index.v1+json"
	manifestSize := int64(256)
	indexSize := int64(512)
	latest := "latest"
	multi := "multi"
	privateManifestDigest := artifactHTTPDigest("a")
	privateChildDigest := artifactHTTPDigest("b")
	privateIndexDigest := artifactHTTPDigest("c")
	publicDigest := artifactHTTPDigest("d")
	for _, observation := range []artifacts.Observation{
		{
			RepositoryID: privateRepository.ID, Digest: privateManifestDigest,
			Kind: string(artifacts.KindManifest), MediaType: &manifestMediaType,
			SizeBytes: &manifestSize, Tag: &latest, ObservedAt: now,
		},
		{
			RepositoryID: privateRepository.ID, Digest: privateIndexDigest,
			Kind: string(artifacts.KindIndex), MediaType: &indexMediaType,
			SizeBytes: &indexSize, Tag: &multi, ObservedAt: now.Add(time.Second),
			Descriptors: &artifacts.DescriptorSetObservation{Items: []artifacts.DescriptorObservation{{
				Digest: privateChildDigest, MediaType: &manifestMediaType, SizeBytes: &manifestSize,
				Platform: &artifacts.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			}}},
		},
		{
			RepositoryID: publicRepository.ID, Digest: publicDigest,
			Kind: string(artifacts.KindManifest), MediaType: &manifestMediaType,
			SizeBytes: &manifestSize, Tag: &latest, ObservedAt: now,
		},
	} {
		if _, err := writeService.ReconcileArtifact(ctx, observation); err != nil {
			t.Fatalf("ReconcileArtifact(%s) error = %v", observation.Digest, err)
		}
	}

	handlerValue, err := New(
		artifactHTTPAuthenticator{users: map[string]auth.User{
			"owner-token": owner.User, "outsider-token": outsider.User,
		}},
		repositoryService,
		queryService,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	router := httpapi.NewRouter()
	RegisterRoutes(router, handlerValue)
	handler := httpapi.WithRequestID(httpapi.Recover(router))
	privateBase := "/api/v1/namespaces/" + owner.PersonalNamespace.Name +
		"/repositories/private-image"
	publicBase := "/api/v1/namespaces/" + owner.PersonalNamespace.Name +
		"/repositories/public-image"

	ownerIndex := artifactHTTPRequest(handler, privateBase+"/artifacts/"+privateIndexDigest, "owner-token")
	if ownerIndex.Code != http.StatusOK ||
		!strings.Contains(ownerIndex.Body.String(), `"manifests":[`) ||
		!strings.Contains(ownerIndex.Body.String(), `"digest":"`+privateChildDigest+`"`) ||
		!strings.Contains(ownerIndex.Body.String(), `"architecture":"arm64"`) ||
		!strings.Contains(ownerIndex.Body.String(), `"size_bytes":256`) {
		t.Fatalf("owner index status/body = %d %s", ownerIndex.Code, ownerIndex.Body.String())
	}

	ownerTag := artifactHTTPRequest(handler, privateBase+"/tags/multi", "owner-token")
	if ownerTag.Code != http.StatusOK ||
		!strings.Contains(ownerTag.Body.String(), `"name":"multi"`) ||
		!strings.Contains(ownerTag.Body.String(), `"artifact":{`) {
		t.Fatalf("owner tag status/body = %d %s", ownerTag.Code, ownerTag.Body.String())
	}

	firstPage := artifactHTTPRequest(handler, privateBase+"/artifacts?limit=1", "owner-token")
	if firstPage.Code != http.StatusOK {
		t.Fatalf("first artifact page status/body = %d %s", firstPage.Code, firstPage.Body.String())
	}
	var page artifactListResponse
	if err := json.Unmarshal(firstPage.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Meta.NextCursor == "" {
		t.Fatalf("first artifact page = %#v, error = %v", page, err)
	}
	secondPage := artifactHTTPRequest(
		handler, privateBase+"/artifacts?limit=1&cursor="+page.Meta.NextCursor, "owner-token",
	)
	if secondPage.Code != http.StatusOK || strings.Contains(secondPage.Body.String(), `"digest":"`+page.Items[0].Digest+`"`) {
		t.Fatalf("second artifact page status/body = %d %s", secondPage.Code, secondPage.Body.String())
	}

	privateDenied := artifactHTTPRequest(handler, privateBase+"/tags", "outsider-token")
	if privateDenied.Code != http.StatusNotFound {
		t.Fatalf("private outsider status/body = %d %s", privateDenied.Code, privateDenied.Body.String())
	}
	publicAllowed := artifactHTTPRequest(handler, publicBase+"/tags/latest", "outsider-token")
	if publicAllowed.Code != http.StatusOK || !strings.Contains(publicAllowed.Body.String(), publicDigest) {
		t.Fatalf("public outsider status/body = %d %s", publicAllowed.Code, publicAllowed.Body.String())
	}
}

func createArtifactHTTPRepository(
	t *testing.T,
	ctx context.Context,
	store *repositorystore.Store,
	namespaceID, name string,
	visibility repositories.Visibility,
	now time.Time,
) repositories.Repository {
	t.Helper()
	repository, err := repositories.New(repositories.NewRepository{
		NamespaceID: namespaceID, RequestedName: name,
		Visibility: visibility, CreatedByUserID: namespaceID,
	}, now)
	if err != nil {
		t.Fatalf("repositories.New(%s) error = %v", name, err)
	}
	if err := store.Create(ctx, repository); err != nil {
		t.Fatalf("repository Store.Create(%s) error = %v", name, err)
	}
	return repository
}

func artifactHTTPIdentity(id auth.ID, username string, now time.Time) auth.Identity {
	return auth.Identity{
		User: auth.User{
			ID: id, Username: username, PersonalNamespace: username,
			CreatedAt: now, UpdatedAt: now,
		},
		Credential: auth.LocalCredential{
			UserID: id, PasswordHash: "test-hash", PasswordChangedAt: now,
			CreatedAt: now, UpdatedAt: now,
		},
		PersonalNamespace: auth.PersonalNamespace{ID: id, Name: username},
	}
}

func artifactHTTPDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

func artifactHTTPRequest(handler http.Handler, target, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.AddCookie(&http.Cookie{Name: authhandler.SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

type artifactHTTPAuthenticator struct{ users map[string]auth.User }

func (a artifactHTTPAuthenticator) Authenticate(_ context.Context, token string) (auth.User, error) {
	user, exists := a.users[token]
	if !exists {
		return auth.User{}, auth.ErrUnauthenticated
	}
	return user, nil
}
