package dce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Every catalog manifest is exercised against a fake API shaped by the
// manifest itself: the connect check, the source list and each operation
// must send the credentials, render their templates completely, decode the
// items and produce a usable memory (an ID, text, a date, a checkpoint).

func TestCatalogLoads(t *testing.T) {
	ms, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) < 40 {
		t.Fatalf("catalog has %d connectors, want at least 40", len(ms))
	}
	seen := map[string]bool{}
	for _, m := range ms {
		if seen[m.Metadata.DisplayName] {
			t.Errorf("display name %q used twice", m.Metadata.DisplayName)
		}
		seen[m.Metadata.DisplayName] = true
		if m.Metadata.Description == "" {
			t.Errorf("%s: no description", m.Metadata.Name)
		}
		for _, f := range m.Spec.Fields {
			if f.Secret && f.Help == "" {
				t.Errorf("%s: secret field %q needs help saying where to create it", m.Metadata.Name, f.Name)
			}
		}
	}
}

// sampleConfig fills every field with a value its pattern accepts.
func sampleConfig(t *testing.T, m *Manifest) map[string]string {
	t.Helper()
	known := map[string]string{
		"orgId": "0f8e3c1a-5b7d-4e8a-9c21-7d4e5f6a8b90", "baseId": "appABCDEFGHIJ1234",
		"guildId": "123456789012345678", "accountId": "12345", "userId": "12345",
		"orgSlug": "gh/acme", "teamId": "team_abc123", "organization": "acme",
		"email": "ana@example.com", "host": "acme.example.com",
	}
	cfg := map[string]string{}
	for _, f := range m.Spec.Fields {
		v := known[f.Name]
		switch {
		case f.Secret:
			v = "secret-" + f.Name + "-1234"
		case v == "" && f.Optional:
			continue
		case v == "":
			v = "acme"
		}
		if f.Pattern != "" {
			re := regexp.MustCompile("^(?:" + f.Pattern + ")$")
			for _, cand := range []string{v, "acme", "12345", firstAlternative(f.Pattern)} {
				if re.MatchString(cand) {
					v = cand
					break
				}
			}
		}
		cfg[f.Name] = v
	}
	if err := CheckConfig(m, cfg); err != nil {
		t.Fatalf("%s: sample config rejected: %v", m.Metadata.Name, err)
	}
	return cfg
}

// firstAlternative turns "a\.b\.c|d" into "a.b.c".
func firstAlternative(p string) string {
	return strings.ReplaceAll(strings.Split(p, "|")[0], `\.`, ".")
}

// pointAt sends the manifest's requests to srv, keeping the base URL's path.
func pointAt(m *Manifest, srv *httptest.Server) *Manifest {
	c := *m
	base := m.Spec.BaseURL
	if i := strings.Index(base, "://"); i >= 0 {
		base = base[i+3:]
	}
	path := ""
	if i := strings.Index(base, "/"); i >= 0 {
		path = base[i:]
	}
	c.Spec.BaseURL = srv.URL + path
	return &c
}

var pathRef = regexp.MustCompile(`\{\{\s*([^{}#/][^{}]*?)\s*\}\}`)

// setPath writes v at a dotted path, creating objects and one-element arrays
// ("a[].b", "a.0.b", "a.-1.b"). "$" or "" is the root.
func setPath(root map[string]any, path string, v any) {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "$."), "$")
	if path == "" {
		return
	}
	if before, after, ok := strings.Cut(path, "[]"); ok {
		after = strings.TrimPrefix(after, ".")
		el := map[string]any{}
		var item any = el
		if after == "" {
			item = v
		} else {
			setPath(el, after, v)
		}
		setSlice(root, before, item)
		return
	}
	parts := strings.Split(path, ".")
	cur := root
	for i, p := range parts {
		last := i == len(parts)-1
		if i+1 < len(parts) && isIndex(parts[i+1]) {
			el := map[string]any{}
			if existing, ok := cur[p].([]any); ok && len(existing) > 0 {
				if m, ok := existing[0].(map[string]any); ok {
					el = m
				}
			}
			cur[p] = []any{el}
			rest := strings.Join(parts[i+2:], ".")
			if rest == "" {
				cur[p] = []any{v}
				return
			}
			setPath(el, rest, v)
			return
		}
		if last {
			if _, exists := cur[p]; !exists {
				cur[p] = v
			}
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
}

func setSlice(root map[string]any, path string, item any) {
	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	cur := root
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	last := parts[len(parts)-1]
	if existing, ok := cur[last].([]any); ok && len(existing) > 0 {
		if dst, ok := existing[0].(map[string]any); ok {
			if src, ok := item.(map[string]any); ok {
				for k, v := range src {
					dst[k] = v
				}
				return
			}
		}
	}
	cur[last] = []any{item}
}

func isIndex(s string) bool {
	return regexp.MustCompile(`^-?[0-9]+$`).MatchString(s)
}

// refs lists the item paths a mapping value reads.
func refs(tpl string) []string {
	if tpl == "" {
		return nil
	}
	if !strings.Contains(tpl, "{{") {
		return []string{tpl}
	}
	var out []string
	for _, b := range eachBlock.FindAllStringSubmatch(tpl, -1) {
		for _, inner := range refs(b[2]) {
			out = append(out, strings.TrimSpace(b[1])+"[]."+inner)
		}
	}
	tpl = eachBlock.ReplaceAllString(tpl, "")
	for _, m := range pathRef.FindAllStringSubmatch(tpl, -1) {
		out = append(out, parseExpr(m[1]).name)
	}
	return out
}

const sampleDate = "2026-10-20T10:00:00Z"

// sampleItem has a value at every path the mapping reads.
func sampleItem(mp MappingSpec, checkpoint string) map[string]any {
	item := map[string]any{}
	for _, tpl := range []string{mp.ID, mp.Title, mp.Content, mp.AuthorID, mp.AuthorName, mp.URL} {
		for _, p := range refs(tpl) {
			setPath(item, p, "v-"+strings.NewReplacer(".", "-", "[]", "").Replace(p))
		}
	}
	for _, p := range append(refs(mp.EventDate), checkpoint) {
		if p != "" {
			// Dates override the text samples set above at the same path.
			setPathForce(item, p, sampleDate)
		}
	}
	return item
}

func setPathForce(root map[string]any, path string, v any) {
	if !strings.Contains(path, ".") && !strings.Contains(path, "[]") {
		root[path] = v
		return
	}
	setPath(root, path, v)
	// setPath keeps existing leaves; overwrite the leaf explicitly.
	if before, after, ok := strings.Cut(path, "."); ok && !isIndex(after) && !strings.Contains(before, "[]") {
		if next, ok := root[before].(map[string]any); ok {
			setPathForce(next, after, v)
		}
	}
}

// respond wraps items at itemsPath; the root itself when it is "$".
func respond(itemsPath string, items []any) any {
	if p := strings.TrimPrefix(strings.TrimPrefix(itemsPath, "$."), "$"); p == "" {
		return items
	}
	root := map[string]any{}
	setPath(root, itemsPath+".0", nil)
	cur := root
	parts := strings.Split(strings.TrimPrefix(itemsPath, "$."), ".")
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = items
			break
		}
		if i+1 < len(parts) && isIndex(parts[i+1]) {
			el := map[string]any{}
			cur[p] = []any{el}
			rest := strings.Join(parts[i+2:], ".")
			sub := respond(rest, items).(map[string]any)
			for k, v := range sub {
				el[k] = v
			}
			return root
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	return root
}

// recorder is a fake API that answers every request with one body and
// records what it was sent.
type recorder struct {
	srv  *httptest.Server
	body any
	reqs []*http.Request
	sent []string
}

func newRecorder(t *testing.T) *recorder {
	r := &recorder{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.reqs = append(r.reqs, req)
		r.sent = append(r.sent, string(b))
		_ = json.NewEncoder(w).Encode(r.body)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

// checkRequest asserts the last request carried the token and, when it had
// a body, valid JSON with every placeholder filled.
func checkRequest(t *testing.T, name string, m *Manifest, cfg map[string]string, r *recorder) {
	t.Helper()
	req, body := r.reqs[len(r.reqs)-1], r.sent[len(r.sent)-1]
	token := cfg[m.Spec.Auth.SecretRef]
	found := strings.Contains(req.URL.RawQuery, token)
	for _, vals := range req.Header {
		for _, v := range vals {
			found = found || strings.Contains(v, token)
			if after, ok := strings.CutPrefix(v, "Basic "); ok {
				raw, _ := base64.StdEncoding.DecodeString(after)
				found = found || strings.Contains(string(raw), token)
			}
		}
	}
	if !found {
		t.Errorf("%s: request to %s does not carry the token", name, req.URL.Path)
	}
	if strings.Contains(req.URL.String(), "{{") || strings.Contains(req.URL.String(), "%7B%7B") {
		t.Errorf("%s: unrendered template in %s", name, req.URL)
	}
	if body != "" {
		if !json.Valid([]byte(body)) {
			t.Errorf("%s: request body is not JSON: %s", name, body)
		}
		if strings.Contains(body, "{{") {
			t.Errorf("%s: unrendered template in body: %s", name, body)
		}
	}
	for k, vals := range req.Header {
		for _, v := range vals {
			if strings.Contains(v, "{{") {
				t.Errorf("%s: unrendered template in header %s", name, k)
			}
		}
	}
}

func TestCatalogConnectorsAgainstFakeAPIs(t *testing.T) {
	ms, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, orig := range ms {
		t.Run(orig.Metadata.Name, func(t *testing.T) {
			ctx := context.Background()
			cfg := sampleConfig(t, orig)
			r := newRecorder(t)
			m := pointAt(orig, r.srv)
			rt := NewRuntimeWithOptions(r.srv.Client(), nil, Options{AllowPrivateIPs: true, MaxPages: 3})

			// Connect: the verify request names the account.
			v := m.Spec.Verify
			acct := map[string]any{}
			for _, p := range append(refs(v.AccountID), refs(v.AccountName)...) {
				setPath(acct, p, "acct-"+p)
			}
			r.body = acct
			if strings.HasPrefix(strings.TrimPrefix(v.AccountID, "$."), "0.") {
				r.body = []any{acct["0"]}
			}
			id, name, err := rt.Verify(ctx, Call{Manifest: m, Config: cfg})
			if err != nil {
				t.Fatalf("verify: %v", err)
			}
			checkRequest(t, "verify", m, cfg, r)
			if v.AccountName != "" && name == "" && id == "" {
				t.Errorf("verify named no account (response %v)", r.body)
			}

			// Sources.
			call := Call{Manifest: m, Config: cfg, SourceID: "all", Source: "All"}
			if s := m.Spec.Sources; s != nil {
				src := map[string]any{}
				for _, p := range append(refs(s.ID), refs(s.Name)...) {
					setPath(src, p, "src-"+strings.ReplaceAll(p, ".", "-"))
				}
				for p, allowed := range s.Where {
					setPath(src, p, allowed[0])
				}
				r.body = respond(s.Operation.Response.ItemsPath, []any{src})
				list, err := rt.ListSources(ctx, call)
				if err != nil {
					t.Fatalf("sources: %v", err)
				}
				checkRequest(t, "sources", m, cfg, r)
				if len(list) != 1 || list[0].ID == "" || list[0].Name == "" {
					t.Fatalf("sources = %+v from %v", list, r.body)
				}
				call.SourceID, call.Source = list[0].ID, list[0].Name
			}

			// Each operation: one page, one item, a complete memory.
			for _, op := range m.Spec.Operations {
				mp := m.Spec.Mapping
				if op.Mapping != nil {
					mp = *op.Mapping
				}
				item := sampleItem(mp, m.Spec.Sync.Checkpoint)
				r.body = respond(op.Response.ItemsPath, []any{item})
				page, err := rt.Page(ctx, call, op)
				if err != nil {
					t.Fatalf("%s: %v", op.Name, err)
				}
				checkRequest(t, op.Name, m, cfg, r)
				if len(page.Items) != 1 {
					t.Fatalf("%s: %d items from %v", op.Name, len(page.Items), r.body)
				}
				it := page.Items[0]
				if it.ID == "" || strings.TrimSpace(it.Content) == "" {
					t.Errorf("%s: item has no ID or content: %+v", op.Name, it)
				}
				if strings.Contains(it.Content, "{{") || strings.Contains(it.Title, "{{") {
					t.Errorf("%s: unrendered template in memory: %q", op.Name, it.Content)
				}
				if mp.EventDate != "" && it.EventDate.IsZero() {
					t.Errorf("%s: event date %q not parsed", op.Name, mp.EventDate)
				}
				if m.Spec.Sync.Checkpoint != "" && page.MaxCheckpoint == "" {
					t.Errorf("%s: checkpoint %q not found on the item", op.Name, m.Spec.Sync.Checkpoint)
				}
			}
		})
	}
}
