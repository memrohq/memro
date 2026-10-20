package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/metrics"
)

type sentryProvider struct {
	m *Manager
}

func newSentryProvider(m *Manager) *sentryProvider {
	return &sentryProvider{m: m}
}

func (p *sentryProvider) enabled() bool {
	return p.m.cfg.SentryEnabled()
}

func (p *sentryProvider) info() ProviderInfo {
	return ProviderInfo{
		Name:             ProviderSentry,
		Enabled:          p.enabled(),
		SourcesManagedBy: "memro", // user selects projects in dashboard
	}
}

func (p *sentryProvider) authorizeURL(state string) string {
	base := p.m.cfg.SentryOAuthURL
	if base == "" {
		base = "https://sentry.io/oauth/authorize/"
	}
	q := url.Values{
		"client_id":     {p.m.cfg.SentryClientID},
		"response_type": {"code"},
		"scope":         {"project:read,event:read,issue:read,org:read"},
		"state":         {state},
		"redirect_uri":  {p.m.callbackURL(ProviderSentry)},
	}
	return base + "?" + q.Encode()
}

type sentryTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (p *sentryProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"Sentry did not return an authorization code."}
	}

	tokenURL := "https://sentry.io/oauth/token/"
	form := url.Values{
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"client_id":     {p.m.cfg.SentryClientID},
		"client_secret": {p.m.cfg.SentryClientSecret},
		"redirect_uri":  {p.m.callbackURL(ProviderSentry)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tResp sentryTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, err
	}
	if tResp.Error != "" || tResp.AccessToken == "" {
		return nil, &UserError{"Sentry sign-in failed: " + tResp.ErrorDesc}
	}

	// Query organizations
	var orgs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := p.get(ctx, tResp.AccessToken, "/organizations/", &orgs); err != nil || len(orgs) == 0 {
		return nil, fmt.Errorf("sentry list organizations: %w", err)
	}

	// Query current user
	var user struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	_ = p.get(ctx, tResp.AccessToken, "/users/me/", &user)

	org := orgs[0]
	userName := user.Name
	if userName == "" {
		userName = user.Email
	}

	return &authorized{
		accountID:   org.Slug,
		accountName: org.Name,
		userID:      user.ID,
		userName:    userName,
		creds: map[string]any{
			"access_token":      tResp.AccessToken,
			"refresh_token":     tResp.RefreshToken,
			"organization_slug": org.Slug,
			"organization_id":   org.ID,
		},
	}, nil
}

func (p *sentryProvider) get(ctx context.Context, token, path string, out any) error {
	base := p.m.cfg.SentryAPIURL
	if base == "" {
		base = "https://sentry.io/api/0"
	}
	u := strings.TrimRight(base, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &revokedError{"Sentry authorization revoked or expired."}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return &rateLimitedError{after: time.Minute}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sentry api status %d: %s", resp.StatusCode, string(b))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *sentryProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.syncProject(ctx, c, j.Key)
	case jobItem:
		return p.syncIssue(ctx, c, j.Key)
	default:
		return &permanentError{"unknown sentry job " + j.Kind}
	}
}

func (p *sentryProvider) refreshSources(ctx context.Context, c *conn) error {
	token := c.str("access_token")
	orgSlug := c.str("organization_slug")
	if orgSlug == "" {
		orgSlug = c.AccountID
	}

	var projs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := p.get(ctx, token, fmt.Sprintf("/organizations/%s/projects/", url.PathEscape(orgSlug)), &projs); err != nil {
		return err
	}

	var list []Source
	for _, prj := range projs {
		list = append(list, Source{
			ID:      prj.Slug,
			Name:    prj.Name,
			Private: false,
		})
	}

	return p.m.replaceSources(ctx, c.ID, list, false)
}

func (p *sentryProvider) syncProject(ctx context.Context, c *conn, projectSlug string) error {
	token := c.str("access_token")
	orgSlug := c.str("organization_slug")
	if orgSlug == "" {
		orgSlug = c.AccountID
	}

	cursor := ""
	_ = p.m.deps.Pool.QueryRow(ctx, `SELECT cursor FROM connector_sources WHERE connection_id = $1 AND external_id = $2`, c.ID, projectSlug).Scan(&cursor)

	path := fmt.Sprintf("/projects/%s/%s/issues/?statsPeriod=14d&limit=50", url.PathEscape(orgSlug), url.PathEscape(projectSlug))
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}

	base := p.m.cfg.SentryAPIURL
	if base == "" {
		base = "https://sentry.io/api/0"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("sentry list issues status %d", resp.StatusCode)
	}

	var issues []sentryIssue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return err
	}

	for _, issue := range issues {
		it := sentryItem(&issue, nil)
		if err := p.m.storeItem(ctx, c, it); err != nil {
			return err
		}
	}

	// Parse pagination link header if present
	nextCursor := parseSentryNextCursor(resp.Header.Get("Link"))
	if nextCursor != "" {
		_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET cursor = $3, last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`,
			c.ID, projectSlug, nextCursor)
		return p.m.enqueue(ctx, nil, c.ID, jobSync, projectSlug, nil, 0)
	}

	// Done
	_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET cursor = '', last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`,
		c.ID, projectSlug)
	return nil
}

func (p *sentryProvider) syncIssue(ctx context.Context, c *conn, issueID string) error {
	token := c.str("access_token")

	var issue sentryIssue
	if err := p.get(ctx, token, fmt.Sprintf("/issues/%s/", url.PathEscape(issueID)), &issue); err != nil {
		return err
	}

	var latestEvent sentryEvent
	_ = p.get(ctx, token, fmt.Sprintf("/issues/%s/events/latest/", url.PathEscape(issueID)), &latestEvent)

	it := sentryItem(&issue, &latestEvent)
	return p.m.storeItem(ctx, c, it)
}

func (p *sentryProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
	existing, err := p.m.sources(ctx, c.ID)
	if err != nil {
		return err
	}
	byID := map[string]Source{}
	for _, s := range existing {
		byID[s.ID] = s
	}

	for id, on := range enabled {
		s, ok := byID[id]
		if !ok {
			return &UserError{"Unknown project " + id + ". Refresh the project list."}
		}
		if err := p.m.setSourceEnabled(ctx, c.ID, id, on); err != nil {
			return err
		}
		if on && !s.Enabled {
			if err := p.m.enqueue(ctx, nil, c.ID, jobSync, id, nil, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *sentryProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	return nil
}

func (p *sentryProvider) manageURL(c *Connection) string {
	orgSlug := c.AccountID
	if orgSlug != "" {
		return fmt.Sprintf("https://sentry.io/settings/%s/integrations/", orgSlug)
	}
	return "https://sentry.io/settings/"
}

// VerifySentrySignature validates Sentry webhook HMAC-SHA256 signature.
func VerifySentrySignature(secret, sig string, body []byte) bool {
	if secret == "" || sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected))) == 1
}

type sentryWebhookPayload struct {
	Action string `json:"action"` // created, resolved, unresolved
	Data   struct {
		Issue struct {
			ID      string `json:"id"`
			Project struct {
				Slug string `json:"slug"`
			} `json:"project"`
		} `json:"issue"`
	} `json:"data"`
	Installation struct {
		UUID string `json:"uuid"`
	} `json:"installation"`
}

// SentryWebhook handles incoming alerts and issue events from Sentry.
func (m *Manager) SentryWebhook(ctx context.Context, hdr http.Header, body []byte) int {
	if m.cfg.SentryWebhookSecret == "" {
		return http.StatusServiceUnavailable
	}

	sig := hdr.Get("Sentry-Hook-Signature")
	if !VerifySentrySignature(m.cfg.SentryWebhookSecret, sig, body) {
		metrics.ConnectorWebhooks.WithLabelValues(ProviderSentry, "bad_signature").Inc()
		return http.StatusUnauthorized
	}

	var p sentryWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return http.StatusBadRequest
	}

	metrics.ConnectorWebhooks.WithLabelValues(ProviderSentry, p.Action).Inc()

	issueID := p.Data.Issue.ID
	if issueID == "" {
		return http.StatusOK
	}

	// Fan out to active Sentry connections
	conns, err := m.connectionsForAccount(ctx, ProviderSentry, "", true)
	if err != nil || len(conns) == 0 {
		return http.StatusOK
	}

	for _, c := range conns {
		if p.Action == "archived" {
			if cc, err := m.open(ctx, c); err == nil {
				_ = m.removeItem(ctx, cc, "issue:"+issueID)
			}
			continue
		}
		_ = m.enqueue(ctx, nil, c.ID, jobItem, issueID, nil, 5*time.Second)
	}

	return http.StatusOK
}

// parseSentryNextCursor parses Sentry pagination Link header:
// '<...&cursor=100:0:0>; rel="next"; results="true"'
func parseSentryNextCursor(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}
	parts := strings.Split(linkHeader, ",")
	for _, part := range parts {
		if strings.Contains(part, `rel="next"`) && strings.Contains(part, `results="true"`) {
			if start := strings.Index(part, "cursor="); start != -1 {
				sub := part[start+len("cursor="):]
				if end := strings.IndexAny(sub, `&>;`); end != -1 {
					return sub[:end]
				}
				return sub
			}
		}
	}
	return ""
}
