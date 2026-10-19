package extract

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// checkChunks asserts the invariants every chunking must keep.
func checkChunks(t *testing.T, text string, chunks []string, size int) {
	t.Helper()
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > size {
			t.Fatalf("chunk %d has %d runes, limit %d", i, n, size)
		}
		if strings.TrimSpace(c) != c || c == "" {
			t.Fatalf("chunk %d is not trimmed or is empty: %q", i, c)
		}
	}
	// No word is lost: every word of the input appears, in order, across the chunks.
	joined := strings.Join(chunks, " ")
	pos := 0
	for _, w := range strings.Fields(Normalize(text)) {
		if utf8.RuneCountInString(w) > size {
			continue // only words longer than a chunk are cut
		}
		i := strings.Index(joined[pos:], w)
		if i < 0 {
			t.Fatalf("word %q missing or out of order", w)
		}
		pos += i
	}
}

func TestChunkKeepsParagraphsAndSentences(t *testing.T) {
	text := "Billing moved to Go in March.\n\nThe API uses snake_case JSON. Deploys go out on Tuesdays.\n\n" +
		strings.Repeat("Ana reviews every migration before it ships. ", 8)
	chunks := ChunkText(text, 200, 60)
	checkChunks(t, text, chunks, 200)
	if !strings.HasPrefix(chunks[0], "Billing moved to Go in March.\n\nThe API uses snake_case JSON. Deploys go out on Tuesdays.\n\n") {
		t.Fatalf("paragraphs not kept together: %q", chunks[0])
	}
	for _, c := range chunks {
		if !strings.HasSuffix(c, ".") {
			t.Errorf("chunk cut mid-sentence: %q", c)
		}
	}
}

func TestChunkOverlapCarriesWholeSentences(t *testing.T) {
	var sb strings.Builder
	for i := range 40 {
		sb.WriteString("Sentence number ")
		sb.WriteString(strings.Repeat("x", i%7))
		sb.WriteString(" ends here. ")
	}
	chunks := ChunkText(sb.String(), 150, 40)
	checkChunks(t, sb.String(), chunks, 150)
	overlapping := 0
	for i := 1; i < len(chunks); i++ {
		prev := SplitSentences(chunks[i-1])
		if strings.HasPrefix(chunks[i], prev[len(prev)-1]) {
			overlapping++
		}
	}
	if overlapping < len(chunks)/2 {
		t.Fatalf("only %d of %d chunks start with the previous chunk's last sentence", overlapping, len(chunks)-1)
	}
}

func TestChunkSplitsOverlongWordsAndUnicode(t *testing.T) {
	text := strings.Repeat("é", 25) + " " + strings.Repeat("日本語のテキスト。", 30)
	chunks := ChunkText(text, 20, 0)
	checkChunks(t, text, chunks, 20)
	if chunks[0] != strings.Repeat("é", 20) {
		t.Fatalf("chunk 0 = %q", chunks[0])
	}
}

func TestChunkEdgeCases(t *testing.T) {
	if ChunkText("", 100, 10) != nil || ChunkText(" \n\n \t", 100, 10) != nil {
		t.Error("blank text should give no chunks")
	}
	if c := ChunkText("short", 100, 10); !slices.Equal(c, []string{"short"}) {
		t.Errorf("short text = %v", c)
	}
	// An overlap that would stop progress is reduced, not looped on.
	c := ChunkText(strings.Repeat("word ", 500), 50, 49)
	checkChunks(t, strings.Repeat("word ", 500), c, 50)
	if len(c) > 200 {
		t.Errorf("%d chunks: overlap was not bounded", len(c))
	}
}

func TestChunkRandomText(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := []string{"memro", "agent", "remembers", "context.", "Why?", "e.g.", "3.5%", "\n", "\n\n", "Dr.", "Smith", "ok!", strings.Repeat("z", 90)}
	for range 200 {
		var sb strings.Builder
		for range rng.Intn(400) {
			sb.WriteString(words[rng.Intn(len(words))])
			sb.WriteByte(' ')
		}
		size := 20 + rng.Intn(300)
		checkChunks(t, sb.String(), ChunkText(sb.String(), size, rng.Intn(size)), size)
	}
}

func FuzzChunkText(f *testing.F) {
	f.Add("Hello world. This is Memro.\n\nNew paragraph!", 30, 10)
	f.Add(strings.Repeat("a", 100), 7, 3)
	f.Fuzz(func(t *testing.T, text string, size, overlap int) {
		if !utf8.ValidString(text) || size < 1 || size > 5000 {
			return
		}
		checkChunks(t, text, ChunkText(text, size, overlap), size)
	})
}

func TestSplitSentences(t *testing.T) {
	cases := map[string][]string{
		"Ship it. Then rest!":                  {"Ship it.", "Then rest!"},
		"Use memro.co, e.g. for agents. Done.": {"Use memro.co, e.g. for agents.", "Done."},
		"Pi is 3.14 today. Dr. Smith agreed.":  {"Pi is 3.14 today.", "Dr. Smith agreed."},
		"J. R. R. Tolkien wrote it. Really?!":  {"J. R. R. Tolkien wrote it.", "Really?!"},
		"- first item\n- second item":          {"- first item", "- second item"},
		`He said "stop." Then left.`:           {`He said "stop."`, "Then left."},
		"No punctuation at all":                {"No punctuation at all"},
		"Wait... what? It works.":              {"Wait...", "what?", "It works."},
	}
	for in, want := range cases {
		if got := SplitSentences(in); !slices.Equal(got, want) {
			t.Errorf("SplitSentences(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	in := "Line one  has\tspaces\r\nhyphen-\nated word\n\n\n\nNext para  "
	want := "Line one has spaces\nhyphenated word\n\nNext para"
	if got := Normalize(in); got != want {
		t.Errorf("Normalize = %q, want %q", got, want)
	}
}

func TestAtomsKeepAbbreviationsAndNumbers(t *testing.T) {
	a := Atoms("Ana works at Acme Inc. since 2024. She likes v2.5 of memro.co, e.g. the API.")
	if len(a) != 2 {
		t.Fatalf("got %d atoms: %+v", len(a), a)
	}
	if a[0].Fact != "Ana works at Acme Inc. since 2024" || !slices.Contains(a[0].Relations, "employed_at") {
		t.Errorf("atom 0 = %+v", a[0])
	}
	if !strings.Contains(a[1].Fact, "v2.5 of memro.co, e.g. the API") || !slices.Contains(a[1].Relations, "prefers") {
		t.Errorf("atom 1 = %+v", a[1])
	}
}

func TestEventDateRules(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	if got := EventDate("We met on December 20 for the launch", now); got == nil || got.Year() != 2025 {
		t.Errorf("December 20 written in January = %v, want 2025", got)
	}
	if got := EventDate("Review is on February 3", now); got == nil || got.Year() != 2026 {
		t.Errorf("a date a few weeks ahead = %v, want 2026", got)
	}
	if got := EventDate("our todays-list app", now); got != nil {
		t.Errorf("'todays' matched 'today': %v", got)
	}
	if got := EventDate("I may 3D print it", now); got != nil {
		t.Errorf("'may 3D' matched a date: %v", got)
	}
}

func TestEntitiesRankProperNouns(t *testing.T) {
	e := Entities("The billing team met Ana. Ana and Ravi ship Memro. The API is fast. Please email ana@acme")
	if len(e) == 0 || e[0] != "ana" {
		t.Fatalf("most frequent proper noun should come first: %v", e)
	}
	for _, bad := range []string{"the", "please", "the api"} {
		if slices.Contains(e, bad) {
			t.Errorf("stopword %q tagged as entity: %v", bad, e)
		}
	}
	for _, want := range []string{"ravi", "memro", "api", "team", "person:ana"} {
		if !slices.Contains(e, want) {
			t.Errorf("missing %q in %v", want, e)
		}
	}
	if len(Entities(strings.Repeat("Alpha Beta Gamma Delta Epsilon Zeta Eta Theta Iota Kappa Lambda Mu Nu Xi Omicron Pi Rho Sigma Tau Upsilon Phi. ", 2))) > 20 {
		t.Error("more than 20 entities")
	}
}
