package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRouterContract(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		contentType  string
		handler      HandlerFunc
		wantStatus   int
		wantCode     string
		wantRequest  string
		requestID    string
		wantResponse string
	}{
		{
			name:        "malformed JSON",
			method:      http.MethodPost,
			path:        "/api/v1/example",
			body:        "{",
			contentType: "application/json",
			handler: func(w http.ResponseWriter, request *http.Request) error {
				var input struct{ Name string }
				return DecodeJSON(w, request, &input)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:         "internal failure",
			method:       http.MethodGet,
			path:         "/api/v1/example",
			handler:      func(http.ResponseWriter, *http.Request) error { return errors.New("database secret") },
			wantStatus:   http.StatusInternalServerError,
			wantCode:     CodeInternal,
			wantResponse: "an internal error occurred",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter()
			router.Handle(test.method, test.path, test.handler)
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set(RequestIDHeader, "request-123")
			recorder := httptest.NewRecorder()
			WithRequestID(Recover(router)).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("body = %s, want code %q", recorder.Body.String(), test.wantCode)
			}
			if strings.Contains(recorder.Body.String(), "database secret") {
				t.Fatalf("body leaked internal error: %s", recorder.Body.String())
			}
			if got := recorder.Header().Get(RequestIDHeader); got != "request-123" {
				t.Fatalf("request ID = %q, want request-123", got)
			}
		})
	}
}

func TestRouterMethodNotAllowedAndNotFound(t *testing.T) {
	router := NewRouter()
	router.Handle(http.MethodPost, "/api/v1/example", func(http.ResponseWriter, *http.Request) error { return nil })
	handler := WithRequestID(router)

	tests := []struct {
		path       string
		wantStatus int
		wantCode   string
	}{
		{path: "/api/v1/example", wantStatus: http.StatusMethodNotAllowed, wantCode: CodeMethodNotAllowed},
		{path: "/api/v1/missing", wantStatus: http.StatusNotFound, wantCode: CodeNotFound},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantCode) {
			t.Fatalf("%s: status/body = %d %s", test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRouterProtocolPathOwnsMethodAndResponseContract(t *testing.T) {
	router := NewRouter()
	router.HandleProtocol("/token", http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/registry-error+json")
		w.WriteHeader(http.StatusTeapot)
	}))
	request := httptest.NewRequest(http.MethodPost, "/token", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTeapot ||
		recorder.Header().Get("Content-Type") != "application/registry-error+json" {
		t.Fatalf("protocol response = %d %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
}

func TestRequestIDReplacesInvalidValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "invalid request id with spaces")
	recorder := httptest.NewRecorder()
	WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if RequestID(request.Context()) == "" {
			t.Fatal("request ID missing from context")
		}
	})).ServeHTTP(recorder, request)
	if got := recorder.Header().Get(RequestIDHeader); len(got) != 32 {
		t.Fatalf("generated request ID length = %d, want 32", len(got))
	}
}

func TestParsePageAndFormatTime(t *testing.T) {
	page, err := ParsePage(map[string][]string{"limit": {"50"}, "cursor": {"opaque"}})
	if err != nil || page.Limit != 50 || page.Cursor != "opaque" {
		t.Fatalf("ParsePage() = %#v, %v", page, err)
	}
	if _, err := ParsePage(map[string][]string{"limit": {"101"}}); err == nil {
		t.Fatal("ParsePage() error = nil, want an error")
	}
	value := time.Date(2026, time.August, 1, 12, 0, 0, 123, time.FixedZone("test", 8*60*60))
	if got := FormatTime(value); got != "2026-08-01T04:00:00.000000123Z" {
		t.Fatalf("FormatTime() = %q", got)
	}
}
