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
