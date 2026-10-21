package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Expiring OAuth tokens (Jira: 1 hour, GitLab: 2 hours, Sentry: 8 hours,
// Linear apps created after October 2025: 24 hours). The access token is
// refreshed shortly before it expires, and once more if the provider answers
// 401 anyway. A connection is revoked only when the refresh itself is
// refused, i.e. the user removed the app or the grant expired.
//
// Atlassian and GitLab rotate the refresh token on every use, so two workers
// (or two API instances) refreshing at once would invalidate each other: a
// refresh holds a Postgres advisory lock on the connection and re-reads the
// credentials first, adopting a token another worker just obtained.

// tokenEndpoint describes how a provider refreshes tokens.
type tokenEndpoint struct {
	URL          string
	ClientID     string
	ClientSecret string
	JSON         bool // Atlassian takes a JSON body; others a form
	RedirectURI  string
}

const refreshEarly = 5 * time.Minute

// tokenExpiry records when a fresh access token expires (expires_in seconds).
func tokenExpiry(now time.Time, expiresIn int64) float64 {
	if expiresIn <= 0 {
		return 0
	}
	return float64(now.Unix() + expiresIn)
}

// accessToken returns a usable access token for c, refreshing it first when
// it expires soon (or always, with force).
func (m *Manager) accessToken(ctx context.Context, c *conn, ep tokenEndpoint, force bool) (string, error) {
	if !force && !m.expiresSoon(c) {
		return c.str("access_token"), nil
	}
	if c.str("refresh_token") == "" {
		if force {
			return "", &revokedError{"The provider no longer accepts this connection. Reconnect it."}
		}
		return c.str("access_token"), nil
	}
	return m.refreshToken(ctx, c, ep, force)
}

func (m *Manager) expiresSoon(c *conn) bool {
	exp, _ := c.creds["expires_at"].(float64)
	return exp > 0 && time.Unix(int64(exp), 0).Before(m.now().Add(refreshEarly))
}

func (m *Manager) refreshToken(ctx context.Context, c *conn, ep tokenEndpoint, force bool) (string, error) {
	tx, err := m.deps.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('connector-refresh:' || $1::text, 0))`, c.ID); err != nil {
		return "", err
	}
	// Another worker may have refreshed while this one waited for the lock.
	var sealed []byte
	if err := tx.QueryRow(ctx, `SELECT credentials FROM connector_connections WHERE id = $1`, c.ID).Scan(&sealed); err != nil {
		return "", err
	}
	latest := map[string]any{}
	if _, err := m.sealer.Open(sealed, c.ID.String(), &latest); err != nil {
		return "", &permanentError{"cannot decrypt connector credentials: " + err.Error()}
	}
	if tok, _ := latest["access_token"].(string); tok != "" && tok != c.str("access_token") {
		c.creds = latest
		if !m.expiresSoon(c) {
			return tok, tx.Commit(ctx)
		}
	} else if !force && !m.expiresSoon(&conn{creds: latest}) {
		c.creds = latest
		return c.str("access_token"), tx.Commit(ctx)
	}

	refresh, _ := latest["refresh_token"].(string)
	t, err := m.exchangeRefresh(ctx, ep, refresh)
	if err != nil {
		return "", err
	}
	latest["access_token"] = t.AccessToken
	if t.RefreshToken != "" {
		latest["refresh_token"] = t.RefreshToken
	}
	latest["expires_at"] = tokenExpiry(m.now(), t.ExpiresIn)
	out, err := m.sealer.Seal(latest, c.ID.String())
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE connector_connections SET credentials = $2, updated_at = NOW() WHERE id = $1`, c.ID, out); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	c.creds = latest
	return t.AccessToken, nil
}

type refreshedToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
}

func (m *Manager) exchangeRefresh(ctx context.Context, ep tokenEndpoint, refresh string) (*refreshedToken, error) {
	vals := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refresh,
		"client_id":     ep.ClientID,
		"client_secret": ep.ClientSecret,
	}
	if ep.RedirectURI != "" {
		vals["redirect_uri"] = ep.RedirectURI
	}
	var body io.Reader
	ctype := "application/x-www-form-urlencoded"
	if ep.JSON {
		b, _ := json.Marshal(vals)
		body, ctype = bytes.NewReader(b), "application/json"
	} else {
		form := url.Values{}
		for k, v := range vals {
			form.Set(k, v)
		}
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Accept", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var t refreshedToken
	_ = json.Unmarshal(raw, &t)
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &rateLimitedError{after: time.Minute}
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("refresh token: provider status %d", resp.StatusCode)
	case resp.StatusCode >= 400 || t.AccessToken == "":
		// invalid_grant and friends: the grant is gone; only a new sign-in helps.
		return nil, &revokedError{"The provider refused to renew access (" + truncate(t.Error, 60) + "). Reconnect it."}
	}
	return &t, nil
}

// oauthDo sends a request built with a current access token. On 401 it
// refreshes once and retries; a second 401 revokes the connection. The
// caller closes the response body.
func (m *Manager) oauthDo(ctx context.Context, c *conn, ep tokenEndpoint, build func(token string) (*http.Request, error)) (*http.Response, error) {
	token, err := m.accessToken(ctx, c, ep, false)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		req, err := build(token)
		if err != nil {
			return nil, err
		}
		resp, err := m.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized {
			return resp, nil
		}
		resp.Body.Close()
		if attempt > 0 || c.str("refresh_token") == "" {
			return nil, &revokedError{"The provider no longer accepts this connection. Reconnect it."}
		}
		if token, err = m.accessToken(ctx, c, ep, true); err != nil {
			return nil, err
		}
	}
}

// apiStatus maps a provider API status to the job error it implies. 403
// means the token cannot read this one thing (a project the user lost access
// to), not that the connection is gone.
func apiStatus(provider string, resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		after := time.Minute
		if s := resp.Header.Get("Retry-After"); s != "" {
			if d, err := time.ParseDuration(strings.TrimSpace(s) + "s"); err == nil && d > 0 {
				after = min(d, time.Hour)
			}
		}
		return &rateLimitedError{after: after}
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return &permanentError{fmt.Sprintf("%s: no access (%d): %s", provider, resp.StatusCode, string(b))}
	case resp.StatusCode >= 400:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s api status %d: %s", provider, resp.StatusCode, string(b))
	}
	return nil
}
