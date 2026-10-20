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
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/metrics"
)

type notionProvider struct {
	m *Manager
}

func newNotionProvider(m *Manager) *notionProvider {
	return &notionProvider{m: m}
}

func (p *notionProvider) enabled() bool {
	return p.m.cfg.NotionEnabled()
}

func (p *notionProvider) info() ProviderInfo {
	return ProviderInfo{
		Name:             ProviderNotion,
		Enabled:          p.enabled(),
		SourcesManagedBy: "memro", // user selects databases and pages in dashboard
	}
}

func (p *notionProvider) authorizeURL(state string) string {
	base := p.m.cfg.NotionOAuthURL
	if base == "" {
		base = "https://api.notion.com/v1/oauth/authorize"
	}
	q := url.Values{
		"client_id":     {p.m.cfg.NotionClientID},
		"response_type": {"code"},
		"owner":         {"user"},
		"redirect_uri":  {p.m.callbackURL(ProviderNotion)},
		"state":         {state},
	}
	return base + "?" + q.Encode()
}

type notionTokenResponse struct {
	AccessToken   string `json:"access_token"`
	TokenType     string `json:"token_type"`
	BotID         string `json:"bot_id"`
	WorkspaceName string `json:"workspace_name"`
	WorkspaceIcon string `json:"workspace_icon"`
	WorkspaceID   string `json:"workspace_id"`
	Owner         struct {
		Type string `json:"type"`
		User struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	} `json:"owner"`
	Error     string `json:"error"`
	ErrorDesc string `json:"error_description"`
}

func (p *notionProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"Notion did not return an authorization code."}
	}

	tokenURL := strings.TrimRight(p.m.cfg.NotionAPIURL, "/") + "/oauth/token"
	if p.m.cfg.NotionAPIURL == "" || p.m.cfg.NotionAPIURL == "https://api.notion.com/v1" {
		tokenURL = "https://api.notion.com/v1/oauth/token"
	}

	payload := map[string]string{
		"grant_type":   "authorization_code",
		"code":         code,
		"redirect_uri": p.m.callbackURL(ProviderNotion),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")
	req.SetBasicAuth(p.m.cfg.NotionClientID, p.m.cfg.NotionClientSecret)

	resp, err := p.m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tResp notionTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, err
	}
	if tResp.Error != "" || tResp.AccessToken == "" {
		msg := tResp.ErrorDesc
		if msg == "" {
			msg = tResp.Error
		}
		return nil, &UserError{"Notion sign-in failed: " + msg}
	}

	userName := tResp.Owner.User.Name
	if userName == "" {
		userName = tResp.Owner.User.Email
	}

	return &authorized{
		accountID:   tResp.WorkspaceID,
		accountName: tResp.WorkspaceName,
		userID:      tResp.Owner.User.ID,
		userName:    userName,
		creds: map[string]any{
			"access_token":   tResp.AccessToken,
			"workspace_id":   tResp.WorkspaceID,
			"workspace_name": tResp.WorkspaceName,
			"bot_id":         tResp.BotID,
		},
	}, nil
}

func (p *notionProvider) request(ctx context.Context, token, method, path string, body any, out any) error {
	base := p.m.cfg.NotionAPIURL
	if base == "" {
		base = "https://api.notion.com/v1"
	}
	u := strings.TrimRight(base, "/") + path

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", "2022-06-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &revokedError{"Notion authorization revoked or expired."}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return &rateLimitedError{after: time.Minute}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion api status %d: %s", resp.StatusCode, string(b))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (p *notionProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.syncSource(ctx, c, j.Key)
	case jobItem:
		return p.syncPage(ctx, c, j.Key)
	default:
		return &permanentError{"unknown notion job " + j.Kind}
	}
}

type notionSearchResults struct {
	Results []struct {
		Object string            `json:"object"`
		ID     string            `json:"id"`
		Title  []notionRichText  `json:"title,omitempty"`
		Props  map[string]map[string]any `json:"properties,omitempty"`
	} `json:"results"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}

func (p *notionProvider) refreshSources(ctx context.Context, c *conn) error {
	token := c.str("access_token")

	// 1. Search for databases
	var dbResults notionSearchResults
	reqBody := map[string]any{
		"filter":    map[string]string{"value": "database", "property": "object"},
		"page_size": 100,
	}
	if err := p.request(ctx, token, http.MethodPost, "/search", reqBody, &dbResults); err != nil {
		return err
	}

	var list []Source
	for _, res := range dbResults.Results {
		title := formatRichText(res.Title)
		if title == "" {
			title = "Untitled Database"
		}
		list = append(list, Source{
			ID:      res.ID,
			Name:    title,
			Private: false,
		})
	}

	// 2. Search for top-level pages
	var pageResults notionSearchResults
	pageReqBody := map[string]any{
		"filter":    map[string]string{"value": "page", "property": "object"},
		"page_size": 50,
	}
	if err := p.request(ctx, token, http.MethodPost, "/search", pageReqBody, &pageResults); err == nil {
		for _, res := range pageResults.Results {
			title := extractPageTitle(res.Props)
			if title == "" || title == "Untitled Page" {
				continue
			}
			list = append(list, Source{
				ID:      res.ID,
				Name:    "Page: " + title,
				Private: false,
			})
		}
	}

	return p.m.replaceSources(ctx, c.ID, list, false)
}

type notionQueryResults struct {
	Results    []notionPage `json:"results"`
	HasMore    bool         `json:"has_more"`
	NextCursor string       `json:"next_cursor"`
}

func (p *notionProvider) syncSource(ctx context.Context, c *conn, sourceID string) error {
	token := c.str("access_token")
	wsName := c.str("workspace_name")

	cursor := ""
	_ = p.m.deps.Pool.QueryRow(ctx, `SELECT cursor FROM connector_sources WHERE connection_id = $1 AND external_id = $2`, c.ID, sourceID).Scan(&cursor)

	// Attempt to query as database
	qBody := map[string]any{"page_size": 50}
	if cursor != "" {
		qBody["start_cursor"] = cursor
	}

	var qRes notionQueryResults
	err := p.request(ctx, token, http.MethodPost, fmt.Sprintf("/databases/%s/query", sourceID), qBody, &qRes)
	if err == nil {
		for _, pg := range qRes.Results {
			if pg.Archived {
				_ = p.m.removeItem(ctx, c, "page:"+pg.ID)
				continue
			}
			blocks, _ := p.fetchBlocks(ctx, token, pg.ID)
			it := notionItem(wsName, &pg, blocks)
			if err := p.m.storeItem(ctx, c, it); err != nil {
				return err
			}
		}

		if qRes.HasMore && qRes.NextCursor != "" {
			_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET cursor = $3, last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`,
				c.ID, sourceID, qRes.NextCursor)
			return p.m.enqueue(ctx, nil, c.ID, jobSync, sourceID, nil, 0)
		}

		_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET cursor = '', last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`,
			c.ID, sourceID)
		return nil
	}

	// If database query failed, try as individual page
	return p.syncPage(ctx, c, sourceID)
}

func (p *notionProvider) fetchBlocks(ctx context.Context, token, blockID string) ([]notionBlock, error) {
	var res struct {
		Results []notionBlock `json:"results"`
	}
	path := fmt.Sprintf("/blocks/%s/children?page_size=100", blockID)
	if err := p.request(ctx, token, http.MethodGet, path, nil, &res); err != nil {
		return nil, err
	}
	return res.Results, nil
}

func (p *notionProvider) syncPage(ctx context.Context, c *conn, pageID string) error {
	token := c.str("access_token")
	wsName := c.str("workspace_name")

	var pg notionPage
	if err := p.request(ctx, token, http.MethodGet, fmt.Sprintf("/pages/%s", pageID), nil, &pg); err != nil {
		return err
	}

	if pg.Archived {
		return p.m.removeItem(ctx, c, "page:"+pg.ID)
	}

	blocks, _ := p.fetchBlocks(ctx, token, pg.ID)
	it := notionItem(wsName, &pg, blocks)
	return p.m.storeItem(ctx, c, it)
}

func (p *notionProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
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
			return &UserError{"Unknown source " + id + ". Refresh the source list."}
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

func (p *notionProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	return nil
}

func (p *notionProvider) manageURL(c *Connection) string {
	return "https://www.notion.so/profile/integrations"
}

// VerifyNotionSignature validates Notion webhook HMAC-SHA256 signature if configured.
func VerifyNotionSignature(secret, sig string, body []byte) bool {
	if secret == "" || sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected))) == 1
}

type notionWebhookPayload struct {
	Type   string `json:"type"` // e.g. "page_updated", "page_created", "page_deleted"
	PageID string `json:"page_id"`
	Data   struct {
		ID string `json:"id"`
	} `json:"data"`
}

// NotionWebhook handles incoming webhooks from Notion integrations.
func (m *Manager) NotionWebhook(ctx context.Context, hdr http.Header, body []byte) int {
	if m.cfg.NotionWebhookSecret != "" {
		sig := hdr.Get("X-Notion-Signature")
		if !VerifyNotionSignature(m.cfg.NotionWebhookSecret, sig, body) {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderNotion, "bad_signature").Inc()
			return http.StatusUnauthorized
		}
	}

	var p notionWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return http.StatusBadRequest
	}

	pageID := p.PageID
	if pageID == "" {
		pageID = p.Data.ID
	}
	if pageID == "" {
		return http.StatusOK
	}

	metrics.ConnectorWebhooks.WithLabelValues(ProviderNotion, p.Type).Inc()

	conns, err := m.connectionsForAccount(ctx, ProviderNotion, "", true)
	if err != nil || len(conns) == 0 {
		return http.StatusOK
	}

	for _, c := range conns {
		if p.Type == "page_deleted" {
			if cc, err := m.open(ctx, c); err == nil {
				_ = m.removeItem(ctx, cc, "page:"+pageID)
			}
			continue
		}
		_ = m.enqueue(ctx, nil, c.ID, jobItem, pageID, nil, 5*time.Second)
	}

	return http.StatusOK
}
