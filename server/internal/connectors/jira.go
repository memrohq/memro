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
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/metrics"
)

type jiraProvider struct {
	m *Manager
}

func newJiraProvider(m *Manager) *jiraProvider {
	return &jiraProvider{m: m}
}

func (p *jiraProvider) enabled() bool {
	return p.m.cfg.JiraEnabled()
}

func (p *jiraProvider) info() ProviderInfo {
	return ProviderInfo{
		Name:             ProviderJira,
		Enabled:          p.enabled(),
		SourcesManagedBy: "memro", // user selects projects in dashboard
	}
}

func (p *jiraProvider) authorizeURL(state string) string {
	base := p.m.cfg.JiraOAuthURL
	if base == "" {
		base = "https://auth.atlassian.com/authorize"
	}
	q := url.Values{
		"audience":      {"api.atlassian.com"},
		"client_id":     {p.m.cfg.JiraClientID},
		"scope":         {"read:jira-work read:jira-user read:me offline_access"},
		"redirect_uri":  {p.m.callbackURL(ProviderJira)},
		"state":         {state},
		"response_type": {"code"},
		"prompt":        {"consent"},
	}
	return base + "?" + q.Encode()
}

type jiraTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type jiraAccessibleResource struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	AvatarURL string   `json:"avatarUrl"`
}

func (p *jiraProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"Jira did not return an authorization code."}
	}

	tokenURL := "https://auth.atlassian.com/oauth/token"
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     p.m.cfg.JiraClientID,
		"client_secret": p.m.cfg.JiraClientSecret,
		"code":          code,
		"redirect_uri":  p.m.callbackURL(ProviderJira),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tResp jiraTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, err
	}
	if tResp.Error != "" || tResp.AccessToken == "" {
		return nil, &UserError{"Jira sign-in failed: " + tResp.ErrorDesc}
	}

	// Fetch accessible resources (Atlassian Cloud sites)
	resReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return nil, err
	}
	resReq.Header.Set("Authorization", "Bearer "+tResp.AccessToken)
	resReq.Header.Set("Accept", "application/json")

	resResp, err := p.m.http.Do(resReq)
	if err != nil {
		return nil, err
	}
	defer resResp.Body.Close()

	var resources []jiraAccessibleResource
	if err := json.NewDecoder(resResp.Body).Decode(&resources); err != nil {
		return nil, err
	}
	if len(resources) == 0 {
		return nil, &UserError{"No accessible Jira sites found for this Atlassian account."}
	}
	site := resources[0] // Primary site

	// Query current user profile
	userURL := fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/myself", site.ID)
	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tResp.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := p.m.http.Do(userReq)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()

	var user struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
		Email       string `json:"emailAddress"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &authorized{
		accountID:   site.ID,
		accountName: site.Name,
		userID:      user.AccountID,
		userName:    user.DisplayName,
		creds: map[string]any{
			"access_token":  tResp.AccessToken,
			"refresh_token": tResp.RefreshToken,
			"expires_at":    tokenExpiry(p.m.now(), tResp.ExpiresIn),
			"cloud_id":      site.ID,
			"site_name":     site.Name,
			"site_url":      site.URL,
		},
	}, nil
}

func (p *jiraProvider) tokenEndpoint() tokenEndpoint {
	return tokenEndpoint{URL: "https://auth.atlassian.com/oauth/token", ClientID: p.m.cfg.JiraClientID,
		ClientSecret: p.m.cfg.JiraClientSecret, JSON: true}
}

func (p *jiraProvider) apiGet(ctx context.Context, c *conn, path string, out any) error {
	base := p.m.cfg.JiraAPIURL
	if base == "" {
		base = "https://api.atlassian.com"
	}
	apiURL := fmt.Sprintf("%s/ex/jira/%s%s", base, url.PathEscape(c.str("cloud_id")), path)
	resp, err := p.m.oauthDo(ctx, c, p.tokenEndpoint(), func(token string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := apiStatus("jira", resp); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *jiraProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.syncProject(ctx, c, j.Key)
	case jobItem:
		return p.syncIssue(ctx, c, j.Key)
	default:
		return &permanentError{"unknown jira job " + j.Kind}
	}
}

func (p *jiraProvider) refreshSources(ctx context.Context, c *conn) error {
	var res struct {
		Values []struct {
			ID   string `json:"id"`
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"values"`
	}

	if err := p.apiGet(ctx, c, "/rest/api/3/project/search?maxResults=100", &res); err != nil {
		return fmt.Errorf("jira list projects: %w", err)
	}

	sources := make([]Source, 0, len(res.Values))
	for _, prj := range res.Values {
		sources = append(sources, Source{
			ID:      prj.Key,
			Name:    fmt.Sprintf("%s (%s)", prj.Name, prj.Key),
			Private: false,
		})
	}
	return p.m.replaceSources(ctx, c.ID, sources, false)
}

// syncProject reads one page of a project's issues, oldest change first,
// from the last checkpoint (less a day: JQL dates are in the user's time
// zone), and queues the next page until the pass is done.
func (p *jiraProvider) syncProject(ctx context.Context, c *conn, projectKey string) error {
	src, err := p.m.source(ctx, c.ID, projectKey)
	if err != nil {
		return err
	}
	if src == nil || !src.Enabled {
		return nil
	}
	var st syncState
	if src.cursor != "" {
		_ = json.Unmarshal([]byte(src.cursor), &st)
	}
	jql := fmt.Sprintf(`project = "%s"`, strings.ReplaceAll(projectKey, `"`, ``))
	if t := parseJiraTime(st.Checkpoint); !t.IsZero() {
		jql += fmt.Sprintf(` AND updated >= "%s"`, t.Add(-24*time.Hour).Format("2006-01-02 15:04"))
	} else if p.m.cfg.BackfillDays > 0 {
		jql += fmt.Sprintf(` AND updated >= -%dd`, p.m.cfg.BackfillDays)
	}
	jql += " ORDER BY updated ASC"
	q := url.Values{"jql": {jql}, "maxResults": {"100"}, "fields": {"*all"}}
	if st.Cursor != "" {
		q.Set("nextPageToken", st.Cursor)
	}
	var res struct {
		Issues        []jiraIssue `json:"issues"`
		NextPageToken string      `json:"nextPageToken"`
		IsLast        bool        `json:"isLast"`
	}
	if err := p.apiGet(ctx, c, "/rest/api/3/search/jql?"+q.Encode(), &res); err != nil {
		return fmt.Errorf("jira search issues: %w", err)
	}
	for i := range res.Issues {
		if err := p.ingestIssue(ctx, c, &res.Issues[i]); err != nil {
			return err
		}
		st.High = laterOf(st.High, res.Issues[i].Fields.Updated)
	}
	if res.NextPageToken != "" && !res.IsLast && len(res.Issues) > 0 {
		st.Cursor = res.NextPageToken
		b, _ := json.Marshal(st)
		if err := p.m.saveCursor(ctx, c.ID, projectKey, string(b), false); err != nil {
			return err
		}
		return p.m.enqueue(ctx, nil, c.ID, jobSync, projectKey, nil, 0)
	}
	b, _ := json.Marshal(syncState{Checkpoint: laterOf(st.Checkpoint, st.High)})
	return p.m.saveCursor(ctx, c.ID, projectKey, string(b), true)
}

func (p *jiraProvider) syncIssue(ctx context.Context, c *conn, issueKey string) error {
	path := fmt.Sprintf("/rest/api/3/issue/%s?expand=renderedFields,changelog", issueKey)
	var issue jiraIssue
	if err := p.apiGet(ctx, c, path, &issue); err != nil {
		return fmt.Errorf("jira get issue %s: %w", issueKey, err)
	}
	// Only projects the user chose: a webhook names any issue on the site.
	if src, err := p.m.source(ctx, c.ID, issue.Fields.Project.Key); err != nil || src == nil || !src.Enabled {
		return err
	}
	return p.ingestIssue(ctx, c, &issue)
}

func (p *jiraProvider) ingestIssue(ctx context.Context, c *conn, issue *jiraIssue) error {
	header, body, meta := renderJiraIssue(issue, nil, c.str("site_url"))
	evDate := parseJiraTime(issue.Fields.Updated)
	if evDate.IsZero() {
		evDate = parseJiraTime(issue.Fields.Created)
	}

	it := item{
		Key:       "issue:" + issue.Key,
		SourceID:  issue.Fields.Project.Key,
		Header:    header,
		Body:      body,
		Type:      domain.Episodic,
		EventDate: &evDate,
		Meta:      meta,
	}
	return p.m.storeItem(ctx, c, it)
}

func (p *jiraProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
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
		if !ok || s.Enabled == on {
			continue
		}
		if err := p.m.setSourceEnabled(ctx, c.ID, id, on); err != nil {
			return err
		}
		if on {
			_ = p.m.enqueue(ctx, nil, c.ID, jobSync, id, nil, 0)
		}
	}
	return nil
}

func (p *jiraProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	return nil
}

func (p *jiraProvider) manageURL(c *Connection) string {
	return ""
}

// JiraWebhookEvent models standard Jira cloud webhook payloads.
type JiraWebhookEvent struct {
	Timestamp          int64     `json:"timestamp"`
	WebhookEvent       string    `json:"webhookEvent"`
	IssueEventTypeName string    `json:"issue_event_type_name"`
	Issue              jiraIssue `json:"issue"`
}

// VerifyJiraWebhookSignature checks webhook authenticity.
func VerifyJiraWebhookSignature(secret, sig string, body []byte) bool {
	if secret == "" || sig == "" {
		return false
	}
	sig = strings.TrimPrefix(sig, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected))) == 1
}

// JiraWebhook processes Jira webhooks. They must carry an HMAC signature
// made with JIRA_WEBHOOK_SECRET; without the secret configured, webhooks are
// off and the periodic sync keeps data current. The issue is re-fetched with
// each matching connection's own token, so a webhook never decides content.
func (m *Manager) JiraWebhook(ctx context.Context, hdr http.Header, body []byte) int {
	if m.cfg.JiraWebhookSecret == "" {
		return http.StatusServiceUnavailable
	}
	sig := hdr.Get("X-Hub-Signature")
	if sig == "" {
		sig = hdr.Get("X-Atlassian-Webhook-Signature")
	}
	if !VerifyJiraWebhookSignature(m.cfg.JiraWebhookSecret, sig, body) {
		metrics.ConnectorWebhooks.WithLabelValues(ProviderJira, "bad_signature").Inc()
		return http.StatusUnauthorized
	}

	var ev JiraWebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return http.StatusBadRequest
	}
	metrics.ConnectorWebhooks.WithLabelValues(ProviderJira, ev.WebhookEvent).Inc()
	if ev.Issue.Key == "" || ev.Issue.Fields.Project.Key == "" {
		return http.StatusOK
	}
	conns, err := m.connectionsWithSource(ctx, ProviderJira, jiraCloudID(ev.Issue.Self), ev.Issue.Fields.Project.Key)
	if err != nil {
		slog.Error("jira webhook", "err", err)
		return http.StatusOK
	}
	for _, c := range conns {
		if ev.WebhookEvent == "jira:issue_deleted" {
			if cc, err := m.open(ctx, c); err == nil {
				_ = m.removeItem(ctx, cc, "issue:"+ev.Issue.Key)
			}
			continue
		}
		_ = m.enqueue(ctx, nil, c.ID, jobItem, ev.Issue.Key, nil, 5*time.Second)
	}
	return http.StatusOK
}

// jiraCloudID is the site ID in an issue's API URL
// (https://api.atlassian.com/ex/jira/<cloud id>/rest/...), or "" for a
// site-hosted URL.
func jiraCloudID(self string) string {
	_, rest, ok := strings.Cut(self, "/ex/jira/")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, "/")
	return id
}
