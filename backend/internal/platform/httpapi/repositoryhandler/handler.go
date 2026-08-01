package repositoryhandler

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

var validID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}

type RepositoryService interface {
	Create(context.Context, string, string, string, repositories.Visibility, string) (repositories.Repository, error)
	List(context.Context, string, string, repositories.PageRequest) (repositories.RepositoryPage, error)
	Detail(context.Context, string, string, string) (repositories.Repository, error)
	Update(context.Context, string, string, string, repositories.UpdateRepository) (repositories.Repository, error)
}

type Handler struct {
	authenticator Authenticator
	repositories  RepositoryService
}

func New(authenticator Authenticator, repositoryService RepositoryService) *Handler {
	return &Handler{authenticator: authenticator, repositories: repositoryService}
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.Handle(http.MethodPost, "/api/v1/namespaces/{namespace}/repositories", handler.create)
	router.Handle(http.MethodGet, "/api/v1/namespaces/{namespace}/repositories", handler.list)
	router.Handle(http.MethodGet, "/api/v1/namespaces/{namespace}/repositories/{repository}", handler.detail)
	router.Handle(http.MethodPatch, "/api/v1/namespaces/{namespace}/repositories/{repository}", handler.update)
}

type createRequest struct {
	Name        string `json:"name"`
	Visibility  string `json:"visibility"`
	Description string `json:"description"`
}

type updateRequest struct {
	Visibility  *string `json:"visibility"`
	Description *string `json:"description"`
}

type repositoryResponse struct {
	ID                        string `json:"id"`
	Namespace                 string `json:"namespace"`
	Name                      string `json:"name"`
	Visibility                string `json:"visibility"`
	Description               string `json:"description"`
	CreatedByUserID           string `json:"created_by_user_id"`
	VisibilityUpdatedByUserID string `json:"visibility_updated_by_user_id"`
	VisibilityUpdatedAt       string `json:"visibility_updated_at"`
	CreatedAt                 string `json:"created_at"`
	UpdatedAt                 string `json:"updated_at"`
}

type repositoryListResponse struct {
	Items []repositoryResponse `json:"items"`
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
	namespaceName, apiError := namespacePath(request)
	if apiError != nil {
		return apiError
	}
	var input createRequest
	if apiError := httpapi.DecodeJSON(w, request, &input); apiError != nil {
		return apiError
	}
	visibility, validation := validateCreate(input)
	if validation != nil {
		return validation
	}
	repository, serviceErr := h.repositories.Create(
		request.Context(), string(user.ID), namespaceName, input.Name, visibility, input.Description,
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	httpapi.WriteJSON(w, http.StatusCreated, mapRepository(repository, namespaceName))
	return nil
}

func (h *Handler) list(w http.ResponseWriter, request *http.Request) error {
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	namespaceName, apiError := namespacePath(request)
	if apiError != nil {
		return apiError
	}
	page, apiError := parsePage(request)
	if apiError != nil {
		return apiError
	}
	result, serviceErr := h.repositories.List(request.Context(), string(user.ID), namespaceName, page)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	items := make([]repositoryResponse, 0, len(result.Items))
	for _, repository := range result.Items {
		items = append(items, mapRepository(repository, namespaceName))
	}
	httpapi.WriteJSON(w, http.StatusOK, repositoryListResponse{
		Items: items, Meta: httpapi.PageMeta{Limit: page.Limit, NextCursor: encodeCursor(result.NextAfter)},
	})
	return nil
}

func (h *Handler) detail(w http.ResponseWriter, request *http.Request) error {
	user, namespaceName, repositoryName, err := h.repositoryContext(request)
	if err != nil {
		return err
	}
	repository, serviceErr := h.repositories.Detail(
		request.Context(), string(user.ID), namespaceName, repositoryName,
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	httpapi.WriteJSON(w, http.StatusOK, mapRepository(repository, namespaceName))
	return nil
}

func (h *Handler) update(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	user, namespaceName, repositoryName, err := h.repositoryContext(request)
	if err != nil {
		return err
	}
	var input updateRequest
	if apiError := httpapi.DecodeJSON(w, request, &input); apiError != nil {
		return apiError
	}
	update, validation := validateUpdate(input)
	if validation != nil {
		return validation
	}
	repository, serviceErr := h.repositories.Update(
		request.Context(), string(user.ID), namespaceName, repositoryName, update,
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	httpapi.WriteJSON(w, http.StatusOK, mapRepository(repository, namespaceName))
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

func (h *Handler) repositoryContext(request *http.Request) (auth.User, string, string, error) {
	user, err := h.currentUser(request)
	if err != nil {
		return auth.User{}, "", "", err
	}
	namespaceName, apiError := namespacePath(request)
	if apiError != nil {
		return auth.User{}, "", "", apiError
	}
	repositoryName := request.PathValue("repository")
	if _, err := repositories.NormalizeName(repositoryName); err != nil {
		return auth.User{}, "", "", httpapi.InvalidRequest("repository name is invalid")
	}
	return user, namespaceName, repositoryName, nil
}

func namespacePath(request *http.Request) (string, *httpapi.Error) {
	name := request.PathValue("namespace")
	if _, err := namespaces.NormalizeName(name); err != nil {
		return "", httpapi.InvalidRequest("namespace name is invalid")
	}
	return name, nil
}

func validateCreate(input createRequest) (repositories.Visibility, *httpapi.Error) {
	fields := make([]httpapi.FieldError, 0, 3)
	if _, err := repositories.NormalizeName(input.Name); err != nil {
		fields = append(fields, httpapi.FieldError{Field: "name", Message: "must be a valid 1 to 64 byte repository name"})
	}
	visibility, err := repositories.ParseVisibility(input.Visibility)
	if err != nil {
		fields = append(fields, httpapi.FieldError{Field: "visibility", Message: "must be PUBLIC or PRIVATE"})
	}
	if len(input.Description) > repositories.MaxDescriptionLength {
		fields = append(fields, httpapi.FieldError{Field: "description", Message: "must contain at most 1024 bytes"})
	}
	if len(fields) > 0 {
		return "", httpapi.ValidationFailed(fields...)
	}
	return visibility, nil
}

func validateUpdate(input updateRequest) (repositories.UpdateRepository, *httpapi.Error) {
	if input.Description == nil && input.Visibility == nil {
		return repositories.UpdateRepository{}, httpapi.ValidationFailed(
			httpapi.FieldError{Field: "body", Message: "must include description or visibility"},
		)
	}
	fields := make([]httpapi.FieldError, 0, 2)
	if input.Description != nil && len(*input.Description) > repositories.MaxDescriptionLength {
		fields = append(fields, httpapi.FieldError{Field: "description", Message: "must contain at most 1024 bytes"})
	}
	var visibility *repositories.Visibility
	if input.Visibility != nil {
		parsed, err := repositories.ParseVisibility(*input.Visibility)
		if err != nil {
			fields = append(fields, httpapi.FieldError{Field: "visibility", Message: "must be PUBLIC or PRIVATE"})
		} else {
			visibility = &parsed
		}
	}
	if len(fields) > 0 {
		return repositories.UpdateRepository{}, httpapi.ValidationFailed(fields...)
	}
	return repositories.UpdateRepository{Description: input.Description, Visibility: visibility}, nil
}

func parsePage(request *http.Request) (repositories.PageRequest, *httpapi.Error) {
	page, err := httpapi.ParsePage(request.URL.Query())
	if err != nil {
		return repositories.PageRequest{}, err
	}
	after := ""
	if page.Cursor != "" {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(page.Cursor)
		if decodeErr != nil || !validID.Match(decoded) {
			return repositories.PageRequest{}, httpapi.InvalidRequest("cursor is invalid")
		}
		after = string(decoded)
	}
	return repositories.PageRequest{Limit: page.Limit, After: after}, nil
}

func encodeCursor(after string) string {
	if after == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(after))
}

func mapRepository(value repositories.Repository, namespaceName string) repositoryResponse {
	normalizedNamespace, _ := namespaces.NormalizeName(namespaceName)
	return repositoryResponse{
		ID: value.ID, Namespace: normalizedNamespace, Name: value.Name,
		Visibility: string(value.Visibility), Description: value.Description,
		CreatedByUserID:           value.CreatedByUserID,
		VisibilityUpdatedByUserID: value.VisibilityUpdatedByUserID,
		VisibilityUpdatedAt:       httpapi.FormatTime(value.VisibilityUpdatedAt),
		CreatedAt:                 httpapi.FormatTime(value.CreatedAt), UpdatedAt: httpapi.FormatTime(value.UpdatedAt),
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrForbidden):
		return httpapi.Forbidden()
	case errors.Is(err, repositories.ErrNotFound):
		return httpapi.NotFound()
	case errors.Is(err, repositories.ErrConflict):
		return httpapi.Conflict("repository already exists in this namespace")
	case errors.Is(err, repositories.ErrInvalidName), errors.Is(err, repositories.ErrInvalidVisibility),
		errors.Is(err, repositories.ErrInvalidRepository), errors.Is(err, repositories.ErrInvalidUpdate),
		errors.Is(err, namespaces.ErrInvalidName):
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
