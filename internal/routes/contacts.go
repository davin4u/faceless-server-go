package routes

import (
	"encoding/json"
	"net/http"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/contactcode"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/socketio"
	"github.com/go-chi/chi/v5"
)

// NewContacts returns a chi router rooted at the path the caller mounts it under
// (callers will mount at "/api/contacts"). It assumes auth.RequireSignatureAuth
// has already populated the context.
func NewContacts(d db.DB, notify socketio.Notifier) http.Handler {
	r := chi.NewRouter()
	r.Get("/", listContacts(d))
	r.Get("/requests", listRequests(d))
	r.Post("/add", addContact(d, notify))
	r.Post("/accept", acceptContact(d, notify))
	r.Post("/reject", rejectContact(d))
	r.Post("/regenerate-code", regenerateCode(d))
	return r
}

func listContacts(d db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		rows, _ := d.All(r.Context(), `
			SELECT u.id, u.contact_code, u.display_name, u.public_key, u.chat_public_key,
			       ap.ciphertext AS avatar_ciphertext, ap.nonce AS avatar_nonce
			FROM contacts c
			JOIN users u ON u.id = CASE WHEN c.user_id = ? THEN c.contact_id ELSE c.user_id END
			LEFT JOIN avatar_pointers ap ON ap.owner_id = u.id AND ap.recipient_id = ?
			WHERE (c.user_id = ? OR c.contact_id = ?) AND c.status = 'accepted'
			GROUP BY u.id, ap.ciphertext, ap.nonce
		`, u.ID, u.ID, u.ID, u.ID)
		writeJSON(w, 200, map[string]any{"contacts": mapContactRows(rows)})
	}
}

func listRequests(d db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		incoming, _ := d.All(r.Context(), `
			SELECT u.id, u.contact_code, u.display_name, c.created_at
			FROM contacts c JOIN users u ON u.id = c.user_id
			WHERE c.contact_id = ? AND c.status = 'pending'
			ORDER BY c.created_at DESC`, u.ID)
		outgoing, _ := d.All(r.Context(), `
			SELECT u.id, u.contact_code, u.display_name, c.created_at
			FROM contacts c JOIN users u ON u.id = c.contact_id
			WHERE c.user_id = ? AND c.status = 'pending'
			ORDER BY c.created_at DESC`, u.ID)
		mapRow := func(rows []db.Row) []map[string]any {
			out := make([]map[string]any, 0, len(rows))
			for _, rr := range rows {
				out = append(out, map[string]any{
					"id":          rr.Str("id"),
					"contactCode": rr.Str("contact_code"),
					"displayName": rr.Str("display_name"),
					"createdAt":   rr.Int("created_at"),
				})
			}
			return out
		}
		incomingMapped := mapRow(incoming)
		writeJSON(w, 200, map[string]any{
			"incoming": incomingMapped,
			"outgoing": mapRow(outgoing),
			"requests": incomingMapped, // backward compat
		})
	}
}

func addContact(d db.DB, notify socketio.Notifier) http.HandlerFunc {
	type req struct {
		ContactCode string `json:"contactCode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.ContactCode == "" {
			writeJSONErr(w, 400, "Contact code is required")
			return
		}
		ctx := r.Context()
		target, _ := d.Get(ctx, `SELECT id FROM users WHERE contact_code = ?`, b.ContactCode)
		if target == nil {
			writeJSONErr(w, 404, "Contact code not found")
			return
		}
		targetID := target.Str("id")
		if targetID == u.ID {
			writeJSONErr(w, 400, "Cannot add yourself as a contact")
			return
		}
		exists, _ := d.Get(ctx,
			`SELECT status FROM contacts WHERE (user_id = ? AND contact_id = ?) OR (user_id = ? AND contact_id = ?)`,
			u.ID, targetID, targetID, u.ID)
		if exists != nil {
			writeJSONErr(w, 409, "Contact relationship already exists")
			return
		}
		if _, err := d.Run(ctx,
			`INSERT INTO contacts (user_id, contact_id, status) VALUES (?, ?, 'pending')`,
			u.ID, targetID); err != nil {
			writeJSONErr(w, 500, "DB error")
			return
		}
		notify.NotifyContactRequest(targetID, socketio.ContactRequestPayload{
			ID: u.ID, ContactCode: u.ContactCode, DisplayName: u.DisplayName,
		})
		writeJSON(w, 200, map[string]string{"status": "sent"})
	}
}

func acceptContact(d db.DB, notify socketio.Notifier) http.HandlerFunc {
	type req struct {
		UserID string `json:"userId"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.UserID == "" {
			writeJSONErr(w, 400, "Requester userId is required")
			return
		}
		ctx := r.Context()
		pending, _ := d.Get(ctx,
			`SELECT 1 FROM contacts WHERE user_id = ? AND contact_id = ? AND status = 'pending'`,
			b.UserID, u.ID)
		if pending == nil {
			writeJSONErr(w, 404, "No pending request from this user")
			return
		}
		if err := d.Tx(ctx, func(tx db.Tx) error {
			if _, err := tx.Run(ctx,
				`UPDATE contacts SET status = 'accepted' WHERE user_id = ? AND contact_id = ?`,
				b.UserID, u.ID); err != nil {
				return err
			}
			_, err := tx.Run(ctx,
				`INSERT INTO contacts (user_id, contact_id, status) VALUES (?, ?, 'accepted')`,
				u.ID, b.UserID)
			return err
		}); err != nil {
			writeJSONErr(w, 500, "DB error")
			return
		}
		notify.NotifyContactAccepted(b.UserID, socketio.ContactAcceptedPayload{
			ID: u.ID, ContactCode: u.ContactCode, DisplayName: u.DisplayName,
			PublicKey: u.PublicKey, ChatPublicKey: u.ChatPublicKey,
		})
		writeJSON(w, 200, map[string]string{"status": "accepted"})
	}
}

func rejectContact(d db.DB) http.HandlerFunc {
	type req struct {
		UserID string `json:"userId"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.UserID == "" {
			writeJSONErr(w, 400, "Requester userId is required")
			return
		}
		_, _ = d.Run(r.Context(),
			`DELETE FROM contacts WHERE user_id = ? AND contact_id = ? AND status = 'pending'`,
			b.UserID, u.ID)
		writeJSON(w, 200, map[string]string{"status": "rejected"})
	}
}

func regenerateCode(d db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		ctx := r.Context()
		row, _ := d.Get(ctx, `SELECT contact_code FROM users WHERE id = ?`, u.ID)
		if row == nil {
			writeJSONErr(w, 404, "User not found")
			return
		}
		oldCode := row.Str("contact_code")
		newCode, err := contactcode.Generate(ctx, d)
		if err != nil {
			writeJSONErr(w, 500, "Failed to allocate contact code")
			return
		}
		err = d.Tx(ctx, func(tx db.Tx) error {
			insertSQL := d.InsertIgnore("retired_codes", "code, retired_at", "?, "+d.NowEpoch())
			if _, err := tx.Run(ctx, insertSQL, oldCode); err != nil {
				return err
			}
			_, err := tx.Run(ctx, `UPDATE users SET contact_code = ? WHERE id = ?`, newCode, u.ID)
			return err
		})
		if err != nil {
			writeJSONErr(w, 500, "DB error")
			return
		}
		writeJSON(w, 200, map[string]string{"contactCode": newCode})
	}
}

// Force chi import for users of this file even before routes are mounted.
var _ = chi.NewRouter
