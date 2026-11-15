package api_test

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/memrohq/memro/server/internal/api"
)

// Evolution benchmark: each scenario saves memories in order through the API,
// then searches the way an agent would and checks what comes back first.
//
//   - update scenarios: the stale fact must be marked superseded and the
//     current one left live ("unresolved" otherwise; ranking first by luck
//     doesn't count);
//   - duplicate scenarios: only one copy should be left unsuperseded;
//   - independent scenarios: nothing may be marked superseded (a false update).
//
// Arms: save-time linking off (GRAPH_WRITE_EDGES=false, prod today) and on.
// Runs with the other database tests (CI); locally:
// MEMRO_TEST_DATABASE_URL=... go test ./internal/api -run TestEvolutionBench -v
type evolutionScenario struct {
	Name        string   `json:"name"`
	Memories    []string `json:"memories"`
	Query       string   `json:"query"`
	Current     string   `json:"current"`
	Stale       string   `json:"stale"`
	Duplicate   bool     `json:"duplicate"`
	Independent bool     `json:"independent"`
}

type evolutionScore struct{ stale, updates, dupLeft, dups, falseUpd, indep int }

func TestEvolutionBench(t *testing.T) {
	raw, err := os.ReadFile("testdata/evolution.json")
	if err != nil {
		t.Fatal(err)
	}
	var scenarios []evolutionScenario
	if err := json.Unmarshal(raw, &scenarios); err != nil {
		t.Fatal(err)
	}

	for _, arm := range []struct {
		name  string
		edges bool
	}{{"linking off", false}, {"linking on", true}} {
		t.Run(arm.name, func(t *testing.T) {
			e := setup(t, func(d *api.Deps) { d.Config.GraphWriteEdges = arm.edges })
			var s evolutionScore
			for _, sc := range scenarios {
				a := newAgent(t)
				e.register(a)
				for _, m := range sc.Memories {
					e.remember(a, m, "semantic")
				}
				q := url.Values{"query": {sc.Query}, "mode": {"keyword"}, "limit": {"5"}, "resolve_contradictions": {"true"}}
				code, body := e.do(&a, "v2", "GET", "/search?"+q.Encode(), nil, nil)
				e.must(200, code, body, "search "+sc.Name)
				var res struct {
					Results []struct {
						Content      string `json:"content"`
						IsSuperseded bool   `json:"is_superseded"`
					} `json:"results"`
				}
				_ = json.Unmarshal(body, &res)

				live := 0 // results not marked superseded, in rank order
				firstLive := ""
				anySuperseded, staleMarked, currentLive := false, false, false
				for _, r := range res.Results {
					if r.IsSuperseded {
						anySuperseded = true
						staleMarked = staleMarked || (sc.Stale != "" && strings.Contains(r.Content, sc.Stale))
						continue
					}
					if live == 0 {
						firstLive = r.Content
					}
					currentLive = currentLive || (sc.Current != "" && strings.Contains(r.Content, sc.Current))
					live++
				}
				switch {
				case sc.Duplicate:
					s.dups++
					if live > 1 {
						s.dupLeft++
						t.Logf("%-34s duplicate left: %d live copies", sc.Name, live)
					}
				case sc.Independent:
					s.indep++
					if anySuperseded {
						s.falseUpd++
						t.Logf("%-34s FALSE UPDATE: an independent fact was superseded", sc.Name)
					}
				default:
					s.updates++
					// Resolved only when Memro marked the old fact as replaced; the newer
					// one merely ranking first is luck, not resolution.
					if !staleMarked || !currentLive {
						s.stale++
						t.Logf("%-34s unresolved: stale marked=%v, top live %q", sc.Name, staleMarked, firstLive)
					}
				}
			}
			t.Logf("== %s: unresolved updates %d/%d · duplicates left %d/%d · false updates %d/%d",
				arm.name, s.stale, s.updates, s.dupLeft, s.dups, s.falseUpd, s.indep)
			// A gate, not just a report: a change that leaves an update unresolved, a
			// duplicate live or (worst) hides an independent fact fails CI.
			if s.stale+s.dupLeft+s.falseUpd > 0 {
				t.Errorf("evolution regressed (%s): see the scenarios logged above", arm.name)
			}
		})
	}
}
