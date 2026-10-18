package engine

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/backend/internal/domain"
)

// --- Classification -----------------------------------------------------------

var (
	reProfile      = regexp.MustCompile(`(?i)^(i am (my name is|a|the|my|an)|i (prefer|like|wish|want|set my))`)
	reEpisodicTime = regexp.MustCompile(`(?i)(today|yesterday|last week|last month|ago|just now|recently)`)
	reEpisodicVerb = regexp.MustCompile(`(?i)(went|did|saw|said|heard|felt|thought|bought|made|worked)`)
)

type Classification struct {
	Type       domain.MemoryType
	Confidence float32
}

// Classify guesses a memory type from its wording, most likely first.
func Classify(content string) []Classification {
	lower := strings.ToLower(content)
	var out []Classification
	if reProfile.MatchString(lower) {
		out = append(out, Classification{domain.Profile, 0.9})
	}
	firstPerson := strings.Contains(lower, "i ") || strings.Contains(lower, "we ")
	if firstPerson && (reEpisodicTime.MatchString(lower) || reEpisodicVerb.MatchString(lower)) {
		out = append(out, Classification{domain.Episodic, 0.8})
	}
	if len(out) == 0 {
		out = append(out, Classification{domain.Semantic, 0.5})
	} else {
		out = append(out, Classification{domain.Semantic, 0.3})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out
}

// --- Contradictions & conflict resolution ------------------------------------

var (
	positiveWords = []string{"love", "like", "enjoy", "great", "good", "best"}
	negativeWords = []string{"hate", "dislike", "bad", "worst", "terrible"}
)

// DetectContradiction returns a 0..1 likelihood that a and b contradict.
func DetectContradiction(a, b string) float32 {
	al, bl := strings.ToLower(a), strings.ToLower(b)
	if (strings.Contains(al, "not ") && strings.Contains(bl, "is ")) ||
		(strings.Contains(al, "is ") && strings.Contains(bl, "not ")) {
		return 0.7
	}
	has := func(s string, words []string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(s, w) })
	}
	aPos, aNeg := has(al, positiveWords), has(al, negativeWords)
	bPos, bNeg := has(bl, positiveWords), has(bl, negativeWords)
	if (aPos && bNeg) || (aNeg && bPos) {
		return 0.6
	}
	return 0
}

type ResolutionWeights struct {
	Recency, Frequency, Confidence, AmbiguityThreshold float32
}

var DefaultWeights = ResolutionWeights{Recency: 0.5, Frequency: 0.3, Confidence: 0.2, AmbiguityThreshold: 0.1}

type conflictSide struct {
	ID         uuid.UUID
	CreatedAt  time.Time
	Access     int32
	Confidence float32
}

func truthScore(s conflictSide, now time.Time, w ResolutionWeights) float32 {
	age := float32(now.Sub(s.CreatedAt).Seconds())
	recency := max(0, 1-age/(30*24*3600))
	frequency := min(1, float32(math.Log(float64(s.Access)+1)/math.Log(100)))
	return w.Recency*recency + w.Frequency*frequency + w.Confidence*s.Confidence
}

func resolve(a, b conflictSide, now time.Time, w ResolutionWeights) domain.ResolutionResult {
	sa, sb := truthScore(a, now, w), truthScore(b, now, w)
	diff := sa - sb
	if diff < 0 {
		diff = -diff
	}
	if diff < w.AmbiguityThreshold {
		exp := now.Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
		return domain.ResolutionResult{A: a.ID, B: b.ID, ExpiresAt: &exp}
	}
	if sa > sb {
		return domain.ResolutionResult{Resolved: &a.ID}
	}
	return domain.ResolutionResult{Resolved: &b.ID}
}

// --- Graph walk -----------------------------------------------------------------

type chainNode struct {
	ID      uuid.UUID
	Content string
}

type chainWalk struct {
	Nodes      []chainNode
	Path       []uuid.UUID
	Confidence float32
}

// bfsWalks follows outgoing edges from start up to maxDepth hops and returns
// every walk that reached the depth limit.
func bfsWalks(start uuid.UUID, nodes []chainNode, edges []domain.Relation, maxDepth int) []chainWalk {
	type item struct {
		id    uuid.UUID
		path  []uuid.UUID
		conf  float32
		depth int
	}
	var walks []chainWalk
	visited := map[[2]any]bool{}
	queue := []item{{start, []uuid.UUID{start}, 1, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= maxDepth {
			w := chainWalk{Path: cur.path, Confidence: cur.conf}
			for _, n := range nodes {
				if slices.Contains(cur.path, n.ID) {
					w.Nodes = append(w.Nodes, n)
				}
			}
			walks = append(walks, w)
			continue
		}
		key := [2]any{cur.id, cur.depth}
		if visited[key] {
			continue
		}
		visited[key] = true
		for _, e := range edges {
			if e.From == cur.id && !slices.Contains(cur.path, e.To) {
				p := append(slices.Clone(cur.path), e.To)
				queue = append(queue, item{e.To, p, cur.conf * e.Confidence, cur.depth + 1})
			}
		}
	}
	return walks
}

// RecencyScore decays exponentially with a 30-day time constant.
func RecencyScore(t, now time.Time) float32 {
	age := now.Sub(t).Seconds()
	return float32(min(1, max(0, math.Exp(-age/(30*24*3600)))))
}
