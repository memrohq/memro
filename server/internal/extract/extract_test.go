package extract

import (
	"slices"
	"testing"
	"time"
)

func TestEventDate(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cases := map[string]*time.Time{
		"I went to the beach on 2024-06-15T14:30:00Z": ptr(time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)),
		"Yesterday I had a great day":                 ptr(now.Add(-24 * time.Hour)),
		"I remember March 15 being special":           ptr(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)),
		"February 31 is not a date":                   nil,
		"just some random text":                       nil,
	}
	for in, want := range cases {
		got := EventDate(in, now)
		if (got == nil) != (want == nil) || (got != nil && !got.Equal(*want)) {
			t.Errorf("EventDate(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEntities(t *testing.T) {
	e := Entities("Completed project Alpha using React and TypeScript, ping ana@acme")
	for _, want := range []string{"project", "react", "typescript", "alpha", "person:ana"} {
		if !slices.Contains(e, want) {
			t.Errorf("missing %q in %v", want, e)
		}
	}
}

func TestAtoms(t *testing.T) {
	a := Atoms("John loves hiking. He goes every weekend! Favorite place?")
	if len(a) != 3 {
		t.Fatalf("got %d atoms", len(a))
	}
	if !slices.Contains(a[0].Entities, "John") || !slices.Contains(a[0].Relations, "enjoys") {
		t.Errorf("bad first atom %+v", a[0])
	}
}

func TestFileTypeAndChunk(t *testing.T) {
	if FileType("a.PDF") != "pdf" || FileType("v.mp4") != "audio" || FileType("main.go") != "text" || FileType("x.zzz") != "unknown" {
		t.Error("file type detection")
	}
	if c := Chunk("abcdefg", 3); !slices.Equal(c, []string{"abc", "def", "g"}) {
		t.Errorf("chunk = %v", c)
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestEntitiesIdentifiers(t *testing.T) {
	e := Entities("The memro.co site has SEO and AEO groundwork. Static files live in public: llms.txt and " +
		"robots.txt. The homepage has FAQPage and JSON-LD markup. Ana prefers tea.")
	for _, want := range []string{"memro.co", "seo", "aeo", "llms.txt", "robots.txt", "faqpage", "json-ld", "ana"} {
		if !slices.Contains(e, want) {
			t.Errorf("missing %q in %v", want, e)
		}
	}
	// Identifiers rank before ordinary capitalized words.
	if i, j := slices.Index(e, "static"), slices.Index(e, "seo"); i < j {
		t.Errorf("%q ranked before %q: %v", "static", "seo", e)
	}
}
