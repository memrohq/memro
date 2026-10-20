package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestVerifyLinearSignature(t *testing.T) {
	body := []byte(`{"action":"create","type":"Issue","data":{"id":"123"}}`)
	secret := "linear_webhook_secret_test"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	if !VerifyLinearSignature(secret, sig, body) {
		t.Fatal("valid signature rejected")
	}
	if VerifyLinearSignature("wrong_secret", sig, body) {
		t.Fatal("wrong secret accepted")
	}
	if VerifyLinearSignature(secret, "bad_signature", body) {
		t.Fatal("bad signature accepted")
	}
	if VerifyLinearSignature(secret, sig, []byte(`{"modified": true}`)) {
		t.Fatal("modified body accepted")
	}
	if VerifyLinearSignature("", sig, body) {
		t.Fatal("empty secret accepted")
	}
}

func TestRenderLinearIssue(t *testing.T) {
	created := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 10, 20, 15, 30, 0, 0, time.UTC)

	issue := &linearIssue{
		ID:          "iss-123",
		Identifier:  "ENG-412",
		Title:       "Fix database pool exhaustion",
		Description: "During blue-green deploys, worker pods exhaust database connections.",
		Priority:    2,
		State: linearState{
			Name: "In Progress",
			Type: "started",
		},
		Assignee: &linearUser{
			ID:   "u-1",
			Name: "Alex Developer",
		},
		Creator: &linearUser{
			ID:   "u-2",
			Name: "Sam Lead",
		},
		Project: &linearProject{
			ID:   "p-1",
			Name: "Reliability Q4",
		},
		Team: linearTeam{
			ID:   "t-1",
			Name: "Core Engineering",
			Key:  "ENG",
		},
		CreatedAt: created,
		UpdatedAt: updated,
		URL:       "https://linear.app/memro/issue/ENG-412/fix-database-pool-exhaustion",
	}

	comments := []linearComment{
		{
			ID:        "c-1",
			Body:      "Discovered after Sentry issue #9412 triggered an alert.",
			User:      &linearUser{Name: "Sam Lead"},
			CreatedAt: created.Add(time.Hour),
		},
		{
			ID:        "c-2",
			Body:      "PR opened in memrohq/memro#6 to increase pool limit and add leases.",
			User:      &linearUser{Name: "Alex Developer"},
			CreatedAt: updated,
		},
	}

	header, body, meta := renderLinearIssue(issue, comments)

	wantHeader := "[Linear ENG-412 · Fix database pool exhaustion · Core Engineering]"
	if header != wantHeader {
		t.Fatalf("got header %q, want %q", header, wantHeader)
	}

	for _, substr := range []string{
		"Issue: ENG-412",
		"Priority: High (2)",
		"Assignee: Alex Developer",
		"Project: Reliability Q4",
		"During blue-green deploys",
		"Discussion:",
		"Sam Lead: Discovered after Sentry issue #9412",
		"Alex Developer: PR opened in memrohq/memro#6",
	} {
		if !strings.Contains(body, substr) {
			t.Fatalf("body missing %q:\n%s", substr, body)
		}
	}

	if meta["identifier"] != "ENG-412" || meta["priority"] != "High" || meta["state"] != "In Progress" {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	it := linearItem(issue, comments)
	if it.Key != "issue:iss-123" {
		t.Fatalf("item key %q", it.Key)
	}
	if it.Header != header {
		t.Fatalf("item header mismatch")
	}
}
