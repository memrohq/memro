package api_test

import (
	"encoding/json"
	"testing"
)

// The server infers a memory's type only when the caller gives none; a valid
// type from the caller always wins, in any case or spacing.
func TestMemoryTypes(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)

	save := func(body map[string]any) (int, string) {
		t.Helper()
		code, b := e.do(&a, "v2", "POST", "/memory", j(body), nil)
		var m struct {
			MemoryType string `json:"memory_type"`
		}
		_ = json.Unmarshal(b, &m)
		return code, m.MemoryType
	}

	inferred := map[string]string{
		"To run integration tests, start PostgreSQL before executing the test suite.": "procedural",
		"User prefers TypeScript":                                      "profile",
		"The P0 billing isolation tests passed on the feature branch.": "episodic",
		"Memro uses PostgreSQL for persistent storage.":                "semantic",
	}
	for content, want := range inferred {
		if code, got := save(map[string]any{"content": content}); code != 200 || got != want {
			t.Errorf("no type: %q → %d %q, want %q", content, code, got, want)
		}
	}
	for _, auto := range []string{"", "auto", " AUTO ", "unknown"} {
		if code, got := save(map[string]any{"content": "User prefers TypeScript", "memory_type": auto}); code != 200 || got != "profile" {
			t.Errorf("memory_type %q → %d %q, want inferred profile", auto, code, got)
		}
	}

	// Explicit types are kept, even when the wording suggests another.
	explicit := map[string]string{"episodic": "episodic", "semantic": "semantic", " Profile ": "profile", "PROCEDURAL": "procedural"}
	for given, want := range explicit {
		if code, got := save(map[string]any{"content": "To deploy, run make release.", "memory_type": given}); code != 200 || got != want {
			t.Errorf("memory_type %q → %d %q, want %q", given, code, got, want)
		}
	}

	if code, _ := save(map[string]any{"content": "x", "memory_type": "fact"}); code != 400 {
		t.Errorf("invalid type: %d, want 400", code)
	}
	for _, blank := range []string{"", "   ", "\n\t"} {
		if code, _ := save(map[string]any{"content": blank}); code != 400 {
			t.Errorf("content %q: %d, want 400", blank, code)
		}
	}
}
