package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
)

// MaxBodyBytes caps the request body size.
func MaxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger logs one structured line per request. It never logs bodies.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func limitHandler(jr jsonresponder.JSONResponder) httprate.Option {
	return httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
		jr.WriteError(w, r, apperrors.TooManyRequests("Terlalu banyak permintaan. Coba lagi sebentar lagi."))
	})
}

// TrustedProxyPrefixes are the networks Caddy can reach the container from
// (loopback and Docker bridge / private ranges). X-Forwarded-For entries from
// these are skipped when resolving the real client IP.
var TrustedProxyPrefixes = []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// ClientIP resolves the caller's IP from X-Forwarded-For, trusting only
// TrustedProxyPrefixes. Read it back with clientIPKey.
func ClientIP() func(http.Handler) http.Handler {
	return middleware.ClientIPFromXFF(TrustedProxyPrefixes...)
}

func clientIPKey(r *http.Request) string {
	if ip := middleware.GetClientIP(r.Context()); ip != "" {
		return httprate.CanonicalizeIP(ip)
	}
	// No XFF header (direct connection, e.g. local development).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return httprate.CanonicalizeIP(host)
}

// RateLimitByIP limits unauthenticated endpoints such as login.
// Requires ClientIP to be mounted earlier in the chain.
func RateLimitByIP(jr jsonresponder.JSONResponder, requests int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.LimitBy(requests, window, func(r *http.Request) (string, error) {
		return "ip:" + clientIPKey(r), nil
	}, limitHandler(jr))
}

// RateLimitByUser limits authenticated endpoints (e.g. AI evaluation) per user.
// Must be mounted after Authenticate.
func RateLimitByUser(jr jsonresponder.JSONResponder, requests int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.LimitBy(requests, window, func(r *http.Request) (string, error) {
		if id, ok := IdentityFromContext(r.Context()); ok {
			return "user:" + id.UserID, nil
		}
		return "ip:" + clientIPKey(r), nil
	}, limitHandler(jr))
}
