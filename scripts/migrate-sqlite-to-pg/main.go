package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/logger"
)

var conflictColumn = map[string]string{
	"users":          "id",
	"contacts":       "user_id, contact_id",
	"messages":       "id",
	"pending_events": "id",
	"retired_codes":  "code",
}

func main() {
	logger.Init(logger.Config{Level: "info", Format: "text"})

	dbPath := flag.String("sqlite", os.Getenv("DB_PATH"), "path to source SQLite file")
	pgURL := flag.String("pg", os.Getenv("DATABASE_URL"), "destination Postgres URL")
	flag.Parse()
	if *dbPath == "" || *pgURL == "" {
		fmt.Fprintln(os.Stderr, "usage: migrate-sqlite-to-pg --sqlite=path.db --pg=postgres://...")
		os.Exit(1)
	}

	ctx := context.Background()

	src, err := db.NewSqlite(*dbPath)
	if err != nil {
		slog.Error("open sqlite", "err", err)
		os.Exit(1)
	}
	defer src.Close()

	dst, err := db.NewPostgres(*pgURL)
	if err != nil {
		slog.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer dst.Close()

	slog.Info("creating destination schema")
	if err := db.InitSchema(ctx, dst); err != nil {
		slog.Error("init schema", "err", err)
		os.Exit(1)
	}

	tables := []string{"users", "contacts", "messages", "pending_events", "retired_codes"}
	for _, t := range tables {
		rows, err := src.All(ctx, "SELECT * FROM "+t)
		if err != nil {
			slog.Error("read sqlite", "table", t, "err", err)
			continue
		}
		slog.Info("migrating", "table", t, "rows", len(rows))
		if len(rows) == 0 {
			continue
		}
		var inserted, skipped int
		for _, r := range rows {
			cols, vals := splitRow(r)
			ph := make([]string, len(cols))
			for i := range ph {
				ph[i] = "?"
			}
			q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING",
				t, strings.Join(cols, ","), strings.Join(ph, ","), conflictColumn[t])
			res, err := dst.Run(ctx, q, vals...)
			if err != nil {
				slog.Error("insert", "table", t, "err", err)
				skipped++
				continue
			}
			if res.Changes > 0 {
				inserted++
			} else {
				skipped++
			}
		}
		slog.Info("migration table done", "table", t, "inserted", inserted, "skipped", skipped)
	}

	// Verify
	for _, t := range tables {
		s, _ := src.Get(ctx, "SELECT COUNT(*) AS count FROM "+t)
		d, _ := dst.Get(ctx, "SELECT COUNT(*) AS count FROM "+t)
		match := "OK"
		if s.Int("count") != d.Int("count") {
			match = "MISMATCH"
		}
		slog.Info("verify", "table", t, "sqlite", s.Int("count"), "postgres", d.Int("count"), "status", match)
	}
}

func splitRow(r db.Row) (cols []string, vals []any) {
	for k, v := range r {
		cols = append(cols, k)
		vals = append(vals, v)
	}
	return
}
