package health

import (
	"encoding/json"
	"net/http"
)

type response struct {
	Status string `json:"status"`
}

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health/live", handleOK)
	mux.HandleFunc("GET /api/v1/health/ready", handleOK)
}

func handleOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response{Status: "ok"})
}
