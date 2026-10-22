// Package config loads server configuration from environment variables.
package config

import (
	"errors"
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

	// Agent self-registration (ADR 0013). IdentityPowBits is the
	// proof-of-work difficulty for POST /identity (leading zero bits of
	// SHA-256; 20 is about a second of CPU). Proofs that are sent are always
	// checked; IdentityPowRequired also rejects registrations without one.
	IdentityPowBits     int
	IdentityPowRequired bool

	// Caps for agents nobody has claimed (no owner). 0 means no cap, which
	// suits self-hosting; Memro Cloud sets them.
	UnclaimedMaxMemories  int
	UnclaimedMaxFileBytes int64
	// UnclaimedCapHint is appended to the cap message, with {agent_id}
	// replaced, e.g. how to pay for a plan.
	UnclaimedCapHint string

	// MetricsToken protects GET /metrics and GET /stats/cache with Bearer auth.
	// When set, it must be at least 32 characters. When empty, metrics and
	// cache stats are served without auth (for self-hosters) and a warning is logged.
	MetricsToken string

	// Public URLs handed to `memro connect`.
	AppURL string // dashboard, e.g. https://app.memro.co
	MCPURL string // hosted MCP endpoint, e.g. https://mcp.memro.co/mcp

	// Connectors (ADR 0011). Each is enabled only when its credentials are set.
	Connectors ConnectorsConfig

	// DevLogOTP writes dashboard sign-in codes to the server log (and still
	// emails them when SMTP is configured), so local development and staging
	// smoke tests can sign in. Never enable it in production.
	DevLogOTP bool
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
		AllowLegacySignatures: envBool("ALLOW_LEGACY_SIGNATURES", false),
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
		IdentityPowBits:       envInt("IDENTITY_POW_BITS", 20),
		IdentityPowRequired:   envBool("IDENTITY_POW_REQUIRED", false),
		UnclaimedMaxMemories:  envInt("UNCLAIMED_MAX_MEMORIES", 0),
		UnclaimedMaxFileBytes: int64(envInt("UNCLAIMED_MAX_FILE_BYTES", 0)),
		UnclaimedCapHint:      strings.TrimSpace(os.Getenv("UNCLAIMED_CAP_HINT")),
		MetricsToken:          strings.TrimSpace(os.Getenv("METRICS_TOKEN")),
		DevLogOTP:             envBool("DEV_LOG_OTP", false),
		AppURL:                strings.TrimRight(env("APP_URL", "http://localhost:5173"), "/"),
		MCPURL:                env("MCP_URL", "http://localhost:8080/mcp"),
		Connectors: ConnectorsConfig{
			EncryptionKey:         strings.TrimSpace(os.Getenv("CONNECTOR_ENCRYPTION_KEY")),
			PreviousEncryptionKey: strings.TrimSpace(os.Getenv("CONNECTOR_ENCRYPTION_KEY_PREVIOUS")),
			CallbackBaseURL:       strings.TrimRight(os.Getenv("CONNECTOR_CALLBACK_BASE_URL"), "/"),
			BackfillDays:          envInt("CONNECTOR_BACKFILL_DAYS", 30),
			SyncInterval:          time.Duration(envInt("CONNECTOR_SYNC_INTERVAL_MINUTES", 30)) * time.Minute,
			Workers:               envInt("CONNECTOR_WORKERS", 4),
			SlackClientID:         strings.TrimSpace(os.Getenv("SLACK_CLIENT_ID")),
			SlackClientSecret:     strings.TrimSpace(os.Getenv("SLACK_CLIENT_SECRET")),
			SlackSigningSecret:    strings.TrimSpace(os.Getenv("SLACK_SIGNING_SECRET")),
			GitHubAppID:           strings.TrimSpace(os.Getenv("GITHUB_APP_ID")),
			GitHubAppSlug:         strings.TrimSpace(os.Getenv("GITHUB_APP_SLUG")),
			GitHubAppPrivateKey:   pemFromEnv(os.Getenv("GITHUB_APP_PRIVATE_KEY")),
			GitHubClientID:        strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
			GitHubClientSecret:    strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
			GitHubWebhookSecret:   strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET")),
			LinearClientID:        strings.TrimSpace(os.Getenv("LINEAR_CLIENT_ID")),
			LinearClientSecret:    strings.TrimSpace(os.Getenv("LINEAR_CLIENT_SECRET")),
			LinearWebhookSecret:   strings.TrimSpace(os.Getenv("LINEAR_WEBHOOK_SECRET")),
			SentryClientID:        strings.TrimSpace(os.Getenv("SENTRY_CLIENT_ID")),
			SentryClientSecret:    strings.TrimSpace(os.Getenv("SENTRY_CLIENT_SECRET")),
			SentryWebhookSecret:   strings.TrimSpace(os.Getenv("SENTRY_WEBHOOK_SECRET")),
			NotionClientID:        strings.TrimSpace(os.Getenv("NOTION_CLIENT_ID")),
			NotionClientSecret:    strings.TrimSpace(os.Getenv("NOTION_CLIENT_SECRET")),
			NotionWebhookSecret:   strings.TrimSpace(os.Getenv("NOTION_WEBHOOK_SECRET")),
			JiraClientID:          strings.TrimSpace(os.Getenv("JIRA_CLIENT_ID")),
			JiraClientSecret:      strings.TrimSpace(os.Getenv("JIRA_CLIENT_SECRET")),
			JiraWebhookSecret:     strings.TrimSpace(os.Getenv("JIRA_WEBHOOK_SECRET")),
			GitLabClientID:        strings.TrimSpace(os.Getenv("GITLAB_CLIENT_ID")),
			GitLabClientSecret:    strings.TrimSpace(os.Getenv("GITLAB_CLIENT_SECRET")),
			GitLabWebhookSecret:   strings.TrimSpace(os.Getenv("GITLAB_WEBHOOK_SECRET")),
			GoogleClientID:        strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
			GoogleClientSecret:    strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
			GoogleWebhookSecret:   strings.TrimSpace(os.Getenv("GOOGLE_WEBHOOK_SECRET")),
			DiscordClientID:       strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
			DiscordClientSecret:   strings.TrimSpace(os.Getenv("DISCORD_CLIENT_SECRET")),
			DiscordPublicKey:      strings.TrimSpace(os.Getenv("DISCORD_PUBLIC_KEY")),
		},
	}
	c.BaseURL = env("BASE_URL", "http://localhost:"+strconv.Itoa(c.Port))
	c.EmailFrom = env("EMAIL_FROM", c.EmailUser)
	// OAuth providers redirect the browser here. It must be on the dashboard's
	// origin (which proxies /api to this server) so the browser-binding cookie
	// set when the flow started is sent back.
	if c.Connectors.CallbackBaseURL == "" {
		c.Connectors.CallbackBaseURL = c.AppURL + "/api"
	}
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, o)
			}
		}
	}
	return c
}

// Validate checks for configuration errors that must prevent the server from starting.
func (c Config) Validate() error {
	if c.MetricsToken != "" && len(c.MetricsToken) < 32 {
		return errors.New("METRICS_TOKEN must be at least 32 characters")
	}
	return nil
}

// ConnectorsConfig configures the Slack and GitHub connectors.
type ConnectorsConfig struct {
	// EncryptionKey (base64, 32 bytes) seals provider tokens at rest. No
	// connector is enabled without it. PreviousEncryptionKey still decrypts
	// during a rotation.
	EncryptionKey         string
	PreviousEncryptionKey string
	// CallbackBaseURL is the public base of the OAuth callbacks, e.g.
	// https://app.memro.co/api (default: APP_URL + "/api").
	CallbackBaseURL string
	BackfillDays    int           // history synced when a channel or repository is added
	SyncInterval    time.Duration // reconciliation sync, in addition to webhooks
	Workers         int

	SlackClientID      string
	SlackClientSecret  string
	SlackSigningSecret string

	GitHubAppID         string
	GitHubAppSlug       string // github.com/apps/<slug>
	GitHubAppPrivateKey string // PEM
	GitHubClientID      string
	GitHubClientSecret  string
	GitHubWebhookSecret string

	LinearClientID      string
	LinearClientSecret  string
	LinearWebhookSecret string

	SentryClientID      string
	SentryClientSecret  string
	SentryWebhookSecret string

	NotionClientID      string
	NotionClientSecret  string
	NotionWebhookSecret string

	JiraClientID      string
	JiraClientSecret  string
	JiraWebhookSecret string

	GitLabClientID      string
	GitLabClientSecret  string
	GitLabWebhookSecret string

	GoogleClientID      string
	GoogleClientSecret  string
	GoogleWebhookSecret string

	DiscordClientID     string
	DiscordClientSecret string
	DiscordPublicKey    string

	// API endpoints, overridden in tests.
	SlackAPIURL     string
	GitHubAPIURL    string
	GitHubWebURL    string
	SlackOAuthURL   string
	LinearAPIURL    string
	LinearOAuthURL  string
	SentryAPIURL    string
	SentryOAuthURL  string
	NotionAPIURL    string
	NotionOAuthURL  string
	JiraAPIURL      string
	JiraOAuthURL    string
	GitLabAPIURL    string
	GitLabOAuthURL  string
	GoogleAPIURL    string
	GoogleOAuthURL  string
	DiscordAPIURL   string
	DiscordOAuthURL string
}

func (c ConnectorsConfig) SlackEnabled() bool {
	return c.EncryptionKey != "" && c.SlackClientID != "" && c.SlackClientSecret != "" && c.SlackSigningSecret != ""
}

func (c ConnectorsConfig) GitHubEnabled() bool {
	return c.EncryptionKey != "" && c.GitHubAppID != "" && c.GitHubAppSlug != "" && c.GitHubAppPrivateKey != "" &&
		c.GitHubClientID != "" && c.GitHubClientSecret != "" && c.GitHubWebhookSecret != ""
}

func (c ConnectorsConfig) LinearEnabled() bool {
	return c.EncryptionKey != "" && c.LinearClientID != "" && c.LinearClientSecret != ""
}

func (c ConnectorsConfig) SentryEnabled() bool {
	return c.EncryptionKey != "" && c.SentryClientID != "" && c.SentryClientSecret != ""
}

func (c ConnectorsConfig) NotionEnabled() bool {
	return c.EncryptionKey != "" && c.NotionClientID != "" && c.NotionClientSecret != ""
}

func (c ConnectorsConfig) JiraEnabled() bool {
	return c.EncryptionKey != "" && c.JiraClientID != "" && c.JiraClientSecret != ""
}

func (c ConnectorsConfig) GitLabEnabled() bool {
	return c.EncryptionKey != "" && c.GitLabClientID != "" && c.GitLabClientSecret != ""
}

func (c ConnectorsConfig) GoogleEnabled() bool {
	return c.EncryptionKey != "" && c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

func (c ConnectorsConfig) DiscordEnabled() bool {
	return c.EncryptionKey != "" && c.DiscordClientID != "" && c.DiscordClientSecret != ""
}

// pemFromEnv accepts a PEM with real newlines or with literal "\n", as
// secret stores and .env files often flatten them.
func pemFromEnv(v string) string {
	v = strings.TrimSpace(v)
	if strings.Contains(v, "\n") {
		return v
	}
	return strings.ReplaceAll(v, `\n`, "\n")
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
