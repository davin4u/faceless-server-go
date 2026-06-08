package db

import (
	"context"
	"testing"
)

func TestBackfillInvitationCodes(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := InitSchema(ctx, d); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	// Simulate a legacy row whose invitation_code was never set.
	if _, err := d.Run(ctx,
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code) VALUES ('legacy','BBBB-3333','Old','pkold',NULL)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Re-run schema/migrations — backfill should assign a code + 3 usages.
	if err := InitSchema(ctx, d); err != nil {
		t.Fatalf("InitSchema rerun: %v", err)
	}
	row, err := d.Get(ctx, `SELECT invitation_code, invitation_code_usages FROM users WHERE id='legacy'`)
	if err != nil || row == nil {
		t.Fatalf("get legacy: %v row=%v", err, row)
	}
	if row.Str("invitation_code") == "" {
		t.Fatalf("invitation_code not backfilled")
	}
	if row.Int("invitation_code_usages") != 3 {
		t.Fatalf("expected 3 usages, got %d", row.Int("invitation_code_usages"))
	}
}

func TestSqliteMigrations_AddsChatPublicKey(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)

	// Simulate an old DB without chat_public_key
	if err := d.Exec(ctx, `
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			contact_code TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL,
			public_key TEXT NOT NULL,
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL
		);
	`); err != nil {
		t.Fatal(err)
	}

	if err := runSqliteMigrations(ctx, d); err != nil {
		t.Fatal(err)
	}

	if !tHasColumn(t, ctx, d, "users", "chat_public_key") {
		t.Errorf("chat_public_key column should have been added")
	}
}

func TestSqliteMigrations_MakesUsernameNullable(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	// NOT NULL username
	if err := d.Exec(ctx, `
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			contact_code TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL,
			public_key TEXT NOT NULL,
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL
		);
	`); err != nil {
		t.Fatal(err)
	}

	if err := runSqliteMigrations(ctx, d); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	rows, _ := d.All(ctx, `PRAGMA table_info(users)`)
	for _, r := range rows {
		if r.Str("name") == "username" && r.Int("notnull") == 1 {
			t.Errorf("username should be nullable after migration")
		}
	}
	if !tHasColumn(t, ctx, d, "users", "chat_public_key") {
		t.Errorf("chat_public_key should still be present after rebuild")
	}
}

func tHasColumn(t *testing.T, ctx context.Context, d DB, table, col string) bool {
	t.Helper()
	rows, err := d.All(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Str("name") == col {
			return true
		}
	}
	return false
}
