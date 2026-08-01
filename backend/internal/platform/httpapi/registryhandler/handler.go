package registryhandler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/platform/httpapi"
)

const (
	maxRawQueryBytes      = 8 * 1024
	maxAuthorizationBytes = 2 * 1024
	maxUsernameBytes      = 64
	maxPasswordBytes      = 1024
)

type TokenIssuer interface {
	Issue(context.Context, registry.IssueRequest) (registry.IssueResult, error)
}

type Handler struct {
	issuer TokenIssuer
	logger *slog.Logger
}

type tokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	IssuedAt    string `json:"issued_at"`
}

type errorResponse struct {
	Errors []errorDetail `json:"errors"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(issuer TokenIssuer, logger *slog.Logger) (*Handler, error) {
	if issuer == nil || logger == nil {
		return nil, errors.New("Registry token handler dependencies must be configured")
	}
	return &Handler{issuer: issuer, logger: logger}, nil
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.HandleProtocol("/token", handler)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	setProtocolHeaders(w)
	defer func() {
		if recover() != nil {
			h.writeError(w, request, http.StatusInternalServerError, "UNKNOWN", "an internal error occurred")
		}
	}()
	if request.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		h.writeError(w, request, http.StatusMethodNotAllowed, "UNSUPPORTED", "method not allowed")
		return
	}

	issueRequest, parseError := parseIssueRequest(request)
	if parseError != nil {
		h.writeIssueError(w, request, parseError)
		return
	}
	if issueRequest.Credentials != nil {
		defer clear(issueRequest.Credentials.Password)
	}
	result, err := h.issuer.Issue(request.Context(), issueRequest)
	if err != nil {
		h.writeIssueError(w, request, err)
		return
	}
	h.logger.InfoContext(
		request.Context(),
		"Registry token issued",
		"request_id", httpapi.RequestID(request.Context()),
		"anonymous", result.Subject.Anonymous(),
		"subject_id", result.Subject.ID,
		"access", result.Access,
		"kid", result.KeyID,
		"status", http.StatusOK,
	)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tokenResponse{
		Token: result.Token, AccessToken: result.Token,
		ExpiresIn: result.ExpiresIn,
		IssuedAt:  result.IssuedAt.UTC().Format(time.RFC3339),
	})
}

func parseIssueRequest(request *http.Request) (registry.IssueRequest, error) {
	if len(request.URL.RawQuery) > maxRawQueryBytes {
		return registry.IssueRequest{}, registry.ErrInvalidRequest
	}
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return registry.IssueRequest{}, registry.ErrInvalidRequest
	}
	services := query["service"]
	if len(services) != 1 || services[0] == "" {
		return registry.IssueRequest{}, registry.ErrInvalidRequest
	}
	clientIDs := query["client_id"]
	if len(clientIDs) > 1 {
		return registry.IssueRequest{}, registry.ErrInvalidRequest
	}
	clientID := ""
	if len(clientIDs) == 1 {
		clientID = clientIDs[0]
	}
	offlineTokens := query["offline_token"]
	if len(offlineTokens) > 1 {
		return registry.IssueRequest{}, registry.ErrInvalidRequest
	}
	if len(offlineTokens) == 1 {
		if _, err := strconv.ParseBool(offlineTokens[0]); err != nil {
			return registry.IssueRequest{}, registry.ErrInvalidRequest
		}
	}
	credentials, err := parseCredentials(request)
	if err != nil {
		return registry.IssueRequest{}, err
	}
	return registry.IssueRequest{
		Service: services[0], RawScopes: query["scope"], ClientID: clientID,
		Credentials: credentials,
	}, nil
}

func parseCredentials(request *http.Request) (*registry.Credentials, error) {
	values := request.Header.Values("Authorization")
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, registry.ErrInvalidCredentials
	}
	if len(values[0]) > maxAuthorizationBytes {
		return nil, registry.ErrInvalidCredentials
	}
	username, password, ok := request.BasicAuth()
	if !ok || len(username) < 1 || len(username) > maxUsernameBytes ||
		len(password) < 1 || len(password) > maxPasswordBytes {
		return nil, registry.ErrInvalidCredentials
	}
	return &registry.Credentials{Username: username, Password: []byte(password)}, nil
}

func (h *Handler) writeIssueError(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, registry.ErrInvalidRequest):
		h.writeError(w, request, http.StatusBadRequest, "DENIED", "invalid token request")
	case errors.Is(err, registry.ErrUnsupportedRequest):
		h.writeError(w, request, http.StatusBadRequest, "UNSUPPORTED", "token request is not supported")
	case errors.Is(err, registry.ErrInvalidCredentials):
		w.Header().Set("WWW-Authenticate", `Basic realm="HubCR Registry"`)
		h.writeError(w, request, http.StatusUnauthorized, "UNAUTHORIZED", "registry credentials are invalid")
	case errors.Is(err, registry.ErrUnavailable):
		h.writeError(w, request, http.StatusServiceUnavailable, "UNAVAILABLE", "token service is unavailable")
	default:
		h.writeError(w, request, http.StatusInternalServerError, "UNKNOWN", "an internal error occurred")
	}
}

func (h *Handler) writeError(
	w http.ResponseWriter,
	request *http.Request,
	status int,
	code, message string,
) {
	h.logger.WarnContext(
		request.Context(),
		"Registry token request failed",
		"request_id", httpapi.RequestID(request.Context()),
		"error_class", code,
		"status", status,
	)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Errors: []errorDetail{{Code: code, Message: message}},
	})
}

func setProtocolHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}
