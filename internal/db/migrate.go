package db

import "context"

// runSqliteMigrations applies SQLite-only schema fixups (column adds, table
// rebuilds for nullability changes). PostgreSQL doesn't need these because the
// CREATE TABLE statements above already define the final schema.
//
// This stub is filled in by Task 8.
func runSqliteMigrations(ctx context.Context, d DB) error {
	return nil
}
