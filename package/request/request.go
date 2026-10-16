// Package request holds helpers for parsing and validating HTTP input.
package request

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"

	apperrors "hearable/backend/package/errors"
)

// DecodeJSON decodes a single JSON object from the request body into dst.
// Body size is bounded by the MaxBodyBytes middleware.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperrors.New(http.StatusRequestEntityTooLarge, "payload_too_large", "Ukuran permintaan terlalu besar.")
		}
		if errors.Is(err, io.EOF) {
			return apperrors.BadRequest("Body permintaan kosong.")
		}
		return apperrors.BadRequest("Format JSON tidak valid.")
	}
	if dec.More() {
		return apperrors.BadRequest("Body permintaan harus berisi satu objek JSON.")
	}
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a canonical UUID string. Validating IDs up
// front avoids sending malformed values to Postgres (which would error).
func IsUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// ParseDifficulty reads the optional ?difficulty= query parameter. Range
// checks happen in the services.
func ParseDifficulty(r *http.Request) (*int, error) {
	raw := r.URL.Query().Get("difficulty")
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, apperrors.BadRequest("difficulty harus berupa angka 1 sampai 4.")
	}
	return &n, nil
}
