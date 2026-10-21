package dce

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Templates: requests may use {{config.<field>}}, {{cursor}}, {{checkpoint}},
// {{offset}}, {{page}}, {{source.id}} and {{source.name}}; mappings use item
// paths, e.g. "{{key}}: {{fields.summary}}" or "labels[].name". Unknown or
// empty values render as "", and a query parameter that renders empty is
// left out.
//
// A value may pass through filters: "{{body|text}}" (HTML to plain text),
// "{{updated|date}}" (YYYY-MM-DD), "{{updated|unix}}" (seconds),
// "{{checkpoint|or:0}}" (a default), "{{fields|kv}}" (an object as
// "key: value" lines, keys sorted), "{{labels[].name|list}}" (one "- " line
// per element) and, in JSON bodies only, "{{cursor|json}}" (a whole JSON
// value: null when empty). A mapping may repeat a block per element:
// "{{#each comments}}{{author.name}}: {{body}}\n{{/each}}".

var (
	placeholder = regexp.MustCompile(`\{\{\s*([^{}#/][^{}]*?)\s*\}\}`)
	eachBlock   = regexp.MustCompile(`(?s)\{\{#each\s+([^{}]+?)\s*\}\}(.*?)\{\{/each\}\}`)
)

// Vars are the values a request template can use.
type Vars struct {
	Config     map[string]string
	Cursor     string
	Checkpoint string
	SourceID   string
	SourceName string
	// Offset and Page are set for offset and page pagination, for APIs that
	// take them in a JSON body rather than the query.
	Offset string
	Page   string
}

func (v Vars) lookup(name string) string {
	switch {
	case strings.HasPrefix(name, "config."):
		return v.Config[strings.TrimPrefix(name, "config.")]
	case name == "cursor":
		return v.Cursor
	case name == "checkpoint":
		return v.Checkpoint
	case name == "offset":
		return v.Offset
	case name == "page":
		return v.Page
	case name == "source.id":
		return v.SourceID
	case name == "source.name":
		return v.SourceName
	}
	return ""
}

// expr is a parsed placeholder: a name and its filters.
type expr struct {
	name    string
	filters []string
}

func parseExpr(s string) expr {
	parts := strings.Split(s, "|")
	e := expr{name: strings.TrimSpace(parts[0])}
	for _, f := range parts[1:] {
		e.filters = append(e.filters, strings.TrimSpace(f))
	}
	return e
}

// applyFilters turns a looked-up value into text. Structural filters (kv,
// list) see the raw value; the rest work on text.
func applyFilters(v any, filters []string) string {
	s := ""
	switch {
	case len(filters) > 0 && filters[0] == "kv":
		s, filters = kvText(v), filters[1:]
	case len(filters) > 0 && filters[0] == "list":
		s, filters = listText(v), filters[1:]
	case v == nil:
	default:
		if l, ok := v.([]string); ok {
			s = strings.Join(l, ", ")
		} else {
			s = stringOf(v)
		}
	}
	for _, f := range filters {
		name, arg, _ := strings.Cut(f, ":")
		switch name {
		case "text":
			s = htmlToText(s)
		case "date":
			if t := ParseDate(s); !t.IsZero() {
				s = t.Format("2006-01-02")
			} else {
				s = ""
			}
		case "unix":
			if t := ParseDate(s); !t.IsZero() {
				s = strconv.FormatInt(t.Unix(), 10)
			} else {
				s = ""
			}
		case "or":
			if s == "" {
				s = arg
			}
		}
	}
	return s
}

// render fills a request template.
func render(tpl string, v Vars) string {
	out, _ := renderAll(tpl, v)
	return out
}

// renderAll fills a template and reports whether every placeholder had a
// value. A query parameter with a missing value is left out entirely, so an
// optional filter ("updated_on > {{checkpoint}}") is not sent half-filled.
func renderAll(tpl string, v Vars) (string, bool) {
	if !strings.Contains(tpl, "{{") {
		return tpl, true
	}
	complete := true
	out := placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		e := parseExpr(placeholder.FindStringSubmatch(m)[1])
		val := applyFilters(v.lookup(e.name), e.filters)
		if val == "" {
			complete = false
		}
		return val
	})
	return out, complete
}

// renderJSON fills a JSON body template: values are escaped for use inside a
// JSON string literal (the template supplies the quotes), so a value cannot
// change the body's structure. "{{name|json}}" instead renders a whole JSON
// value: null when empty, otherwise a quoted string.
func renderJSON(tpl string, v Vars) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		e := parseExpr(placeholder.FindStringSubmatch(m)[1])
		asJSON := len(e.filters) > 0 && e.filters[len(e.filters)-1] == "json"
		if asJSON {
			e.filters = e.filters[:len(e.filters)-1]
		}
		val := applyFilters(v.lookup(e.name), e.filters)
		if asJSON && val == "" {
			return "null"
		}
		b, _ := json.Marshal(val)
		if asJSON {
			return string(b)
		}
		return string(b[1 : len(b)-1])
	})
}

// renderItem fills a mapping value: a bare path, or a template of paths.
func renderItem(tpl string, item any) string {
	if tpl == "" {
		return ""
	}
	if !strings.Contains(tpl, "{{") {
		return applyFilters(itemValue(item, tpl), nil)
	}
	out := eachBlock.ReplaceAllStringFunc(tpl, func(m string) string {
		sub := eachBlock.FindStringSubmatch(m)
		list, _ := ExtractSlice(item, strings.TrimSpace(sub[1]))
		var b strings.Builder
		for _, el := range list {
			b.WriteString(renderItem(sub[2], el))
			b.WriteString("\n")
		}
		return b.String()
	})
	out = placeholder.ReplaceAllStringFunc(out, func(m string) string {
		e := parseExpr(placeholder.FindStringSubmatch(m)[1])
		return applyFilters(itemValue(item, e.name), e.filters)
	})
	return tidy(out)
}

// itemValue is the value at path; "a[].b" collects b over the array a as
// []string, and "a[]" the array's own scalar elements.
func itemValue(item any, path string) any {
	before, after, ok := strings.Cut(path, "[]")
	if !ok {
		v, _ := ExtractValue(item, path)
		return v
	}
	list, _ := ExtractSlice(item, before)
	var out []string
	for _, el := range list {
		if s := applyFilters(itemValue(el, strings.TrimPrefix(after, ".")), nil); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// kvText renders an object as "key: value" lines with sorted keys (so the
// text, and the memory built from it, is the same on every sync).
func kvText(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		var s string
		switch val := obj[k].(type) {
		case []any:
			var parts []string
			for _, el := range val {
				if t := scalarText(el); t != "" {
					parts = append(parts, t)
				}
			}
			s = strings.Join(parts, ", ")
		default:
			s = scalarText(val)
		}
		if s != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", k, s))
		}
	}
	return strings.Join(lines, "\n")
}

// scalarText is a scalar's text, or a nested object's "name"/"text"/"email"
// (Airtable collaborators and attachments, for instance).
func scalarText(v any) string {
	if obj, ok := v.(map[string]any); ok {
		for _, k := range []string{"name", "text", "email", "filename", "url"} {
			if s, ok := obj[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	if v == nil {
		return ""
	}
	return stringOf(v)
}

func listText(v any) string {
	var items []string
	switch l := v.(type) {
	case []string:
		items = l
	case []any:
		for _, el := range l {
			if s := scalarText(el); s != "" {
				items = append(items, s)
			}
		}
	}
	var b strings.Builder
	for _, s := range items {
		b.WriteString("- ")
		b.WriteString(strings.TrimSpace(s))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

var (
	htmlBreak  = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/h[1-6]|/tr|/blockquote|/pre)\s*/?>`)
	htmlItem   = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
	htmlDrop   = regexp.MustCompile(`(?is)<\s*(script|style)[^>]*>.*?<\s*/\s*(script|style)\s*>`)
	htmlTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRun   = regexp.MustCompile(`[ \t\x{00a0}]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// htmlToText turns HTML (Confluence storage format, Zendesk and Intercom
// bodies) into readable text: block ends become line breaks, list items
// "- ", tags are dropped and entities decoded.
func htmlToText(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return strings.TrimSpace(s)
	}
	s = htmlDrop.ReplaceAllString(s, "")
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = htmlItem.ReplaceAllString(s, "- ")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRun.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// emptyLabel is a line that is only a label left behind by a missing value,
// e.g. "Assignee:". Manifests keep a label and its value on one line.
var emptyLabel = regexp.MustCompile(`^[\p{L}\p{N} ./#()_-]{1,40}:$`)

// tidy drops empty-label lines and collapses runs of blank lines.
func tidy(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		t := strings.TrimSpace(line)
		if emptyLabel.MatchString(t) {
			continue
		}
		if t == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// configRefs lists the config fields the strings use.
func configRefs(ss ...string) []string {
	var out []string
	for _, s := range ss {
		for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
			if name, ok := strings.CutPrefix(parseExpr(m[1]).name, "config."); ok {
				out = append(out, name)
			}
		}
	}
	return out
}

// stripTemplates replaces placeholders with a neutral token, to validate
// the shape of a templated URL.
func stripTemplates(s string) string {
	return placeholder.ReplaceAllString(s, "x")
}
