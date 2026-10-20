package api_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

// Keyword search matches memories sharing some of a question's words, and
// max_per_session spreads results across conversations when asked.
func TestSearchFindsPartialMatchesAndSpreadsAcrossSessions(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	ctx := context.Background()
	search := func(q, extra string) []string {
		t.Helper()
		code, body := e.do(&a, "v2", "GET", "/search?query="+url.QueryEscape(q)+"&limit=3&mode=keyword"+extra, nil, nil)
		e.must(200, code, body, "search "+q)
		var r struct {
			Results []struct {
				Content string `json:"content"`
			} `json:"results"`
		}
		_ = json.Unmarshal(body, &r)
		var out []string
		for _, x := range r.Results {
			out = append(out, x.Content)
		}
		return out
	}

	e.remember(a, "The asylum decision arrived after fourteen months.", "episodic")
	if got := search("How long did I wait for the decision on my asylum application?", ""); len(got) != 1 {
		t.Fatalf("a question sharing some words must find the memory: %v", got)
	}
	if got := search(`it's "C++" & (a|b) : !x`, ""); got == nil {
		t.Log("punctuation-only query matched nothing (fine); the point is it did not error")
	}

	// Three memories of one conversation and one of another, all matching.
	s1, s2 := uuid.New(), uuid.New()
	for i, sid := range []uuid.UUID{s1, s1, s1, s2} {
		body := j(map[string]any{"content": []string{"deploy notes alpha", "deploy notes beta", "deploy notes gamma", "deploy notes delta"}[i],
			"memory_type": "episodic", "session_id": sid.String()})
		code, b := e.do(&a, "v2", "POST", "/memory", body, nil)
		e.must(200, code, b, "remember in session")
	}
	_ = ctx
	got := search("deploy notes", "&max_per_session=1")
	seenOther := false
	for _, c := range got[:2] {
		seenOther = seenOther || c == "deploy notes delta"
	}
	if !seenOther {
		t.Fatalf("with max_per_session=1 the second conversation must appear in the top 2: %v", got)
	}
	if got := search("deploy notes", ""); len(got) != 3 {
		t.Fatalf("no cap by default: %v", got)
	}
}
