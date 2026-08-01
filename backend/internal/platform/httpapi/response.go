package httpapi

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Fields  []FieldError `json:"fields,omitempty"`
}

type errorEnvelope struct {
	Error     errorBody `json:"error"`
	RequestID string    `json:"request_id"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func WriteError(w http.ResponseWriter, request *http.Request, err error) {
	apiError := classifyError(err)
	WriteJSON(w, apiError.Status, errorEnvelope{
		Error: errorBody{
			Code:    apiError.Code,
			Message: apiError.Message,
			Fields:  apiError.Fields,
		},
		RequestID: RequestID(request.Context()),
	})
}
