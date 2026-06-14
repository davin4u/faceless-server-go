package db

import (
	"context"
	"testing"
	"time"
)

func TestCleanupStaleMessages_UnlinksFiles(t *testing.T) {
	d := newSchemaDB(t)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uA','AAAA-2222','A','pkA')`)
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uB','BBBB-3333','B','pkB')`)
	old := time.Now().Unix() - 31*86400
	_, _ = d.Run(ctx, `INSERT INTO messages (id, sender_id, receiver_id, ciphertext, nonce, timestamp, delivered) VALUES ('m1','uA','uB','c','n',?,0)`, old)
	_, _ = d.Run(ctx, `INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, message_id, created_at) VALUES ('f1','uA','uB','k1',10,'committed','m1',?)`, old)

	if _, err := CleanupStaleMessages(ctx, d); err != nil {
		t.Fatal(err)
	}
	row, _ := d.Get(ctx, `SELECT message_id, status FROM files WHERE id = 'f1'`)
	if row == nil {
		t.Fatal("file row should still exist (sweep deletes it later)")
	}
	if row.Str("message_id") != "" {
		t.Fatalf("message_id should be nulled, got %q", row.Str("message_id"))
	}
}
