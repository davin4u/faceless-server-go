package db

import (
	"context"
	"os"
	"testing"
)

func newPgOrSkip(t *testing.T) DB {
	t.Helper()
	url := os.Getenv("TEST_PG_URL")
	if url == "" {
		t.Skip("TEST_PG_URL not set; skipping postgres integration tests")
	}
	d, err := NewPostgres(url)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	// Use a private schema for isolation
	if err := d.Exec(context.Background(), "DROP SCHEMA IF EXISTS test_facelessgo CASCADE"); err != nil {
		t.Fatal(err)
	}
	if err := d.Exec(context.Background(), "CREATE SCHEMA test_facelessgo; SET search_path TO test_facelessgo"); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPostgres_PlaceholderConversion(t *testing.T) {
	got := convertPlaceholders("SELECT * FROM t WHERE a=? AND b=? OR c=?")
	want := "SELECT * FROM t WHERE a=$1 AND b=$2 OR c=$3"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestPostgres_GetAll(t *testing.T) {
	ctx := context.Background()
	d := newPgOrSkip(t)
	if err := d.Exec(ctx, `CREATE TABLE t (id TEXT PRIMARY KEY, n INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Run(ctx, `INSERT INTO t (id, n) VALUES (?, ?)`, "a", 1); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get(ctx, `SELECT id, n FROM t WHERE id = ?`, "a")
	if err != nil {
		t.Fatal(err)
	}
	if row.Str("id") != "a" || row.Int("n") != 1 {
		t.Errorf("got %+v", row)
	}
}

func TestPostgres_NowEpochAndInsertIgnore(t *testing.T) {
	url := os.Getenv("TEST_PG_URL")
	if url == "" {
		t.Skip()
	}
	d, _ := NewPostgres(url)
	defer d.Close()
	if d.Dialect() != "postgres" {
		t.Error("Dialect")
	}
	if d.NowEpoch() != "EXTRACT(EPOCH FROM NOW())::INTEGER" {
		t.Errorf("NowEpoch = %q", d.NowEpoch())
	}
	got := d.InsertIgnore("retired_codes", "code, retired_at", "?, EXTRACT(EPOCH FROM NOW())::INTEGER")
	want := "INSERT INTO retired_codes (code, retired_at) VALUES (?, EXTRACT(EPOCH FROM NOW())::INTEGER) ON CONFLICT DO NOTHING"
	if got != want {
		t.Errorf("InsertIgnore mismatch:\n got  %q\n want %q", got, want)
	}
}
