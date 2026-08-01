package artifacthandler

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

const (
	testUserID       = "71717171-7171-4717-8717-717171717171"
	testRepositoryID = "72727272-7272-4727-8727-727272727272"
	testDigest       = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testChildDigest  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestArtifactAndTagRoutesReturnRepositoryScopedMetadata(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 123000000, time.UTC)
	mediaType := "application/vnd.oci.image.index.v1+json"
	childMediaType := "application/vnd.oci.image.manifest.v1+json"
	size := int64(512)
	childSize := int64(256)
	createdAt := now.Add(-time.Hour)
	artifact := artifacts.Artifact{
		ID: "artifact-id", RepositoryID: testRepositoryID, Digest: artifacts.Digest(testDigest),
		Kind: artifacts.KindIndex, MediaType: &mediaType, SizeBytes: &size,
		SourceCreatedAt: &createdAt, DescriptorsComplete: true,
		DiscoveredAt: now, UpdatedAt: now,
	}
	tag := artifacts.Tag{
		RepositoryID: testRepositoryID, Name: artifacts.TagName("latest"), ArtifactID: "artifact-id",
		Digest: artifacts.Digest(testDigest), CreatedAt: now, UpdatedAt: now,
	}
	service := &handlerArtifacts{
		artifactPage: artifacts.ArtifactPage{Items: []artifacts.Artifact{artifact}, NextAfter: testDigest},
		tagPage:      artifacts.TagPage{Items: []artifacts.Tag{tag}, NextAfter: "latest"},
		tag:          tag,
		snapshot: artifacts.Snapshot{Artifact: artifact, Descriptors: []artifacts.ManifestDescriptor{{
			Position: 0, ChildArtifactID: "child-id", Digest: artifacts.Digest(testChildDigest),
			MediaType: &childMediaType, SizeBytes: &childSize,
			Platform: &artifacts.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
		}},
		},
	}
	repositories := &handlerRepositories{repository: repositories.Repository{ID: testRepositoryID}}
	handler := testHandler(t, repositories, service)

	tests := []struct {
		name         string
		path         string
		wantContains []string
	}{
		{
			name: "artifact list", path: "/api/v1/namespaces/team/repositories/backend/artifacts?limit=1",
			wantContains: []string{`"digest":"` + testDigest + `"`, `"kind":"INDEX"`, `"next_cursor"`},
		},
		{
			name: "artifact detail", path: "/api/v1/namespaces/team/repositories/backend/artifacts/" + testDigest,
			wantContains: []string{`"manifests":[`, `"digest":"` + testChildDigest + `"`, `"architecture":"arm64"`, `"variant":"v8"`},
		},
		{
			name: "tag list", path: "/api/v1/namespaces/team/repositories/backend/tags?limit=1",
			wantContains: []string{`"name":"latest"`, `"digest":"` + testDigest + `"`, `"next_cursor"`},
		},
		{
			name: "tag detail", path: "/api/v1/namespaces/team/repositories/backend/tags/latest",
			wantContains: []string{`"name":"latest"`, `"artifact":{`, `"media_type":"` + mediaType + `"`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := authenticatedRequest(test.path)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			for _, value := range test.wantContains {
				if !strings.Contains(recorder.Body.String(), value) {
					t.Fatalf("body = %s, want %q", recorder.Body.String(), value)
				}
			}
		})
	}
	if repositories.actorUserID != testUserID || repositories.namespace != "team" ||
		repositories.repositoryName != "backend" || service.repositoryID != testRepositoryID {
		t.Fatalf("repository scope = %#v / %#v", repositories, service)
	}
}

func TestArtifactRoutesAuthenticateAndHidePrivateRepositoryExistence(t *testing.T) {
	repositoryService := &handlerRepositories{repository: repositories.Repository{ID: testRepositoryID}}
	artifactService := &handlerArtifacts{}
	handler := testHandler(t, repositoryService, artifactService)

	missingSession := httptest.NewRequest(
		http.MethodGet, "/api/v1/namespaces/team/repositories/private/artifacts", nil,
	)
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missingSession)
	if missingRecorder.Code != http.StatusUnauthorized || repositoryService.calls != 0 {
		t.Fatalf("missing session status/repository calls = %d %d", missingRecorder.Code, repositoryService.calls)
	}

	repositoryService.err = repositories.ErrNotFound
	privateRequest := authenticatedRequest("/api/v1/namespaces/team/repositories/private/tags")
	privateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(privateRecorder, privateRequest)
	if privateRecorder.Code != http.StatusNotFound || artifactService.calls != 0 {
		t.Fatalf("private status/artifact calls/body = %d %d %s", privateRecorder.Code, artifactService.calls, privateRecorder.Body.String())
	}
}

func TestArtifactRoutesRejectInvalidPathsAndCursors(t *testing.T) {
	repositoryService := &handlerRepositories{repository: repositories.Repository{ID: testRepositoryID}}
	artifactService := &handlerArtifacts{}
	handler := testHandler(t, repositoryService, artifactService)

	badDigest := authenticatedRequest(
		"/api/v1/namespaces/team/repositories/backend/artifacts/not-a-digest",
	)
	badDigestRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badDigestRecorder, badDigest)
	if badDigestRecorder.Code != http.StatusBadRequest || artifactService.calls != 0 {
		t.Fatalf("bad digest status/calls = %d %d", badDigestRecorder.Code, artifactService.calls)
	}

	badTag := authenticatedRequest("/api/v1/namespaces/team/repositories/backend/tags/bad%2Ftag")
	badTagRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badTagRecorder, badTag)
	if badTagRecorder.Code != http.StatusBadRequest || artifactService.calls != 0 {
		t.Fatalf("bad tag status/calls = %d %d", badTagRecorder.Code, artifactService.calls)
	}

	badCursor := base64.RawURLEncoding.EncodeToString([]byte("not-a-digest"))
	cursorRequest := authenticatedRequest(
		"/api/v1/namespaces/team/repositories/backend/artifacts?cursor=" + badCursor,
	)
	cursorRecorder := httptest.NewRecorder()
	handler.ServeHTTP(cursorRecorder, cursorRequest)
	if cursorRecorder.Code != http.StatusBadRequest || artifactService.calls != 0 {
		t.Fatalf("bad cursor status/calls = %d %d", cursorRecorder.Code, artifactService.calls)
	}
}

func TestArtifactRoutesMapNotFoundAndHidePersistenceErrors(t *testing.T) {
	repositoryService := &handlerRepositories{repository: repositories.Repository{ID: testRepositoryID}}
	artifactService := &handlerArtifacts{err: artifacts.ErrNotFound}
	handler := testHandler(t, repositoryService, artifactService)

	notFound := authenticatedRequest(
		"/api/v1/namespaces/team/repositories/backend/artifacts/" + testDigest,
	)
	notFoundRecorder := httptest.NewRecorder()
	handler.ServeHTTP(notFoundRecorder, notFound)
	if notFoundRecorder.Code != http.StatusNotFound {
		t.Fatalf("not found status/body = %d %s", notFoundRecorder.Code, notFoundRecorder.Body.String())
	}

	artifactService.err = errors.New("database password leaked")
	failed := authenticatedRequest("/api/v1/namespaces/team/repositories/backend/tags")
	failedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(failedRecorder, failed)
	if failedRecorder.Code != http.StatusInternalServerError ||
		strings.Contains(failedRecorder.Body.String(), "database password") {
		t.Fatalf("failure status/body = %d %s", failedRecorder.Code, failedRecorder.Body.String())
	}
}

func TestArtifactDetailDistinguishesUnknownAndConfirmedEmptyDescriptors(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	repositoryService := &handlerRepositories{repository: repositories.Repository{ID: testRepositoryID}}
	artifactService := &handlerArtifacts{snapshot: artifacts.Snapshot{Artifact: artifacts.Artifact{
		ID: "artifact-id", RepositoryID: testRepositoryID, Digest: artifacts.Digest(testDigest),
		Kind: artifacts.KindIndex, DiscoveredAt: now, UpdatedAt: now,
	}}}
	handler := testHandler(t, repositoryService, artifactService)
	path := "/api/v1/namespaces/team/repositories/backend/artifacts/" + testDigest

	unknown := authenticatedRequest(path)
	unknownRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unknownRecorder, unknown)
	if unknownRecorder.Code != http.StatusOK || strings.Contains(unknownRecorder.Body.String(), `"manifests"`) {
		t.Fatalf("unknown descriptor response = %d %s", unknownRecorder.Code, unknownRecorder.Body.String())
	}

	artifactService.snapshot.Artifact.DescriptorsComplete = true
	artifactService.snapshot.Descriptors = []artifacts.ManifestDescriptor{}
	knownEmpty := authenticatedRequest(path)
	knownEmptyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(knownEmptyRecorder, knownEmpty)
	if knownEmptyRecorder.Code != http.StatusOK || !strings.Contains(knownEmptyRecorder.Body.String(), `"manifests":[]`) {
		t.Fatalf("known empty descriptor response = %d %s", knownEmptyRecorder.Code, knownEmptyRecorder.Body.String())
	}
}

func TestArtifactHandlerConstructorAndMethodContract(t *testing.T) {
	if _, err := New(nil, &handlerRepositories{}, &handlerArtifacts{}); err == nil {
		t.Fatal("New() error = nil, want dependency error")
	}
	handler := testHandler(t, &handlerRepositories{}, &handlerArtifacts{})
	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/namespaces/team/repositories/backend/artifacts", nil,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("method status/allow = %d %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func testHandler(
	t *testing.T,
	repositoryService *handlerRepositories,
	artifactService *handlerArtifacts,
) http.Handler {
	t.Helper()
	handler, err := New(handlerAuthenticator{}, repositoryService, artifactService)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	router := httpapi.NewRouter()
	RegisterRoutes(router, handler)
	return httpapi.WithRequestID(httpapi.Recover(router))
}

func authenticatedRequest(target string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.AddCookie(&http.Cookie{Name: authhandler.SessionCookieName, Value: "session-token"})
	return request
}

type handlerAuthenticator struct{}

func (handlerAuthenticator) Authenticate(context.Context, string) (auth.User, error) {
	return auth.User{ID: testUserID, Username: "owner"}, nil
}

type handlerRepositories struct {
	repository     repositories.Repository
	err            error
	calls          int
	actorUserID    string
	namespace      string
	repositoryName string
}

func (s *handlerRepositories) Detail(
	_ context.Context,
	actorUserID, namespace, repositoryName string,
) (repositories.Repository, error) {
	s.calls++
	s.actorUserID = actorUserID
	s.namespace = namespace
	s.repositoryName = repositoryName
	return s.repository, s.err
}

type handlerArtifacts struct {
	snapshot     artifacts.Snapshot
	tag          artifacts.Tag
	artifactPage artifacts.ArtifactPage
	tagPage      artifacts.TagPage
	err          error
	calls        int
	repositoryID string
}

func (s *handlerArtifacts) ArtifactDetail(
	_ context.Context, repositoryID, _ string,
) (artifacts.Snapshot, error) {
	s.calls++
	s.repositoryID = repositoryID
	return s.snapshot, s.err
}

func (s *handlerArtifacts) TagDetail(
	_ context.Context, repositoryID, _ string,
) (artifacts.Tag, error) {
	s.calls++
	s.repositoryID = repositoryID
	return s.tag, s.err
}

func (s *handlerArtifacts) ListArtifacts(
	_ context.Context, repositoryID string, _ int, _ string,
) (artifacts.ArtifactPage, error) {
	s.calls++
	s.repositoryID = repositoryID
	return s.artifactPage, s.err
}

func (s *handlerArtifacts) ListTags(
	_ context.Context, repositoryID string, _ int, _ string,
) (artifacts.TagPage, error) {
	s.calls++
	s.repositoryID = repositoryID
	return s.tagPage, s.err
}
