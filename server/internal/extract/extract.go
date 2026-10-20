// Package extract pulls structure out of memory text: event dates, entity
// tags, atomic facts, file types and PDF text.
package extract

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// --- Event dates ----------------------------------------------------------------

var (
	reISODate  = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}):(\d{2})Z?)?`)
	monthNames = []string{"january", "february", "march", "april", "may", "june", "july",
		"august", "september", "october", "november", "december"}
	reMonthDay = map[string]*regexp.Regexp{}
)

var reEntityWords []*regexp.Regexp

func init() {
	for _, w := range entityWords {
		reEntityWords = append(reEntityWords, regexp.MustCompile(`\b`+w+`\b`))
	}
	for _, m := range monthNames {
		reMonthDay[m] = regexp.MustCompile(`\b` + m + `\s+(\d{1,2})\b`)
	}
}

var reRelative = []struct {
	re    *regexp.Regexp
	delta time.Duration
}{
	{regexp.MustCompile(`\byesterday\b`), -24 * time.Hour},
	{regexp.MustCompile(`\btomorrow\b`), 24 * time.Hour},
	{regexp.MustCompile(`\btoday\b`), 0},
	{regexp.MustCompile(`\blast week\b`), -7 * 24 * time.Hour},
	{regexp.MustCompile(`\bnext week\b`), 7 * 24 * time.Hour},
	{regexp.MustCompile(`\blast month\b`), -30 * 24 * time.Hour},
	{regexp.MustCompile(`\bnext month\b`), 30 * 24 * time.Hour},
}

// EventDate finds when the event described in content happened: an ISO
// date, a relative phrase ("yesterday", "last week") or "March 15". A month
// and day without a year that would be more than two months in the future
// is taken to be last year ("December 20", written in January).
func EventDate(content string, now time.Time) *time.Time {
	if t := isoDate(content); t != nil {
		return t
	}
	lower := strings.ToLower(content)
	for _, r := range reRelative {
		if r.re.MatchString(lower) {
			t := now.Add(r.delta)
			return &t
		}
	}
	for i, m := range monthNames {
		if !strings.Contains(lower, m) {
			continue
		}
		if c := reMonthDay[m].FindStringSubmatch(lower); c != nil {
			day, _ := strconv.Atoi(c[1])
			t := time.Date(now.Year(), time.Month(i+1), day, 0, 0, 0, 0, time.UTC)
			if t.Day() != day { // reject overflow like "February 31"
				continue
			}
			if t.After(now.AddDate(0, 2, 0)) {
				t = t.AddDate(-1, 0, 0)
			}
			return &t
		}
	}
	return nil
}

func isoDate(content string) *time.Time {
	c := reISODate.FindStringSubmatch(content)
	if c == nil {
		return nil
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	y, mo, d := n(c[1]), n(c[2]), n(c[3])
	h, mi, s := n(c[4]), n(c[5]), n(c[6])
	t := time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != mo || t.Day() != d || h > 23 || mi > 59 || s > 59 {
		return nil
	}
	return &t
}

// --- Entity tags ----------------------------------------------------------------

var (
	reProperNoun  = regexp.MustCompile(`\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\b`)
	reTimeWord    = regexp.MustCompile(`\b(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec|monday|tuesday|wednesday|thursday|friday|saturday|sunday|yesterday|today|tomorrow)\b`)
	reEmailHandle = regexp.MustCompile(`\b[A-Za-z]+@[A-Za-z]+\b`)
	// Identifiers that name things but aren't Title Case: acronyms (SEO,
	// JSON-LD), CamelCase (FAQPage, OpenAI) and dotted names (memro.co, llms.txt).
	reIdentifier = regexp.MustCompile(`\b(?:[A-Z][A-Z0-9]+(?:-[A-Z0-9]+)*|[A-Z]{2,}[a-z0-9]+|[A-Z]?[a-z0-9]+(?:[A-Z]+[a-z0-9]*)+|[A-Za-z][\w-]+(?:\.[A-Za-z]\w+)+)\b`)
	entityWords  = []string{"project", "work", "meeting", "call", "email", "team", "manager", "colleague",
		"travel", "vacation", "trip", "conference", "event", "presentation", "talk", "python", "rust",
		"javascript", "react", "typescript", "database", "api", "server", "client"}
)

// Capitalized words that start sentences rather than name things.
var notEntities = map[string]bool{
	"the": true, "this": true, "that": true, "these": true, "those": true, "a": true, "an": true,
	"we": true, "i": true, "it": true, "he": true, "she": true, "they": true, "you": true, "our": true,
	"my": true, "your": true, "their": true, "its": true, "in": true, "on": true, "at": true, "for": true,
	"to": true, "of": true, "and": true, "but": true, "or": true, "if": true, "when": true, "then": true,
	"there": true, "here": true, "please": true, "also": true, "after": true, "before": true, "with": true,
	"from": true, "by": true, "as": true, "is": true, "was": true, "are": true, "were": true, "be": true,
	"do": true, "does": true, "did": true, "not": true, "no": true, "yes": true, "so": true, "all": true,
	"some": true, "any": true, "each": true, "every": true, "what": true, "why": true, "how": true,
	"who": true, "where": true, "which": true, "use": true, "run": true, "remember": true, "note": true,
}

// Entities extracts up to 20 lowercase tags used for search pre-filtering:
// proper nouns and identifiers (acronyms, CamelCase, dotted names), most
// frequent first with identifiers winning ties, then topic keywords, people
// from email-style handles and time words.
func Entities(content string) []string {
	counts := map[string]int{}
	first := map[string]int{}
	ident := map[string]bool{}
	count := func(key string, pos int) {
		if _, seen := first[key]; !seen {
			first[key] = pos
		}
		counts[key]++
	}
	for _, loc := range reIdentifier.FindAllStringIndex(content, -1) {
		if m := content[loc[0]:loc[1]]; len(m) >= 2 && len(m) < 50 {
			key := strings.ToLower(m)
			ident[key] = true
			count(key, loc[0])
		}
	}
	for i, m := range reProperNoun.FindAllString(content, -1) {
		l := len(m)
		if l <= 2 || l >= 50 {
			continue
		}
		key := strings.ToLower(m)
		if notEntities[key] {
			continue
		}
		// "The Billing Team" -> "billing team"
		if w, rest, ok := strings.Cut(key, " "); ok && notEntities[w] {
			key = rest
		}
		count(key, len(content)+i)
	}
	nouns := make([]string, 0, len(counts))
	for k := range counts {
		nouns = append(nouns, k)
	}
	sort.Slice(nouns, func(i, j int) bool {
		a, b := nouns[i], nouns[j]
		if counts[a] != counts[b] {
			return counts[a] > counts[b]
		}
		if ident[a] != ident[b] {
			return ident[a]
		}
		return first[a] < first[b]
	})

	seen := map[string]bool{}
	var out []string
	add := func(tag string) {
		if !seen[tag] && len(out) < 20 {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	for _, n := range nouns {
		add(n)
	}
	lower := strings.ToLower(content)
	for i, w := range entityWords {
		if reEntityWords[i].MatchString(lower) {
			add(w)
		}
	}
	for _, m := range reEmailHandle.FindAllString(content, -1) {
		name, _, _ := strings.Cut(strings.ToLower(m), "@")
		add("person:" + name)
	}
	for _, m := range reTimeWord.FindAllString(lower, -1) {
		add("time:" + m)
	}
	return out
}

// --- Atomic facts ---------------------------------------------------------------

type Atom struct {
	Fact       string   `json:"fact"`
	Confidence float32  `json:"confidence"`
	Entities   []string `json:"entities"`
	Relations  []string `json:"relations"`
}

var relationAliases = []struct{ phrase, alias string }{
	{"likes", "prefers"}, {"loves", "enjoys"}, {"hates", "dislikes"},
	{"works at", "employed_at"}, {"lives in", "residence"}, {"studies", "learning"},
}

// Atoms splits a chunk into sentence-level facts.
func Atoms(chunk string) []Atom {
	atoms := []Atom{}
	for _, s := range SplitSentences(chunk) {
		s = strings.TrimRight(strings.TrimSpace(s), ".!?")
		if s == "" {
			continue
		}
		a := Atom{Fact: s, Confidence: 0.8, Entities: []string{}, Relations: []string{}}
		for _, w := range strings.Fields(s) {
			if r := []rune(w); len(w) > 3 && unicode.IsUpper(r[0]) {
				a.Entities = append(a.Entities, w)
			}
		}
		lower := strings.ToLower(s)
		for _, r := range relationAliases {
			if strings.Contains(lower, r.phrase) {
				a.Relations = append(a.Relations, r.alias)
			}
		}
		atoms = append(atoms, a)
	}
	return atoms
}

// --- Files ----------------------------------------------------------------------

func FileType(name string) string {
	switch strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".") {
	case "pdf":
		return "pdf"
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "heic", "tiff":
		return "image"
	case "mp3", "wav", "m4a", "ogg", "flac", "mp4", "mov", "avi", "mkv", "webm":
		return "audio" // video is processed as audio
	case "txt", "md", "json", "csv", "yaml", "yml", "xml", "html", "css", "rs", "py", "js", "ts",
		"tsx", "jsx", "go", "c", "cpp", "h", "sh", "sql":
		return "text"
	case "docx", "doc":
		return "docx"
	case "xlsx", "xls":
		return "xlsx"
	case "pptx", "ppt":
		return "pptx"
	}
	return "unknown"
}

// PDFText extracts the plain text of every page, pages separated by a
// blank line. Malformed PDFs return an error instead of crashing (the
// parser panics on some inputs).
func PDFText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("could not read PDF: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		pageText, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(pageText)
		sb.WriteString("\n\n")
		if sb.Len() > maxExtractedText*4 {
			break
		}
	}
	return Normalize(sb.String()), nil
}

func isUTF8(b []byte) bool { return utf8.Valid(b) }
