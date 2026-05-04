// Package auth provides HTTP middleware for Ed25519 request signatures and
// admin bearer tokens.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/davin4u/faceless-server-go/internal/crypto"
	"github.com/davin4u/faceless-server-go/internal/db"
)

type ctxKey string

const userKey ctxKey = "user"

// User holds the authenticated user's identity fields, populated from the
// users table after a successful signature verification.
type User struct {
	ID            string
	PublicKey     string
	ChatPublicKey string
	DisplayName   string
	ContactCode   string
}

// UserFromCtx returns the authenticated user, or nil if the request is unauthenticated.
func UserFromCtx(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// WithUser stores u in ctx (used by tests and any non-middleware insertion).
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// RequireSignatureAuth verifies x-public-key + x-signature + x-timestamp headers
// against the raw request body. Attaches the looked-up User to the request context.
//
// Wire-compatibility note: the signature is verified against the RAW body bytes
// exactly as received — never re-serialized — because the client signs
// JSON.stringify(req.body) and the exact byte sequence matters.
func RequireSignatureAuth(d db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pub := r.Header.Get("X-Public-Key")
			sig := r.Header.Get("X-Signature")
			ts := r.Header.Get("X-Timestamp")
			if pub == "" || sig == "" || ts == "" {
				writeErr(w, 401, "Missing auth headers")
				return
			}

			tsInt, err := strconv.ParseInt(ts, 10, 64)
			if err != nil {
				writeErr(w, 401, "Invalid timestamp")
				return
			}
			if abs(time.Now().Unix()-tsInt) > 300 {
				writeErr(w, 401, "Request expired")
				return
			}

			// Read raw body; restore for downstream handler.
			var raw []byte
			if r.Body != nil {
				raw, _ = io.ReadAll(r.Body)
				_ = r.Body.Close()
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))

			bodyStr := string(raw)
			if r.Method == http.MethodGet {
				bodyStr = ""
			}
			if !crypto.VerifyREST(pub, sig, bodyStr, ts, r.Method) {
				slog.Warn("auth.signature.invalid",
					"method", r.Method, "path", r.URL.Path,
					"public_key_prefix", safePrefix(pub),
				)
				writeErr(w, 401, "Invalid signature")
				return
			}

			row, err := d.Get(r.Context(),
				`SELECT id, public_key, chat_public_key, display_name, contact_code FROM users WHERE public_key = ?`, pub)
			if err != nil {
				writeErr(w, 500, "DB error")
				return
			}
			if row == nil {
				writeErr(w, 401, "Unknown identity")
				return
			}
			u := &User{
				ID:            row.Str("id"),
				PublicKey:     row.Str("public_key"),
				ChatPublicKey: row.Str("chat_public_key"),
				DisplayName:   row.Str("display_name"),
				ContactCode:   row.Str("contact_code"),
			}
			ctx := context.WithValue(r.Context(), userKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func safePrefix(s string) string {
	if len(s) > 8 {
		return s[:8] + "…"
	}
	return s
}
