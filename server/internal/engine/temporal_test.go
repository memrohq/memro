package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/domain"
)

func TestContradictionDetectionDurations(t *testing.T) {
	a := "I lived in that apartment for 6 months"
	b := "I lived in that apartment for 2 years"
	conf := DetectContradiction(a, b)
	if conf < 0.7 {
		t.Fatalf("expected duration contradiction conf >= 0.7, got %f", conf)
	}

	c := "I have 50 coins in my collection"
	d := "I have 75 coins in my collection"
	confCD := DetectContradiction(c, d)
	if confCD < 0.7 {
		t.Fatalf("expected quantity contradiction conf >= 0.7, got %f", confCD)
	}
}

func TestResolveSearchContradictions(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	t1 := "2023-01-15T10:00:00Z"
	t2 := "2023-07-20T14:30:00Z"

	results := []domain.SearchResult{
		{
			MemoryID:     id1.String(),
			Content:      "I lived in that apartment for 6 months",
			Score:        0.9,
			EventDate:    &t1,
			DocumentDate: &t1,
		},
		{
			MemoryID:     id2.String(),
			Content:      "I lived in that apartment for 2 years",
			Score:        0.85,
			EventDate:    &t2,
			DocumentDate: &t2,
		},
	}

	resolveSearchContradictions(results, nil)

	if !results[0].IsSuperseded {
		t.Errorf("expected older memory %s to be superseded", id1)
	}
	if results[0].SupersededBy == nil || *results[0].SupersededBy != id2.String() {
		t.Errorf("expected superseded_by to be %s, got %v", id2, results[0].SupersededBy)
	}
	if results[1].IsSuperseded {
		t.Errorf("expected newer memory %s to NOT be superseded", id2)
	}
}

func TestResolveSearchContradictionsExplicitRelation(t *testing.T) {
	idOld := uuid.New()
	idNew := uuid.New()

	results := []domain.SearchResult{
		{
			MemoryID: idOld.String(),
			Content:  "The project lead is Marco",
			Score:    0.9,
		},
		{
			MemoryID: idNew.String(),
			Content:  "The project lead is Ana",
			Score:    0.8,
		},
	}

	// In memory_relations: source updates target (idNew updates idOld)
	relations := []domain.Relation{
		{
			From: idNew,
			To:   idOld,
			Type: domain.RelUpdates,
		},
	}

	resolveSearchContradictions(results, relations)

	if !results[0].IsSuperseded {
		t.Errorf("expected idOld to be superseded by explicit relation")
	}
	if results[0].SupersededBy == nil || *results[0].SupersededBy != idNew.String() {
		t.Errorf("expected superseded_by %s, got %v", idNew, results[0].SupersededBy)
	}
}

func TestRenderLedgerFromResults(t *testing.T) {
	anchor, _ := time.Parse(time.RFC3339, "2023-05-20T00:00:00Z")
	d1 := "2023-01-10T12:00:00Z"
	d2 := "2023-02-15T15:00:00Z"

	results := []domain.SearchResult{
		{
			MemoryID:  "mem-1",
			Content:   "Signed the apartment lease",
			EventDate: &d1,
		},
		{
			MemoryID:     "mem-2",
			Content:      "Purchased the furniture",
			EventDate:    &d2,
			IsSuperseded: true,
		},
	}

	ledger := RenderLedgerFromResults(anchor, results)
	if ledger == "" {
		t.Fatal("expected rendered ledger, got empty string")
	}

	if !strings.Contains(ledger, "Deterministic Temporal Ledger") {
		t.Errorf("missing ledger title in:\n%s", ledger)
	}
	if !strings.Contains(ledger, "Signed the apartment lease") {
		t.Errorf("missing mem-1 in ledger:\n%s", ledger)
	}
	if !strings.Contains(ledger, "[SUPERSEDED] Purchased the furniture") {
		t.Errorf("missing superseded marker in ledger:\n%s", ledger)
	}
}

func TestDetectQueryIntent(t *testing.T) {
	cases := []struct {
		query  string
		expect string
	}{
		{"How often do I play tennis with my friends at the local park previously? How often do I play now?", "evolution"},
		{"Did I mostly recently increase or decrease the limit on coffee?", "evolution"},
		{"For coffee ratio, did I switch to more water or less?", "evolution"},
		{"What day of the week did I meet with my previous language exchange tutor Juan?", "historical"},
		{"What was my former manager Rachel's team size?", "historical"},
		{"What is the total amount of money I earned from selling products at the markets?", "aggregation"},
		{"How many sports have I played competitively in the past?", "historical"}, // "in the past" marks historical
		{"How many tops have I bought from H&M?", "aggregation"},
		{"How long have I been living in my current apartment in Harajuku?", "current"},
		{"How many pages of A Short History have I read so far?", "current"},
		{"What brand of BBQ sauce am I currently obsessed with?", "current"},
		{"Tell me about Algernon's backstory", "multi_hop"},
	}

	for _, c := range cases {
		got := DetectQueryIntent(c.query)
		if got != c.expect {
			t.Errorf("query %q: expected intent %q, got %q", c.query, c.expect, got)
		}
	}
}

func TestResolveSearchContradictionsAggregation(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	t1 := "2023-01-15T10:00:00Z"
	t2 := "2023-07-20T14:30:00Z"

	results := []domain.SearchResult{
		{
			MemoryID:     id1.String(),
			Content:      "I earned $400 at the Downtown Farmers Market",
			Score:        0.9,
			EventDate:    &t1,
			DocumentDate: &t1,
		},
		{
			MemoryID:     id2.String(),
			Content:      "I earned $350 at the Uptown Farmers Market",
			Score:        0.85,
			EventDate:    &t2,
			DocumentDate: &t2,
		},
	}

	// In aggregation mode, differing amounts across dates should NOT supersede
	resolveSearchContradictions(results, nil, true)

	if results[0].IsSuperseded {
		t.Errorf("expected older memory %s to NOT be superseded in aggregation mode", id1)
	}
	if results[1].IsSuperseded {
		t.Errorf("expected newer memory %s to NOT be superseded in aggregation mode", id2)
	}
}

func TestContradictionDetectionFalsePositives(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"I ran 5 miles at the park today", "I walked 2 miles at the park yesterday"},
		{"User: I bought 3 plants for the garden", "User: the garden needs 2 hours of watering"},
		{"User: hi there", "User: how are you"},
	} {
		if c := DetectContradiction(tc.a, tc.b); c >= 0.7 {
			t.Errorf("DetectContradiction(%q, %q) = %v, want < 0.7", tc.a, tc.b, c)
		}
	}
}

func TestContradictionDetectionKeyValue(t *testing.T) {
	if c := DetectContradiction("Favorite color: blue", "Favorite color: red"); c < 0.7 {
		t.Errorf("key: value conflict = %v, want >= 0.7", c)
	}
	if c := DetectContradiction("Favorite color: blue", "Favorite color: light blue"); c >= 0.7 {
		t.Errorf("refined value = %v, want < 0.7", c)
	}
}

func TestResolveSearchContradictionsBackwardsEdge(t *testing.T) {
	idOld, idNew := uuid.New(), uuid.New()
	tOld, tNew := "2023-01-15T10:00:00Z", "2023-07-20T14:30:00Z"
	results := []domain.SearchResult{
		{MemoryID: idNew.String(), Content: "Team offsite is in Lisbon", EventDate: &tNew},
		{MemoryID: idOld.String(), Content: "Remote work days are Fridays", EventDate: &tOld},
	}
	// Written by an old Evolve run in arbitrary order: older -> newer.
	relations := []domain.Relation{{From: idOld, To: idNew, Type: domain.RelUpdates}}
	resolveSearchContradictions(results, relations)
	if results[0].IsSuperseded {
		t.Error("newer memory marked superseded by a backwards edge")
	}
	if !results[1].IsSuperseded || results[1].SupersededBy == nil || *results[1].SupersededBy != idNew.String() {
		t.Errorf("older memory = %+v, want superseded by %s", results[1], idNew)
	}
}

func TestCacheKeyCoversResponseFields(t *testing.T) {
	tenant := uuid.New()
	a1, a2 := time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	base := domain.SearchQuery{AgentID: "agent", Query: "x", Limit: 5, IncludeLedger: true}
	variants := []domain.SearchQuery{base, base, base, base, base}
	variants[1].AnchorDate = &a1
	variants[2].AnchorDate = &a2
	variants[3].AdaptiveDiversity = true
	variants[4].AdaptiveDiversity, variants[4].AggregatePerSession = true, 2
	seen := map[string]int{}
	for i, q := range variants {
		k := cacheKey(q, tenant)
		if j, ok := seen[k]; ok {
			t.Errorf("variants %d and %d share a cache key", j, i)
		}
		seen[k] = i
	}
}
