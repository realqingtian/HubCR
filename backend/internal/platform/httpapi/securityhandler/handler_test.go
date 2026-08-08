package securityhandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/httpapi/authhandler"
)

const (
	securityHandlerUserID       = "11111111-1111-4111-8111-111111111111"
	securityHandlerRepositoryID = "22222222-2222-4222-8222-222222222222"
	securityHandlerDigest       = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestSecurityRouteReturnsTruthfulCompletedAndStaleEvidence(t *testing.T) {
	now := time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)
	target, _ := security.NewTarget(
		securityHandlerRepositoryID, "team", "image", securityHandlerDigest,
	)
	detail := security.Detail{
		Workflow: security.Workflow{
			ID: "workflow", Target: target, ScanJobID: "scan", SBOMJobID: "sbom",
			CreatedAt: now, UpdatedAt: now,
		},
		Scan: security.ResultStatus{State: security.ResultStale, Attempts: 1, UpdatedAt: now},
		SBOM: security.ResultStatus{State: security.ResultCompleted, Attempts: 1, UpdatedAt: now},
		ToolVersion: &security.ToolVersion{
			ScannerVersion: "0.72.0", DatabaseSchemaVersion: 2,
			DatabaseUpdatedAt: now.Add(-time.Hour), DatabaseDownloadedAt: now.Add(-time.Minute),
			ObservedAt: now,
		},
		FindingCount: 1, SeverityCounts: map[string]int{"CRITICAL": 1},
		SBOMFormat: security.CycloneDXFormat, ScannedAt: &now, SBOMCreatedAt: &now,
		Signature: &security.VerificationDetail{
			Workflow: security.VerificationWorkflow{
				ID: "signature-workflow", Target: target, PolicyID: "policy",
				PolicyVersion: 2, JobID: "signature-job", CreatedAt: now,
			},
			Status:        security.ResultStatus{State: security.ResultCompleted, Attempts: 1, UpdatedAt: now},
			CosignVersion: "v3.0.6", CompletedAt: &now,
			Evidence: []security.SignatureEvidence{{
				CryptographicEvidence: security.CryptographicEvidence{
					SignatureDigest: target.Digest, Kind: security.SignatureKindSignature,
					SignerType:     security.SignerPublicKey,
					KeyFingerprint: securityHandlerDigest, State: security.CryptoValid,
				},
				TrustState: security.TrustTrusted, Reason: security.SignatureReasonTrustedKey,
			}},
		},
	}
	securityService := &handlerSecurity{detail: detail}
	handler := testSecurityHandler(t, &handlerRepositories{
		repository: repositories.Repository{ID: securityHandlerRepositoryID},
	}, securityService)
	request := authenticatedSecurityRequest(
		"/api/v1/namespaces/team/repositories/image/artifacts/" + securityHandlerDigest + "/security",
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{
		`"digest":"` + securityHandlerDigest + `"`, `"state":"STALE"`,
		`"scanner_version":"0.72.0"`, `"database_schema_version":2`,
		`"finding_count":1`, `"CRITICAL":1`, `"format":"CYCLONEDX_JSON"`,
		`"cosign_version":"v3.0.6"`, `"policy_version":2`,
		`"cryptographic_state":"VALID"`, `"trust_state":"TRUSTED"`,
	} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("body = %s, want %q", recorder.Body.String(), expected)
		}
	}
}

func TestSecurityRouteShowsQueuedAndFailedWithoutInventingEvidence(t *testing.T) {
	now := time.Now().UTC()
	target, _ := security.NewTarget(securityHandlerRepositoryID, "team", "image", securityHandlerDigest)
	detail := security.Detail{
		Workflow: security.Workflow{ID: "workflow", Target: target, ScanJobID: "scan", SBOMJobID: "sbom", CreatedAt: now, UpdatedAt: now},
		Scan:     security.ResultStatus{State: security.ResultQueued, UpdatedAt: now},
		SBOM:     security.ResultStatus{State: security.ResultFailed, ErrorCode: "SCANNER_UNAVAILABLE", Attempts: 3, UpdatedAt: now},
	}
	handler := testSecurityHandler(t, &handlerRepositories{
		repository: repositories.Repository{ID: securityHandlerRepositoryID},
	}, &handlerSecurity{detail: detail})
	request := authenticatedSecurityRequest(
		"/api/v1/namespaces/team/repositories/image/artifacts/" + securityHandlerDigest + "/security",
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"QUEUED"`) ||
		!strings.Contains(recorder.Body.String(), `"error_code":"SCANNER_UNAVAILABLE"`) ||
		!strings.Contains(recorder.Body.String(), `"signature":{"state":"ABSENT","evidence":[]}`) ||
		strings.Contains(recorder.Body.String(), `"tool"`) || strings.Contains(recorder.Body.String(), `"finding_count"`) {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSecurityRouteShowsRunningState(t *testing.T) {
	now := time.Now().UTC()
	target, _ := security.NewTarget(securityHandlerRepositoryID, "team", "image", securityHandlerDigest)
	detail := security.Detail{
		Workflow: security.Workflow{ID: "workflow", Target: target, ScanJobID: "scan", SBOMJobID: "sbom", CreatedAt: now, UpdatedAt: now},
		Scan:     security.ResultStatus{State: security.ResultRunning, Attempts: 1, UpdatedAt: now},
		SBOM:     security.ResultStatus{State: security.ResultQueued, UpdatedAt: now},
	}
	handler := testSecurityHandler(t, &handlerRepositories{
		repository: repositories.Repository{ID: securityHandlerRepositoryID},
	}, &handlerSecurity{detail: detail})
	request := authenticatedSecurityRequest(
		"/api/v1/namespaces/team/repositories/image/artifacts/" + securityHandlerDigest + "/security",
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"RUNNING"`) ||
		!strings.Contains(recorder.Body.String(), `"attempts":1`) {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSecurityRouteAuthenticatesAndHidesPrivateRepository(t *testing.T) {
	repositoriesService := &handlerRepositories{err: repositories.ErrNotFound}
	securityService := &handlerSecurity{}
	handler := testSecurityHandler(t, repositoriesService, securityService)

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/team/repositories/image/artifacts/"+securityHandlerDigest+"/security", nil)
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusUnauthorized || repositoriesService.calls != 0 {
		t.Fatalf("missing session status/calls = %d %d", missingRecorder.Code, repositoriesService.calls)
	}

	privateRequest := authenticatedSecurityRequest(
		"/api/v1/namespaces/team/repositories/image/artifacts/" + securityHandlerDigest + "/security",
	)
	privateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(privateRecorder, privateRequest)
	if privateRecorder.Code != http.StatusNotFound || securityService.calls != 0 {
		t.Fatalf("private status/security calls = %d %d", privateRecorder.Code, securityService.calls)
	}
}

func TestSecurityRouteMapsMissingWorkflowAndHidesInternalError(t *testing.T) {
	repositoryService := &handlerRepositories{repository: repositories.Repository{ID: securityHandlerRepositoryID}}
	securityService := &handlerSecurity{err: security.ErrNotFound}
	handler := testSecurityHandler(t, repositoryService, securityService)
	path := "/api/v1/namespaces/team/repositories/image/artifacts/" + securityHandlerDigest + "/security"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedSecurityRequest(path))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing workflow status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	securityService.err = errors.New("database password leaked")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedSecurityRequest(path))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "database password") {
		t.Fatalf("internal status/body = %d %s", recorder.Code, recorder.Body.String())
	}
}

func testSecurityHandler(
	t *testing.T,
	repositoriesService *handlerRepositories,
	securityService *handlerSecurity,
) http.Handler {
	t.Helper()
	handler, err := New(
		&handlerAuthenticator{user: auth.User{ID: auth.ID(securityHandlerUserID)}},
		repositoriesService, securityService,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	router := httpapi.NewRouter()
	RegisterRoutes(router, handler)
	return httpapi.WithRequestID(router)
}

func authenticatedSecurityRequest(path string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: authhandler.SessionCookieName, Value: "session"})
	return request
}

type handlerAuthenticator struct {
	user auth.User
	err  error
}

func (a *handlerAuthenticator) Authenticate(context.Context, string) (auth.User, error) {
	return a.user, a.err
}

type handlerRepositories struct {
	repository repositories.Repository
	err        error
	calls      int
}

func (r *handlerRepositories) Detail(context.Context, string, string, string) (repositories.Repository, error) {
	r.calls++
	return r.repository, r.err
}

type handlerSecurity struct {
	detail security.Detail
	err    error
	calls  int
}

func (s *handlerSecurity) Detail(context.Context, string, string) (security.Detail, error) {
	s.calls++
	return s.detail, s.err
}
