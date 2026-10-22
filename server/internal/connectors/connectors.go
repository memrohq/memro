// Package connectors syncs content from Slack workspaces and GitHub App
// installations into the memory of the agent that connected them (ADR 0011).
//
// A connection is authorised with OAuth and kept current by provider
// webhooks (Slack Events API, GitHub App webhooks) plus a periodic
// reconciliation sync. All provider work runs as small, retryable jobs in
// Postgres (connector_jobs), so it survives restarts and is shared safely by
// every running API instance.
package connectors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/connectors/dce"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/store"
)

const (
	ProviderSlack   = "slack"
	ProviderGitHub  = "github"
	ProviderLinear  = "linear"
	ProviderSentry  = "sentry"
	ProviderNotion  = "notion"
	ProviderJira    = "jira"
	ProviderGitLab  = "gitlab"
	ProviderGoogle  = "google"
	ProviderDiscord = "discord"

	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusError   = "error"
	StatusRevoked = "revoked"
)

// Connection is what the API returns about a connection. Credentials are
// never part of it.
type Connection struct {
	ID               uuid.UUID  `json:"id"`
	Provider         string     `json:"provider"`
	AccountID        string     `json:"account_id"`
	AccountName      string     `json:"account_name"`
	ExternalUserID   string     `json:"external_user_id"`
	ExternalUserName string     `json:"external_user_name"`
	Status           string     `json:"status"`
	StatusDetail     string     `json:"status_detail"`
	CreatedAt        time.Time  `json:"created_at"`
	LastSyncedAt     *time.Time `json:"last_synced_at"`
	SourcesEnabled   int        `json:"sources_enabled"`
	SourcesTotal     int        `json:"sources_total"`
	Memories         int        `json:"memories"`
	PendingJobs      int        `json:"pending_jobs"`
	// ManageURL is where the user changes what the provider shares (GitHub
	// repository access); empty when Memro manages it (Slack channels).
	ManageURL string `json:"manage_url,omitempty"`

	TenantID uuid.UUID `json:"-"`
	AgentID  string    `json:"-"`
	sealed   []byte
}

// Source is a Slack channel or a GitHub repository of a connection.
type Source struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Private      bool       `json:"private"`
	Enabled      bool       `json:"enabled"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	cursor       string
}

// ProviderInfo tells the dashboard which connectors this server offers.
type ProviderInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// SourcesManagedBy is "memro" when channels are picked in the dashboard
	// and "provider" when they are chosen on the provider's site.
	SourcesManagedBy string `json:"sources_managed_by"`
	Kind             string `json:"kind,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	Category         string `json:"category,omitempty"`
	Description      string `json:"description,omitempty"`
	DocsURL          string `json:"docs_url,omitempty"`
	// Fields are a token connector's connect form (the token and any
	// account details such as a subdomain).
	Fields []FieldInfo `json:"fields,omitempty"`
}

// FieldInfo is one input of a token connector's connect form.
type FieldInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

// Errors returned to API handlers.
var (
	ErrNotFound        = errors.New("connection not found")
	ErrUnknownProvider = errors.New("unknown connector")
	ErrDisabled        = errors.New("this connector is not configured on this server")
	ErrBadState        = errors.New("this sign-in link is invalid or has expired; start again from the dashboard")
)

// UserError is a failure the user can act on; its message is shown as is.
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

// Errors that change how a failed job is handled.
type (
	// rateLimitedError: try again after the given delay without counting an attempt.
	rateLimitedError struct{ after time.Duration }
	// revokedError: the provider no longer accepts the connection's credentials.
	revokedError struct{ msg string }
	// permanentError: retrying cannot help; drop the job.
	permanentError struct{ msg string }
)

func (e *rateLimitedError) Error() string { return fmt.Sprintf("rate limited; retry in %s", e.after) }
func (e *revokedError) Error() string     { return e.msg }
func (e *permanentError) Error() string   { return e.msg }

// provider is one connector implementation.
type provider interface {
	enabled() bool
	info() ProviderInfo
	authorizeURL(state string) string
	// complete finishes the OAuth redirect and returns the account to save.
	complete(ctx context.Context, q url.Values) (*authorized, error)
	// handle runs a job of one of the provider's kinds.
	handle(ctx context.Context, c *conn, j *job) error
	// setSources applies the user's channel selection.
	setSources(ctx context.Context, c *conn, enabled map[string]bool) error
	// revoke gives up the connection's credentials at the provider (best effort).
	revoke(ctx context.Context, c *conn, lastForAccount bool) error
	manageURL(c *Connection) string
}

// authorized is the result of a completed OAuth flow.
type authorized struct {
	accountID, accountName string
	userID, userName       string
	creds                  any
}

// Deps are the server components the connectors write memories through.
type Deps struct {
	Config  config.Config
	Pool    *pgxpool.Pool
	Store   *store.Store
	Indexer engine.Indexer
	// Invalidate drops cached search results for an agent.
	Invalidate func(agentID string)
	Quota      extension.QuotaPolicy
	Usage      extension.UsageMeter
	HTTPClient *http.Client
	// Catalog replaces the embedded connector catalog and CatalogClient the
	// hardened client its requests use; with CatalogClient set, private
	// addresses are allowed. Both exist for tests against fake APIs and are
	// never set in production.
	Catalog       []*dce.Manifest
	CatalogClient *http.Client
}

// Manager owns the connectors, their jobs and their webhooks.
type Manager struct {
	deps      Deps
	cfg       config.ConnectorsConfig
	sealer    *sealer
	providers map[string]provider
	slack     *slackProvider
	github    *githubProvider
	linear    *linearProvider
	sentry    *sentryProvider
	notion    *notionProvider
	jira      *jiraProvider
	gitlab    *gitlabProvider
	catalog   []*catalogProvider
	http      *http.Client
	wake      chan struct{}
	now       func() time.Time

	namesMu sync.Mutex
	names   map[string]cachedName // Slack user names, by team/user

	deliveriesMu sync.Mutex
	deliveries   map[string]time.Time
}

// New builds the manager. Connectors whose credentials are missing stay
// disabled; a malformed encryption key or GitHub private key is an error.
func New(d Deps) (*Manager, error) {
	cfg := d.Config.Connectors
	if cfg.SlackAPIURL == "" {
		cfg.SlackAPIURL = "https://slack.com/api"
	}
	if cfg.SlackOAuthURL == "" {
		cfg.SlackOAuthURL = "https://slack.com/oauth/v2/authorize"
	}
	if cfg.GitHubAPIURL == "" {
		cfg.GitHubAPIURL = "https://api.github.com"
	}
	if cfg.GitHubWebURL == "" {
		cfg.GitHubWebURL = "https://github.com"
	}
	if cfg.LinearAPIURL == "" {
		cfg.LinearAPIURL = "https://api.linear.app/graphql"
	}
	if cfg.LinearOAuthURL == "" {
		cfg.LinearOAuthURL = "https://linear.app/oauth/authorize"
	}
	if cfg.SentryAPIURL == "" {
		cfg.SentryAPIURL = "https://sentry.io/api/0"
	}
	if cfg.SentryOAuthURL == "" {
		cfg.SentryOAuthURL = "https://sentry.io/oauth/authorize/"
	}
	if cfg.NotionAPIURL == "" {
		cfg.NotionAPIURL = "https://api.notion.com/v1"
	}
	if cfg.NotionOAuthURL == "" {
		cfg.NotionOAuthURL = "https://api.notion.com/v1/oauth/authorize"
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 30 * time.Minute
	}
	if cfg.BackfillDays < 0 {
		cfg.BackfillDays = 0
	}
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	m := &Manager{deps: d, cfg: cfg, http: client, wake: make(chan struct{}, 1), now: time.Now,
		providers: map[string]provider{}, names: map[string]cachedName{}, deliveries: map[string]time.Time{}}
	if cfg.EncryptionKey != "" {
		s, err := newSealer(cfg.EncryptionKey, cfg.PreviousEncryptionKey)
		if err != nil {
			return nil, err
		}
		m.sealer = s
	}
	m.slack = &slackProvider{m: m}
	gh, err := newGitHubProvider(m)
	if err != nil {
		return nil, err
	}
	m.github = gh
	m.linear = newLinearProvider(m)
	m.sentry = newSentryProvider(m)
	m.notion = newNotionProvider(m)
	m.jira = newJiraProvider(m)
	m.gitlab = newGitLabProvider(m)
	m.providers[ProviderSlack] = m.slack
	m.providers[ProviderGitHub] = m.github
	m.providers[ProviderLinear] = m.linear
	m.providers[ProviderSentry] = m.sentry
	m.providers[ProviderNotion] = m.notion
	m.providers[ProviderJira] = m.jira
	m.providers[ProviderGitLab] = m.gitlab

	// Catalog connectors (ADR 0014): one manifest each, connected with a token.
	catalog := d.Catalog
	if catalog == nil {
		var err error
		if catalog, err = dce.Catalog(); err != nil {
			return nil, fmt.Errorf("connector catalog: %w", err)
		}
	}
	rt := dce.NewRuntimeWithOptions(d.CatalogClient, nil, dce.Options{AllowPrivateIPs: d.CatalogClient != nil})
	for _, man := range catalog {
		if _, native := m.providers[man.Metadata.Name]; native {
			return nil, fmt.Errorf("catalog connector %q clashes with a built-in connector", man.Metadata.Name)
		}
		cp := &catalogProvider{m: m, man: man, rt: rt}
		m.providers[man.Metadata.Name] = cp
		m.catalog = append(m.catalog, cp)
	}

	for name, p := range m.providers {
		slog.Info("connector", "provider", name, "enabled", p.enabled())
	}
	return m, nil
}

// Providers lists every connector and whether it is configured.
func (m *Manager) Providers() []ProviderInfo {
	list := []ProviderInfo{
		withMetadata(m.slack.info(), "Slack", "oauth", "communication", "Conversations from channels you choose, kept up to date as people post, edit, and delete."),
		withMetadata(m.github.info(), "GitHub", "oauth", "engineering", "Issues, pull requests, discussions, and commits from the repositories you share."),
		withMetadata(m.linear.info(), "Linear", "oauth", "work", "Issues, initiatives, priorities, and discussion from the teams you select."),
		withMetadata(m.sentry.info(), "Sentry", "oauth", "engineering", "Production issues, exceptions, stack traces, and culprit files from the projects you choose."),
		withMetadata(m.notion.info(), "Notion", "oauth", "knowledge", "Product specs, architectural decisions, RFCs, and engineering knowledge bases."),
		withMetadata(m.jira.info(), "Jira", "oauth", "work", "Epics, stories, bugs, comments, and sprint changes from your Atlassian workspace."),
		withMetadata(m.gitlab.info(), "GitLab", "oauth", "engineering", "Merge requests, issues, commits, and pipeline logs from your GitLab repositories."),
	}
	for _, p := range m.catalog {
		list = append(list, p.info())
	}
	return list
}

func withMetadata(info ProviderInfo, displayName, kind, category, description string) ProviderInfo {
	info.DisplayName = displayName
	info.Kind = kind
	info.Category = category
	if info.Description == "" {
		info.Description = description
	}
	return info
}

func (m *Manager) provider(name string) (provider, error) {
	p, ok := m.providers[name]
	if !ok {
		return nil, ErrUnknownProvider
	}
	if !p.enabled() {
		return nil, ErrDisabled
	}
	return p, nil
}

// DashboardURL is where the browser lands after an OAuth flow.
func (m *Manager) DashboardURL(provider string, err error) string {
	q := url.Values{"provider": {provider}}
	if err != nil {
		q.Set("error", err.Error())
	} else {
		q.Set("connected", "1")
	}
	return m.deps.Config.AppURL + "/integrations?" + q.Encode()
}

func (m *Manager) callbackURL(provider string) string {
	return m.cfg.CallbackBaseURL + "/connectors/" + provider + "/callback"
}

// conn is a connection loaded for a job, with its decrypted credentials.
type conn struct {
	Connection
	m     *Manager
	creds map[string]any
}

func (c *conn) str(key string) string {
	v, _ := c.creds[key].(string)
	return v
}

// saveCreds seals and stores updated credentials (e.g. after a token refresh).
func (c *conn) saveCreds(ctx context.Context) error {
	sealed, err := c.m.sealer.Seal(c.creds, c.ID.String())
	if err != nil {
		return err
	}
	_, err = c.m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET credentials = $2, updated_at = NOW() WHERE id = $1`, c.ID, sealed)
	return err
}

func (m *Manager) open(ctx context.Context, c *Connection) (*conn, error) {
	cc := &conn{Connection: *c, m: m, creds: map[string]any{}}
	stale, err := m.sealer.Open(c.sealed, c.ID.String(), &cc.creds)
	if err != nil {
		return nil, &permanentError{"cannot decrypt connector credentials: " + err.Error()}
	}
	if stale {
		if err := cc.saveCreds(ctx); err != nil {
			slog.Warn("re-seal connector credentials", "connection", c.ID, "err", err)
		}
	}
	return cc, nil
}

const (
	webhookMaxSkew     = 5 * time.Minute
	webhookDeliveryTTL = 24 * time.Hour
)

// VerifyWebhookTimestamp checks if a timestamp is within the accepted clock skew window (5 minutes).
func VerifyWebhookTimestamp(ts, now time.Time) bool {
	d := now.Sub(ts)
	return d <= webhookMaxSkew && d >= -webhookMaxSkew
}

// ClaimWebhookDelivery records a delivery ID for a provider. It returns true if
// the delivery was newly claimed, and false if it was already processed (a replay).
func (m *Manager) ClaimWebhookDelivery(ctx context.Context, provider, deliveryID string) (bool, error) {
	if deliveryID == "" {
		return true, nil
	}
	if m.deps.Pool == nil {
		m.deliveriesMu.Lock()
		defer m.deliveriesMu.Unlock()
		key := provider + ":" + deliveryID
		if _, exists := m.deliveries[key]; exists {
			return false, nil
		}
		m.deliveries[key] = m.now()
		return true, nil
	}
	tag, err := m.deps.Pool.Exec(ctx, `INSERT INTO connector_webhook_deliveries (provider, delivery_id, received_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (provider, delivery_id) DO NOTHING`, provider, deliveryID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CleanWebhookDeliveries drops delivery records older than the given TTL.
func (m *Manager) CleanWebhookDeliveries(ctx context.Context, olderThan time.Duration) (int64, error) {
	if m.deps.Pool == nil {
		m.deliveriesMu.Lock()
		defer m.deliveriesMu.Unlock()
		cutoff := m.now().Add(-olderThan)
		var cleaned int64
		for k, t := range m.deliveries {
			if t.Before(cutoff) {
				delete(m.deliveries, k)
				cleaned++
			}
		}
		return cleaned, nil
	}
	tag, err := m.deps.Pool.Exec(ctx, `DELETE FROM connector_webhook_deliveries WHERE received_at < NOW() - make_interval(secs => $1)`, olderThan.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
