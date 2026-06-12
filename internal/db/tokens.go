package db

import (
	"context"
	"time"
)

// UpsertDeviceToken stores (or refreshes) an FCM token for a user. Tokens are
// globally unique (PK); re-registering a token re-points it at this user and
// refreshes last_seen.
func UpsertDeviceToken(ctx context.Context, d DB, userID, token, platform string) error {
	now := time.Now().Unix()
	_, err := d.Run(ctx,
		`INSERT INTO device_tokens (token, user_id, platform, created_at, last_seen)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(token) DO UPDATE SET user_id = excluded.user_id, last_seen = excluded.last_seen`,
		token, userID, platform, now, now)
	return err
}

// DeleteDeviceToken removes a token but only if it belongs to userID (used by
// the authenticated unregister endpoint).
func DeleteDeviceToken(ctx context.Context, d DB, userID, token string) error {
	_, err := d.Run(ctx, `DELETE FROM device_tokens WHERE token = ? AND user_id = ?`, token, userID)
	return err
}

// DeleteToken removes a token unconditionally (used when FCM reports it invalid).
func DeleteToken(ctx context.Context, d DB, token string) error {
	_, err := d.Run(ctx, `DELETE FROM device_tokens WHERE token = ?`, token)
	return err
}

// GetUserTokens returns all FCM tokens registered for a user.
func GetUserTokens(ctx context.Context, d DB, userID string) ([]string, error) {
	rows, err := d.All(ctx, `SELECT token FROM device_tokens WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Str("token"))
	}
	return out, nil
}
