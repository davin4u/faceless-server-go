package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type sqliteDriver struct {
	db *sql.DB
}

// NewSqlite opens (or creates) a SQLite database at the given path with WAL
// journaling and foreign keys enabled.
func NewSqlite(path string) (DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create dir: %w", err)
		}
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc.org/sqlite is concurrency-safe but better-sqlite3 wasn't pooled,
	// so keep behavior deterministic with a small pool.
	db.SetMaxOpenConns(8)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &sqliteDriver{db: db}, nil
}

func (s *sqliteDriver) Get(ctx context.Context, query string, args ...any) (Row, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanRow(rows)
}

func (s *sqliteDriver) All(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

func (s *sqliteDriver) Run(ctx context.Context, query string, args ...any) (Result, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return Result{}, err
	}
	n, _ := res.RowsAffected()
	return Result{Changes: n}, nil
}

func (s *sqliteDriver) Exec(ctx context.Context, query string) error {
	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *sqliteDriver) Tx(ctx context.Context, fn func(tx Tx) error) error {
	t, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	wrapper := &sqliteTx{tx: t}
	if err := fn(wrapper); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

func (s *sqliteDriver) Close() error    { return s.db.Close() }
func (s *sqliteDriver) Dialect() string { return "sqlite" }
func (s *sqliteDriver) NowEpoch() string { return "unixepoch()" }
func (s *sqliteDriver) InsertIgnore(t, c, p string) string {
	return fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)", t, c, p)
}

// Raw exposes the underlying *sql.DB for migrations that need PRAGMA introspection.
func (s *sqliteDriver) Raw() *sql.DB { return s.db }

type sqliteTx struct{ tx *sql.Tx }

func (t *sqliteTx) Get(ctx context.Context, q string, a ...any) (Row, error) {
	rows, err := t.tx.QueryContext(ctx, q, a...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanRow(rows)
}
func (t *sqliteTx) All(ctx context.Context, q string, a ...any) ([]Row, error) {
	rows, err := t.tx.QueryContext(ctx, q, a...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}
func (t *sqliteTx) Run(ctx context.Context, q string, a ...any) (Result, error) {
	res, err := t.tx.ExecContext(ctx, q, a...)
	if err != nil {
		return Result{}, err
	}
	n, _ := res.RowsAffected()
	return Result{Changes: n}, nil
}

// --- shared scan helpers ---

func scanRow(rows *sql.Rows) (Row, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	r := make(Row, len(cols))
	for i, c := range cols {
		r[c] = values[i]
	}
	return r, nil
}

func scanAll(rows *sql.Rows) ([]Row, error) {
	var out []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
