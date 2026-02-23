package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort              string
	ServerReadHeaderTimeout time.Duration
	ServerWriteTimeout      time.Duration
	ServerIdleTimeout       time.Duration
	RequestTimeout          time.Duration
	TransferTimeout         time.Duration
	TransferIdleTimeout     time.Duration
	StorageRoot             string
	MaxUploadSize           int64
	JWTSecret               string
	JWTAccessTTL            time.Duration
	JWTRefreshTTL           time.Duration
	CORSOrigins             []string
	RateLimitRPM            int
	AuthRateLimitRPM        int
	SearchMaxDepth          int
	SearchTimeout           time.Duration
	AllowedMIMETypes        []string
	TrashRoot               string
	ThumbnailRoot           string

	// Chunked uploads
	ChunkTempDir string
	ChunkMaxSize int64
	ChunkExpiry  time.Duration

	// Database (required)
	DatabaseURL string
	DBMaxConns  int32
	DBMinConns  int32
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		ServerPort:              getEnv("SERVER_PORT", "8080"),
		ServerReadHeaderTimeout: getDuration("SERVER_READ_HEADER_TIMEOUT", 5*time.Second),
		ServerWriteTimeout:      getDuration("SERVER_WRITE_TIMEOUT", 10*time.Minute),
		ServerIdleTimeout:       getDuration("SERVER_IDLE_TIMEOUT", 120*time.Second),
		RequestTimeout:          getDuration("REQUEST_TIMEOUT", 30*time.Second),
		TransferTimeout:         getDuration("TRANSFER_TIMEOUT", 10*time.Minute),
		TransferIdleTimeout:     getDuration("TRANSFER_IDLE_TIMEOUT", 60*time.Second),
		StorageRoot:             getEnv("STORAGE_ROOT", "./data"),
		MaxUploadSize:           getInt64("MAX_UPLOAD_SIZE", 1073741824),
		JWTSecret:               strings.TrimSpace(os.Getenv("JWT_SECRET")),
		JWTAccessTTL:            getDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:           getDuration("JWT_REFRESH_TTL", 168*time.Hour),
		CORSOrigins:             splitCSV(getEnv("CORS_ORIGINS", "*")),
		RateLimitRPM:            getInt("RATE_LIMIT_RPM", 0),
		AuthRateLimitRPM:        getInt("AUTH_RATE_LIMIT_RPM", 60),
		SearchMaxDepth:          getInt("SEARCH_MAX_DEPTH", 10),
		SearchTimeout:           getDuration("SEARCH_TIMEOUT", 30*time.Second),
		AllowedMIMETypes:        splitCSV(strings.TrimSpace(os.Getenv("ALLOWED_MIME_TYPES"))),
		TrashRoot:               getEnv("TRASH_ROOT", "./data/.trash"),
		ThumbnailRoot:           getEnv("THUMBNAIL_ROOT", "./data/.thumbnails"),

		ChunkTempDir: getEnv("CHUNK_TEMP_DIR", "./data/.chunks"),
		ChunkMaxSize: getInt64("CHUNK_MAX_SIZE", 50*1024*1024),
		ChunkExpiry:  getDuration("CHUNK_EXPIRY", 24*time.Hour),

		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),

		DBMaxConns: int32(getInt("DB_MAX_CONNS", 10)),
		DBMinConns: int32(getInt("DB_MIN_CONNS", 2)),
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

	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters long for security; generate one with: openssl rand -base64 48")
	}

	// Warn about known-insecure default values.
	if c.JWTSecret == "change-me-in-production" {
		return fmt.Errorf("JWT_SECRET is set to the insecure default; please set a strong random value")
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

	if strings.TrimSpace(c.ThumbnailRoot) == "" {
		return fmt.Errorf("THUMBNAIL_ROOT cannot be empty")
	}

	if strings.TrimSpace(c.ChunkTempDir) == "" {
		return fmt.Errorf("CHUNK_TEMP_DIR cannot be empty")
	}

	if c.ChunkMaxSize <= 0 {
		return fmt.Errorf("CHUNK_MAX_SIZE must be positive")
	}

	if c.ChunkExpiry <= 0 {
		return fmt.Errorf("CHUNK_EXPIRY must be positive")
	}

	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// Security warnings (non-fatal but logged).
	for _, origin := range c.CORSOrigins {
		if origin == "*" {
			slog.Warn("CORS_ORIGINS is set to wildcard '*' — this is insecure for production; set specific origins instead")
			break
		}
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
