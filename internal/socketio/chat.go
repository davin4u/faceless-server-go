package socketio

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/davin4u/faceless-server-go/internal/stats"
	"github.com/google/uuid"
	socketio "github.com/zishang520/socket.io/v2/socket"
)

type chatPayload struct {
	ID         string `json:"id"`
	To         string `json:"to"`
	From       string `json:"from"`
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
	Timestamp  int64  `json:"timestamp,omitempty"`
}

func (s *Server) registerChatHandlers(socket *socketio.Socket) {
	data, _ := socket.Data().(map[string]any)
	userID, _ := data["user_id"].(string)
	connID, _ := data["conn_id"].(string)
	ctx := context.Background()

	socket.On("message:send", func(args ...any) {
		var p chatPayload
		if !decodeArg(args, &p) {
			slog.Warn("chat.message_send.bad_payload", "conn_id", connID, "from", userID)
			socket.Emit("error", map[string]string{"message": "Invalid message payload"})
			return
		}
		if p.ID == "" || p.To == "" || p.Ciphertext == "" || p.Nonce == "" {
			slog.Warn("chat.message_send.missing_fields",
				"conn_id", connID, "from", userID, "to", p.To, "id", p.ID)
			socket.Emit("error", map[string]string{"message": "Missing required message fields"})
			return
		}

		// Verify accepted-contact relationship in sender's direction
		row, _ := s.d.Get(ctx,
			`SELECT 1 FROM contacts WHERE user_id = ? AND contact_id = ? AND status = 'accepted'`,
			userID, p.To)
		if row == nil {
			slog.Warn("chat.message_send.not_contacts", "from", userID, "to", p.To)
			socket.Emit("error", map[string]string{"message": "Recipient is not in your contacts"})
			return
		}

		ts := p.Timestamp
		if ts == 0 {
			ts = time.Now().Unix()
		}
		if _, err := s.d.Run(ctx,
			`INSERT INTO messages (id, sender_id, receiver_id, ciphertext, nonce, timestamp) VALUES (?, ?, ?, ?, ?, ?)`,
			p.ID, userID, p.To, p.Ciphertext, p.Nonce, ts); err != nil {
			slog.Error("chat.message_send.db_error", "from", userID, "to", p.To, "err", err)
			socket.Emit("error", map[string]string{"message": "DB error"})
			return
		}
		socket.Emit("message:sent", map[string]any{"id": p.ID, "timestamp": ts})
		slog.Info("chat.message_send.stored",
			"from", userID, "to", p.To, "id", p.ID, "ciphertext_bytes", len(p.Ciphertext))

		go func() {
			if err := stats.IncrementDaily(ctx, s.d, stats.ColMessagesSent, 1); err != nil {
				slog.Error("stats.messages_sent.error", "err", err)
			}
		}()

		// Fan-out to recipient's online sockets (app sockets only — service
		// sockets do not receive chat. The Node implementation routes to all
		// sockets of a user, but only app sockets register chat handlers.)
		s.presence.EmitToUserAppOnly(p.To, "message:receive", map[string]any{
			"id": p.ID, "from": userID, "ciphertext": p.Ciphertext,
			"nonce": p.Nonce, "timestamp": ts,
		})

		// Recipient has no foreground app socket → wake them via FCM. No-op
		// if the user has no tokens (persistent-mode) or FCM is unconfigured.
		if !s.presence.HasAppSocket(p.To) {
			go s.push.SendMessageWake(context.Background(), p.To, userID)
		}
	})

	socket.On("message:ack", func(args ...any) {
		var p struct {
			MessageID string `json:"messageId"`
		}
		if !decodeArg(args, &p) || p.MessageID == "" {
			return
		}
		row, _ := s.d.Get(ctx, `SELECT sender_id FROM messages WHERE id = ? AND receiver_id = ?`, p.MessageID, userID)
		if row == nil {
			return
		}
		s.presence.EmitToUserAppOnly(row.Str("sender_id"), "message:delivered",
			map[string]string{"messageId": p.MessageID})
		_, _ = s.d.Run(ctx, `DELETE FROM messages WHERE id = ?`, p.MessageID)
		slog.Debug("chat.message_ack", "user_id", userID, "message_id", p.MessageID)
	})

	socket.On("message:delete", func(args ...any) {
		var p struct {
			MessageID string `json:"messageId"`
			To        string `json:"to"`
		}
		if !decodeArg(args, &p) || p.MessageID == "" || p.To == "" {
			return
		}
		_, _ = s.d.Run(ctx, `DELETE FROM messages WHERE id = ? AND sender_id = ?`, p.MessageID, userID)
		if s.files != nil {
			s.files.DeleteByMessage(ctx, p.MessageID, userID)
		}

		if s.presence.IsUserOnline(p.To) {
			s.presence.EmitToUserAppOnly(p.To, "message:deleted",
				map[string]string{"messageId": p.MessageID, "from": userID})
			slog.Info("chat.message_delete.relayed", "from", userID, "to", p.To, "id", p.MessageID)
		} else {
			payload := map[string]string{"messageId": p.MessageID, "from": userID}
			b, _ := json.Marshal(payload)
			_, _ = s.d.Run(ctx,
				`INSERT INTO pending_events (id, user_id, event_type, payload, timestamp) VALUES (?, ?, 'message:deleted', ?, ?)`,
				uuid.NewString(), p.To, string(b), time.Now().Unix())
			slog.Info("chat.message_delete.queued", "from", userID, "to", p.To, "id", p.MessageID)
		}
	})

	socket.On("typing", func(args ...any) {
		var p struct {
			To       string `json:"to"`
			IsTyping bool   `json:"isTyping"`
		}
		if !decodeArg(args, &p) || p.To == "" {
			return
		}
		s.presence.EmitToUserAppOnly(p.To, "typing", map[string]any{
			"from": userID, "isTyping": p.IsTyping,
		})
	})

	socket.On("profile:avatar", func(args ...any) {
		var p struct {
			To         string `json:"to"`
			Ciphertext string `json:"ciphertext"`
			Nonce      string `json:"nonce"`
		}
		if !decodeArg(args, &p) || p.To == "" || p.Ciphertext == "" || p.Nonce == "" {
			return
		}
		// Only deliver between accepted contacts (sender's direction).
		rel, _ := s.d.Get(ctx,
			`SELECT 1 FROM contacts WHERE user_id = ? AND contact_id = ? AND status = 'accepted'`, userID, p.To)
		if rel == nil {
			return
		}
		out := map[string]string{"from": userID, "ciphertext": p.Ciphertext, "nonce": p.Nonce}
		if s.presence.IsUserOnline(p.To) {
			s.presence.EmitToUserAppOnly(p.To, "profile:avatar", out)
			slog.Info("profile.avatar.relayed", "from", userID, "to", p.To)
		} else {
			b, _ := json.Marshal(out)
			_, _ = s.d.Run(ctx,
				`INSERT INTO pending_events (id, user_id, event_type, payload, timestamp) VALUES (?, ?, 'profile:avatar', ?, ?)`,
				uuid.NewString(), p.To, string(b), time.Now().Unix())
			slog.Info("profile.avatar.queued", "from", userID, "to", p.To)
		}
	})
}

// decodeArg JSON-roundtrips args[0] into v, returning false on error.
// zishang520/socket.io delivers payloads as map[string]any after parsing JSON,
// so we re-marshal/unmarshal into a typed struct for ergonomics.
func decodeArg(args []any, v any) bool {
	if len(args) == 0 {
		return false
	}
	b, err := json.Marshal(args[0])
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}
