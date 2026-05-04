package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireAdminBearer returns middleware that allows only requests bearing the
// configured Bearer token. If secret is empty, returns 503 (admin disabled).
func RequireAdminBearer(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				writeErr(w, 503, "Admin API not configured")
				return
			}
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeErr(w, 401, "Unauthorized")
				return
			}
			token := strings.TrimPrefix(h, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
				writeErr(w, 401, "Unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
