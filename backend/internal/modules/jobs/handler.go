package jobs

import (
	"context"
	"errors"
	"fmt"
)

type Handler interface {
	Handle(context.Context, Job) error
}

type HandlerFunc func(context.Context, Job) error

func (function HandlerFunc) Handle(ctx context.Context, job Job) error {
	return function(ctx, job)
}

type HandlerError struct {
	code     ErrorCode
	terminal bool
	cause    error
}

func Retryable(rawCode string, cause error) error {
	return newHandlerError(rawCode, false, cause)
}

func Permanent(rawCode string, cause error) error {
	return newHandlerError(rawCode, true, cause)
}

func newHandlerError(rawCode string, terminal bool, cause error) error {
	code, err := ParseErrorCode(rawCode)
	if err != nil || cause == nil {
		return fmt.Errorf("construct job handler failure: %w", ErrInvalidJob)
	}
	return &HandlerError{code: code, terminal: terminal, cause: cause}
}

func (e *HandlerError) Error() string { return "job handler failed" }

func (e *HandlerError) Unwrap() error { return e.cause }

func ClassifyHandlerError(err error) (ErrorCode, bool) {
	var handlerError *HandlerError
	if errors.As(err, &handlerError) {
		return handlerError.code, handlerError.terminal
	}
	return ErrorCode("HANDLER_FAILURE"), false
}
