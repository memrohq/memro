package dce

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Tests for the engine extensions used by the connector catalog.

func manifestFor(t *testing.T, y string) *Manifest {
	t.Helper()
	m, err := ParseManifestYAML([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func testRuntime(srv *httptest.Server) *Runtime {
	return NewRuntimeWithOptions(srv.Client(), nil, Options{AllowPrivateIPs: true, MaxPages: 10})
}

func TestTemplatesFieldsAndBasicAuth(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = io.WriteString(w, `{"accountId":"a1","displayName":"Ana","issues":[{"id":"10","key":"ENG-1","fields":{"summary":"Fix login","status":{"name":"Done"},"labels":[{"name":"auth"},{"name":"p1"}],"assignee":null,"updated":"2026-10-20T10:00:00.000+0000"}}]}`)
	}))
	defer srv.Close()
	m := manifestFor(t, `
apiVersion: memro.dev/v1
kind: Connector
metadata: {name: test, version: "1"}
spec:
  type: rest
  baseURL: "`+srv.URL+`/{{config.site}}"
  fields:
    - {name: site, label: Site, pattern: "[a-z]+"}
    - {name: email, label: Email}
    - {name: token, label: Token, secret: true}
    - {name: missing, label: Missing, optional: true}
  auth: {type: basic, username: "{{config.email}}", password: "{{config.token}}"}
  verify: {path: /myself, accountId: accountId, accountName: "{{displayName}} ({{accountId}})"}
  operations:
    - name: issues
      method: GET
      path: /search
      queryParams: {jql: "updated >= '{{checkpoint}}'", empty: "{{config.missing}}"}
      response: {itemsPath: issues}
  mapping:
    id: key
    title: "{{key}}: {{fields.summary}}"
    content: |
      {{key}}: {{fields.summary}}
      Status: {{fields.status.name}}
      Labels: {{fields.labels[].name}}
      Assignee: {{fields.assignee.displayName}}
    eventDate: fields.updated
  sync: {checkpoint: fields.updated}
`)
	cfg := map[string]string{"site": "acme", "email": "ana@example.com", "token": "tok-123"}
	rt := testRuntime(srv)
	id, name, err := rt.Verify(context.Background(), Call{Manifest: m, Config: cfg})
	if err != nil || id != "a1" || name != "Ana (a1)" {
		t.Fatalf("verify = %q %q %v", id, name, err)
	}
	if u, p, _ := got.BasicAuth(); u != "ana@example.com" || p != "tok-123" || got.URL.Path != "/acme/myself" {
		t.Fatalf("basic auth %q %q, path %s", u, p, got.URL.Path)
	}

	page, err := rt.Page(context.Background(), Call{Manifest: m, Config: cfg, Checkpoint: "2026-10-01"}, m.Spec.Operations[0])
	if err != nil {
		t.Fatal(err)
	}
	if q := got.URL.Query(); q.Get("jql") != "updated >= '2026-10-01'" || q.Has("empty") {
		t.Fatalf("query = %v (an empty template must drop the parameter)", q)
	}
	it := page.Items[0]
	want := "ENG-1: Fix login\nStatus: Done\nLabels: auth, p1"
	if it.Content != want || it.Title != "ENG-1: Fix login" || it.MemoryType != "semantic" {
		t.Fatalf("content = %q title = %q type = %q", it.Content, it.Title, it.MemoryType)
	}
	if !it.EventDate.Equal(time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)) || page.MaxCheckpoint != "2026-10-20T10:00:00.000+0000" {
		t.Fatalf("date %v checkpoint %q", it.EventDate, page.MaxCheckpoint)
	}

	if err := CheckConfig(m, map[string]string{"site": "evil.com/x", "email": "a", "token": "b"}); err == nil {
		t.Fatal("a site outside the pattern must be refused")
	}
	if err := CheckConfig(m, map[string]string{"site": "acme", "email": "a"}); err == nil {
		t.Fatal("a missing required field must be refused")
	}
}

func TestHostFieldNeedsPattern(t *testing.T) {
	_, err := ParseManifestYAML([]byte(`
apiVersion: memro.dev/v1
kind: Connector
metadata: {name: t}
spec:
  type: rest
  baseURL: "https://{{config.subdomain}}.zendesk.com/api/v2"
  fields: [{name: subdomain, label: Subdomain}, {name: token, label: Token, secret: true}]
  auth: {type: bearer, secretRef: token}
  operations: [{name: o, method: GET, path: /x, response: {itemsPath: x}}]
  mapping: {id: id, content: body}
`))
	if err == nil || !strings.Contains(err.Error(), "needs a pattern") {
		t.Fatalf("err = %v", err)
	}
}

func TestPaginationStyles(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/next":
			if r.URL.Query().Get("p") == "2" {
				_, _ = io.WriteString(w, `{"items":[{"id":"b"}],"next":null}`)
				return
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"a"}],"next":"`+srvURL+`/next?p=2"}`)
		case "/evil":
			_, _ = io.WriteString(w, `{"items":[{"id":"a"}],"next":"https://attacker.example/steal"}`)
		case "/link":
			if r.URL.Query().Get("page") != "2" {
				w.Header().Set("Link", `<`+srvURL+`/link?page=2>; rel="next", <`+srvURL+`/link?page=9>; rel="last"`)
			}
			page := r.URL.Query().Get("page")
			if page == "" {
				page = "1"
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"`+page+`"}]}`)
		case "/page":
			n := r.URL.Query().Get("page")
			if n == "3" {
				_, _ = io.WriteString(w, `{"items":[]}`)
				return
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"p`+n+`"},{"id":"q`+n+`"}]}`)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	m := manifestFor(t, `
apiVersion: memro.dev/v1
kind: Connector
metadata: {name: t}
spec:
  type: rest
  baseURL: "`+srv.URL+`"
  auth: {type: none}
  operations:
    - {name: next, method: GET, path: /next, pagination: {type: nextURL, cursorPath: next}, response: {itemsPath: items}}
    - {name: evil, method: GET, path: /evil, pagination: {type: nextURL, cursorPath: next}, response: {itemsPath: items}}
    - {name: link, method: GET, path: /link, pagination: {type: linkHeader}, response: {itemsPath: items}}
    - {name: page, method: GET, path: /page, pagination: {type: page, limit: 2}, response: {itemsPath: items}}
  mapping: {id: id, content: id}
`)
	rt := testRuntime(srv)
	all := func(op OperationSpec) []string {
		var ids []string
		c := Call{Manifest: m}
		for i := 0; i < 5; i++ {
			p, err := rt.Page(context.Background(), c, op)
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range p.Items {
				ids = append(ids, it.ID)
			}
			if p.NextCursor == "" {
				break
			}
			c.Cursor = p.NextCursor
		}
		return ids
	}
	if got := strings.Join(all(m.Spec.Operations[0]), ","); got != "a,b" {
		t.Errorf("nextURL: %s", got)
	}
	if got := strings.Join(all(m.Spec.Operations[1]), ","); got != "a" {
		t.Errorf("a next URL on another host must not be followed (token leak): %s", got)
	}
	if got := strings.Join(all(m.Spec.Operations[2]), ","); got != "1,2" {
		t.Errorf("linkHeader: %s", got)
	}
	if got := strings.Join(all(m.Spec.Operations[3]), ","); got != "p1,q1,p2,q2" {
		t.Errorf("page: %s", got)
	}
}

func TestGraphQLBodyIsEscaped(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"data":{"items":{"nodes":[{"id":"1"}],"cursor":"c2"}}}`)
	}))
	defer srv.Close()
	m := manifestFor(t, `
apiVersion: memro.dev/v1
kind: Connector
metadata: {name: t}
spec:
  type: rest
  baseURL: "`+srv.URL+`"
  auth: {type: none}
  operations:
    - name: q
      method: POST
      path: /graphql
      body: '{"query":"query($c:String){items(after:$c){nodes{id}}}","variables":{"c":"{{cursor}}"}}'
      pagination: {type: cursor, cursorPath: data.items.cursor}
      response: {itemsPath: data.items.nodes}
  mapping: {id: id, content: id, memoryType: episodic}
`)
	p, err := testRuntime(srv).Page(context.Background(), Call{Manifest: m, Cursor: `x","evil":"1`}, m.Spec.Operations[0])
	if err != nil {
		t.Fatal(err)
	}
	vars := body["variables"].(map[string]any)
	if vars["c"] != `x","evil":"1` || len(vars) != 1 {
		t.Fatalf("a cursor must stay inside its string: %v", body)
	}
	if p.NextCursor != "c2" || p.Items[0].MemoryType != "episodic" {
		t.Fatalf("page = %+v", p)
	}
}

func TestQueryAuthIsRedactedFromErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key "+r.URL.Query().Get("key"), http.StatusUnauthorized)
	}))
	defer srv.Close()
	m := manifestFor(t, `
apiVersion: memro.dev/v1
kind: Connector
metadata: {name: t}
spec:
  type: rest
  baseURL: "`+srv.URL+`"
  fields: [{name: token, label: Token, secret: true}]
  auth: {type: query, header: key, secretRef: token}
  operations: [{name: o, method: GET, path: /x, response: {itemsPath: x}}]
  mapping: {id: id, content: id}
`)
	_, err := testRuntime(srv).Page(context.Background(), Call{Manifest: m, Config: map[string]string{"token": "sekrit-value"}}, m.Spec.Operations[0])
	var he *HTTPError
	if err == nil || strings.Contains(err.Error(), "sekrit-value") || errors.As(err, &he) {
		t.Fatalf("err = %v (must be redacted)", err)
	}
}

func TestSafeClientRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := SafeClient(false).Get(srv.URL) // 127.0.0.1
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("connecting to loopback must be refused at dial time: %v", err)
	}
	if err := ValidateDestinationURL("http://example.com/x", false); err == nil {
		t.Fatal("plain http to a third-party API must be refused")
	}
}

func TestParseDateAndCheckpoints(t *testing.T) {
	for in, want := range map[string]time.Time{
		"2026-10-20T10:00:00Z":          time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
		"2026-10-20T15:30:00.000+0530":  time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
		"1792483200":                    time.Unix(1792483200, 0).UTC(),
		"1792483200000":                 time.Unix(1792483200, 0).UTC(),
		"2026-10-20":                    time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		"Tue, 20 Oct 2026 10:00:00 GMT": time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
	} {
		if got := ParseDate(in); !got.Equal(want) {
			t.Errorf("ParseDate(%q) = %v, want %v", in, got, want)
		}
	}
	if got := laterCheckpoint("2026-10-20T10:00:00Z", "2026-10-19T10:00:00Z"); got != "2026-10-20T10:00:00Z" {
		t.Errorf("later = %s", got)
	}
	if ExtractString(map[string]any{"n": 1234567890123.0}, "n") != "1234567890123" {
		t.Error("large numbers must not use exponents")
	}
	if ExtractString(map[string]any{"a": []any{map[string]any{"b": "x"}, map[string]any{"b": "y"}}}, "a.1.b") != "y" {
		t.Error("array index")
	}
}
