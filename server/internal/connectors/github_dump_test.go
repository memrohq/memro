package connectors

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitHubDumpChunks writes the memories the GitHub connector would store for
// a dump made by bench/github_history/fetch.py, so the benchmark stores exactly
// what a synced repo holds:
//
//	MEMRO_GITHUB_DUMP=in.json MEMRO_GITHUB_CHUNKS=out.jsonl go test ./internal/connectors -run TestGitHubDumpChunks
//
// Without those variables it checks the same path on a small inline dump.
func TestGitHubDumpChunks(t *testing.T) {
	in, out := os.Getenv("MEMRO_GITHUB_DUMP"), os.Getenv("MEMRO_GITHUB_CHUNKS")
	if in == "" || out == "" {
		dir := t.TempDir()
		in, out = filepath.Join(dir, "in.json"), filepath.Join(dir, "out.jsonl")
		body := strings.Repeat("A paragraph about landing worktree branches. ", 60) + "\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)"
		dump := `[{"number": 92, "title": "CLI: memro land", "body": ` + jsonString(body) + `, "state": "closed",
			"html_url": "https://github.com/acme/api/pull/92", "comments": 1, "created_at": "2026-11-18T10:00:00Z",
			"user": {"login": "ana"}, "pull_request": {"merged_at": "2026-11-18T12:00:00Z"}, "_repo": "acme/api",
			"_comments": [{"body": "Looks good.", "created_at": "2026-11-18T11:00:00Z", "user": {"login": "ben"}}]}]`
		if err := os.WriteFile(in, []byte(dump), 0o600); err != nil {
			t.Fatal(err)
		}
		defer checkDumpChunks(t, out)
	}
	writeDumpChunks(t, in, out)
}

func writeDumpChunks(t *testing.T, in, out string) {
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

// checkDumpChunks: the inline PR is long enough for several chunks, each with
// the connector's header, the comment kept and the agent footer dropped.
func checkDumpChunks(t *testing.T, out string) {
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var all []string
	for s := bufio.NewScanner(f); s.Scan(); {
		var c struct{ Content string }
		if err := json.Unmarshal(s.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(c.Content, "[GitHub acme/api#92 · CLI: memro land]\n") {
			t.Fatalf("chunk without the header: %.80q", c.Content)
		}
		all = append(all, c.Content)
	}
	text := strings.Join(all, "\n")
	if len(all) < 2 || !strings.Contains(text, "ben commented on 2026-11-18: Looks good.") || strings.Contains(text, "Generated with") {
		t.Fatalf("%d chunks, want several with the comment and no footer:\n%s", len(all), text)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
