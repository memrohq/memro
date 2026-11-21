package connectors

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

	if resp.StatusCode == http.StatusUnauthorized {
		return &revokedError{"Notion authorization revoked or expired."}
	}
	if err := apiStatus("notion", resp); err != nil {
		return err
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
		if id, ok := strings.CutPrefix(j.Key, "tree:"); ok {
			return p.syncPage(ctx, c, id, true)
		}
		return p.syncPage(ctx, c, j.Key, false)
	default:
		return &permanentError{"unknown notion job " + j.Kind}
	}
}

type notionSearchResult struct {
	Object string                    `json:"object"`
	ID     string                    `json:"id"`
	Parent notionParent              `json:"parent"`
	Title  []notionRichText          `json:"title,omitempty"`
	Props  map[string]map[string]any `json:"properties,omitempty"`
}

type notionSearchResults struct {
	Results    []notionSearchResult `json:"results"`
	HasMore    bool                 `json:"has_more"`
	NextCursor string               `json:"next_cursor"`
}

// notionSearchPages bounds one source refresh: 100 results per request.
const notionSearchPages = 20

// refreshSources lists the databases shared with Memro, and the pages at the
// top of each shared tree (a page whose parent Memro can also read is part
// of that parent's tree, and database rows belong to their database).
func (p *notionProvider) refreshSources(ctx context.Context, c *conn) error {
	token := c.str("access_token")
	search := func(object string) ([]notionSearchResult, error) {
		var out []notionSearchResult
		cursor := ""
		for range notionSearchPages {
			body := map[string]any{"filter": map[string]string{"value": object, "property": "object"}, "page_size": 100}
			if cursor != "" {
				body["start_cursor"] = cursor
			}
			var res notionSearchResults
			if err := p.request(ctx, token, http.MethodPost, "/search", body, &res); err != nil {
				return nil, err
			}
			out = append(out, res.Results...)
			if !res.HasMore || res.NextCursor == "" {
				break
			}
			cursor = res.NextCursor
		}
		return out, nil
	}

	dbs, err := search("database")
	if err != nil {
		return err
	}
	var list []Source
	for _, res := range dbs {
		title := formatRichText(res.Title)
		if title == "" {
			title = "Untitled Database"
		}
		list = append(list, Source{ID: res.ID, Name: title})
	}

	pages, err := search("page")
	if err != nil {
		return err
	}
	readable := map[string]bool{}
	for _, res := range pages {
		readable[res.ID] = true
	}
	for _, res := range pages {
		if res.Parent.DatabaseID != "" || res.Parent.BlockID != "" || readable[res.Parent.PageID] {
			continue
		}
		title := extractPageTitle(res.Props)
		if title == "" || title == "Untitled Page" {
			continue
		}
		list = append(list, Source{ID: res.ID, Name: "Page: " + title})
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

	src, err := p.m.source(ctx, c.ID, sourceID)
	if err != nil {
		return err
	}
	if src == nil || !src.Enabled {
		return nil
	}
	var st syncState
	if src.cursor != "" {
		_ = json.Unmarshal([]byte(src.cursor), &st) // a pre-checkpoint cursor restarts the pass
	}

	// Only rows edited since the last finished pass, oldest edit first.
	qBody := map[string]any{"page_size": 50,
		"sorts": []map[string]string{{"timestamp": "last_edited_time", "direction": "ascending"}}}
	if st.Checkpoint != "" {
		qBody["filter"] = map[string]any{"timestamp": "last_edited_time",
			"last_edited_time": map[string]string{"on_or_after": st.Checkpoint}}
	} else if p.m.cfg.BackfillDays > 0 {
		qBody["filter"] = map[string]any{"timestamp": "last_edited_time",
			"last_edited_time": map[string]string{"on_or_after": p.m.now().AddDate(0, 0, -p.m.cfg.BackfillDays).UTC().Format(time.RFC3339)}}
	}
	if st.Cursor != "" {
		qBody["start_cursor"] = st.Cursor
	}

	var qRes notionQueryResults
	err = p.request(ctx, token, http.MethodPost, fmt.Sprintf("/databases/%s/query", sourceID), qBody, &qRes)
	if err == nil {
		for _, pg := range qRes.Results {
			if pg.Archived {
				_ = p.m.removeItem(ctx, c, "page:"+pg.ID)
				continue
			}
			blocks, err := p.fetchBlocks(ctx, token, pg.ID)
			if err != nil {
				return err // keep the stored page rather than save it empty
			}
			it := notionItem(wsName, &pg, blocks, sourceID)
			if err := p.m.storeItem(ctx, c, it); err != nil {
				return err
			}
			st.High = laterOf(st.High, pg.LastEditedTime.UTC().Format(time.RFC3339))
		}

		if qRes.HasMore && qRes.NextCursor != "" {
			st.Cursor = qRes.NextCursor
			b, _ := json.Marshal(st)
			if err := p.m.saveCursor(ctx, c.ID, sourceID, string(b), false); err != nil {
				return err
			}
			return p.m.enqueue(ctx, nil, c.ID, jobSync, sourceID, nil, 0)
		}

		if err := p.sweepDatabase(ctx, c, token, sourceID); err != nil {
			return err
		}
		b, _ := json.Marshal(syncState{Checkpoint: laterOf(st.Checkpoint, st.High)})
		return p.m.saveCursor(ctx, c.ID, sourceID, string(b), true)
	}

	// Not a database: a page. Sync it and its sub-pages, and re-check every
	// page stored under it, so ones moved away or no longer shared are removed.
	keys, err := p.m.sourceItemKeys(ctx, c.ID, sourceID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if id, ok := strings.CutPrefix(k, "page:"); ok && id != sourceID {
			if err := p.m.enqueue(ctx, nil, c.ID, jobItem, "tree:"+id, nil, 0); err != nil {
				return err
			}
		}
	}
	if err := p.syncPage(ctx, c, sourceID, true); err != nil {
		return err
	}
	return p.m.saveCursor(ctx, c.ID, sourceID, "", true)
}

// notionSweepPages bounds the row listing that finds deleted rows.
const notionSweepPages = 100

// sweepDatabase removes stored rows that are no longer in the database
// (deleted, trashed, or no longer shared). A database too large to list in
// one pass is not swept.
func (p *notionProvider) sweepDatabase(ctx context.Context, c *conn, token, dbID string) error {
	live := map[string]bool{}
	cursor := ""
	for i := 0; ; i++ {
		if i == notionSweepPages {
			slog.Warn("notion database too large to sweep", "database", dbID)
			return nil
		}
		body := map[string]any{"page_size": 100}
		if cursor != "" {
			body["start_cursor"] = cursor
		}
		var res notionQueryResults
		if err := p.request(ctx, token, http.MethodPost, fmt.Sprintf("/databases/%s/query", dbID), body, &res); err != nil {
			return err
		}
		for _, pg := range res.Results {
			live["page:"+pg.ID] = true
		}
		if !res.HasMore || res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	keys, err := p.m.sourceItemKeys(ctx, c.ID, dbID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if !live[k] {
			if err := p.m.removeItem(ctx, c, k); err != nil {
				return err
			}
		}
	}
	return nil
}

// Bounds on one page's content: nesting deeper than this, or blocks past
// this many, are left out.
const (
	notionMaxDepth  = 3
	notionMaxBlocks = 1000
)

// fetchBlocks reads a page's blocks in order, every page of them, with the
// children of toggles, columns and nested lists placed after their parent.
// Child pages and databases are synced as pages of their own.
func (p *notionProvider) fetchBlocks(ctx context.Context, token, blockID string) ([]notionBlock, error) {
	var out []notionBlock
	return out, p.appendBlocks(ctx, token, blockID, 0, &out)
}

func (p *notionProvider) appendBlocks(ctx context.Context, token, blockID string, depth int, out *[]notionBlock) error {
	cursor := ""
	for {
		var res struct {
			Results    []notionBlock `json:"results"`
			HasMore    bool          `json:"has_more"`
			NextCursor string        `json:"next_cursor"`
		}
		path := fmt.Sprintf("/blocks/%s/children?page_size=100", blockID)
		if cursor != "" {
			path += "&start_cursor=" + url.QueryEscape(cursor)
		}
		if err := p.request(ctx, token, http.MethodGet, path, nil, &res); err != nil {
			return err
		}
		for _, b := range res.Results {
			if len(*out) >= notionMaxBlocks {
				return nil
			}
			*out = append(*out, b)
			if b.HasChild && depth+1 < notionMaxDepth && b.Type != "child_page" && b.Type != "child_database" {
				if err := p.appendBlocks(ctx, token, b.ID, depth+1, out); err != nil {
					return err
				}
			}
		}
		if !res.HasMore || res.NextCursor == "" {
			return nil
		}
		cursor = res.NextCursor
	}
}

// syncPage stores one page if it sits under a chosen source, and removes it
// if it is gone, trashed, no longer shared, or outside every chosen source.
// With walk set it also queues the page's sub-pages.
func (p *notionProvider) syncPage(ctx context.Context, c *conn, pageID string, walk bool) error {
	token := c.str("access_token")
	wsName := c.str("workspace_name")

	var pg notionPage
	err := p.request(ctx, token, http.MethodGet, fmt.Sprintf("/pages/%s", pageID), nil, &pg)
	var pe *permanentError
	if errors.As(err, &pe) {
		return p.m.removeItem(ctx, c, "page:"+pageID) // deleted or no longer shared
	}
	if err != nil {
		return err
	}
	if pg.Archived || pg.InTrash {
		return p.m.removeItem(ctx, c, "page:"+pg.ID)
	}
	sourceID, err := p.chosenRoot(ctx, c, token, &pg)
	if err != nil {
		return err
	}
	if sourceID == "" {
		return p.m.removeItem(ctx, c, "page:"+pg.ID)
	}

	// Unchanged since it was stored: its text and its sub-pages are the same.
	meta, err := p.m.storedMeta(ctx, c.ID, "page:"+pg.ID)
	if err != nil {
		return err
	}
	if meta != nil && meta["last_edited"] == pg.LastEditedTime.UTC().Format(time.RFC3339) {
		return nil
	}

	blocks, err := p.fetchBlocks(ctx, token, pg.ID)
	if err != nil {
		return err // keep the stored page rather than save it empty
	}
	if err := p.m.storeItem(ctx, c, notionItem(wsName, &pg, blocks, sourceID)); err != nil {
		return err
	}
	if walk {
		for _, b := range blocks {
			if b.Type == "child_page" {
				if err := p.m.enqueue(ctx, nil, c.ID, jobItem, "tree:"+b.ID, nil, 0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// notionMaxAncestors bounds the walk up from a page to a chosen source.
const notionMaxAncestors = 8

// chosenRoot returns the chosen source a page syncs under: the page itself,
// its database, or the nearest chosen page above it. "" if there is none.
func (p *notionProvider) chosenRoot(ctx context.Context, c *conn, token string, pg *notionPage) (string, error) {
	isChosen := func(id string) (bool, error) {
		src, err := p.m.source(ctx, c.ID, id)
		return src != nil && src.Enabled, err
	}
	if ok, err := isChosen(pg.ID); ok || err != nil {
		return pg.ID, err
	}
	parent := pg.Parent
	for range notionMaxAncestors {
		id := parent.DatabaseID + parent.PageID + parent.BlockID // at most one is set
		if id == "" {
			return "", nil // reached the workspace
		}
		if ok, err := isChosen(id); ok || err != nil {
			return id, err
		}
		var up struct {
			Parent notionParent `json:"parent"`
		}
		path := "/blocks/" + id
		if parent.DatabaseID != "" {
			path = "/databases/" + id
		}
		err := p.request(ctx, token, http.MethodGet, path, nil, &up)
		var pe *permanentError
		if errors.As(err, &pe) {
			return "", nil // above here Memro cannot read
		}
		if err != nil {
			return "", err
		}
		parent = up.Parent
	}
	return "", nil
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

// VerifyNotionSignature checks X-Notion-Signature ("sha256=<hex>"), an
// HMAC-SHA256 of the body keyed with the subscription's verification token.
func VerifyNotionSignature(secret, sig string, body []byte) bool {
	sig = strings.TrimPrefix(strings.TrimSpace(sig), "sha256=")
	if secret == "" || sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(expected)) == 1
}

// notionWebhookPayload is a Notion integration webhook event
// (developers.notion.com/reference/webhooks).
type notionWebhookPayload struct {
	Type        string `json:"type"` // page.created, page.content_updated, page.deleted, ...
	WorkspaceID string `json:"workspace_id"`
	Entity      struct {
		ID   string `json:"id"`
		Type string `json:"type"` // page, database, comment
	} `json:"entity"`
	Data struct {
		PageID string `json:"page_id"` // comment events name their page here
	} `json:"data"`
	VerificationToken string `json:"verification_token"`
}

// NotionWebhook handles Notion integration webhooks. Setting up a
// subscription sends a one-time verification token; until NOTION_WEBHOOK_SECRET
// is set to it, that request is logged for the operator and every other
// request is refused. After that, only signed events are accepted, and only
// connections to that workspace that chose the page act on them.
func (m *Manager) NotionWebhook(ctx context.Context, hdr http.Header, body []byte) int {
	var p notionWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return http.StatusBadRequest
	}
	if m.cfg.NotionWebhookSecret == "" {
		if p.VerificationToken != "" && p.Type == "" {
			slog.Warn("notion webhook verification request: set NOTION_WEBHOOK_SECRET to this token and redeploy, then verify the subscription in Notion",
				"verification_token", p.VerificationToken)
			return http.StatusOK
		}
		return http.StatusServiceUnavailable
	}
	if !VerifyNotionSignature(m.cfg.NotionWebhookSecret, hdr.Get("X-Notion-Signature"), body) {
		metrics.ConnectorWebhooks.WithLabelValues(ProviderNotion, "bad_signature").Inc()
		return http.StatusUnauthorized
	}
	metrics.ConnectorWebhooks.WithLabelValues(ProviderNotion, p.Type).Inc()

	pageID := ""
	switch p.Entity.Type {
	case "page":
		pageID = p.Entity.ID
	case "comment":
		pageID = p.Data.PageID
	}
	if pageID == "" || p.WorkspaceID == "" {
		return http.StatusOK // database and other events: the periodic sync covers them
	}
	conns, err := m.connectionsForAccount(ctx, ProviderNotion, p.WorkspaceID, true)
	if err != nil {
		slog.Error("notion webhook", "err", err)
		return http.StatusOK
	}
	for _, c := range conns {
		if p.Type == "page.deleted" {
			if cc, err := m.open(ctx, c); err == nil {
				_ = m.removeItem(ctx, cc, "page:"+pageID)
			}
			continue
		}
		_ = m.enqueue(ctx, nil, c.ID, jobItem, pageID, nil, 5*time.Second)
	}
	return http.StatusOK
}
