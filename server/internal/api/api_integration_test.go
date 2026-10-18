package api_test

// End-to-end tests against a real Postgres with pgvector. They are skipped
// unless MEMRO_TEST_DATABASE_URL is set, e.g.:
//
//	docker run -d -p 55432:5432 -e POSTGRES_USER=memro -e POSTGRES_PASSWORD=memro pgvector/pgvector:pg16
//	MEMRO_TEST_DATABASE_URL=postgres://memro:memro@localhost:55432/memro go test ./internal/api/

import (
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
	"github.com/memrohq/memro/server/internal/email"
	"github.com/memrohq/memro/server/internal/embed"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/filestore"
	"github.com/memrohq/memro/server/internal/indexer"
	"github.com/memrohq/memro/server/internal/store"
	"github.com/memrohq/memro/server/migrations"
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
	deps := api.Deps{Config: cfg, Store: st, Engine: engine.New(pool, prov, ix), Embed: prov,
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
	return resp.StatusCode, b
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
	code, b := e.do(&a, "v1", "POST", "/memory", j(req), nil)
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
	code, b = e.do(&stranger, "v1", "GET", "/memory/agent/"+stranger.id, nil, nil)
	e.must(401, code, b, "unregistered agent")

	// Wrong key for the claimed agent id.
	forged := agent{id: a.id, priv: stranger.priv}
	code, b = e.do(&forged, "v1", "GET", "/memory/agent/"+a.id, nil, nil)
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

func TestMemoryLifecycleAndSearch(t *testing.T) {
	e := setup(t)
	a, b := newAgent(t), newAgent(t)
	e.register(a)
	e.register(b)

	// MCP-style request: no agent_id, no visibility, with metadata.
	code, body := e.do(&a, "v1", "POST", "/memory", j(map[string]any{
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

	code, body = e.do(&a, "v1", "POST", "/memory", j(map[string]any{"agent_id": b.id, "content": "x"}), nil)
	e.must(403, code, body, "write into another agent")

	e.waitEmbedded(a, 3)

	code, body = e.do(&a, "v1", "GET", "/search?agent_id="+a.id+"&query=editor+dark+mode&mode=semantic&limit=2", nil, nil)
	e.must(200, code, body, "semantic search")
	var sr struct {
		Results []struct{ Content string }
		Mode    string
	}
	_ = json.Unmarshal(body, &sr)
	if len(sr.Results) == 0 || !strings.Contains(sr.Results[0].Content, "dark mode") || sr.Mode != "semantic" {
		t.Fatalf("semantic search = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/search?query=billing&mode=keyword", nil, nil)
	e.must(200, code, body, "keyword search")
	if !strings.Contains(string(body), "billing service") {
		t.Fatalf("keyword search = %s", body)
	}

	code, body = e.do(&b, "v1", "GET", "/search?agent_id="+a.id+"&query=billing", nil, nil)
	e.must(403, code, body, "search another agent")

	// Cached results must be invalidated when a memory is added.
	e.do(&a, "v1", "GET", "/search?query=kubernetes", nil, nil)
	e.remember(a, "We migrated the cluster to kubernetes", "semantic")
	code, body = e.do(&a, "v1", "GET", "/search?query=kubernetes&mode=keyword", nil, nil)
	if !strings.Contains(string(body), "kubernetes") {
		t.Fatalf("stale search cache: %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/search/temporal?query=billing+service+production&temporal_weight=0.5", nil, nil)
	e.must(200, code, body, "temporal search")
	if !strings.Contains(string(body), "Observed:") {
		t.Fatalf("temporal search = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/memory/agent/"+a.id+"?memory_type=profile", nil, nil)
	e.must(200, code, body, "list memories")
	var list []map[string]any
	_ = json.Unmarshal(body, &list)
	if len(list) != 1 {
		t.Fatalf("filtered list = %s", body)
	}
	code, body = e.do(&b, "v1", "GET", "/memory/agent/"+a.id, nil, nil)
	e.must(403, code, body, "list another agent")

	code, body = e.do(&a, "v1", "GET", "/export/"+a.id, nil, nil)
	e.must(200, code, body, "export")
	if !strings.Contains(string(body), `"total":4`) {
		t.Fatalf("export = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/stats/temporal", nil, nil)
	e.must(200, code, body, "temporal stats")
	if !strings.Contains(string(body), `"total_memories":4`) {
		t.Fatalf("stats = %s", body)
	}

	id := list[0]["id"].(string)
	code, body = e.do(&b, "v1", "DELETE", "/memory/"+id, nil, nil)
	e.must(404, code, body, "delete another agent's memory")
	code, body = e.do(&a, "v1", "DELETE", "/memory/"+id, nil, nil)
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

	code, body := e.do(&a, "v1", "POST", "/mee/reason", j(map[string]string{"query": "spicy food"}), nil)
	e.must(200, code, body, "reason")
	if !strings.Contains(string(body), "spicy") {
		t.Fatalf("reason = %s", body)
	}

	code, body = e.do(&a, "v1", "POST", "/mee/evolve", nil, nil)
	e.must(200, code, body, "evolve")
	var ev struct {
		MemoriesEvolved    int      `json:"memories_evolved"`
		PatternsIdentified []string `json:"patterns_identified"`
	}
	_ = json.Unmarshal(body, &ev)
	if ev.MemoriesEvolved != 3 || len(ev.PatternsIdentified) < 2 {
		t.Fatalf("evolve = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/analyze/conflicts", nil, nil)
	e.must(200, code, body, "analyze conflicts")
	if !strings.Contains(string(body), `"total_conflicts":1`) {
		t.Fatalf("conflicts = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/memory/chain/"+love, nil, nil)
	e.must(200, code, body, "memory chain")
	if !strings.Contains(string(body), `"relation_type":"updates"`) {
		t.Fatalf("chain = %s", body)
	}

	code, body = e.do(&a, "v1", "POST", "/mee/resolve", j(map[string]string{"memory_a_id": love, "memory_b_id": hate}), nil)
	e.must(200, code, body, "resolve")
	if !strings.Contains(string(body), "Resolved") && !strings.Contains(string(body), "Ambiguous") {
		t.Fatalf("resolve = %s", body)
	}
	code, body = e.do(&a, "v1", "POST", "/mee/resolve", j(map[string]string{"memory_a_id": love, "memory_b_id": foreign}), nil)
	e.must(404, code, body, "resolve with another agent's memory")

	code, body = e.do(&a, "v1", "GET", "/mee/timeline/"+love, nil, nil)
	e.must(200, code, body, "timeline")
	if !strings.Contains(string(body), "memory.create") {
		t.Fatalf("timeline = %s", body)
	}
	code, body = e.do(&a, "v1", "GET", "/mee/timeline/"+foreign, nil, nil)
	e.must(404, code, body, "timeline of another agent's memory")

	code, body = e.do(&a, "v1", "POST", "/memory/generate-atoms", j(map[string]string{"chunk": "Ana works at Acme. She loves tea."}), nil)
	e.must(200, code, body, "generate atoms")
	if !strings.Contains(string(body), "employed_at") {
		t.Fatalf("atoms = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/usage", nil, nil)
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
	code, body := e.do(&a, "v1", "POST", "/upload", buf.Bytes(), map[string]string{"Content-Type": w.FormDataContentType()})
	e.must(200, code, body, "upload")
	var up struct {
		FileID        string  `json:"file_id"`
		ExtractedText *string `json:"extracted_text"`
	}
	_ = json.Unmarshal(body, &up)
	if up.ExtractedText == nil || !strings.Contains(*up.ExtractedText, "Go backend") {
		t.Fatalf("upload = %s", body)
	}

	code, body = e.do(&a, "v1", "POST", "/memory/from-file", j(map[string]string{
		"agent_id": a.id, "file_id": up.FileID, "memory_type": "semantic", "visibility": "private"}), nil)
	e.must(200, code, body, "memory from file")

	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID, nil, nil)
	e.must(200, code, body, "asset")
	if !strings.Contains(string(body), "Quarterly planning") {
		t.Fatalf("asset = %s", body)
	}
	code, body = e.do(nil, "", "GET", "/assets/"+up.FileID[:4], nil, nil)
	e.must(404, code, body, "asset by prefix")

	code, body = e.do(&a, "v1", "POST", "/hooks", j(map[string]string{"event_type": "memory_created", "url": "http://169.254.169.254/latest"}), nil)
	e.must(400, code, body, "metadata-server webhook")
	code, body = e.do(&a, "v1", "POST", "/hooks", j(map[string]string{"event_type": "memory_created", "url": "https://hooks.example.com/memro"}), nil)
	e.must(201, code, body, "register hook")
	code, body = e.do(&a, "v1", "GET", "/hooks/memory_created", nil, nil)
	e.must(200, code, body, "list hooks")
	if !strings.Contains(string(body), "hooks.example.com") {
		t.Fatalf("hooks = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/tenant/00000000-0000-0000-0000-000000000000/usage?hours=1", nil, nil)
	e.must(200, code, body, "tenant usage")
	if strings.Contains(string(body), `"metric":"memories_stored","value":0`) {
		t.Fatalf("usage not recorded: %s", body)
	}

	code, body = e.do(nil, "", "POST", "/auth/login", j(map[string]string{"email": "dev@memro.co"}), nil)
	e.must(503, code, body, "login without SMTP")

	code, body = e.do(&a, "v1", "DELETE", "/identity/"+newAgent(t).id, nil, nil)
	e.must(403, code, body, "delete another identity")
	code, body = e.do(&a, "v1", "DELETE", "/identity/"+a.id, nil, nil)
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
	code, body := e.do(&a, "v1", "POST", "/memory", j(map[string]any{"content": "third memory"}), nil)
	e.must(403, code, body, "custom quota")
	if !strings.Contains(string(body), "soft cap reached (2/agent)") {
		t.Fatalf("quota message = %s", body)
	}

	code, body = e.do(&a, "v1", "GET", "/search?query=memory&mode=keyword", nil, nil)
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
	bearer := func(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

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
