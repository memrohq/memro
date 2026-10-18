// Package extract pulls structure out of memory text: event dates, entity
// tags, atomic facts, file types and PDF text.
package extract

import (
	"bytes"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// --- Event dates ----------------------------------------------------------------

var (
	reISODate  = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}):(\d{2})Z?)?`)
	monthNames = []string{"january", "february", "march", "april", "may", "june", "july",
		"august", "september", "october", "november", "december"}
	reMonthDay = map[string]*regexp.Regexp{}
)

func init() {
	for _, m := range monthNames {
		reMonthDay[m] = regexp.MustCompile(m + `\s+(\d{1,2})`)
	}
}

// EventDate finds when the event described in content happened: an ISO
// date, a relative phrase ("yesterday", "last week") or "March 15".
func EventDate(content string, now time.Time) *time.Time {
	if t := isoDate(content); t != nil {
		return t
	}
	lower := strings.ToLower(content)
	rel := []struct {
		phrase string
		delta  time.Duration
	}{
		{"yesterday", -24 * time.Hour}, {"today", 0}, {"tomorrow", 24 * time.Hour},
		{"last week", -7 * 24 * time.Hour}, {"next week", 7 * 24 * time.Hour},
		{"last month", -30 * 24 * time.Hour}, {"next month", 30 * 24 * time.Hour},
	}
	for _, r := range rel {
		if strings.Contains(lower, r.phrase) {
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
			if t.Day() == day { // reject overflow like "February 31"
				return &t
			}
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
	entityWords   = []string{"project", "work", "meeting", "call", "email", "team", "manager", "colleague",
		"travel", "vacation", "trip", "conference", "event", "presentation", "talk", "python", "rust",
		"javascript", "react", "typescript", "database", "api", "server", "client"}
)

// Entities extracts up to 20 lowercase tags used for search pre-filtering.
func Entities(content string) []string {
	set := map[string]struct{}{}
	for _, m := range reProperNoun.FindAllString(content, -1) {
		if l := len(m); l > 2 && l < 50 {
			set[strings.ToLower(m)] = struct{}{}
		}
	}
	lower := strings.ToLower(content)
	for _, w := range entityWords {
		if strings.Contains(lower, w) {
			set[w] = struct{}{}
		}
	}
	for _, m := range reTimeWord.FindAllString(lower, -1) {
		set["time:"+m] = struct{}{}
	}
	for _, m := range reEmailHandle.FindAllString(content, -1) {
		name, _, _ := strings.Cut(strings.ToLower(m), "@")
		set["person:"+name] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > 20 {
		out = out[:20]
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
	for _, s := range strings.FieldsFunc(chunk, func(r rune) bool { return r == '.' || r == '!' || r == '?' }) {
		s = strings.TrimSpace(s)
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

// PDFText extracts the plain text of every page.
func PDFText(data []byte) (string, error) {
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
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteByte(' ')
	}
	return strings.TrimSpace(sb.String()), nil
}

// Chunk splits text into pieces of at most size runes.
func Chunk(text string, size int) []string {
	runes := []rune(text)
	var out []string
	for i := 0; i < len(runes); i += size {
		end := min(i+size, len(runes))
		if c := strings.TrimSpace(string(runes[i:end])); c != "" {
			out = append(out, c)
		}
	}
	return out
}
