package db

import "context"

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
