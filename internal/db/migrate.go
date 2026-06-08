package db

import (
	"context"
	"crypto/rand"
	"errors"
)

// runSqliteMigrations applies SQLite-only schema fixups. Idempotent.
//
// 1. Add chat_public_key column if missing.
// 2. Rebuild users table if username/password_hash are still NOT NULL (legacy schema).
func runSqliteMigrations(ctx context.Context, d DB) error {
	hasChat, err := sqliteHasColumn(ctx, d, "users", "chat_public_key")
	if err != nil {
		return err
	}
	if !hasChat {
		if err := d.Exec(ctx, `ALTER TABLE users ADD COLUMN chat_public_key TEXT`); err != nil {
			return err
		}
	}

	usernameNotNull, err := sqliteColumnIsNotNull(ctx, d, "users", "username")
	if err != nil {
		return err
	}
	if usernameNotNull {
		// Determine whether the old table has a created_at column so the
		// INSERT … SELECT can copy it (or default to now if absent).
		hasCreatedAt, err := sqliteHasColumn(ctx, d, "users", "created_at")
		if err != nil {
			return err
		}
		createdAtExpr := "unixepoch()"
		if hasCreatedAt {
			createdAtExpr = "COALESCE(created_at, unixepoch())"
		}

		// Rebuild the table to make username/password_hash nullable.
		stmts := []string{
			`PRAGMA foreign_keys = OFF`,
			`BEGIN`,
			`DROP TABLE IF EXISTS users_new`,
			`CREATE TABLE users_new (
				id TEXT PRIMARY KEY,
				contact_code TEXT UNIQUE NOT NULL,
				display_name TEXT NOT NULL,
				public_key TEXT NOT NULL,
				chat_public_key TEXT,
				username TEXT,
				password_hash TEXT,
				created_at INTEGER NOT NULL DEFAULT (unixepoch())
			)`,
			`INSERT INTO users_new (id, contact_code, display_name, public_key, chat_public_key, username, password_hash, created_at)
				SELECT id, contact_code, display_name, public_key, chat_public_key, username, password_hash,
					` + createdAtExpr + ` FROM users`,
			`DROP TABLE users`,
			`ALTER TABLE users_new RENAME TO users`,
			`COMMIT`,
			`PRAGMA foreign_keys = ON`,
		}
		for _, s := range stmts {
			if err := d.Exec(ctx, s); err != nil {
				return err
			}
		}
	}
	// Migration: add invitation_code + invitation_code_usages, then backfill.
	hasInvite, err := sqliteHasColumn(ctx, d, "users", "invitation_code")
	if err != nil {
		return err
	}
	if !hasInvite {
		if err := d.Exec(ctx, `ALTER TABLE users ADD COLUMN invitation_code TEXT`); err != nil {
			return err
		}
	}
	hasUsages, err := sqliteHasColumn(ctx, d, "users", "invitation_code_usages")
	if err != nil {
		return err
	}
	if !hasUsages {
		if err := d.Exec(ctx, `ALTER TABLE users ADD COLUMN invitation_code_usages INTEGER NOT NULL DEFAULT 3`); err != nil {
			return err
		}
	}
	missing, err := d.All(ctx, `SELECT id FROM users WHERE invitation_code IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range missing {
		code, err := backfillInviteCode(ctx, d)
		if err != nil {
			return err
		}
		if _, err := d.Run(ctx,
			`UPDATE users SET invitation_code = ?, invitation_code_usages = 3 WHERE id = ?`,
			code, r.Str("id")); err != nil {
			return err
		}
	}

	return nil
}

func sqliteHasColumn(ctx context.Context, d DB, table, col string) (bool, error) {
	rows, err := d.All(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.Str("name") == col {
			return true, nil
		}
	}
	return false, nil
}

func sqliteColumnIsNotNull(ctx context.Context, d DB, table, col string) (bool, error) {
	rows, err := d.All(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.Str("name") == col {
			return r.Int("notnull") == 1, nil
		}
	}
	return false, nil
}

const inviteCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func backfillInviteCode(ctx context.Context, d DB) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		var raw [8]byte
		for i := 0; i < 8; i++ {
			raw[i] = inviteCharset[int(b[i])%len(inviteCharset)]
		}
		code := string(raw[:4]) + "-" + string(raw[4:])
		row, err := d.Get(ctx, `SELECT 1 FROM users WHERE invitation_code = ?`, code)
		if err != nil {
			return "", err
		}
		if row == nil {
			return code, nil
		}
	}
	return "", errors.New("failed to generate unique invitation code during backfill")
}
