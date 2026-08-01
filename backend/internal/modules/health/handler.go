package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type response struct {
	Status string `json:"status"`
}

type Checker interface {
	Check(context.Context) error
}

type handler struct {
	readinessTimeout time.Duration
	checkers         []Checker
}

type RouteRegistrar interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}

func RegisterRoutes(mux RouteRegistrar, readinessTimeout time.Duration, checkers ...Checker) {
	h := handler{readinessTimeout: readinessTimeout, checkers: checkers}
	mux.HandleFunc("GET /api/v1/health/live", handleOK)
	mux.HandleFunc("GET /api/v1/health/ready", h.handleReady)
}

func handleOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response{Status: "ok"})
}

func (h handler) handleReady(w http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), h.readinessTimeout)
	defer cancel()

	for _, checker := range h.checkers {
		if err := checker.Check(ctx); err != nil {
			writeStatus(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
	}
	writeStatus(w, http.StatusOK, "ok")
}

func writeStatus(w http.ResponseWriter, statusCode int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response{Status: status})
}
