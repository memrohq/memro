package connectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/domain"
)

type linearIssue struct {
	ID          string         `json:"id"`
	Identifier  string         `json:"identifier"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Priority    int            `json:"priority"`
	State       linearState    `json:"state"`
	Assignee    *linearUser    `json:"assignee"`
	Creator     *linearUser    `json:"creator"`
	Project     *linearProject `json:"project"`
	Team        linearTeam     `json:"team"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	URL         string         `json:"url"`
}

type linearState struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type linearUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type linearProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type linearTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type linearComment struct {
	ID        string      `json:"id"`
	Body      string      `json:"body"`
	User      *linearUser `json:"user"`
	CreatedAt time.Time   `json:"createdAt"`
}

func priorityName(p int) string {
	switch p {
	case 1:
		return "Urgent"
	case 2:
		return "High"
	case 3:
		return "Medium"
	case 4:
		return "Low"
	default:
		return "None"
	}
}

// renderLinearIssue turns a Linear issue and its comments into a clean
// episodic/semantic memory item.
func renderLinearIssue(issue *linearIssue, comments []linearComment) (header, body string, meta map[string]any) {
	teamName := issue.Team.Name
	if teamName == "" {
		teamName = issue.Team.Key
	}
	header = fmt.Sprintf("[Linear %s · %s · %s]", issue.Identifier, issue.Title, teamName)

	var b strings.Builder
	fmt.Fprintf(&b, "Issue: %s\nTitle: %s\nTeam: %s (%s)\n", issue.Identifier, issue.Title, issue.Team.Name, issue.Team.Key)
	if issue.State.Name != "" {
		fmt.Fprintf(&b, "State: %s\n", issue.State.Name)
	}
	if issue.Priority > 0 {
		fmt.Fprintf(&b, "Priority: %s (%d)\n", priorityName(issue.Priority), issue.Priority)
	}
	if issue.Assignee != nil && issue.Assignee.Name != "" {
		fmt.Fprintf(&b, "Assignee: %s\n", issue.Assignee.Name)
	}
	if issue.Creator != nil && issue.Creator.Name != "" {
		fmt.Fprintf(&b, "Creator: %s\n", issue.Creator.Name)
	}
	if issue.Project != nil && issue.Project.Name != "" {
		fmt.Fprintf(&b, "Project: %s\n", issue.Project.Name)
	}
	if !issue.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "Created: %s", issue.CreatedAt.Format("2006-01-02"))
		if !issue.UpdatedAt.IsZero() {
			fmt.Fprintf(&b, " · Updated: %s", issue.UpdatedAt.Format("2006-01-02"))
		}
		b.WriteString("\n")
	}
	if issue.URL != "" {
		fmt.Fprintf(&b, "URL: %s\n", issue.URL)
	}

	if desc := strings.TrimSpace(issue.Description); desc != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(desc)
		b.WriteString("\n")
	}

	if len(comments) > 0 {
		b.WriteString("\nDiscussion:\n")
		for _, c := range comments {
			author := "Someone"
			if c.User != nil && c.User.Name != "" {
				author = c.User.Name
			}
			dateStr := ""
			if !c.CreatedAt.IsZero() {
				dateStr = c.CreatedAt.Format("2006-01-02 15:04") + " "
			}
			commentBody := strings.TrimSpace(c.Body)
			if commentBody != "" {
				fmt.Fprintf(&b, "- %s%s: %s\n", dateStr, author, commentBody)
			}
		}
	}

	meta = map[string]any{
		"source":      ProviderLinear,
		"connector":   true,
		"issue_id":    issue.ID,
		"identifier":  issue.Identifier,
		"team_id":     issue.Team.ID,
		"team_key":    issue.Team.Key,
		"url":         issue.URL,
		"priority":    priorityName(issue.Priority),
		"state":       issue.State.Name,
		"state_type":  issue.State.Type,
	}
	if issue.Project != nil {
		meta["project"] = issue.Project.Name
	}

	return header, strings.TrimRight(b.String(), "\n"), meta
}

func linearItem(issue *linearIssue, comments []linearComment) item {
	header, body, meta := renderLinearIssue(issue, comments)
	updated := issue.UpdatedAt
	if updated.IsZero() {
		updated = issue.CreatedAt
	}
	return item{
		Key:       "issue:" + issue.ID,
		SourceID:  issue.Team.ID,
		Header:    header,
		Body:      body,
		Type:      domain.Semantic,
		EventDate: &updated,
		Meta:      meta,
	}
}
