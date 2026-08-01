package httpapi

import (
	"errors"
	"net/http"
)

const (
	CodeInvalidRequest   = "invalid_request"
	CodeValidationFailed = "validation_failed"
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeAuthentication   = "authentication_failed"
	CodeRateLimited      = "rate_limited"
	CodeForbidden        = "forbidden"
	CodeConflict         = "conflict"
	CodeInternal         = "internal_error"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func AuthenticationFailed() *Error {
	return &Error{Status: http.StatusUnauthorized, Code: CodeAuthentication, Message: "authentication failed"}
}

func RateLimited() *Error {
	return &Error{Status: http.StatusTooManyRequests, Code: CodeRateLimited, Message: "too many authentication attempts"}
}

func Forbidden() *Error {
	return &Error{Status: http.StatusForbidden, Code: CodeForbidden, Message: "action is not permitted"}
}

func NotFound() *Error {
	return &Error{Status: http.StatusNotFound, Code: CodeNotFound, Message: "resource not found"}
}

func Conflict(message string) *Error {
	return &Error{Status: http.StatusConflict, Code: CodeConflict, Message: message}
}

type Error struct {
	Status  int
	Code    string
	Message string
	Fields  []FieldError
	cause   error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.cause
}

func InvalidRequest(message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: CodeInvalidRequest, Message: message}
}

func ValidationFailed(fields ...FieldError) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    CodeValidationFailed,
		Message: "request validation failed",
		Fields:  fields,
	}
}

func internalError(cause error) *Error {
	return &Error{
		Status:  http.StatusInternalServerError,
		Code:    CodeInternal,
		Message: "an internal error occurred",
		cause:   cause,
	}
}

func classifyError(err error) *Error {
	var apiError *Error
	if errors.As(err, &apiError) {
		return apiError
	}
	return internalError(err)
}
