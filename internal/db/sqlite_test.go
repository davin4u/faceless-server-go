package db

import (
	"context"
	"path/filepath"
	"testing"
)

func newSqlite(t *testing.T) DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := NewSqlite(path)
	if err != nil {
		t.Fatalf("NewSqlite: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestSqlite_GetRunAll(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)

	if err := d.Exec(ctx, `CREATE TABLE t (id TEXT PRIMARY KEY, n INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Run(ctx, `INSERT INTO t (id, n) VALUES (?, ?)`, "a", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Run(ctx, `INSERT INTO t (id, n) VALUES (?, ?)`, "b", 2); err != nil {
		t.Fatal(err)
	}

	row, err := d.Get(ctx, `SELECT id, n FROM t WHERE id = ?`, "a")
	if err != nil {
		t.Fatal(err)
	}
	if row.Str("id") != "a" || row.Int("n") != 1 {
		t.Errorf("got %+v", row)
	}

	rows, err := d.All(ctx, `SELECT id, n FROM t ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len=%d", len(rows))
	}
	if rows[0].Str("id") != "a" || rows[1].Str("id") != "b" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestSqlite_GetMissingReturnsNilRow(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := d.Exec(ctx, `CREATE TABLE t (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get(ctx, `SELECT id FROM t WHERE id = ?`, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Errorf("want nil row, got %+v", row)
	}
}

func TestSqlite_TxCommitRollback(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := d.Exec(ctx, `CREATE TABLE t (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	// Commit
	if err := d.Tx(ctx, func(tx Tx) error {
		_, e := tx.Run(ctx, `INSERT INTO t (id) VALUES (?)`, "x")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	// Rollback (return error from fn)
	wantErr := "boom"
	err := d.Tx(ctx, func(tx Tx) error {
		_, _ = tx.Run(ctx, `INSERT INTO t (id) VALUES (?)`, "y")
		return errBoom(wantErr)
	})
	if err == nil || err.Error() != wantErr {
		t.Errorf("err = %v", err)
	}
	rows, _ := d.All(ctx, `SELECT id FROM t ORDER BY id`)
	if len(rows) != 1 || rows[0].Str("id") != "x" {
		t.Errorf("expected only 'x' after rollback, got %+v", rows)
	}
}

type errBoom string

func (e errBoom) Error() string { return string(e) }

func TestSqlite_NowEpochAndInsertIgnore(t *testing.T) {
	d := newSqlite(t)
	if d.Dialect() != "sqlite" {
		t.Errorf("Dialect() = %q", d.Dialect())
	}
	if d.NowEpoch() != "unixepoch()" {
		t.Errorf("NowEpoch = %q", d.NowEpoch())
	}
	got := d.InsertIgnore("retired_codes", "code, retired_at", "?, unixepoch()")
	want := "INSERT OR IGNORE INTO retired_codes (code, retired_at) VALUES (?, unixepoch())"
	if got != want {
		t.Errorf("InsertIgnore = %q", got)
	}
}
