package dce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPagination_PageBased(t *testing.T) {
	var requestedPages []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		var pNum int
		fmt.Sscanf(page, "%d", &pNum)
		requestedPages = append(requestedPages, pNum)

		w.Header().Set("Content-Type", "application/json")
		switch pNum {
		case 1:
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "p1-1", "name": "Task 1", "desc": "Desc 1"},
					{"id": "p1-2", "name": "Task 2", "desc": "Desc 2"},
				},
			})
		case 2:
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "p2-1", "name": "Task 3", "desc": "Desc 3"},
					{"id": "p2-2", "name": "Task 4", "desc": "Desc 4"},
				},
			})
		case 3: // Partial page (less than limit of 2) -> indicates end of pagination
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "p3-1", "name": "Task 5", "desc": "Desc 5"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{},
			})
		}
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-paged", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:   "list",
					Method: "GET",
					Path:   "/tasks",
					Pagination: PaginationSpec{
						Type:      "page",
						PageParam: "p",
						Limit:     2,
					},
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Title: "$.name", Content: "$.desc"},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{AllowPrivateIPs: true})

	res, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ItemsSynced != 5 {
		t.Fatalf("expected 5 items synced across 3 pages, got %d", res.ItemsSynced)
	}
	if res.PagesProcessed != 3 {
		t.Fatalf("expected 3 pages processed, got %d", res.PagesProcessed)
	}
	if res.HasMore {
		t.Fatalf("expected HasMore = false at end of partial page")
	}
	if len(sink.items) != 5 {
		t.Fatalf("expected 5 items in sink, got %d", len(sink.items))
	}
}

func TestPagination_OffsetBased(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		limit := r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")

		if limit != "2" {
			t.Errorf("expected limit=2, got %s", limit)
		}

		switch offset {
		case "0":
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "o-1", "desc": "Item 1"},
					{"id": "o-2", "desc": "Item 2"},
				},
			})
		case "2":
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "o-3", "desc": "Item 3"},
					{"id": "o-4", "desc": "Item 4"},
				},
			})
		default: // Empty -> reached end
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{},
			})
		}
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-offset", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:   "list",
					Method: "GET",
					Path:   "/items",
					Pagination: PaginationSpec{
						Type:        "offset",
						OffsetParam: "offset",
						LimitParam:  "limit",
						Limit:       2,
					},
					Response: ResponseSpec{ItemsPath: "data"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc"},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{AllowPrivateIPs: true})

	res, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ItemsSynced != 4 {
		t.Fatalf("expected 4 items synced, got %d", res.ItemsSynced)
	}
	if res.PagesProcessed != 3 { // offset 0, offset 2, offset 4 (empty)
		t.Fatalf("expected 3 pages processed, got %d", res.PagesProcessed)
	}
}

func TestPagination_MaxPagesBound(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		// Always returns full page with next cursor to simulate unbounded stream
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": fmt.Sprintf("inf-%d", atomic.LoadInt32(&callCount)), "desc": "infinite"},
			},
			"next": fmt.Sprintf("cursor-%d", atomic.LoadInt32(&callCount)),
		})
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-infinite", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:   "list",
					Method: "GET",
					Path:   "/stream",
					Pagination: PaginationSpec{
						Type:        "cursor",
						CursorParam: "after",
						CursorPath:  "next",
					},
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc"},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{
		AllowPrivateIPs: true,
		MaxPages:        3, // Bound to 3 pages max
	})

	res, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.PagesProcessed != 3 {
		t.Fatalf("expected exactly 3 pages processed, got %d", res.PagesProcessed)
	}
	if !res.HasMore {
		t.Fatalf("expected HasMore = true when stopped by MaxPages bound")
	}
}

func TestRetry_429WithRetryAfterHeader(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limit exceeded"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "ok-1", "desc": "success"}},
		})
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-429", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:     "list",
					Method:   "GET",
					Path:     "/rate",
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc"},
			Policies: PolicySpec{
				MaxRetries: 3,
			},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{AllowPrivateIPs: true})

	res, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err != nil {
		t.Fatalf("expected recovery after 429, got error: %v", err)
	}
	if res.ItemsSynced != 1 {
		t.Fatalf("expected 1 item synced, got %d", res.ItemsSynced)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestRetry_Transient5xxRecovery(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att == 1 {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("502 Bad Gateway"))
			return
		}
		if att == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("503 Service Unavailable"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "rec-1", "desc": "recovered"}},
		})
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-5xx", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:     "list",
					Method:   "GET",
					Path:     "/flaky",
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping:  MappingSpec{ID: "$.id", Content: "$.desc"},
			Policies: PolicySpec{MaxRetries: 3},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{AllowPrivateIPs: true})

	res, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err != nil {
		t.Fatalf("expected recovery after transient 502/503, got: %v", err)
	}
	if res.ItemsSynced != 1 {
		t.Fatalf("expected 1 item synced, got %d", res.ItemsSynced)
	}
}

func TestFastFail_401Unauthorized(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api token"}`))
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-auth-fail", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "bearer", SecretRef: "TOKEN"},
			Operations: []OperationSpec{
				{
					Name:     "list",
					Method:   "GET",
					Path:     "/secure",
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping:  MappingSpec{ID: "$.id", Content: "$.desc"},
			Policies: PolicySpec{MaxRetries: 5},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{"TOKEN": "bad-key"}, Options{AllowPrivateIPs: true})

	_, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err == nil {
		t.Fatal("expected error on 401 Unauthorized")
	}

	// Must fail fast on attempt 1 without wasting retries
	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatalf("expected exactly 1 attempt for 401 Unauthorized, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestSecretRedaction_InErrorsAndPayloads(t *testing.T) {
	secretVal := "super-confidential-token-999"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf(`{"error": "rejected token %s"}`, secretVal)))
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-redact", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "bearer", SecretRef: "SECRET"},
			Operations: []OperationSpec{
				{
					Name:     "list",
					Method:   "GET",
					Path:     "/error",
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc"},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{"SECRET": secretVal}, Options{AllowPrivateIPs: true})

	_, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, secretVal) {
		t.Fatalf("leak detected! Secret value found in error message: %s", errMsg)
	}
	if !strings.Contains(errMsg, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in error message: %s", errMsg)
	}
}

func TestResponseSizeLimit_Enforcement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Output 50KB payload
		largeStr := strings.Repeat("A", 50*1024)
		w.Write([]byte(fmt.Sprintf(`{"items":[{"id":"1","desc":"%s"}]}`, largeStr)))
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-size-limit", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:     "list",
					Method:   "GET",
					Path:     "/large",
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc"},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{
		AllowPrivateIPs:  true,
		MaxResponseBytes: 10 * 1024, // 10KB limit
	})

	_, err := runtime.Execute(context.Background(), manifest, sink, "")
	if err == nil {
		t.Fatal("expected ErrBodyTooLarge")
	}
	if err != ErrBodyTooLarge {
		t.Fatalf("expected ErrBodyTooLarge, got: %v", err)
	}
}

func TestTimestampParsing_Formats(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want time.Time
	}{
		{
			name: "RFC3339",
			val:  "2026-10-21T04:15:30Z",
			want: time.Date(2026, 10, 21, 4, 15, 30, 0, time.UTC),
		},
		{
			name: "UnixMillisFloat",
			val:  float64(1700000000000), // 2023-11-14T22:13:20Z
			want: time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC),
		},
		{
			name: "UnixMillisString",
			val:  "1700000000000",
			want: time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC),
		},
		{
			name: "UnixSecondsFloat",
			val:  float64(1700000000),
			want: time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC),
		},
		{
			name: "ISODateOnly",
			val:  "2026-10-21",
			want: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTimestamp(tc.val)
			if !got.Equal(tc.want) {
				t.Fatalf("parseTimestamp(%v) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

func TestHighWaterMark_MonotonicCheckpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Unordered timestamps: Middle item has the latest timestamp
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "1", "updated": "2026-10-20T10:00:00Z", "desc": "first"},
				{"id": "2", "updated": "2026-10-20T18:00:00Z", "desc": "highest"},
				{"id": "3", "updated": "2026-10-20T12:00:00Z", "desc": "older"},
			},
		})
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-checkpoint", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{
					Name:     "list",
					Method:   "GET",
					Path:     "/items",
					Response: ResponseSpec{ItemsPath: "items"},
				},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc", EventDate: "$.updated"},
			Sync: SyncSpec{
				Mode:       "incremental",
				Checkpoint: "updated",
			},
		},
	}

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{AllowPrivateIPs: true})

	res, err := runtime.Execute(context.Background(), manifest, sink, "2026-10-19T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must be highest timestamp ("2026-10-20T18:00:00Z"), not last item ("2026-10-20T12:00:00Z")
	if res.NewCheckpoint != "2026-10-20T18:00:00Z" {
		t.Fatalf("expected high-water mark '2026-10-20T18:00:00Z', got %q", res.NewCheckpoint)
	}
}

func TestContextCancellation_AbortsImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	manifest := &Manifest{
		APIVersion: "memro.dev/v1",
		Kind:       "Connector",
		Metadata:   Metadata{Name: "test-cancel", Version: "1.0.0"},
		Spec: Spec{
			Type:    "rest",
			BaseURL: server.URL,
			Auth:    AuthSpec{Type: "none"},
			Operations: []OperationSpec{
				{Name: "list", Method: "GET", Path: "/slow", Response: ResponseSpec{ItemsPath: "items"}},
			},
			Mapping: MappingSpec{ID: "$.id", Content: "$.desc"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	sink := &memorySink{}
	runtime := NewRuntimeWithOptions(server.Client(), MapSecretResolver{}, Options{AllowPrivateIPs: true})

	_, err := runtime.Execute(ctx, manifest, sink, "")
	if err == nil {
		t.Fatal("expected context timeout / cancellation error")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context error, got: %v", err)
	}
}

func TestSSRFProtection_Matrix(t *testing.T) {
	testCases := []struct {
		url       string
		shouldErr bool
	}{
		{"http://127.0.0.1:8080/api", true},
		{"http://localhost:8080/api", true},
		{"http://10.0.0.1/api", true},
		{"http://192.168.1.1/api", true},
		{"http://172.16.0.1/api", true},
		{"http://169.254.169.254/latest/meta-data", true},
		{"ftp://example.com/api", true},
		{"file:///etc/passwd", true},
		{"https://api.github.com/v3", false},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			err := ValidateDestinationURL(tc.url, false)
			if tc.shouldErr && err == nil {
				t.Fatalf("expected URL %q to be blocked by SSRF", tc.url)
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected URL %q to be allowed, got error: %v", tc.url, err)
			}
		})
	}
}

func TestManifestValidation_EdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		manifest *Manifest
		wantErr  string
	}{
		{
			name: "missing apiVersion",
			manifest: &Manifest{
				Kind:     "Connector",
				Metadata: Metadata{Name: "test"},
				Spec:     Spec{Type: "rest", BaseURL: "https://api.test.com"},
			},
			wantErr: "missing apiVersion",
		},
		{
			name: "wrong kind",
			manifest: &Manifest{
				APIVersion: "memro.dev/v1",
				Kind:       "ConfigMap",
				Metadata:   Metadata{Name: "test"},
				Spec:       Spec{Type: "rest", BaseURL: "https://api.test.com"},
			},
			wantErr: "manifest kind must be 'Connector'",
		},
		{
			name: "missing id mapping",
			manifest: &Manifest{
				APIVersion: "memro.dev/v1",
				Kind:       "Connector",
				Metadata:   Metadata{Name: "test"},
				Spec: Spec{
					Type:       "rest",
					BaseURL:    "https://api.test.com",
					Auth:       AuthSpec{Type: "none"},
					Operations: []OperationSpec{{Name: "list", Method: "GET", Path: "/", Response: ResponseSpec{ItemsPath: "data"}}},
					Mapping:    MappingSpec{Content: "$.desc"},
				},
			},
			wantErr: "spec.mapping.id is required",
		},
		{
			name: "unsupported HTTP method",
			manifest: &Manifest{
				APIVersion: "memro.dev/v1",
				Kind:       "Connector",
				Metadata:   Metadata{Name: "test"},
				Spec: Spec{
					Type:       "rest",
					BaseURL:    "https://api.test.com",
					Auth:       AuthSpec{Type: "none"},
					Operations: []OperationSpec{{Name: "list", Method: "DELETE", Path: "/", Response: ResponseSpec{ItemsPath: "data"}}},
					Mapping:    MappingSpec{ID: "$.id", Content: "$.desc"},
				},
			},
			wantErr: "unsupported method \"DELETE\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.manifest.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}
