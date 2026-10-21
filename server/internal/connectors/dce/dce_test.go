package dce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseManifestYAML(t *testing.T) {
	manifestYAML := []byte(`
apiVersion: memro.dev/v1
kind: Connector

metadata:
  name: linear-dce
  version: 1.0.0
  description: Linear REST declarative connector

spec:
  type: rest
  baseURL: https://api.linear.app

  auth:
    type: bearer
    secretRef: LINEAR_API_KEY

  operations:
    - name: list_issues
      method: GET
      path: /api/issues
      pagination:
        type: cursor
        cursorParam: after
        cursorPath: nextCursor
      response:
        itemsPath: data.issues

  mapping:
    id: $.id
    title: $.title
    content: $.description
    eventDate: $.updatedAt

  sync:
    mode: incremental
    checkpoint: updatedAt

  policies:
    timeout: 10s
    maxRetries: 3
`)

	m, err := ParseManifestYAML(manifestYAML)
	if err != nil {
		t.Fatalf("ParseManifestYAML failed: %v", err)
	}

	if m.Metadata.Name != "linear-dce" {
		t.Fatalf("expected name 'linear-dce', got %q", m.Metadata.Name)
	}
	if m.Spec.Auth.Type != "bearer" {
		t.Fatalf("expected auth type 'bearer', got %q", m.Spec.Auth.Type)
	}
	if len(m.Spec.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Spec.Operations))
	}
	if m.Spec.Mapping.ID != "$.id" {
		t.Fatalf("expected mapping.id '$.id', got %q", m.Spec.Mapping.ID)
	}
	if m.Spec.Policies.MaxRetries != 3 {
		t.Fatalf("expected maxRetries 3, got %d", m.Spec.Policies.MaxRetries)
	}
}

func TestSSRFProtection(t *testing.T) {
	blockedURLs := []string{
		"http://127.0.0.1:8080/admin",
		"http://localhost:5432",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/secrets",
		"http://192.168.1.1/router",
		"http://172.16.0.1/internal",
		"ftp://api.example.com",
	}

	for _, u := range blockedURLs {
		err := ValidateDestinationURL(u, false)
		if err == nil {
			t.Fatalf("expected URL %q to be blocked by SSRF validation, but it passed", u)
		}
	}

	// Permitted public URLs
	allowedURLs := []string{
		"https://api.linear.app",
		"https://api.github.com",
	}

	for _, u := range allowedURLs {
		if err := ValidateDestinationURL(u, false); err != nil {
			t.Fatalf("expected public URL %q to be allowed, got error: %v", u, err)
		}
	}
}

func TestSecretRedaction(t *testing.T) {
	secret := "super-secret-linear-token-987"
	leakErr := errors.New("request failed with Authorization: Bearer " + secret)

	sanitized := sanitizeError(leakErr, secret)
	if strings.Contains(sanitized.Error(), secret) {
		t.Fatalf("secret leaked in sanitized error: %s", sanitized.Error())
	}
	if !strings.Contains(sanitized.Error(), "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in error, got: %s", sanitized.Error())
	}
}

type memorySink struct {
	items []*NormalizedItem
}

func (s *memorySink) IngestItem(ctx context.Context, item *NormalizedItem) error {
	s.items = append(s.items, item)
	return nil
}

func TestRuntimeExecutionWithCursorPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer secret-token-xyz" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		cursor := r.URL.Query().Get("after")
		w.Header().Set("Content-Type", "application/json")

		if cursor == "" {
			resp := map[string]any{
				"data": map[string]any{
					"issues": []map[string]any{
						{
							"id":          "ISS-1",
							"title":       "Database pool leak",
							"description": "Fix connection exhaustion under load.",
							"updatedAt":   "2026-10-21T02:00:00Z",
						},
						{
							"id":          "ISS-2",
							"title":       "Add DCE engine",
							"description": "Declarative connectors architecture for Memro.",
							"updatedAt":   "2026-10-21T03:00:00Z",
						},
					},
				},
				"nextCursor": "page-2-cursor",
			}
			json.NewEncoder(w).Encode(resp)
			return
		} else if cursor == "page-2-cursor" {
			resp := map[string]any{
				"data": map[string]any{
					"issues": []map[string]any{
						{
							"id":          "ISS-3",
							"title":       "Benchmark 500 questions",
							"description": "Run LongMemEval with session continuity.",
							"updatedAt":   "2026-10-21T04:00:00Z",
						},
					},
				},
				"nextCursor": "",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata: Metadata{
			Name:    "test-service",
			Version: "1.0.0",
		},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth: AuthSpec{
				Type:      "bearer",
				SecretRef: "TEST_KEY",
			},
			Operations: []OperationSpec{
				{
					Name:   "list_items",
					Method: "GET",
					Path:   "/api/items",
					Pagination: PaginationSpec{
						Type:        "cursor",
						CursorParam: "after",
						CursorPath:  "nextCursor",
					},
					Response: ResponseSpec{
						ItemsPath: "data.issues",
					},
				},
			},
			Mapping: MappingSpec{
				ID:        "$.id",
				Title:     "$.title",
				Content:   "$.description",
				EventDate: "$.updatedAt",
			},
			Sync: SyncSpec{
				Mode:       "incremental",
				Checkpoint: "updatedAt",
			},
			Policies: PolicySpec{
				Timeout:    "5s",
				MaxRetries: 2,
			},
		},
	}

	secrets := MapSecretResolver{
		"TEST_KEY": "secret-token-xyz",
	}

	runtime := NewRuntimeWithOptions(server.Client(), secrets, Options{AllowPrivateIPs: true})
	sink := &memorySink{}

	res, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err != nil {
		t.Fatalf("Runtime.Execute failed: %v", err)
	}

	if res.ItemsSynced != 3 {
		t.Fatalf("expected 3 items synced across 2 pages, got %d", res.ItemsSynced)
	}
	if len(sink.items) != 3 {
		t.Fatalf("expected 3 items in sink, got %d", len(sink.items))
	}

	item1 := sink.items[0]
	if item1.ID != "ISS-1" || item1.Title != "Database pool leak" {
		t.Fatalf("item 0 mismatch: %+v", item1)
	}

	item3 := sink.items[2]
	if item3.ID != "ISS-3" || !strings.Contains(item3.Content, "LongMemEval") {
		t.Fatalf("item 2 mismatch: %+v", item3)
	}

	expectedDate, _ := time.Parse(time.RFC3339, "2026-10-21T04:00:00Z")
	if !item3.EventDate.Equal(expectedDate) {
		t.Fatalf("expected event date %v, got %v", expectedDate, item3.EventDate)
	}

	if res.NewCheckpoint != "2026-10-21T04:00:00Z" {
		t.Fatalf("expected checkpoint '2026-10-21T04:00:00Z', got %q", res.NewCheckpoint)
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata: Metadata{
			Name:    "linear-api",
			Version: "1.0.0",
		},
		Spec: Spec{
			Type:    "rest",
			BaseURL: "https://api.linear.app",
			Auth: AuthSpec{
				Type:      "bearer",
				SecretRef: "LINEAR_KEY",
			},
			Operations: []OperationSpec{
				{
					Name:   "list",
					Method: "GET",
					Path:   "/issues",
					Response: ResponseSpec{
						ItemsPath: "data.issues",
					},
				},
			},
			Mapping: MappingSpec{
				ID:      "$.id",
				Content: "$.title",
			},
		},
	}

	if err := reg.Register(manifest); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if reg.Count() != 1 {
		t.Fatalf("expected 1 manifest, got %d", reg.Count())
	}

	m, ok := reg.Get("LINEAR-API") // case-insensitive lookup
	if !ok || m.Metadata.Name != "linear-api" {
		t.Fatalf("lookup failed for 'LINEAR-API'")
	}

	all := reg.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 manifest in All(), got %d", len(all))
	}
}

func TestCatalogManifests(t *testing.T) {
	reg := NewRegistry()
	if err := reg.LoadFromDir("./catalog"); err != nil {
		t.Fatalf("failed to load catalog manifests: %v", err)
	}
	t.Logf("Successfully loaded and validated %d manifests from catalog", reg.Count())
	for _, m := range reg.All() {
		if err := m.Validate(); err != nil {
			t.Errorf("manifest %s failed validation: %v", m.Metadata.Name, err)
		}
	}
}
