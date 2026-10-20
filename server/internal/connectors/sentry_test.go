package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestVerifySentrySignature(t *testing.T) {
	body := []byte(`{"action":"created","data":{"issue":{"id":"9412"}}}`)
	secret := "sentry_client_secret_test"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	if !VerifySentrySignature(secret, sig, body) {
		t.Fatal("valid signature rejected")
	}
	if VerifySentrySignature("wrong_secret", sig, body) {
		t.Fatal("wrong secret accepted")
	}
	if VerifySentrySignature(secret, "bad_signature", body) {
		t.Fatal("bad signature accepted")
	}
	if VerifySentrySignature(secret, sig, []byte(`{"modified": true}`)) {
		t.Fatal("modified body accepted")
	}
	if VerifySentrySignature("", sig, body) {
		t.Fatal("empty secret accepted")
	}
}

func TestRenderSentryIssue(t *testing.T) {
	first := time.Date(2026, 10, 19, 14, 0, 0, 0, time.UTC)
	last := time.Date(2026, 10, 21, 10, 30, 0, 0, time.UTC)

	issue := &sentryIssue{
		ID:        "9412",
		ShortID:   "MEMRO-42",
		Title:     "DBConnectionTimeout: pgx: pool exhausted",
		Culprit:   "server/internal/connectors/store.go in ClaimJobs",
		Level:     "error",
		Status:    "unresolved",
		Count:     142,
		UserCount: 28,
		FirstSeen: first,
		LastSeen:  last,
		Permalink: "https://sentry.io/organizations/memro/issues/9412/",
		Project: sentryProjectSummary{
			ID:   "101",
			Name: "Memro Backend",
			Slug: "memro-backend",
		},
	}

	event := &sentryEvent{
		ID:      "evt-1",
		Message: "pgx: pool exhausted (max 20 connections)",
		Tags: []sentryTag{
			{Key: "release", Value: "v0.1.4"},
			{Key: "environment", Value: "production"},
			{Key: "server_name", Value: "api-green-1"},
		},
		Entries: []sentryEntry{
			{
				Type: "exception",
				Data: struct {
					Values []sentryExceptionValue `json:"values"`
				}{
					Values: []sentryExceptionValue{
						{
							Type:  "DBConnectionTimeout",
							Value: "pgx: pool exhausted (max 20 connections)",
							Stacktrace: &sentryStacktrace{
								Frames: []sentryFrame{
									{
										Filename: "server/internal/connectors/jobs.go",
										Function: "Manager.worker",
										Lineno:   112,
										Context:  "err := m.processJob(ctx, j)",
									},
									{
										Filename: "server/internal/connectors/store.go",
										Function: "Manager.ClaimJobs",
										Lineno:   45,
										Context:  "tx, err := m.deps.Pool.Acquire(ctx)",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	header, body, meta := renderSentryIssue(issue, event)

	wantHeader := "[Sentry · memro-backend · Issue #9412 · DBConnectionTimeout: pgx: pool exhausted]"
	if header != wantHeader {
		t.Fatalf("got header %q, want %q", header, wantHeader)
	}

	for _, substr := range []string{
		"Issue: MEMRO-42 (ID: 9412)",
		"Culprit: server/internal/connectors/store.go in ClaimJobs",
		"Occurrences: 142 events across 28 users",
		"Exception: DBConnectionTimeout: pgx: pool exhausted (max 20 connections)",
		"at Manager.ClaimJobs in server/internal/connectors/store.go:45",
		"release: v0.1.4",
		"environment: production",
	} {
		if !strings.Contains(body, substr) {
			t.Fatalf("body missing %q:\n%s", substr, body)
		}
	}

	if meta["issue_id"] != "9412" || meta["project_slug"] != "memro-backend" || meta["level"] != "error" {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	it := sentryItem(issue, event)
	if it.Key != "issue:9412" {
		t.Fatalf("item key %q", it.Key)
	}
	if it.SourceID != "memro-backend" {
		t.Fatalf("item source_id %q", it.SourceID)
	}
}

func TestParseSentryNextCursor(t *testing.T) {
	link := `<https://sentry.io/api/0/projects/memro/memro-backend/issues/?cursor=100:0:1>; rel="previous"; results="false", <https://sentry.io/api/0/projects/memro/memro-backend/issues/?cursor=100:50:0>; rel="next"; results="true"`
	got := parseSentryNextCursor(link)
	if got != "100:50:0" {
		t.Fatalf("parseSentryNextCursor got %q, want %q", got, "100:50:0")
	}

	// When results="false", there is no next page
	noMore := `<...>; rel="next"; results="false"`
	if parseSentryNextCursor(noMore) != "" {
		t.Fatal("expected empty cursor when results is false")
	}
}
