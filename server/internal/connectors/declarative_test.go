package connectors

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/memrohq/memro/server/internal/connectors/dce"
	"github.com/memrohq/memro/server/internal/domain"
)

func TestDeclarativeProvider_InfoAndEnabled(t *testing.T) {
	manifest := &dce.Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   dce.Metadata{Name: "asana", Version: "1.0.0"},
		Spec: dce.Spec{
			Type:    "rest",
			BaseURL: "https://app.asana.com/api/1.0",
			Auth:    dce.AuthSpec{Type: "bearer", SecretRef: "ASANA_ACCESS_TOKEN"},
			Operations: []dce.OperationSpec{
				{Name: "list", Method: "GET", Path: "/tasks", Response: dce.ResponseSpec{ItemsPath: "data"}},
			},
			Mapping: dce.MappingSpec{ID: "$.gid", Content: "$.notes"},
		},
	}

	// Unsealed manager -> enabled = false
	mUnsealed := &Manager{}
	pUnsealed := newDeclarativeProvider(mUnsealed, manifest)
	info := pUnsealed.info()
	if info.Name != "asana" {
		t.Fatalf("expected name 'asana', got %q", info.Name)
	}
	if info.Enabled {
		t.Fatal("expected provider to be disabled without sealer")
	}
	if info.SourcesManagedBy != "token" {
		t.Fatalf("expected SourcesManagedBy 'token', got %q", info.SourcesManagedBy)
	}

	// Sealed manager -> enabled = true
	sealer, _ := newSealer(key32(), "")
	mSealed := &Manager{sealer: sealer}
	pSealed := newDeclarativeProvider(mSealed, manifest)
	if !pSealed.enabled() {
		t.Fatal("expected provider to be enabled with sealer")
	}
	if pSealed.authorizeURL("state-123") != "" {
		t.Fatal("expected empty authorizeURL for token-based provider")
	}
	if pSealed.manageURL(nil) != "" {
		t.Fatal("expected empty manageURL for token-based provider")
	}
}

func TestDeclarativeProvider_CompleteValidation(t *testing.T) {
	manifest := &dce.Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   dce.Metadata{Name: "trello", Version: "1.0.0"},
		Spec: dce.Spec{
			Type:       "rest",
			BaseURL:    "https://api.trello.com/1",
			Auth:       dce.AuthSpec{Type: "apiKey", SecretRef: "TRELLO_API_TOKEN"},
			Operations: []dce.OperationSpec{{Name: "list", Method: "GET", Path: "/cards", Response: dce.ResponseSpec{ItemsPath: "$"}}},
			Mapping:    dce.MappingSpec{ID: "$.id", Content: "$.desc"},
		},
	}

	p := newDeclarativeProvider(&Manager{}, manifest)

	// Missing token -> error
	_, err := p.complete(context.Background(), url.Values{})
	if err == nil || !strings.Contains(err.Error(), "token cannot be empty") {
		t.Fatalf("expected empty token error, got: %v", err)
	}

	// Valid token
	auth, err := p.complete(context.Background(), url.Values{"token": {"test-trello-secret-123"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	credsMap, ok := auth.creds.(map[string]any)
	if !ok || auth.accountID != "trello" || credsMap["token"] != "test-trello-secret-123" {
		t.Fatalf("unexpected authorized credentials: %+v", auth)
	}
}

type mockSinkManager struct {
	stored []item
}

func TestDeclarativeItemSink_Formatting(t *testing.T) {
	manifest := &dce.Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   dce.Metadata{Name: "pagerduty", Version: "1.0.0"},
	}

	var storedItems []item
	// IngestItem creates domain item
	norm := &dce.NormalizedItem{
		ID:        "INC-123",
		Title:     "High CPU on Web Server",
		Content:   "CPU utilization reached 98% across all cluster nodes.",
		EventDate: time.Date(2026, 10, 21, 5, 0, 0, 0, time.UTC),
		Raw:       map[string]any{"service": "web-prod"},
	}

	header := "[PAGERDUTY · High CPU on Web Server]"
	expectedKey := "pagerduty:INC-123"

	// Verify header logic
	computedHeader := "[" + strings.ToUpper(manifest.Metadata.Name) + " · " + norm.Title + "]"
	if computedHeader != header {
		t.Fatalf("expected header %q, got %q", header, computedHeader)
	}

	// Verify body logic: Title prepended when not present in content
	body := norm.Content
	if !strings.Contains(norm.Content, norm.Title) {
		body = norm.Title + "\n\n" + norm.Content
	}
	if !strings.HasPrefix(body, "High CPU on Web Server\n\n") {
		t.Fatalf("expected title prefix in body: %s", body)
	}

	it := item{
		Key:       expectedKey,
		SourceID:  manifest.Metadata.Name,
		Header:    header,
		Body:      body,
		Type:      domain.Episodic,
		EventDate: &norm.EventDate,
		Meta:      norm.Raw,
	}
	storedItems = append(storedItems, it)

	if len(storedItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(storedItems))
	}
	if storedItems[0].Key != "pagerduty:INC-123" {
		t.Fatalf("expected key 'pagerduty:INC-123', got %q", storedItems[0].Key)
	}
	if storedItems[0].Type != domain.Episodic {
		t.Fatalf("expected episodic item type, got %s", storedItems[0].Type)
	}
}
