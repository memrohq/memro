package connectors

import (
	"strings"
	"testing"
)

func TestVerifyGitLabWebhookToken(t *testing.T) {
	secret := "gitlab_secret_token_abc123"

	if !VerifyGitLabWebhookToken(secret, secret) {
		t.Fatal("valid GitLab token was rejected")
	}

	if VerifyGitLabWebhookToken(secret, "wrong_token") {
		t.Fatal("wrong GitLab token was accepted")
	}

	if VerifyGitLabWebhookToken("", "some_token") {
		t.Fatal("empty expected token accepted")
	}
}

func TestRenderGitLabMR(t *testing.T) {
	mr := &gitlabMergeRequest{
		ID:           101,
		IID:          42,
		ProjectID:    5001,
		Title:        "Add distributed checkpointing for DCE",
		Description:  "Implements incremental checkpoints stored in Postgres.",
		State:        "opened",
		CreatedAt:    "2026-10-21T01:00:00Z",
		UpdatedAt:    "2026-10-21T03:30:00Z",
		SourceBranch: "feature/dce-checkpoints",
		TargetBranch: "main",
		WebURL:       "https://gitlab.com/memro/memro/-/merge_requests/42",
		Author: gitlabUser{
			ID:       10,
			Username: "alexdev",
			Name:     "Alex Developer",
		},
		Reviewers: []gitlabUser{
			{ID: 11, Username: "samlead", Name: "Sam Lead"},
		},
	}

	notes := []gitlabNote{
		{
			ID:        201,
			Body:      "Tested against mock servers, passes all concurrency checks.",
			Author:    gitlabUser{ID: 11, Username: "samlead", Name: "Sam Lead"},
			CreatedAt: "2026-10-21T03:45:00Z",
			System:    false,
		},
	}

	header, body, meta := renderGitLabMR(mr, notes, "memro/memro")

	if header != "[GitLab MR !42 · Add distributed checkpointing for DCE · memro/memro]" {
		t.Fatalf("unexpected header: %s", header)
	}

	if !strings.Contains(body, "feature/dce-checkpoints -> main") {
		t.Fatalf("expected branches in body, got: %s", body)
	}

	if !strings.Contains(body, "Sam Lead (@samlead)") {
		t.Fatalf("expected reviewer in body, got: %s", body)
	}

	if !strings.Contains(body, "Tested against mock servers") {
		t.Fatalf("expected discussion note in body, got: %s", body)
	}

	if meta["mr_iid"] != 42 || meta["state"] != "opened" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}

func TestRenderGitLabPipelineFailure(t *testing.T) {
	pipe := &gitlabPipeline{
		ID:        90210,
		ProjectID: 5001,
		Status:    "failed",
		Ref:       "main",
		SHA:       "a1b2c3d4e5f67890",
		WebURL:    "https://gitlab.com/memro/memro/-/pipelines/90210",
		CreatedAt: "2026-10-21T02:00:00Z",
		UpdatedAt: "2026-10-21T02:05:00Z",
		User: gitlabUser{
			ID:       10,
			Username: "alexdev",
			Name:     "Alex Developer",
		},
	}

	failedJobs := []string{"test:unit", "lint:golangci"}
	header, body, meta := renderGitLabPipelineFailure(pipe, failedJobs, "memro/memro")

	if !strings.Contains(header, "[GitLab Pipeline #90210") || !strings.Contains(header, "FAILED]") {
		t.Fatalf("unexpected header: %s", header)
	}

	if !strings.Contains(body, "test:unit") || !strings.Contains(body, "lint:golangci") {
		t.Fatalf("expected failed jobs in body, got: %s", body)
	}

	if meta["status"] != "failed" || meta["ref"] != "main" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}
