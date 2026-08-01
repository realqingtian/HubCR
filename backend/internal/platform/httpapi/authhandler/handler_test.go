package authhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/platform/httpapi"
)

func TestLoginSetsOpaqueSecureCookieWithoutReturningToken(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service := &handlerTestAuthenticator{loginResult: auth.LoginResult{
		User: auth.User{
			ID:        "11111111-1111-4111-8111-111111111111",
			Username:  "owner",
			CreatedAt: now,
		},
		Token:     "opaque-secret-token",
		ExpiresAt: now.Add(24 * time.Hour),
	}}
	handler := testHandler(service, true)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"correct"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.10:1234"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), service.loginResult.Token) {
		t.Fatal("login response exposed the session token")
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName || cookie.Value != service.loginResult.Token ||
		!cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %#v", cookie)
	}
	if service.loginInput.RateLimitKey != "192.0.2.10" {
		t.Fatalf("rate limit key = %q, want remote IP", service.loginInput.RateLimitKey)
	}
}

func TestLoginFailureIsUniformAndValidationRunsBeforeService(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "wrong credentials",
			body:       `{"username":"owner","password":"wrong"}`,
			serviceErr: auth.ErrUnauthenticated,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpapi.CodeAuthentication,
		},
		{
			name:       "rate limited",
			body:       `{"username":"owner","password":"wrong"}`,
			serviceErr: auth.ErrRateLimited,
			wantStatus: http.StatusTooManyRequests,
			wantCode:   httpapi.CodeRateLimited,
		},
		{
			name:       "invalid fields",
			body:       `{"username":"","password":""}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpapi.CodeValidationFailed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &handlerTestAuthenticator{loginError: test.serviceErr}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			testHandler(service, false).ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "owner") {
				t.Fatalf("error response disclosed username: %s", recorder.Body.String())
			}
		})
	}
}

func TestCurrentUserAndLogoutSessionHandling(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service := &handlerTestAuthenticator{user: auth.User{
		ID: "11111111-1111-4111-8111-111111111111", Username: "owner", CreatedAt: now,
	}}
	handler := testHandler(service, false)

	missingRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missingRequest)
	if missingRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing-cookie status = %d", missingRecorder.Code)
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session-token"})
	meRecorder := httptest.NewRecorder()
	handler.ServeHTTP(meRecorder, meRequest)
	if meRecorder.Code != http.StatusOK || !strings.Contains(meRecorder.Body.String(), `"username":"owner"`) {
		t.Fatalf("me status/body = %d %s", meRecorder.Code, meRecorder.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session-token"})
	logoutRecorder := httptest.NewRecorder()
	handler.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusNoContent || service.logoutToken != "session-token" {
		t.Fatalf("logout status/token = %d %q", logoutRecorder.Code, service.logoutToken)
	}
	cookies := logoutRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("logout cookie = %#v", cookies)
	}
}

func TestCrossSiteLoginIsRejected(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"correct"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	recorder := httptest.NewRecorder()
	testHandler(&handlerTestAuthenticator{}, false).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func testHandler(authenticator Authenticator, secure bool) http.Handler {
	router := httpapi.NewRouter()
	RegisterRoutes(router, New(authenticator, secure))
	return httpapi.WithRequestID(httpapi.Recover(router))
}

type handlerTestAuthenticator struct {
	loginInput  auth.LoginInput
	loginResult auth.LoginResult
	loginError  error
	user        auth.User
	authError   error
	logoutToken string
	logoutError error
}

func (a *handlerTestAuthenticator) Login(_ context.Context, input auth.LoginInput) (auth.LoginResult, error) {
	a.loginInput = input
	return a.loginResult, a.loginError
}
func (a *handlerTestAuthenticator) Authenticate(context.Context, string) (auth.User, error) {
	return a.user, a.authError
}
func (a *handlerTestAuthenticator) Logout(_ context.Context, token string) error {
	a.logoutToken = token
	return a.logoutError
}
