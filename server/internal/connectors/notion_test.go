package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestVerifyNotionSignature(t *testing.T) {
	body := []byte(`{"type":"page_updated","page_id":"pg-1234"}`)
	secret := "notion_test_secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	if !VerifyNotionSignature(secret, sig, body) {
		t.Fatal("valid signature rejected")
	}
	if VerifyNotionSignature("wrong_secret", sig, body) {
		t.Fatal("wrong secret accepted")
	}
	if VerifyNotionSignature(secret, "bad_sig", body) {
		t.Fatal("bad signature accepted")
	}
	if VerifyNotionSignature("", sig, body) {
		t.Fatal("empty secret accepted")
	}
}

func TestRenderNotionPage(t *testing.T) {
	lastEdited := time.Date(2026, 10, 20, 16, 45, 0, 0, time.UTC)

	page := &notionPage{
		ID:             "page-99",
		LastEditedTime: lastEdited,
		URL:            "https://notion.so/acme/page-99",
		Parent: notionParent{
			Type:       "database_id",
			DatabaseID: "db-eng-specs",
		},
		Properties: map[string]map[string]any{
			"Name": {
				"type": "title",
				"title": []any{
					map[string]any{"plain_text": "RFC 42: Cross-Source Connector Architecture"},
				},
			},
			"Status": {
				"type":   "status",
				"status": map[string]any{"name": "In Review"},
			},
			"Tags": {
				"type": "multi_select",
				"multi_select": []any{
					map[string]any{"name": "Architecture"},
					map[string]any{"name": "Backend"},
				},
			},
			"Sprint": {
				"type":   "select",
				"select": map[string]any{"name": "2026-Q4-Sprint-3"},
			},
		},
	}

	blocks := []notionBlock{
		{
			Type: "heading_1",
			Heading1: &notionTextObj{
				RichText: []notionRichText{
					{PlainText: "Overview & Objectives"},
				},
			},
		},
		{
			Type: "paragraph",
			Paragraph: &notionTextObj{
				RichText: []notionRichText{
					{PlainText: "This RFC links with Linear ticket "},
					{PlainText: "ENG-404", Annotations: struct {
						Bold          bool `json:"bold"`
						Italic        bool `json:"italic"`
						Strikethrough bool `json:"strikethrough"`
						Underline     bool `json:"underline"`
						Code          bool `json:"code"`
					}{Bold: true}},
					{PlainText: " and resolves issue "},
					{PlainText: "#142", Href: "https://github.com/acme/api/issues/142"},
					{PlainText: " reported in Sentry #9412."},
				},
			},
		},
		{
			Type: "to_do",
			ToDo: &notionToDoObj{
				Checked: true,
				RichText: []notionRichText{
					{PlainText: "Implement OAuth flow"},
				},
			},
		},
		{
			Type: "to_do",
			ToDo: &notionToDoObj{
				Checked: false,
				RichText: []notionRichText{
					{PlainText: "Deploy to staging"},
				},
			},
		},
		{
			Type: "code",
			Code: &notionCodeObj{
				Language: "go",
				RichText: []notionRichText{
					{PlainText: "func SyncNotion() error { return nil }"},
				},
			},
		},
		{
			Type: "callout",
			Callout: &notionTextObj{
				RichText: []notionRichText{
					{PlainText: "Important: Ensure AES-256-GCM token encryption."},
				},
			},
		},
	}

	header, body, meta := renderNotionPage("Acme Corp", page, blocks)

	wantHeader := "[Notion · Acme Corp · RFC 42: Cross-Source Connector Architecture]"
	if header != wantHeader {
		t.Fatalf("got header %q, want %q", header, wantHeader)
	}

	for _, substr := range []string{
		"Title: RFC 42: Cross-Source Connector Architecture",
		"Workspace: Acme Corp",
		"Status: In Review",
		"Tags: Architecture, Backend",
		"Sprint: 2026-Q4-Sprint-3",
		"# Overview & Objectives",
		"This RFC links with Linear ticket **ENG-404**",
		"[#142](https://github.com/acme/api/issues/142)",
		"- [x] Implement OAuth flow",
		"- [ ] Deploy to staging",
		"```go\nfunc SyncNotion() error { return nil }\n```",
		"> Important: Ensure AES-256-GCM token encryption.",
	} {
		if !strings.Contains(body, substr) {
			t.Fatalf("body missing %q:\n%s", substr, body)
		}
	}

	if meta["page_id"] != "page-99" || meta["database_id"] != "db-eng-specs" || meta["source"] != ProviderNotion {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	// Verify cross-source reference extraction on the rendered Notion page body
	refs := ExtractReferences(body)
	hasTicket, hasGitHub, hasSentry := false, false, false
	for _, r := range refs {
		if r.Kind == "ticket" && r.Key == "eng-404" {
			hasTicket = true
		}
		if r.Kind == "github" && r.Key == "#142" {
			hasGitHub = true
		}
		if r.Kind == "sentry" && r.Key == "9412" {
			hasSentry = true
		}
	}

	if !hasTicket {
		t.Errorf("expected ticket ref eng-404 in refs: %+v", refs)
	}
	if !hasGitHub {
		t.Errorf("expected github ref #142 in refs: %+v", refs)
	}
	if !hasSentry {
		t.Errorf("expected sentry ref 9412 in refs: %+v", refs)
	}

	it := notionItem("Acme Corp", page, blocks)
	if it.Key != "page:page-99" {
		t.Fatalf("got item key %q", it.Key)
	}
	if it.SourceID != "db-eng-specs" {
		t.Fatalf("got item source_id %q", it.SourceID)
	}
}
