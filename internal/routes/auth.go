// Package routes assembles HTTP handlers under chi routers.
package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/davin4u/faceless-server-go/internal/contactcode"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/namegen"
	"github.com/davin4u/faceless-server-go/internal/pow"
	"github.com/davin4u/faceless-server-go/internal/stats"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// NewAuth returns a chi router with the unauthenticated /api endpoints mounted.
func NewAuth(d db.DB, p *pow.Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/pow/challenge", powChallenge(p))
	r.Post("/api/register", register(d, p))
	r.Post("/api/recover", recover_(d))
	r.Post("/api/generate-name", generateName(d))
	r.Post("/api/invite/validate", inviteValidate(d))
	return r
}

func powChallenge(p *pow.Service) http.HandlerFunc {
	type req struct {
		Action string `json:"action"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Action == "" {
			writeJSONErr(w, 400, "Action is required")
			return
		}
		c := p.Generate(body.Action)
		writeJSON(w, 200, c)
	}
}

func register(d db.DB, p *pow.Service) http.HandlerFunc {
	type req struct {
		Challenge      string `json:"challenge"`
		Nonce          *int   `json:"nonce"`
		PublicKey      string `json:"publicKey"`
		ChatPublicKey  string `json:"chatPublicKey"`
		DisplayName    string `json:"displayName"`
		InvitationCode string `json:"invitationCode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeJSONErr(w, 400, "Invalid JSON")
			return
		}
		if b.Challenge == "" {
			writeJSONErr(w, 400, "Challenge is required")
			return
		}
		if b.Nonce == nil {
			writeJSONErr(w, 400, "Nonce is required")
			return
		}
		if b.PublicKey == "" {
			writeJSONErr(w, 400, "Public key is required")
			return
		}
		if b.ChatPublicKey == "" {
			writeJSONErr(w, 400, "Chat public key is required")
			return
		}
		dn := strings.TrimSpace(b.DisplayName)
		if l := len(dn); l < 1 || l > 50 {
			writeJSONErr(w, 400, "Display name must be 1-50 characters")
			return
		}
		invite := strings.ToUpper(strings.TrimSpace(b.InvitationCode))
		if invite == "" {
			writeJSONErr(w, 400, "Invitation code is required")
			return
		}
		if !p.Verify(b.Challenge, *b.Nonce) {
			writeJSONErr(w, 400, "Invalid proof of work")
			return
		}
		ctx := r.Context()
		if row, _ := d.Get(ctx, `SELECT 1 FROM users WHERE public_key = ?`, b.PublicKey); row != nil {
			writeJSONErr(w, 409, "Public key already registered")
			return
		}
		if row, _ := d.Get(ctx, `SELECT 1 FROM users WHERE display_name = ?`, dn); row != nil {
			writeJSONErr(w, 409, "Display name already taken")
			return
		}
		id := uuid.NewString()
		code, err := contactcode.Generate(ctx, d)
		if err != nil {
			writeJSONErr(w, 500, "Failed to allocate contact code")
			return
		}
		ownInvite, err := contactcode.GenerateInvitation(ctx, d)
		if err != nil {
			writeJSONErr(w, 500, "Failed to allocate invitation code")
			return
		}

		// Atomically consume one invite use (race-safe: guarded by > 0).
		consumed, err := d.Run(ctx,
			`UPDATE users SET invitation_code_usages = invitation_code_usages - 1
			 WHERE invitation_code = ? AND invitation_code_usages > 0`, invite)
		if err != nil {
			writeJSONErr(w, 500, "DB error")
			return
		}
		if consumed.Changes == 0 {
			writeJSONErr(w, 400, "Invalid or exhausted invitation code")
			return
		}

		if _, err := d.Run(ctx,
			`INSERT INTO users (id, contact_code, display_name, public_key, chat_public_key, invitation_code, invitation_code_usages)
			 VALUES (?, ?, ?, ?, ?, ?, 3)`,
			id, code, dn, b.PublicKey, b.ChatPublicKey, ownInvite,
		); err != nil {
			// Roll back the consumed invite so a failed insert does not burn it.
			_, _ = d.Run(ctx,
				`UPDATE users SET invitation_code_usages = invitation_code_usages + 1 WHERE invitation_code = ?`, invite)
			writeJSONErr(w, 500, "DB error")
			return
		}

		// Fire-and-forget stats
		go func() {
			if err := stats.IncrementDaily(ctx, d, stats.ColRegistrations, 1); err != nil {
				slog.Error("stats.registrations.error", "err", err)
			}
		}()
		writeJSON(w, 201, map[string]any{
			"id":                   id,
			"contactCode":          code,
			"displayName":          dn,
			"invitationCode":       ownInvite,
			"invitationCodeUsages": 3,
		})
	}
}

func recover_(d db.DB) http.HandlerFunc {
	type req struct {
		PublicKey string `json:"publicKey"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.PublicKey == "" {
			writeJSONErr(w, 400, "Public key is required")
			return
		}
		ctx := r.Context()
		row, _ := d.Get(ctx,
			`SELECT id, contact_code, display_name, public_key, chat_public_key, invitation_code, invitation_code_usages FROM users WHERE public_key = ?`,
			b.PublicKey)
		if row == nil {
			writeJSONErr(w, 404, "Identity not found")
			return
		}
		uid := row.Str("id")
		contacts, _ := d.All(ctx, `
			SELECT u.id, u.contact_code, u.display_name, u.public_key, u.chat_public_key
			FROM contacts c
			JOIN users u ON u.id = CASE WHEN c.user_id = ? THEN c.contact_id ELSE c.user_id END
			WHERE (c.user_id = ? OR c.contact_id = ?) AND c.status = 'accepted'
			GROUP BY u.id
		`, uid, uid, uid)

		out := map[string]any{
			"id":                   uid,
			"contactCode":          row.Str("contact_code"),
			"displayName":          row.Str("display_name"),
			"publicKey":            row.Str("public_key"),
			"chatPublicKey":        row.Str("chat_public_key"),
			"invitationCode":       row.Str("invitation_code"),
			"invitationCodeUsages": row.Int("invitation_code_usages"),
			"contacts":             mapContactRows(contacts),
		}
		writeJSON(w, 200, out)
	}
}

func generateName(d db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := namegen.GenerateDisplayName(r.Context(), d)
		if err != nil {
			writeJSONErr(w, 500, "Failed to generate name")
			return
		}
		writeJSON(w, 200, map[string]string{"name": n})
	}
}

func inviteValidate(d db.DB) http.HandlerFunc {
	type req struct {
		InvitationCode string `json:"invitationCode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeJSON(w, 200, map[string]bool{"valid": false})
			return
		}
		code := strings.ToUpper(strings.TrimSpace(b.InvitationCode))
		if code == "" {
			writeJSON(w, 200, map[string]bool{"valid": false})
			return
		}
		row, _ := d.Get(r.Context(),
			`SELECT 1 FROM users WHERE invitation_code = ? AND invitation_code_usages > 0`, code)
		writeJSON(w, 200, map[string]bool{"valid": row != nil})
	}
}

func mapContactRows(rows []db.Row) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]string{
			"id":            r.Str("id"),
			"contactCode":   r.Str("contact_code"),
			"displayName":   r.Str("display_name"),
			"publicKey":     r.Str("public_key"),
			"chatPublicKey": r.Str("chat_public_key"),
		})
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
