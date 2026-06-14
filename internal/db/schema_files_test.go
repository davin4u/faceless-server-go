package db

import (
	"context"
	"path/filepath"
	"testing"
)

func newSchemaDB(t *testing.T) DB {
	t.Helper()
	d, err := Open("sqlite", filepath.Join(t.TempDir(), "schema.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSchema_FilesTableInsertable(t *testing.T) {
	d := newSchemaDB(t)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('u1','AAAA-2222','A','pk1')`)
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('u2','BBBB-3333','B','pk2')`)
	_, err := d.Run(ctx,
		`INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES (?,?,?,?,?,?,?)`,
		"f1", "u1", "u2", "key1", 1234, "pending", 1000)
	if err != nil {
		t.Fatalf("insert into files: %v", err)
	}
	row, err := d.Get(ctx, `SELECT status, size_bytes FROM files WHERE id = ?`, "f1")
	if err != nil || row == nil {
		t.Fatalf("get file: err=%v row=%v", err, row)
	}
	if row.Str("status") != "pending" || row.Int("size_bytes") != 1234 {
		t.Errorf("unexpected row: %+v", row)
	}
}
