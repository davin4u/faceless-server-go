package db

import (
	"context"
	"testing"
	"time"
)

func TestCleanup_StaleMessages(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := InitSchema(ctx, d); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	old := now - 31*86400  // > 30 days
	fresh := now - 100

	// Insert a user and two messages
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('u', 'AAAA-2222', 'A', 'pk')`)
	_, _ = d.Run(ctx, `INSERT INTO messages (id, sender_id, receiver_id, ciphertext, nonce, timestamp, delivered) VALUES ('m1','u','u','c','n',?,0)`, old)
	_, _ = d.Run(ctx, `INSERT INTO messages (id, sender_id, receiver_id, ciphertext, nonce, timestamp, delivered) VALUES ('m2','u','u','c','n',?,0)`, fresh)

	n, err := CleanupStaleMessages(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	row, _ := d.Get(ctx, `SELECT id FROM messages WHERE id='m1'`)
	if row != nil {
		t.Error("m1 should be gone")
	}
	row, _ = d.Get(ctx, `SELECT id FROM messages WHERE id='m2'`)
	if row == nil {
		t.Error("m2 should remain")
	}
}

func TestCleanup_RetiredCodes(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := InitSchema(ctx, d); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	_, _ = d.Run(ctx, `INSERT INTO retired_codes (code, retired_at) VALUES ('OLD-1234', ?)`, now-90000)
	_, _ = d.Run(ctx, `INSERT INTO retired_codes (code, retired_at) VALUES ('NEW-5678', ?)`, now-100)

	if _, err := CleanupRetiredCodes(ctx, d); err != nil {
		t.Fatal(err)
	}
	rows, _ := d.All(ctx, `SELECT code FROM retired_codes`)
	if len(rows) != 1 || rows[0].Str("code") != "NEW-5678" {
		t.Errorf("rows = %+v", rows)
	}
}
