package socketio

import (
	"context"
	"encoding/json"
	"log/slog"

	socketio "github.com/zishang520/socket.io/v2/socket"
)

func (s *Server) deliverPending(socket *socketio.Socket, userID string) {
	ctx := context.Background()

	// 1. Push current online status of accepted contacts to this socket
	contacts, err := s.presence.acceptedContactIDs(ctx, userID)
	if err != nil {
		slog.Error("delivery.contacts.error", "user_id", userID, "err", err)
	}
	for _, cid := range contacts {
		if s.presence.HasAppSocket(cid) {
			socket.Emit("presence:update", map[string]any{"userId": cid, "online": true})
		}
	}

	// 2. Drain undelivered messages
	rows, err := s.d.All(ctx,
		`SELECT id, sender_id, ciphertext, nonce, timestamp FROM messages WHERE receiver_id = ? AND delivered = 0 ORDER BY timestamp ASC`,
		userID)
	if err != nil {
		slog.Error("delivery.messages.error", "user_id", userID, "err", err)
		return
	}
	for _, r := range rows {
		socket.Emit("message:receive", map[string]any{
			"id":         r.Str("id"),
			"from":       r.Str("sender_id"),
			"ciphertext": r.Str("ciphertext"),
			"nonce":      r.Str("nonce"),
			"timestamp":  r.Int("timestamp"),
		})
	}
	if len(rows) > 0 {
		slog.Info("delivery.messages.drained", "user_id", userID, "count", len(rows))
	}

	// 3. Drain pending events
	events, err := s.d.All(ctx,
		`SELECT id, event_type, payload FROM pending_events WHERE user_id = ? ORDER BY timestamp ASC`,
		userID)
	if err != nil {
		slog.Error("delivery.events.error", "user_id", userID, "err", err)
		return
	}
	for _, e := range events {
		var payload any
		_ = json.Unmarshal([]byte(e.Str("payload")), &payload)
		socket.Emit(e.Str("event_type"), payload)
	}
	if len(events) > 0 {
		_, _ = s.d.Run(ctx, `DELETE FROM pending_events WHERE user_id = ?`, userID)
		slog.Info("delivery.events.drained", "user_id", userID, "count", len(events))
	}
}
