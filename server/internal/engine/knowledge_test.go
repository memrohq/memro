package engine

import (
	"encoding/json"
	"slices"
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
		// The four kinds of memory worth keeping (ADR 0012 follow-up).
		"Memro uses PostgreSQL for persistent storage.":                               domain.Semantic,
		"User prefers reviewing migrations before deployment.":                        domain.Profile,
		"User prefers TypeScript":                                                     domain.Profile,
		"The user always wants concise answers.":                                      domain.Profile,
		"I'm a backend engineer on the billing team":                                  domain.Profile,
		"To run integration tests, start PostgreSQL before executing the test suite.": domain.Procedural,
		"How to deploy: push to main, then promote the tested SHA.":                   domain.Procedural,
		"Run `make release` to publish the CLI.":                                      domain.Procedural,
		"Step 1: dump the billing tables.":                                            domain.Procedural,
		"The P0 billing isolation tests passed on the feature branch.":                domain.Episodic,
		"PR #6 was merged and 0.1.5 published to npm.":                                domain.Episodic,
		// States, not events, stay semantic.
		"Memro is deployed on one GCP VM in asia-south1.":        domain.Semantic,
		"The API went through Cloudflare before reaching Caddy.": domain.Semantic,
		"Billing lives only in memro-cloud.":                     domain.Semantic,
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
		t.Error("false positive on unrelated statements")
	}
	if DetectContradiction("Billing contact is Marco", "Billing contact is Ana") <= 0.5 {
		t.Error("slot-based entity conflict not detected")
	}
	if DetectContradiction("User prefers TypeScript", "User prefers Python") <= 0.5 {
		t.Error("preference slot conflict not detected")
	}
	if DetectContradiction("Use TypeScript for backend", "Don't use TypeScript for backend") <= 0.5 {
		t.Error("direct negation conflict not detected")
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

func TestBFSWalksIncludesShortDeadEndPaths(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	nodes := []chainNode{{ID: a, Content: "A"}, {ID: b, Content: "B"}}
	edges := []domain.Relation{{From: a, To: b, Confidence: 0.8}}
	walks := bfsWalks(a, nodes, edges, 2)
	if len(walks) != 1 {
		t.Fatalf("walks = %+v, want one short path ending at B", walks)
	}
	if len(walks[0].Path) != 2 || walks[0].Path[1] != b {
		t.Fatalf("path = %v, want A -> B", walks[0].Path)
	}
	if walks[0].Confidence < 0.799 || walks[0].Confidence > 0.801 {
		t.Fatalf("confidence = %v, want 0.8", walks[0].Confidence)
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

// Regression (LongMemEval run, 2026-10-21): with keyword hits weighted 0.8
// and each list cut to the limit before fusing, keyword's best match scored
// below semantic's 10th and hybrid search never returned keyword-only hits.
func TestFuseRRFKeepsTheBestOfEachList(t *testing.T) {
	ids := func(n int) []uuid.UUID {
		out := make([]uuid.UUID, n)
		for i := range out {
			out[i] = uuid.New()
		}
		return out
	}
	semantic, keyword := ids(50), ids(50)

	top, scores := fuseRRF(semantic, keyword, 10)
	if len(top) != 10 {
		t.Fatalf("len = %d, want 10", len(top))
	}
	in := func(id uuid.UUID) bool { return slices.Contains(top, id) }
	if !in(semantic[0]) || !in(keyword[0]) {
		t.Fatal("the best match of each list must be returned")
	}
	if in(semantic[9]) && !in(keyword[1]) {
		t.Fatal("semantic's 10th outranks keyword's 2nd: lists are not weighted equally")
	}

	// Found by both: first, with the top score.
	both := keyword[7]
	semantic[3] = both
	top, scores = fuseRRF(semantic, keyword, 10)
	if top[0] != both || scores[both] <= scores[semantic[0]] {
		t.Fatalf("an item in both lists should rank first, got %v (score %.3f)", top[0], scores[both])
	}
	if s, _ := fuseRRF([]uuid.UUID{both}, []uuid.UUID{both}, 1); scores == nil || len(s) != 1 {
		t.Fatal("single item")
	}
	if _, sc := fuseRRF([]uuid.UUID{both}, []uuid.UUID{both}, 1); sc[both] != 1 {
		t.Fatalf("first in both lists scores %v, want 1", sc[both])
	}

	// One side empty (keyword-only or semantic-only mode): order is kept.
	top, _ = fuseRRF(nil, keyword, 5)
	if !slices.Equal(top, keyword[:5]) {
		t.Fatal("a single list keeps its order")
	}
}

func TestPickDiverseSpreadsAcrossConversations(t *testing.T) {
	a1, a2, a3, b1, c1, solo1, solo2 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	order := []uuid.UUID{a1, a2, a3, b1, solo1, solo2, c1}
	group := map[uuid.UUID]string{a1: "A", a2: "A", a3: "A", b1: "B", c1: "C"} // solo: no session
	got := pickDiverse(order, group, 5, 1)
	want := []uuid.UUID{a1, b1, solo1, solo2, c1}
	if !slices.Equal(got, want) {
		t.Fatalf("one per conversation first, ungrouped memories stand alone: %v", got)
	}
	got = pickDiverse(order, group, 7, 1)
	if !slices.Equal(got[5:], []uuid.UUID{a2, a3}) {
		t.Fatalf("remaining places are filled in rank order: %v", got)
	}
	if got := pickDiverse(order, group, 4, 2); !slices.Equal(got, []uuid.UUID{a1, a2, b1, solo1}) {
		t.Fatalf("cap of two: %v", got)
	}
}
