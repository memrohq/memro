package connectors

import (
	"encoding/json"
	"os"
	"testing"
)

// TestGitHubDumpChunks writes the memories the GitHub connector would store for
// a dump made by bench/github_history/fetch.py, so the benchmark stores exactly
// what a synced repo holds. Skipped unless MEMRO_GITHUB_DUMP is set:
//
//	MEMRO_GITHUB_DUMP=in.json MEMRO_GITHUB_CHUNKS=out.jsonl go test ./internal/connectors -run TestGitHubDumpChunks
func TestGitHubDumpChunks(t *testing.T) {
	in, out := os.Getenv("MEMRO_GITHUB_DUMP"), os.Getenv("MEMRO_GITHUB_CHUNKS")
	if in == "" || out == "" {
		t.Skip("MEMRO_GITHUB_DUMP and MEMRO_GITHUB_CHUNKS not set")
	}
	raw, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	var dump []struct {
		githubIssue
		Comments []githubComment `json:"_comments"`
		Repo     string          `json:"_repo"`
	}
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, d := range dump {
		header, body, meta := renderGitHubIssue(d.Repo, &d.githubIssue, d.Comments)
		it := item{Header: header, Body: body, Meta: meta}
		for _, c := range it.chunks() {
			if err := enc.Encode(map[string]any{"content": c.content, "metadata": meta, "event_date": d.CreatedAt}); err != nil {
				t.Fatal(err)
			}
		}
	}
}
