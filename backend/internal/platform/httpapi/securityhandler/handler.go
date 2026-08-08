package securityhandler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}

type RepositoryService interface {
	Detail(context.Context, string, string, string) (repositories.Repository, error)
}

type SecurityService interface {
	Detail(context.Context, string, string) (security.Detail, error)
}

type Handler struct {
	authenticator Authenticator
	repositories  RepositoryService
	security      SecurityService
}

func New(
	authenticator Authenticator,
	repositoryService RepositoryService,
	securityService SecurityService,
) (*Handler, error) {
	if authenticator == nil || repositoryService == nil || securityService == nil {
		return nil, errors.New("security handler dependencies must be configured")
	}
	return &Handler{
		authenticator: authenticator, repositories: repositoryService, security: securityService,
	}, nil
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.Handle(
		http.MethodGet,
		"/api/v1/namespaces/{namespace}/repositories/{repository}/artifacts/{digest}/security",
		handler.detail,
	)
}

type response struct {
	Digest    string            `json:"digest"`
	Scan      resultResponse    `json:"scan"`
	SBOM      resultResponse    `json:"sbom"`
	Signature signatureResponse `json:"signature"`
}

type resultResponse struct {
	State        string          `json:"state"`
	ErrorCode    string          `json:"error_code,omitempty"`
	Attempts     int             `json:"attempts"`
	UpdatedAt    string          `json:"updated_at"`
	CompletedAt  *string         `json:"completed_at,omitempty"`
	Tool         *toolResponse   `json:"tool,omitempty"`
	FindingCount *int            `json:"finding_count,omitempty"`
	Severities   *map[string]int `json:"severity_counts,omitempty"`
	Format       string          `json:"format,omitempty"`
}

type toolResponse struct {
	Name                  string `json:"name"`
	ScannerVersion        string `json:"scanner_version"`
	DatabaseSchemaVersion int    `json:"database_schema_version"`
	DatabaseUpdatedAt     string `json:"database_updated_at"`
	DatabaseDownloadedAt  string `json:"database_downloaded_at"`
}

type signatureResponse struct {
	State         string                      `json:"state"`
	ErrorCode     string                      `json:"error_code,omitempty"`
	Attempts      int                         `json:"attempts,omitempty"`
	UpdatedAt     string                      `json:"updated_at,omitempty"`
	PolicyID      string                      `json:"policy_id,omitempty"`
	PolicyVersion int64                       `json:"policy_version,omitempty"`
	CosignVersion string                      `json:"cosign_version,omitempty"`
	CompletedAt   *string                     `json:"completed_at,omitempty"`
	Evidence      []signatureEvidenceResponse `json:"evidence"`
}

type signatureEvidenceResponse struct {
	Kind               string `json:"kind"`
	SignatureDigest    string `json:"signature_digest"`
	SignerType         string `json:"signer_type"`
	KeyFingerprint     string `json:"key_fingerprint,omitempty"`
	OIDCIssuer         string `json:"oidc_issuer,omitempty"`
	Subject            string `json:"subject,omitempty"`
	CryptographicState string `json:"cryptographic_state"`
	TrustState         string `json:"trust_state"`
	Reason             string `json:"reason"`
}

func (h *Handler) detail(w http.ResponseWriter, request *http.Request) error {
	user, err := h.currentUser(request)
	if err != nil {
		return err
	}
	namespaceName := request.PathValue("namespace")
	if _, err := namespaces.NormalizeName(namespaceName); err != nil {
		return httpapi.InvalidRequest("namespace name is invalid")
	}
	repositoryName := request.PathValue("repository")
	if _, err := repositories.NormalizeName(repositoryName); err != nil {
		return httpapi.InvalidRequest("repository name is invalid")
	}
	digest := request.PathValue("digest")
	if _, err := artifacts.ParseDigest(digest); err != nil {
		return httpapi.InvalidRequest("artifact digest is invalid")
	}
	repository, err := h.repositories.Detail(
		request.Context(), string(user.ID), namespaceName, repositoryName,
	)
	if err != nil {
		return mapError(err)
	}
	detail, err := h.security.Detail(request.Context(), repository.ID, digest)
	if err != nil {
		return mapError(err)
	}
	if detail.Workflow.Target.RepositoryID != repository.ID ||
		detail.Workflow.Target.Digest.String() != digest {
		return security.ErrInvalid
	}
	httpapi.WriteJSON(w, http.StatusOK, mapDetail(detail))
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

func mapDetail(detail security.Detail) response {
	return response{
		Digest: detail.Workflow.Target.Digest.String(),
		Scan: mapResult(
			detail.Scan, detail.ScannedAt, detail.ToolVersion,
			detail.FindingCount, detail.SeverityCounts, "",
		),
		SBOM: mapResult(
			detail.SBOM, detail.SBOMCreatedAt, nil, 0, nil, detail.SBOMFormat,
		),
		Signature: mapSignature(detail.Signature),
	}
}

func mapSignature(detail *security.VerificationDetail) signatureResponse {
	if detail == nil {
		return signatureResponse{State: "ABSENT", Evidence: []signatureEvidenceResponse{}}
	}
	result := signatureResponse{
		State: string(detail.Status.State), ErrorCode: detail.Status.ErrorCode,
		Attempts: detail.Status.Attempts, UpdatedAt: httpapi.FormatTime(detail.Status.UpdatedAt),
		PolicyID: detail.Workflow.PolicyID, PolicyVersion: detail.Workflow.PolicyVersion,
		CosignVersion: detail.CosignVersion,
		Evidence:      make([]signatureEvidenceResponse, 0, len(detail.Evidence)),
	}
	if detail.CompletedAt != nil {
		formatted := httpapi.FormatTime(*detail.CompletedAt)
		result.CompletedAt = &formatted
	}
	for _, evidence := range detail.Evidence {
		result.Evidence = append(result.Evidence, signatureEvidenceResponse{
			Kind: string(evidence.Kind), SignatureDigest: evidence.SignatureDigest.String(),
			SignerType: string(evidence.SignerType), KeyFingerprint: evidence.KeyFingerprint,
			OIDCIssuer: evidence.OIDCIssuer, Subject: evidence.Subject,
			CryptographicState: string(evidence.State), TrustState: string(evidence.TrustState),
			Reason: evidence.Reason,
		})
	}
	return result
}

func mapResult(
	status security.ResultStatus,
	completedAt *time.Time,
	version *security.ToolVersion,
	findingCount int,
	severities map[string]int,
	format string,
) resultResponse {
	result := resultResponse{
		State: string(status.State), ErrorCode: status.ErrorCode,
		Attempts: status.Attempts, UpdatedAt: httpapi.FormatTime(status.UpdatedAt),
	}
	if completedAt != nil {
		formatted := httpapi.FormatTime(*completedAt)
		result.CompletedAt = &formatted
	}
	if version != nil {
		result.Tool = &toolResponse{
			Name: security.ScannerNameTrivy, ScannerVersion: version.ScannerVersion,
			DatabaseSchemaVersion: version.DatabaseSchemaVersion,
			DatabaseUpdatedAt:     httpapi.FormatTime(version.DatabaseUpdatedAt),
			DatabaseDownloadedAt:  httpapi.FormatTime(version.DatabaseDownloadedAt),
		}
		count := findingCount
		result.FindingCount = &count
		counts := severities
		if counts == nil {
			counts = map[string]int{}
		}
		result.Severities = &counts
	}
	if completedAt != nil && format != "" {
		result.Format = format
	}
	return result
}

func mapError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrForbidden):
		return httpapi.Forbidden()
	case errors.Is(err, repositories.ErrNotFound), errors.Is(err, security.ErrNotFound):
		return httpapi.NotFound()
	default:
		return err
	}
}
