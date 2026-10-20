package engine

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/domain"
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
	positiveWords = []string{"love", "like", "enjoy", "great", "good", "best", "prefer"}
	negativeWords = []string{"hate", "dislike", "bad", "worst", "terrible", "avoid"}
	negationWords = []string{"not ", "never ", "don't ", "do not ", "doesn't ", "cannot ", "can't ", "avoid ", "stop "}

	reSlotPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(.+?)\s+(?:is|was|are|were|be|has been)\s+(?:a|an|the\s+)?(.+)$`),
		regexp.MustCompile(`(?i)^(.+?)\s+(?:prefers?|likes?|wants?|uses?|chose|switched to)\s+(.+)$`),
		regexp.MustCompile(`(?i)^(?:(.+?)\s+)?(?:lives in|moved to|based in|located in)\s+(.+)$`),
		regexp.MustCompile(`(?i)^(.+?)\s*:\s*(.+)$`),
	}
)

func cleanText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimRight(s, ".!?,;")
	return s
}

// DetectContradiction returns a 0..1 likelihood that a and b contradict.
func DetectContradiction(a, b string) float32 {
	al, bl := cleanText(a), cleanText(b)
	if al == bl || al == "" || bl == "" {
		return 0
	}

	// 1. Slot-based conflict: same subject & relation, conflicting values
	// e.g. "billing contact is Marco" vs "billing contact is Ana"
	// e.g. "user prefers TypeScript" vs "user prefers Python"
	for _, re := range reSlotPatterns {
		mA := re.FindStringSubmatch(al)
		mB := re.FindStringSubmatch(bl)
		if len(mA) >= 3 && len(mB) >= 3 {
			subjA, valA := strings.TrimSpace(mA[1]), strings.TrimSpace(mA[2])
			subjB, valB := strings.TrimSpace(mB[1]), strings.TrimSpace(mB[2])
			if subjA == subjB && valA != valB {
				if !strings.Contains(valA, valB) && !strings.Contains(valB, valA) {
					return 0.75
				}
			}
		}
	}

	// 2. Direct Negation with common root topic
	hasNeg := func(s string) bool {
		for _, w := range negationWords {
			if strings.Contains(s, w) {
				return true
			}
		}
		return false
	}
	negA, negB := hasNeg(al), hasNeg(bl)
	if negA != negB {
		stripNeg := func(s string) string {
			for _, w := range negationWords {
				s = strings.ReplaceAll(s, w, " ")
			}
			return strings.Join(strings.Fields(s), " ")
		}
		cleanA, cleanB := stripNeg(al), stripNeg(bl)
		wordsA := strings.Fields(cleanA)
		wordsB := strings.Fields(cleanB)
		overlap := 0
		for _, wa := range wordsA {
			if len(wa) > 3 && slices.Contains(wordsB, wa) {
				overlap++
			}
		}
		if overlap >= 1 {
			return 0.8
		}
	}

	// 3. Sentiment / polarity flip
	has := func(s string, words []string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(s, w) })
	}
	aPos, aNeg := has(al, positiveWords), has(al, negativeWords)
	bPos, bNeg := has(bl, positiveWords), has(bl, negativeWords)
	if (aPos && bNeg) || (aNeg && bPos) {
		wordsA := strings.Fields(al)
		wordsB := strings.Fields(bl)
		for _, wa := range wordsA {
			if len(wa) > 3 && slices.Contains(wordsB, wa) {
				return 0.7
			}
		}
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
