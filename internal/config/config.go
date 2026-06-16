// Package config loads server configuration from environment variables (and .env in dev).
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           int
	DBType         string // "sqlite" | "postgres"
	DBPath         string
	DatabaseURL    string
	PoWDifficulty  int
	AdminSecret    string
	LogLevel       string
	LogFormat      string
	LogICE         bool
	FCMCredentials string // path to Firebase service-account JSON; empty = push disabled

	// File storage (S3-compatible). Feature is disabled unless S3Bucket is set.
	S3Endpoint             string
	S3Region               string
	S3Bucket               string
	S3AccessKey            string
	S3SecretKey            string
	S3UseSSL               bool
	MaxStoragePerUserBytes int64
	MaxFileSizeBytes       int64
	AvatarMaxBytes         int64
}

// Load reads .env (if present) then environment variables and returns Config.
func Load() Config {
	_ = godotenv.Load() // ignore error; .env is optional

	return Config{
		Port:           getInt("PORT", 3000),
		DBType:         getStr("DB_TYPE", "sqlite"),
		DBPath:         getStr("DB_PATH", "./data/messenger.db"),
		DatabaseURL:    getStr("DATABASE_URL", ""),
		PoWDifficulty:  getInt("POW_DIFFICULTY", 14),
		AdminSecret:    getStr("ADMIN_SECRET", ""),
		LogLevel:       getStr("LOG_LEVEL", "info"),
		LogFormat:      getStr("LOG_FORMAT", "json"),
		LogICE:         strings.EqualFold(getStr("LOG_ICE", "false"), "true"),
		FCMCredentials: getStr("FCM_CREDENTIALS", ""),

		S3Endpoint:             getStr("S3_ENDPOINT", ""),
		S3Region:               getStr("S3_REGION", ""),
		S3Bucket:               getStr("S3_BUCKET", ""),
		S3AccessKey:            getStr("S3_ACCESS_KEY", ""),
		S3SecretKey:            getStr("S3_SECRET_KEY", ""),
		S3UseSSL:               getBool("S3_USE_SSL", true),
		MaxStoragePerUserBytes: int64(getInt("MAX_STORAGE_PER_USER_GB", 10)) * 1024 * 1024 * 1024,
		MaxFileSizeBytes:       int64(getInt("MAX_FILE_SIZE_MB", 25)) * 1024 * 1024,
		AvatarMaxBytes:         int64(getInt("AVATAR_MAX_SIZE_MB", 2)) * 1024 * 1024,
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

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return strings.EqualFold(v, "true") || v == "1"
}
