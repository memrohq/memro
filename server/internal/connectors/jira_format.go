package connectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/domain"
)

type jiraIssue struct {
	ID     string          `json:"id"`
	Key    string          `json:"key"`
	Self   string          `json:"self"`
	Fields jiraIssueFields `json:"fields"`
}

type jiraIssueFields struct {
	Summary     string        `json:"summary"`
	Description string        `json:"description"`
	Status      jiraStatus    `json:"status"`
	Priority    *jiraPriority `json:"priority"`
	IssueType   jiraIssueType `json:"issuetype"`
	Project     jiraProject   `json:"project"`
	Assignee    *jiraUser     `json:"assignee"`
	Reporter    *jiraUser     `json:"reporter"`
	Created     string        `json:"created"`
	Updated     string        `json:"updated"`
}

type jiraStatus struct {
	Name           string             `json:"name"`
	StatusCategory jiraStatusCategory `json:"statusCategory"`
}

type jiraStatusCategory struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type jiraPriority struct {
	Name string `json:"name"`
}

type jiraIssueType struct {
	Name string `json:"name"`
}

type jiraProject struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type jiraUser struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
}

type jiraComment struct {
	ID      string    `json:"id"`
	Author  *jiraUser `json:"author"`
	Body    string    `json:"body"`
	Created string    `json:"created"`
}

func parseJiraTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Jira ISO 8601 formats: 2026-10-21T03:45:00.000+0000 or RFC3339
	if t, err := time.Parse("2006-01-02T15:04:05.000-0700", s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// renderJiraIssue formats a Jira issue and its comments into a clean context memory item.
func renderJiraIssue(issue *jiraIssue, comments []jiraComment, baseURL string) (header, body string, meta map[string]any) {
	projName := issue.Fields.Project.Name
	if projName == "" {
		projName = issue.Fields.Project.Key
	}
	header = fmt.Sprintf("[Jira %s · %s · %s]", issue.Key, issue.Fields.Summary, projName)

	var b strings.Builder
	fmt.Fprintf(&b, "Issue: %s\nTitle: %s\nProject: %s (%s)\n", issue.Key, issue.Fields.Summary, projName, issue.Fields.Project.Key)
	if issue.Fields.IssueType.Name != "" {
		fmt.Fprintf(&b, "Type: %s\n", issue.Fields.IssueType.Name)
	}
	if issue.Fields.Status.Name != "" {
		fmt.Fprintf(&b, "Status: %s\n", issue.Fields.Status.Name)
	}
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != "" {
		fmt.Fprintf(&b, "Priority: %s\n", issue.Fields.Priority.Name)
	}
	if issue.Fields.Assignee != nil && issue.Fields.Assignee.DisplayName != "" {
		fmt.Fprintf(&b, "Assignee: %s\n", issue.Fields.Assignee.DisplayName)
	}
	if issue.Fields.Reporter != nil && issue.Fields.Reporter.DisplayName != "" {
		fmt.Fprintf(&b, "Reporter: %s\n", issue.Fields.Reporter.DisplayName)
	}

	created := parseJiraTime(issue.Fields.Created)
	updated := parseJiraTime(issue.Fields.Updated)
	if !created.IsZero() {
		fmt.Fprintf(&b, "Created: %s", created.Format("2006-01-02"))
		if !updated.IsZero() {
			fmt.Fprintf(&b, " · Updated: %s", updated.Format("2006-01-02"))
		}
		b.WriteString("\n")
	}

	issueURL := issue.Self
	if baseURL != "" {
		issueURL = fmt.Sprintf("%s/browse/%s", strings.TrimRight(baseURL, "/"), issue.Key)
	}
	if issueURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", issueURL)
	}

	if issue.Fields.Description != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(strings.TrimSpace(issue.Fields.Description))
		b.WriteString("\n")
	}

	if len(comments) > 0 {
		b.WriteString("\nDiscussion:\n")
		for _, c := range comments {
			author := "User"
			if c.Author != nil && c.Author.DisplayName != "" {
				author = c.Author.DisplayName
			}
			cTime := parseJiraTime(c.Created)
			dateStr := ""
			if !cTime.IsZero() {
				dateStr = fmt.Sprintf(" (%s)", cTime.Format("2006-01-02 15:04"))
			}
			fmt.Fprintf(&b, "- %s%s:\n  %s\n", author, dateStr, strings.ReplaceAll(strings.TrimSpace(c.Body), "\n", "\n  "))
		}
	}

	meta = map[string]any{
		"source":        "jira",
		"connector":     true,
		"issue_key":     issue.Key,
		"issue_id":      issue.ID,
		"project_key":   issue.Fields.Project.Key,
		"project_name":  issue.Fields.Project.Name,
		"status":        issue.Fields.Status.Name,
		"type":          domain.Semantic,
		"url":           issueURL,
		"comments":      len(comments),
	}
	if issue.Fields.Assignee != nil {
		meta["assignee"] = issue.Fields.Assignee.DisplayName
	}

	return header, b.String(), meta
}
