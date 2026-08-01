package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type checkerFunc func(context.Context) error

func (f checkerFunc) Check(ctx context.Context) error {
	return f(ctx)
}

func TestLiveRoute(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, time.Second)

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

func TestReadinessFailsClosedAndRecovers(t *testing.T) {
	mux := http.NewServeMux()
	checkError := errors.New("database unavailable")
	RegisterRoutes(mux, time.Second, checkerFunc(func(context.Context) error {
		return checkError
	}))

	assertStatus := func(want int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("status = %d, want %d", recorder.Code, want)
		}
	}

	assertStatus(http.StatusServiceUnavailable)
	checkError = nil
	assertStatus(http.StatusOK)
}

func TestLivenessDoesNotDependOnReadinessChecks(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, time.Second, checkerFunc(func(context.Context) error {
		return errors.New("database unavailable")
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
