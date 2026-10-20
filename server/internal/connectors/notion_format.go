package connectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/domain"
)

type notionRichText struct {
	Type        string `json:"type"`
	PlainText   string `json:"plain_text"`
	Href        string `json:"href,omitempty"`
	Annotations struct {
		Bold          bool `json:"bold"`
		Italic        bool `json:"italic"`
		Strikethrough bool `json:"strikethrough"`
		Underline     bool `json:"underline"`
		Code          bool `json:"code"`
	} `json:"annotations"`
}

func formatRichText(rts []notionRichText) string {
	var b strings.Builder
	for _, rt := range rts {
		txt := rt.PlainText
		if txt == "" {
			continue
		}
		if rt.Annotations.Code {
			txt = "`" + txt + "`"
		}
		if rt.Annotations.Bold && rt.Annotations.Italic {
			txt = "***" + txt + "***"
		} else if rt.Annotations.Bold {
			txt = "**" + txt + "**"
		} else if rt.Annotations.Italic {
			txt = "*" + txt + "*"
		}
		if rt.Annotations.Strikethrough {
			txt = "~~" + txt + "~~"
		}
		if rt.Href != "" {
			txt = fmt.Sprintf("[%s](%s)", txt, rt.Href)
		}
		b.WriteString(txt)
	}
	return b.String()
}

type notionParent struct {
	Type       string `json:"type"`
	DatabaseID string `json:"database_id,omitempty"`
	PageID     string `json:"page_id,omitempty"`
	Workspace  bool   `json:"workspace,omitempty"`
}

type notionPage struct {
	ID             string                    `json:"id"`
	CreatedTime    time.Time                 `json:"created_time"`
	LastEditedTime time.Time                 `json:"last_edited_time"`
	Archived       bool                      `json:"archived"`
	URL            string                    `json:"url"`
	Parent         notionParent              `json:"parent"`
	Properties     map[string]map[string]any `json:"properties"`
}

type notionBlock struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	HasChild  bool           `json:"has_children"`
	Paragraph *notionTextObj `json:"paragraph,omitempty"`
	Heading1  *notionTextObj `json:"heading_1,omitempty"`
	Heading2  *notionTextObj `json:"heading_2,omitempty"`
	Heading3  *notionTextObj `json:"heading_3,omitempty"`
	Bulleted  *notionTextObj `json:"bulleted_list_item,omitempty"`
	Numbered  *notionTextObj `json:"numbered_list_item,omitempty"`
	ToDo      *notionToDoObj `json:"to_do,omitempty"`
	Code      *notionCodeObj `json:"code,omitempty"`
	Callout   *notionTextObj `json:"callout,omitempty"`
	Quote     *notionTextObj `json:"quote,omitempty"`
	Divider   map[string]any `json:"divider,omitempty"`
}

type notionTextObj struct {
	RichText []notionRichText `json:"rich_text"`
}

type notionToDoObj struct {
	RichText []notionRichText `json:"rich_text"`
	Checked  bool             `json:"checked"`
}

type notionCodeObj struct {
	RichText []notionRichText `json:"rich_text"`
	Language string           `json:"language"`
}

func extractPageTitle(props map[string]map[string]any) string {
	for _, prop := range props {
		if pType, ok := prop["type"].(string); ok && pType == "title" {
			if rts, ok := prop["title"].([]any); ok {
				var titles []string
				for _, r := range rts {
					if m, ok := r.(map[string]any); ok {
						if pt, ok := m["plain_text"].(string); ok {
							titles = append(titles, pt)
						}
					}
				}
				if len(titles) > 0 {
					return strings.TrimSpace(strings.Join(titles, ""))
				}
			}
		}
	}
	return "Untitled Page"
}

func extractPropertyDetails(props map[string]map[string]any) []string {
	var details []string
	for name, prop := range props {
		pType, _ := prop["type"].(string)
		switch pType {
		case "status":
			if statusObj, ok := prop["status"].(map[string]any); ok {
				if sName, ok := statusObj["name"].(string); ok && sName != "" {
					details = append(details, fmt.Sprintf("%s: %s", name, sName))
				}
			}
		case "select":
			if selObj, ok := prop["select"].(map[string]any); ok {
				if sName, ok := selObj["name"].(string); ok && sName != "" {
					details = append(details, fmt.Sprintf("%s: %s", name, sName))
				}
			}
		case "multi_select":
			if opts, ok := prop["multi_select"].([]any); ok && len(opts) > 0 {
				var names []string
				for _, opt := range opts {
					if m, ok := opt.(map[string]any); ok {
						if n, ok := m["name"].(string); ok && n != "" {
							names = append(names, n)
						}
					}
				}
				if len(names) > 0 {
					details = append(details, fmt.Sprintf("%s: %s", name, strings.Join(names, ", ")))
				}
			}
		case "people":
			if people, ok := prop["people"].([]any); ok && len(people) > 0 {
				var names []string
				for _, p := range people {
					if m, ok := p.(map[string]any); ok {
						if n, ok := m["name"].(string); ok && n != "" {
							names = append(names, n)
						}
					}
				}
				if len(names) > 0 {
					details = append(details, fmt.Sprintf("%s: %s", name, strings.Join(names, ", ")))
				}
			}
		case "date":
			if dateObj, ok := prop["date"].(map[string]any); ok {
				if start, ok := dateObj["start"].(string); ok && start != "" {
					if end, ok := dateObj["end"].(string); ok && end != "" {
						details = append(details, fmt.Sprintf("%s: %s → %s", name, start, end))
					} else {
						details = append(details, fmt.Sprintf("%s: %s", name, start))
					}
				}
			}
		}
	}
	return details
}

func renderBlocks(blocks []notionBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		switch block.Type {
		case "paragraph":
			if block.Paragraph != nil {
				text := formatRichText(block.Paragraph.RichText)
				if text != "" {
					b.WriteString(text)
					b.WriteString("\n\n")
				}
			}
		case "heading_1":
			if block.Heading1 != nil {
				b.WriteString("# ")
				b.WriteString(formatRichText(block.Heading1.RichText))
				b.WriteString("\n\n")
			}
		case "heading_2":
			if block.Heading2 != nil {
				b.WriteString("## ")
				b.WriteString(formatRichText(block.Heading2.RichText))
				b.WriteString("\n\n")
			}
		case "heading_3":
			if block.Heading3 != nil {
				b.WriteString("### ")
				b.WriteString(formatRichText(block.Heading3.RichText))
				b.WriteString("\n\n")
			}
		case "bulleted_list_item":
			if block.Bulleted != nil {
				b.WriteString("- ")
				b.WriteString(formatRichText(block.Bulleted.RichText))
				b.WriteString("\n")
			}
		case "numbered_list_item":
			if block.Numbered != nil {
				b.WriteString("1. ")
				b.WriteString(formatRichText(block.Numbered.RichText))
				b.WriteString("\n")
			}
		case "to_do":
			if block.ToDo != nil {
				if block.ToDo.Checked {
					b.WriteString("- [x] ")
				} else {
					b.WriteString("- [ ] ")
				}
				b.WriteString(formatRichText(block.ToDo.RichText))
				b.WriteString("\n")
			}
		case "code":
			if block.Code != nil {
				lang := block.Code.Language
				if lang == "" {
					lang = "text"
				}
				fmt.Fprintf(&b, "```%s\n%s\n```\n\n", lang, formatRichText(block.Code.RichText))
			}
		case "callout":
			if block.Callout != nil {
				fmt.Fprintf(&b, "> %s\n\n", formatRichText(block.Callout.RichText))
			}
		case "quote":
			if block.Quote != nil {
				fmt.Fprintf(&b, "> %s\n\n", formatRichText(block.Quote.RichText))
			}
		case "divider":
			b.WriteString("---\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func renderNotionPage(workspaceName string, page *notionPage, blocks []notionBlock) (header, body string, meta map[string]any) {
	title := extractPageTitle(page.Properties)
	ws := workspaceName
	if ws == "" {
		ws = "Workspace"
	}
	header = fmt.Sprintf("[Notion · %s · %s]", ws, title)

	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", title)
	fmt.Fprintf(&b, "Workspace: %s\n", ws)
	if page.URL != "" {
		fmt.Fprintf(&b, "URL: %s\n", page.URL)
	}

	if !page.LastEditedTime.IsZero() {
		fmt.Fprintf(&b, "Last Edited: %s\n", page.LastEditedTime.Format("2006-01-02 15:04"))
	}

	props := extractPropertyDetails(page.Properties)
	if len(props) > 0 {
		b.WriteString("\nProperties:\n")
		for _, p := range props {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}

	content := renderBlocks(blocks)
	if content != "" {
		b.WriteString("\n---\n\n")
		b.WriteString(content)
	}

	meta = map[string]any{
		"source":         ProviderNotion,
		"connector":      true,
		"page_id":        page.ID,
		"workspace_name": ws,
		"title":          title,
		"url":            page.URL,
	}
	if page.Parent.DatabaseID != "" {
		meta["database_id"] = page.Parent.DatabaseID
	}

	return header, strings.TrimRight(b.String(), "\n"), meta
}

func notionItem(workspaceName string, page *notionPage, blocks []notionBlock) item {
	header, body, meta := renderNotionPage(workspaceName, page, blocks)
	sourceID := page.Parent.DatabaseID
	if sourceID == "" {
		sourceID = page.Parent.PageID
	}
	if sourceID == "" {
		sourceID = "workspace"
	}
	eventDate := page.LastEditedTime
	if eventDate.IsZero() {
		eventDate = page.CreatedTime
	}
	return item{
		Key:       "page:" + page.ID,
		SourceID:  sourceID,
		Header:    header,
		Body:      body,
		Type:      domain.Semantic,
		EventDate: &eventDate,
		Meta:      meta,
	}
}
