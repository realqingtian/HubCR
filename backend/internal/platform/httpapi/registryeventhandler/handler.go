package registryeventhandler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/platform/httpapi"
)

const (
	RegistryEventPath        = "/internal/registry/events"
	maxNotificationBodyBytes = 1024 * 1024
	minEventTokenBytes       = 32
	maxEventTokenBytes       = 512
)

type NotificationProcessor interface {
	Process(context.Context, registry.NotificationEnvelope) (registry.NotificationResult, error)
}

type Handler struct {
	processor   NotificationProcessor
	tokenDigest [sha256.Size]byte
	logger      *slog.Logger
}

func New(processor NotificationProcessor, token []byte, logger *slog.Logger) (*Handler, error) {
	if processor == nil || logger == nil || !validEventToken(token) {
		return nil, errors.New("Registry event handler dependencies must be configured")
	}
	return &Handler{
		processor: processor, tokenDigest: sha256.Sum256(token), logger: logger,
	}, nil
}

func RegisterRoutes(router *httpapi.Router, handler *Handler) {
	router.HandleProtocol(RegistryEventPath, handler)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	setHeaders(w)
	defer func() {
		if recover() != nil {
			h.writeFailure(w, request, http.StatusInternalServerError, "UNKNOWN")
		}
	}()
	if !h.authenticate(request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="HubCR Distribution events"`)
		h.writeFailure(w, request, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		h.writeFailure(w, request, http.StatusMethodNotAllowed, "UNSUPPORTED")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != registry.NotificationEventsMediaType {
		h.writeFailure(w, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE")
		return
	}
	if request.ContentLength > maxNotificationBodyBytes {
		h.writeFailure(w, request, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
		return
	}

	request.Body = http.MaxBytesReader(w, request.Body, maxNotificationBodyBytes)
	decoder := json.NewDecoder(request.Body)
	var envelope registry.NotificationEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		h.writeDecodeFailure(w, request, err)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		h.writeDecodeFailure(w, request, err)
		return
	}

	result, err := h.processor.Process(request.Context(), envelope)
	if err != nil {
		h.writeProcessFailure(w, request, err)
		return
	}
	h.logger.InfoContext(
		request.Context(),
		"Registry notification accepted",
		"request_id", httpapi.RequestID(request.Context()),
		"processed", result.Processed,
		"ignored", result.Ignored,
		"status", http.StatusAccepted,
	)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) authenticate(request *http.Request) bool {
	values := request.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > len("Bearer ")+maxEventTokenBytes {
		return false
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !validEventToken([]byte(token)) {
		return false
	}
	provided := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(provided[:], h.tokenDigest[:]) == 1
}

func (h *Handler) writeDecodeFailure(w http.ResponseWriter, request *http.Request, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		h.writeFailure(w, request, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
		return
	}
	h.writeFailure(w, request, http.StatusBadRequest, "INVALID")
}

func (h *Handler) writeProcessFailure(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, registry.ErrInvalidNotification):
		h.writeFailure(w, request, http.StatusBadRequest, "INVALID")
	case errors.Is(err, registry.ErrNotificationConflict):
		h.writeFailure(w, request, http.StatusConflict, "CONFLICT")
	case errors.Is(err, registry.ErrNotificationUnavailable):
		h.writeFailure(w, request, http.StatusServiceUnavailable, "UNAVAILABLE")
	default:
		h.writeFailure(w, request, http.StatusInternalServerError, "UNKNOWN")
	}
}

func (h *Handler) writeFailure(
	w http.ResponseWriter,
	request *http.Request,
	status int,
	errorClass string,
) {
	h.logger.WarnContext(
		request.Context(),
		"Registry notification rejected",
		"request_id", httpapi.RequestID(request.Context()),
		"error_class", errorClass,
		"status", status,
	)
	w.WriteHeader(status)
}

func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func validEventToken(token []byte) bool {
	if len(token) < minEventTokenBytes || len(token) > maxEventTokenBytes {
		return false
	}
	for _, character := range token {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}
