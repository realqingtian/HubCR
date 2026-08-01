package authhandler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"hubcr.io/hubcr/internal/modules/auth"
	"hubcr.io/hubcr/internal/platform/httpapi"
)

const SessionCookieName = "hubcr_session"

type Authenticator interface {
	Login(context.Context, auth.LoginInput) (auth.LoginResult, error)
	Authenticate(context.Context, string) (auth.User, error)
	Logout(context.Context, string) error
}

type Handler struct {
	authenticator Authenticator
	cookieSecure  bool
}

func New(authenticator Authenticator, cookieSecure bool) *Handler {
	return &Handler{authenticator: authenticator, cookieSecure: cookieSecure}
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.Handle(http.MethodPost, "/api/v1/auth/login", handler.login)
	router.Handle(http.MethodPost, "/api/v1/auth/logout", handler.logout)
	router.Handle(http.MethodGet, "/api/v1/auth/me", handler.currentUser)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	PersonalNamespace string `json:"personal_namespace"`
	CreatedAt         string `json:"created_at"`
}

type loginResponse struct {
	User      userResponse `json:"user"`
	ExpiresAt string       `json:"expires_at"`
}

func (h *Handler) login(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	var input loginRequest
	if err := httpapi.DecodeJSON(w, request, &input); err != nil {
		return err
	}
	fields := validateLogin(input)
	if len(fields) > 0 {
		return httpapi.ValidationFailed(fields...)
	}

	password := []byte(input.Password)
	defer clear(password)
	result, err := h.authenticator.Login(request.Context(), auth.LoginInput{
		Username:     input.Username,
		Password:     password,
		RateLimitKey: clientKey(request),
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			return httpapi.AuthenticationFailed()
		case errors.Is(err, auth.ErrRateLimited):
			return httpapi.RateLimited()
		default:
			return err
		}
	}

	h.setSessionCookie(w, result.Token, result.ExpiresAt)
	httpapi.WriteJSON(w, http.StatusOK, loginResponse{
		User:      mapUser(result.User),
		ExpiresAt: httpapi.FormatTime(result.ExpiresAt),
	})
	return nil
}

func (h *Handler) currentUser(w http.ResponseWriter, request *http.Request) error {
	token, err := sessionToken(request)
	if err != nil {
		return httpapi.AuthenticationFailed()
	}
	user, err := h.authenticator.Authenticate(request.Context(), token)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			return httpapi.AuthenticationFailed()
		}
		return err
	}
	httpapi.WriteJSON(w, http.StatusOK, mapUser(user))
	return nil
}

func (h *Handler) logout(w http.ResponseWriter, request *http.Request) error {
	if err := rejectCrossSite(request); err != nil {
		return err
	}
	token, _ := sessionToken(request)
	if err := h.authenticator.Logout(request.Context(), token); err != nil {
		return err
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func validateLogin(input loginRequest) []httpapi.FieldError {
	fields := make([]httpapi.FieldError, 0, 2)
	if len(input.Username) < 1 || len(input.Username) > 64 {
		fields = append(fields, httpapi.FieldError{Field: "username", Message: "must contain 1 to 64 characters"})
	}
	if len(input.Password) < 1 || len(input.Password) > 1024 {
		fields = append(fields, httpapi.FieldError{Field: "password", Message: "must contain 1 to 1024 bytes"})
	}
	return fields
}

func mapUser(user auth.User) userResponse {
	return userResponse{
		ID:                string(user.ID),
		Username:          user.Username,
		PersonalNamespace: user.PersonalNamespace,
		CreatedAt:         httpapi.FormatTime(user.CreatedAt),
	}
}

func sessionToken(request *http.Request) (string, error) {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", http.ErrNoCookie
	}
	return cookie.Value, nil
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func rejectCrossSite(request *http.Request) *httpapi.Error {
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return httpapi.InvalidRequest("cross-site authentication request rejected")
	}
	return nil
}

func clientKey(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}
