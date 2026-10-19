package memro

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
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
