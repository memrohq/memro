package engine

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTemporalLedger(t *testing.T) {
	anchor := time.Date(2023, 5, 20, 12, 0, 0, 0, time.UTC)

	inputs := []TimelineInput{
		{
			MemoryID: "m-3",
			Date:     time.Date(2023, 2, 27, 10, 0, 0, 0, time.UTC),
			Summary:  "Washed my car and rotated tires.",
		},
		{
			MemoryID: "m-1",
			Date:     time.Date(2023, 1, 10, 9, 0, 0, 0, time.UTC),
			Summary:  "Attended workshop on Effective Communication.",
		},
		{
			MemoryID: "m-2",
			Date:     time.Date(2023, 2, 14, 15, 0, 0, 0, time.UTC),
			Summary:  "Took bike in for repairs due to gear issues.",
		},
		{
			MemoryID: "m-4",
			Date:     time.Date(2023, 1, 17, 14, 0, 0, 0, time.UTC),
			Summary:  "Team planning meeting.",
		},
	}

	ledger := BuildTemporalLedger(anchor, inputs)

	if len(ledger.Events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(ledger.Events))
	}

	// Verify chronological order: Jan 10 -> Jan 17 -> Feb 14 -> Feb 27
	if ledger.Events[0].MemoryID != "m-1" || ledger.Events[1].MemoryID != "m-4" ||
		ledger.Events[2].MemoryID != "m-2" || ledger.Events[3].MemoryID != "m-3" {
		t.Fatalf("events not sorted chronologically: %+v", ledger.Events)
	}

	// Verify exact interval math: Jan 10 to Jan 17 is exactly 7 days
	jan17 := ledger.Events[1]
	if jan17.DaysAfterPrev != 7 {
		t.Fatalf("expected 7 days between Jan 10 and Jan 17, got %d", jan17.DaysAfterPrev)
	}

	// Verify monthly sequence in February:
	// Event 1 in Feb is bike (Feb 14)
	// Event 2 in Feb is car (Feb 27)
	febEvents := ledger.MonthlyIndex["2023-02"]
	if len(febEvents) != 2 {
		t.Fatalf("expected 2 events in Feb, got %d", len(febEvents))
	}
	if ledger.Events[febEvents[0]-1].MemoryID != "m-2" {
		t.Fatalf("expected bike (m-2) to be first event in Feb")
	}
	if ledger.Events[febEvents[1]-1].MemoryID != "m-3" {
		t.Fatalf("expected car (m-3) to be second event in Feb")
	}

	rendered := ledger.RenderLedger()
	if !strings.Contains(rendered, "Deterministic Temporal Ledger") {
		t.Fatalf("missing ledger title in render: %s", rendered)
	}
	if !strings.Contains(rendered, "+7 days") {
		t.Fatalf("expected '+7 days' interval in rendered output")
	}
	if !strings.Contains(rendered, "1st event on Feb 14") || !strings.Contains(rendered, "2nd event on Feb 27") {
		t.Fatalf("missing February order in rendered output: %s", rendered)
	}
}
