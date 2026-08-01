package artifacthandler

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}

type RepositoryService interface {
	Detail(context.Context, string, string, string) (repositories.Repository, error)
}

type ArtifactService interface {
	ArtifactDetail(context.Context, string, string) (artifacts.Snapshot, error)
	TagDetail(context.Context, string, string) (artifacts.Tag, error)
	ListArtifacts(context.Context, string, int, string) (artifacts.ArtifactPage, error)
	ListTags(context.Context, string, int, string) (artifacts.TagPage, error)
}

type Handler struct {
	authenticator Authenticator
	repositories  RepositoryService
	artifacts     ArtifactService
}

func New(
	authenticator Authenticator,
	repositoryService RepositoryService,
	artifactService ArtifactService,
) (*Handler, error) {
	if authenticator == nil || repositoryService == nil || artifactService == nil {
		return nil, errors.New("Artifact handler dependencies must be configured")
	}
	return &Handler{
		authenticator: authenticator,
		repositories:  repositoryService,
		artifacts:     artifactService,
	}, nil
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.Handle(
		http.MethodGet,
		"/api/v1/namespaces/{namespace}/repositories/{repository}/artifacts",
		handler.listArtifacts,
	)
	router.Handle(
		http.MethodGet,
		"/api/v1/namespaces/{namespace}/repositories/{repository}/artifacts/{digest}",
		handler.artifactDetail,
	)
	router.Handle(
		http.MethodGet,
		"/api/v1/namespaces/{namespace}/repositories/{repository}/tags",
		handler.listTags,
	)
	router.Handle(
		http.MethodGet,
		"/api/v1/namespaces/{namespace}/repositories/{repository}/tags/{tag}",
		handler.tagDetail,
	)
}

type artifactResponse struct {
	Digest              string              `json:"digest"`
	Kind                string              `json:"kind"`
	MediaType           *string             `json:"media_type,omitempty"`
	SizeBytes           *int64              `json:"size_bytes,omitempty"`
	SourceCreatedAt     *string             `json:"source_created_at,omitempty"`
	DescriptorsComplete bool                `json:"descriptors_complete"`
	DiscoveredAt        string              `json:"discovered_at"`
	UpdatedAt           string              `json:"updated_at"`
	Manifests           *[]manifestResponse `json:"manifests,omitempty"`
}

type manifestResponse struct {
	Position  int               `json:"position"`
	Digest    string            `json:"digest"`
	MediaType *string           `json:"media_type,omitempty"`
	SizeBytes *int64            `json:"size_bytes,omitempty"`
	Platform  *platformResponse `json:"platform,omitempty"`
}

type platformResponse struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
}

type tagResponse struct {
	Name      string            `json:"name"`
	Digest    string            `json:"digest"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
	Artifact  *artifactResponse `json:"artifact,omitempty"`
}

type artifactListResponse struct {
	Items []artifactResponse `json:"items"`
	Meta  httpapi.PageMeta   `json:"meta"`
}

type tagListResponse struct {
	Items []tagResponse    `json:"items"`
	Meta  httpapi.PageMeta `json:"meta"`
}

func (h *Handler) listArtifacts(w http.ResponseWriter, request *http.Request) error {
	_, repository, err := h.repositoryContext(request)
	if err != nil {
		return err
	}
	page, apiError := parsePage(request, artifacts.ParseDigest)
	if apiError != nil {
		return apiError
	}
	result, serviceErr := h.artifacts.ListArtifacts(
		request.Context(), repository.ID, page.Limit, page.After,
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	items := make([]artifactResponse, 0, len(result.Items))
	for _, artifact := range result.Items {
		items = append(items, mapArtifact(artifact))
	}
	httpapi.WriteJSON(w, http.StatusOK, artifactListResponse{
		Items: items,
		Meta:  httpapi.PageMeta{Limit: page.Limit, NextCursor: encodeCursor(result.NextAfter)},
	})
	return nil
}

func (h *Handler) artifactDetail(w http.ResponseWriter, request *http.Request) error {
	_, repository, err := h.repositoryContext(request)
	if err != nil {
		return err
	}
	digest := request.PathValue("digest")
	if _, parseErr := artifacts.ParseDigest(digest); parseErr != nil {
		return httpapi.InvalidRequest("artifact digest is invalid")
	}
	result, serviceErr := h.artifacts.ArtifactDetail(request.Context(), repository.ID, digest)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	httpapi.WriteJSON(w, http.StatusOK, mapSnapshot(result))
	return nil
}

func (h *Handler) listTags(w http.ResponseWriter, request *http.Request) error {
	_, repository, err := h.repositoryContext(request)
	if err != nil {
		return err
	}
	page, apiError := parsePage(request, artifacts.ParseTagName)
	if apiError != nil {
		return apiError
	}
	result, serviceErr := h.artifacts.ListTags(
		request.Context(), repository.ID, page.Limit, page.After,
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	items := make([]tagResponse, 0, len(result.Items))
	for _, tag := range result.Items {
		items = append(items, mapTag(tag))
	}
	httpapi.WriteJSON(w, http.StatusOK, tagListResponse{
		Items: items,
		Meta:  httpapi.PageMeta{Limit: page.Limit, NextCursor: encodeCursor(result.NextAfter)},
	})
	return nil
}

func (h *Handler) tagDetail(w http.ResponseWriter, request *http.Request) error {
	_, repository, err := h.repositoryContext(request)
	if err != nil {
		return err
	}
	tagName := request.PathValue("tag")
	if _, parseErr := artifacts.ParseTagName(tagName); parseErr != nil {
		return httpapi.InvalidRequest("tag name is invalid")
	}
	tag, serviceErr := h.artifacts.TagDetail(request.Context(), repository.ID, tagName)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	snapshot, serviceErr := h.artifacts.ArtifactDetail(
		request.Context(), repository.ID, tag.Digest.String(),
	)
	if serviceErr != nil {
		return mapError(serviceErr)
	}
	if snapshot.Artifact.Digest != tag.Digest {
		return artifacts.ErrInvalidArtifact
	}
	response := mapTag(tag)
	artifact := mapSnapshot(snapshot)
	response.Artifact = &artifact
	httpapi.WriteJSON(w, http.StatusOK, response)
	return nil
}

func (h *Handler) repositoryContext(
	request *http.Request,
) (auth.User, repositories.Repository, error) {
	user, err := h.currentUser(request)
	if err != nil {
		return auth.User{}, repositories.Repository{}, err
	}
	namespaceName := request.PathValue("namespace")
	if _, normalizeErr := namespaces.NormalizeName(namespaceName); normalizeErr != nil {
		return auth.User{}, repositories.Repository{}, httpapi.InvalidRequest("namespace name is invalid")
	}
	repositoryName := request.PathValue("repository")
	if _, normalizeErr := repositories.NormalizeName(repositoryName); normalizeErr != nil {
		return auth.User{}, repositories.Repository{}, httpapi.InvalidRequest("repository name is invalid")
	}
	repository, serviceErr := h.repositories.Detail(
		request.Context(), string(user.ID), namespaceName, repositoryName,
	)
	if serviceErr != nil {
		return auth.User{}, repositories.Repository{}, mapError(serviceErr)
	}
	return user, repository, nil
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

type pageRequest struct {
	Limit int
	After string
}

func parsePage[T ~string](
	request *http.Request,
	parse func(string) (T, error),
) (pageRequest, *httpapi.Error) {
	page, apiError := httpapi.ParsePage(request.URL.Query())
	if apiError != nil {
		return pageRequest{}, apiError
	}
	after := ""
	if page.Cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(page.Cursor)
		if err != nil {
			return pageRequest{}, httpapi.InvalidRequest("cursor is invalid")
		}
		after = string(decoded)
		if _, err := parse(after); err != nil {
			return pageRequest{}, httpapi.InvalidRequest("cursor is invalid")
		}
	}
	return pageRequest{Limit: page.Limit, After: after}, nil
}

func encodeCursor(after string) string {
	if after == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(after))
}

func mapArtifact(value artifacts.Artifact) artifactResponse {
	response := artifactResponse{
		Digest: value.Digest.String(), Kind: string(value.Kind),
		MediaType: value.MediaType, SizeBytes: value.SizeBytes,
		DescriptorsComplete: value.DescriptorsComplete,
		DiscoveredAt:        httpapi.FormatTime(value.DiscoveredAt),
		UpdatedAt:           httpapi.FormatTime(value.UpdatedAt),
	}
	if value.SourceCreatedAt != nil {
		createdAt := httpapi.FormatTime(*value.SourceCreatedAt)
		response.SourceCreatedAt = &createdAt
	}
	return response
}

func mapSnapshot(value artifacts.Snapshot) artifactResponse {
	response := mapArtifact(value.Artifact)
	if value.Artifact.Kind != artifacts.KindIndex || !value.Artifact.DescriptorsComplete {
		return response
	}
	manifests := make([]manifestResponse, 0, len(value.Descriptors))
	for _, descriptor := range value.Descriptors {
		item := manifestResponse{
			Position: descriptor.Position, Digest: descriptor.Digest.String(),
			MediaType: descriptor.MediaType, SizeBytes: descriptor.SizeBytes,
		}
		if descriptor.Platform != nil {
			item.Platform = &platformResponse{
				OS: descriptor.Platform.OS, Architecture: descriptor.Platform.Architecture,
				Variant: descriptor.Platform.Variant,
			}
		}
		manifests = append(manifests, item)
	}
	response.Manifests = &manifests
	return response
}

func mapTag(value artifacts.Tag) tagResponse {
	return tagResponse{
		Name: value.Name.String(), Digest: value.Digest.String(),
		CreatedAt: httpapi.FormatTime(value.CreatedAt), UpdatedAt: httpapi.FormatTime(value.UpdatedAt),
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrForbidden):
		return httpapi.Forbidden()
	case errors.Is(err, repositories.ErrNotFound), errors.Is(err, artifacts.ErrNotFound):
		return httpapi.NotFound()
	default:
		return err
	}
}
