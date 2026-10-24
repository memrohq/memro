package engine

import (
	"context"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/metrics"
)

const (
	GraphMaxDepth = 2
	GraphMaxSeeds = 10
	GraphMaxEdges = 2000
	GraphMaxPaths = 200
)

type graphEdge struct {
	From       uuid.UUID
	To         uuid.UUID
	Type       domain.RelationType
	Confidence float32
}

type graphWalk struct {
	SeedID     uuid.UUID
	TargetID   uuid.UUID
	Path       []uuid.UUID
	Types      []domain.RelationType
	Confidence float32
	Hops       int
}

// ExpandGraph executes bounded graph expansion from hybrid search seeds.
// It is strictly budgeted, tenant- and agent-scoped, and protects Recall@5 by never
// displacing the top 5 hybrid results.
func (e *Engine) ExpandGraph(ctx context.Context, q domain.SearchQuery, tenantID uuid.UUID, hybridResults []domain.SearchResult) ([]domain.SearchResult, *domain.GraphDiagnostics) {
	if !q.GraphRetrievalEnabled {
		return hybridResults, &domain.GraphDiagnostics{Applied: false, Reason: "disabled"}
	}
	if q.GraphMode == "" {
		return hybridResults, &domain.GraphDiagnostics{Applied: false, Reason: "not_requested"}
	}
	if len(hybridResults) == 0 {
		return hybridResults, &domain.GraphDiagnostics{Applied: false, Reason: "no_seeds"}
	}

	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()

	numSeeds := min(len(hybridResults), min(q.Limit, GraphMaxSeeds))
	seeds := hybridResults[:numSeeds]

	seedScores := map[uuid.UUID]float32{}
	seedIDs := make([]uuid.UUID, 0, numSeeds)
	existingIDs := map[uuid.UUID]bool{}
	for _, r := range hybridResults {
		if id, err := uuid.Parse(r.MemoryID); err == nil {
			existingIDs[id] = true
		}
	}
	for _, s := range seeds {
		if id, err := uuid.Parse(s.MemoryID); err == nil {
			seedIDs = append(seedIDs, id)
			seedScores[id] = s.Score
		}
	}

	edges, truncatedEdges, err := e.fetchScopedEdges(timeoutCtx, seedIDs, tenantID, q.AgentID)
	if err != nil {
		metrics.GraphExpansionErrors.Inc()
		return hybridResults, &domain.GraphDiagnostics{
			Applied:   false,
			Mode:      q.GraphMode,
			Seeds:     numSeeds,
			ElapsedMS: time.Since(start).Milliseconds(),
			Reason:    "edge_fetch_error",
		}
	}

	walks, truncatedPaths := e.traverseGraph(seedIDs, edges, q.GraphMode)

	// Group walks by target node and pick the best scoring path
	type candidateScore struct {
		targetID   uuid.UUID
		seedID     uuid.UUID
		path       []uuid.UUID
		types      []string
		hops       int
		finalScore float32
	}

	bestCandidates := map[uuid.UUID]candidateScore{}
	for _, w := range walks {
		if existingIDs[w.TargetID] {
			continue // Already in hybrid results
		}
		seedScore := seedScores[w.SeedID]
		typeWeight := relationWeight(w.Types)
		// Score = seed_score * path_confidence * 0.7^(hops-1) * type_weight
		decay := float32(math.Pow(0.7, float64(max(0, w.Hops-1))))
		score := seedScore * w.Confidence * decay * typeWeight

		prev, exists := bestCandidates[w.TargetID]
		if !exists || score > prev.finalScore {
			typeStrs := make([]string, len(w.Types))
			for i, t := range w.Types {
				typeStrs[i] = string(t)
			}
			bestCandidates[w.TargetID] = candidateScore{
				targetID:   w.TargetID,
				seedID:     w.SeedID,
				path:       w.Path,
				types:      typeStrs,
				hops:       w.Hops,
				finalScore: score,
			}
		}
	}

	if len(bestCandidates) == 0 {
		return hybridResults, &domain.GraphDiagnostics{
			Applied:       true,
			Mode:          q.GraphMode,
			Seeds:         numSeeds,
			EdgesExamined: len(edges),
			Paths:         len(walks),
			NodesAdded:    0,
			Truncated:     truncatedEdges || truncatedPaths,
			ElapsedMS:     time.Since(start).Milliseconds(),
			Reason:        "no_new_nodes",
		}
	}

	// Rank candidate nodes by score
	candList := make([]candidateScore, 0, len(bestCandidates))
	for _, c := range bestCandidates {
		candList = append(candList, c)
	}
	sort.Slice(candList, func(i, j int) bool {
		return candList[i].finalScore > candList[j].finalScore
	})

	// Max graph additions: ceil(limit / 3)
	maxGraphSlots := int(math.Ceil(float64(q.Limit) / 3.0))
	if len(candList) > maxGraphSlots {
		candList = candList[:maxGraphSlots]
	}

	candIDs := make([]uuid.UUID, len(candList))
	candScoreMap := make(map[uuid.UUID]float32, len(candList))
	for i, c := range candList {
		candIDs[i] = c.targetID
		candScoreMap[c.targetID] = c.finalScore
	}

	loadedGraphResults, err := e.loadResults(timeoutCtx, candIDs, candScoreMap)
	if err != nil {
		metrics.GraphExpansionErrors.Inc()
		return hybridResults, &domain.GraphDiagnostics{
			Applied:   false,
			Mode:      q.GraphMode,
			Seeds:     numSeeds,
			ElapsedMS: time.Since(start).Milliseconds(),
			Reason:    "load_results_error",
		}
	}

	// Attach Graph provenance metadata
	loadedMap := map[uuid.UUID]domain.SearchResult{}
	for _, res := range loadedGraphResults {
		if id, err := uuid.Parse(res.MemoryID); err == nil {
			c := bestCandidates[id]
			pathStrs := make([]string, len(c.path))
			for pIdx, pNode := range c.path {
				pathStrs[pIdx] = pNode.String()
			}
			res.Graph = &domain.ResultGraphInfo{
				SeedID:        c.seedID.String(),
				Path:          pathStrs,
				RelationTypes: c.types,
				Hops:          c.hops,
			}
			loadedMap[id] = res
		}
	}

	// Slot replacement preserving top 5 hybrid results
	keepHybridCount := max(5, q.Limit-len(candList))
	if keepHybridCount > len(hybridResults) {
		keepHybridCount = len(hybridResults)
	}

	finalResults := make([]domain.SearchResult, 0, q.Limit)
	finalResults = append(finalResults, hybridResults[:keepHybridCount]...)

	// Append graph results that were loaded
	for _, c := range candList {
		if res, ok := loadedMap[c.targetID]; ok {
			finalResults = append(finalResults, res)
		}
	}

	// If there is still capacity, fill with remaining hybrid results
	if len(finalResults) < q.Limit && len(hybridResults) > keepHybridCount {
		for _, h := range hybridResults[keepHybridCount:] {
			if len(finalResults) >= q.Limit {
				break
			}
			finalResults = append(finalResults, h)
		}
	}

	metrics.GraphExpansionApplied.WithLabelValues(q.GraphMode).Inc()

	diag := &domain.GraphDiagnostics{
		Applied:       true,
		Mode:          q.GraphMode,
		Seeds:         numSeeds,
		EdgesExamined: len(edges),
		Paths:         len(walks),
		NodesAdded:    len(candList),
		Truncated:     truncatedEdges || truncatedPaths,
		ElapsedMS:     time.Since(start).Milliseconds(),
	}

	return finalResults, diag
}

func (e *Engine) fetchScopedEdges(ctx context.Context, seeds []uuid.UUID, tenantID uuid.UUID, agentID string) ([]graphEdge, bool, error) {
	if len(seeds) == 0 {
		return nil, false, nil
	}
	frontier := make([]uuid.UUID, len(seeds))
	copy(frontier, seeds)

	var allEdges []graphEdge
	seenEdge := map[[3]any]bool{}
	truncated := false

	// Fetch up to 2 hops
	for hop := 0; hop < GraphMaxDepth; hop++ {
		if len(frontier) == 0 || len(allEdges) >= GraphMaxEdges {
			break
		}
		rows, err := e.pool.Query(ctx, `
			SELECT source_memory_id, target_memory_id, relation_type, COALESCE(confidence, 1.0)::REAL
			FROM memory_relations
			WHERE (source_memory_id = ANY($1) OR target_memory_id = ANY($1))
			  AND tenant_id = $2 AND created_by = $3
			LIMIT $4`, frontier, tenantID, agentID, GraphMaxEdges-len(allEdges))
		if err != nil {
			return nil, false, err
		}

		var nextFrontier []uuid.UUID
		for rows.Next() {
			var r graphEdge
			var t string
			if err := rows.Scan(&r.From, &r.To, &t, &r.Confidence); err != nil {
				rows.Close()
				return nil, false, err
			}
			r.Type = domain.RelationType(t)

			key := [3]any{r.From, r.To, r.Type}
			if !seenEdge[key] {
				seenEdge[key] = true
				allEdges = append(allEdges, r)
				if !slices.Contains(nextFrontier, r.From) {
					nextFrontier = append(nextFrontier, r.From)
				}
				if !slices.Contains(nextFrontier, r.To) {
					nextFrontier = append(nextFrontier, r.To)
				}
			}
			if len(allEdges) >= GraphMaxEdges {
				truncated = true
				break
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, false, err
		}
		frontier = nextFrontier
	}

	return allEdges, truncated, nil
}

func (e *Engine) traverseGraph(seedIDs []uuid.UUID, edges []graphEdge, mode string) ([]graphWalk, bool) {
	type queueItem struct {
		seedID uuid.UUID
		curID  uuid.UUID
		path   []uuid.UUID
		types  []domain.RelationType
		conf   float32
		depth  int
	}

	var walks []graphWalk
	queue := make([]queueItem, 0, len(seedIDs))
	for _, s := range seedIDs {
		queue = append(queue, queueItem{
			seedID: s,
			curID:  s,
			path:   []uuid.UUID{s},
			conf:   1.0,
			depth:  0,
		})
	}

	truncated := false
	visitedLegacy := map[[2]any]bool{}

	for len(queue) > 0 {
		if len(walks) >= GraphMaxPaths {
			truncated = true
			break
		}
		cur := queue[0]
		queue = queue[1:]

		if mode == "legacy" {
			// Legacy: drops short dead ends; visited set per node and depth
			if cur.depth >= GraphMaxDepth {
				if len(cur.path) > 1 {
					walks = append(walks, graphWalk{
						SeedID:     cur.seedID,
						TargetID:   cur.curID,
						Path:       cur.path,
						Types:      cur.types,
						Confidence: cur.conf,
						Hops:       cur.depth,
					})
				}
				continue
			}
			vKey := [2]any{cur.curID, cur.depth}
			if visitedLegacy[vKey] {
				continue
			}
			visitedLegacy[vKey] = true

			for _, edge := range edges {
				nextID, valid := stepEdge(cur.curID, edge)
				if valid && !slices.Contains(cur.path, nextID) {
					p := append(slices.Clone(cur.path), nextID)
					t := append(slices.Clone(cur.types), edge.Type)
					queue = append(queue, queueItem{
						seedID: cur.seedID,
						curID:  nextID,
						path:   p,
						types:  t,
						conf:   cur.conf * edge.Confidence,
						depth:  cur.depth + 1,
					})
				}
			}
		} else {
			// Bounded (PR #43): preserves short terminal paths, converging branches, path-local cycle check
			var nextEdges []struct {
				to   uuid.UUID
				edge graphEdge
			}
			if cur.depth < GraphMaxDepth {
				for _, edge := range edges {
					nextID, valid := stepEdge(cur.curID, edge)
					if valid && !slices.Contains(cur.path, nextID) {
						nextEdges = append(nextEdges, struct {
							to   uuid.UUID
							edge graphEdge
						}{nextID, edge})
					}
				}
			}

			if cur.depth >= GraphMaxDepth || len(nextEdges) == 0 {
				if len(cur.path) > 1 {
					walks = append(walks, graphWalk{
						SeedID:     cur.seedID,
						TargetID:   cur.curID,
						Path:       cur.path,
						Types:      cur.types,
						Confidence: cur.conf,
						Hops:       cur.depth,
					})
				}
				continue
			}

			for _, n := range nextEdges {
				p := append(slices.Clone(cur.path), n.to)
				t := append(slices.Clone(cur.types), n.edge.Type)
				queue = append(queue, queueItem{
					seedID: cur.seedID,
					curID:  n.to,
					path:   p,
					types:  t,
					conf:   cur.conf * n.edge.Confidence,
					depth:  cur.depth + 1,
				})
			}
		}
	}

	return walks, truncated
}

func stepEdge(cur uuid.UUID, e graphEdge) (uuid.UUID, bool) {
	if e.Type == domain.RelAssociated {
		if e.From == cur {
			return e.To, true
		}
		if e.To == cur {
			return e.From, true
		}
		return uuid.Nil, false
	}
	// Directed edge: follow From -> To
	if e.From == cur {
		return e.To, true
	}
	return uuid.Nil, false
}

func relationWeight(types []domain.RelationType) float32 {
	weight := float32(1.0)
	for _, t := range types {
		switch t {
		case domain.RelUpdates, domain.RelCauses:
			weight *= 1.0
		case domain.RelDerives, domain.RelExtends:
			weight *= 0.8
		case domain.RelAssociated:
			weight *= 0.6
		default:
			weight *= 0.5
		}
	}
	return weight
}
