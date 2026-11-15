package engine

import "time"

// SetGraphBudget changes the graph expansion budget for a test and returns a restore func.
func SetGraphBudget(d time.Duration) (restore func()) {
	old := graphBudget
	graphBudget = d
	return func() { graphBudget = old }
}
