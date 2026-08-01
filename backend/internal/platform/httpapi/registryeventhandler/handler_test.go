package registryeventhandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/platform/httpapi"
)

const registryEventTestToken = "0123456789abcdef0123456789abcdef"

func TestHandlerAcceptsAuthenticatedNotification(t *testing.T) {
	processor := &eventProcessor{result: registry.NotificationResult{Processed: 1, Ignored: 2}}
	handler := newEventHandler(t, processor)
	envelope := registry.NotificationEnvelope{Events: []registry.NotificationEvent{{
		ID: "event-id", Timestamp: time.Now(), Action: registry.NotificationActionPush,
		Target: registry.NotificationTarget{
			MediaType:  registry.OCIImageManifestMediaType,
			Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Repository: "team/image",
		},
	}}}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, RegistryEventPath, strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+registryEventTestToken)
	request.Header.Set("Content-Type", registry.NotificationEventsMediaType)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || processor.calls != 1 || len(processor.envelope.Events) != 1 {
		t.Fatalf("response/calls = %d/%d envelope=%#v", recorder.Code, processor.calls, processor.envelope)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}
}

func TestHandlerRejectsAuthenticationBeforeProcessing(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
	}{
		{name: "missing"},
		{name: "wrong", headers: []string{"Bearer wrong-token-value-that-is-long-enough"}},
		{name: "basic", headers: []string{"Basic " + registryEventTestToken}},
		{name: "duplicates", headers: []string{"Bearer " + registryEventTestToken, "Bearer " + registryEventTestToken}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processor := &eventProcessor{}
			handler := newEventHandler(t, processor)
			request := httptest.NewRequest(http.MethodPost, RegistryEventPath, strings.NewReader(`{"events":[]}`))
			request.Header.Set("Content-Type", registry.NotificationEventsMediaType)
			for _, header := range test.headers {
				request.Header.Add("Authorization", header)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized || processor.calls != 0 {
				t.Fatalf("response/calls = %d/%d, want 401/0", recorder.Code, processor.calls)
			}
			if !strings.HasPrefix(recorder.Header().Get("WWW-Authenticate"), "Bearer ") {
				t.Fatalf("WWW-Authenticate = %q", recorder.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestHandlerValidatesMethodMediaTypeAndBody(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		contentType string
		body        string
		wantStatus  int
	}{
		{name: "method", method: http.MethodGet, contentType: registry.NotificationEventsMediaType, body: `{}`, wantStatus: http.StatusMethodNotAllowed},
		{name: "media type", method: http.MethodPost, contentType: "application/json", body: `{}`, wantStatus: http.StatusUnsupportedMediaType},
		{name: "malformed", method: http.MethodPost, contentType: registry.NotificationEventsMediaType, body: `{"events":`, wantStatus: http.StatusBadRequest},
		{name: "trailing body", method: http.MethodPost, contentType: registry.NotificationEventsMediaType, body: `{"events":[]} {}`, wantStatus: http.StatusBadRequest},
		{name: "oversized", method: http.MethodPost, contentType: registry.NotificationEventsMediaType, body: strings.Repeat("x", maxNotificationBodyBytes+1), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processor := &eventProcessor{}
			handler := newEventHandler(t, processor)
			request := httptest.NewRequest(test.method, RegistryEventPath, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer "+registryEventTestToken)
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || processor.calls != 0 {
				t.Fatalf("response/calls = %d/%d, want %d/0", recorder.Code, processor.calls, test.wantStatus)
			}
		})
	}
}

func TestHandlerMapsReconciliationFailuresForDistributionRetry(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid", err: registry.ErrInvalidNotification, wantStatus: http.StatusBadRequest},
		{name: "conflict", err: registry.ErrNotificationConflict, wantStatus: http.StatusConflict},
		{name: "unavailable", err: registry.ErrNotificationUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "unknown", err: errors.New("unexpected"), wantStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processor := &eventProcessor{err: test.err}
			handler := newEventHandler(t, processor)
			request := httptest.NewRequest(http.MethodPost, RegistryEventPath, strings.NewReader(`{"events":[{}]}`))
			request.Header.Set("Authorization", "Bearer "+registryEventTestToken)
			request.Header.Set("Content-Type", registry.NotificationEventsMediaType)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || processor.calls != 1 {
				t.Fatalf("response/calls = %d/%d, want %d/1", recorder.Code, processor.calls, test.wantStatus)
			}
		})
	}
}

func TestRegisterRoutesAndConstructorValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	processor := &eventProcessor{}
	if _, err := New(nil, []byte(registryEventTestToken), logger); err == nil {
		t.Fatal("New(nil processor) error = nil")
	}
	if _, err := New(processor, []byte("short"), logger); err == nil {
		t.Fatal("New(short token) error = nil")
	}
	handler := newEventHandler(t, processor)
	router := httpapi.NewRouter()
	RegisterRoutes(router, handler)
	request := httptest.NewRequest(http.MethodPost, RegistryEventPath, strings.NewReader(`{"events":[{}]}`))
	request.Header.Set("Authorization", "Bearer "+registryEventTestToken)
	request.Header.Set("Content-Type", registry.NotificationEventsMediaType)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("registered route status = %d, want 202", recorder.Code)
	}
}

func newEventHandler(t *testing.T, processor NotificationProcessor) *Handler {
	t.Helper()
	handler, err := New(
		processor,
		[]byte(registryEventTestToken),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}

type eventProcessor struct {
	calls    int
	envelope registry.NotificationEnvelope
	result   registry.NotificationResult
	err      error
}

func (p *eventProcessor) Process(
	_ context.Context,
	envelope registry.NotificationEnvelope,
) (registry.NotificationResult, error) {
	p.calls++
	p.envelope = envelope
	return p.result, p.err
}
