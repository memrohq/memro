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
	"strconv"
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
			"expires_at":        tokenExpiry(p.m.now(), tResp.ExpiresIn),
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

func (p *sentryProvider) tokenEndpoint() tokenEndpoint {
	return tokenEndpoint{URL: "https://sentry.io/oauth/token/", ClientID: p.m.cfg.SentryClientID,
		ClientSecret: p.m.cfg.SentryClientSecret}
}

// call is a GET with the connection's token, refreshed as needed. It
// returns the Link header's next cursor (results="true" only).
func (p *sentryProvider) call(ctx context.Context, c *conn, path string, out any) (string, error) {
	base := p.m.cfg.SentryAPIURL
	if base == "" {
		base = "https://sentry.io/api/0"
	}
	resp, err := p.m.oauthDo(ctx, c, p.tokenEndpoint(), func(token string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := apiStatus("sentry", resp); err != nil {
		return "", err
	}
	return parseSentryNextCursor(resp.Header.Get("Link")), json.NewDecoder(resp.Body).Decode(out)
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
	orgSlug := c.str("organization_slug")
	if orgSlug == "" {
		orgSlug = c.AccountID
	}

	var projs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if _, err := p.call(ctx, c, fmt.Sprintf("/organizations/%s/projects/", url.PathEscape(orgSlug)), &projs); err != nil {
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

// syncProject reads one page of the project's issues seen in the last 14
// days and queues the next page until the pass is done.
func (p *sentryProvider) syncProject(ctx context.Context, c *conn, projectSlug string) error {
	src, err := p.m.source(ctx, c.ID, projectSlug)
	if err != nil {
		return err
	}
	if src == nil || !src.Enabled {
		return nil
	}
	orgSlug := c.str("organization_slug")
	if orgSlug == "" {
		orgSlug = c.AccountID
	}
	q := url.Values{"statsPeriod": {"14d"}, "limit": {"100"}, "sort": {"date"}}
	if src.cursor != "" {
		q.Set("cursor", src.cursor)
	}
	var issues []sentryIssue
	next, err := p.call(ctx, c, fmt.Sprintf("/projects/%s/%s/issues/?%s", url.PathEscape(orgSlug), url.PathEscape(projectSlug), q.Encode()), &issues)
	if err != nil {
		return err
	}
	for i := range issues {
		if err := p.m.storeItem(ctx, c, sentryItem(&issues[i], nil)); err != nil {
			return err
		}
	}
	if next != "" && len(issues) > 0 {
		if err := p.m.saveCursor(ctx, c.ID, projectSlug, next, false); err != nil {
			return err
		}
		return p.m.enqueue(ctx, nil, c.ID, jobSync, projectSlug, nil, 0)
	}
	return p.m.saveCursor(ctx, c.ID, projectSlug, "", true)
}

func (p *sentryProvider) syncIssue(ctx context.Context, c *conn, issueID string) error {
	var issue sentryIssue
	if _, err := p.call(ctx, c, fmt.Sprintf("/issues/%s/", url.PathEscape(issueID)), &issue); err != nil {
		return err
	}
	// Only projects the user chose: a webhook may name any issue.
	if src, err := p.m.source(ctx, c.ID, issue.Project.Slug); err != nil || src == nil || !src.Enabled {
		return err
	}
	var latestEvent sentryEvent
	_, _ = p.call(ctx, c, fmt.Sprintf("/issues/%s/events/latest/", url.PathEscape(issueID)), &latestEvent)
	return p.m.storeItem(ctx, c, sentryItem(&issue, &latestEvent))
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

	if tsHeader := hdr.Get("Sentry-Hook-Timestamp"); tsHeader != "" {
		sec, err := strconv.ParseInt(tsHeader, 10, 64)
		if err != nil || !VerifyWebhookTimestamp(time.Unix(sec, 0), m.now()) {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderSentry, "stale_timestamp").Inc()
			return http.StatusUnauthorized
		}
	}

	deliveryID := hdr.Get("Request-ID")
	if deliveryID == "" {
		deliveryID = hdr.Get("Request-Id")
	}
	if deliveryID != "" {
		claimed, err := m.ClaimWebhookDelivery(ctx, ProviderSentry, deliveryID)
		if err != nil {
			return http.StatusInternalServerError
		}
		if !claimed {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderSentry, "duplicate").Inc()
			return http.StatusOK
		}
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

	// Only connections that chose this project; each re-fetches the issue
	// with its own token.
	slug := p.Data.Issue.Project.Slug
	if slug == "" {
		return http.StatusOK
	}

	if m.deps.Pool == nil {
		return http.StatusOK
	}

	conns, err := m.connectionsWithSource(ctx, ProviderSentry, "", slug)
	if err != nil {
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
