package db

import (
	"context"
	"testing"
)

func TestInitSchema_SQLite_CreatesAllTables(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)

	if err := InitSchema(ctx, d); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	wantTables := []string{"users", "contacts", "messages", "pending_events", "retired_codes", "daily_stats"}
	for _, tbl := range wantTables {
		row, err := d.Get(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl)
		if err != nil {
			t.Fatalf("query for %s: %v", tbl, err)
		}
		if row == nil {
			t.Errorf("table %q missing after InitSchema", tbl)
		}
	}
}

func TestInitSchema_SQLite_CreatesIndexes(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := InitSchema(ctx, d); err != nil {
		t.Fatal(err)
	}
	wantIdx := []string{
		"idx_users_contact_code",
		"idx_users_public_key",
		"idx_users_chat_public_key",
		"idx_messages_receiver_delivered",
		"idx_pending_events_user",
	}
	for _, idx := range wantIdx {
		row, err := d.Get(ctx, `SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx)
		if err != nil {
			t.Fatal(err)
		}
		if row == nil {
			t.Errorf("index %q missing", idx)
		}
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := InitSchema(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := InitSchema(ctx, d); err != nil {
		t.Fatalf("second InitSchema: %v", err)
	}
}
