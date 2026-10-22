package connectors

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/metrics"
)

type linearProvider struct {
	m *Manager
}

func newLinearProvider(m *Manager) *linearProvider {
	return &linearProvider{m: m}
}

func (p *linearProvider) enabled() bool {
	return p.m.cfg.LinearEnabled()
}

func (p *linearProvider) info() ProviderInfo {
	return ProviderInfo{
		Name:             ProviderLinear,
		Enabled:          p.enabled(),
		SourcesManagedBy: "memro", // user selects teams in dashboard
	}
}

func (p *linearProvider) authorizeURL(state string) string {
	base := p.m.cfg.LinearOAuthURL
	if base == "" {
		base = "https://linear.app/oauth/authorize"
	}
	q := url.Values{
		"client_id":     {p.m.cfg.LinearClientID},
		"redirect_uri":  {p.m.callbackURL(ProviderLinear)},
		"response_type": {"code"},
		"scope":         {"read,comments:create"},
		"state":         {state},
		"prompt":        {"consent"},
	}
	return base + "?" + q.Encode()
}

type linearTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (p *linearProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"Linear did not return an authorization code."}
	}

	tokenURL := "https://api.linear.app/oauth/token"
	form := url.Values{
		"code":          {code},
		"redirect_uri":  {p.m.callbackURL(ProviderLinear)},
		"client_id":     {p.m.cfg.LinearClientID},
		"client_secret": {p.m.cfg.LinearClientSecret},
		"grant_type":    {"authorization_code"},
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

	var tResp linearTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, err
	}
	if tResp.Error != "" || tResp.AccessToken == "" {
		return nil, &UserError{"Linear sign-in failed: " + tResp.ErrorDesc}
	}

	// Query viewer identity and organization
	var viewerQuery struct {
		Data struct {
			Viewer struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"viewer"`
			Organization struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				URLKey string `json:"urlKey"`
			} `json:"organization"`
		} `json:"data"`
	}
	qStr := `query { viewer { id name email } organization { id name urlKey } }`
	if err := p.gql(ctx, tResp.AccessToken, qStr, nil, &viewerQuery); err != nil {
		return nil, fmt.Errorf("linear query viewer: %w", err)
	}

	org := viewerQuery.Data.Organization
	viewer := viewerQuery.Data.Viewer

	return &authorized{
		accountID:   org.ID,
		accountName: org.Name,
		userID:      viewer.ID,
		userName:    viewer.Name,
		creds: map[string]any{
			"access_token":         tResp.AccessToken,
			"refresh_token":        tResp.RefreshToken,
			"expires_at":           tokenExpiry(p.m.now(), tResp.ExpiresIn),
			"organization_id":      org.ID,
			"organization_name":    org.Name,
			"organization_url_key": org.URLKey,
		},
	}, nil
}

// gql runs a query with a bare token (while connecting).
func (p *linearProvider) gql(ctx context.Context, token, query string, vars map[string]any, out any) error {
	req, err := p.gqlRequest(ctx, token, query, vars)
	if err != nil {
		return err
	}
	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	return p.gqlResult(resp, out)
}

// gqlConn runs a query with the connection's token, refreshing it as needed
// (Linear apps created after October 2025 get 24-hour tokens).
func (p *linearProvider) gqlConn(ctx context.Context, c *conn, query string, vars map[string]any, out any) error {
	ep := tokenEndpoint{URL: "https://api.linear.app/oauth/token", ClientID: p.m.cfg.LinearClientID,
		ClientSecret: p.m.cfg.LinearClientSecret}
	resp, err := p.m.oauthDo(ctx, c, ep, func(token string) (*http.Request, error) {
		return p.gqlRequest(ctx, token, query, vars)
	})
	if err != nil {
		return err
	}
	return p.gqlResult(resp, out)
}

func (p *linearProvider) gqlRequest(ctx context.Context, token, query string, vars map[string]any) (*http.Request, error) {
	apiURL := p.m.cfg.LinearAPIURL
	if apiURL == "" {
		apiURL = "https://api.linear.app/graphql"
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

func (p *linearProvider) gqlResult(resp *http.Response, out any) error {
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return &revokedError{"Linear authorization revoked or expired."}
	}
	if err := apiStatus("linear", resp); err != nil {
		return err
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *linearProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.syncTeam(ctx, c, j.Key)
	case jobItem:
		return p.syncIssue(ctx, c, j.Key)
	default:
		return &permanentError{"unknown linear job " + j.Kind}
	}
}

func (p *linearProvider) refreshSources(ctx context.Context, c *conn) error {
	var q struct {
		Data struct {
			Teams struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Key  string `json:"key"`
				} `json:"nodes"`
			} `json:"teams"`
		} `json:"data"`
	}
	query := `query { teams { nodes { id name key } } }`
	if err := p.gqlConn(ctx, c, query, nil, &q); err != nil {
		return err
	}

	var list []Source
	for _, t := range q.Data.Teams.Nodes {
		list = append(list, Source{
			ID:      t.ID,
			Name:    fmt.Sprintf("%s (%s)", t.Name, t.Key),
			Private: false,
		})
	}

	// Teams are opt-in (enableNew = false), dashboard user selects which ones to sync
	return p.m.replaceSources(ctx, c.ID, list, false)
}

func (p *linearProvider) syncTeam(ctx context.Context, c *conn, teamID string) error {

	// Read team cursor
	cursor := ""
	_ = p.m.deps.Pool.QueryRow(ctx, `SELECT cursor FROM connector_sources WHERE connection_id = $1 AND external_id = $2`, c.ID, teamID).Scan(&cursor)

	var q struct {
		Data struct {
			Team struct {
				Issues struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []linearIssue `json:"nodes"`
				} `json:"issues"`
			} `json:"team"`
		} `json:"data"`
	}

	vars := map[string]any{"teamId": teamID}
	if cursor != "" {
		vars["after"] = cursor
	}

	query := `query($teamId: String!, $after: String) {
		team(id: $teamId) {
			issues(first: 50, after: $after, orderBy: updatedAt) {
				pageInfo { hasNextPage endCursor }
				nodes {
					id identifier title description priority
					state { name type }
					assignee { id name email }
					creator { id name email }
					project { id name }
					team { id name key }
					createdAt updatedAt url
				}
			}
		}
	}`

	if err := p.gqlConn(ctx, c, query, vars, &q); err != nil {
		return err
	}

	for _, issue := range q.Data.Team.Issues.Nodes {
		it := linearItem(&issue, nil)
		if err := p.m.storeItem(ctx, c, it); err != nil {
			return err
		}
	}

	pageInfo := q.Data.Team.Issues.PageInfo
	if pageInfo.HasNextPage && pageInfo.EndCursor != "" {
		// Update cursor and queue next page
		if err := p.m.saveCursor(ctx, c.ID, teamID, pageInfo.EndCursor, false); err != nil {
			return err
		}
		return p.m.enqueue(ctx, nil, c.ID, jobSync, teamID, nil, 0)
	}

	// Done: clear cursor
	return p.m.saveCursor(ctx, c.ID, teamID, "", true)
}

func (p *linearProvider) syncIssue(ctx context.Context, c *conn, issueID string) error {
	var q struct {
		Data struct {
			Issue *linearIssue `json:"issue"`
		} `json:"data"`
	}

	query := `query($id: String!) {
		issue(id: $id) {
			id identifier title description priority
			state { name type }
			assignee { id name email }
			creator { id name email }
			project { id name }
			team { id name key }
			createdAt updatedAt url
			comments(first: 50) {
				nodes { id body user { id name email } createdAt }
			}
		}
	}`

	if err := p.gqlConn(ctx, c, query, map[string]any{"id": issueID}, &q); err != nil {
		return err
	}

	if q.Data.Issue == nil {
		// Issue deleted on Linear
		return p.m.removeItem(ctx, c, "issue:"+issueID)
	}
	// Only teams the user chose: a webhook names issues from every team in
	// the organization, private ones included. An issue moved out of a chosen
	// team is removed.
	if src, err := p.m.source(ctx, c.ID, q.Data.Issue.Team.ID); err != nil {
		return err
	} else if src == nil || !src.Enabled {
		return p.m.removeItem(ctx, c, "issue:"+issueID)
	}

	it := linearItem(q.Data.Issue, nil)
	return p.m.storeItem(ctx, c, it)
}

func (p *linearProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
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
			return &UserError{"Unknown team " + id + ". Refresh the team list."}
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

func (p *linearProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	return nil // best-effort
}

func (p *linearProvider) manageURL(c *Connection) string {
	urlKey := c.AccountName
	if urlKey != "" {
		return "https://linear.app/" + urlKey + "/settings"
	}
	return "https://linear.app/settings"
}

// VerifyLinearSignature validates the HMAC-SHA256 signature from Linear webhooks.
func VerifyLinearSignature(secret, sig string, body []byte) bool {
	if secret == "" || sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected))) == 1
}

type linearWebhookPayload struct {
	WebhookID        string `json:"webhookId"`
	WebhookTimestamp int64  `json:"webhookTimestamp"`
	Action           string `json:"action"` // create, update, remove
	Type             string `json:"type"`   // Issue, Comment, Project
	OrganizationID   string `json:"organizationId"`
	Data             struct {
		ID      string `json:"id"`
		TeamID  string `json:"teamId"`
		IssueID string `json:"issueId"` // present on comment events
	} `json:"data"`
}

// LinearWebhook processes incoming real-time notifications from Linear.
func (m *Manager) LinearWebhook(ctx context.Context, hdr http.Header, body []byte) int {
	if m.cfg.LinearWebhookSecret == "" {
		return http.StatusServiceUnavailable
	}

	sig := hdr.Get("Linear-Signature")
	if !VerifyLinearSignature(m.cfg.LinearWebhookSecret, sig, body) {
		metrics.ConnectorWebhooks.WithLabelValues(ProviderLinear, "bad_signature").Inc()
		return http.StatusUnauthorized
	}

	var p linearWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return http.StatusBadRequest
	}

	if p.WebhookTimestamp > 0 {
		ts := time.UnixMilli(p.WebhookTimestamp)
		if !VerifyWebhookTimestamp(ts, m.now()) {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderLinear, "stale_timestamp").Inc()
			return http.StatusUnauthorized
		}
	}

	if p.WebhookID != "" {
		claimed, err := m.ClaimWebhookDelivery(ctx, ProviderLinear, p.WebhookID)
		if err != nil {
			return http.StatusInternalServerError
		}
		if !claimed {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderLinear, "duplicate").Inc()
			return http.StatusOK
		}
	}

	metrics.ConnectorWebhooks.WithLabelValues(ProviderLinear, p.Type).Inc()

	if m.deps.Pool == nil {
		return http.StatusOK
	}

	// Find active connections for this Linear organization
	conns, err := m.connectionsForAccount(ctx, ProviderLinear, p.OrganizationID, true)
	if err != nil || len(conns) == 0 {
		return http.StatusOK // acknowledged
	}

	targetIssueID := p.Data.ID
	if p.Type == "Comment" && p.Data.IssueID != "" {
		targetIssueID = p.Data.IssueID
	}

	for _, c := range conns {
		if p.Action == "remove" && p.Type == "Issue" {
			if cc, err := m.open(ctx, c); err == nil {
				_ = m.removeItem(ctx, cc, "issue:"+targetIssueID)
			}
			continue
		}
		// Enqueue jobItem to re-fetch issue and comments with 5s delay to coalesce bursts
		_ = m.enqueue(ctx, nil, c.ID, jobItem, targetIssueID, nil, 5*time.Second)
	}

	return http.StatusOK
}
