// Package config loads server configuration from environment variables (and .env in dev).
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          int
	DBType        string // "sqlite" | "postgres"
	DBPath        string
	DatabaseURL   string
	PoWDifficulty int
	AdminSecret   string
	LogLevel      string
	LogFormat     string
	LogICE        bool
}

// Load reads .env (if present) then environment variables and returns Config.
func Load() Config {
	_ = godotenv.Load() // ignore error; .env is optional

	return Config{
		Port:          getInt("PORT", 3000),
		DBType:        getStr("DB_TYPE", "sqlite"),
		DBPath:        getStr("DB_PATH", "./data/messenger.db"),
		DatabaseURL:   getStr("DATABASE_URL", ""),
		PoWDifficulty: getInt("POW_DIFFICULTY", 14),
		AdminSecret:   getStr("ADMIN_SECRET", ""),
		LogLevel:      getStr("LOG_LEVEL", "info"),
		LogFormat:     getStr("LOG_FORMAT", "json"),
		LogICE:        strings.EqualFold(getStr("LOG_ICE", "false"), "true"),
	}
}

func getStr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
