package connectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/domain"
)

type sentryProjectSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type sentryIssue struct {
	ID        string               `json:"id"`
	ShortID   string               `json:"shortId"`
	Title     string               `json:"title"`
	Culprit   string               `json:"culprit"`
	Level     string               `json:"level"`
	Status    string               `json:"status"`
	Count     int                  `json:"count"`
	UserCount int                  `json:"userCount"`
	FirstSeen time.Time            `json:"firstSeen"`
	LastSeen  time.Time            `json:"lastSeen"`
	Permalink string               `json:"permalink"`
	Project   sentryProjectSummary `json:"project"`
	Metadata  map[string]any       `json:"metadata"`
}

type sentryTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type sentryFrame struct {
	Filename string `json:"filename"`
	Function string `json:"function"`
	Lineno   int    `json:"lineno"`
	Context  string `json:"context_line"`
	InApp    bool   `json:"in_app"`
}

type sentryStacktrace struct {
	Frames []sentryFrame `json:"frames"`
}

type sentryExceptionValue struct {
	Type       string            `json:"type"`
	Value      string            `json:"value"`
	Stacktrace *sentryStacktrace `json:"stacktrace"`
}

type sentryEntry struct {
	Type string `json:"type"`
	Data struct {
		Values []sentryExceptionValue `json:"values"`
	} `json:"data"`
}

type sentryEvent struct {
	ID          string        `json:"id"`
	EventID     string        `json:"eventID"`
	Message     string        `json:"message"`
	DateCreated time.Time     `json:"dateCreated"`
	Tags        []sentryTag   `json:"tags"`
	Entries     []sentryEntry `json:"entries"`
}

func renderSentryIssue(issue *sentryIssue, latestEvent *sentryEvent) (header, body string, meta map[string]any) {
	projName := issue.Project.Slug
	if projName == "" {
		projName = issue.Project.Name
	}
	header = fmt.Sprintf("[Sentry · %s · Issue #%s · %s]", projName, issue.ID, issue.Title)

	var b strings.Builder
	fmt.Fprintf(&b, "Issue: %s (ID: %s)\nTitle: %s\n", issue.ShortID, issue.ID, issue.Title)
	fmt.Fprintf(&b, "Project: %s (%s)\n", issue.Project.Name, issue.Project.Slug)
	if issue.Status != "" {
		fmt.Fprintf(&b, "Status: %s\n", issue.Status)
	}
	if issue.Level != "" {
		fmt.Fprintf(&b, "Level: %s\n", issue.Level)
	}
	if issue.Culprit != "" {
		fmt.Fprintf(&b, "Culprit: %s\n", issue.Culprit)
	}

	if issue.Count > 0 {
		fmt.Fprintf(&b, "Occurrences: %d events", issue.Count)
		if issue.UserCount > 0 {
			fmt.Fprintf(&b, " across %d users", issue.UserCount)
		}
		b.WriteString("\n")
	}

	if !issue.FirstSeen.IsZero() {
		fmt.Fprintf(&b, "First Seen: %s", issue.FirstSeen.Format("2006-01-02 15:04"))
		if !issue.LastSeen.IsZero() {
			fmt.Fprintf(&b, " · Last Seen: %s", issue.LastSeen.Format("2006-01-02 15:04"))
		}
		b.WriteString("\n")
	}

	if issue.Permalink != "" {
		fmt.Fprintf(&b, "URL: %s\n", issue.Permalink)
	}

	// Extract stacktrace from latest event
	if latestEvent != nil {
		var stackLines []string
		for _, entry := range latestEvent.Entries {
			if entry.Type == "exception" {
				for _, exc := range entry.Data.Values {
					if exc.Type != "" || exc.Value != "" {
						fmt.Fprintf(&b, "\nException: %s: %s\n", exc.Type, exc.Value)
					}
					if exc.Stacktrace != nil && len(exc.Stacktrace.Frames) > 0 {
						frames := exc.Stacktrace.Frames
						// Keep up to the 6 most relevant frames (most recent/innermost at end)
						start := 0
						if len(frames) > 6 {
							start = len(frames) - 6
						}
						for _, f := range frames[start:] {
							loc := fmt.Sprintf("%s:%d", f.Filename, f.Lineno)
							fn := f.Function
							if fn == "" {
								fn = "<anonymous>"
							}
							line := fmt.Sprintf("  at %s in %s", fn, loc)
							if f.Context != "" {
								line += fmt.Sprintf("\n    --> %s", strings.TrimSpace(f.Context))
							}
							stackLines = append(stackLines, line)
						}
					}
				}
			}
		}

		if len(stackLines) > 0 {
			b.WriteString("Stacktrace:\n")
			b.WriteString(strings.Join(stackLines, "\n"))
			b.WriteString("\n")
		}

		// Tags (release, environment, runtime)
		var tagLines []string
		for _, tag := range latestEvent.Tags {
			switch tag.Key {
			case "release", "environment", "server_name", "os", "runtime", "handled":
				tagLines = append(tagLines, fmt.Sprintf("%s: %s", tag.Key, tag.Value))
			}
		}
		if len(tagLines) > 0 {
			b.WriteString("\nTags:\n")
			for _, t := range tagLines {
				fmt.Fprintf(&b, "- %s\n", t)
			}
		}
	}

	meta = map[string]any{
		"source":       ProviderSentry,
		"connector":    true,
		"issue_id":     issue.ID,
		"short_id":     issue.ShortID,
		"project_id":   issue.Project.ID,
		"project_slug": issue.Project.Slug,
		"level":        issue.Level,
		"status":       issue.Status,
		"culprit":      issue.Culprit,
		"url":          issue.Permalink,
	}

	return header, strings.TrimRight(b.String(), "\n"), meta
}

func sentryItem(issue *sentryIssue, latestEvent *sentryEvent) item {
	header, body, meta := renderSentryIssue(issue, latestEvent)
	eventDate := issue.LastSeen
	if eventDate.IsZero() {
		eventDate = issue.FirstSeen
	}
	return item{
		Key:       "issue:" + issue.ID,
		SourceID:  issue.Project.Slug,
		Header:    header,
		Body:      body,
		Type:      domain.Episodic,
		EventDate: &eventDate,
		Meta:      meta,
	}
}
