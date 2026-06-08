package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_TYPE", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("POW_DIFFICULTY", "")
	t.Setenv("ADMIN_SECRET", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("LOG_ICE", "")
	t.Setenv("DATABASE_URL", "")

	c := Load()

	if c.Port != 3000 {
		t.Errorf("Port = %d, want 3000", c.Port)
	}
	if c.DBType != "sqlite" {
		t.Errorf("DBType = %q, want sqlite", c.DBType)
	}
	if c.DBPath != "./data/messenger.db" {
		t.Errorf("DBPath = %q", c.DBPath)
	}
	if c.PoWDifficulty != 14 {
		t.Errorf("PoWDifficulty = %d, want 14", c.PoWDifficulty)
	}
	if c.AdminSecret != "" {
		t.Errorf("AdminSecret should default to empty")
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", c.LogLevel)
	}
	if c.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", c.LogFormat)
	}
	if c.LogICE {
		t.Errorf("LogICE should default false")
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("DB_TYPE", "postgres")
	t.Setenv("DATABASE_URL", "postgres://x:y@localhost:5432/z")
	t.Setenv("POW_DIFFICULTY", "16")
	t.Setenv("ADMIN_SECRET", "abc")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("LOG_ICE", "true")

	c := Load()

	if c.Port != 8080 || c.DBType != "postgres" || c.DatabaseURL == "" ||
		c.PoWDifficulty != 16 || c.AdminSecret != "abc" || c.LogLevel != "debug" ||
		c.LogFormat != "text" || !c.LogICE {
		t.Errorf("overrides not applied: %+v", c)
	}
}
