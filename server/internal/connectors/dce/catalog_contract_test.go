package dce

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Contract tests: the exact requests a few catalog connectors must send,
// taken from each service's API documentation, and the engine behaviour
// they rely on.

func catalogManifest(t *testing.T, name string) *Manifest {
	t.Helper()
	ms, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.Metadata.Name == name {
			return m
		}
	}
	t.Fatalf("no catalog connector %q", name)
	return nil
}

// capture runs one page of op 0 (or verify, when verify is true) against a
// server answering body, and returns the request it saw and its body.
func capture(t *testing.T, name string, cfg map[string]string, cursor, body string, verify bool) (*http.Request, string, *PageResult) {
	t.Helper()
	var got *http.Request
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		got, sent = r, string(b)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	m := pointAt(catalogManifest(t, name), srv)
	rt := NewRuntimeWithOptions(srv.Client(), nil, Options{AllowPrivateIPs: true})
	call := Call{Manifest: m, Config: cfg, Cursor: cursor, SourceID: "src1"}
	if verify {
		if _, _, err := rt.Verify(context.Background(), call); err != nil {
			t.Fatal(err)
		}
		return got, sent, nil
	}
	page, err := rt.Page(context.Background(), call, m.Spec.Operations[0])
	if err != nil {
		t.Fatal(err)
	}
	return got, sent, page
}

func TestContractAuthHeaders(t *testing.T) {
	cases := []struct {
		name, header, want string
		cfg                map[string]string
	}{
		{"pagerduty", "Authorization", "Token token=k1", map[string]string{"token": "k1"}},
		{"opsgenie", "Authorization", "GenieKey k1", map[string]string{"token": "k1"}},
		{"readwise", "Authorization", "Token k1", map[string]string{"token": "k1"}},
		{"circleci", "Circle-Token", "k1", map[string]string{"token": "k1", "orgSlug": "gh/acme"}},
		{"clickup", "Authorization", "k1", map[string]string{"token": "k1"}},
		{"shortcut", "Shortcut-Token", "k1", map[string]string{"token": "k1"}},
		{"postman", "X-Api-Key", "k1", map[string]string{"token": "k1"}},
		{"pipedrive", "X-Api-Token", "k1", map[string]string{"token": "k1"}},
		{"snyk", "Authorization", "token k1", map[string]string{"token": "k1", "orgId": "0f8e3c1a-5b7d-4e8a-9c21-7d4e5f6a8b90", "host": "api.snyk.io"}},
		{"discord", "Authorization", "Bot k1", map[string]string{"token": "k1", "guildId": "123456789012345678"}},
		{"zotero", "Zotero-Api-Key", "k1", map[string]string{"token": "k1", "userId": "42"}},
		{"plane", "X-Api-Key", "k1", map[string]string{"token": "k1", "workspace": "acme", "host": "api.plane.so"}},
		{"todoist", "Authorization", "Bearer k1", map[string]string{"token": "k1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _, _ := capture(t, c.name, c.cfg, "", `{}`, true)
			if got := req.Header.Get(c.header); got != c.want {
				t.Fatalf("%s = %q, want %q", c.header, got, c.want)
			}
		})
	}
}

func TestContractBasicAuthAndExtraHeaders(t *testing.T) {
	req, _, _ := capture(t, "zendesk", map[string]string{"subdomain": "acme", "email": "a@x.co", "token": "t1"}, "", `{}`, true)
	if u, p, _ := req.BasicAuth(); u != "a@x.co/token" || p != "t1" {
		t.Errorf("zendesk basic auth = %q %q", u, p)
	}
	req, _, _ = capture(t, "close", map[string]string{"token": "t1"}, "", `{}`, true)
	if u, p, _ := req.BasicAuth(); u != "t1" || p != "" {
		t.Errorf("close basic auth = %q %q", u, p)
	}
	req, _, _ = capture(t, "freshdesk", map[string]string{"domain": "acme", "token": "t1"}, "", `{}`, true)
	if u, p, _ := req.BasicAuth(); u != "t1" || p != "X" {
		t.Errorf("freshdesk basic auth = %q %q", u, p)
	}
	req, _, _ = capture(t, "datadog", map[string]string{"site": "datadoghq.eu", "apiKey": "a1", "appKey": "b2"}, "", `[]`, true)
	if req.Header.Get("DD-API-KEY") != "a1" || req.Header.Get("DD-APPLICATION-KEY") != "b2" {
		t.Errorf("datadog keys = %q %q", req.Header.Get("DD-API-KEY"), req.Header.Get("DD-APPLICATION-KEY"))
	}
	req, _, _ = capture(t, "harvest", map[string]string{"token": "t1", "accountId": "777"}, "", `{}`, true)
	if req.Header.Get("Harvest-Account-Id") != "777" || req.Header.Get("Authorization") != "Bearer t1" {
		t.Errorf("harvest headers = %v", req.Header)
	}
	req, _, _ = capture(t, "trello", map[string]string{"apiKey": "k1", "token": "t1"}, "", `{}`, true)
	if q := req.URL.Query(); q.Get("key") != "k1" || q.Get("token") != "t1" {
		t.Errorf("trello query = %v", req.URL.RawQuery)
	}
}

func TestContractPaginationShapes(t *testing.T) {
	// Datadog numbers pages from 0.
	req, _, _ := capture(t, "datadog", map[string]string{"site": "datadoghq.com", "apiKey": "a", "appKey": "b"}, "", `[]`, false)
	if req.URL.Query().Get("page") != "0" {
		t.Errorf("datadog first page = %q, want 0", req.URL.Query().Get("page"))
	}
	// Render's next cursor is the last item's cursor.
	_, _, page := capture(t, "render", map[string]string{"token": "t"}, "",
		`[{"cursor":"c1","deploy":{"id":"d1","status":"live","createdAt":"2026-10-20T10:00:00Z"}},{"cursor":"c2","deploy":{"id":"d2","status":"live","createdAt":"2026-10-20T11:00:00Z"}}]`, false)
	if page.NextCursor != "c2" || len(page.Items) != 2 || page.MaxCheckpoint != "2026-10-20T11:00:00Z" {
		t.Errorf("render page = %+v", page)
	}
	// Outline pages through a JSON body offset.
	_, body, _ := capture(t, "outline", map[string]string{"host": "docs.acme.com", "token": "t"}, "2", `{"data":[]}`, false)
	var b map[string]any
	if err := json.Unmarshal([]byte(body), &b); err != nil || b["offset"] != float64(200) {
		t.Errorf("outline body = %s", body)
	}
	// GraphQL cursors: null on the first page, a string after.
	_, body, _ = capture(t, "monday", map[string]string{"token": "t"}, "", `{"data":{"boards":[{"items_page":{"items":[]}}]}}`, false)
	if !strings.Contains(body, `"cursor":null`) || !strings.Contains(body, `"board":["src1"]`) || !json.Valid([]byte(body)) {
		t.Errorf("monday first body = %s", body)
	}
	_, body, _ = capture(t, "newrelic", map[string]string{"token": "t", "accountId": "123", "host": "api.newrelic.com"}, `abc"x`, `{}`, false)
	if !strings.Contains(body, `"cursor":"abc\"x"`) || !strings.Contains(body, `"account":123`) || !json.Valid([]byte(body)) {
		t.Errorf("newrelic body = %s", body)
	}
	// Shortcut needs a query even on the first sync.
	req, _, _ = capture(t, "shortcut", map[string]string{"token": "t"}, "", `{"data":[]}`, false)
	if q := req.URL.Query().Get("query"); q != "updated:2000-01-01..*" {
		t.Errorf("shortcut query = %q", q)
	}
}

func TestContractIncrementalFilters(t *testing.T) {
	run := func(name string, cfg map[string]string, checkpoint, param, want string) {
		t.Helper()
		var got *http.Request
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()
		m := pointAt(catalogManifest(t, name), srv)
		rt := NewRuntimeWithOptions(srv.Client(), nil, Options{AllowPrivateIPs: true})
		if _, err := rt.Page(context.Background(), Call{Manifest: m, Config: cfg, Checkpoint: checkpoint, SourceID: "s"}, m.Spec.Operations[0]); err != nil {
			t.Fatal(err)
		}
		if v := got.URL.Query().Get(param); v != want {
			t.Errorf("%s: %s = %q, want %q", name, param, v, want)
		}
	}
	run("asana", map[string]string{"token": "t"}, "2026-10-01T00:00:00Z", "modified_since", "2026-10-01T00:00:00Z")
	run("asana", map[string]string{"token": "t"}, "", "modified_since", "")
	run("bitbucket", map[string]string{"email": "a@x.co", "token": "t", "workspace": "acme"}, "2026-10-01T00:00:00Z", "q", "updated_on > 2026-10-01T00:00:00Z")
	run("clickup", map[string]string{"token": "t"}, "1760000000000", "date_updated_gt", "1760000000000")
	run("shortcut", map[string]string{"token": "t"}, "2026-10-01T08:30:00Z", "query", "updated:2026-10-01..*")
	run("freshdesk", map[string]string{"domain": "acme", "token": "t"}, "", "updated_since", "2000-01-01T00:00:00Z")
	run("harvest", map[string]string{"token": "t", "accountId": "1"}, "2026-10-01T00:00:00Z", "updated_since", "2026-10-01T00:00:00Z")
}

func TestContractDiscordKeepsTextChannelsOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"1","name":"general","type":0},{"id":"2","name":"Voice","type":2},{"id":"3","name":"news","type":5},{"id":"4","name":"Category","type":4}]`))
	}))
	defer srv.Close()
	m := pointAt(catalogManifest(t, "discord"), srv)
	rt := NewRuntimeWithOptions(srv.Client(), nil, Options{AllowPrivateIPs: true})
	list, err := rt.ListSources(context.Background(), Call{Manifest: m, Config: map[string]string{"token": "t", "guildId": "123456789012345678"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "general" || list[1].Name != "news" {
		t.Fatalf("sources = %+v", list)
	}
}

func TestContractConfluenceBodyIsText(t *testing.T) {
	_, _, page := capture(t, "confluence", map[string]string{"site": "acme", "email": "a@x.co", "token": "t"}, "",
		`{"results":[{"id":"9","title":"Runbook","version":{"createdAt":"2026-10-20T10:00:00.000Z"},"body":{"storage":{"value":"<h1>Deploy</h1><p>Run <code>make</code> &amp; wait.</p><ul><li>one</li><li>two</li></ul><script>x()</script>"}}}]}`, false)
	want := "Runbook\n\nDeploy\nRun make & wait.\n- one\n- two"
	if len(page.Items) != 1 || page.Items[0].Content != want {
		t.Fatalf("content = %q, want %q", page.Items[0].Content, want)
	}
}

func TestContractAirtableFieldsAreStable(t *testing.T) {
	body := `{"records":[{"id":"rec1","createdTime":"2026-10-20T10:00:00.000Z","fields":{"Status":"Open","Name":"Launch","Owner":{"name":"Ana"},"Tags":["a","b"],"Count":3}}]}`
	_, _, p1 := capture(t, "airtable", map[string]string{"token": "t", "baseId": "appABCDEFGHIJ1234"}, "", body, false)
	_, _, p2 := capture(t, "airtable", map[string]string{"token": "t", "baseId": "appABCDEFGHIJ1234"}, "", body, false)
	want := "Count: 3\nName: Launch\nOwner: Ana\nStatus: Open\nTags: a, b"
	if p1.Items[0].Content != want || p2.Items[0].Content != want || p1.Items[0].Title != "Launch" {
		t.Fatalf("content = %q / %q", p1.Items[0].Content, p2.Items[0].Content)
	}
}

func TestContractMondayColumnsAndReadwiseHighlights(t *testing.T) {
	_, _, page := capture(t, "monday", map[string]string{"token": "t"}, "",
		`{"data":{"boards":[{"items_page":{"cursor":"next1","items":[{"id":"5","name":"Launch","updated_at":"2026-10-20T10:00:00Z","group":{"title":"Q4"},"column_values":[{"column":{"title":"Status"},"text":"Working on it"},{"column":{"title":"Owner"},"text":"Ana"},{"column":{"title":"Empty"},"text":""}]}]}}]}}`, false)
	if page.NextCursor != "next1" || !strings.Contains(page.Items[0].Content, "Status: Working on it\nOwner: Ana") ||
		strings.Contains(page.Items[0].Content, "Empty") {
		t.Fatalf("monday = %+v %q", page, page.Items[0].Content)
	}
	_, _, page = capture(t, "readwise", map[string]string{"token": "t"}, "",
		`{"results":[{"user_book_id":7,"readable_title":"Deep Work","author":"Newport","category":"books","highlights":[{"text":"Focus.","note":"","highlighted_at":"2026-01-01T00:00:00Z"},{"text":"Depth.","note":"key","highlighted_at":"2026-02-01T00:00:00Z"}]}]}`, false)
	c := page.Items[0].Content
	if !strings.Contains(c, "- Focus.") || !strings.Contains(c, "- Depth.\n  Note: key") || page.Items[0].EventDate.Month() != 2 {
		t.Fatalf("readwise = %q", c)
	}
}

func TestRetriesResendTheBody(t *testing.T) {
	var calls atomic.Int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		bodies = append(bodies, string(b))
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"boards":[{"items_page":{"items":[]}}]}}`))
	}))
	defer srv.Close()
	m := pointAt(catalogManifest(t, "monday"), srv)
	rt := NewRuntimeWithOptions(srv.Client(), nil, Options{AllowPrivateIPs: true})
	if _, err := rt.Page(context.Background(), Call{Manifest: m, Config: map[string]string{"token": "t"}, SourceID: "1"}, m.Spec.Operations[0]); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Fatalf("bodies = %q", bodies)
	}
}

func TestErrorsNeverCarryTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid key k-secret-123 for token t-secret-456"}`))
	}))
	defer srv.Close()
	m := pointAt(catalogManifest(t, "trello"), srv)
	rt := NewRuntimeWithOptions(srv.Client(), nil, Options{AllowPrivateIPs: true})
	_, _, err := rt.Verify(context.Background(), Call{Manifest: m, Config: map[string]string{"apiKey": "k-secret-123", "token": "t-secret-456"}})
	var he *HTTPError
	if !errors.As(err, &he) && err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "k-secret-123") || strings.Contains(err.Error(), "t-secret-456") {
		t.Fatalf("error leaks a secret: %v", err)
	}
}

func TestRestrictedAddresses(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1",
		"0.0.0.0", "198.18.0.1", "::1", "fc00::1", "fe80::1", "64:ff9b::a00:1", "2002:a00:1::", "::ffff:10.0.0.1", "224.0.0.1"} {
		if !isPrivateOrRestrictedIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "151.101.1.69", "2606:4700::1111"} {
		if isPrivateOrRestrictedIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestSafeClientRefusesRedirectsToOtherHosts(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("credentials followed a redirect to another host: %v", r.Header)
	}))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/steal", http.StatusFound)
	}))
	defer first.Close()
	req, _ := http.NewRequest(http.MethodGet, first.URL, nil)
	req.Header.Set("Authorization", "Bearer secret")
	if _, err := SafeClient(true).Do(req); err == nil || !strings.Contains(err.Error(), "another host") {
		t.Fatalf("err = %v", err)
	}
	// Without private addresses allowed, a loopback target is refused when connecting.
	if _, err := SafeClient(false).Get(first.URL); err == nil || !strings.Contains(err.Error(), "SSRF") {
		t.Fatalf("loopback err = %v", err)
	}
}

func TestLaterCheckpointComparesNumbersAsNumbers(t *testing.T) {
	if got := laterCheckpoint("9", "10"); got != "10" {
		t.Errorf("laterCheckpoint(9, 10) = %s", got)
	}
	if got := laterCheckpoint("2026-10-02T00:00:00Z", "2026-10-01T23:00:00+02:00"); got != "2026-10-02T00:00:00Z" {
		t.Errorf("dates: %s", got)
	}
}
