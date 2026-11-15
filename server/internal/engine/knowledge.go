package engine

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/domain"
)

// --- Classification -----------------------------------------------------------
//
// Classify is a fallback for memories saved without a type; callers that name
// a type are never overridden. It is a few cheap wording rules, not a model:
// ambiguous text falls through to semantic, which is the safe default.

var (
	// profile: who someone is, or how they like to work.
	reProfileIdentity = regexp.MustCompile(`^(i am|i'm|my name is|call me)\b`)
	reProfileSubject  = regexp.MustCompile(`^(i|i'm|my|the user|user|users|they|he|she)\b`)
	reProfileVerb     = regexp.MustCompile(`\b(prefers?|preference|likes?|loves?|dislikes?|hates?|wants?|would rather|always|never|usually)\b`)

	// procedural: how to do something.
	reProceduralLead = regexp.MustCompile(`^(to [a-z][^,:]*[,:]|in order to\b|how to\b|steps?\b|(run|install|start|stop|restart|deploy|build|execute|set up|configure|create|open|call|use)\s)`)
	reProceduralStep = regexp.MustCompile(`\b(step \d+|then run|first run|before running|make sure to)\b`)

	// episodic: something that happened.
	reFirstPerson  = regexp.MustCompile(`\b(i|we)\b`)
	reEpisodicTime = regexp.MustCompile(`\b(today|yesterday|last (week|month|night)|ago|just now|recently)\b`)
	reEpisodicVerb = regexp.MustCompile(`\b(went|did|saw|said|heard|felt|thought|bought|made|worked|met|decided)\b`)
	reEventVerb    = regexp.MustCompile(`\b(passed|failed|merged|shipped|launched|released|published|deployed|completed|finished|fixed|migrated)\b`)
	reStateVerb    = regexp.MustCompile(`\b(is|are|be|been|being|gets?|stays?)\s+(passed|failed|merged|shipped|launched|released|published|deployed|completed|finished|fixed|migrated)\b`)
)

type Classification struct {
	Type       domain.MemoryType
	Confidence float32
}

// Classify guesses a memory type from its wording, most likely first.
func Classify(content string) []Classification {
	lower := strings.ToLower(strings.TrimSpace(content))
	var out []Classification
	if reProfileIdentity.MatchString(lower) || (reProfileSubject.MatchString(lower) && reProfileVerb.MatchString(lower)) {
		out = append(out, Classification{domain.Profile, 0.9})
	}
	if reProceduralLead.MatchString(lower) || reProceduralStep.MatchString(lower) {
		out = append(out, Classification{domain.Procedural, 0.85})
	}
	happened := reFirstPerson.MatchString(lower) && (reEpisodicTime.MatchString(lower) || reEpisodicVerb.MatchString(lower))
	completed := reEventVerb.MatchString(lower) && !reStateVerb.MatchString(lower)
	if happened || completed {
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
	// Sentiment stems, ignored when deciding whether two statements make the same claim.
	positiveWords = []string{"love", "like", "enjoy", "great", "good", "best", "prefer"}
	negativeWords = []string{"hate", "dislike", "bad", "worst", "terrible", "avoid"}
	// Whole words only: as substrings, "like" matched "likely" and "dislike",
	// "bad" matched "badge" and "best" matched "bestseller".
	rePositive = regexp.MustCompile(`\b(?:lov(?:e|es|ed|ing)|lik(?:e|es|ed|ing)|enjoy(?:s|ed|ing)?|great|good|best|prefer(?:s|red|ring)?)\b`)
	reNegative = regexp.MustCompile(`\b(?:hat(?:e|es|ed|ing)|dislik(?:e|es|ed|ing)|bad|worst|terrible|avoid(?:s|ed|ing)?)\b`)
	reNegation = regexp.MustCompile(`\b(?:not|never|don't|do not|doesn't|cannot|can't|avoid|stop)\b`)

	reSlotPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(.+?)\s+(?:is|was|are|were|be|has been)\s+(?:a|an|the\s+)?(.+)$`),
		regexp.MustCompile(`(?i)^(.+?)\s+(?:prefers?|likes?|wants?|uses?|chose|switched to)\s+(.+)$`),
		regexp.MustCompile(`(?i)^(?:(.+?)\s+)?(?:lives in|moved to|based in|located in)\s+(.+)$`),
		regexp.MustCompile(`(?i)^(.+?)\s+(?:lived in|stayed in|rented|occupied|worked at)\s+(.+)$`),
		regexp.MustCompile(`(?i)^(.+?)\s+(?:has|had|owns|collected|bought|possesses)\s+(\d+\s+.+)$`),
		regexp.MustCompile(`(?i)^(.+?)\s+(?:plays with|uses|selected|drives|switched to|chose)\s+(.+)$`),
	}

	// reKeyValue matches one-line "key: value" facts ("favorite color: blue").
	// The key is 1-4 words and can't end in a digit, so clock times ("at 10:30")
	// don't match; speaker labels are excluded in keyValueSlot.
	reKeyValue  = regexp.MustCompile(`(?i)^([a-z][a-z' -]{0,40}?[a-z])\s*:\s*([^\n]+)$`)
	speakerKeys = []string{"user", "assistant", "system", "human", "ai", "me", "q", "a"}
)

// keyValueSlot returns the key and value of a one-line "key: value" fact.
func keyValueSlot(s string) (key, val string, ok bool) {
	m := reKeyValue.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	key, val = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	if len(strings.Fields(key)) > 4 || slices.Contains(speakerKeys, key) || val == "" {
		return "", "", false
	}
	return key, val, true
}

// normSubject drops a leading "the" so "the billing contact" and "billing
// contact" are one subject.
func normSubject(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "the ")
}

type factSlot struct {
	kind      int // index into reSlotPatterns; "key: value" counts as 0 ("X is Y")
	subj, val string
}

// factSlots returns every (subject, value) pair the slot patterns and the
// "key: value" form find in a cleaned sentence.
func factSlots(s string) []factSlot {
	var out []factSlot
	for i, re := range reSlotPatterns {
		if m := re.FindStringSubmatch(s); len(m) >= 3 && strings.TrimSpace(m[1]) != "" {
			out = append(out, factSlot{i, normSubject(m[1]), strings.TrimSpace(m[2])})
		}
	}
	if k, v, ok := keyValueSlot(s); ok {
		out = append(out, factSlot{0, normSubject(k), v})
	}
	return out
}

var (
	// "deploys moved to tuesdays", "memro moved its data from mongodb to postgres"
	reChange = regexp.MustCompile(`^(.+?)\s+(?:has\s+|have\s+|had\s+|was\s+|were\s+)?(?:moved|changed|switched|shifted|migrated)\b(.*?)\s+to\s+(.+)$`)
	// "the team offsite is in lisbon", "deploys happen on fridays"
	reWhereWhen  = regexp.MustCompile(`^(.+?)\s+(?:is|are|was|were|happens?|takes?\s+place|(?:is|are)\s+held)\s+(?:in|on|at|every)\b`)
	reTimeWord   = regexp.MustCompile(`\b(mondays?|tuesdays?|wednesdays?|thursdays?|fridays?|saturdays?|sundays?|weekends?|mornings?|afternoons?|evenings?|nights?|daily|weekly|monthly|january|february|march|april|may|june|july|august|september|october|november|december|\d{1,2}(:\d\d)?\s*(am|pm))\b`)
	reProperName = regexp.MustCompile(`\b[A-Z][a-z]`)
	// "invoice 1042", "order #77", "PR 57", "#12"
	reIdentifier = regexp.MustCompile(`(?:\b(?:invoice|order|ticket|issue|pr|case|ref|id|no)\s*#?|#)\s*\d+`)
	// "the cluster endpoint is db-old.internal:5432"
	reIsValue = regexp.MustCompile(`^(.+?)\s+(?:is|was|are|were)\s+(?:at\s+)?(\S+)$`)
	// a host or URL: "db.internal:5432", "https://api.memro.co/v1"
	reEndpoint = regexp.MustCompile(`^(?:[a-z][a-z0-9+.-]*://)?[a-z0-9-]+(?:\.[a-z0-9-]+)+(?::\d+)?(?:/\S*)?$`)
)

// changeReplaces reports whether change ("X moved/changed/switched [from A]
// to B", raw text) replaces other: other mentions the A it moved from, or says
// where or when X is with a value of the same kind as B (both times, or both
// named places), and doesn't already say B. "Ana moved to the Berlin office"
// leaves "Ana is in the design team" alone.
func changeReplaces(changeRaw, otherRaw string) bool {
	change, other := cleanText(changeRaw), cleanText(otherRaw)
	m := reChange.FindStringSubmatch(change)
	if m == nil {
		return false
	}
	subj, mid, to := normSubject(m[1]), m[2], strings.TrimSpace(m[3])
	if subj == "" || to == "" || strings.Contains(other, to) {
		return false
	}
	if i := strings.Index(mid, "from "); i >= 0 {
		from := strings.TrimSpace(mid[i+len("from "):])
		if from != "" && strings.HasPrefix(normSubject(other), subj) && strings.Contains(other, from) {
			return true
		}
	}
	// "the cluster endpoint is db-old.internal:5432" vs "the cluster migrated to db-new.internal:5432"
	if m := reIsValue.FindStringSubmatch(strings.TrimRight(other, ".")); m != nil && strings.HasPrefix(normSubject(m[1]), subj) &&
		reEndpoint.MatchString(m[2]) && reEndpoint.MatchString(strings.TrimRight(to, ".")) {
		return true
	}
	loc := reWhereWhen.FindStringSubmatchIndex(other)
	if loc == nil || normSubject(other[loc[2]:loc[3]]) != subj {
		return false
	}
	// cleanText only lowercases and trims, so offsets map back to the raw text.
	otherVal := strings.TrimSpace(strings.TrimSpace(otherRaw)[loc[1]:])
	toRaw := strings.TrimSpace(strings.TrimSpace(changeRaw)[strings.LastIndex(change, to):])
	if tA, tB := reTimeWord.MatchString(to), reTimeWord.MatchString(strings.ToLower(otherVal)); tA || tB {
		return tA && tB
	}
	return reProperName.MatchString(toRaw) && reProperName.MatchString(otherVal)
}

// NearDuplicate reports whether two memories say the same thing in nearly the
// same words: at least three content words each, and 80% of them shared.
func NearDuplicate(a, b string) bool {
	wa, wb := contentWords(a), contentWords(b)
	if len(wa) < 3 || len(wb) < 3 {
		return false
	}
	shared := 0
	for w := range wa {
		if wb[w] {
			shared++
		}
	}
	return float64(shared)/float64(len(wa)+len(wb)-shared) >= 0.8
}

var (
	reNegatedUse = regexp.MustCompile(`\b(?:never|don't|do not|doesn't|does not|stop|stopped|no longer)\s+(?:use|using|uses|run|running|install|installing)\s+([a-z0-9][a-z0-9.+-]*)`)
	reUse        = regexp.MustCompile(`\b(?:use|uses|using|runs?|install|installs)\s+([a-z0-9][a-z0-9.+-]*)`)
)

// negatesUse reports whether neg says to stop using a tool that other says is
// used ("never use npm" vs "we use npm to install packages").
func negatesUse(neg, other string) bool {
	for _, m := range reNegatedUse.FindAllStringSubmatch(neg, -1) {
		tool := m[1]
		if reNegatedUse.MatchString(other) && strings.Contains(other, " "+tool) {
			continue // other also negates it
		}
		for _, u := range reUse.FindAllStringSubmatch(other, -1) {
			if u[1] == tool {
				return true
			}
		}
	}
	return false
}

// sameClaim reports whether two statements share most of their content words
// (Jaccard >= 0.5), ignoring words that start with any of the ignore prefixes.
func sameClaim(a, b string, ignore []string) bool {
	keep := func(ws map[string]bool) map[string]bool {
		out := map[string]bool{}
		for w := range ws {
			if !slices.ContainsFunc(ignore, func(p string) bool { return strings.HasPrefix(w, p) }) {
				out[w] = true
			}
		}
		return out
	}
	wa, wb := keep(contentWords(a)), keep(contentWords(b))
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	shared := 0
	for w := range wa {
		if wb[w] {
			shared++
		}
	}
	return float64(shared)/float64(len(wa)+len(wb)-shared) >= 0.5
}

var stopWords = map[string]bool{"the": true, "and": true, "for": true, "are": true, "was": true, "with": true,
	"this": true, "that": true, "its": true, "our": true, "your": true, "has": true, "have": true}

func contentWords(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-')
	}) {
		w = strings.Trim(w, ".-")
		if len(w) >= 3 && !stopWords[w] {
			out[w] = true
		}
	}
	return out
}

func cleanText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimRight(s, ".!?,;")
	return s
}

var reDurationOrNumber = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\s*(years?|months?|weeks?|days?|hours?|coins?|dollars?|items?|percent|%)?\b`)

// DetectContradiction returns a 0..1 likelihood that a and b contradict.
func DetectContradiction(a, b string) float32 {
	return DetectContradictionWithIntent(a, b, false)
}

// DetectContradictionWithIntent returns contradiction likelihood, skipping number-based
// contradiction if isAggregation is true (since differing numbers across dates in aggregation
// queries are distinct additive items rather than contradictions).
func DetectContradictionWithIntent(a, b string, isAggregation bool) float32 {
	al, bl := cleanText(a), cleanText(b)
	if al == bl || al == "" || bl == "" {
		return 0
	}

	// 1. Slot-based conflict: same subject & relation, conflicting values
	// e.g. "billing contact is Marco" vs "billing contact is Ana"
	// e.g. "user prefers TypeScript" vs "user prefers Python"
	// Slots of the same kind are compared; "X is Y" and "X: Y" are one kind, so
	// "the renewal owner is Marco" and "renewal owner: Ana" meet.
	for _, sa := range factSlots(al) {
		for _, sb := range factSlots(bl) {
			if sa.kind == sb.kind && sa.subj == sb.subj && sa.val != sb.val &&
				!strings.Contains(sa.val, sb.val) && !strings.Contains(sb.val, sa.val) {
				return 0.75
			}
		}
	}

	// A change statement ("deploys moved to tuesdays") replaces an earlier
	// statement of where or when the same subject was.
	if changeReplaces(a, b) || changeReplaces(b, a) {
		return 0.75
	}

	// 2. Direct Negation with common root topic
	negA, negB := reNegation.MatchString(al), reNegation.MatchString(bl)
	if negA != negB {
		stripNeg := func(s string) string {
			return strings.Join(strings.Fields(reNegation.ReplaceAllString(s, " ")), " ")
		}
		// "never use npm" contradicts "we use npm …" whatever else either says.
		if negatesUse(al, bl) || negatesUse(bl, al) {
			return 0.8
		}
		// Otherwise the two must make the same claim apart from the negation: one shared
		// word let "I'm not sure how to use it with my products" contradict any
		// chat message about products.
		if sameClaim(stripNeg(al), stripNeg(bl), nil) {
			return 0.8
		}
	}

	// 3. Sentiment / polarity flip
	aPos, aNeg := rePositive.MatchString(al), reNegative.MatchString(al)
	bPos, bNeg := rePositive.MatchString(bl), reNegative.MatchString(bl)
	if (aPos && bNeg) || (aNeg && bPos) {
		if sameClaim(al, bl, append(slices.Clone(positiveWords), negativeWords...)) {
			return 0.7
		}
		return 0.6
	}

	// 4. Topic overlap with conflicting numbers/durations
	// Skip if aggregation query (differing numbers across events are cumulative)
	if isAggregation {
		return 0
	}
	// e.g. "I lived in that apartment for 6 months" vs "I lived in that apartment for 2 years"
	// e.g. "I collected 50 coins" vs "I collected 75 coins"
	// The statements must say nearly the same thing apart from the numbers:
	// sharing a couple of words ("I ran 5 miles at the park" vs "I walked 2
	// miles at the park") is not enough.
	// Identifiers ("invoice 1042", "#57") name different things; they aren't
	// quantities that changed.
	qa, qb := reIdentifier.ReplaceAllString(al, " "), reIdentifier.ReplaceAllString(bl, " ")
	numsA := reDurationOrNumber.FindAllString(qa, -1)
	numsB := reDurationOrNumber.FindAllString(qb, -1)
	if len(numsA) > 0 && len(numsB) > 0 && !slices.ContainsFunc(numsA, func(n string) bool { return slices.Contains(numsB, n) }) {
		if wordJaccard(reDurationOrNumber.ReplaceAllString(qa, " "), reDurationOrNumber.ReplaceAllString(qb, " ")) >= 0.6 {
			return 0.75
		}
	}

	return 0
}

// wordJaccard is the Jaccard similarity of the words of a and b longer than
// two letters, ignoring punctuation.
func wordJaccard(a, b string) float64 {
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
			if len(w) > 2 {
				out[w] = true
			}
		}
		return out
	}
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(wa)+len(wb)-inter)
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
	queue := []item{{start, []uuid.UUID{start}, 1, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		var next []domain.Relation
		if cur.depth < maxDepth {
			for _, e := range edges {
				if e.From == cur.id && !slices.Contains(cur.path, e.To) {
					next = append(next, e)
				}
			}
		}
		if cur.depth >= maxDepth || len(next) == 0 {
			// A valid short path is still useful when it terminates before the
			// requested depth. Previously such paths disappeared entirely.
			w := chainWalk{Path: cur.path, Confidence: cur.conf}
			for _, n := range nodes {
				if slices.Contains(cur.path, n.ID) {
					w.Nodes = append(w.Nodes, n)
				}
			}
			if len(w.Path) > 1 {
				walks = append(walks, w)
			}
			continue
		}
		for _, e := range next {
			p := append(slices.Clone(cur.path), e.To)
			queue = append(queue, item{e.To, p, cur.conf * e.Confidence, cur.depth + 1})
		}
	}
	return walks
}

// RecencyScore decays exponentially with a 30-day time constant.
func RecencyScore(t, now time.Time) float32 {
	age := now.Sub(t).Seconds()
	return float32(min(1, max(0, math.Exp(-age/(30*24*3600)))))
}
