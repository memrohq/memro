package connectors

import (
	"context"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Slack's message markup: <@U123>, <#C123|name>, <!here>, <url|label>.
var slackToken = regexp.MustCompile(`<([^<>]+)>`)

// formatText turns Slack markup into plain text, resolving user mentions.
func (p *slackProvider) formatText(ctx context.Context, c *conn, text string) string {
	return formatSlackText(text, func(id string) string { return p.userName(ctx, c, id) })
}

func formatSlackText(text string, name func(string) string) string {
	out := slackToken.ReplaceAllStringFunc(text, func(tok string) string {
		inner := tok[1 : len(tok)-1]
		target, label, _ := strings.Cut(inner, "|")
		switch {
		case strings.HasPrefix(target, "@"):
			if label != "" {
				return "@" + label
			}
			return "@" + name(target[1:])
		case strings.HasPrefix(target, "#"):
			if label != "" {
				return "#" + label
			}
			return "#" + target[1:]
		case strings.HasPrefix(target, "!subteam^"):
			if label != "" {
				return label
			}
			return "@group"
		case strings.HasPrefix(target, "!date^"):
			if label != "" {
				return label
			}
			return "a date"
		case strings.HasPrefix(target, "!"):
			return "@" + strings.TrimPrefix(target, "!")
		case strings.HasPrefix(target, "mailto:"):
			if label != "" {
				return label
			}
			return strings.TrimPrefix(target, "mailto:")
		}
		if label != "" && label != target {
			return label + " (" + target + ")"
		}
		return target
	})
	return strings.TrimSpace(html.UnescapeString(out))
}

// renderSlackDoc lays a channel-day out as plain text: one paragraph per
// thread, replies under their parent. It returns "" when nothing is left.
func renderSlackDoc(doc *slackDoc) (string, time.Time) {
	type msg struct {
		ts string
		e  slackEntry
	}
	threads := map[string][]msg{}
	var roots []string
	for ts, e := range doc.Messages {
		if e.Deleted || strings.TrimSpace(e.Text) == "" {
			continue
		}
		root := ts
		if e.Thread != "" {
			root = e.Thread
		}
		if _, ok := threads[root]; !ok {
			roots = append(roots, root)
		}
		threads[root] = append(threads[root], msg{ts, e})
	}
	if len(roots) == 0 {
		return "", time.Time{}
	}
	sort.Slice(roots, func(i, j int) bool { return tsLess(roots[i], roots[j]) })
	var b strings.Builder
	for i, root := range roots {
		ms := threads[root]
		sort.Slice(ms, func(i, j int) bool { return tsLess(ms[i].ts, ms[j].ts) })
		if i > 0 {
			b.WriteString("\n\n")
		}
		for k, mm := range ms {
			if k > 0 {
				b.WriteString("\n")
			}
			// A reply whose parent is gone (deleted, or older than the sync
			// window) is still marked as a reply.
			if mm.ts != root {
				b.WriteString("↳ ")
			}
			b.WriteString(tsTime(mm.ts).UTC().Format("15:04"))
			b.WriteString(" ")
			b.WriteString(mm.e.Name)
			b.WriteString(": ")
			b.WriteString(mm.e.Text)
		}
	}
	return b.String(), tsTime(roots[0]).UTC()
}

// tsLess compares Slack timestamps ("1700000000.000100"); "" sorts first.
func tsLess(a, b string) bool {
	if a == "" || b == "" {
		return a == "" && b != ""
	}
	ai, af, _ := strings.Cut(a, ".")
	bi, bf, _ := strings.Cut(b, ".")
	if len(ai) != len(bi) {
		return len(ai) < len(bi)
	}
	if ai != bi {
		return ai < bi
	}
	for len(af) < len(bf) {
		af += "0"
	}
	for len(bf) < len(af) {
		bf += "0"
	}
	return af < bf
}

func tsTime(ts string) time.Time {
	sec, _, _ := strings.Cut(ts, ".")
	n, _ := strconv.ParseInt(sec, 10, 64)
	return time.Unix(n, 0)
}
