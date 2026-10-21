package api_test

// End-to-end tests of the Jira, Linear, Notion, GitLab and Sentry
// connectors against fake provider APIs. Every outgoing request is routed
// to one fake server (the original host travels in X-Test-Host), so the real
// OAuth and API URLs are exercised: connecting, syncing with pagination,
// refreshing expiring tokens (rotation, a forced refresh after 401, a
// refused refresh), and webhooks: signatures, routing to the connections
// that chose the source, and deletes.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/internal/connectors"
)

// toFake sends every request to target, keeping the path and query.
type toFake struct{ target *url.URL }

func (t toFake) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme, r2.URL.Host, r2.Host = t.target.Scheme, t.target.Host, t.target.Host
	r2.Header.Set("X-Test-Host", r.URL.Host)
	return http.DefaultTransport.RoundTrip(r2)
}

func fakeClient(srv *httptest.Server) func(*connectors.Deps) {
	u, _ := url.Parse(srv.URL)
	return func(d *connectors.Deps) { d.HTTPClient = &http.Client{Transport: toFake{u}, Timeout: 10 * time.Second} }
}

func hmacHex(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// webhook posts a provider webhook with the given headers.
func (e *connEnv) webhook(provider string, body []byte, hdr map[string]string) int {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/connectors/"+provider+"/webhook", strings.NewReader(string(body)))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (e *connEnv) pendingJobs() int {
	var n int
	_ = e.st.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM connector_jobs`).Scan(&n)
	return n
}

// connectOAuth runs authorize → provider → callback and returns the connection.
func (e *connEnv) connectOAuth(provider string) connectors.Connection {
	e.t.Helper()
	u, cookie := e.authorize(provider)
	q := e.callback(provider, url.Values{"code": {"code-1"}, "state": {u.Query().Get("state")}}, cookie)
	if q.Get("connected") != "1" {
		e.t.Fatalf("%s connect: %v", provider, q)
	}
	for _, c := range e.list().Connections {
		if c.Provider == provider {
			return c
		}
	}
	e.t.Fatalf("no %s connection", provider)
	return connectors.Connection{}
}

func (e *connEnv) enableSources(id fmt.Stringer, ids ...string) {
	e.t.Helper()
	en := map[string]bool{}
	for _, s := range ids {
		en[s] = true
	}
	resp, b := e.call("PUT", "/connectors/"+id.String()+"/sources", map[string]any{"enabled": en}, nil)
	e.must(200, resp.StatusCode, b, "enable sources")
}

// --- Jira ---------------------------------------------------------------------

type fakeJira struct {
	mu            sync.Mutex
	access        string          // the access token currently valid
	refresh       string          // the refresh token currently valid
	firstExpires  int64           // expires_in of the code exchange
	refreshes     []string        // refresh tokens presented
	refuseRefresh bool            // answer invalid_grant
	rejectOnce    map[string]bool // access tokens to answer 401 to once
	searches      []url.Values
	n             int
}

func (f *fakeJira) issue(key string) map[string]any {
	project := strings.Split(key, "-")[0]
	return map[string]any{"id": "1" + strings.Split(key, "-")[1], "key": key,
		"self": "https://api.atlassian.com/ex/jira/cloud-1/rest/api/3/issue/" + key,
		"fields": map[string]any{"summary": "Summary of " + key, "description": "Details of " + key,
			"status": map[string]any{"name": "In Progress"}, "issuetype": map[string]any{"name": "Bug"},
			"project": map[string]any{"id": "100", "key": project, "name": project + " project"},
			"created": "2026-10-19T10:00:00.000+0000", "updated": "2026-10-20T10:00:00.000+0000"}}
}

func (f *fakeJira) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/oauth/token" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			switch body["grant_type"] {
			case "authorization_code":
				f.access, f.refresh = "a1", "r1"
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": f.access, "refresh_token": f.refresh, "expires_in": f.firstExpires})
			case "refresh_token":
				f.refreshes = append(f.refreshes, body["refresh_token"])
				if f.refuseRefresh || body["refresh_token"] != f.refresh {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
					return
				}
				f.n++
				f.access, f.refresh = fmt.Sprintf("a%d", f.n+1), fmt.Sprintf("r%d", f.n+1) // rotation
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": f.access, "refresh_token": f.refresh, "expires_in": 3600})
			}
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok != f.access || f.rejectOnce[tok] {
			delete(f.rejectOnce, tok)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch p := r.URL.Path; {
		case p == "/oauth/token/accessible-resources":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "cloud-1", "url": "https://acme.atlassian.net", "name": "Acme"}})
		case p == "/ex/jira/cloud-1/rest/api/3/myself":
			_ = json.NewEncoder(w).Encode(map[string]any{"accountId": "u-1", "displayName": "Ana"})
		case p == "/ex/jira/cloud-1/rest/api/3/project/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"values": []map[string]any{
				{"id": "100", "key": "ENG", "name": "Engineering"}, {"id": "101", "key": "SEC", "name": "Security"}}})
		case p == "/ex/jira/cloud-1/rest/api/3/search/jql":
			q := r.URL.Query()
			f.searches = append(f.searches, q)
			if q.Get("nextPageToken") == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{f.issue("ENG-1"), f.issue("ENG-2")}, "nextPageToken": "page2"})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{f.issue("ENG-3")}, "isLast": true})
			}
		case strings.HasPrefix(p, "/ex/jira/cloud-1/rest/api/3/issue/"):
			_ = json.NewEncoder(w).Encode(f.issue(strings.TrimPrefix(p, "/ex/jira/cloud-1/rest/api/3/issue/")))
		case strings.HasPrefix(p, "/rest/api/3/search"):
			t.Errorf("the removed /rest/api/3/search endpoint was called")
			w.WriteHeader(http.StatusGone)
		default:
			t.Errorf("unexpected Jira request %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func jiraHook(e *connEnv, secret string, event, key, self string) int {
	f := &fakeJira{}
	is := f.issue(key)
	if self != "" {
		is["self"] = self
	}
	body, _ := json.Marshal(map[string]any{"webhookEvent": event, "issue": is})
	return e.webhook("jira", body, map[string]string{"X-Hub-Signature": "sha256=" + hmacHex(secret, body)})
}

func TestJiraConnector(t *testing.T) {
	f := &fakeJira{firstExpires: 60, rejectOnce: map[string]bool{}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	const secret = "jira-hook-secret"
	e := setupConnectorsDeps(t, 1000, func(c *config.Config) {
		c.Connectors.JiraClientID, c.Connectors.JiraClientSecret, c.Connectors.JiraWebhookSecret = "jid", "jsecret", secret
	}, fakeClient(srv))

	conn := e.connectOAuth("jira")
	if conn.AccountID != "cloud-1" || conn.AccountName != "Acme" {
		t.Fatalf("connection = %+v", conn)
	}
	for _, p := range e.list().Providers {
		if p.Name == "jira" && p.Kind != "oauth" {
			t.Fatalf("jira listed as %q", p.Kind)
		}
	}

	// The first token expires within minutes: it is refreshed before use, and
	// the rotated refresh token replaces the old one.
	e.drain()
	e.enableSources(conn.ID, "ENG")
	e.drain()
	if len(f.refreshes) != 1 || f.refreshes[0] != "r1" {
		t.Fatalf("refreshes = %v, want one with r1", f.refreshes)
	}
	text := e.memoryText()
	for _, k := range []string{"ENG-1", "ENG-2", "ENG-3"} {
		if !strings.Contains(text, k) {
			t.Fatalf("%s missing:\n%s", k, text)
		}
	}
	if len(f.searches) != 2 || !strings.Contains(f.searches[0].Get("jql"), `project = "ENG"`) ||
		f.searches[1].Get("nextPageToken") != "page2" {
		t.Fatalf("searches = %v", f.searches)
	}

	// A second pass filters on the checkpoint (less a day).
	f.searches = nil
	e.call("POST", "/connectors/"+conn.ID.String()+"/sync", nil, nil)
	e.drain()
	if len(f.searches) == 0 || !strings.Contains(f.searches[0].Get("jql"), `updated >= "2026-10-19 10:00"`) {
		t.Fatalf("second pass jql = %v", f.searches)
	}

	// A 401 on a token that has not expired forces one refresh, then succeeds.
	f.rejectOnce[f.access] = true
	e.call("POST", "/connectors/"+conn.ID.String()+"/sync", nil, nil)
	e.drain()
	if len(f.refreshes) != 2 || f.refreshes[1] != "r2" {
		t.Fatalf("refreshes = %v, want r1, r2", f.refreshes)
	}
	if l := e.list(); l.Connections[0].Status != connectors.StatusActive {
		t.Fatalf("status after a forced refresh: %+v", l.Connections[0])
	}

	// Webhooks: signed, for this site, for a chosen project only.
	if code := e.webhook("jira", []byte(`{"webhookEvent":"jira:issue_updated"}`), nil); code != 401 {
		t.Fatalf("unsigned webhook: %d", code)
	}
	req, _ := http.NewRequest("POST", e.srv.URL+"/connectors/jira/webhook?secret="+secret, strings.NewReader(`{"webhookEvent":"jira:issue_updated","issue":{"key":"ENG-9"}}`))
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 401 {
		t.Fatalf("a secret in the URL was accepted: %d", resp.StatusCode)
	}
	if code := jiraHook(e, secret, "jira:issue_updated", "SEC-7", ""); code != 200 || e.pendingJobs() != 0 {
		t.Fatalf("a project nobody chose queued work: %d, %d jobs", code, e.pendingJobs())
	}
	if code := jiraHook(e, secret, "jira:issue_updated", "ENG-5", "https://api.atlassian.com/ex/jira/other-site/rest/api/3/issue/1"); code != 200 || e.pendingJobs() != 0 {
		t.Fatalf("another site's issue queued work: %d, %d jobs", code, e.pendingJobs())
	}
	if code := jiraHook(e, secret, "jira:issue_updated", "ENG-5", ""); code != 200 || e.pendingJobs() != 1 {
		t.Fatalf("chosen project: %d, %d jobs", code, e.pendingJobs())
	}
	e.drain()
	if !strings.Contains(e.memoryText(), "Summary of ENG-5") {
		t.Fatal("webhook issue not stored")
	}
	jiraHook(e, secret, "jira:issue_deleted", "ENG-5", "")
	if strings.Contains(e.memoryText(), "Summary of ENG-5") {
		t.Fatal("deleted issue still in memory")
	}

	// A refused refresh revokes the connection: only a new sign-in helps.
	f.rejectOnce[f.access] = true
	f.refuseRefresh = true
	e.call("POST", "/connectors/"+conn.ID.String()+"/sync", nil, nil)
	e.drain()
	if l := e.list(); l.Connections[0].Status != connectors.StatusRevoked {
		t.Fatalf("status after a refused refresh: %+v", l.Connections[0])
	}
}

func TestOAuthWebhooksOffWithoutSecrets(t *testing.T) {
	e := setupConnectors(t, 100, func(*config.Config) {})
	for _, p := range []string{"jira", "gitlab", "notion", "sentry", "linear"} {
		if code := e.webhook(p, []byte(`{"object_kind":"merge_request","project":{"id":1},"type":"page.deleted","entity":{"id":"x","type":"page"},"workspace_id":"w"}`), nil); code != 503 {
			t.Errorf("%s webhook without a configured secret: %d, want 503", p, code)
		}
	}
}

// --- Linear -------------------------------------------------------------------

func TestLinearWebhookRespectsChosenTeams(t *testing.T) {
	var mu sync.Mutex
	issues := map[string]string{"iss-1": "team-a", "iss-2": "team-b"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "lin-1", "expires_in": 86400, "refresh_token": "lr-1"})
			return
		}
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		switch {
		case strings.Contains(q.Query, "viewer"):
			_, _ = io.WriteString(w, `{"data":{"viewer":{"id":"v1","name":"Ana"},"organization":{"id":"org-1","name":"Acme","urlKey":"acme"}}}`)
		case strings.Contains(q.Query, "teams"):
			_, _ = io.WriteString(w, `{"data":{"teams":{"nodes":[{"id":"team-a","name":"Alpha","key":"A"},{"id":"team-b","name":"Private","key":"B"}]}}}`)
		case strings.Contains(q.Query, "team(id"):
			_, _ = io.WriteString(w, `{"data":{"team":{"issues":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}`)
		case strings.Contains(q.Query, "issue(id"):
			id, _ := q.Variables["id"].(string)
			team := issues[id]
			if team == "" {
				_, _ = io.WriteString(w, `{"data":{"issue":null}}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"issue": map[string]any{
				"id": id, "identifier": strings.ToUpper(id), "title": "Title of " + id, "description": "Secret plans " + id,
				"team": map[string]any{"id": team, "name": team, "key": "T"}, "state": map[string]any{"name": "Todo"},
				"createdAt": "2026-10-20T10:00:00Z", "updatedAt": "2026-10-20T10:00:00Z", "url": "https://linear.app/x"}}})
		}
	}))
	defer srv.Close()
	const secret = "linear-secret"
	e := setupConnectorsDeps(t, 1000, func(c *config.Config) {
		c.Connectors.LinearClientID, c.Connectors.LinearClientSecret, c.Connectors.LinearWebhookSecret = "lid", "lsec", secret
	}, fakeClient(srv))
	conn := e.connectOAuth("linear")
	e.drain()
	e.enableSources(conn.ID, "team-a")
	e.drain()

	hook := func(action, id string) {
		body, _ := json.Marshal(map[string]any{"action": action, "type": "Issue", "organizationId": "org-1",
			"data": map[string]any{"id": id, "teamId": issues[id]}})
		if code := e.webhook("linear", body, map[string]string{"Linear-Signature": hmacHex(secret, body)}); code != 200 {
			t.Fatalf("linear webhook: %d", code)
		}
		e.drain()
	}
	hook("update", "iss-2")
	if strings.Contains(e.memoryText(), "Secret plans iss-2") {
		t.Fatal("an issue from a team the user did not choose was stored")
	}
	hook("update", "iss-1")
	if !strings.Contains(e.memoryText(), "Secret plans iss-1") {
		t.Fatal("an issue from the chosen team was not stored")
	}
	// Moved to a team that is not chosen: removed.
	issues["iss-1"] = "team-b"
	hook("update", "iss-1")
	if strings.Contains(e.memoryText(), "Secret plans iss-1") {
		t.Fatal("an issue moved out of the chosen team stayed in memory")
	}
}

// --- Notion -------------------------------------------------------------------

func TestNotionWebhooks(t *testing.T) {
	pages := map[string]string{"page-1": "db-1", "page-2": "db-2"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case p == "/v1/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"n-1","workspace_id":"ws-1","workspace_name":"Acme","bot_id":"b1","owner":{"type":"user","user":{"id":"u1","name":"Ana"}}}`)
		case p == "/v1/search":
			_, _ = io.WriteString(w, `{"results":[{"id":"db-1","title":[{"plain_text":"Specs"}]},{"id":"db-2","title":[{"plain_text":"HR"}]}]}`)
		case strings.HasPrefix(p, "/v1/databases/"):
			_, _ = io.WriteString(w, `{"results":[],"has_more":false}`)
		case strings.HasPrefix(p, "/v1/pages/"):
			id := strings.TrimPrefix(p, "/v1/pages/")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "url": "https://notion.so/" + id,
				"created_time": "2026-10-20T10:00:00Z", "last_edited_time": "2026-10-20T11:00:00Z",
				"parent":     map[string]any{"type": "database_id", "database_id": pages[id]},
				"properties": map[string]any{"Name": map[string]any{"type": "title", "title": []any{map[string]any{"plain_text": "Page " + id}}}}})
		case strings.HasPrefix(p, "/v1/blocks/"):
			_, _ = io.WriteString(w, `{"results":[{"id":"b","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Body text"}]}}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	const secret = "notion-verification-token"
	e := setupConnectorsDeps(t, 1000, func(c *config.Config) {
		c.Connectors.NotionClientID, c.Connectors.NotionClientSecret, c.Connectors.NotionWebhookSecret = "nid", "nsec", secret
	}, fakeClient(srv))
	conn := e.connectOAuth("notion")
	e.drain()
	e.enableSources(conn.ID, "db-1")
	e.drain()

	send := func(typ, page string) int {
		body, _ := json.Marshal(map[string]any{"type": typ, "workspace_id": "ws-1", "entity": map[string]any{"id": page, "type": "page"}})
		code := e.webhook("notion", body, map[string]string{"X-Notion-Signature": "sha256=" + hmacHex(secret, body)})
		e.drain()
		return code
	}
	if code := e.webhook("notion", []byte(`{"type":"page.deleted","workspace_id":"ws-1","entity":{"id":"page-1","type":"page"}}`),
		map[string]string{"X-Notion-Signature": "sha256=00"}); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	if code := send("page.content_updated", "page-2"); code != 200 || strings.Contains(e.memoryText(), "Page page-2") {
		t.Fatalf("a page outside the chosen databases was stored (%d)", code)
	}
	if code := send("page.content_updated", "page-1"); code != 200 || !strings.Contains(e.memoryText(), "Page page-1") {
		t.Fatalf("a page in the chosen database was not stored (%d):\n%s", code, e.memoryText())
	}
	if send("page.deleted", "page-1"); strings.Contains(e.memoryText(), "Page page-1") {
		t.Fatal("a deleted page stayed in memory")
	}
}

func TestNotionVerificationRequestWithoutSecret(t *testing.T) {
	e := setupConnectors(t, 100, func(c *config.Config) {
		c.Connectors.NotionClientID, c.Connectors.NotionClientSecret = "nid", "nsec"
	})
	if code := e.webhook("notion", []byte(`{"verification_token":"secret_abc"}`), nil); code != 200 {
		t.Fatalf("verification request: %d", code)
	}
	if code := e.webhook("notion", []byte(`{"type":"page.deleted","workspace_id":"w","entity":{"id":"p","type":"page"}}`), nil); code != 503 {
		t.Fatalf("an event before the secret is set: %d", code)
	}
}

// --- GitLab and Sentry -----------------------------------------------------------

func TestGitLabAndSentryWebhooksReachOnlyChosenProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Header.Get("X-Test-Host")
		switch p := r.URL.Path; {
		case p == "/oauth/token" && host == "gitlab.com":
			_, _ = io.WriteString(w, `{"access_token":"g-1","refresh_token":"gr-1","expires_in":7200}`)
		case p == "/api/v4/user":
			_, _ = io.WriteString(w, `{"id":42,"username":"ana","name":"Ana"}`)
		case p == "/api/v4/projects":
			_, _ = io.WriteString(w, `[{"id":7,"path_with_namespace":"acme/api","visibility":"private"},{"id":8,"path_with_namespace":"acme/secret","visibility":"private"}]`)
		case strings.HasPrefix(p, "/api/v4/projects/") && strings.HasSuffix(p, "/notes"):
			_, _ = io.WriteString(w, `[]`)
		case strings.HasPrefix(p, "/api/v4/projects/") && strings.Contains(p, "/merge_requests/"):
			parts := strings.Split(p, "/")
			pid, _ := strconv.Atoi(parts[4])
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "iid": 3, "project_id": pid, "title": "MR in project " + parts[4],
				"state": "opened", "created_at": "2026-10-20T10:00:00Z", "updated_at": "2026-10-20T10:00:00Z", "author": map[string]any{"username": "ana"}})
		case strings.HasPrefix(p, "/api/v4/projects/"):
			_, _ = io.WriteString(w, `[]`) // merge request and pipeline lists
		case p == "/oauth/token/" && host == "sentry.io":
			_, _ = io.WriteString(w, `{"access_token":"s-1","refresh_token":"sr-1","expires_in":28800}`)
		case p == "/api/0/organizations/":
			_, _ = io.WriteString(w, `[{"id":"1","slug":"acme","name":"Acme"}]`)
		case p == "/api/0/users/me/":
			_, _ = io.WriteString(w, `{"id":"u1","name":"Ana"}`)
		case p == "/api/0/organizations/acme/projects/":
			_, _ = io.WriteString(w, `[{"id":"1","slug":"web","name":"Web"},{"id":"2","slug":"payments","name":"Payments"}]`)
		case strings.HasPrefix(p, "/api/0/projects/"):
			_, _ = io.WriteString(w, `[]`)
		case strings.HasPrefix(p, "/api/0/issues/") && strings.HasSuffix(p, "/events/latest/"):
			_, _ = io.WriteString(w, `{}`)
		case strings.HasPrefix(p, "/api/0/issues/"):
			id := strings.Split(p, "/")[4]
			slug := map[string]string{"11": "web", "22": "payments"}[id]
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "shortId": "S-" + id, "title": "Crash " + id, "level": "error",
				"status": "unresolved", "count": "1234", "userCount": 9, "firstSeen": "2026-10-20T10:00:00Z", "lastSeen": "2026-10-20T11:00:00Z",
				"project": map[string]any{"slug": slug, "name": slug}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	const glSecret, seSecret = "gitlab-token", "sentry-secret"
	e := setupConnectorsDeps(t, 1000, func(c *config.Config) {
		c.Connectors.GitLabClientID, c.Connectors.GitLabClientSecret, c.Connectors.GitLabWebhookSecret = "gid", "gsec", glSecret
		c.Connectors.SentryClientID, c.Connectors.SentryClientSecret, c.Connectors.SentryWebhookSecret = "sid", "ssec", seSecret
	}, fakeClient(srv))

	gl := e.connectOAuth("gitlab")
	e.drain()
	e.enableSources(gl.ID, "7")
	e.drain()
	mr := func(project int) int {
		body, _ := json.Marshal(map[string]any{"object_kind": "merge_request", "project": map[string]any{"id": project},
			"object_attributes": map[string]any{"iid": 3}})
		code := e.webhook("gitlab", body, map[string]string{"X-Gitlab-Token": glSecret})
		e.drain()
		return code
	}
	if code := e.webhook("gitlab", []byte(`{"object_kind":"merge_request","project":{"id":7}}`), map[string]string{"X-Gitlab-Token": "wrong"}); code != 401 {
		t.Fatalf("gitlab wrong token: %d", code)
	}
	if mr(8); strings.Contains(e.memoryText(), "MR in project 8") {
		t.Fatal("gitlab: a project the user did not choose was stored")
	}
	if mr(7); !strings.Contains(e.memoryText(), "MR in project 7") {
		t.Fatalf("gitlab: the chosen project's MR was not stored:\n%s", e.memoryText())
	}

	se := e.connectOAuth("sentry")
	e.drain()
	e.enableSources(se.ID, "web")
	e.drain()
	issue := func(id, slug string) int {
		body, _ := json.Marshal(map[string]any{"action": "created", "data": map[string]any{"issue": map[string]any{"id": id, "project": map[string]any{"slug": slug}}}})
		code := e.webhook("sentry", body, map[string]string{"Sentry-Hook-Signature": hmacHex(seSecret, body)})
		e.drain()
		return code
	}
	if issue("22", "payments"); strings.Contains(e.memoryText(), "Crash 22") {
		t.Fatal("sentry: a project the user did not choose was stored")
	}
	if issue("11", "web"); !strings.Contains(e.memoryText(), "Crash 11") || !strings.Contains(e.memoryText(), "1234") {
		t.Fatalf("sentry: the chosen project's issue (count sent as a string) was not stored:\n%s", e.memoryText())
	}
}
