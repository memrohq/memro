package connectors

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type gitlabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type gitlabMergeRequest struct {
	ID           int          `json:"id"`
	IID          int          `json:"iid"`
	ProjectID    int          `json:"project_id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	State        string       `json:"state"` // opened, closed, merged
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
	MergedAt     *string      `json:"merged_at,omitempty"`
	SourceBranch string       `json:"source_branch"`
	TargetBranch string       `json:"target_branch"`
	WebURL       string       `json:"web_url"`
	Author       gitlabUser   `json:"author"`
	Assignees    []gitlabUser `json:"assignees,omitempty"`
	Reviewers    []gitlabUser `json:"reviewers,omitempty"`
}

type gitlabIssue struct {
	ID          int          `json:"id"`
	IID         int          `json:"iid"`
	ProjectID   int          `json:"project_id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	State       string       `json:"state"` // opened, closed
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	ClosedAt    *string      `json:"closed_at,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
	WebURL      string       `json:"web_url"`
	Author      gitlabUser   `json:"author"`
	Assignees   []gitlabUser `json:"assignees,omitempty"`
}

type gitlabPipeline struct {
	ID        int        `json:"id"`
	ProjectID int        `json:"project_id"`
	Status    string     `json:"status"` // failed, success, running
	Ref       string     `json:"ref"`
	SHA       string     `json:"sha"`
	WebURL    string     `json:"web_url"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	User      gitlabUser `json:"user"`
}

type gitlabNote struct {
	ID        int        `json:"id"`
	Body      string     `json:"body"`
	Author    gitlabUser `json:"author"`
	CreatedAt string     `json:"created_at"`
	System    bool       `json:"system"`
}

func parseGitLabTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

var (
	gitlabIssueRefRegex = regexp.MustCompile(`(?:\b|#)(\d+)\b`)
	gitlabMRRefRegex    = regexp.MustCompile(`(?:\b|!)(\d+)\b`)
)

func renderGitLabMR(mr *gitlabMergeRequest, notes []gitlabNote, projectName string) (header, body string, meta map[string]any) {
	header = fmt.Sprintf("[GitLab MR !%d · %s · %s]", mr.IID, mr.Title, projectName)

	var b strings.Builder
	fmt.Fprintf(&b, "Merge Request: !%d\nTitle: %s\nProject: %s\n", mr.IID, mr.Title, projectName)
	fmt.Fprintf(&b, "Branches: %s -> %s\nState: %s\nAuthor: %s (@%s)\n", mr.SourceBranch, mr.TargetBranch, mr.State, mr.Author.Name, mr.Author.Username)

	if len(mr.Assignees) > 0 {
		var names []string
		for _, a := range mr.Assignees {
			names = append(names, fmt.Sprintf("%s (@%s)", a.Name, a.Username))
		}
		fmt.Fprintf(&b, "Assignees: %s\n", strings.Join(names, ", "))
	}
	if len(mr.Reviewers) > 0 {
		var names []string
		for _, r := range mr.Reviewers {
			names = append(names, fmt.Sprintf("%s (@%s)", r.Name, r.Username))
		}
		fmt.Fprintf(&b, "Reviewers: %s\n", strings.Join(names, ", "))
	}

	created := parseGitLabTime(mr.CreatedAt)
	updated := parseGitLabTime(mr.UpdatedAt)
	if !created.IsZero() {
		fmt.Fprintf(&b, "Created: %s", created.Format("2006-01-02"))
		if !updated.IsZero() {
			fmt.Fprintf(&b, " · Updated: %s", updated.Format("2006-01-02"))
		}
		b.WriteString("\n")
	}

	if mr.WebURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", mr.WebURL)
	}

	if strings.TrimSpace(mr.Description) != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(strings.TrimSpace(mr.Description))
		b.WriteString("\n")
	}

	// Discussions / Notes
	var userNotes []gitlabNote
	for _, n := range notes {
		if !n.System && strings.TrimSpace(n.Body) != "" {
			userNotes = append(userNotes, n)
		}
	}

	if len(userNotes) > 0 {
		b.WriteString("\nDiscussion:\n")
		for _, n := range userNotes {
			nDate := parseGitLabTime(n.CreatedAt)
			dateStr := ""
			if !nDate.IsZero() {
				dateStr = fmt.Sprintf(" (%s)", nDate.Format("2006-01-02 15:04"))
			}
			fmt.Fprintf(&b, "- %s (@%s)%s:\n  %s\n", n.Author.Name, n.Author.Username, dateStr, strings.TrimSpace(n.Body))
		}
	}

	meta = map[string]any{
		"mr_iid":        mr.IID,
		"project_name":  projectName,
		"state":         mr.State,
		"source_branch": mr.SourceBranch,
		"target_branch": mr.TargetBranch,
		"author":        mr.Author.Username,
	}

	return header, b.String(), meta
}

func renderGitLabPipelineFailure(p *gitlabPipeline, failedJobs []string, projectName string) (header, body string, meta map[string]any) {
	header = fmt.Sprintf("[GitLab Pipeline #%d · %s · FAILED]", p.ID, projectName)

	var b strings.Builder
	fmt.Fprintf(&b, "Pipeline: #%d\nStatus: FAILED\nProject: %s\nBranch: %s\nCommit: %s\nTriggered by: %s (@%s)\n",
		p.ID, projectName, p.Ref, p.SHA, p.User.Name, p.User.Username)

	if p.WebURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", p.WebURL)
	}

	if len(failedJobs) > 0 {
		b.WriteString("\nFailed Jobs:\n")
		for _, j := range failedJobs {
			fmt.Fprintf(&b, "- %s\n", j)
		}
	}

	meta = map[string]any{
		"pipeline_id":  p.ID,
		"project_name": projectName,
		"status":       p.Status,
		"ref":          p.Ref,
		"sha":          p.SHA,
	}

	return header, b.String(), meta
}
