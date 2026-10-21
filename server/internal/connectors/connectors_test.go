package connectors

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/memrohq/memro/server/config"
)

func key32() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func TestSealerRoundTripAndRotation(t *testing.T) {
	oldKey, newKey := key32(), key32()
	old, err := newSealer(oldKey, "")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := old.Seal(map[string]string{"bot_token": "xoxb-secret"}, "conn-1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "xoxb-secret") {
		t.Fatal("token stored in plaintext")
	}

	var out map[string]string
	if stale, err := old.Open(sealed, "conn-1", &out); err != nil || stale || out["bot_token"] != "xoxb-secret" {
		t.Fatalf("open: %v stale=%v %v", err, stale, out)
	}
	// Bound to the connection: cannot be replayed onto another one.
	if _, err := old.Open(sealed, "conn-2", &out); err == nil {
		t.Fatal("opened with the wrong associated data")
	}
	// Tampering is detected.
	bad := append([]byte{}, sealed...)
	bad[len(bad)-1] ^= 1
	if _, err := old.Open(bad, "conn-1", &out); err == nil {
		t.Fatal("tampered value opened")
	}
	// After rotation the old value still opens and is reported stale.
	rotated, err := newSealer(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if stale, err := rotated.Open(sealed, "conn-1", &out); err != nil || !stale {
		t.Fatalf("rotated open: %v stale=%v", err, stale)
	}
	// Without the previous key it cannot.
	only, _ := newSealer(newKey, "")
	if _, err := only.Open(sealed, "conn-1", &out); err == nil {
		t.Fatal("opened without the key")
	}
	if _, err := newSealer("short", ""); err == nil {
		t.Fatal("accepted a short key")
	}
}

func slackHeaders(secret string, ts time.Time, body []byte) http.Header {
	s := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + s + ":"))
	mac.Write(body)
	h := http.Header{}
	h.Set("X-Slack-Request-Timestamp", s)
	h.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	return h
}

func TestVerifySlackSignature(t *testing.T) {
	now := time.Now()
	body := []byte(`{"type":"event_callback"}`)
	if !VerifySlackSignature("s3cret", slackHeaders("s3cret", now, body), body, now) {
		t.Fatal("valid signature rejected")
	}
	if VerifySlackSignature("other", slackHeaders("s3cret", now, body), body, now) {
		t.Fatal("wrong secret accepted")
	}
	if VerifySlackSignature("s3cret", slackHeaders("s3cret", now, body), []byte(`{"type":"x"}`), now) {
		t.Fatal("modified body accepted")
	}
	if VerifySlackSignature("s3cret", slackHeaders("s3cret", now.Add(-10*time.Minute), body), body, now) {
		t.Fatal("replayed (stale) request accepted")
	}
	if VerifySlackSignature("s3cret", http.Header{}, body, now) {
		t.Fatal("unsigned request accepted")
	}
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte("hook"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyGitHubSignature("hook", sig, body) {
		t.Fatal("valid signature rejected")
	}
	for _, bad := range []string{"", "sha1=abc", sig[:len(sig)-2] + "00"} {
		if VerifyGitHubSignature("hook", bad, body) {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestFormatSlackText(t *testing.T) {
	names := map[string]string{"U1": "ana"}
	got := formatSlackText("hey <@U1> and <@U2|bob>, see <#C1|eng> &amp; <https://x.io|docs> or <https://y.io> <!here> <mailto:a@b.c|a@b.c> 1 &lt; 2",
		func(id string) string {
			if n, ok := names[id]; ok {
				return n
			}
			return id
		})
	want := "hey @ana and @bob, see #eng & docs (https://x.io) or https://y.io @here a@b.c 1 < 2"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestTSLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"", "1.0", true}, {"1.0", "", false}, {"", "", false},
		{"1700000000.000100", "1700000000.000200", true},
		{"1700000000.0002", "1700000000.000100", false},
		{"999999999.9", "1700000000.0", true},
		{"1700000000.000100", "1700000000.000100", false},
	}
	for _, c := range cases {
		if got := tsLess(c.a, c.b); got != c.want {
			t.Errorf("tsLess(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestRenderSlackDoc(t *testing.T) {
	doc := &slackDoc{Channel: "C1", ChannelName: "eng", Day: "2023-11-14", Messages: map[string]slackEntry{
		"1700000000.000100": {Name: "ana", Text: "deploy is blocked", V: "1700000000.000100"},
		"1700000060.000100": {Name: "bob", Text: "on it", Thread: "1700000000.000100", V: "1700000060.000100"},
		"1700000030.000100": {Name: "cy", Text: "lunch?", V: "1700000030.000100"},
		"1700000040.000100": {Name: "cy", Deleted: true, V: "1700000099.000000"},
		"1700000050.000100": {Name: "di", Text: "orphan reply", Thread: "1699999999.000000", V: "1700000050.000100"},
	}}
	got, first := renderSlackDoc(doc)
	lines := strings.Split(got, "\n")
	if len(lines) != 6 || !strings.Contains(lines[0], "↳") || !strings.HasSuffix(lines[0], "di: orphan reply") ||
		!strings.HasSuffix(lines[2], "ana: deploy is blocked") || lines[3] != "↳ 22:14 bob: on it" ||
		!strings.HasSuffix(lines[5], "cy: lunch?") || lines[1] != "" || lines[4] != "" {
		t.Fatalf("unexpected render:\n%s", got)
	}
	if first.Unix() != 1699999999 {
		t.Fatalf("first = %v", first)
	}
	if body, _ := renderSlackDoc(&slackDoc{Messages: map[string]slackEntry{"1.0": {Deleted: true}}}); body != "" {
		t.Fatalf("deleted-only doc rendered %q", body)
	}
}

func TestRenderGitHubIssue(t *testing.T) {
	merged := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	is := &githubIssue{Number: 42, Title: "Fix login", Body: "Body text\n<!-- template hint -->", State: "closed",
		HTMLURL: "https://github.com/o/r/pull/42", CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		User: githubUser{Login: "ana"}}
	is.PullRequest = &struct {
		MergedAt *time.Time `json:"merged_at"`
	}{&merged}
	header, body, meta := renderGitHubIssue("o/r", is, []githubComment{
		{Body: "LGTM", User: githubUser{Login: "bob"}, CreatedAt: merged},
		{Body: "Coverage 90%", User: githubUser{Login: "ci[bot]", Type: "Bot"}},
	})
	if header != "[GitHub o/r#42 · Fix login]" {
		t.Fatalf("header %q", header)
	}
	for _, s := range []string{"Pull request #42: Fix login", "State: merged", "Body text", "bob commented on 2026-10-02: LGTM"} {
		if !strings.Contains(body, s) {
			t.Fatalf("body missing %q:\n%s", s, body)
		}
	}
	if strings.Contains(body, "template hint") || strings.Contains(body, "Coverage") {
		t.Fatalf("body has template comments or bot comments:\n%s", body)
	}
	if meta["kind"] != "pull_request" || meta["state"] != "merged" {
		t.Fatalf("meta %v", meta)
	}
}

func TestItemChunksCarryHeader(t *testing.T) {
	it := item{Header: "[Slack #eng · 2026-10-20 · Acme]", Body: strings.Repeat("A sentence about deploys. ", 200)}
	chunks := it.chunks()
	if len(chunks) < 2 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if !strings.HasPrefix(c.content, it.Header+"\n") {
			t.Fatalf("chunk without header: %.60q", c.content)
		}
		if len([]rune(c.content)) > 10_000 {
			t.Fatal("chunk exceeds memory content limit")
		}
	}
	if again := it.chunks(); again[0].hash != chunks[0].hash {
		t.Fatal("chunk hashes are not stable")
	}
}

func TestGitHubAppJWT(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	cfg := config.Config{Connectors: config.ConnectorsConfig{GitHubAppID: "12345", GitHubAppPrivateKey: pemKey}}
	m, err := New(Deps{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := m.github.appJWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q", jwt)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("signature: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Iss      string
		Iat, Exp int64
	}
	_ = json.Unmarshal(raw, &claims)
	if claims.Iss != "12345" || claims.Exp-claims.Iat > 600 || claims.Exp <= time.Now().Unix() {
		t.Fatalf("claims %+v", claims)
	}

	// PKCS#8 keys load too; garbage does not.
	p8, _ := x509.MarshalPKCS8PrivateKey(key)
	if _, err := parseRSAKey(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: p8}))); err != nil {
		t.Fatal(err)
	}
	cfg.Connectors.GitHubAppPrivateKey = "not a key"
	if _, err := New(Deps{Config: cfg}); err == nil {
		t.Fatal("accepted a malformed private key")
	}
}

func TestProvidersDisabledWithoutConfig(t *testing.T) {
	m, err := New(Deps{Config: config.Config{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range m.Providers() {
		if p.Enabled {
			t.Fatalf("%s enabled without credentials", p.Name)
		}
	}
	// Credentials without an encryption key are not enough.
	m, _ = New(Deps{Config: config.Config{Connectors: config.ConnectorsConfig{
		SlackClientID: "a", SlackClientSecret: "b", SlackSigningSecret: "c"}}})
	if m.slack.enabled() {
		t.Fatal("slack enabled without an encryption key")
	}
}

func TestBackoff(t *testing.T) {
	if backoff(1) != 30*time.Second || backoff(2) != time.Minute || backoff(20) != time.Hour {
		t.Fatalf("backoff: %v %v %v", backoff(1), backoff(2), backoff(20))
	}
}
