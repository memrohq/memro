package connectors

import (
	"fmt"
	"regexp"
	"strings"
)

// HTML comments are how issue and PR templates hide their instructions.
var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func cleanMarkdown(s string) string {
	s = htmlComment.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

// renderGitHubIssue lays out an issue or pull request with its discussion.
// Comments from bots (CI, dependency updates) are left out.
func renderGitHubIssue(repo string, is *githubIssue, comments []githubComment) (string, string, map[string]any) {
	kind, state := "Issue", is.State
	if is.PullRequest != nil {
		kind = "Pull request"
		if is.PullRequest.MergedAt != nil {
			state = "merged"
		}
	}
	if is.State == "closed" && is.StateReason == "not_planned" {
		state = "closed (not planned)"
	}
	header := fmt.Sprintf("[GitHub %s#%d · %s]", repo, is.Number, truncate(is.Title, 120))

	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d: %s\n", kind, is.Number, is.Title)
	fmt.Fprintf(&b, "State: %s · Opened by %s on %s", state, is.User.Login, is.CreatedAt.UTC().Format("2006-01-02"))
	if len(is.Labels) > 0 {
		names := make([]string, len(is.Labels))
		for i, l := range is.Labels {
			names[i] = l.Name
		}
		fmt.Fprintf(&b, " · Labels: %s", strings.Join(names, ", "))
	}
	b.WriteString("\n" + is.HTMLURL)
	if body := cleanMarkdown(is.Body); body != "" {
		b.WriteString("\n\n" + body)
	}
	for _, c := range comments {
		text := cleanMarkdown(c.Body)
		if text == "" || c.User.Type == "Bot" {
			continue
		}
		fmt.Fprintf(&b, "\n\n%s commented on %s: %s", c.User.Login, c.CreatedAt.UTC().Format("2006-01-02"), text)
	}
	meta := map[string]any{"repo": repo, "number": is.Number, "kind": strings.ToLower(strings.ReplaceAll(kind, " ", "_")),
		"state": state, "url": is.HTMLURL, "title": is.Title}
	return header, b.String(), meta
}
