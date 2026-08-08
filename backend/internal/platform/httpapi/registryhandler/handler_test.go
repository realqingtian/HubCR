package registryhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/platform/httpapi"
	"hubcr.io/hubcr/internal/platform/observability"
)

func TestTokenHandlerSuccessContractAndCredentialIsolation(t *testing.T) {
	var logs bytes.Buffer
	issuer := &handlerIssuer{result: registry.IssueResult{
		Token: "signed-secret-token", ExpiresIn: 300,
		IssuedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		Subject:  registry.Subject{ID: "opaque-user-id"},
		Access: []registry.Access{
			{
				Type: registry.ResourceRepository, Name: "team/alpha",
				Actions: []registry.Action{registry.ActionPull},
			},
		},
		KeyID: "test-key",
	}}
	handler := newTestHandler(t, issuer, &logs)
	request := httptest.NewRequest(
		http.MethodGet,
		"/token?service=hubcr-registry&scope=repository%3Ateam%2Fzeta%3Apull"+
			"&scope=repository%3Ateam%2Falpha%3Apull%2Cpush&client_id=docker&account=owner",
		nil,
	)
	request.SetBasicAuth("owner", "super-secret-password")
	request.Header.Set("Cookie", "hubcr_session=must-be-ignored")
	request.Header.Set(httpapi.RequestIDHeader, "registry-request")
	recorder := httptest.NewRecorder()
	httpapi.WithRequestID(handler).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProtocolHeaders(t, recorder)
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("token endpoint enabled CORS")
	}
	var response tokenResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Token != issuer.result.Token ||
		response.AccessToken != issuer.result.Token ||
		response.ExpiresIn != 300 ||
		response.IssuedAt != "2026-08-01T12:00:00Z" {
		t.Fatalf("response = %#v", response)
	}
	if issuer.request.Service != "hubcr-registry" ||
		issuer.request.ClientID != "docker" ||
		len(issuer.request.RawScopes) != 2 ||
		issuer.username != "owner" ||
		issuer.password != "super-secret-password" {
		t.Fatalf("issuer request = %#v, credentials %q %q", issuer.request, issuer.username, issuer.password)
	}
	logged := logs.String()
	for _, secret := range []string{
		"signed-secret-token", "super-secret-password", "hubcr_session", "Authorization",
	} {
		if strings.Contains(logged, secret) {
			t.Fatalf("logs leaked %q: %s", secret, logged)
		}
	}
}

func TestTokenHandlerAcceptsOfflineTokenWithoutIssuingRefreshToken(t *testing.T) {
	issuer := &handlerIssuer{result: registry.IssueResult{
		Token: "short-lived-token", ExpiresIn: 300,
		IssuedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}}
	handler := newTestHandler(t, issuer, &bytes.Buffer{})
	request := httptest.NewRequest(
		http.MethodGet,
		"/token?service=hubcr-registry&offline_token=true",
		nil,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "refresh_token") {
		t.Fatalf("response unexpectedly contains a refresh token: %s", recorder.Body.String())
	}
}

func TestTokenHandlerRecordsBoundedPolicyDecisionMetricsAndLogs(t *testing.T) {
	metrics := observability.NewRegistryMetrics()
	var logs bytes.Buffer
	handler, err := New(
		&handlerIssuer{result: registry.IssueResult{
			Token: "opaque-token", ExpiresIn: 300, IssuedAt: time.Now(),
			Access: []registry.Access{{
				Type: registry.ResourceRepository, Name: "team/image",
				Actions: []registry.Action{registry.ActionPull},
			}},
		}},
		slog.New(slog.NewJSONHandler(&logs, nil)),
		metrics,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"/token?service=hubcr-registry&scope=repository%3Ateam%2Fimage%3Apull%2Cpush",
		nil,
	)
	request.Header.Set(httpapi.RequestIDHeader, "token-decision-request")
	recorder := httptest.NewRecorder()
	httpapi.WithRequestID(handler).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	metricBody := scrapeMetrics(t, metrics)
	for _, line := range []string{
		`hubcr_registry_token_requests_total{outcome="issued"} 1`,
		`hubcr_registry_token_actions_total{action="pull",decision="granted"} 1`,
		`hubcr_registry_token_actions_total{action="push",decision="denied"} 1`,
	} {
		if !strings.Contains(metricBody, line+"\n") {
			t.Fatalf("metrics omitted %q:\n%s", line, metricBody)
		}
	}
	logged := logs.String()
	for _, field := range []string{
		`"request_id":"token-decision-request"`,
		`"outcome":"issued"`,
		`"requested_action_count":2`,
		`"granted_action_count":1`,
		`"denied_action_count":1`,
	} {
		if !strings.Contains(logged, field) {
			t.Fatalf("structured log omitted %q: %s", field, logged)
		}
	}
	if strings.Contains(logged, "team/image") || strings.Contains(logged, "opaque-token") {
		t.Fatalf("structured log included repository or token content: %s", logged)
	}
}

func TestTokenHandlerProtocolErrors(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		authorize  func(*http.Request)
		issuerErr  error
		wantStatus int
		wantCode   string
	}{
		{
			name: "missing service", method: http.MethodGet, target: "/token",
			wantStatus: http.StatusBadRequest, wantCode: "DENIED",
		},
		{
			name: "repeated service", method: http.MethodGet,
			target:     "/token?service=one&service=two",
			wantStatus: http.StatusBadRequest, wantCode: "DENIED",
		},
		{
			name: "malformed query", method: http.MethodGet,
			target:     "/token?service=hubcr-registry&scope=%zz",
			wantStatus: http.StatusBadRequest, wantCode: "DENIED",
		},
		{
			name: "invalid offline token", method: http.MethodGet,
			target:     "/token?service=hubcr-registry&offline_token=invalid",
			wantStatus: http.StatusBadRequest, wantCode: "DENIED",
		},
		{
			name: "bearer credential", method: http.MethodGet,
			target: "/token?service=hubcr-registry",
			authorize: func(request *http.Request) {
				request.Header.Set("Authorization", "Bearer secret")
			},
			wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED",
		},
		{
			name: "oversized Basic credential", method: http.MethodGet,
			target: "/token?service=hubcr-registry",
			authorize: func(request *http.Request) {
				request.SetBasicAuth("owner", strings.Repeat("x", maxPasswordBytes+1))
			},
			wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED",
		},
		{
			name: "invalid Basic credential", method: http.MethodGet,
			target: "/token?service=hubcr-registry",
			authorize: func(request *http.Request) {
				request.SetBasicAuth("owner", "wrong")
			},
			issuerErr:  registry.ErrInvalidCredentials,
			wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED",
		},
		{
			name: "rate limited credential", method: http.MethodGet,
			target:     "/token?service=hubcr-registry",
			issuerErr:  registry.ErrRateLimited,
			wantStatus: http.StatusTooManyRequests, wantCode: "TOOMANYREQUESTS",
		},
		{
			name: "dependency unavailable", method: http.MethodGet,
			target:     "/token?service=hubcr-registry",
			issuerErr:  errors.New("wrapped: " + registry.ErrUnavailable.Error()),
			wantStatus: http.StatusInternalServerError, wantCode: "UNKNOWN",
		},
		{
			name: "method not allowed", method: http.MethodPost,
			target:     "/token?service=hubcr-registry",
			wantStatus: http.StatusMethodNotAllowed, wantCode: "UNSUPPORTED",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			issuer := &handlerIssuer{err: test.issuerErr}
			handler := newTestHandler(t, issuer, &logs)
			request := httptest.NewRequest(test.method, test.target, nil)
			if test.authorize != nil {
				test.authorize(request)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus ||
				!strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			assertProtocolHeaders(t, recorder)
			if test.wantStatus == http.StatusUnauthorized &&
				recorder.Header().Get("WWW-Authenticate") != `Basic realm="HubCR Registry"` {
				t.Fatalf("WWW-Authenticate = %q", recorder.Header().Get("WWW-Authenticate"))
			}
			if test.wantStatus == http.StatusMethodNotAllowed &&
				recorder.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow = %q", recorder.Header().Get("Allow"))
			}
			if test.wantStatus == http.StatusTooManyRequests && recorder.Header().Get("Retry-After") != "60" {
				t.Fatalf("Retry-After = %q", recorder.Header().Get("Retry-After"))
			}
		})
	}
}

func TestTokenHandlerMapsWrappedUnavailableAndRecoversWithRegistryEnvelope(t *testing.T) {
	t.Run("wrapped unavailable", func(t *testing.T) {
		handler := newTestHandler(
			t,
			&handlerIssuer{err: errors.Join(errors.New("database"), registry.ErrUnavailable)},
			&bytes.Buffer{},
		)
		request := httptest.NewRequest(
			http.MethodGet, "/token?service=hubcr-registry", nil,
		)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable ||
			!strings.Contains(recorder.Body.String(), `"code":"UNAVAILABLE"`) {
			t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("panic", func(t *testing.T) {
		handler := newTestHandler(t, panicIssuer{}, &bytes.Buffer{})
		request := httptest.NewRequest(
			http.MethodGet, "/token?service=hubcr-registry", nil,
		)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError ||
			!strings.Contains(recorder.Body.String(), `"code":"UNKNOWN"`) {
			t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
		}
	})
}

func newTestHandler(t *testing.T, issuer TokenIssuer, logs *bytes.Buffer) *Handler {
	t.Helper()
	handler, err := New(
		issuer,
		slog.New(slog.NewJSONHandler(logs, nil)),
		observability.NewRegistryMetrics(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}

func assertProtocolHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("Content-Type") != "application/json" ||
		recorder.Header().Get("Cache-Control") != "no-store" ||
		recorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("protocol headers = %#v", recorder.Header())
	}
}

func scrapeMetrics(t *testing.T, metrics *observability.RegistryMetrics) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, observability.RegistryMetricsPath, nil)
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", recorder.Code)
	}
	return recorder.Body.String()
}

type handlerIssuer struct {
	request  registry.IssueRequest
	result   registry.IssueResult
	err      error
	username string
	password string
}

func (i *handlerIssuer) Issue(
	_ context.Context,
	request registry.IssueRequest,
) (registry.IssueResult, error) {
	i.request = request
	if request.Credentials != nil {
		i.username = request.Credentials.Username
		i.password = string(request.Credentials.Password)
	}
	return i.result, i.err
}

type panicIssuer struct{}

func (panicIssuer) Issue(context.Context, registry.IssueRequest) (registry.IssueResult, error) {
	panic("secret panic")
}
