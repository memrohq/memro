package api_test

// End-to-end tests of the Slack and GitHub connectors against fake provider
// APIs: OAuth with browser binding, source selection, history sync,
// webhooks (new, edited, deleted content), revocation and disconnect.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/api"
	"github.com/memrohq/memro/server/internal/connectors"
	"github.com/memrohq/memro/server/internal/domain"
)

type connEnv struct {
	*env
	mgr     *connectors.Manager
	a       agent
	session string
}

func setupConnectors(t *testing.T, memoryCap int, mutate func(*config.Config)) *connEnv {
	return setupConnectorsDeps(t, memoryCap, mutate, nil)
}

// setupConnectorsDeps also lets a test change the connector manager's
// dependencies (a fake HTTP transport, a test catalog).
func setupConnectorsDeps(t *testing.T, memoryCap int, mutate func(*config.Config), deps func(*connectors.Deps)) *connEnv {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	var mgr *connectors.Manager
	e := setup(t, func(d *api.Deps) {
		d.Config.AppURL = "http://dash.test"
		d.Config.Connectors.EncryptionKey = base64.StdEncoding.EncodeToString(key)
		d.Config.Connectors.CallbackBaseURL = "http://dash.test/api"
		d.Config.Connectors.BackfillDays = 30
		d.Config.MemorySoftCap = memoryCap
		mutate(&d.Config)
		quota := extension.PerAgentCap{Max: memoryCap}
		d.Quota = quota
		cd := connectors.Deps{Config: d.Config, Pool: d.Store.Pool, Store: d.Store, Indexer: d.Indexer,
			Invalidate: d.Engine.Cache().InvalidateAgent, Quota: quota}
		if deps != nil {
			deps(&cd)
		}
		m, err := connectors.New(cd)
		if err != nil {
			t.Fatal(err)
		}
		mgr = m
		d.Connectors = m
	})
	a, session := e.dashboardSession(fmt.Sprintf("conn-%d@example.com", time.Now().UnixNano()))
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = e.st.Pool.Exec(ctx, `DELETE FROM connector_jobs`)
		_, _ = e.st.Pool.Exec(ctx, `DELETE FROM connector_connections WHERE agent_id = $1`, a.id)
	})
	return &connEnv{env: e, mgr: mgr, a: a, session: session}
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// call sends a dashboard request (session auth unless hdr overrides it).
func (e *connEnv) call(method, path string, body any, hdr map[string]string) (*http.Response, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(j(body)))
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+e.session)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// drain runs connector jobs, delays included, until none are left.
func (e *connEnv) drain() {
	e.t.Helper()
	ctx := context.Background()
	for range 100 {
		if _, err := e.st.Pool.Exec(ctx, `UPDATE connector_jobs SET run_at = NOW() - INTERVAL '1 second',
			locked_until = CASE WHEN locked_until IS NULL THEN NULL ELSE NOW() - INTERVAL '1 second' END`); err != nil {
			e.t.Fatal(err)
		}
		if err := e.mgr.RunOnce(ctx); err != nil {
			e.t.Fatal(err)
		}
		var n int
		_ = e.st.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_jobs`).Scan(&n)
		if n == 0 {
			return
		}
	}
	rows, _ := e.st.Pool.Query(ctx, `SELECT kind, key, attempts, COALESCE(last_error, '') FROM connector_jobs`)
	defer rows.Close()
	for rows.Next() {
		var kind, key, msg string
		var n int
		_ = rows.Scan(&kind, &key, &n, &msg)
		e.t.Errorf("job left: %s %s attempts=%d err=%s", kind, key, n, msg)
	}
	e.t.FailNow()
}

func (e *connEnv) memories() []domain.Memory {
	e.t.Helper()
	ms, err := e.st.ListMemories(context.Background(), e.a.id, uuid.Nil, "", "", 100)
	if err != nil {
		e.t.Fatal(err)
	}
	return ms
}

func (e *connEnv) memoryText() string {
	var b strings.Builder
	for _, m := range e.memories() {
		b.WriteString(m.Content + "\n---\n")
	}
	return b.String()
}

type connList struct {
	Providers   []connectors.ProviderInfo `json:"providers"`
	Connections []connectors.Connection   `json:"connections"`
}

func (e *connEnv) list() connList {
	e.t.Helper()
	resp, b := e.call("GET", "/connectors", nil, nil)
	e.must(200, resp.StatusCode, b, "list connectors")
	var l connList
	_ = json.Unmarshal(b, &l)
	return l
}

// authorize starts an OAuth flow and returns the provider URL and cookie.
func (e *connEnv) authorize(provider string) (*url.URL, *http.Cookie) {
	e.t.Helper()
	resp, b := e.call("POST", "/connectors/"+provider+"/authorize", nil, nil)
	e.must(200, resp.StatusCode, b, "authorize")
	var r struct{ URL string }
	_ = json.Unmarshal(b, &r)
	u, err := url.Parse(r.URL)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == connectors.StateCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				e.t.Fatalf("state cookie is not HttpOnly/SameSite=Lax: %+v", c)
			}
			return u, c
		}
	}
	e.t.Fatal("no state cookie")
	return nil, nil
}

// callback follows the provider's redirect back and returns where the
// browser is sent next.
func (e *connEnv) callback(provider string, q url.Values, cookie *http.Cookie) url.Values {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+"/connectors/"+provider+"/callback?"+q.Encode(), nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		e.t.Fatalf("callback status %d", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if !strings.HasPrefix(loc.String(), "http://dash.test/integrations?") {
		e.t.Fatalf("callback redirected to %s", loc)
	}
	return loc.Query()
}

// --- Fake Slack -----------------------------------------------------------------------

type fakeSlack struct {
	team     string
	mu       sync.Mutex
	day      time.Time
	joined   []string
	revoked  int
	messages map[string][]map[string]any // channel -> history, newest first
	replies  map[string][]map[string]any // root ts -> thread
}

func ts(t time.Time) string { return fmt.Sprintf("%d.000100", t.Unix()) }

func newFakeSlack(t *testing.T) (*fakeSlack, *httptest.Server) {
	day := time.Now().UTC().Truncate(24 * time.Hour)
	at := func(m int) string { return ts(day.Add(time.Hour + time.Duration(m)*time.Minute)) }
	f := &fakeSlack{day: day, team: "T" + strconv.FormatInt(time.Now().UnixNano(), 36),
		messages: map[string][]map[string]any{"C1": {
			{"type": "message", "subtype": "channel_join", "user": "U3", "text": "<@U3> has joined the channel", "ts": at(5)},
			{"type": "message", "bot_id": "B1", "text": "bot says hi", "ts": at(4)},
			{"type": "message", "user": "U3", "text": "lunch?", "ts": at(3)},
			{"type": "message", "user": "U1", "text": "deploy is blocked, <@U2> can you look &amp; fix?", "ts": at(1),
				"thread_ts": at(1), "reply_count": 1, "latest_reply": at(2)},
		}},
		replies: map[string][]map[string]any{at(1): {
			{"type": "message", "user": "U1", "text": "deploy is blocked, <@U2> can you look &amp; fix?", "ts": at(1), "thread_ts": at(1)},
			{"type": "message", "user": "U2", "text": "on it", "ts": at(2), "thread_ts": at(1)},
		}},
	}
	names := map[string]string{"U1": "ana", "U2": "bob", "U3": "cy"}
	members := map[string][]string{"C2": {"U2"}, "C3": {"U1", "U2"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		method := strings.TrimPrefix(r.URL.Path, "/")
		reply := func(v map[string]any) { v["ok"] = true; _ = json.NewEncoder(w).Encode(v) }
		if method == "oauth.v2.access" {
			if r.Form.Get("client_secret") != "slack-secret" || r.Form.Get("code") != "good-code" {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_code"})
				return
			}
			reply(map[string]any{"access_token": "xoxb-1", "token_type": "bot", "bot_user_id": "UBOT", "app_id": "A1",
				"team": map[string]string{"id": f.team, "name": "Acme"}, "authed_user": map[string]string{"id": "U1"}})
			return
		}
		if r.Header.Get("Authorization") != "Bearer xoxb-1" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_auth"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch method {
		case "users.info":
			u := r.Form.Get("user")
			reply(map[string]any{"user": map[string]any{"id": u, "name": names[u], "profile": map[string]string{"display_name": names[u]}}})
		case "conversations.list":
			reply(map[string]any{"channels": []map[string]any{
				{"id": "C1", "name": "general"}, {"id": "C2", "name": "secret", "is_private": true},
				{"id": "C3", "name": "team", "is_private": true}}})
		case "conversations.members":
			reply(map[string]any{"members": members[r.Form.Get("channel")]})
		case "conversations.join":
			f.joined = append(f.joined, r.Form.Get("channel"))
			reply(map[string]any{})
		case "conversations.history":
			if r.Form.Get("oldest") == "" {
				t.Errorf("history without oldest bound")
			}
			reply(map[string]any{"messages": f.messages[r.Form.Get("channel")], "has_more": false})
		case "conversations.replies":
			reply(map[string]any{"messages": f.replies[r.Form.Get("ts")]})
		case "auth.revoke":
			f.revoked++
			reply(map[string]any{"revoked": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "unknown_method"})
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (e *connEnv) slackEvent(secret, team string, event map[string]any) (int, []byte) {
	e.t.Helper()
	body := j(map[string]any{"type": "event_callback", "team_id": team, "event": event})
	tsNow := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + tsNow + ":"))
	mac.Write(body)
	return e.do(nil, "", "POST", "/connectors/slack/events", body, map[string]string{
		"X-Slack-Request-Timestamp": tsNow, "X-Slack-Signature": "v0=" + hex.EncodeToString(mac.Sum(nil))})
}

func TestSlackConnector(t *testing.T) {
	fake, slackSrv := newFakeSlack(t)
	e := setupConnectors(t, 1000, func(c *config.Config) {
		c.Connectors.SlackClientID, c.Connectors.SlackClientSecret, c.Connectors.SlackSigningSecret = "slack-id", "slack-secret", "signing"
		c.Connectors.SlackAPIURL, c.Connectors.SlackOAuthURL = slackSrv.URL, slackSrv.URL+"/authorize"
	})

	l := e.list()
	if len(l.Connections) != 0 {
		t.Fatalf("expected 0 connections: %+v", l)
	}
	var slackEnabled, githubEnabled bool
	for _, p := range l.Providers {
		if p.Name == "slack" {
			slackEnabled = p.Enabled
		}
		if p.Name == "github" {
			githubEnabled = p.Enabled
		}
	}
	if !slackEnabled || githubEnabled {
		t.Fatalf("providers: %+v", l)
	}
	// Connectors are a dashboard action: a signed agent request may not use them.
	code, body := e.do(&e.a, "v2", "GET", "/connectors", nil, nil)
	e.must(403, code, body, "connectors with agent signature")

	authURL, cookie := e.authorize("slack")
	q := authURL.Query()
	if q.Get("client_id") != "slack-id" || q.Get("redirect_uri") != "http://dash.test/api/connectors/slack/callback" ||
		!strings.Contains(q.Get("scope"), "channels:history") || strings.Contains(q.Get("scope"), "chat:write") {
		t.Fatalf("authorize URL: %s", authURL)
	}
	state := q.Get("state")

	// Without the browser's cookie the callback is refused (login CSRF), and
	// that attempt does not use up the state.
	if r := e.callback("slack", url.Values{"state": {state}, "code": {"good-code"}}, nil); r.Get("error") == "" {
		t.Fatalf("callback without cookie succeeded: %v", r)
	}
	if r := e.callback("slack", url.Values{"state": {state}, "code": {"good-code"}}, cookie); r.Get("connected") != "1" {
		t.Fatalf("callback failed: %v", r)
	}
	if r := e.callback("slack", url.Values{"state": {state}, "code": {"good-code"}}, cookie); r.Get("error") == "" {
		t.Fatal("state accepted twice")
	}

	l = e.list()
	if len(l.Connections) != 1 || l.Connections[0].AccountName != "Acme" || l.Connections[0].Status != "active" ||
		l.Connections[0].ExternalUserName != "ana" {
		t.Fatalf("connections: %+v", l.Connections)
	}
	conn := l.Connections[0]
	var sealed []byte
	_ = e.st.Pool.QueryRow(context.Background(), `SELECT credentials FROM connector_connections WHERE id = $1`, conn.ID).Scan(&sealed)
	if strings.Contains(string(sealed), "xoxb-1") {
		t.Fatal("bot token stored in plaintext")
	}

	e.drain()
	resp, b := e.call("GET", "/connectors/"+conn.ID.String()+"/sources", nil, nil)
	e.must(200, resp.StatusCode, b, "sources")
	var srcs []connectors.Source
	_ = json.Unmarshal(b, &srcs)
	names := []string{}
	for _, s := range srcs {
		names = append(names, s.Name)
		if s.Enabled {
			t.Fatalf("channel %s enabled before the user chose it", s.Name)
		}
	}
	// #secret is private and ana is not a member: never offered.
	if strings.Join(names, ",") != "general,team" {
		t.Fatalf("sources: %v", names)
	}
	resp, b = e.call("PUT", "/connectors/"+conn.ID.String()+"/sources", map[string]any{"enabled": map[string]bool{"C2": true}}, nil)
	e.must(400, resp.StatusCode, b, "enable a channel the user cannot see")
	resp, b = e.call("PUT", "/connectors/"+conn.ID.String()+"/sources", map[string]any{"enabled": map[string]bool{"C1": true}}, nil)
	e.must(200, resp.StatusCode, b, "enable #general")
	if strings.Join(fake.joined, ",") != "C1" {
		t.Fatalf("joined %v", fake.joined)
	}

	e.drain()
	day := fake.day.Format("2006-01-02")
	text := e.memoryText()
	for _, want := range []string{"[Slack #general · " + day + " · Acme]", "ana: deploy is blocked, @bob can you look & fix?",
		"↳ 01:02 bob: on it", "cy: lunch?"} {
		if !strings.Contains(text, want) {
			t.Fatalf("memories missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "bot says hi") || strings.Contains(text, "has joined") {
		t.Fatalf("bot or join messages synced:\n%s", text)
	}
	ms := e.memories()
	var meta map[string]any
	_ = json.Unmarshal(ms[0].Metadata, &meta)
	if meta["source"] != "slack" || meta["channel"] != "C1" || ms[0].MemoryType != "episodic" {
		t.Fatalf("memory metadata: %s type %s", ms[0].Metadata, ms[0].MemoryType)
	}

	// Events: Slack's URL check, signatures, then new / edited / deleted messages.
	code, body = e.slackEvent("wrong", fake.team, map[string]any{"type": "message"})
	e.must(401, code, body, "bad signature")
	code, body = e.do(nil, "", "POST", "/connectors/slack/events", j(map[string]any{"type": "url_verification", "challenge": "abc"}), nil)
	e.must(401, code, body, "unsigned url_verification")

	newTS := ts(fake.day.Add(3 * time.Hour))
	code, body = e.slackEvent("signing", fake.team, map[string]any{"type": "message", "channel": "C1", "user": "U2", "text": "shipped the fix", "ts": newTS, "event_ts": newTS})
	e.must(200, code, body, "message event")
	// A message in a channel nobody syncs is not queued.
	code, body = e.slackEvent("signing", fake.team, map[string]any{"type": "message", "channel": "C3", "user": "U2", "text": "private stuff", "ts": newTS})
	e.must(200, code, body, "message in unsynced channel")
	e.drain()
	if text = e.memoryText(); !strings.Contains(text, "bob: shipped the fix") || strings.Contains(text, "private stuff") {
		t.Fatalf("after message event:\n%s", text)
	}

	editTS := ts(fake.day.Add(4 * time.Hour))
	code, body = e.slackEvent("signing", fake.team, map[string]any{"type": "message", "subtype": "message_changed", "channel": "C1", "event_ts": editTS,
		"message": map[string]any{"type": "message", "user": "U2", "text": "shipped the real fix", "ts": newTS, "edited": map[string]string{"ts": editTS}}})
	e.must(200, code, body, "edit event")
	e.drain()
	if text = e.memoryText(); !strings.Contains(text, "shipped the real fix") || strings.Contains(text, "shipped the fix") {
		t.Fatalf("after edit:\n%s", text)
	}

	delTS := ts(fake.day.Add(5 * time.Hour))
	code, body = e.slackEvent("signing", fake.team, map[string]any{"type": "message", "subtype": "message_deleted", "channel": "C1",
		"deleted_ts": newTS, "event_ts": delTS, "ts": delTS})
	e.must(200, code, body, "delete event")
	e.drain()
	if text = e.memoryText(); strings.Contains(text, "shipped") || !strings.Contains(text, "lunch?") {
		t.Fatalf("after delete:\n%s", text)
	}
	// A late, out-of-order copy of the original message doesn't resurrect it.
	code, body = e.slackEvent("signing", fake.team, map[string]any{"type": "message", "channel": "C1", "user": "U2", "text": "shipped the fix", "ts": newTS})
	e.must(200, code, body, "late duplicate")
	e.drain()
	if strings.Contains(e.memoryText(), "shipped") {
		t.Fatal("deleted message came back")
	}

	// Disconnect with the memories; the bot token is revoked (last user).
	resp, b = e.call("DELETE", "/connectors/"+conn.ID.String()+"?delete_memories=true", nil, nil)
	e.must(200, resp.StatusCode, b, "disconnect")
	if !strings.Contains(string(b), `"deleted_memories":1`) || fake.revoked != 1 {
		t.Fatalf("disconnect: %s revoked=%d", b, fake.revoked)
	}
	if len(e.memories()) != 0 || len(e.list().Connections) != 0 {
		t.Fatal("connection or memories left after disconnect")
	}
}

func TestSlackUninstallRevokes(t *testing.T) {
	fake, slackSrv := newFakeSlack(t)
	e := setupConnectors(t, 1000, func(c *config.Config) {
		c.Connectors.SlackClientID, c.Connectors.SlackClientSecret, c.Connectors.SlackSigningSecret = "slack-id", "slack-secret", "signing"
		c.Connectors.SlackAPIURL, c.Connectors.SlackOAuthURL = slackSrv.URL, slackSrv.URL+"/authorize"
	})
	u, cookie := e.authorize("slack")
	e.callback("slack", url.Values{"state": {u.Query().Get("state")}, "code": {"good-code"}}, cookie)
	code, body := e.slackEvent("signing", fake.team, map[string]any{"type": "app_uninstalled"})
	e.must(200, code, body, "uninstall event")
	if c := e.list().Connections; len(c) != 1 || c[0].Status != "revoked" {
		t.Fatalf("after uninstall: %+v", c)
	}
	resp, b := e.call("POST", "/connectors/"+e.list().Connections[0].ID.String()+"/sync", nil, nil)
	e.must(400, resp.StatusCode, b, "sync a revoked connection")
}

// --- Fake GitHub ------------------------------------------------------------------------

type fakeGitHub struct {
	mu          sync.Mutex
	issues      map[int]map[string]any
	comments    map[int][]map[string]any
	revokedUser []string
	appTokens   int
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *httptest.Server) {
	created := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	f := &fakeGitHub{
		issues: map[int]map[string]any{
			1: {"number": 1, "title": "Login fails on Safari", "body": "Steps: open the app in Safari.", "state": "open",
				"html_url": "https://github.com/acme-org/api/issues/1", "comments": 2, "created_at": created, "updated_at": created,
				"user": map[string]string{"login": "ana"}, "labels": []map[string]string{{"name": "bug"}}},
			2: {"number": 2, "title": "Add rate limits", "body": "Implements token buckets.", "state": "closed",
				"html_url": "https://github.com/acme-org/api/pull/2", "comments": 0, "created_at": created, "updated_at": created,
				"user": map[string]string{"login": "bob"}, "pull_request": map[string]any{"merged_at": created}},
		},
		comments: map[int][]map[string]any{1: {
			{"body": "Reproduced on Safari 18.", "created_at": created, "user": map[string]string{"login": "bob", "type": "User"}},
			{"body": "Build passed", "created_at": created, "user": map[string]string{"login": "ci[bot]", "type": "Bot"}},
		}},
	}
	users := map[string]map[string]any{"ghu_ana": {"id": 7, "login": "ana"}, "ghu_mallory": {"id": 9, "login": "mallory"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		write := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
		p := r.URL.Path
		switch {
		case p == "/login/oauth/access_token":
			_ = r.ParseForm()
			if r.Form.Get("client_secret") != "gh-secret" {
				write(map[string]string{"error": "incorrect_client_credentials"})
				return
			}
			tok := map[string]string{"gh-code": "ghu_ana", "gh-code-mallory": "ghu_mallory"}[r.Form.Get("code")]
			if tok == "" {
				write(map[string]string{"error": "bad_verification_code", "error_description": "The code is incorrect"})
				return
			}
			write(map[string]any{"access_token": tok, "refresh_token": "ghr_" + tok, "expires_in": 28800})
		case p == "/user":
			if u, ok := users[auth]; ok {
				write(u)
				return
			}
			w.WriteHeader(401)
		case p == "/user/installations":
			insts := []map[string]any{}
			if auth == "ghu_ana" {
				insts = append(insts, map[string]any{"id": 100, "account": map[string]string{"login": "acme-org", "type": "Organization"}})
			}
			write(map[string]any{"total_count": len(insts), "installations": insts})
		case p == "/user/installations/100/repositories":
			if auth != "ghu_ana" {
				w.WriteHeader(404)
				return
			}
			write(map[string]any{"total_count": 1, "repositories": []map[string]any{{"id": 555, "full_name": "acme-org/api", "private": true}}})
		case p == "/app/installations/100/access_tokens" && r.Method == "POST":
			if strings.Count(auth, ".") != 2 {
				w.WriteHeader(401)
				return
			}
			f.appTokens++
			write(map[string]any{"token": "ghs_inst", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
		case strings.HasPrefix(p, "/applications/") && r.Method == "DELETE":
			var b struct {
				AccessToken string `json:"access_token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			f.revokedUser = append(f.revokedUser, b.AccessToken)
			w.WriteHeader(204)
		case strings.HasPrefix(p, "/repos/acme-org/api/issues"):
			if auth != "ghs_inst" {
				w.WriteHeader(401)
				return
			}
			rest := strings.Trim(strings.TrimPrefix(p, "/repos/acme-org/api/issues"), "/")
			if rest == "" {
				if r.URL.Query().Get("since") == "" || r.URL.Query().Get("state") != "all" {
					t.Errorf("issue list query: %s", r.URL.RawQuery)
				}
				list := []map[string]any{}
				if r.URL.Query().Get("page") == "1" {
					for _, n := range []int{1, 2} {
						if is, ok := f.issues[n]; ok {
							list = append(list, is)
						}
					}
				}
				write(list)
				return
			}
			num, comments := strings.CutSuffix(rest, "/comments")
			n, _ := strconv.Atoi(num)
			is, ok := f.issues[n]
			if !ok {
				w.WriteHeader(404)
				write(map[string]string{"message": "Not Found"})
				return
			}
			if comments {
				write(f.comments[n])
				return
			}
			write(is)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func githubConfig(srvURL string) func(*config.Config) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return func(c *config.Config) {
		g := &c.Connectors
		g.GitHubAppID, g.GitHubAppSlug, g.GitHubAppPrivateKey = "1234", "memro-test", pemKey
		g.GitHubClientID, g.GitHubClientSecret, g.GitHubWebhookSecret = "gh-client", "gh-secret", "hook-secret"
		g.GitHubAPIURL, g.GitHubWebURL = srvURL, srvURL
	}
}

func (e *connEnv) githubHook(secret, event string, payload map[string]any) (int, []byte) {
	e.t.Helper()
	body := j(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return e.do(nil, "", "POST", "/connectors/github/webhook", body, map[string]string{
		"X-GitHub-Event": event, "X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))})
}

func (e *connEnv) connectGitHub() connectors.Connection {
	e.t.Helper()
	u, cookie := e.authorize("github")
	r := e.callback("github", url.Values{"state": {u.Query().Get("state")}, "code": {"gh-code"},
		"installation_id": {"100"}, "setup_action": {"install"}}, cookie)
	if r.Get("connected") != "1" {
		e.t.Fatalf("github connect: %v", r)
	}
	return e.list().Connections[0]
}

func TestGitHubConnector(t *testing.T) {
	fake, ghSrv := newFakeGitHub(t)
	e := setupConnectors(t, 1000, githubConfig(ghSrv.URL))

	authURL, cookie := e.authorize("github")
	if authURL.Path != "/apps/memro-test/installations/new" || authURL.Query().Get("state") == "" {
		t.Fatalf("authorize URL: %s", authURL)
	}
	// Someone who can't see installation 100 cannot connect it by putting
	// its ID in the callback URL.
	r := e.callback("github", url.Values{"state": {authURL.Query().Get("state")}, "code": {"gh-code-mallory"},
		"installation_id": {"100"}, "setup_action": {"install"}}, cookie)
	if !strings.Contains(r.Get("error"), "doesn't have access") {
		t.Fatalf("forged installation: %v", r)
	}
	if len(e.list().Connections) != 0 {
		t.Fatal("forged installation was connected")
	}

	conn := e.connectGitHub()
	if conn.AccountName != "acme-org" || conn.ExternalUserName != "ana" ||
		conn.ManageURL != ghSrv.URL+"/apps/memro-test/installations/100" {
		t.Fatalf("connection: %+v", conn)
	}
	e.drain()
	text := e.memoryText()
	for _, want := range []string{"[GitHub acme-org/api#1 · Login fails on Safari]", "Issue #1: Login fails on Safari",
		"Labels: bug", "bob commented on", "Reproduced on Safari 18.", "Pull request #2: Add rate limits", "State: merged"} {
		if !strings.Contains(text, want) {
			t.Fatalf("memories missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Build passed") {
		t.Fatal("bot comment synced")
	}
	if c := e.list().Connections[0]; c.SourcesEnabled != 1 || c.Memories != 2 || c.LastSyncedAt == nil {
		t.Fatalf("stats: %+v", c)
	}

	hook := func(event, action string, number int) {
		t.Helper()
		code, body := e.githubHook("hook-secret", event, map[string]any{"action": action,
			"installation": map[string]any{"id": 100}, "repository": map[string]any{"id": 555, "full_name": "acme-org/api"},
			"issue": map[string]any{"number": number}})
		e.must(202, code, body, event+" webhook")
	}
	code, body := e.githubHook("wrong", "issues", map[string]any{"action": "edited"})
	e.must(401, code, body, "bad webhook signature")

	// Edit: the issue is re-read, the old text replaced.
	fake.mu.Lock()
	fake.issues[1]["title"] = "Login fails on Safari 18"
	fake.mu.Unlock()
	hook("issues", "edited", 1)
	e.drain()
	if text = e.memoryText(); !strings.Contains(text, "Issue #1: Login fails on Safari 18\n") || strings.Contains(text, "Safari]") {
		t.Fatalf("after edit:\n%s", text)
	}

	// A long issue spans several memories. One the user deletes stays
	// deleted when another part of the issue changes.
	long := strings.Repeat("The session cookie is dropped after the redirect. ", 120)
	fake.mu.Lock()
	fake.issues[1]["body"] = long
	fake.mu.Unlock()
	hook("issues", "edited", 1)
	e.drain()
	var first domain.Memory
	n := 0
	for _, m := range e.memories() {
		if strings.Contains(m.Content, "acme-org/api#1") {
			n++
			if strings.Contains(m.Content, "Issue #1:") {
				first = m
			}
		}
	}
	if n < 3 || first.ID == uuid.Nil {
		t.Fatalf("long issue stored as %d memories", n)
	}
	resp, b := e.call("DELETE", "/memory/"+first.ID.String(), nil, nil)
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		t.Fatalf("delete memory: %d %s", resp.StatusCode, b)
	}
	fake.mu.Lock()
	fake.comments[1] = append(fake.comments[1], map[string]any{"body": "Fixed in #2.", "created_at": time.Now().UTC().Format(time.RFC3339),
		"user": map[string]string{"login": "ana", "type": "User"}})
	fake.issues[1]["comments"] = 3
	fake.mu.Unlock()
	hook("issue_comment", "created", 1)
	e.drain()
	text = e.memoryText()
	if !strings.Contains(text, "Fixed in #2.") {
		t.Fatalf("new comment missing:\n%s", text)
	}
	if strings.Contains(text, "Issue #1: Login fails") {
		t.Fatal("a memory the user deleted was recreated")
	}

	// Deleted at the source: its memories go too.
	fake.mu.Lock()
	delete(fake.issues, 1)
	fake.mu.Unlock()
	hook("issues", "deleted", 1)
	e.drain()
	if text = e.memoryText(); strings.Contains(text, "acme-org/api#1") || !strings.Contains(text, "acme-org/api#2") {
		t.Fatalf("after delete:\n%s", text)
	}

	// Disconnect keeps memories unless asked; the user token is revoked.
	resp, b = e.call("DELETE", "/connectors/"+conn.ID.String(), nil, nil)
	e.must(200, resp.StatusCode, b, "disconnect")
	if len(fake.revokedUser) != 1 || fake.revokedUser[0] != "ghu_ana" {
		t.Fatalf("revoked: %v", fake.revokedUser)
	}
	if !strings.Contains(e.memoryText(), "acme-org/api#2") {
		t.Fatal("memories deleted without being asked")
	}
}

func TestGitHubUninstallAndQuota(t *testing.T) {
	_, ghSrv := newFakeGitHub(t)
	e := setupConnectors(t, 1, githubConfig(ghSrv.URL))
	conn := e.connectGitHub()
	e.drain()
	// Two items, room for one memory: the connection pauses with a reason.
	c := e.list().Connections[0]
	if c.Status != "paused" || !strings.Contains(c.StatusDetail, "cap") || c.Memories != 1 {
		t.Fatalf("after quota: %+v", c)
	}
	code, body := e.githubHook("hook-secret", "installation", map[string]any{"action": "deleted", "installation": map[string]any{"id": 100}})
	e.must(202, code, body, "uninstall webhook")
	if c := e.list().Connections[0]; c.Status != "revoked" || c.ID != conn.ID {
		t.Fatalf("after uninstall: %+v", c)
	}
}
