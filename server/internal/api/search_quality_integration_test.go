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

func TestSearchTemporalOrderingAndContradictions(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)

	t1 := "2023-01-10T10:00:00Z"
	t2 := "2023-08-20T14:00:00Z"

	// Memory 1: older statement
	body1 := j(map[string]any{
		"content":     "I lived in that apartment for 6 months",
		"memory_type": "episodic",
		"event_date":  t1,
	})
	code1, b1 := e.do(&a, "v2", "POST", "/memory", body1, nil)
	e.must(200, code1, b1, "remember older statement")

	// Memory 2: newer statement contradicting older
	body2 := j(map[string]any{
		"content":     "I lived in that apartment for 2 years",
		"memory_type": "episodic",
		"event_date":  t2,
	})
	code2, b2 := e.do(&a, "v2", "POST", "/memory", body2, nil)
	e.must(200, code2, b2, "remember newer statement")

	// Search keyword mode with resolve_conflicts=true and include_ledger=true
	code, body := e.do(&a, "v2", "GET", "/search?query="+url.QueryEscape("apartment")+
		"&mode=keyword&limit=5&resolve_conflicts=true&include_ledger=true&order=chronological", nil, nil)
	e.must(200, code, body, "search apartment")

	var resp struct {
		Results []struct {
			MemoryID     string  `json:"memory_id"`
			Content      string  `json:"content"`
			EventDate    *string `json:"event_date"`
			IsSuperseded bool    `json:"is_superseded"`
			SupersededBy *string `json:"superseded_by"`
		} `json:"results"`
		Ledger *string `json:"ledger"`
	}
	_ = json.Unmarshal(body, &resp)

	if len(resp.Results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(resp.Results))
	}

	// Because order=chronological, results[0] should be older (t1) and results[1] newer (t2)
	if resp.Results[0].Content != "I lived in that apartment for 6 months" {
		t.Errorf("expected chronological order, first item was: %s", resp.Results[0].Content)
	}

	// Older memory must be marked as superseded
	if !resp.Results[0].IsSuperseded {
		t.Errorf("expected older memory to have is_superseded=true")
	}
	if resp.Results[0].SupersededBy == nil || *resp.Results[0].SupersededBy != resp.Results[1].MemoryID {
		t.Errorf("expected superseded_by=%s, got %v", resp.Results[1].MemoryID, resp.Results[0].SupersededBy)
	}

	// Newer memory must NOT be marked as superseded
	if resp.Results[1].IsSuperseded {
		t.Errorf("expected newer memory to have is_superseded=false")
	}

	// Ledger should be generated and contain the ledger table
	if resp.Ledger == nil || *resp.Ledger == "" {
		t.Errorf("expected ledger markdown table to be present")
	}
}

