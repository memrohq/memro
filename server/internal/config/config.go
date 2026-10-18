// Package config loads server configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port             int
	BaseURL          string
	DatabaseURL      string
	DBMaxConns       int32
	DBMinConns       int32
	MigrateOnStartup bool

	RateLimitMax    int
	RateLimitWindow time.Duration
	// TrustProxyHeaders makes the server read the client IP from
	// X-Forwarded-For / X-Real-IP. Enable only behind a reverse proxy.
	TrustProxyHeaders bool
	// AllowLegacySignatures accepts v1 signatures (timestamp + body only).
	// v1 does not bind the method, path or query, so it should be turned off
	// once every SDK sends v2 signatures.
	AllowLegacySignatures bool
	AllowedOrigins        []string

	EmbeddingProvider string
	EmbeddingModel    string
	OpenAIAPIKey      string
	VoyageAPIKey      string
	EmbeddingBaseURL  string
	EmbeddingCacheMax int

	FileStoragePath string
	MaxBodyBytes    int64

	EmailHost string
	EmailPort int
	EmailUser string
	EmailPass string
	EmailFrom string

	MemorySoftCap int
}

func Load() Config {
	c := Config{
		Port:                  envInt("PORT", 8081),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		DBMaxConns:            int32(envInt("DB_MAX_CONNECTIONS", 20)),
		DBMinConns:            int32(envInt("DB_MIN_CONNECTIONS", 2)),
		MigrateOnStartup:      envBool("MIGRATE_ON_STARTUP", true),
		RateLimitMax:          envInt("RATE_LIMIT_MAX", 200),
		RateLimitWindow:       time.Duration(envInt("RATE_LIMIT_WINDOW_SECONDS", 60)) * time.Second,
		TrustProxyHeaders:     envBool("TRUST_PROXY_HEADERS", false),
		AllowLegacySignatures: envBool("ALLOW_LEGACY_SIGNATURES", true),
		EmbeddingProvider:     strings.ToLower(env("EMBEDDING_PROVIDER", "openai")),
		EmbeddingModel:        os.Getenv("EMBEDDING_MODEL"),
		OpenAIAPIKey:          os.Getenv("OPENAI_API_KEY"),
		VoyageAPIKey:          env("VOYAGE_API_KEY", os.Getenv("ANTHROPIC_API_KEY")),
		EmbeddingBaseURL:      os.Getenv("EMBEDDING_BASE_URL"),
		EmbeddingCacheMax:     envInt("EMBEDDING_CACHE_SIZE", 2048),
		FileStoragePath:       env("FILE_STORAGE_PATH", "./data/files"),
		MaxBodyBytes:          int64(envInt("MAX_BODY_BYTES", 10_000_000)),
		EmailHost:             strings.TrimSpace(os.Getenv("EMAIL_HOST")),
		EmailPort:             envInt("EMAIL_PORT", 587),
		EmailUser:             strings.TrimSpace(os.Getenv("EMAIL_HOST_USER")),
		EmailPass:             strings.TrimSpace(os.Getenv("EMAIL_HOST_PASSWORD")),
		MemorySoftCap:         envInt("MEMORY_SOFT_CAP", 1000),
	}
	c.BaseURL = env("BASE_URL", "http://localhost:"+strconv.Itoa(c.Port))
	c.EmailFrom = env("EMAIL_FROM", c.EmailUser)
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, o)
			}
		}
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}
