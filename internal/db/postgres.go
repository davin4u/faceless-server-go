package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgDriver struct {
	pool *pgxpool.Pool
}

// NewPostgres opens a pgx connection pool against the given DSN.
func NewPostgres(connString string) (DB, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pg: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping pg: %w", err)
	}
	return &pgDriver{pool: pool}, nil
}

func (p *pgDriver) Get(ctx context.Context, query string, args ...any) (Row, error) {
	rows, err := p.pool.Query(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return pgScanRow(rows)
}

func (p *pgDriver) All(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := p.pool.Query(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := pgScanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *pgDriver) Run(ctx context.Context, query string, args ...any) (Result, error) {
	tag, err := p.pool.Exec(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return Result{}, err
	}
	return Result{Changes: tag.RowsAffected()}, nil
}

func (p *pgDriver) Exec(ctx context.Context, query string) error {
	_, err := p.pool.Exec(ctx, query)
	return err
}

func (p *pgDriver) Tx(ctx context.Context, fn func(tx Tx) error) error {
	t, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(&pgTx{ctx: ctx, tx: t}); err != nil {
		_ = t.Rollback(ctx)
		return err
	}
	return t.Commit(ctx)
}

func (p *pgDriver) Close() error                        { p.pool.Close(); return nil }
func (p *pgDriver) Dialect() string                     { return "postgres" }
func (p *pgDriver) NowEpoch() string                    { return "EXTRACT(EPOCH FROM NOW())::INTEGER" }
func (p *pgDriver) InsertIgnore(t, c, ph string) string {
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING", t, c, ph)
}

type pgTx struct {
	ctx context.Context
	tx  pgx.Tx
}

func (t *pgTx) Get(ctx context.Context, query string, args ...any) (Row, error) {
	rows, err := t.tx.Query(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return pgScanRow(rows)
}

func (t *pgTx) All(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := t.tx.Query(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := pgScanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (t *pgTx) Run(ctx context.Context, query string, args ...any) (Result, error) {
	tag, err := t.tx.Exec(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return Result{}, err
	}
	return Result{Changes: tag.RowsAffected()}, nil
}

// convertPlaceholders rewrites ?, ?, ? as $1, $2, $3 for pgx.
// Note: this naive approach assumes the SQL contains no literal '?' characters.
// Audit any future SQL string literals if that changes.
func convertPlaceholders(sql string) string {
	var b strings.Builder
	n := 0
	for _, r := range sql {
		if r == '?' {
			n++
			b.WriteString(fmt.Sprintf("$%d", n))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func pgScanRow(rows pgx.Rows) (Row, error) {
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	fds := rows.FieldDescriptions()
	r := make(Row, len(fds))
	for i, f := range fds {
		r[string(f.Name)] = vals[i]
	}
	return r, nil
}
