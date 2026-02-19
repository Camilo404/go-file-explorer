package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort         string
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration
	ServerIdleTimeout  time.Duration
	RequestTimeout     time.Duration
	StorageRoot        string
	MaxUploadSize      int64
	JWTSecret          string
	JWTAccessTTL       time.Duration
	JWTRefreshTTL      time.Duration
	CORSOrigins        []string
	RateLimitRPM       int
	AuthRateLimitRPM   int
	SearchMaxDepth     int
	SearchTimeout      time.Duration
	UsersFile          string
	AllowedMIMETypes   []string
	TrashRoot          string
	TrashIndexFile     string
	AuditLogFile       string
	ThumbnailRoot      string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		ServerPort:         getEnv("SERVER_PORT", "8080"),
		ServerReadTimeout:  getDuration("SERVER_READ_TIMEOUT", 15*time.Second),
		ServerWriteTimeout: getDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
		ServerIdleTimeout:  getDuration("SERVER_IDLE_TIMEOUT", 120*time.Second),
		RequestTimeout:     getDuration("REQUEST_TIMEOUT", 30*time.Second),
		StorageRoot:        getEnv("STORAGE_ROOT", "./data"),
		MaxUploadSize:      getInt64("MAX_UPLOAD_SIZE", 1073741824),
		JWTSecret:          strings.TrimSpace(os.Getenv("JWT_SECRET")),
		JWTAccessTTL:       getDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:      getDuration("JWT_REFRESH_TTL", 168*time.Hour),
		CORSOrigins:        splitCSV(getEnv("CORS_ORIGINS", "*")),
		RateLimitRPM:       getInt("RATE_LIMIT_RPM", 100),
		AuthRateLimitRPM:   getInt("AUTH_RATE_LIMIT_RPM", 10),
		SearchMaxDepth:     getInt("SEARCH_MAX_DEPTH", 10),
		SearchTimeout:      getDuration("SEARCH_TIMEOUT", 30*time.Second),
		UsersFile:          getEnv("USERS_FILE", "./users.json"),
		AllowedMIMETypes:   splitCSV(strings.TrimSpace(os.Getenv("ALLOWED_MIME_TYPES"))),
		TrashRoot:          getEnv("TRASH_ROOT", "./state/trash"),
		TrashIndexFile:     getEnv("TRASH_INDEX_FILE", "./state/trash-index.json"),
		AuditLogFile:       getEnv("AUDIT_LOG_FILE", "./state/audit.log"),
		ThumbnailRoot:      getEnv("THUMBNAIL_ROOT", "./state/thumbnails"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.JWTSecret) == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}

	if c.ServerPort == "" {
		return fmt.Errorf("SERVER_PORT cannot be empty")
	}

	if c.StorageRoot == "" {
		return fmt.Errorf("STORAGE_ROOT cannot be empty")
	}

	if c.MaxUploadSize <= 0 {
		return fmt.Errorf("MAX_UPLOAD_SIZE must be positive")
	}

	if c.RequestTimeout <= 0 {
		return fmt.Errorf("REQUEST_TIMEOUT must be positive")
	}

	if strings.TrimSpace(c.TrashRoot) == "" {
		return fmt.Errorf("TRASH_ROOT cannot be empty")
	}

	if strings.TrimSpace(c.TrashIndexFile) == "" {
		return fmt.Errorf("TRASH_INDEX_FILE cannot be empty")
	}

	if strings.TrimSpace(c.AuditLogFile) == "" {
		return fmt.Errorf("AUDIT_LOG_FILE cannot be empty")
	}

	if strings.TrimSpace(c.ThumbnailRoot) == "" {
		return fmt.Errorf("THUMBNAIL_ROOT cannot be empty")
	}

	return nil
}

func getEnv(key string, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	return v
}

func getInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return v
}

func getInt64(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}

	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}

	return v
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}

	return out
}
