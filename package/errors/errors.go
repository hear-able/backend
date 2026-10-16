// Package errors defines the application error type that handlers translate
// into HTTP responses. Anything that is not an *AppError is treated as an
// unexpected internal error and never shown to clients verbatim.
package errors

import (
	"errors"
	"net/http"
)

type AppError struct {
	Status    int    `json:"-"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
	Err       error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

// As reports whether err is (or wraps) an *AppError.
func As(err error) (*AppError, bool) {
	var appErr *AppError
	ok := errors.As(err, &appErr)
	return appErr, ok
}

func New(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

func BadRequest(message string) *AppError {
	return New(http.StatusBadRequest, "bad_request", message)
}

func Unauthorized(message string) *AppError {
	return New(http.StatusUnauthorized, "unauthorized", message)
}

func Forbidden(message string) *AppError {
	return New(http.StatusForbidden, "forbidden", message)
}

func NotFound(message string) *AppError {
	return New(http.StatusNotFound, "not_found", message)
}

func Conflict(message string) *AppError {
	return New(http.StatusConflict, "conflict", message)
}

func Unprocessable(message string) *AppError {
	return New(http.StatusUnprocessableEntity, "unprocessable_entity", message)
}

func TooManyRequests(message string) *AppError {
	return New(http.StatusTooManyRequests, "too_many_requests", message)
}

// Upstream marks a failure of an external dependency (e.g. the AI provider).
// The client may retry the same request.
func Upstream(status int, message string, err error) *AppError {
	return &AppError{Status: status, Code: "upstream_error", Message: message, Retryable: true, Err: err}
}
