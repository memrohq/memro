package extract

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Default sizes for splitting documents into memories.
const (
	DefaultChunkSize    = 1500
	DefaultChunkOverlap = 200
)

var (
	reManyNewlines = regexp.MustCompile(`\n{3,}`)
	reSpaces       = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	reHyphenBreak  = regexp.MustCompile(`(\p{L})-\n(\p{Ll})`)
)

// Normalize tidies extracted text: unified newlines, collapsed runs of
// spaces, words re-joined across hyphenated line breaks, at most one blank
// line between paragraphs.
func Normalize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\x00", "")
	text = reHyphenBreak.ReplaceAllString(text, "$1$2")
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(reSpaces.ReplaceAllString(l, " "))
	}
	text = strings.Join(lines, "\n")
	return strings.TrimSpace(reManyNewlines.ReplaceAllString(text, "\n\n"))
}

// ChunkText splits text into pieces of at most size runes for storage and
// embedding. It keeps paragraphs, then sentences, then words together where
// it can, and starts each chunk with up to overlap runes of whole sentences
// from the end of the previous one so context isn't lost at the boundary.
// A word is only cut when it alone is longer than size.
func ChunkText(text string, size, overlap int) []string {
	if size <= 0 {
		size = DefaultChunkSize
	}
	if overlap < 0 || overlap >= size/2 {
		overlap = size / 10
	}
	text = Normalize(text)
	if text == "" {
		return nil
	}

	// Units are the smallest pieces kept whole: paragraphs that fit, else
	// their sentences, else words. sep is what joins a unit to the previous one.
	type unit struct {
		text string
		sep  string
	}
	var units []unit
	for i, para := range strings.Split(text, "\n\n") {
		sep := "\n\n"
		if i == 0 {
			sep = ""
		}
		if runeLen(para) <= size {
			units = append(units, unit{para, sep})
			continue
		}
		for j, s := range SplitSentences(para) {
			if j > 0 {
				sep = " "
			}
			for k, piece := range splitLong(s, size) {
				if k > 0 {
					sep = " "
				}
				units = append(units, unit{piece, sep})
			}
		}
	}

	var chunks []string
	var cur []unit
	curLen := 0
	length := func(us []unit) int {
		n := 0
		for i, u := range us {
			if i > 0 {
				n += runeLen(u.sep)
			}
			n += runeLen(u.text)
		}
		return n
	}
	flush := func() {
		var sb strings.Builder
		for i, u := range cur {
			if i > 0 {
				sb.WriteString(u.sep)
			}
			sb.WriteString(u.text)
		}
		if s := strings.TrimSpace(sb.String()); s != "" {
			chunks = append(chunks, s)
		}
	}
	for _, u := range units {
		add := runeLen(u.text)
		if len(cur) > 0 {
			add += runeLen(u.sep)
		}
		if len(cur) > 0 && curLen+add > size {
			flush()
			// Carry whole trailing units into the next chunk, within overlap.
			var carry []unit
			for i := len(cur) - 1; i >= 0; i-- {
				next := append([]unit{cur[i]}, carry...)
				if length(next) > overlap {
					break
				}
				carry = next
			}
			cur = carry
			// The carried context plus this unit must still fit.
			for len(cur) > 0 && length(append(append([]unit{}, cur...), u)) > size {
				cur = cur[1:]
			}
			curLen = length(cur)
			add = runeLen(u.text)
			if len(cur) > 0 {
				add += runeLen(u.sep)
			}
		}
		cur = append(cur, u)
		curLen += add
	}
	if len(cur) > 0 {
		flush()
	}
	return chunks
}

// Chunk splits text into pieces of at most size runes with the default
// overlap. See ChunkText.
func Chunk(text string, size int) []string {
	return ChunkText(text, size, min(DefaultChunkOverlap, size/10))
}

// splitLong breaks a sentence longer than size at word boundaries, and a
// single word longer than size into size-rune pieces.
func splitLong(s string, size int) []string {
	if runeLen(s) <= size {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	for _, w := range strings.Fields(s) {
		for runeLen(w) > size {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			r := []rune(w)
			out = append(out, string(r[:size]))
			w = string(r[size:])
		}
		if cur.Len() > 0 && runeLen(cur.String())+1+runeLen(w) > size {
			out = append(out, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// --- Sentences --------------------------------------------------------------------

// Abbreviations that end with a period but don't end a sentence.
var abbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true, "sr": true, "jr": true, "st": true,
	"vs": true, "etc": true, "inc": true, "ltd": true, "co": true, "corp": true, "no": true, "fig": true,
	"e.g": true, "i.e": true, "approx": true, "dept": true, "est": true, "jan": true, "feb": true,
	"mar": true, "apr": true, "jun": true, "jul": true, "aug": true, "sep": true, "sept": true,
	"oct": true, "nov": true, "dec": true, "u.s": true, "u.k": true, "a.m": true, "p.m": true,
}

// SplitSentences splits text into sentences. Line breaks end a sentence (so
// list items stay separate); ., ! and ? end one when followed by space and
// not part of an abbreviation, initial, decimal number or domain name.
func SplitSentences(text string) []string {
	var out []string
	for _, line := range strings.Split(Normalize(text), "\n") {
		out = append(out, splitLine(line)...)
	}
	return out
}

func splitLine(line string) []string {
	var out []string
	runes := []rune(line)
	start := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		// Swallow runs like "?!" or "..." and closing quotes/brackets.
		end := i + 1
		for end < len(runes) && strings.ContainsRune(".!?\"')]”’", runes[end]) {
			end++
		}
		if end < len(runes) && !unicode.IsSpace(runes[end]) {
			i = end - 1 // "memro.co", "3.5", "e.g.x"
			continue
		}
		if r == '.' && end-i == 1 && isAbbreviation(runes[start:i]) {
			continue
		}
		if s := strings.TrimSpace(string(runes[start:end])); s != "" {
			out = append(out, s)
		}
		start = end
		i = end - 1
	}
	if s := strings.TrimSpace(string(runes[start:])); s != "" {
		out = append(out, s)
	}
	return out
}

// isAbbreviation reports whether the word before a period is an abbreviation
// or a single-letter initial ("J. Smith").
func isAbbreviation(before []rune) bool {
	j := len(before)
	for j > 0 && !unicode.IsSpace(before[j-1]) {
		j--
	}
	word := strings.ToLower(strings.TrimLeft(string(before[j:]), "(\"'"))
	if word == "" {
		return false
	}
	if utf8.RuneCountInString(word) == 1 && unicode.IsLetter([]rune(word)[0]) {
		return true
	}
	return abbreviations[word]
}
