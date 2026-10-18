package engine

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/domain"
)

func TestClassify(t *testing.T) {
	cases := map[string]domain.MemoryType{
		"I prefer dark mode":              domain.Profile,
		"Yesterday I went to the market":  domain.Episodic,
		"Paris is the capital of France":  domain.Semantic,
		"We worked on the launch all day": domain.Episodic,
	}
	for in, want := range cases {
		if got := Classify(in)[0].Type; got != want {
			t.Errorf("Classify(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDetectContradiction(t *testing.T) {
	if DetectContradiction("I love hiking", "I hate hiking") <= 0.5 {
		t.Error("sentiment flip not detected")
	}
	if DetectContradiction("The sky is blue", "Coffee is hot") > 0.5 {
		// "is" + "is" without "not" is not a contradiction
		t.Error("false positive")
	}
}

func TestResolve(t *testing.T) {
	now := time.Now()
	fresh := conflictSide{ID: uuid.New(), CreatedAt: now, Access: 10, Confidence: 1}
	stale := conflictSide{ID: uuid.New(), CreatedAt: now.Add(-60 * 24 * time.Hour), Access: 0, Confidence: 0.5}
	r := resolve(fresh, stale, now, DefaultWeights)
	if r.Resolved == nil || *r.Resolved != fresh.ID {
		t.Fatalf("expected fresh memory to win, got %+v", r)
	}
	same := conflictSide{ID: uuid.New(), CreatedAt: now, Access: 10, Confidence: 1}
	r = resolve(fresh, same, now, DefaultWeights)
	if r.Resolved != nil || r.ExpiresAt == nil {
		t.Fatalf("expected ambiguous result, got %+v", r)
	}
}

// The JSON shape must match the original Rust enum serialization.
func TestResolutionResultJSON(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	b, _ := json.Marshal(domain.ResolutionResult{Resolved: &id})
	if string(b) != `{"Resolved":"11111111-1111-1111-1111-111111111111"}` {
		t.Fatalf("resolved json = %s", b)
	}
	exp := "2026-01-01T00:00:00Z"
	b, _ = json.Marshal(domain.ResolutionResult{A: id, B: id, ExpiresAt: &exp})
	if !strings.HasPrefix(string(b), `{"Ambiguous":["11111111`) {
		t.Fatalf("ambiguous json = %s", b)
	}
	var back domain.ResolutionResult
	if err := json.Unmarshal(b, &back); err != nil || back.ExpiresAt == nil || *back.ExpiresAt != exp {
		t.Fatalf("round trip failed: %v %+v", err, back)
	}
}

func TestBFSWalks(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	nodes := []chainNode{{a, "A"}, {b, "B"}, {c, "C"}}
	edges := []domain.Relation{{From: a, To: b, Confidence: 0.9}, {From: b, To: c, Confidence: 0.5}}
	walks := bfsWalks(a, nodes, edges, 2)
	if len(walks) != 1 || len(walks[0].Path) != 3 {
		t.Fatalf("walks = %+v", walks)
	}
	if got := walks[0].Confidence; got < 0.449 || got > 0.451 {
		t.Fatalf("confidence = %v, want 0.45", got)
	}
}

func TestResultCacheInvalidation(t *testing.T) {
	c := NewResultCache()
	q := domain.SearchQuery{AgentID: "agent", Query: "x", Limit: 5}
	c.Set(q, uuid.Nil, &domain.SearchResponse{Total: 1})
	if _, ok := c.Get(q, uuid.Nil); !ok {
		t.Fatal("expected hit")
	}
	c.InvalidateAgent("agent")
	if _, ok := c.Get(q, uuid.Nil); ok {
		t.Fatal("expected miss after invalidation")
	}
}
