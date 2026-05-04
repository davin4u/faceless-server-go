package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestInit_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	Init(Config{Level: "info", Format: "json", Out: &buf})

	slog.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("expected JSON line, got %q (err=%v)", buf.String(), err)
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", rec["msg"])
	}
	if rec["k"] != "v" {
		t.Errorf("k = %v, want v", rec["k"])
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
}

func TestInit_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	Init(Config{Level: "debug", Format: "text", Out: &buf})

	slog.Debug("debug-line", "x", 1)

	if !strings.Contains(buf.String(), "debug-line") {
		t.Errorf("expected text output to contain 'debug-line', got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "x=1") {
		t.Errorf("expected text output to contain 'x=1', got %q", buf.String())
	}
}

func TestInit_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	Init(Config{Level: "warn", Format: "json", Out: &buf})

	slog.Info("dropped")
	slog.Warn("kept")

	if strings.Contains(buf.String(), "dropped") {
		t.Errorf("info-level message should be filtered out at warn, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "kept") {
		t.Errorf("warn message should appear, got %q", buf.String())
	}
}
