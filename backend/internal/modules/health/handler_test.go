package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveRoute(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}
