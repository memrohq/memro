package memro

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// verifyV2 checks a request the way the server does.
func verifyV2(t *testing.T, r *http.Request, body []byte) {
	t.Helper()
	if r.Header.Get("X-Signature-Version") != "2" {
		t.Fatalf("signature version = %q", r.Header.Get("X-Signature-Version"))
	}
	pub, _ := hex.DecodeString(r.Header.Get("X-Agent-Id"))
	sig, _ := hex.DecodeString(r.Header.Get("X-Signature"))
	payload := append([]byte(r.Header.Get("X-Timestamp")+"\n"+r.Method+"\n"+r.URL.RequestURI()+"\n"), body...)
	if !ed25519.Verify(pub, payload, sig) {
		t.Fatalf("signature does not verify for %s %s", r.Method, r.URL.RequestURI())
	}
}

func signedAgent(t *testing.T, h http.HandlerFunc) *Agent {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	kp, _ := GenerateKeypair()
	return NewAgent(kp.PublicKey, kp.PrivateKey, NewClient(&ClientOptions{BaseURL: srv.URL}))
}

func TestSignedRequestsVerifyLikeTheServer(t *testing.T) {
	var calls atomic.Int32
	a := signedAgent(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		verifyV2(t, r, body)
		calls.Add(1)
		switch {
		case r.URL.Path == "/memory":
			io.WriteString(w, `{"id":"m1","content":"hi","memory_type":"semantic"}`)
		case r.URL.Path == "/search":
			if r.URL.Query().Get("query") != "what & why?" {
				t.Errorf("query = %q", r.URL.Query().Get("query"))
			}
			io.WriteString(w, `{"results":[{"memory_id":"m1","score":0.9}]}`)
		case r.URL.Path == "/upload":
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Errorf("content type = %q", r.Header.Get("Content-Type"))
			}
			io.WriteString(w, `{"file_id":"f1","file_name":"a.txt","file_type":"text","file_size":2}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	ctx := context.Background()
	if _, err := a.Remember(ctx, "hi", &RememberOptions{Type: MemoryTypeSemantic}); err != nil {
		t.Fatal(err)
	}
	res, err := a.Recall(ctx, "what & why?", &RecallOptions{Mode: SearchKeyword})
	if err != nil || len(res) != 1 {
		t.Fatalf("recall = %v, %v", res, err)
	}
	if _, err := a.UploadFile(ctx, "a.txt", "text/plain", []byte("hi"), false); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.DeleteMemory(ctx, "m1"); !ok || err != nil {
		t.Fatalf("delete = %v, %v", ok, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestAPIKeyIsABearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer memro_sk_test" || r.Header.Get("X-Signature") != "" {
			t.Errorf("headers = %v", r.Header)
		}
		io.WriteString(w, `{"agent_id":"abc","auth":"api_key"}`)
	}))
	defer srv.Close()
	a, err := AgentFromAPIKey(context.Background(), "memro_sk_test", &ClientOptions{BaseURL: srv.URL})
	if err != nil || a.AgentID != "abc" {
		t.Fatalf("agent = %v, %v", a, err)
	}
	if _, err := AgentFromAPIKey(context.Background(), strings.Repeat("a", 64), &ClientOptions{BaseURL: srv.URL}); err == nil {
		t.Fatal("a non-API key was accepted")
	}
}

func TestRetriesRateLimitsButNotPOSTGatewayErrors(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/whoami" && n.Add(1) == 1:
			w.Header().Set("Retry-After", "0")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		case r.URL.Path == "/auth/whoami":
			io.WriteString(w, `{"agent_id":"abc"}`)
		default:
			n.Add(100)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	c := NewClient(&ClientOptions{BaseURL: srv.URL, APIKey: "memro_sk_x"})
	if who, err := c.WhoAmI(context.Background(), nil); err != nil || who.AgentID != "abc" {
		t.Fatalf("whoami = %v, %v", who, err)
	}
	if _, err := c.CreateMemory(context.Background(), map[string]any{"content": "x"}, nil); err == nil {
		t.Fatal("expected a 502")
	}
	if n.Load() != 102 {
		t.Fatalf("POST was retried: %d", n.Load())
	}
}

func TestDefaultsToMemroCloud(t *testing.T) {
	t.Setenv("MEMRO_BASE_URL", "")
	if got := NewClient(nil).BaseURL(); got != DefaultBaseURL {
		t.Fatalf("base URL = %q", got)
	}
}

func TestCreateIdentitySolvesTheServerProof(t *testing.T) {
	var sent struct {
		PublicKey string   `json:"public_key"`
		Pow       PowProof `json:"pow"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/pow":
			io.WriteString(w, `{"difficulty":8,"required":true}`)
		case "/identity":
			_ = json.NewDecoder(r.Body).Decode(&sent)
			io.WriteString(w, `{"agent_id":"x"}`)
		}
	}))
	defer srv.Close()
	id, err := NewClient(&ClientOptions{BaseURL: srv.URL}).CreateIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("memro-identity-v1:%s:%d:%s", sent.PublicKey, sent.Pow.Timestamp, sent.Pow.Nonce)))
	if sent.PublicKey != id.AgentID || sum[0] != 0 {
		t.Fatalf("proof %+v for %s does not meet 8 bits", sent.Pow, sent.PublicKey)
	}
}

func TestCreateIdentityWithoutPowEndpoint(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/pow" {
			http.Error(w, `{"error":"Identity not found"}`, http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	if _, err := NewClient(&ClientOptions{BaseURL: srv.URL}).CreateIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["pow"]; ok {
		t.Fatal("sent a proof to a server that has none")
	}
}

func TestRecallSendsSearchOptions(t *testing.T) {
	var got []string
	a := signedAgent(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		got = append(got, fmt.Sprintf("%s|%s|%s", q.Get("graph"), q.Get("adaptive_diversity"), q.Get("max_per_session")))
		io.WriteString(w, `{"results":[],"total":0}`)
	})
	off, on := false, true
	for _, opts := range []*RecallOptions{nil, {AdaptiveDiversity: &off}, {AdaptiveDiversity: &on, MaxPerSession: 2, Graph: "bounded"}} {
		if _, err := a.Recall(context.Background(), "how many", opts); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"||", "|false|", "bounded|true|2"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("query params = %q, want %q", got, want)
	}
}

func TestCreateMemoriesBatch(t *testing.T) {
	var received []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/memories/batch" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ids":   []string{"id-1", "id-2"},
			"count": 2,
			"memories": []map[string]any{
				{"id": "id-1", "content": "first"},
				{"id": "id-2", "content": "second"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(&ClientOptions{BaseURL: srv.URL, APIKey: "memro_sk_testkey"})
	agent := NewAgent("agent-123", "", c)
	ids, err := agent.RememberBatch(context.Background(), []BatchMemoryItem{
		{Content: "first", Opts: &RememberOptions{Type: MemoryTypeSemantic}},
		{Content: "second"},
	})
	if err != nil {
		t.Fatalf("RememberBatch: %v", err)
	}
	if len(ids) != 2 || ids[0] != "id-1" || ids[1] != "id-2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
	if len(received) != 2 || received[0]["content"] != "first" || received[1]["content"] != "second" {
		t.Fatalf("unexpected payload sent: %v", received)
	}
}
