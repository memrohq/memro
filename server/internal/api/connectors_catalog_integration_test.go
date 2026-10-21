package api_test

// End-to-end tests of catalog (token) connectors against a fake API:
// connecting checks the token and never echoes or stores it in clear, a sync
// pages across several jobs, a later pass starts from the checkpoint (not
// from it as a page cursor), a refused token revokes the connection, and one
// source never syncs twice at once.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/internal/connectors"
	"github.com/memrohq/memro/server/internal/connectors/dce"
)

// fakeCatalogAPI serves a paged list of items to one valid token.
type fakeCatalogAPI struct {
	mu       sync.Mutex
	token    string
	items    int       // how many items exist
	updated  time.Time // every item's last change
	requests []url2    // item list requests seen
}

type url2 struct{ cursor, since, team string }

func (f *fakeCatalogAPI) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
		return
	}
	switch {
	case r.URL.Path == "/me":
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "u1", "name": "Ana's workspace"})
	case strings.HasPrefix(r.URL.Path, "/teams/") && strings.HasSuffix(r.URL.Path, "/items"):
		q := r.URL.Query()
		f.requests = append(f.requests, url2{cursor: q.Get("cursor"), since: q.Get("since"), team: strings.Split(r.URL.Path, "/")[2]})
		start, _ := strconv.Atoi(q.Get("cursor"))
		var items []map[string]any
		for i := start; i < min(start+2, f.items); i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("it-%d", i), "title": fmt.Sprintf("Item %d", i),
				"body": fmt.Sprintf("Body of item %d", i), "updated": f.updated.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)})
		}
		next := ""
		if start+2 < f.items {
			next = strconv.Itoa(start + 2)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "next": next})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func fakeManifest(t *testing.T, baseURL string) *dce.Manifest {
	t.Helper()
	m, err := dce.ParseManifestYAML([]byte(`
apiVersion: memro.dev/v1
kind: Connector
metadata: {name: fakeapp, version: "1", displayName: Fake App, category: work, description: Test items, docsURL: "https://example.com/docs"}
spec:
  type: rest
  baseURL: "` + baseURL + `"
  fields:
    - {name: token, label: API token, secret: true, help: Create one in settings.}
    - {name: team, label: Team, pattern: "[a-z]+"}
  auth: {type: bearer, secretRef: token}
  verify: {path: /me, accountId: id, accountName: name}
  operations:
    - name: items
      method: GET
      path: "/teams/{{config.team}}/items"
      queryParams: {since: "{{checkpoint}}"}
      pagination: {type: cursor, cursorParam: cursor, cursorPath: next}
      response: {itemsPath: items}
  mapping:
    id: id
    title: "{{title}}"
    content: |
      {{title}}
      {{body}}
    eventDate: updated
  sync: {mode: incremental, checkpoint: updated}
`))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func setupCatalog(t *testing.T, f *fakeCatalogAPI) *connEnv {
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return setupConnectorsDeps(t, 1000, func(*config.Config) {}, func(d *connectors.Deps) {
		d.Catalog = []*dce.Manifest{fakeManifest(t, srv.URL)}
		d.CatalogClient = srv.Client()
	})
}

func (e *connEnv) connectToken(body any) (int, []byte) {
	e.t.Helper()
	resp, b := e.call("POST", "/connectors/fakeapp/token", body, nil)
	return resp.StatusCode, b
}

func TestCatalogConnectorEndToEnd(t *testing.T) {
	f := &fakeCatalogAPI{token: "tok-good-123456", items: 13, updated: time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)}
	e := setupCatalog(t, f)
	ctx := context.Background()

	// The catalog entry describes its connect form.
	var info connectors.ProviderInfo
	for _, p := range e.list().Providers {
		if p.Name == "fakeapp" {
			info = p
		}
	}
	if info.Kind != "token" || len(info.Fields) != 2 || !info.Fields[0].Secret || info.DisplayName != "Fake App" {
		t.Fatalf("provider info = %+v", info)
	}

	// A wrong token or a bad field is refused with a message, and nothing is saved.
	if code, b := e.connectToken(map[string]any{"fields": map[string]string{"token": "nope", "team": "eng"}}); code != 400 ||
		!strings.Contains(string(b), "refused the token") {
		t.Fatalf("bad token: %d %s", code, b)
	}
	if code, b := e.connectToken(map[string]any{"fields": map[string]string{"token": f.token, "team": "../x"}}); code != 400 ||
		!strings.Contains(string(b), "Team") {
		t.Fatalf("bad field: %d %s", code, b)
	}
	if n := len(e.list().Connections); n != 0 {
		t.Fatalf("%d connections after refused attempts", n)
	}

	// The right token connects; it is never returned or stored in clear.
	code, b := e.connectToken(map[string]any{"fields": map[string]string{"token": f.token, "team": "eng"}})
	if code != 201 || strings.Contains(string(b), f.token) {
		t.Fatalf("connect: %d %s", code, b)
	}
	var conn connectors.Connection
	_ = json.Unmarshal(b, &conn)
	if conn.AccountName != "Ana's workspace" {
		t.Fatalf("account name = %q", conn.AccountName)
	}
	var sealed []byte
	if err := e.st.Pool.QueryRow(ctx, `SELECT credentials FROM connector_connections WHERE id = $1`, conn.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(f.token)) {
		t.Fatal("token stored in clear")
	}
	if _, lb := e.call("GET", "/connectors", nil, nil); strings.Contains(string(lb), f.token) {
		t.Fatal("token returned by the list")
	}

	// The first sync reads all 7 pages, more than one job's worth (5).
	e.drain()
	if n := len(e.memories()); n != 13 {
		t.Fatalf("%d memories after the first sync, want 13", n)
	}
	if !strings.Contains(e.memoryText(), "[Fake App") || !strings.Contains(e.memoryText(), "Body of item 12") {
		t.Fatalf("memory text:\n%s", e.memoryText())
	}
	if len(f.requests) != 7 || f.requests[0].since != "" || f.requests[0].cursor != "" || f.requests[5].cursor != "10" {
		t.Fatalf("first pass requests = %+v", f.requests)
	}

	// A second pass starts from page one with the checkpoint as a filter.
	f.requests = nil
	if resp, b := e.call("POST", "/connectors/"+conn.ID.String()+"/sync", nil, nil); resp.StatusCode != 202 {
		t.Fatalf("sync now: %d %s", resp.StatusCode, b)
	}
	e.drain()
	want := f.updated.Add(12 * time.Minute).Format(time.RFC3339)
	if len(f.requests) == 0 || f.requests[0].cursor != "" || f.requests[0].since != want {
		t.Fatalf("second pass first request = %+v, want since=%s and no cursor", f.requests, want)
	}
	if n := len(e.memories()); n != 13 {
		t.Fatalf("%d memories after an unchanged resync, want 13", n)
	}

	// A refused token revokes the connection and drops its jobs.
	f.mu.Lock()
	f.token = "rotated"
	f.mu.Unlock()
	e.call("POST", "/connectors/"+conn.ID.String()+"/sync", nil, nil)
	e.drain()
	l := e.list()
	if len(l.Connections) != 1 || l.Connections[0].Status != connectors.StatusRevoked ||
		!strings.Contains(l.Connections[0].StatusDetail, "Fake App") {
		t.Fatalf("after 401: %+v", l.Connections)
	}

	// An older dashboard that sends the token alone still connects.
	f.mu.Lock()
	f.token = "tok-new-987654"
	f.mu.Unlock()
	code, b = e.connectToken(map[string]any{"token": f.token, "fields": map[string]string{"team": "eng"}})
	if code != 201 {
		t.Fatalf("legacy token connect: %d %s", code, b)
	}
	if l := e.list(); l.Connections[0].Status != connectors.StatusActive {
		t.Fatalf("reconnect did not reactivate: %+v", l.Connections)
	}
}

func TestCatalogSourceNeverSyncsTwiceAtOnce(t *testing.T) {
	f := &fakeCatalogAPI{token: "tok-good-123456", items: 2, updated: time.Now().Add(-time.Hour)}
	e := setupCatalog(t, f)
	ctx := context.Background()
	code, b := e.connectToken(map[string]any{"fields": map[string]string{"token": f.token, "team": "eng"}})
	if code != 201 {
		t.Fatalf("connect: %d %s", code, b)
	}
	var conn connectors.Connection
	_ = json.Unmarshal(b, &conn)
	e.drain()

	// One worker is syncing "all" (leased); a fresh copy of the same job must wait.
	if _, err := e.st.Pool.Exec(ctx, `INSERT INTO connector_jobs (connection_id, kind, key, run_at, locked_until)
		VALUES ($1, 'sync', 'all', NOW() - INTERVAL '1 minute', NOW() + INTERVAL '5 minutes')`, conn.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Pool.Exec(ctx, `INSERT INTO connector_jobs (connection_id, kind, key, run_at)
		VALUES ($1, 'sync', 'all', NOW() - INTERVAL '1 minute')`, conn.ID); err != nil {
		t.Fatal(err)
	}
	before := len(f.requests)
	if err := e.mgr.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var waiting, attempts int
	_ = e.st.Pool.QueryRow(ctx, `SELECT COUNT(*), COALESCE(MAX(attempts), 0) FROM connector_jobs
		WHERE connection_id = $1 AND locked_until IS NULL`, conn.ID).Scan(&waiting, &attempts)
	if waiting != 1 || attempts != 0 || len(f.requests) != before {
		t.Fatalf("the duplicate ran while the first was leased: waiting=%d attempts=%d requests=%d->%d", waiting, attempts, before, len(f.requests))
	}
}
