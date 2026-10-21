package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestVerifyJiraWebhookSignature(t *testing.T) {
	secret := "test_jira_secret_123"
	payload := []byte(`{"webhookEvent":"jira:issue_updated","issue":{"key":"ENG-101"}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	if !VerifyJiraWebhookSignature(secret, "sha256="+sig, payload) {
		t.Fatal("valid Jira webhook signature was rejected")
	}

	if VerifyJiraWebhookSignature(secret, "sha256=invalid", payload) {
		t.Fatal("invalid Jira webhook signature was accepted")
	}

	if VerifyJiraWebhookSignature("wrong_secret", "sha256="+sig, payload) {
		t.Fatal("wrong secret accepted")
	}
}

func TestRenderJiraIssue(t *testing.T) {
	issue := &jiraIssue{
		ID:  "10023",
		Key: "PROJ-456",
		Fields: jiraIssueFields{
			Summary:     "Migrate vector store to pgvector HNSW",
			Description: "We should switch IVFFlat index to HNSW for lower p99 latency.",
			Status: jiraStatus{
				Name: "In Progress",
			},
			Priority: &jiraPriority{
				Name: "High",
			},
			Project: jiraProject{
				ID:   "10",
				Key:  "PROJ",
				Name: "Infrastructure",
			},
			IssueType: jiraIssueType{
				Name: "Task",
			},
			Reporter: &jiraUser{
				AccountID:   "usr-1",
				DisplayName: "Sarah Architect",
			},
			Assignee: &jiraUser{
				AccountID:   "usr-2",
				DisplayName: "Bob Engineer",
			},
			Created: "2026-10-18T10:00:00.000+0000",
			Updated: "2026-10-20T14:30:00.000+0000",
		},
	}

	comments := []jiraComment{
		{
			ID: "c-1",
			Author: &jiraUser{
				AccountID:   "usr-2",
				DisplayName: "Bob Engineer",
			},
			Body:    "Linked to PR memrohq/memro#42 and GitHub issue #12.",
			Created: "2026-10-20T14:45:00.000+0000",
		},
	}

	header, body, meta := renderJiraIssue(issue, comments, "https://mycompany.atlassian.net")
	if header == "" || body == "" {
		t.Fatal("rendered Jira issue is empty")
	}
	if !strings.Contains(header, "[Jira PROJ-456") {
		t.Fatalf("expected '[Jira PROJ-456' in header, got: %s", header)
	}
	if !strings.Contains(body, "Sarah Architect") {
		t.Fatalf("expected creator name in body, got: %s", body)
	}
	if !strings.Contains(body, "Bob Engineer") {
		t.Fatalf("expected assignee name in body, got: %s", body)
	}
	if meta["issue_key"] != "PROJ-456" {
		t.Fatalf("expected meta issue_key 'PROJ-456', got %v", meta["issue_key"])
	}
}
