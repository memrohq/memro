package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/migrations"
)

func TestVerifyWebhookTimestamp(t *testing.T) {
	now := time.Now()

	// Inside window
	if !VerifyWebhookTimestamp(now, now) {
		t.Fatal("exact current time was rejected")
	}
	if !VerifyWebhookTimestamp(now.Add(-4*time.Minute), now) {
		t.Fatal("timestamp 4 minutes ago was rejected")
	}
	if !VerifyWebhookTimestamp(now.Add(4*time.Minute), now) {
		t.Fatal("timestamp 4 minutes in future was rejected")
	}

	// Outside window (stale / far future)
	if VerifyWebhookTimestamp(now.Add(-6*time.Minute), now) {
		t.Fatal("stale timestamp 6 minutes ago was accepted")
	}
	if VerifyWebhookTimestamp(now.Add(-24*time.Hour), now) {
		t.Fatal("stale timestamp 24 hours ago was accepted")
	}
	if VerifyWebhookTimestamp(now.Add(6*time.Minute), now) {
		t.Fatal("future timestamp 6 minutes ahead was accepted")
	}
}

func TestGitHubWebhookReplay(t *testing.T) {
	ctx := context.Background()
	secret := "gh-secret"
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	m, err := New(Deps{Config: config.Config{
		Connectors: config.ConnectorsConfig{
			GitHubAppID:         "123",
			GitHubAppSlug:       "memro",
			GitHubClientID:      "client",
			GitHubClientSecret:  "secret",
			GitHubWebhookSecret: secret,
			GitHubAppPrivateKey: pemKey,
			EncryptionKey:       key32(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"opened","installation":{"id":123}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	hdr := http.Header{}
	hdr.Set("X-Hub-Signature-256", sig)
	hdr.Set("X-GitHub-Event", "issues")
	hdr.Set("X-GitHub-Delivery", "delivery-uuid-1")

	// First delivery: claimed and processed
	code := m.GitHubWebhook(ctx, hdr, body)
	if code != http.StatusOK {
		t.Fatalf("first delivery returned %d, want 200", code)
	}

	// Replayed delivery with same X-GitHub-Delivery: acknowledged without re-processing
	code = m.GitHubWebhook(ctx, hdr, body)
	if code != http.StatusOK {
		t.Fatalf("replayed delivery returned %d, want 200", code)
	}

	// Directly verify ClaimWebhookDelivery returns false for replayed ID
	claimed, err := m.ClaimWebhookDelivery(ctx, ProviderGitHub, "delivery-uuid-1")
	if err != nil || claimed {
		t.Fatalf("expected claim=false for duplicate delivery, got claimed=%v err=%v", claimed, err)
	}
	// Different delivery ID is accepted
	claimed, err = m.ClaimWebhookDelivery(ctx, ProviderGitHub, "delivery-uuid-2")
	if err != nil || !claimed {
		t.Fatalf("expected claim=true for new delivery, got claimed=%v err=%v", claimed, err)
	}
}

func TestLinearWebhookReplayAndTimestamp(t *testing.T) {
	ctx := context.Background()
	secret := "linear-secret"
	m, err := New(Deps{Config: config.Config{
		Connectors: config.ConnectorsConfig{
			LinearClientID:      "client",
			LinearClientSecret:  "secret",
			LinearWebhookSecret: secret,
			EncryptionKey:       key32(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	makePayload := func(webhookID string, ts time.Time) ([]byte, http.Header) {
		p := map[string]any{
			"action":           "create",
			"type":             "Issue",
			"organizationId":   "org-1",
			"webhookId":        webhookID,
			"webhookTimestamp": ts.UnixMilli(),
			"data":             map[string]any{"id": "issue-1"},
		}
		b, _ := json.Marshal(p)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(b)
		h := http.Header{}
		h.Set("Linear-Signature", hex.EncodeToString(mac.Sum(nil)))
		return b, h
	}

	// 1. Fresh timestamp and new delivery ID
	b, h := makePayload("linear-del-1", now)
	code := m.LinearWebhook(ctx, h, b)
	if code != http.StatusOK {
		t.Fatalf("fresh webhook returned %d, want 200", code)
	}

	// 2. Replayed delivery ID
	bReplay, hReplay := makePayload("linear-del-1", now)
	code = m.LinearWebhook(ctx, hReplay, bReplay)
	if code != http.StatusOK {
		t.Fatalf("replayed webhook returned %d, want 200", code)
	}

	// 3. Stale timestamp (10 minutes ago)
	bStale, hStale := makePayload("linear-del-2", now.Add(-10*time.Minute))
	code = m.LinearWebhook(ctx, hStale, bStale)
	if code != http.StatusUnauthorized {
		t.Fatalf("stale webhook returned %d, want 401 Unauthorized", code)
	}

	// 4. Far future timestamp (10 minutes ahead)
	bFuture, hFuture := makePayload("linear-del-3", now.Add(10*time.Minute))
	code = m.LinearWebhook(ctx, hFuture, bFuture)
	if code != http.StatusUnauthorized {
		t.Fatalf("future webhook returned %d, want 401 Unauthorized", code)
	}
}

func TestSentryWebhookReplayAndTimestamp(t *testing.T) {
	ctx := context.Background()
	secret := "sentry-secret"
	m, err := New(Deps{Config: config.Config{
		Connectors: config.ConnectorsConfig{
			SentryClientID:      "client",
			SentryClientSecret:  "secret",
			SentryWebhookSecret: secret,
			EncryptionKey:       key32(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	makeRequest := func(reqID string, ts time.Time) ([]byte, http.Header) {
		body := []byte(`{"action":"created","data":{"issue":{"id":"101","project":{"slug":"api"}}}}`)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		h := http.Header{}
		h.Set("Sentry-Hook-Signature", hex.EncodeToString(mac.Sum(nil)))
		if reqID != "" {
			h.Set("Request-ID", reqID)
		}
		if !ts.IsZero() {
			h.Set("Sentry-Hook-Timestamp", strconv.FormatInt(ts.Unix(), 10))
		}
		return body, h
	}

	// 1. Fresh timestamp and new Request-ID
	b, h := makeRequest("sentry-req-1", now)
	code := m.SentryWebhook(ctx, h, b)
	if code != http.StatusOK {
		t.Fatalf("fresh sentry webhook returned %d, want 200", code)
	}

	// 2. Replayed Request-ID
	bReplay, hReplay := makeRequest("sentry-req-1", now)
	code = m.SentryWebhook(ctx, hReplay, bReplay)
	if code != http.StatusOK {
		t.Fatalf("replayed sentry webhook returned %d, want 200", code)
	}

	// 3. Stale timestamp (15 minutes ago)
	bStale, hStale := makeRequest("sentry-req-2", now.Add(-15*time.Minute))
	code = m.SentryWebhook(ctx, hStale, bStale)
	if code != http.StatusUnauthorized {
		t.Fatalf("stale sentry webhook returned %d, want 401 Unauthorized", code)
	}
}

func TestWebhookDeliveryTTLCleanup(t *testing.T) {
	ctx := context.Background()
	m, err := New(Deps{Config: config.Config{}})
	if err != nil {
		t.Fatal(err)
	}

	// Insert an old delivery directly in memory
	m.deliveriesMu.Lock()
	m.deliveries["github:old-delivery"] = m.now().Add(-48 * time.Hour)
	m.deliveries["github:recent-delivery"] = m.now().Add(-1 * time.Hour)
	m.deliveriesMu.Unlock()

	cleaned, err := m.CleanWebhookDeliveries(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("expected 1 delivery cleaned, got %d", cleaned)
	}

	m.deliveriesMu.Lock()
	_, oldExists := m.deliveries["github:old-delivery"]
	_, recentExists := m.deliveries["github:recent-delivery"]
	m.deliveriesMu.Unlock()

	if oldExists {
		t.Fatal("old delivery was not cleaned")
	}
	if !recentExists {
		t.Fatal("recent delivery was unexpectedly cleaned")
	}
}

func TestPostgresWebhookDeduplicationAndTTL(t *testing.T) {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MEMRO_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool, err := db.Open(ctx, url, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}

	m, err := New(Deps{
		Pool: pool,
		Config: config.Config{
			Connectors: config.ConnectorsConfig{
				EncryptionKey: key32(),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	testDelivID := "pg-test-" + uuid.NewString()

	// 1. Claim delivery
	claimed, err := m.ClaimWebhookDelivery(ctx, ProviderGitHub, testDelivID)
	if err != nil || !claimed {
		t.Fatalf("first claim failed: claimed=%v, err=%v", claimed, err)
	}

	// 2. Replay delivery - must return claimed=false, err=nil
	claimed, err = m.ClaimWebhookDelivery(ctx, ProviderGitHub, testDelivID)
	if err != nil || claimed {
		t.Fatalf("replayed claim should be false: claimed=%v, err=%v", claimed, err)
	}

	// 3. Insert an old delivery and verify TTL cleanup
	oldDelivID := "pg-old-" + uuid.NewString()
	_, err = pool.Exec(ctx, `INSERT INTO connector_webhook_deliveries (provider, delivery_id, received_at) VALUES ($1, $2, $3)`,
		ProviderGitHub, oldDelivID, time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	cleaned, err := m.CleanWebhookDeliveries(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if cleaned < 1 {
		t.Fatalf("expected at least 1 row cleaned, got %d", cleaned)
	}

	// Verify oldDelivID is gone
	var count int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_webhook_deliveries WHERE provider = $1 AND delivery_id = $2`, ProviderGitHub, oldDelivID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected old delivery to be deleted, found count=%d", count)
	}

	// Verify testDelivID is still present
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_webhook_deliveries WHERE provider = $1 AND delivery_id = $2`, ProviderGitHub, testDelivID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected recent delivery to remain, found count=%d", count)
	}
}
