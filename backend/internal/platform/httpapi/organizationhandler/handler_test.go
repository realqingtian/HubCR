package organizationhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

const (
	testUserID = "11111111-1111-4111-8111-111111111111"
	testOrgID  = "22222222-2222-4222-8222-222222222222"
)

func TestOrganizationCreateListAndDynamicDetail(t *testing.T) {
	now := time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC)
	service := &handlerTestOrganizations{organization: organizations.Organization{
		ID: testOrgID, NamespaceID: "33333333-3333-4333-8333-333333333333",
		NamespaceName: "platform-team", Description: "images", CreatedByUserID: testUserID,
		CreatedAt: now, UpdatedAt: now,
	}}
	handler := testOrganizationHandler(service)

	create := authenticatedRequest(http.MethodPost, "/api/v1/organizations", `{"name":"Platform-Team","description":"images"}`)
	create.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusCreated || !strings.Contains(createRecorder.Body.String(), `"namespace":"platform-team"`) {
		t.Fatalf("create status/body = %d %s", createRecorder.Code, createRecorder.Body.String())
	}

	service.organizations = []organizations.Organization{service.organization}
	list := authenticatedRequest(http.MethodGet, "/api/v1/organizations?limit=1", "")
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"limit":1`) {
		t.Fatalf("list status/body = %d %s", listRecorder.Code, listRecorder.Body.String())
	}

	detail := authenticatedRequest(http.MethodGet, "/api/v1/organizations/"+testOrgID, "")
	detailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(detailRecorder, detail)
	if detailRecorder.Code != http.StatusOK || service.detailOrganizationID != testOrgID {
		t.Fatalf("detail status/id = %d %q, body %s", detailRecorder.Code, service.detailOrganizationID, detailRecorder.Body.String())
	}
}

func TestOrganizationAuthenticationValidationAndAuthorizationErrors(t *testing.T) {
	service := &handlerTestOrganizations{}
	handler := testOrganizationHandler(service)

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d", missingRecorder.Code)
	}

	badCursor := authenticatedRequest(http.MethodGet, "/api/v1/organizations?cursor=bad", "")
	badCursorRecorder := httptest.NewRecorder()
	handler.ServeHTTP(badCursorRecorder, badCursor)
	if badCursorRecorder.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor status = %d", badCursorRecorder.Code)
	}

	service.serviceError = organizations.ErrForbidden
	detail := authenticatedRequest(http.MethodGet, "/api/v1/organizations/"+testOrgID, "")
	detailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(detailRecorder, detail)
	if detailRecorder.Code != http.StatusForbidden || !strings.Contains(detailRecorder.Body.String(), httpapi.CodeForbidden) {
		t.Fatalf("forbidden status/body = %d %s", detailRecorder.Code, detailRecorder.Body.String())
	}
}

func TestMemberMutationRoutesAndCrossSiteProtection(t *testing.T) {
	service := &handlerTestOrganizations{}
	handler := testOrganizationHandler(service)
	add := authenticatedRequest(
		http.MethodPost,
		"/api/v1/organizations/"+testOrgID+"/members",
		`{"user_id":"33333333-3333-4333-8333-333333333333","role":"WRITER"}`,
	)
	add.Header.Set("Content-Type", "application/json")
	addRecorder := httptest.NewRecorder()
	handler.ServeHTTP(addRecorder, add)
	if addRecorder.Code != http.StatusNoContent || service.addedRole != organizations.RoleWriter {
		t.Fatalf("add status/role = %d %q", addRecorder.Code, service.addedRole)
	}

	crossSite := authenticatedRequest(http.MethodDelete, "/api/v1/organizations/"+testOrgID+"/members/"+testUserID, "")
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	crossSiteRecorder := httptest.NewRecorder()
	handler.ServeHTTP(crossSiteRecorder, crossSite)
	if crossSiteRecorder.Code != http.StatusBadRequest {
		t.Fatalf("cross-site status = %d", crossSiteRecorder.Code)
	}
}

func testOrganizationHandler(service *handlerTestOrganizations) http.Handler {
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

type handlerTestOrganizations struct {
	organization         organizations.Organization
	organizations        []organizations.Organization
	serviceError         error
	detailOrganizationID string
	addedRole            organizations.Role
}

func (s *handlerTestOrganizations) Create(context.Context, string, string, string) (organizations.Organization, error) {
	return s.organization, s.serviceError
}
func (s *handlerTestOrganizations) ListForUser(context.Context, string, organizations.PageRequest) (organizations.OrganizationPage, error) {
	return organizations.OrganizationPage{Items: s.organizations}, s.serviceError
}
func (s *handlerTestOrganizations) ForMember(_ context.Context, organizationID, _ string) (organizations.Organization, error) {
	s.detailOrganizationID = organizationID
	return s.organization, s.serviceError
}
func (s *handlerTestOrganizations) Members(context.Context, string, string, organizations.PageRequest) (organizations.MembershipPage, error) {
	return organizations.MembershipPage{}, s.serviceError
}
func (s *handlerTestOrganizations) AddMember(_ context.Context, _, _, _ string, role organizations.Role) error {
	s.addedRole = role
	return s.serviceError
}
func (s *handlerTestOrganizations) ChangeMemberRole(context.Context, string, string, string, organizations.Role) error {
	return s.serviceError
}
func (s *handlerTestOrganizations) RemoveMember(context.Context, string, string, string) error {
	return s.serviceError
}
