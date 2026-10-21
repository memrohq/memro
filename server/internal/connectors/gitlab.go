package connectors

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/metrics"
)

type gitlabProvider struct {
	m *Manager
}

func newGitLabProvider(m *Manager) *gitlabProvider {
	return &gitlabProvider{m: m}
}

func (p *gitlabProvider) enabled() bool {
	return p.m.cfg.GitLabEnabled()
}

func (p *gitlabProvider) info() ProviderInfo {
	return ProviderInfo{
		Name:             ProviderGitLab,
		Enabled:          p.enabled(),
		SourcesManagedBy: "memro", // user selects projects in dashboard
	}
}

func (p *gitlabProvider) authorizeURL(state string) string {
	base := p.m.cfg.GitLabOAuthURL
	if base == "" {
		base = "https://gitlab.com/oauth/authorize"
	}
	q := url.Values{
		"client_id":     {p.m.cfg.GitLabClientID},
		"redirect_uri":  {p.m.callbackURL(ProviderGitLab)},
		"response_type": {"code"},
		"state":         {state},
		"scope":         {"read_api read_user openid"},
	}
	return base + "?" + q.Encode()
}

type gitlabTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (p *gitlabProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"GitLab did not return an authorization code."}
	}

	tokenURL := "https://gitlab.com/oauth/token"
	form := url.Values{
		"client_id":     {p.m.cfg.GitLabClientID},
		"client_secret": {p.m.cfg.GitLabClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {p.m.callbackURL(ProviderGitLab)},
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

	var tResp gitlabTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return nil, err
	}
	if tResp.Error != "" || tResp.AccessToken == "" {
		return nil, &UserError{"GitLab sign-in failed: " + tResp.ErrorDesc}
	}

	// Fetch current user
	apiURL := p.m.cfg.GitLabAPIURL
	if apiURL == "" {
		apiURL = "https://gitlab.com/api/v4"
	}
	uReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/user", nil)
	if err != nil {
		return nil, err
	}
	uReq.Header.Set("Authorization", "Bearer "+tResp.AccessToken)

	uResp, err := p.m.http.Do(uReq)
	if err != nil {
		return nil, err
	}
	defer uResp.Body.Close()

	var user gitlabUser
	if err := json.NewDecoder(uResp.Body).Decode(&user); err != nil {
		return nil, err
	}

	userIDStr := strconv.Itoa(user.ID)
	return &authorized{
		accountID:   userIDStr,
		accountName: user.Username,
		userID:      userIDStr,
		userName:    user.Name,
		creds: map[string]any{
			"access_token":  tResp.AccessToken,
			"refresh_token": tResp.RefreshToken,
			"user_id":       user.ID,
			"username":      user.Username,
		},
	}, nil
}

func (p *gitlabProvider) apiGet(ctx context.Context, c *conn, path string, out any) error {
	token := c.str("access_token")
	base := p.m.cfg.GitLabAPIURL
	if base == "" {
		base = "https://gitlab.com/api/v4"
	}
	fullURL := base + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &revokedError{"GitLab authorization revoked or expired."}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return &rateLimitedError{after: time.Minute}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab api status %d: %s", resp.StatusCode, string(b))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *gitlabProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.syncProject(ctx, c, j.Key)
	case jobItem:
		return p.syncMR(ctx, c, j.Key)
	default:
		return &permanentError{"unknown gitlab job " + j.Kind}
	}
}

func (p *gitlabProvider) refreshSources(ctx context.Context, c *conn) error {
	var projects []struct {
		ID                int    `json:"id"`
		NameWithNamespace string `json:"name_with_namespace"`
		PathWithNamespace string `json:"path_with_namespace"`
		Visibility        string `json:"visibility"`
	}

	path := "/projects?membership=true&min_access_level=20&per_page=100&simple=true&order_by=last_activity_at"
	if err := p.apiGet(ctx, c, path, &projects); err != nil {
		return fmt.Errorf("gitlab list projects: %w", err)
	}

	sources := make([]Source, 0, len(projects))
	for _, prj := range projects {
		sources = append(sources, Source{
			ID:      strconv.Itoa(prj.ID),
			Name:    prj.PathWithNamespace,
			Private: prj.Visibility != "public",
		})
	}
	return p.m.replaceSources(ctx, c.ID, sources, false)
}

func (p *gitlabProvider) syncProject(ctx context.Context, c *conn, projectIDStr string) error {
	var mrs []gitlabMergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests?order_by=updated_at&per_page=50", projectIDStr)
	if err := p.apiGet(ctx, c, path, &mrs); err != nil {
		return fmt.Errorf("gitlab list mrs: %w", err)
	}

	for _, mr := range mrs {
		if err := p.ingestMR(ctx, c, projectIDStr, &mr); err != nil {
			return err
		}
	}

	// Also check for recent failed pipelines
	var pipelines []gitlabPipeline
	pipePath := fmt.Sprintf("/projects/%s/pipelines?status=failed&per_page=10", projectIDStr)
	if err := p.apiGet(ctx, c, pipePath, &pipelines); err == nil {
		for _, pipe := range pipelines {
			_ = p.ingestPipelineFailure(ctx, c, projectIDStr, &pipe)
		}
	}

	return p.m.saveCursor(ctx, c.ID, projectIDStr, "", true)
}

func (p *gitlabProvider) syncMR(ctx context.Context, c *conn, key string) error {
	// key is "projectId:mrIid"
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid gitlab mr key %q", key)
	}
	projectID, mrIID := parts[0], parts[1]

	path := fmt.Sprintf("/projects/%s/merge_requests/%s", projectID, mrIID)
	var mr gitlabMergeRequest
	if err := p.apiGet(ctx, c, path, &mr); err != nil {
		return fmt.Errorf("gitlab get mr %s: %w", key, err)
	}

	return p.ingestMR(ctx, c, projectID, &mr)
}

func (p *gitlabProvider) ingestMR(ctx context.Context, c *conn, projectID string, mr *gitlabMergeRequest) error {
	// Fetch discussions/notes for context
	var notes []gitlabNote
	notesPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes?per_page=30&order_by=created_at&sort=asc", projectID, mr.IID)
	_ = p.apiGet(ctx, c, notesPath, &notes)

	sourceName := projectID
	if src, err := p.m.source(ctx, c.ID, projectID); err == nil && src.Name != "" {
		sourceName = src.Name
	}

	header, body, meta := renderGitLabMR(mr, notes, sourceName)
	evDate := parseGitLabTime(mr.UpdatedAt)
	if evDate.IsZero() {
		evDate = parseGitLabTime(mr.CreatedAt)
	}

	it := item{
		Key:       fmt.Sprintf("mr:%s:%d", projectID, mr.IID),
		SourceID:  projectID,
		Header:    header,
		Body:      body,
		Type:      domain.Episodic,
		EventDate: &evDate,
		Meta:      meta,
	}
	return p.m.storeItem(ctx, c, it)
}

func (p *gitlabProvider) ingestPipelineFailure(ctx context.Context, c *conn, projectID string, pipe *gitlabPipeline) error {
	sourceName := projectID
	if src, err := p.m.source(ctx, c.ID, projectID); err == nil && src.Name != "" {
		sourceName = src.Name
	}

	header, body, meta := renderGitLabPipelineFailure(pipe, nil, sourceName)
	evDate := parseGitLabTime(pipe.UpdatedAt)
	if evDate.IsZero() {
		evDate = parseGitLabTime(pipe.CreatedAt)
	}

	it := item{
		Key:       fmt.Sprintf("pipeline:%s:%d", projectID, pipe.ID),
		SourceID:  projectID,
		Header:    header,
		Body:      body,
		Type:      domain.Episodic,
		EventDate: &evDate,
		Meta:      meta,
	}
	return p.m.storeItem(ctx, c, it)
}

func (p *gitlabProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
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

func (p *gitlabProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	return nil
}

func (p *gitlabProvider) manageURL(c *Connection) string {
	return ""
}

// VerifyGitLabWebhookToken checks the X-Gitlab-Token header value.
func VerifyGitLabWebhookToken(expectedToken, receivedToken string) bool {
	if expectedToken == "" || receivedToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expectedToken), []byte(receivedToken)) == 1
}

type gitlabWebhookPayload struct {
	ObjectKind string `json:"object_kind"` // merge_request, pipeline, issue
	Project    struct {
		ID int `json:"id"`
	} `json:"project"`
	ObjectAttributes struct {
		IID int `json:"iid"`
		ID  int `json:"id"`
	} `json:"object_attributes"`
}

// GitLabWebhook processes incoming GitLab system webhooks.
func (m *Manager) GitLabWebhook(ctx context.Context, hdr http.Header, body []byte) int {
	if m.cfg.GitLabWebhookSecret != "" {
		token := hdr.Get("X-Gitlab-Token")
		if !VerifyGitLabWebhookToken(m.cfg.GitLabWebhookSecret, token) {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderGitLab, "bad_token").Inc()
			return http.StatusUnauthorized
		}
	}

	var p gitlabWebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return http.StatusBadRequest
	}

	metrics.ConnectorWebhooks.WithLabelValues(ProviderGitLab, p.ObjectKind).Inc()

	projectIDStr := strconv.Itoa(p.Project.ID)
	conns, err := m.connectionsForAccount(ctx, ProviderGitLab, "", false)
	if err != nil || len(conns) == 0 {
		return http.StatusOK
	}

	for _, c := range conns {
		if p.ObjectKind == "merge_request" && p.ObjectAttributes.IID > 0 {
			key := fmt.Sprintf("%s:%d", projectIDStr, p.ObjectAttributes.IID)
			_ = m.enqueue(ctx, nil, c.ID, jobItem, key, nil, 5*time.Second)
		} else if p.ObjectKind == "pipeline" {
			_ = m.enqueue(ctx, nil, c.ID, jobSync, projectIDStr, nil, 5*time.Second)
		}
	}

	return http.StatusOK
}
