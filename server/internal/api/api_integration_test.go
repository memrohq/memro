package api_test

// End-to-end tests against a real Postgres with pgvector. They are skipped
// unless MEMRO_TEST_DATABASE_URL is set, e.g.:
//
//	docker run -d -p 55432:5432 -e POSTGRES_USER=memro -e POSTGRES_PASSWORD=memro pgvector/pgvector:pg16
//	MEMRO_TEST_DATABASE_URL=postgres://memro:memro@localhost:55432/memro go test ./internal/api/

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/api"
	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/email"
	"github.com/memrohq/memro/server/internal/embed"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/filestore"
	"github.com/memrohq/memro/server/internal/indexer"
	"github.com/memrohq/memro/server/internal/metrics"
	"github.com/memrohq/memro/server/internal/store"
	"github.com/memrohq/memro/server/migrations"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeEmbeddings returns bag-of-words vectors, so texts sharing words are
// close in cosine distance.
func fakeEmbeddings() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Input []string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		var data []item
		for i, text := range req.Input {
			v := make([]float32, 64)
			for _, wd := range strings.Fields(strings.ToLower(text)) {
				h := fnv.New32a()
				h.Write([]byte(strings.Trim(wd, ".,!?")))
				v[h.Sum32()%64]++
			}
			var n float64
			for _, x := range v {
				n += float64(x * x)
			}
			if n == 0 {
				v[0], n = 1, 1
			}
			for j := range v {
				v[j] /= float32(math.Sqrt(n))
			}
			data = append(data, item{v, i})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
}

type env struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
}

type agent struct {
	id   string
	priv ed25519.PrivateKey
}

func newAgent(t *testing.T) agent {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return agent{hex.EncodeToString(pub), priv}
}

func setup(t *testing.T, customize ...func(*api.Deps)) *env {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("MEMRO_REQUIRE_DB_TESTS") != "" {
			t.Fatal("MEMRO_REQUIRE_DB_TESTS is set but MEMRO_TEST_DATABASE_URL is empty")
		}
		t.Skip("MEMRO_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool, err := db.Open(ctx, url, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	fake := fakeEmbeddings()
	t.Cleanup(fake.Close)

	cfg := config.Load()
	cfg.RateLimitMax, cfg.RateLimitWindow = 10_000, time.Minute
	cfg.MemorySoftCap, cfg.MaxBodyBytes = 1000, 10_000_000
	cfg.AllowLegacySignatures = true
	cfg.FileStoragePath = t.TempDir()

	st := store.New(pool)
	prov := embed.New("openai", "fake-64", "test-key", "", fake.URL, 100)
	ix := indexer.New(st, prov)
	go ix.Run(ctx)
	files, _ := filestore.NewLocal(cfg.FileStoragePath)
	cfg.BaseURL = "http://example.test"
	eng := engine.New(pool, prov, ix)
	ix.OnIndexed = eng.Cache().InvalidateAgent
	deps := api.Deps{Config: cfg, Store: st, Engine: eng, Embed: prov,
		Indexer: ix, Files: files, Email: email.New("", 0, "", "", "")}
	for _, c := range customize {
		c(&deps)
	}
	srv := api.NewServer(deps)
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return &env{t, hs, st}
}

// do sends a request; sign is "", "v1" or "v2".
func (e *env) do(a *agent, sign, method, path string, body []byte, hdr map[string]string) (int, []byte) {
	e.t.Helper()
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(body))
		if body != nil && hdr["Content-Type"] == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		if a != nil && sign != "" {
			ts := strconv.FormatInt(time.Now().Unix(), 10)
			var payload []byte
			if sign == "v2" {
				payload = []byte(ts + "\n" + method + "\n" + req.URL.RequestURI() + "\n" + string(body))
				req.Header.Set("X-Signature-Version", "2")
			} else {
				payload = append([]byte(ts), body...)
			}
			req.Header.Set("X-Agent-Id", a.id)
			req.Header.Set("X-Timestamp", ts)
			req.Header.Set("X-Signature", hex.EncodeToString(ed25519.Sign(a.priv, payload)))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			e.t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if a != nil && sign != "" && resp.StatusCode == 401 && strings.Contains(string(b), "Request already used") && attempt < 2 {
			time.Sleep(1100 * time.Millisecond)
			continue
		}
		return resp.StatusCode, b
	}
	return 0, nil
}

func (e *env) must(want int, got int, body []byte, what string) {
	e.t.Helper()
	if got != want {
		e.t.Fatalf("%s: status %d, want %d: %s", what, got, want, body)
	}
}

func j(v any) []byte { b, _ := json.Marshal(v); return b }

func (e *env) register(a agent) {
	e.t.Helper()
	code, b := e.do(nil, "", "POST", "/identity", j(map[string]string{"public_key": a.id}), nil)
	e.must(200, code, b, "register identity")
}

func (e *env) remember(a agent, content, memType string) string {
	e.t.Helper()
	req := map[string]any{"content": content}
	if memType != "" {
		req["memory_type"] = memType
	}
	code, b := e.do(&a, "v2", "POST", "/memory", j(req), nil)
	e.must(200, code, b, "create memory")
	var m struct{ ID string }
	_ = json.Unmarshal(b, &m)
	return m.ID
}

func (e *env) waitEmbedded(a agent, n int) {
	e.t.Helper()
	for range 100 {
		var c int
		_ = e.st.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM memory_embeddings WHERE agent_id = $1`, a.id).Scan(&c)
		if c >= n {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	e.t.Fatalf("memories were not embedded")
}

func TestHealthAndIdentity(t *testing.T) {
	e := setup(t)
	code, b := e.do(nil, "", "GET", "/health", nil, nil)
	e.must(200, code, b, "health")
	if !strings.Contains(string(b), `"vector_store":"ok"`) {
		t.Fatalf("health = %s", b)
	}

	a := newAgent(t)
	e.register(a)
	e.register(a) // idempotent
	code, b = e.do(nil, "", "GET", "/identity/"+a.id, nil, nil)
	e.must(200, code, b, "get identity")
	code, b = e.do(nil, "", "POST", "/identity", j(map[string]string{"public_key": "abc"}), nil)
	e.must(400, code, b, "bad public key")
}

func TestAuthRejections(t *testing.T) {
	e := setup(t)
	a, stranger := newAgent(t), newAgent(t)
	e.register(a)

	code, b := e.do(nil, "", "GET", "/memory/agent/"+a.id, nil, nil)
	e.must(401, code, b, "unsigned")
	// Regression: any bearer token used to be accepted.
	code, b = e.do(nil, "", "GET", "/memory/agent/"+a.id, nil, map[string]string{"Authorization": "Bearer whatever", "X-Agent-Id": a.id})
	e.must(401, code, b, "fake bearer token")
	code, b = e.do(&stranger, "v2", "GET", "/memory/agent/"+stranger.id, nil, nil)
	e.must(401, code, b, "unregistered agent")

	// Wrong key for the claimed agent id.
	forged := agent{id: a.id, priv: stranger.priv}
	code, b = e.do(&forged, "v2", "GET", "/memory/agent/"+a.id, nil, nil)
	e.must(401, code, b, "forged signature")

	// v2 binds the request line and cannot be replayed.
	code, b = e.do(&a, "v2", "GET", "/memory/agent/"+a.id+"?limit=5", nil, nil)
	e.must(200, code, b, "v2 signed request")

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := hex.EncodeToString(ed25519.Sign(a.priv, []byte(ts+"\nGET\n/usage\n")))
	h := map[string]string{"X-Agent-Id": a.id, "X-Timestamp": ts, "X-Signature": sig, "X-Signature-Version": "2"}
	code, b = e.do(nil, "", "GET", "/usage", nil, h)
	e.must(200, code, b, "v2 first use")
	code, b = e.do(nil, "", "GET", "/usage", nil, h)
	e.must(401, code, b, "v2 replay")
	code, b = e.do(nil, "", "DELETE", "/identity/"+a.id, nil, h)
	e.must(401, code, b, "v2 signature reused on another route")
}

func TestSignatureHexCaseReplay(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := []byte(ts + "\nGET\n/usage\n")
	sig := hex.EncodeToString(ed25519.Sign(a.priv, payload))

	// 1. A valid v2 request -> 200
	h := map[string]string{
		"X-Agent-Id":          a.id,
		"X-Timestamp":         ts,
		"X-Signature":         sig,
		"X-Signature-Version": "2",
	}
	code, b := e.do(nil, "", "GET", "/usage", nil, h)
	e.must(200, code, b, "valid v2 request")

	// 2. The identical request again -> 401 "Request already used"
	code, b = e.do(nil, "", "GET", "/usage", nil, h)
	e.must(401, code, b, "identical request replay")
	if !strings.Contains(string(b), "Request already used") {
		t.Fatalf("expected 'Request already used', got %s", b)
	}

	// 3. The same signature uppercased -> 401 "Request already used"
	hUpper := map[string]string{
		"X-Agent-Id":          a.id,
		"X-Timestamp":         ts,
		"X-Signature":         strings.ToUpper(sig),
		"X-Signature-Version": "2",
	}
	code, b = e.do(nil, "", "GET", "/usage", nil, hUpper)
	e.must(401, code, b, "uppercased signature replay")
	if !strings.Contains(string(b), "Request already used") {
		t.Fatalf("expected 'Request already used', got %s", b)
	}

	// 4. Mixed case -> 401 "Request already used"
	mixed := make([]byte, len(sig))
	for i := 0; i < len(sig); i++ {
		c := sig[i]
		if i%2 == 0 && c >= 'a' && c <= 'f' {
			mixed[i] = c - 'a' + 'A'
		} else {
			mixed[i] = c
		}
	}
	hMixed := map[string]string{
		"X-Agent-Id":          a.id,
		"X-Timestamp":         ts,
		"X-Signature":         string(mixed),
		"X-Signature-Version": "2",
	}
	code, b = e.do(nil, "", "GET", "/usage", nil, hMixed)
	e.must(401, code, b, "mixed-case signature replay")
	if !strings.Contains(string(b), "Request already used") {
		t.Fatalf("expected 'Request already used', got %s", b)
	}

	// 5. Non-128 hex chars -> 401
	hShort := map[string]string{
		"X-Agent-Id":          a.id,
		"X-Timestamp":         ts,
		"X-Signature":         sig[:126],
		"X-Signature-Version": "2",
	}
	code, b = e.do(nil, "", "GET", "/usage", nil, hShort)
	e.must(401, code, b, "short signature rejected")
}

func TestMemoryLifecycleAndSearch(t *testing.T) {
	e := setup(t)
	a, b := newAgent(t), newAgent(t)
	e.register(a)
	e.register(b)

	// MCP-style request: no agent_id, no visibility, with metadata.
	code, body := e.do(&a, "v2", "POST", "/memory", j(map[string]any{
		"content": "User prefers dark mode in the editor", "memory_type": "profile", "metadata": map[string]any{"source": "mcp"},
	}), nil)
	e.must(200, code, body, "create memory")
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["memory_type"] != "profile" || m["visibility"] != "private" || m["metadata"].(map[string]any)["source"] != "mcp" {
		t.Fatalf("unexpected memory json: %s", body)
	}

	e.remember(a, "Deployed the billing service to production yesterday", "episodic")
	e.remember(a, "The office coffee machine is on the third floor", "")

	code, body = e.do(&a, "v2", "POST", "/memory", j(map[string]any{"agent_id": b.id, "content": "x"}), nil)
	e.must(403, code, body, "write into another agent")

	e.waitEmbedded(a, 3)

	code, body = e.do(&a, "v2", "GET", "/search?agent_id="+a.id+"&query=editor+dark+mode&mode=semantic&limit=2", nil, nil)
	e.must(200, code, body, "semantic search")
	var sr struct {
		Results []struct{ Content string }
		Mode    string
	}
	_ = json.Unmarshal(body, &sr)
	if len(sr.Results) == 0 || !strings.Contains(sr.Results[0].Content, "dark mode") || sr.Mode != "semantic" {
		t.Fatalf("semantic search = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/search?query=billing&mode=keyword", nil, nil)
	e.must(200, code, body, "keyword search")
	if !strings.Contains(string(body), "billing service") {
		t.Fatalf("keyword search = %s", body)
	}

	code, body = e.do(&b, "v2", "GET", "/search?agent_id="+a.id+"&query=billing", nil, nil)
	e.must(403, code, body, "search another agent")

	// Cached results must be invalidated when a memory is added.
	e.do(&a, "v2", "GET", "/search?query=kubernetes", nil, nil)
	e.remember(a, "We migrated the cluster to kubernetes", "semantic")
	code, body = e.do(&a, "v2", "GET", "/search?query=kubernetes&mode=keyword", nil, nil)
	if !strings.Contains(string(body), "kubernetes") {
		t.Fatalf("stale search cache: %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/search/temporal?query=billing+service+production&temporal_weight=0.5", nil, nil)
	e.must(200, code, body, "temporal search")
	if !strings.Contains(string(body), "Observed:") {
		t.Fatalf("temporal search = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/memory/agent/"+a.id+"?memory_type=profile", nil, nil)
	e.must(200, code, body, "list memories")
	var list []map[string]any
	_ = json.Unmarshal(body, &list)
	if len(list) != 1 {
		t.Fatalf("filtered list = %s", body)
	}
	code, body = e.do(&b, "v2", "GET", "/memory/agent/"+a.id, nil, nil)
	e.must(403, code, body, "list another agent")

	code, body = e.do(&a, "v2", "GET", "/export/"+a.id, nil, nil)
	e.must(200, code, body, "export")
	if !strings.Contains(string(body), `"total":4`) {
		t.Fatalf("export = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/stats/temporal", nil, nil)
	e.must(200, code, body, "temporal stats")
	if !strings.Contains(string(body), `"total_memories":4`) {
		t.Fatalf("stats = %s", body)
	}

	id := list[0]["id"].(string)
	code, body = e.do(&b, "v2", "DELETE", "/memory/"+id, nil, nil)
	e.must(404, code, body, "delete another agent's memory")
	code, body = e.do(&a, "v2", "DELETE", "/memory/"+id, nil, nil)
	e.must(204, code, body, "delete memory")
}

func TestCognitiveEngine(t *testing.T) {
	e := setup(t)
	a, other := newAgent(t), newAgent(t)
	e.register(a)
	e.register(other)

	e.remember(a, "I went hiking in the alps last week", "episodic")
	e.remember(a, "I bought new climbing shoes", "episodic")
	e.remember(a, "I worked late on the launch", "episodic")
	love := e.remember(a, "I love spicy food", "profile")
	hate := e.remember(a, "I hate spicy food", "profile")
	foreign := e.remember(other, "Some other agent fact", "semantic")
	e.waitEmbedded(a, 5)

	code, body := e.do(&a, "v2", "POST", "/mee/reason", j(map[string]string{"query": "spicy food"}), nil)
	e.must(200, code, body, "reason")
	if !strings.Contains(string(body), "spicy") {
		t.Fatalf("reason = %s", body)
	}

	code, body = e.do(&a, "v2", "POST", "/mee/evolve", nil, nil)
	e.must(200, code, body, "evolve")
	var ev struct {
		MemoriesEvolved    int      `json:"memories_evolved"`
		PatternsIdentified []string `json:"patterns_identified"`
	}
	_ = json.Unmarshal(body, &ev)
	if ev.MemoriesEvolved != 3 || len(ev.PatternsIdentified) < 2 {
		t.Fatalf("evolve = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/analyze/conflicts", nil, nil)
	e.must(200, code, body, "analyze conflicts")
	if !strings.Contains(string(body), `"total_conflicts":1`) {
		t.Fatalf("conflicts = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/memory/chain/"+love, nil, nil)
	e.must(200, code, body, "memory chain")
	if !strings.Contains(string(body), `"relation_type":"updates"`) {
		t.Fatalf("chain = %s", body)
	}

	code, body = e.do(&a, "v2", "POST", "/mee/resolve", j(map[string]string{"memory_a_id": love, "memory_b_id": hate}), nil)
	e.must(200, code, body, "resolve")
	if !strings.Contains(string(body), "Resolved") && !strings.Contains(string(body), "Ambiguous") {
		t.Fatalf("resolve = %s", body)
	}
	code, body = e.do(&a, "v2", "POST", "/mee/resolve", j(map[string]string{"memory_a_id": love, "memory_b_id": foreign}), nil)
	e.must(404, code, body, "resolve with another agent's memory")

	code, body = e.do(&a, "v2", "GET", "/mee/timeline/"+love, nil, nil)
	e.must(200, code, body, "timeline")
	if !strings.Contains(string(body), "memory.create") {
		t.Fatalf("timeline = %s", body)
	}
	code, body = e.do(&a, "v2", "GET", "/mee/timeline/"+foreign, nil, nil)
	e.must(404, code, body, "timeline of another agent's memory")

	code, body = e.do(&a, "v2", "POST", "/memory/generate-atoms", j(map[string]string{"chunk": "Ana works at Acme. She loves tea."}), nil)
	e.must(200, code, body, "generate atoms")
	if !strings.Contains(string(body), "employed_at") {
		t.Fatalf("atoms = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/usage", nil, nil)
	e.must(200, code, body, "usage")
	if !strings.Contains(string(body), `"reason_calls":1`) || !strings.Contains(string(body), `"evolve_calls":1`) {
		t.Fatalf("usage = %s", body)
	}
}

func TestFilesHooksTenants(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("agent_id", a.id)
	fw, _ := w.CreateFormFile("file", "notes.md")
	_, _ = fw.Write([]byte("Quarterly planning notes: ship the Go backend."))
	_ = w.Close()
	code, body := e.do(&a, "v2", "POST", "/upload", buf.Bytes(), map[string]string{"Content-Type": w.FormDataContentType()})
	e.must(200, code, body, "upload")
	var up struct {
		FileID        string  `json:"file_id"`
		ExtractedText *string `json:"extracted_text"`
	}
	_ = json.Unmarshal(body, &up)
	if up.ExtractedText == nil || !strings.Contains(*up.ExtractedText, "Go backend") {
		t.Fatalf("upload = %s", body)
	}

	code, body = e.do(&a, "v2", "POST", "/memory/from-file", j(map[string]string{
		"agent_id": a.id, "file_id": up.FileID, "memory_type": "semantic", "visibility": "private"}), nil)
	e.must(200, code, body, "memory from file")

	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID, nil, nil)
	e.must(200, code, body, "asset")
	if !strings.Contains(string(body), "Quarterly planning") {
		t.Fatalf("asset = %s", body)
	}
	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID[:4], nil, nil)
	e.must(404, code, body, "asset by prefix")

	code, body = e.do(&a, "v2", "POST", "/hooks", j(map[string]string{"event_type": "memory_created", "url": "http://169.254.169.254/latest"}), nil)
	e.must(400, code, body, "metadata-server webhook")
	code, body = e.do(&a, "v2", "POST", "/hooks", j(map[string]string{"event_type": "memory_created", "url": "https://hooks.example.com/memro"}), nil)
	e.must(201, code, body, "register hook")
	code, body = e.do(&a, "v2", "GET", "/hooks/memory_created", nil, nil)
	e.must(200, code, body, "list hooks")
	if !strings.Contains(string(body), "hooks.example.com") {
		t.Fatalf("hooks = %s", body)
	}

	// The shared default tenant's usage spans every user (ADR 0012); see
	// TestTenantEndpointsRequirePlatformAdmin for who may read it.
	code, body = e.do(&a, "v2", "GET", "/tenant/00000000-0000-0000-0000-000000000000/usage?hours=1", nil, nil)
	e.must(403, code, body, "default tenant usage")

	code, body = e.do(nil, "", "POST", "/auth/login", j(map[string]string{"email": "dev@memro.co"}), nil)
	e.must(503, code, body, "login without SMTP")

	code, body = e.do(&a, "v2", "DELETE", "/identity/"+newAgent(t).id, nil, nil)
	e.must(403, code, body, "delete another identity")
	code, body = e.do(&a, "v2", "DELETE", "/identity/"+a.id, nil, nil)
	e.must(204, code, body, "delete own identity")
}

func TestExtensionPoints(t *testing.T) {
	var mu sync.Mutex
	var events []extension.UsageEvent
	meter := extension.UsageMeterFunc(func(_ context.Context, ev extension.UsageEvent) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
		return nil
	})
	e := setup(t, func(d *api.Deps) {
		d.Usage = meter
		d.Quota = extension.PerAgentCap{Max: 2}
		d.Routes = map[string]http.Handler{
			"POST /billing/webhook": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }),
		}
	})
	a := newAgent(t)
	e.register(a)
	e.remember(a, "first memory", "semantic")
	e.remember(a, "second memory", "semantic")
	code, body := e.do(&a, "v2", "POST", "/memory", j(map[string]any{"content": "third memory"}), nil)
	e.must(403, code, body, "custom quota")
	if !strings.Contains(string(body), "soft cap reached (2/agent)") {
		t.Fatalf("quota message = %s", body)
	}

	code, body = e.do(&a, "v2", "GET", "/search?query=memory&mode=keyword", nil, nil)
	e.must(200, code, body, "search")
	mu.Lock()
	got := map[string]int{}
	for _, ev := range events {
		if ev.AgentID == a.id {
			got[ev.Metric]++
		}
	}
	mu.Unlock()
	if got[extension.MetricMemoriesStored] != 2 || got[extension.MetricMemoriesSearched] != 1 {
		t.Fatalf("usage events = %v", got)
	}

	code, body = e.do(nil, "", "POST", "/billing/webhook", []byte("{}"), nil)
	e.must(202, code, body, "extra route")
}

func TestDevLogOTPSignIn(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.DevLogOTP = true })
	code, body := e.do(nil, "", "POST", "/auth/login", j(map[string]string{"email": "dev@example.com"}), nil)
	e.must(200, code, body, "login with DEV_LOG_OTP")

	// Wrong codes are rejected and count toward the attempt limit.
	code, body = e.do(nil, "", "POST", "/auth/verify", j(map[string]string{"email": "dev@example.com", "code": "000000"}), nil)
	if code != 401 && code != 429 {
		t.Fatalf("wrong code: status %d: %s", code, body)
	}
}

// dashboardSession creates a user with an agent and a session token, as the
// email login would.
func (e *env) dashboardSession(email string) (agent, string) {
	e.t.Helper()
	ctx := context.Background()
	a := newAgent(e.t)
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		e.t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		e.t.Fatalf("user: %v", err)
	}
	if err := e.st.CreateIdentity(ctx, a.id, &u.ID, uuid.Nil); err != nil {
		e.t.Fatal(err)
	}
	token, hash, _ := auth.NewSessionToken()
	if err := e.st.CreateSession(ctx, u.ID, uuid.Nil, hash, time.Now().Add(time.Hour)); err != nil {
		e.t.Fatal(err)
	}
	return a, token
}

func TestDeviceLoginAndAPIKeys(t *testing.T) {
	e := setup(t)
	a, session := e.dashboardSession(fmt.Sprintf("dev-%d@example.com", time.Now().UnixNano()))

	code, body := e.do(nil, "", "POST", "/auth/device/code", j(map[string]string{"client_name": "memro connect (test)"}), nil)
	e.must(200, code, body, "device code")
	var dc struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri_complete"`
	}
	_ = json.Unmarshal(body, &dc)
	if len(dc.UserCode) != 9 || !strings.Contains(dc.URI, "/connect?code="+dc.UserCode) {
		t.Fatalf("device code response: %s", body)
	}

	code, body = e.do(nil, "", "POST", "/auth/device/token", j(map[string]string{"device_code": dc.DeviceCode}), nil)
	if code != 400 || !strings.Contains(string(body), "authorization_pending") {
		t.Fatalf("expected pending, got %d %s", code, body)
	}

	code, body = e.do(nil, "", "GET", "/auth/device?user_code="+dc.UserCode, nil, bearer(session))
	e.must(200, code, body, "device info")
	if !strings.Contains(string(body), "memro connect (test)") || !strings.Contains(string(body), `"pending"`) {
		t.Fatalf("device info: %s", body)
	}
	code, body = e.do(nil, "", "POST", "/auth/device/approve", j(map[string]string{"user_code": strings.ToLower(dc.UserCode)}), bearer(session))
	e.must(200, code, body, "approve")

	code, body = e.do(nil, "", "POST", "/auth/device/token", j(map[string]string{"device_code": dc.DeviceCode}), nil)
	e.must(200, code, body, "token exchange")
	var tok struct {
		APIKey  string `json:"api_key"`
		AgentID string `json:"agent_id"`
		MCPURL  string `json:"mcp_url"`
	}
	_ = json.Unmarshal(body, &tok)
	if !strings.HasPrefix(tok.APIKey, "memro_sk_") || tok.AgentID != a.id || tok.MCPURL == "" {
		t.Fatalf("token response: %s", body)
	}
	code, body = e.do(nil, "", "POST", "/auth/device/token", j(map[string]string{"device_code": dc.DeviceCode}), nil)
	if code != 400 || !strings.Contains(string(body), "invalid_grant") {
		t.Fatalf("device code reused: %d %s", code, body)
	}

	// The key works for memory operations.
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(tok.APIKey))
	e.must(200, code, body, "whoami with api key")
	if !strings.Contains(string(body), a.id) || !strings.Contains(string(body), `"api_key"`) {
		t.Fatalf("whoami: %s", body)
	}
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]any{"content": "written with an API key"}), bearer(tok.APIKey))
	e.must(200, code, body, "memory via api key")

	// But it cannot manage keys or approve logins.
	code, body = e.do(nil, "", "GET", "/api-keys", nil, bearer(tok.APIKey))
	e.must(403, code, body, "api key listing keys")
	code, body = e.do(nil, "", "POST", "/auth/device/approve", j(map[string]string{"user_code": "BCDF-GHJK"}), bearer(tok.APIKey))
	e.must(403, code, body, "api key approving login")

	// The dashboard lists and revokes it.
	code, body = e.do(nil, "", "GET", "/api-keys", nil, bearer(session))
	e.must(200, code, body, "list keys")
	var keys []struct{ ID, Prefix string }
	_ = json.Unmarshal(body, &keys)
	if len(keys) != 1 || !strings.HasPrefix(tok.APIKey, keys[0].Prefix) || strings.Contains(string(body), tok.APIKey) {
		t.Fatalf("key listing: %s", body)
	}
	code, body = e.do(nil, "", "DELETE", "/api-keys/"+keys[0].ID, nil, bearer(session))
	e.must(204, code, body, "revoke")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(tok.APIKey))
	e.must(401, code, body, "revoked key")

	code, body = e.do(nil, "", "POST", "/api-keys", j(map[string]string{"name": "ci"}), bearer(session))
	e.must(200, code, body, "create key in dashboard")
	if !strings.Contains(string(body), `"key":"memro_sk_`) {
		t.Fatalf("create key: %s", body)
	}
}

// Signing in creates an agent but never hands its private key to the browser;
// the session alone is enough to use it.
func TestSignInDoesNotReturnPrivateKey(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	email := fmt.Sprintf("nokey-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetUserOTP(ctx, email, uuid.Nil, auth.HashOTP(email, "123456"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(nil, "", "POST", "/auth/verify", j(map[string]string{"email": email, "code": "123456"}), nil)
	e.must(200, code, body, "verify")
	if strings.Contains(string(body), "private_key") {
		t.Fatalf("verify returned a private key: %s", body)
	}
	var res struct {
		Token   string `json:"token"`
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal(body, &res)
	if res.AgentID == "" {
		t.Fatalf("no agent created: %s", body)
	}
	bearer := map[string]string{"Authorization": "Bearer " + res.Token}
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer)
	e.must(200, code, body, "whoami")
	if !strings.Contains(string(body), res.AgentID) {
		t.Fatalf("whoami: %s", body)
	}
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]any{"content": "saved with a session"}), bearer)
	e.must(200, code, body, "memory via session")
}

// Webhooks belong to the agent that registered them, even when agents share
// a tenant.
func TestHooksAreScopedToTheAgent(t *testing.T) {
	e := setup(t)
	a, b := newAgent(t), newAgent(t)
	e.register(a)
	e.register(b)
	code, body := e.do(&a, "v2", "POST", "/hooks", j(map[string]string{"event_type": "memory_created", "url": "https://a.example.com/hook"}), nil)
	e.must(201, code, body, "register hook as A")

	code, body = e.do(&b, "v2", "GET", "/hooks/memory_created", nil, nil)
	e.must(200, code, body, "list hooks as B")
	if strings.Contains(string(body), "a.example.com") {
		t.Fatalf("B sees A's hook: %s", body)
	}
	if urls, err := e.st.GetHooks(context.Background(), b.id, "memory_created"); err != nil || len(urls) != 0 {
		t.Fatalf("B's memories would notify %v (err %v)", urls, err)
	}
	if urls, _ := e.st.GetHooks(context.Background(), a.id, "memory_created"); len(urls) != 1 {
		t.Fatalf("A's hooks = %v", urls)
	}
	hook := j(map[string]string{"event_type": "memory_created", "url": "https://a.example.com/hook"})
	code, body = e.do(&b, "v2", "DELETE", "/hooks", hook, nil)
	e.must(404, code, body, "B deletes A's hook")
	code, body = e.do(&a, "v2", "DELETE", "/hooks", hook, nil)
	e.must(204, code, body, "A deletes its hook")
	if urls, _ := e.st.GetHooks(context.Background(), a.id, "memory_created"); len(urls) != 0 {
		t.Fatalf("hook not deleted: %v", urls)
	}
}

func (e *env) userWithCode(code string) string {
	e.t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("otp-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetUserOTP(ctx, email, uuid.Nil, auth.HashOTP(email, code), time.Now().Add(time.Minute)); err != nil {
		e.t.Fatal(err)
	}
	return email
}

// Parallel guesses must not get past the attempt limit.
func TestOTPAttemptLimitHoldsUnderConcurrency(t *testing.T) {
	e := setup(t)
	email := e.userWithCode("123456")
	var wg sync.WaitGroup
	var mu sync.Mutex
	checked := 0
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := e.do(nil, "", "POST", "/auth/verify", j(map[string]string{"email": email, "code": fmt.Sprintf("9%05d", i)}), nil)
			if code == 401 {
				mu.Lock()
				checked++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if checked > 5 {
		t.Fatalf("%d guesses were checked, want at most 5", checked)
	}
	code, body := e.do(nil, "", "POST", "/auth/verify", j(map[string]string{"email": email, "code": "123456"}), nil)
	e.must(429, code, body, "right code after the limit")
}

func TestOTPIsSingleUse(t *testing.T) {
	e := setup(t)
	email := e.userWithCode("654321")
	code, body := e.do(nil, "", "POST", "/auth/verify", j(map[string]string{"email": email, "code": "654321"}), nil)
	e.must(200, code, body, "first use")
	code, body = e.do(nil, "", "POST", "/auth/verify", j(map[string]string{"email": email, "code": "654321"}), nil)
	e.must(401, code, body, "second use")
}

func TestSignInCodesAreThrottledPerEmail(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.DevLogOTP = true })
	for i := range 5 {
		code, body := e.do(nil, "", "POST", "/auth/login", j(map[string]string{"email": "throttle@example.com"}), nil)
		e.must(200, code, body, fmt.Sprintf("code %d", i+1))
	}
	code, body := e.do(nil, "", "POST", "/auth/login", j(map[string]string{"email": "throttle@example.com"}), nil)
	e.must(429, code, body, "sixth code within the hour")
	code, body = e.do(nil, "", "POST", "/auth/login", j(map[string]string{"email": "other@example.com"}), nil)
	e.must(200, code, body, "another email")
}

// A memory with no embedding yet (just written, or embedding failed) is
// still found by temporal search, by keyword.
func TestTemporalSearchFindsMemoriesWithoutEmbeddings(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	now := time.Now().UTC()
	m := &domain.Memory{ID: uuid.New(), AgentID: a.id, TenantID: uuid.Nil, Content: "The invoicing pipeline moved to Rust",
		MemoryType: domain.Semantic, Visibility: domain.Private, CreatedAt: now, DocumentDate: now, LastAccessedAt: now, Confidence: 1}
	if err := e.st.InsertMemory(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(&a, "v2", "GET", "/search/temporal?query=invoicing+pipeline", nil, nil)
	e.must(200, code, body, "temporal search")
	if !strings.Contains(string(body), m.ID.String()) {
		t.Fatalf("un-embedded memory not found: %s", body)
	}
}

// Deleting an agent removes its uploaded files too, so /assets stops serving them.
func TestDeleteIdentityRemovesFiles(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "secret.txt")
	fw.Write([]byte("private notes"))
	mw.WriteField("agent_id", a.id)
	mw.Close()
	code, body := e.do(&a, "v2", "POST", "/upload", buf.Bytes(), map[string]string{"Content-Type": mw.FormDataContentType()})
	e.must(200, code, body, "upload")
	var up struct {
		FileID string `json:"file_id"`
	}
	_ = json.Unmarshal(body, &up)
	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID, nil, nil)
	e.must(200, code, body, "asset before delete")

	code, body = e.do(&a, "v2", "DELETE", "/identity/"+a.id, nil, nil)
	e.must(204, code, body, "delete identity")
	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID, nil, nil)
	e.must(404, code, body, "asset after delete")
}

// Deleting the last memory made from a file deletes the file.
func TestDeletingAMemoryRemovesItsFile(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "note.txt")
	fw.Write([]byte("a private note"))
	mw.WriteField("agent_id", a.id)
	mw.Close()
	code, body := e.do(&a, "v2", "POST", "/upload", buf.Bytes(), map[string]string{"Content-Type": mw.FormDataContentType()})
	e.must(200, code, body, "upload")
	var up struct {
		FileID string `json:"file_id"`
	}
	_ = json.Unmarshal(body, &up)
	var ids [2]string
	for i := range ids {
		code, body = e.do(&a, "v2", "POST", "/memory/from-file", j(map[string]string{"file_id": up.FileID, "memory_type": []string{"semantic", "episodic"}[i]}), nil)
		e.must(200, code, body, "memory from file")
		var m struct {
			MemoryID string `json:"memory_id"`
		}
		_ = json.Unmarshal(body, &m)
		ids[i] = m.MemoryID
	}
	code, body = e.do(&a, "v2", "DELETE", "/memory/"+ids[0], nil, nil)
	e.must(204, code, body, "delete first memory")
	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID, nil, nil)
	e.must(200, code, body, "file still used by the second memory")
	code, body = e.do(&a, "v2", "DELETE", "/memory/"+ids[1], nil, nil)
	e.must(204, code, body, "delete second memory")
	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID, nil, nil)
	e.must(404, code, body, "file after its last memory is deleted")
}

func (e *env) uploadForm(a *agent, path, field, name string, data []byte, extra map[string]string) (int, []byte) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile(field, name)
	fw.Write(data)
	for k, v := range extra {
		mw.WriteField(k, v)
	}
	mw.Close()
	return e.do(a, "v2", "POST", path, buf.Bytes(), map[string]string{"Content-Type": mw.FormDataContentType()})
}

func docx(t *testing.T, paragraphs []string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	var sb strings.Builder
	sb.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paragraphs {
		sb.WriteString("<w:p><w:r><w:t>" + p + "</w:t></w:r></w:p>")
	}
	sb.WriteString("</w:body></w:document>")
	w.Write([]byte(sb.String()))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A Word document becomes overlapping chunks linked to the stored file; the
// file goes away with its last chunk.
func TestUploadDocumentChunksOfficeFiles(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	var paras []string
	for i := range 30 {
		paras = append(paras, fmt.Sprintf("Section %d: the billing service retries failed charges three times before alerting on-call.", i))
	}
	code, body := e.uploadForm(&a, "/upload/document", "document", "runbook.docx", docx(t, paras), nil)
	e.must(200, code, body, "upload docx")
	var mems []struct {
		ID       string `json:"id"`
		Content  string `json:"content"`
		FileID   string `json:"file_id"`
		Metadata struct {
			Source string `json:"source"`
			Chunk  int    `json:"chunk"`
			Chunks int    `json:"chunks"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &mems); err != nil || len(mems) < 2 {
		t.Fatalf("chunks = %s (%v)", body, err)
	}
	for i, m := range mems {
		if len([]rune(m.Content)) > 1500 || strings.HasPrefix(m.Content, "[DOC CHUNK]") || !strings.HasSuffix(m.Content, ".") {
			t.Errorf("chunk %d is malformed: %q", i, m.Content)
		}
		if m.FileID == "" || m.FileID != mems[0].FileID || m.Metadata.Source != "runbook.docx" ||
			m.Metadata.Chunk != i+1 || m.Metadata.Chunks != len(mems) {
			t.Errorf("chunk %d metadata = %+v file %s", i, m.Metadata, m.FileID)
		}
	}
	code, body = e.do(&a, "v2", "GET", "/search?mode=keyword&query=retries+failed+charges", nil, nil)
	e.must(200, code, body, "search chunks")
	if !strings.Contains(string(body), "retries failed charges") {
		t.Fatalf("chunks not searchable: %s", body)
	}
	code, body = e.do(nil, "", "GET", "/assets/"+mems[0].FileID, nil, nil)
	e.must(200, code, body, "document file stored")
	for _, m := range mems {
		code, body = e.do(&a, "v2", "DELETE", "/memory/"+m.ID, nil, nil)
		e.must(204, code, body, "delete chunk")
	}
	code, body = e.do(nil, "", "GET", "/assets/"+mems[0].FileID, nil, nil)
	e.must(404, code, body, "file deleted with its last chunk")
}

func TestUploadDocumentRejectsUnsupportedFiles(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	code, body := e.uploadForm(&a, "/upload/document", "document", "photo.png", []byte{0x89, 'P', 'N', 'G', 0, 1}, nil)
	e.must(400, code, body, "image as document")
	code, body = e.uploadForm(&a, "/upload/document", "document", "broken.pdf", []byte("%PDF-1.4 truncated"), nil)
	e.must(400, code, body, "broken PDF")
}

// Long extracted text becomes several memories instead of being cut at 10,000 characters.
func TestMemoryFromLongFileIsChunked(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	text := strings.Repeat("Memro keeps long documents whole by chunking them. ", 400) + "The final sentence is kept."
	code, body := e.uploadForm(&a, "/upload", "file", "long.txt", []byte(text), map[string]string{"agent_id": a.id})
	e.must(200, code, body, "upload")
	var up struct {
		FileID string `json:"file_id"`
	}
	_ = json.Unmarshal(body, &up)
	code, body = e.do(&a, "v2", "POST", "/memory/from-file", j(map[string]string{"file_id": up.FileID, "memory_type": "semantic"}), nil)
	e.must(200, code, body, "memory from long file")
	var res struct {
		MemoryIDs []string `json:"memory_ids"`
		Chunks    int      `json:"chunks"`
	}
	_ = json.Unmarshal(body, &res)
	if res.Chunks < 10 || len(res.MemoryIDs) != res.Chunks {
		t.Fatalf("from-file = %s", body)
	}
	code, body = e.do(&a, "v2", "GET", "/search?mode=keyword&query=final+sentence+kept", nil, nil)
	e.must(200, code, body, "search tail")
	if !strings.Contains(string(body), "The final sentence is kept.") {
		t.Fatalf("end of the document was lost: %s", body)
	}
}

// gatePolicy is a QuotaPolicy that also implements extension.UsageGate: it
// records every check and refuses queries once maxQueries have been allowed.
type gatePolicy struct {
	extension.PerAgentCap
	mu         sync.Mutex
	checks     []extension.UsageEvent
	maxQueries int
	queries    int
}

func (g *gatePolicy) AllowUsage(_ context.Context, tenantID uuid.UUID, agentID, metric string, amount int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checks = append(g.checks, extension.UsageEvent{TenantID: tenantID, AgentID: agentID, Metric: metric, Value: amount})
	if metric == extension.MetricQueries {
		if g.queries >= g.maxQueries {
			return &extension.QuotaError{Message: "Monthly query limit reached"}
		}
		g.queries++
	}
	return nil
}

func TestUsageGate(t *testing.T) {
	var mu sync.Mutex
	recorded := map[string]int64{}
	meter := extension.UsageMeterFunc(func(_ context.Context, ev extension.UsageEvent) error {
		mu.Lock()
		defer mu.Unlock()
		recorded[ev.Metric] += ev.Value
		return nil
	})
	policy := &gatePolicy{maxQueries: 2}
	e := setup(t, func(d *api.Deps) { d.Usage = meter; d.Quota = policy })
	a := newAgent(t)
	e.register(a)

	e.remember(a, "deploys happen on tuesdays", "semantic") // 26 bytes -> 7 tokens
	code, body := e.uploadForm(&a, "/upload", "file", "n.txt", []byte("twenty bytes of text"), map[string]string{"agent_id": a.id})
	e.must(200, code, body, "upload")

	code, body = e.do(&a, "v2", "GET", "/search?query=deploys&mode=keyword", nil, nil)
	e.must(200, code, body, "first query")
	code, body = e.do(&a, "v2", "GET", "/search/temporal?query=deploys", nil, nil)
	e.must(200, code, body, "second query")
	code, body = e.do(&a, "v2", "POST", "/mee/reason", j(map[string]string{"query": "when do deploys happen"}), nil)
	e.must(403, code, body, "third query over the limit")
	if !strings.Contains(string(body), "Monthly query limit reached") {
		t.Fatalf("message = %s", body)
	}

	policy.mu.Lock()
	for _, c := range policy.checks {
		if c.AgentID != a.id {
			t.Errorf("gate got agent %q, want %q", c.AgentID, a.id)
		}
	}
	checks := append([]extension.UsageEvent(nil), policy.checks...)
	policy.mu.Unlock()
	want := map[string]int64{extension.MetricStoredTokens: 7, extension.MetricIngestBytes: 20, extension.MetricQueries: 3}
	got := map[string]int64{}
	for _, c := range checks {
		got[c.Metric] += c.Value
	}
	for m, v := range want {
		if got[m] != v {
			t.Errorf("gate %s = %d, want %d (checks %+v)", m, got[m], v, checks)
		}
	}

	// Only successful actions are recorded: two queries, not the refused third.
	mu.Lock()
	defer mu.Unlock()
	if recorded[extension.MetricQueries] != 2 || recorded[extension.MetricStoredTokens] != 7 || recorded[extension.MetricIngestBytes] != 20 {
		t.Fatalf("recorded = %v", recorded)
	}
}

// A policy without UsageGate is never asked about usage.
func TestQuotaPolicyWithoutGateIsUnchanged(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Quota = extension.PerAgentCap{Max: 100} })
	a := newAgent(t)
	e.register(a)
	for i := range 3 {
		code, body := e.do(&a, "v2", "GET", fmt.Sprintf("/search?query=q%d&mode=keyword", i), nil, nil)
		e.must(200, code, body, "search")
	}
}

func TestLegacyV1Signatures(t *testing.T) {
	// 1. Verify default in config.Load() is false
	os.Unsetenv("ALLOW_LEGACY_SIGNATURES")
	defaultCfg := config.Load()
	if defaultCfg.AllowLegacySignatures {
		t.Fatal("expected AllowLegacySignatures to default to false")
	}

	// 2. Test v1 rejected by default when AllowLegacySignatures is false
	eDisabled := setup(t, func(d *api.Deps) {
		d.Config.AllowLegacySignatures = false
	})
	agentDisabled := newAgent(t)
	eDisabled.register(agentDisabled)

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := hex.EncodeToString(ed25519.Sign(agentDisabled.priv, []byte(ts)))
	h := map[string]string{
		"X-Agent-Id":  agentDisabled.id,
		"X-Timestamp": ts,
		"X-Signature": sig,
	}
	code, b := eDisabled.do(nil, "", "GET", "/usage", nil, h)
	eDisabled.must(401, code, b, "v1 rejected by default")
	if !strings.Contains(string(b), "Legacy signatures are disabled") {
		t.Fatalf("expected legacy signatures disabled message, got %s", b)
	}

	// 3. With ALLOW_LEGACY_SIGNATURES=true: accepted once, replay -> 401, counter increments
	eEnabled := setup(t, func(d *api.Deps) {
		d.Config.AllowLegacySignatures = true
	})
	agentEnabled := newAgent(t)
	eEnabled.register(agentEnabled)

	initialCount := testutil.ToFloat64(metrics.LegacySignatureRequests)

	ts2 := strconv.FormatInt(time.Now().Unix(), 10)
	sig2 := hex.EncodeToString(ed25519.Sign(agentEnabled.priv, []byte(ts2)))
	h2 := map[string]string{
		"X-Agent-Id":  agentEnabled.id,
		"X-Timestamp": ts2,
		"X-Signature": sig2,
	}
	// Accepted once
	code, b = eEnabled.do(nil, "", "GET", "/usage", nil, h2)
	eEnabled.must(200, code, b, "v1 accepted when enabled")

	// Replay -> 401
	code, b = eEnabled.do(nil, "", "GET", "/usage", nil, h2)
	eEnabled.must(401, code, b, "v1 replay rejected")
	if !strings.Contains(string(b), "Request already used") {
		t.Fatalf("expected 'Request already used', got %s", b)
	}

	// Counter increments
	finalCount := testutil.ToFloat64(metrics.LegacySignatureRequests)
	if finalCount != initialCount+1 {
		t.Fatalf("expected counter to increment by 1, before=%f, after=%f", initialCount, finalCount)
	}
}
