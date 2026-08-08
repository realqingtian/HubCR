package repositoryhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

const (
	testUserID       = "51515151-5151-4515-8515-515151515151"
	testRepositoryID = "52525252-5252-4525-8525-525252525252"
)

func TestRepositoryCreateListDetailAndUpdateRoutes(t *testing.T) {
	now := time.Date(2026, 8, 1, 22, 30, 0, 0, time.UTC)
	service := &handlerTestRepositories{repository: repositories.Repository{
		ID: testRepositoryID, NamespaceID: "53535353-5353-4535-8535-535353535353",
		Name: "backend", Visibility: repositories.VisibilityPrivate, Description: "images",
		CreatedByUserID: testUserID, VisibilityUpdatedByUserID: testUserID,
		VisibilityUpdatedAt: now, CreatedAt: now, UpdatedAt: now,
	}}
	handler := testRepositoryHandler(service)

	create := authenticatedRequest(
		http.MethodPost, "/api/v1/namespaces/Platform-Team/repositories",
		`{"name":"Backend","visibility":"PRIVATE","description":"images"}`,
	)
	create.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusCreated ||
		!strings.Contains(createRecorder.Body.String(), `"namespace":"platform-team"`) ||
		service.createdVisibility != repositories.VisibilityPrivate {
		t.Fatalf("create status/body/visibility = %d %s %q", createRecorder.Code, createRecorder.Body.String(), service.createdVisibility)
	}

	service.page = repositories.RepositoryPage{
		Items: []repositories.Repository{service.repository}, NextAfter: testRepositoryID,
	}
	list := authenticatedRequest(http.MethodGet, "/api/v1/namespaces/platform-team/repositories?limit=1", "")
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"next_cursor"`) {
		t.Fatalf("list status/body = %d %s", listRecorder.Code, listRecorder.Body.String())
	}

	detail := authenticatedRequest(http.MethodGet, "/api/v1/namespaces/platform-team/repositories/backend", "")
	detailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(detailRecorder, detail)
	if detailRecorder.Code != http.StatusOK || service.detailName != "backend" ||
		!strings.Contains(detailRecorder.Body.String(), `"capabilities":{"can_pull":true,"can_push":true}`) {
		t.Fatalf("detail status/name/body = %d %q %s", detailRecorder.Code, service.detailName, detailRecorder.Body.String())
	}

	update := authenticatedRequest(
		http.MethodPatch, "/api/v1/namespaces/platform-team/repositories/backend",
		`{"visibility":"PUBLIC","description":"public images"}`,
	)
	update.Header.Set("Content-Type", "application/json")
	updateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(updateRecorder, update)
	if updateRecorder.Code != http.StatusOK || service.update.Visibility == nil ||
		*service.update.Visibility != repositories.VisibilityPublic || service.update.Description == nil {
		t.Fatalf("update status/input/body = %d %#v %s", updateRecorder.Code, service.update, updateRecorder.Body.String())
	}
}

func TestRepositoryAuthenticationValidationAndPolicyErrors(t *testing.T) {
	service := &handlerTestRepositories{}
	handler := testRepositoryHandler(service)

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/platform/repositories", nil)
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d", missingRecorder.Code)
	}

	invalidCreate := authenticatedRequest(
		http.MethodPost, "/api/v1/namespaces/platform/repositories",
		`{"name":"bad/name","visibility":"","description":""}`,
	)
	invalidCreate.Header.Set("Content-Type", "application/json")
	invalidRecorder := httptest.NewRecorder()
	handler.ServeHTTP(invalidRecorder, invalidCreate)
	if invalidRecorder.Code != http.StatusUnprocessableEntity || service.createCalls != 0 {
		t.Fatalf("invalid create status/calls = %d %d", invalidRecorder.Code, service.createCalls)
	}

	emptyUpdate := authenticatedRequest(
		http.MethodPatch, "/api/v1/namespaces/platform/repositories/backend", `{}`,
	)
	emptyUpdate.Header.Set("Content-Type", "application/json")
	emptyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(emptyRecorder, emptyUpdate)
	if emptyRecorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty update status/body = %d %s", emptyRecorder.Code, emptyRecorder.Body.String())
	}

	badCursor := authenticatedRequest(http.MethodGet, "/api/v1/namespaces/platform/repositories?cursor=bad", "")
	badCursorRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badCursorRecorder, badCursor)
	if badCursorRecorder.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor status = %d", badCursorRecorder.Code)
	}

	service.serviceError = repositories.ErrForbidden
	forbidden := authenticatedRequest(
		http.MethodPatch, "/api/v1/namespaces/platform/repositories/backend", `{"description":"denied"}`,
	)
	forbidden.Header.Set("Content-Type", "application/json")
	forbiddenRecorder := httptest.NewRecorder()
	handler.ServeHTTP(forbiddenRecorder, forbidden)
	if forbiddenRecorder.Code != http.StatusForbidden || !strings.Contains(forbiddenRecorder.Body.String(), httpapi.CodeForbidden) {
		t.Fatalf("forbidden status/body = %d %s", forbiddenRecorder.Code, forbiddenRecorder.Body.String())
	}

	service.serviceError = repositories.ErrNotFound
	private := authenticatedRequest(http.MethodGet, "/api/v1/namespaces/platform/repositories/private", "")
	privateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(privateRecorder, private)
	if privateRecorder.Code != http.StatusNotFound {
		t.Fatalf("private discovery status = %d", privateRecorder.Code)
	}
}

func TestRepositoryMutationsRejectCrossSiteRequests(t *testing.T) {
	service := &handlerTestRepositories{}
	handler := testRepositoryHandler(service)
	request := authenticatedRequest(
		http.MethodPost, "/api/v1/namespaces/platform/repositories",
		`{"name":"backend","visibility":"PRIVATE"}`,
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || service.createCalls != 0 {
		t.Fatalf("cross-site status/calls = %d %d", recorder.Code, service.createCalls)
	}
}

func testRepositoryHandler(service *handlerTestRepositories) http.Handler {
	router := httpapi.NewRouter()
	RegisterRoutes(router, New(handlerTestAuthenticator{}, service))
	return httpapi.WithRequestID(httpapi.Recover(router))
}

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: authhandler.SessionCookieName, Value: "token"})
	return request
}

type handlerTestAuthenticator struct{}

func (handlerTestAuthenticator) Authenticate(context.Context, string) (auth.User, error) {
	return auth.User{ID: testUserID, Username: "owner"}, nil
}

type handlerTestRepositories struct {
	repository        repositories.Repository
	page              repositories.RepositoryPage
	serviceError      error
	createdVisibility repositories.Visibility
	createCalls       int
	detailName        string
	update            repositories.UpdateRepository
}

func (s *handlerTestRepositories) Create(
	_ context.Context, _, _, _ string, visibility repositories.Visibility, _ string,
) (repositories.Repository, error) {
	s.createCalls++
	s.createdVisibility = visibility
	return s.repository, s.serviceError
}
func (s *handlerTestRepositories) List(
	context.Context, string, string, repositories.PageRequest,
) (repositories.RepositoryPage, error) {
	return s.page, s.serviceError
}
func (s *handlerTestRepositories) Detail(
	_ context.Context, _, _, repositoryName string,
) (repositories.Repository, error) {
	s.detailName = repositoryName
	return s.repository, s.serviceError
}
func (s *handlerTestRepositories) DetailWithCapabilities(
	_ context.Context, _, _, repositoryName string,
) (repositories.RepositoryDetail, error) {
	s.detailName = repositoryName
	return repositories.RepositoryDetail{
		Repository: s.repository,
		Capabilities: repositories.RepositoryCapabilities{
			CanPull: true,
			CanPush: true,
		},
	}, s.serviceError
}
func (s *handlerTestRepositories) Update(
	_ context.Context, _, _, _ string, update repositories.UpdateRepository,
) (repositories.Repository, error) {
	s.update = update
	return s.repository, s.serviceError
}
