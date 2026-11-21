package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func TestJiraIssueFromV3Response(t *testing.T) {
	// Jira's v3 API sends rich text as Atlassian Document Format and the
	// issue's comments under fields.comment.
	raw := `{"id":"1","key":"ENG-7","fields":{"summary":"Pool exhaustion","project":{"key":"ENG","name":"Engineering"},
		"description":{"type":"doc","version":1,"content":[
			{"type":"paragraph","content":[{"type":"text","text":"Pool runs out "},{"type":"mention","attrs":{"text":"@ana"}}]},
			{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"raise limit"}]}]}]}]},
		"comment":{"comments":[{"id":"9","author":{"displayName":"Bob"},"created":"2026-10-20T14:45:00.000+0000",
			"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Fixed in #42"}]}]}}]}}}`
	var issue jiraIssue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatal(err)
	}
	_, body, _ := renderJiraIssue(&issue, issue.Fields.Comment.Comments, "")
	for _, s := range []string{"Pool runs out @ana", "- raise limit", "Bob (2026-10-20 14:45):", "Fixed in #42"} {
		if !strings.Contains(body, s) {
			t.Fatalf("body missing %q:\n%s", s, body)
		}
	}
	if strings.Contains(body, "Pool exhaustion") {
		t.Fatalf("body repeats the title that the header already has:\n%s", body)
	}
}
