package organizationhandler

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/organizations"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

var validID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}

type OrganizationService interface {
	Create(context.Context, string, string, string) (organizations.Organization, error)
	ListForUser(context.Context, string, organizations.PageRequest) (organizations.OrganizationPage, error)
	ForMember(context.Context, string, string) (organizations.Organization, error)
	Members(context.Context, string, string, organizations.PageRequest) (organizations.MembershipPage, error)
	AddMember(context.Context, string, string, string, organizations.Role) error
	ChangeMemberRole(context.Context, string, string, string, organizations.Role) error
	RemoveMember(context.Context, string, string, string) error
}

type Handler struct {
	authenticator Authenticator
	organizations OrganizationService
}

func New(authenticator Authenticator, organizationService OrganizationService) *Handler {
	return &Handler{authenticator: authenticator, organizations: organizationService}
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.Handle(http.MethodPost, "/api/v1/organizations", handler.create)
	router.Handle(http.MethodGet, "/api/v1/organizations", handler.list)
	router.Handle(http.MethodGet, "/api/v1/organizations/{organization_id}", handler.detail)
	router.Handle(http.MethodGet, "/api/v1/organizations/{organization_id}/members", handler.members)
	router.Handle(http.MethodPost, "/api/v1/organizations/{organization_id}/members", handler.addMember)
	router.Handle(http.MethodPatch, "/api/v1/organizations/{organization_id}/members/{user_id}", handler.changeMemberRole)
	router.Handle(http.MethodDelete, "/api/v1/organizations/{organization_id}/members/{user_id}", handler.removeMember)
}

type createRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type memberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type roleRequest struct {
	Role string `json:"role"`
}

type organizationResponse struct {
	ID              string `json:"id"`
	Namespace       string `json:"namespace"`
	Description     string `json:"description"`
	CreatedByUserID string `json:"created_by_user_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type membershipResponse struct {
	UserID        string `json:"user_id"`
	Role          string `json:"role"`
	AddedByUserID string `json:"added_by_user_id"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type organizationListResponse struct {
	Items []organizationResponse `json:"items"`
	Meta  httpapi.PageMeta       `json:"meta"`
}

type membershipListResponse struct {
	Items []membershipResponse `json:"items"`
	Meta  httpapi.PageMeta     `json:"meta"`
}

func (h *Handler) create(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	var input createRequest
	if apiError := httpapi.DecodeJSON(w, request, &input); apiError != nil {
		return apiError
	}
	fields := make([]httpapi.FieldError, 0, 2)
	if input.Name == "" || len(input.Name) > namespaces.MaxNameLength {
		fields = append(fields, httpapi.FieldError{Field: "name", Message: "must contain 1 to 64 characters"})
	}
	if len(input.Description) > 1024 {
		fields = append(fields, httpapi.FieldError{Field: "description", Message: "must contain at most 1024 bytes"})
	}
	if len(fields) > 0 {
		return httpapi.ValidationFailed(fields...)
	}
	organization, serviceErr := h.organizations.Create(
		request.Context(), string(user.ID), input.Name, input.Description,
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	httpapi.WriteJSON(w, http.StatusCreated, mapOrganization(organization))
	return nil
}

func (h *Handler) list(w http.ResponseWriter, request *http.Request) error {
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	page, apiError := parsePage(request)
	if apiError != nil {
		return apiError
	}
	result, serviceErr := h.organizations.ListForUser(request.Context(), string(user.ID), page)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	items := make([]organizationResponse, 0, len(result.Items))
	for _, organization := range result.Items {
		items = append(items, mapOrganization(organization))
	}
	httpapi.WriteJSON(w, http.StatusOK, organizationListResponse{
		Items: items, Meta: httpapi.PageMeta{Limit: page.Limit, NextCursor: encodeCursor(result.NextAfter)},
	})
	return nil
}

func (h *Handler) detail(w http.ResponseWriter, request *http.Request) error {
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	organizationID, apiError := pathID(request, "organization_id")
	if apiError != nil {
		return apiError
	}
	organization, serviceErr := h.organizations.ForMember(request.Context(), organizationID, string(user.ID))
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	httpapi.WriteJSON(w, http.StatusOK, mapOrganization(organization))
	return nil
}

func (h *Handler) members(w http.ResponseWriter, request *http.Request) error {
	user, organizationID, page, err := h.memberListContext(request)
	if err != nil {
		return err
	}
	result, serviceErr := h.organizations.Members(request.Context(), organizationID, string(user.ID), page)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	items := make([]membershipResponse, 0, len(result.Items))
	for _, membership := range result.Items {
		items = append(items, mapMembership(membership))
	}
	httpapi.WriteJSON(w, http.StatusOK, membershipListResponse{
		Items: items, Meta: httpapi.PageMeta{Limit: page.Limit, NextCursor: encodeCursor(result.NextAfter)},
	})
	return nil
}

func (h *Handler) addMember(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	organizationID, apiError := pathID(request, "organization_id")
	if apiError != nil {
		return apiError
	}
	var input memberRequest
	if apiError := httpapi.DecodeJSON(w, request, &input); apiError != nil {
		return apiError
	}
	role, validation := validateMember(input.UserID, input.Role)
	if validation != nil {
		return validation
	}
	if serviceErr := h.organizations.AddMember(
		request.Context(), organizationID, string(user.ID), input.UserID, role,
	); serviceErr != nil {
		return mapError(serviceErr)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) changeMemberRole(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	user, organizationID, targetUserID, err := h.memberMutationContext(request)
	if err != nil {
		return err
	}
	var input roleRequest
	if apiError := httpapi.DecodeJSON(w, request, &input); apiError != nil {
		return apiError
	}
	role, roleErr := organizations.ParseRole(input.Role)
	if roleErr != nil {
		return httpapi.ValidationFailed(httpapi.FieldError{Field: "role", Message: "must be OWNER, ADMIN, WRITER, or READER"})
	}
	if serviceErr := h.organizations.ChangeMemberRole(
		request.Context(), organizationID, string(user.ID), targetUserID, role,
	); serviceErr != nil {
		return mapError(serviceErr)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) removeMember(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	user, organizationID, targetUserID, err := h.memberMutationContext(request)
	if err != nil {
		return err
	}
	if serviceErr := h.organizations.RemoveMember(
		request.Context(), organizationID, string(user.ID), targetUserID,
	); serviceErr != nil {
		return mapError(serviceErr)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) currentUser(request *http.Request) (auth.User, error) {
	cookie, err := request.Cookie(authhandler.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return auth.User{}, httpapi.AuthenticationFailed()
	}
	user, err := h.authenticator.Authenticate(request.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			return auth.User{}, httpapi.AuthenticationFailed()
		}
		return auth.User{}, err
	}
	return user, nil
}

func (h *Handler) memberListContext(request *http.Request) (auth.User, string, organizations.PageRequest, error) {
	user, err := h.currentUser(request)
	if err != nil {
		return auth.User{}, "", organizations.PageRequest{}, err
	}
	organizationID, apiError := pathID(request, "organization_id")
	if apiError != nil {
		return auth.User{}, "", organizations.PageRequest{}, apiError
	}
	page, apiError := parsePage(request)
	if apiError != nil {
		return auth.User{}, "", organizations.PageRequest{}, apiError
	}
	return user, organizationID, page, nil
}

func (h *Handler) memberMutationContext(request *http.Request) (auth.User, string, string, error) {
	user, err := h.currentUser(request)
	if err != nil {
		return auth.User{}, "", "", err
	}
	organizationID, apiError := pathID(request, "organization_id")
	if apiError != nil {
		return auth.User{}, "", "", apiError
	}
	userID, apiError := pathID(request, "user_id")
	if apiError != nil {
		return auth.User{}, "", "", apiError
	}
	return user, organizationID, userID, nil
}

func validateMember(userID, roleValue string) (organizations.Role, *httpapi.Error) {
	fields := make([]httpapi.FieldError, 0, 2)
	if !validID.MatchString(userID) {
		fields = append(fields, httpapi.FieldError{Field: "user_id", Message: "must be a valid ID"})
	}
	role, err := organizations.ParseRole(roleValue)
	if err != nil {
		fields = append(fields, httpapi.FieldError{Field: "role", Message: "must be OWNER, ADMIN, WRITER, or READER"})
	}
	if len(fields) > 0 {
		return "", httpapi.ValidationFailed(fields...)
	}
	return role, nil
}

func pathID(request *http.Request, name string) (string, *httpapi.Error) {
	value := request.PathValue(name)
	if !validID.MatchString(value) {
		return "", httpapi.InvalidRequest(name + " is invalid")
	}
	return value, nil
}

func parsePage(request *http.Request) (organizations.PageRequest, *httpapi.Error) {
	page, err := httpapi.ParsePage(request.URL.Query())
	if err != nil {
		return organizations.PageRequest{}, err
	}
	after := ""
	if page.Cursor != "" {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(page.Cursor)
		if decodeErr != nil || !validID.Match(decoded) {
			return organizations.PageRequest{}, httpapi.InvalidRequest("cursor is invalid")
		}
		after = string(decoded)
	}
	return organizations.PageRequest{Limit: page.Limit, After: after}, nil
}

func encodeCursor(after string) string {
	if after == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(after))
}

func mapOrganization(value organizations.Organization) organizationResponse {
	return organizationResponse{
		ID: value.ID, Namespace: value.NamespaceName, Description: value.Description,
		CreatedByUserID: value.CreatedByUserID,
		CreatedAt:       httpapi.FormatTime(value.CreatedAt), UpdatedAt: httpapi.FormatTime(value.UpdatedAt),
	}
}

func mapMembership(value organizations.Membership) membershipResponse {
	return membershipResponse{
		UserID: value.UserID, Role: string(value.Role), AddedByUserID: value.AddedByUserID,
		CreatedAt: httpapi.FormatTime(value.CreatedAt), UpdatedAt: httpapi.FormatTime(value.UpdatedAt),
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, organizations.ErrForbidden):
		return httpapi.Forbidden()
	case errors.Is(err, organizations.ErrNotFound):
		return httpapi.NotFound()
	case errors.Is(err, organizations.ErrConflict):
		return httpapi.Conflict("organization or membership already exists")
	case errors.Is(err, organizations.ErrLastOwner):
		return httpapi.Conflict("organization must retain at least one owner")
	case errors.Is(err, organizations.ErrInvalidRole), errors.Is(err, organizations.ErrInvalidMember), errors.Is(err, namespaces.ErrInvalidName):
		return httpapi.ValidationFailed()
	default:
		return err
	}
}

func rejectCrossSite(request *http.Request) *httpapi.Error {
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return httpapi.InvalidRequest("cross-site request rejected")
	}
	return nil
}
