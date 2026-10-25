package engine

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildGraphView(t *testing.T) {
	s1 := uuid.New()
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mems := []graphViewMemory{
		// Newest first, as the query returns them.
		{ID: d, Content: "Lone fact", MemoryType: "semantic", Confidence: 1, Tags: []string{"Solo"}, At: t0.Add(3 * time.Hour)},
		{ID: c, Content: "Deploy moved to Fridays", MemoryType: "episodic", Confidence: 0.5, Tags: []string{"deploy"}, SessionID: &s1, At: t0.Add(2 * time.Hour)},
		{ID: b, Content: "Deploy runs on GCP", MemoryType: "semantic", Confidence: 1, Tags: []string{"Deploy", "gcp", "deploy"}, SessionID: &s1, At: t0.Add(time.Hour)},
		{ID: a, Content: "Deploy runs on Thursdays", MemoryType: "episodic", Confidence: 1, Tags: []string{"deploy"}, SessionID: &s1, At: t0},
	}
	g := buildGraphView(mems, []graphViewRelation{{From: c, To: a, Type: "updates"}})

	kinds := map[string]string{}
	for _, n := range g.Nodes {
		kinds[n.ID] = n.Kind
	}
	if len(g.Nodes) != 5 || kinds["entity:deploy"] != "entity" {
		t.Fatalf("nodes = %+v, want 4 memories + entity:deploy", g.Nodes)
	}
	if _, ok := kinds["entity:solo"]; ok {
		t.Error("an entity mentioned once should be left out")
	}
	if _, ok := kinds["entity:gcp"]; ok {
		t.Error("an entity mentioned once should be left out")
	}

	count := map[string]int{}
	has := map[[3]string]bool{}
	for _, l := range g.Links {
		count[l.Type]++
		has[[3]string{l.Source, l.Target, l.Type}] = true
	}
	if count["mentions"] != 3 {
		t.Errorf("mentions links = %d, want 3 (duplicate tags count once)", count["mentions"])
	}
	if count["updates"] != 1 || !has[[3]string{c.String(), a.String(), "updates"}] {
		t.Errorf("updates link missing: %+v", g.Links)
	}
	// Session links chain a -> b -> c by time, not a clique.
	if count["same_session"] != 2 || !has[[3]string{a.String(), b.String(), "same_session"}] || !has[[3]string{b.String(), c.String(), "same_session"}] {
		t.Errorf("same_session links = %+v, want a->b and b->c", g.Links)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("  héllo   wörld  ", 20); got != "héllo wörld" {
		t.Errorf("got %q", got)
	}
	if got := truncateRunes("abcdef", 4); got != "abc…" {
		t.Errorf("got %q", got)
	}
}
