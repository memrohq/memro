package connectors

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/memrohq/memro/server/internal/metrics"
)

const (
	githubAPIVersion  = "2022-11-28"
	githubPageSize    = 50
	githubItemDelay   = 10 * time.Second // coalesces a burst of webhooks for one issue
	githubMaxComments = 1000
)

type githubProvider struct {
	m   *Manager
	key *rsa.PrivateKey

	mu         sync.Mutex
	instTokens map[string]cachedToken
	refreshMu  sync.Mutex
}

var errUnauthorized = errors.New("unauthorized")

type cachedToken struct {
	token   string
	expires time.Time
}

func newGitHubProvider(m *Manager) (*githubProvider, error) {
	p := &githubProvider{m: m, instTokens: map[string]cachedToken{}}
	if pemKey := m.cfg.GitHubAppPrivateKey; pemKey != "" {
		key, err := parseRSAKey(pemKey)
		if err != nil {
			return nil, fmt.Errorf("GITHUB_APP_PRIVATE_KEY: %w", err)
		}
		p.key = key
	}
	return p, nil
}

func parseRSAKey(s string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("not a PEM private key")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("unsupported private key format")
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App keys are RSA keys")
	}
	return rk, nil
}

func (p *githubProvider) enabled() bool {
	c := p.m.cfg
	return p.m.sealer != nil && p.key != nil && c.GitHubAppID != "" && c.GitHubAppSlug != "" &&
		c.GitHubClientID != "" && c.GitHubClientSecret != "" && c.GitHubWebhookSecret != ""
}

func (p *githubProvider) info() ProviderInfo {
	return ProviderInfo{Name: ProviderGitHub, Enabled: p.enabled(), SourcesManagedBy: "provider"}
}

// authorizeURL installs the app (or edits an existing installation). The
// app must have "Request user authorization (OAuth) during installation"
// and "Redirect on update" enabled, so GitHub returns a code we can use to
// check who installed it.
func (p *githubProvider) authorizeURL(state string) string {
	return p.m.cfg.GitHubWebURL + "/apps/" + url.PathEscape(p.m.cfg.GitHubAppSlug) + "/installations/new?state=" + url.QueryEscape(state)
}

func (p *githubProvider) manageURL(c *Connection) string {
	if c.AccountID == "" {
		return ""
	}
	return p.m.cfg.GitHubWebURL + "/apps/" + url.PathEscape(p.m.cfg.GitHubAppSlug) + "/installations/" + url.PathEscape(c.AccountID)
}

// --- Tokens ---------------------------------------------------------------------

// appJWT authenticates as the GitHub App (RS256, valid 9 minutes).
func (p *githubProvider) appJWT() (string, error) {
	now := p.m.now()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": p.m.cfg.GitHubAppID,
	})
	signing := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

// installationToken returns a token for the installation, cached until
// five minutes before it expires (GitHub issues them for an hour).
func (p *githubProvider) installationToken(ctx context.Context, inst string) (string, error) {
	p.mu.Lock()
	if t, ok := p.instTokens[inst]; ok && t.expires.After(p.m.now().Add(5*time.Minute)) {
		p.mu.Unlock()
		return t.token, nil
	}
	p.mu.Unlock()
	jwt, err := p.appJWT()
	if err != nil {
		return "", err
	}
	var r struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	status, _, err := p.do(ctx, http.MethodPost, p.m.cfg.GitHubAPIURL+"/app/installations/"+url.PathEscape(inst)+"/access_tokens", jwt, nil, &r)
	switch {
	case status == http.StatusNotFound:
		return "", &revokedError{"The Memro GitHub App was uninstalled from this account."}
	case status == http.StatusForbidden && err == nil:
		return "", &revokedError{"The Memro GitHub App installation is suspended."}
	case err != nil:
		return "", err
	}
	p.mu.Lock()
	p.instTokens[inst] = cachedToken{r.Token, r.ExpiresAt}
	p.mu.Unlock()
	return r.Token, nil
}

func (p *githubProvider) forgetInstallation(inst string) {
	p.mu.Lock()
	delete(p.instTokens, inst)
	p.mu.Unlock()
}

type githubUserToken struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

func (p *githubProvider) exchange(ctx context.Context, params url.Values) (*githubUserToken, error) {
	params.Set("client_id", p.m.cfg.GitHubClientID)
	params.Set("client_secret", p.m.cfg.GitHubClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.m.cfg.GitHubWebURL+"/login/oauth/access_token", strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("github oauth: HTTP %d", resp.StatusCode)
	}
	var t githubUserToken
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t); err != nil {
		return nil, fmt.Errorf("github oauth: invalid response")
	}
	return &t, nil
}

func (p *githubProvider) setUserToken(c map[string]any, t *githubUserToken) {
	now := p.m.now().Unix()
	c["user_token"] = t.AccessToken
	c["user_refresh_token"] = t.RefreshToken
	c["user_expires_at"] = float64(0)
	if t.ExpiresIn > 0 {
		c["user_expires_at"] = float64(now + t.ExpiresIn)
	}
}

// userToken returns the token of the GitHub user who connected, refreshing
// it when it is about to expire. It is used to check which repositories
// that user may read.
func (p *githubProvider) userToken(ctx context.Context, c *conn) (string, error) {
	exp, _ := c.creds["user_expires_at"].(float64)
	if exp == 0 || time.Unix(int64(exp), 0).After(p.m.now().Add(5*time.Minute)) {
		return c.str("user_token"), nil
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	if fresh, err := p.m.connection(ctx, c.ID); err == nil {
		if fc, err := p.m.open(ctx, fresh); err == nil {
			c.creds = fc.creds
			if e, _ := c.creds["user_expires_at"].(float64); e == 0 || time.Unix(int64(e), 0).After(p.m.now().Add(5*time.Minute)) {
				return c.str("user_token"), nil
			}
		}
	}
	t, err := p.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.str("user_refresh_token")}})
	if err != nil {
		return "", err
	}
	if t.Error != "" || t.AccessToken == "" {
		return "", &revokedError{"Your GitHub authorization expired. Reconnect GitHub."}
	}
	p.setUserToken(c.creds, t)
	if err := c.saveCreds(ctx); err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

// --- HTTP -------------------------------------------------------------------------

// do calls the GitHub API. It returns the status for the caller to
// interpret 404/410, and turns rate limits and server errors into errors.
func (p *githubProvider) do(ctx context.Context, method, u, token string, body any, out any) (int, http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	req.Header.Set("User-Agent", "memro-connector")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.m.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && (resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0")) {
		after := retryAfter(resp.Header, time.Minute)
		if resp.Header.Get("Retry-After") == "" {
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				after = max(time.Until(time.Unix(reset, 0))+time.Second, time.Second)
			}
		}
		return resp.StatusCode, resp.Header, &rateLimitedError{after: after}
	}
	if resp.StatusCode >= 500 {
		return resp.StatusCode, resp.Header, fmt.Errorf("github %s %s: HTTP %d", method, redactURL(u), resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return resp.StatusCode, resp.Header, fmt.Errorf("github %s: %w", redactURL(u), errUnauthorized)
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header, nil
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(out); err != nil {
			return resp.StatusCode, resp.Header, fmt.Errorf("github %s: invalid response: %w", redactURL(u), err)
		}
	}
	return resp.StatusCode, resp.Header, nil
}

func redactURL(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i]
	}
	return u
}

// --- OAuth ------------------------------------------------------------------------

type githubInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

var digits = regexp.MustCompile(`^[0-9]{1,20}$`)

func (p *githubProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	if q.Get("setup_action") == "request" {
		return nil, &UserError{"An owner of that GitHub organization has to approve Memro. Connect again once they have."}
	}
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"GitHub did not return an authorization code. (Server setup: enable \"Request user authorization (OAuth) during installation\" for the GitHub App.)"}
	}
	inst := q.Get("installation_id")
	if !digits.MatchString(inst) {
		return nil, &UserError{"GitHub did not say which installation to connect. Install the app from the dashboard."}
	}
	t, err := p.exchange(ctx, url.Values{"code": {code}})
	if err != nil {
		slog.Warn("github oauth exchange", "err", err)
		return nil, &UserError{"GitHub sign-in failed. Please try again."}
	}
	if t.Error != "" || t.AccessToken == "" {
		return nil, &UserError{"GitHub sign-in failed: " + truncate(t.ErrorDescription, 120)}
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if status, _, err := p.do(ctx, http.MethodGet, p.m.cfg.GitHubAPIURL+"/user", t.AccessToken, nil, &user); err != nil || status != http.StatusOK {
		return nil, &UserError{"Couldn't read your GitHub account. Please try again."}
	}
	// The installation ID in the URL is only trusted if this user can see
	// that installation; anyone can put any ID in a link.
	found, err := p.findUserInstallation(ctx, t.AccessToken, inst)
	if err != nil {
		slog.Warn("github list user installations", "err", err)
		return nil, &UserError{"Couldn't verify the GitHub installation. Please try again."}
	}
	if found == nil {
		return nil, &UserError{"Your GitHub account doesn't have access to that installation."}
	}
	creds := map[string]any{"installation_id": inst, "account_type": found.Account.Type, "account_login": found.Account.Login}
	p.setUserToken(creds, t)
	return &authorized{accountID: inst, accountName: found.Account.Login, userID: strconv.FormatInt(user.ID, 10),
		userName: user.Login, creds: creds}, nil
}

func (p *githubProvider) findUserInstallation(ctx context.Context, token, inst string) (*githubInstallation, error) {
	for page := 1; page <= 20; page++ {
		var r struct {
			Installations []githubInstallation `json:"installations"`
		}
		u := fmt.Sprintf("%s/user/installations?per_page=100&page=%d", p.m.cfg.GitHubAPIURL, page)
		status, _, err := p.do(ctx, http.MethodGet, u, token, nil, &r)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("github: list installations: HTTP %d", status)
		}
		for i := range r.Installations {
			if strconv.FormatInt(r.Installations[i].ID, 10) == inst {
				return &r.Installations[i], nil
			}
		}
		if len(r.Installations) < 100 {
			return nil, nil
		}
	}
	return nil, nil
}

func (p *githubProvider) revoke(ctx context.Context, c *conn, _ bool) error {
	// Revoke only this connection's user token. The installation itself may
	// serve other people and is removed by its owner on GitHub.
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		p.m.cfg.GitHubAPIURL+"/applications/"+url.PathEscape(p.m.cfg.GitHubClientID)+"/token",
		strings.NewReader(mustJSON(map[string]string{"access_token": c.str("user_token")})))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.m.cfg.GitHubClientID, p.m.cfg.GitHubClientSecret)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusUnprocessableEntity {
		return fmt.Errorf("github revoke: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (p *githubProvider) setSources(context.Context, *conn, map[string]bool) error {
	return &UserError{"Choose repositories on GitHub (Manage on GitHub); Memro syncs every repository you share with it."}
}

// --- Jobs ---------------------------------------------------------------------------

func (p *githubProvider) handle(ctx context.Context, c *conn, j *job) error {
	var err error
	switch j.Kind {
	case jobSources:
		err = p.refreshSources(ctx, c)
	case jobSync:
		err = p.syncRepo(ctx, c, j.Key)
	case jobItem:
		err = p.syncItem(ctx, c, j.Key, j.Payload)
	default:
		return &permanentError{"unknown github job " + j.Kind}
	}
	if errors.Is(err, errUnauthorized) {
		p.forgetInstallation(c.AccountID) // the next attempt mints a new token
	}
	return err
}

type githubRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
}

// refreshSources syncs the repositories that are both shared with the
// installation and readable by the user who connected it.
func (p *githubProvider) refreshSources(ctx context.Context, c *conn) error {
	token, err := p.userToken(ctx, c)
	if err != nil {
		return err
	}
	var list []Source
	for page := 1; page <= 100; page++ {
		var r struct {
			TotalCount   int          `json:"total_count"`
			Repositories []githubRepo `json:"repositories"`
		}
		u := fmt.Sprintf("%s/user/installations/%s/repositories?per_page=100&page=%d", p.m.cfg.GitHubAPIURL, url.PathEscape(c.AccountID), page)
		status, _, err := p.do(ctx, http.MethodGet, u, token, nil, &r)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound || status == http.StatusForbidden {
			return &revokedError{"Your GitHub account no longer has access to this installation."}
		}
		if status != http.StatusOK {
			return fmt.Errorf("github: list repositories: HTTP %d", status)
		}
		for _, repo := range r.Repositories {
			list = append(list, Source{ID: strconv.FormatInt(repo.ID, 10), Name: repo.FullName, Private: repo.Private})
		}
		if len(r.Repositories) < 100 {
			break
		}
	}
	if err := p.m.replaceSources(ctx, c.ID, list, true); err != nil {
		return err
	}
	for _, s := range list {
		if err := p.m.enqueue(ctx, nil, c.ID, jobSync, s.ID, nil, 0); err != nil {
			return err
		}
	}
	return nil
}

type githubCursor struct {
	Since string `json:"since,omitempty"`
	Page  int    `json:"page,omitempty"`
}

type githubIssue struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	StateReason string     `json:"state_reason"`
	HTMLURL     string     `json:"html_url"`
	Comments    int        `json:"comments"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	User        githubUser `json:"user"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest *struct {
		MergedAt *time.Time `json:"merged_at"`
	} `json:"pull_request"`
}

type githubUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type githubComment struct {
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	User      githubUser `json:"user"`
}

// syncRepo queues every issue and pull request updated since the cursor,
// one page per run.
func (p *githubProvider) syncRepo(ctx context.Context, c *conn, repoID string) error {
	src, err := p.m.source(ctx, c.ID, repoID)
	if err != nil || src == nil || !src.Enabled {
		return err
	}
	token, err := p.installationToken(ctx, c.AccountID)
	if err != nil {
		return err
	}
	var cur githubCursor
	_ = json.Unmarshal([]byte(src.cursor), &cur)
	if cur.Since == "" {
		cur.Since = p.m.now().AddDate(0, 0, -p.m.cfg.BackfillDays).UTC().Format(time.RFC3339)
	}
	if cur.Page < 1 {
		cur.Page = 1
	}
	q := url.Values{"state": {"all"}, "sort": {"updated"}, "direction": {"asc"}, "since": {cur.Since},
		"per_page": {strconv.Itoa(githubPageSize)}, "page": {strconv.Itoa(cur.Page)}}
	var issues []githubIssue
	status, _, err := p.do(ctx, http.MethodGet, p.m.cfg.GitHubAPIURL+"/repos/"+src.Name+"/issues?"+q.Encode(), token, nil, &issues)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone, http.StatusMovedPermanently, http.StatusForbidden:
		// Gone, renamed or issues disabled: the list refresh sorts it out.
		slog.Info("github repo not readable; refreshing repository list", "connection", c.ID, "repo", src.Name, "status", status)
		return p.m.enqueue(ctx, nil, c.ID, jobSources, jobSources, nil, 0)
	default:
		return fmt.Errorf("github: list issues: HTTP %d", status)
	}
	for _, is := range issues {
		if err := p.m.enqueue(ctx, nil, c.ID, jobItem, repoID+"#"+strconv.Itoa(is.Number),
			map[string]any{"repo": src.Name, "number": is.Number}, 0); err != nil {
			return err
		}
	}
	if len(issues) == githubPageSize {
		last := issues[len(issues)-1].UpdatedAt.UTC().Format(time.RFC3339)
		if last == cur.Since {
			cur.Page++ // a page full of identical timestamps: step through it
		} else {
			cur.Since, cur.Page = last, 1
		}
		if err := p.m.saveCursor(ctx, c.ID, repoID, mustJSON(cur), false); err != nil {
			return err
		}
		return p.m.enqueue(ctx, nil, c.ID, jobSync, repoID, nil, 0)
	}
	next := cur.Since
	if len(issues) > 0 {
		next = issues[len(issues)-1].UpdatedAt.UTC().Format(time.RFC3339)
	}
	return p.m.saveCursor(ctx, c.ID, repoID, mustJSON(githubCursor{Since: next}), true)
}

// syncItem fetches one issue or pull request with its comments and stores
// it. It always reads the current state from the API rather than trusting
// webhook payloads, so edits, deletions and out-of-order events converge.
func (p *githubProvider) syncItem(ctx context.Context, c *conn, key string, payload json.RawMessage) error {
	var it struct {
		Repo   string `json:"repo"`
		Number int    `json:"number"`
	}
	if err := json.Unmarshal(payload, &it); err != nil || it.Repo == "" || it.Number <= 0 {
		return &permanentError{"bad github item job"}
	}
	repoID, _, _ := strings.Cut(key, "#")
	src, err := p.m.source(ctx, c.ID, repoID)
	if err != nil || src == nil || !src.Enabled {
		return err
	}
	token, err := p.installationToken(ctx, c.AccountID)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/repos/%s/issues/%d", p.m.cfg.GitHubAPIURL, src.Name, it.Number)
	var is githubIssue
	status, _, err := p.do(ctx, http.MethodGet, base, token, nil, &is)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone:
		return p.m.removeItem(ctx, c, key)
	default:
		return fmt.Errorf("github: get issue: HTTP %d", status)
	}
	var comments []githubComment
	if is.Comments > 0 {
		for page := 1; page <= githubMaxComments/100; page++ {
			var batch []githubComment
			status, _, err := p.do(ctx, http.MethodGet, fmt.Sprintf("%s/comments?per_page=100&page=%d", base, page), token, nil, &batch)
			if err != nil {
				return err
			}
			if status != http.StatusOK {
				break
			}
			comments = append(comments, batch...)
			if len(batch) < 100 {
				break
			}
		}
	}
	header, body, meta := renderGitHubIssue(src.Name, &is, comments)
	created := is.CreatedAt.UTC()
	return p.m.storeItem(ctx, c, item{Key: key, SourceID: repoID, Header: header, Body: body, Type: "semantic",
		EventDate: &created, Meta: meta})
}

// --- Webhooks -----------------------------------------------------------------------

// VerifyGitHubSignature checks X-Hub-Signature-256.
func VerifyGitHubSignature(secret string, sig string, body []byte) bool {
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal([]byte("sha256="+hex.EncodeToString(mac.Sum(nil))), []byte(sig))
}

// GitHubWebhook handles a GitHub App webhook delivery and returns the status
// to reply with. Work is queued, never done inline.
func (m *Manager) GitHubWebhook(ctx context.Context, h http.Header, body []byte) int {
	if !m.github.enabled() {
		return http.StatusNotFound
	}
	if !VerifyGitHubSignature(m.cfg.GitHubWebhookSecret, h.Get("X-Hub-Signature-256"), body) {
		metrics.ConnectorWebhooks.WithLabelValues(ProviderGitHub, "bad_signature").Inc()
		return http.StatusUnauthorized
	}
	if deliveryID := h.Get("X-GitHub-Delivery"); deliveryID != "" {
		claimed, err := m.ClaimWebhookDelivery(ctx, ProviderGitHub, deliveryID)
		if err != nil {
			return http.StatusInternalServerError
		}
		if !claimed {
			metrics.ConnectorWebhooks.WithLabelValues(ProviderGitHub, "duplicate").Inc()
			return http.StatusOK
		}
	}
	var ev struct {
		Action       string `json:"action"`
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
		Repository *githubRepo `json:"repository"`
		Issue      *struct {
			Number int `json:"number"`
		} `json:"issue"`
		PullRequest *struct {
			Number int `json:"number"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return http.StatusBadRequest
	}
	if m.deps.Pool == nil {
		return http.StatusOK
	}
	event := h.Get("X-GitHub-Event")
	inst := strconv.FormatInt(ev.Installation.ID, 10)
	result := "ignored"
	var err error
	switch event {
	case "installation":
		switch ev.Action {
		case "deleted":
			result = "uninstalled"
			m.github.forgetInstallation(inst)
			err = m.revokeAccount(ctx, ProviderGitHub, inst, "The Memro GitHub App was uninstalled.")
		case "suspend":
			result = "suspended"
			m.github.forgetInstallation(inst)
			_, err = m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET status = 'paused',
				status_detail = 'The GitHub App installation is suspended.', updated_at = NOW()
				WHERE provider = 'github' AND account_id = $1 AND status = 'active'`, inst)
		case "unsuspend":
			result = "unsuspended"
			_, err = m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET status = 'active', status_detail = '', updated_at = NOW()
				WHERE provider = 'github' AND account_id = $1 AND status = 'paused'`, inst)
			if err == nil {
				err = m.queueForInstallation(ctx, inst, jobSources, jobSources, nil, "")
			}
		}
	case "installation_repositories", "repository":
		result = "queued"
		err = m.queueForInstallation(ctx, inst, jobSources, jobSources, nil, "")
	case "issues", "issue_comment", "pull_request", "pull_request_review":
		number := 0
		if ev.Issue != nil {
			number = ev.Issue.Number
		} else if ev.PullRequest != nil {
			number = ev.PullRequest.Number
		}
		if ev.Repository == nil || number == 0 {
			break
		}
		repoID := strconv.FormatInt(ev.Repository.ID, 10)
		result = "queued"
		err = m.queueForInstallation(ctx, inst, jobItem, repoID+"#"+strconv.Itoa(number),
			map[string]any{"repo": ev.Repository.FullName, "number": number}, repoID)
	}
	if err != nil {
		slog.Error("github webhook", "event", event, "err", err)
		metrics.ConnectorWebhooks.WithLabelValues(ProviderGitHub, "error").Inc()
		return http.StatusInternalServerError
	}
	metrics.ConnectorWebhooks.WithLabelValues(ProviderGitHub, result).Inc()
	return http.StatusAccepted
}

// queueForInstallation queues a job for each active connection of an
// installation, limited to connections that sync repoID when it is set.
func (m *Manager) queueForInstallation(ctx context.Context, inst, kind, key string, payload any, repoID string) error {
	q := `SELECT c.id FROM connector_connections c WHERE c.provider = 'github' AND c.account_id = $1 AND c.status = 'active'`
	args := []any{inst}
	if repoID != "" {
		q += ` AND EXISTS (SELECT 1 FROM connector_sources s WHERE s.connection_id = c.id AND s.external_id = $2 AND s.enabled)`
		args = append(args, repoID)
	}
	rows, err := m.deps.Pool.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	delay := time.Duration(0)
	if kind == jobItem {
		delay = githubItemDelay
	}
	for _, id := range ids {
		if err := m.enqueue(ctx, nil, id, kind, key, payload, delay); err != nil {
			return err
		}
	}
	return nil
}
