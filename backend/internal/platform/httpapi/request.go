package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

const maxJSONBodyBytes int64 = 1 << 20

func DecodeJSON(w http.ResponseWriter, request *http.Request, destination any) *Error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return InvalidRequest("Content-Type must be application/json")
	}

	request.Body = http.MaxBytesReader(w, request.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return InvalidRequest("request body must not be empty")
		}
		return InvalidRequest("request body must contain valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return InvalidRequest("request body must contain one JSON value")
	}
	return nil
}
