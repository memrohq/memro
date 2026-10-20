package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TimelineEvent is a single chronologically sorted point in time extracted from memories.
type TimelineEvent struct {
	Index            int
	Date             time.Time
	DateStr          string
	DaysBeforeAnchor int
	DaysAfterPrev    int
	Summary          string
	MemoryID         string
}

// TemporalLedger provides deterministic calendar and sequence calculations for LLM context assembly.
// It eliminates arithmetic and calendar hallucinations by pre-calculating relative day offsets and chronological ordering.
type TemporalLedger struct {
	AnchorDate   time.Time
	Events       []TimelineEvent
	MonthlyIndex map[string][]int // "2023-02" -> [event indices in chronological order]
}

// BuildTemporalLedger sorts memories chronologically and calculates deterministic day differences.
func BuildTemporalLedger(anchor time.Time, memories []TimelineInput) *TemporalLedger {
	ledger := &TemporalLedger{
		AnchorDate:   anchor,
		MonthlyIndex: make(map[string][]int),
	}

	valid := make([]TimelineInput, 0, len(memories))
	for _, m := range memories {
		if !m.Date.IsZero() {
			valid = append(valid, m)
		}
	}

	// Sort chronologically (oldest to newest)
	sort.SliceStable(valid, func(i, j int) bool {
		return valid[i].Date.Before(valid[j].Date)
	})

	var prevDate time.Time
	for i, m := range valid {
		daysBefore := int(anchor.Sub(m.Date).Hours() / 24)
		daysAfterPrev := 0
		if !prevDate.IsZero() {
			daysAfterPrev = int(m.Date.Sub(prevDate).Hours() / 24)
		}

		ev := TimelineEvent{
			Index:            i + 1,
			Date:             m.Date,
			DateStr:          m.Date.Format("2006-01-02"),
			DaysBeforeAnchor: daysBefore,
			DaysAfterPrev:    daysAfterPrev,
			Summary:          m.Summary,
			MemoryID:         m.MemoryID,
		}
		ledger.Events = append(ledger.Events, ev)

		monthKey := m.Date.Format("2006-01")
		ledger.MonthlyIndex[monthKey] = append(ledger.MonthlyIndex[monthKey], ev.Index)

		prevDate = m.Date
	}

	return ledger
}

// TimelineInput represents a dated item to be processed into the ledger.
type TimelineInput struct {
	MemoryID string
	Date     time.Time
	Summary  string
}

// RenderLedger produces an unambiguous temporal ledger block for inclusion in LLM prompts.
func (l *TemporalLedger) RenderLedger() string {
	if len(l.Events) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Deterministic Temporal Ledger (Chronologically Verified)\n")
	if !l.AnchorDate.IsZero() {
		fmt.Fprintf(&b, "Current Reference Date: %s\n\n", l.AnchorDate.Format("2006-01-02"))
	}

	b.WriteString("| # | Date | Relative to Current | Interval Since Prev | Event Summary |\n")
	b.WriteString("|---|---|---|---|---|\n")

	for _, ev := range l.Events {
		rel := ""
		if ev.DaysBeforeAnchor > 0 {
			rel = fmt.Sprintf("%d days ago", ev.DaysBeforeAnchor)
		} else if ev.DaysBeforeAnchor == 0 {
			rel = "same day"
		} else {
			rel = fmt.Sprintf("in %d days", -ev.DaysBeforeAnchor)
		}

		interval := "-"
		if ev.Index > 1 {
			interval = fmt.Sprintf("+%d days", ev.DaysAfterPrev)
		}

		summary := strings.ReplaceAll(ev.Summary, "\n", " ")
		if len(summary) > 120 {
			summary = summary[:117] + "..."
		}

		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n", ev.Index, ev.DateStr, rel, interval, summary)
	}

	// Add monthly sequence breakdown if multiple events occur in same month
	hasMultiMonth := false
	for _, indices := range l.MonthlyIndex {
		if len(indices) > 1 {
			hasMultiMonth = true
			break
		}
	}

	if hasMultiMonth {
		b.WriteString("\n**Monthly Event Sequences (Verified Order):**\n")
		// Sort month keys
		var months []string
		for m := range l.MonthlyIndex {
			months = append(months, m)
		}
		sort.Strings(months)

		for _, m := range months {
			indices := l.MonthlyIndex[m]
			if len(indices) > 1 {
				var items []string
				for seq, idx := range indices {
					ev := l.Events[idx-1]
					ord := "1st"
					if seq == 1 {
						ord = "2nd"
					} else if seq == 2 {
						ord = "3rd"
					} else if seq > 2 {
						ord = fmt.Sprintf("%dth", seq+1)
					}
					items = append(items, fmt.Sprintf("%s event on %s (#%d)", ord, ev.Date.Format("Jan 02"), idx))
				}
				fmt.Fprintf(&b, "- In **%s**: %s\n", m, strings.Join(items, " -> "))
			}
		}
	}

	return b.String()
}
