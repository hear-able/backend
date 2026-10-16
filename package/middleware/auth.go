package middleware

import (
	"context"
	"net/http"
	"strings"

	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/token"
)

type identityKey struct{}

// Identity is the authenticated caller, derived only from a verified token.
type Identity struct {
	UserID string
	Role   string
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// WithIdentity is exported for handler tests.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

type tokenVerifier interface {
	Verify(raw string) (token.Claims, error)
}

// Authenticate requires a valid "Authorization: Bearer <access token>".
func Authenticate(verifier tokenVerifier, jr jsonresponder.JSONResponder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || strings.TrimSpace(raw) == "" {
				jr.WriteError(w, r, apperrors.Unauthorized("Token akses diperlukan."))
				return
			}
			claims, err := verifier.Verify(strings.TrimSpace(raw))
			if err != nil {
				jr.WriteError(w, r, apperrors.Unauthorized("Token akses tidak valid atau kedaluwarsa."))
				return
			}
			ctx := WithIdentity(r.Context(), Identity{UserID: claims.UserID, Role: claims.Role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole rejects authenticated callers whose role is not in roles.
func RequireRole(jr jsonresponder.JSONResponder, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFromContext(r.Context())
			if !ok {
				jr.WriteError(w, r, apperrors.Unauthorized("Token akses diperlukan."))
				return
			}
			for _, role := range roles {
				if id.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			jr.WriteError(w, r, apperrors.Forbidden("Anda tidak memiliki akses ke sumber ini."))
		})
	}
}
