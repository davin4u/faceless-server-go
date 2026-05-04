// Package db provides a unified async-style interface to SQLite or PostgreSQL.
package db

import (
	"context"
	"fmt"
)

// Row is a column-keyed result row. Values come back as int64, float64, string,
// []byte, bool, or nil — matching what the underlying driver returns.
type Row map[string]any

func (r Row) Str(key string) string {
	v, ok := r[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}

func (r Row) Int(key string) int64 {
	v, ok := r[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// Result describes a non-query mutation outcome.
type Result struct {
	Changes int64
}

// Tx is the per-transaction handle passed to a transaction function.
type Tx interface {
	Get(ctx context.Context, sql string, args ...any) (Row, error)
	All(ctx context.Context, sql string, args ...any) ([]Row, error)
	Run(ctx context.Context, sql string, args ...any) (Result, error)
}

// DB is the package-level interface used by the rest of the server.
type DB interface {
	Get(ctx context.Context, sql string, args ...any) (Row, error)
	All(ctx context.Context, sql string, args ...any) ([]Row, error)
	Run(ctx context.Context, sql string, args ...any) (Result, error)
	Exec(ctx context.Context, sql string) error
	Tx(ctx context.Context, fn func(tx Tx) error) error
	Close() error

	// InsertIgnore returns dialect-specific "INSERT OR IGNORE" / "ON CONFLICT DO NOTHING".
	InsertIgnore(table, columns, placeholders string) string
	// NowEpoch returns the dialect-specific SQL expression for current unix epoch seconds.
	NowEpoch() string
	// Dialect returns "sqlite" or "postgres".
	Dialect() string
}
