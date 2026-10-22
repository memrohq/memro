package engine

import (
	"testing"

	"github.com/memrohq/memro/server/internal/domain"
)

func TestIsAggregateQuery(t *testing.T) {
	for q, want := range map[string]bool{
		"How many times did I bake something in the past two weeks?": true,
		"What is the total amount I earned at the markets?":          true,
		"List the books I mentioned":                                 true,
		"Which restaurants have we both liked?":                      true,
		"What time did I reach the clinic on Monday?":                false,
		"Where do I live now?":                                       false,
		"Is the listing still up?":                                   false, // "list" only as a whole word
		"What did I eat at Totally Thai?":                            false,
	} {
		if got := IsAggregateQuery(q); got != want {
			t.Errorf("IsAggregateQuery(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestPerSessionFor(t *testing.T) {
	e := &Engine{}
	agg := domain.SearchQuery{Query: "How many siblings do I have?"}
	plain := domain.SearchQuery{Query: "Where do I live?"}
	if got := e.perSessionFor(agg); got != 0 {
		t.Fatalf("off by default, got %d", got)
	}
	e.SetAggregatePerSession(3)
	if got := e.perSessionFor(agg); got != 3 {
		t.Fatalf("aggregate question: got %d, want 3", got)
	}
	if got := e.perSessionFor(plain); got != 0 {
		t.Fatalf("other questions are left alone, got %d", got)
	}
	agg.PerSession = 1
	if got := e.perSessionFor(agg); got != 1 {
		t.Fatalf("the caller's max_per_session wins, got %d", got)
	}
	e.SetAggregatePerSession(-2)
	if got := e.perSessionFor(domain.SearchQuery{Query: "how many"}); got != 0 {
		t.Fatalf("negative turns it off, got %d", got)
	}
}
