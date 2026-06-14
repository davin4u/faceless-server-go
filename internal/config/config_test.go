package config

import (
	"os"
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

func TestLoad_FileStorageDefaults(t *testing.T) {
	for _, k := range []string{"S3_BUCKET", "S3_ENDPOINT", "MAX_STORAGE_TOTAL_GB", "MAX_FILE_SIZE_MB", "S3_USE_SSL"} {
		os.Unsetenv(k)
	}
	c := Load()
	if c.S3Bucket != "" {
		t.Errorf("S3Bucket default = %q, want empty", c.S3Bucket)
	}
	if c.MaxFileSizeBytes != 25*1024*1024 {
		t.Errorf("MaxFileSizeBytes default = %d, want %d", c.MaxFileSizeBytes, 25*1024*1024)
	}
	if c.MaxStorageTotalBytes != 10*1024*1024*1024 {
		t.Errorf("MaxStorageTotalBytes default = %d, want %d", c.MaxStorageTotalBytes, 10*1024*1024*1024)
	}
}

func TestLoad_FileStorageFromEnv(t *testing.T) {
	os.Setenv("S3_BUCKET", "faceless-files")
	os.Setenv("S3_ENDPOINT", "s3.example.com")
	os.Setenv("S3_USE_SSL", "true")
	os.Setenv("MAX_STORAGE_TOTAL_GB", "20")
	os.Setenv("MAX_FILE_SIZE_MB", "50")
	defer func() {
		for _, k := range []string{"S3_BUCKET", "S3_ENDPOINT", "S3_USE_SSL", "MAX_STORAGE_TOTAL_GB", "MAX_FILE_SIZE_MB"} {
			os.Unsetenv(k)
		}
	}()
	c := Load()
	if c.S3Bucket != "faceless-files" || c.S3Endpoint != "s3.example.com" || !c.S3UseSSL {
		t.Errorf("unexpected S3 config: %+v", c)
	}
	if c.MaxStorageTotalBytes != 20*1024*1024*1024 {
		t.Errorf("MaxStorageTotalBytes = %d", c.MaxStorageTotalBytes)
	}
	if c.MaxFileSizeBytes != 50*1024*1024 {
		t.Errorf("MaxFileSizeBytes = %d", c.MaxFileSizeBytes)
	}
}
