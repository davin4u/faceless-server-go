// Package logger configures the global slog handler used by the rest of the server.
//
// Init() must be called once at startup, before any slog call. Subsequent calls
// replace the global handler.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Level  string    // "debug" | "info" | "warn" | "error"
	Format string    // "json" | "text"
	Out    io.Writer // defaults to os.Stdout if nil
}

func Init(cfg Config) {
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}

	var lvl slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler
	if strings.ToLower(cfg.Format) == "text" {
		h = slog.NewTextHandler(out, opts)
	} else {
		h = slog.NewJSONHandler(out, opts)
	}
	slog.SetDefault(slog.New(h))
}
