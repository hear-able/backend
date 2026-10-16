package jsonresponder

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	apperrors "hearable/backend/package/errors"
)

const (
	JSONContentType = "application/json"
	JSONCharset     = "utf-8"
)

type JSONResponder interface {
	Write(w http.ResponseWriter, statusCode int, data interface{}) error
	WriteSimple(w http.ResponseWriter, statusCode int, message string) error
	WriteError(w http.ResponseWriter, r *http.Request, err error)
}

type jsonResponder struct {
	contentType string
}

func NewDefaultJSONResponder() JSONResponder {
	return &jsonResponder{
		contentType: fmt.Sprintf("%s; charset=%s", JSONContentType, JSONCharset),
	}
}

func (j *jsonResponder) Write(w http.ResponseWriter, statusCode int, data interface{}) error {
	if data == nil {
		w.WriteHeader(statusCode)
		return nil
	}

	// Marshal before writing headers so a marshalling failure can still
	// become a 500 and Content-Length is sent (headers set after
	// WriteHeader are ignored).
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", j.contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(statusCode)
	_, err = w.Write(content)
	return err
}

func (j *jsonResponder) WriteSimple(w http.ResponseWriter, statusCode int, message string) error {
	return j.Write(w, statusCode, SimpleMessage{Message: message})
}

// WriteError writes err as {"error": {...}}. Unknown errors are logged and
// replaced with a generic 500 so internal details never reach the client.
func (j *jsonResponder) WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := apperrors.As(err)
	if !ok {
		slog.ErrorContext(r.Context(), "internal error", "method", r.Method, "path", r.URL.Path, "error", err)
		appErr = apperrors.New(http.StatusInternalServerError, "internal_error", "Terjadi kesalahan pada server.")
	} else if appErr.Status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	}

	if writeErr := j.Write(w, appErr.Status, ErrorBody{Error: appErr}); writeErr != nil {
		slog.ErrorContext(r.Context(), "failed to write error response", "error", writeErr)
	}
}
