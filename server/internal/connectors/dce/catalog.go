package dce

import (
	"embed"
	"strings"
)

//go:embed catalog/*.yaml
var CatalogFS embed.FS

// CategoryMapping maps connector names to intuitive dashboard categories.
var CategoryMapping = map[string]string{
	"asana":    "work",
	"monday":   "work",
	"trello":   "work",
	"clickup":  "work",
	"basecamp": "work",
	"shortcut": "work",
	"youtrack": "work",
	"todoist":  "work",
	"height":   "work",
	"linear":   "work",
	"jira":     "work",

	"github":     "engineering",
	"gitlab":     "engineering",
	"sentry":     "engineering",
	"datadog":    "engineering",
	"pagerduty":  "engineering",
	"opsgenie":   "engineering",
	"newrelic":   "engineering",
	"circleci":   "engineering",
	"snyk":       "engineering",
	"sonarcloud": "engineering",
	"netlify":    "engineering",
	"vercel":     "engineering",
	"railway":    "engineering",
	"render":     "engineering",
	"supabase":   "engineering",
	"postman":    "engineering",
	"bitbucket":  "engineering",

	"notion":     "knowledge",
	"confluence": "knowledge",
	"airtable":   "knowledge",
	"coda":       "knowledge",
	"outline":    "knowledge",
	"obsidian":   "knowledge",
	"readwise":   "knowledge",
	"raindrop":   "knowledge",
	"pocket":     "knowledge",
	"zotero":     "knowledge",

	"slack":     "communication",
	"discord":   "communication",
	"hubspot":   "communication",
	"intercom":  "communication",
	"zendesk":   "communication",
	"freshdesk": "communication",
	"front":     "communication",
	"close":     "communication",
	"pipedrive": "communication",
	"harvest":   "communication",
	"helpscout": "communication",
}

// DisplayNames maps connector names to proper display casing.
var DisplayNames = map[string]string{
	"asana":      "Asana",
	"monday":     "Monday.com",
	"trello":     "Trello",
	"clickup":    "ClickUp",
	"basecamp":   "Basecamp",
	"shortcut":   "Shortcut",
	"youtrack":   "YouTrack",
	"todoist":    "Todoist",
	"height":     "Height",
	"linear":     "Linear",
	"jira":       "Jira",
	"github":     "GitHub",
	"gitlab":     "GitLab",
	"sentry":     "Sentry",
	"datadog":    "Datadog",
	"pagerduty":  "PagerDuty",
	"opsgenie":   "Opsgenie",
	"newrelic":   "New Relic",
	"circleci":   "CircleCI",
	"snyk":       "Snyk",
	"sonarcloud": "SonarCloud",
	"netlify":    "Netlify",
	"vercel":     "Vercel",
	"railway":    "Railway",
	"render":     "Render",
	"supabase":   "Supabase",
	"postman":    "Postman",
	"bitbucket":  "Bitbucket",
	"notion":     "Notion",
	"confluence": "Confluence",
	"airtable":   "Airtable",
	"coda":       "Coda",
	"outline":    "Outline",
	"obsidian":   "Obsidian",
	"readwise":   "Readwise",
	"raindrop":   "Raindrop.io",
	"pocket":     "Pocket",
	"zotero":     "Zotero",
	"slack":      "Slack",
	"discord":    "Discord",
	"hubspot":    "HubSpot",
	"intercom":   "Intercom",
	"zendesk":    "Zendesk",
	"freshdesk":  "Freshdesk",
	"front":      "Front",
	"close":      "Close CRM",
	"pipedrive":  "Pipedrive",
	"harvest":    "Harvest",
	"helpscout":  "Help Scout",
}

// FormattedDisplayName returns the formatted name for a connector.
func FormattedDisplayName(name string) string {
	if dn, ok := DisplayNames[strings.ToLower(name)]; ok {
		return dn
	}
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

// CategoryFor returns the category for a connector name.
func CategoryFor(name string) string {
	if cat, ok := CategoryMapping[strings.ToLower(name)]; ok {
		return cat
	}
	return "work"
}
