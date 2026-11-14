package api

import "testing"

func TestUsageCost(t *testing.T) {
	m := map[string]int64{
		"stored_tokens": 900_000, "query_tokens": 100_000, // 1M embedding tokens
		"vision_input_tokens": 1_000_000, "vision_output_tokens": 100_000,
	}
	// 1M × $0.02 + 1M × $2.50 + 0.1M × $10 = $3.52
	if got := usageCost(m, "text-embedding-3-small"); got != 3.52 {
		t.Fatalf("cost = %v, want 3.52", got)
	}
	// An unknown embedding model costs nothing rather than guessing.
	if got := usageCost(map[string]int64{"stored_tokens": 1_000_000}, "fake-64"); got != 0 {
		t.Fatalf("unknown model cost = %v", got)
	}
	if got := usageCost(map[string]int64{"query_tokens": 7}, "text-embedding-3-small"); got != 0 {
		t.Fatalf("7 tokens = %v, want 0 (rounds to the micro-dollar)", got)
	}
}
