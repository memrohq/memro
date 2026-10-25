package api_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/auth"
)

func TestBatchMemoryIngest(t *testing.T) {
	e := setup(t)

	t.Run("Success_PreservesAttributesAndInputOrder", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)

		now := time.Now().UTC().Truncate(time.Second)
		evDate1 := now.Add(-2 * time.Hour).Format(time.RFC3339)
		evDate2 := now.Add(-1 * time.Hour).Format(time.RFC3339)
		session1 := uuid.New().String()
		session2 := uuid.New().String()

		batch := []map[string]any{
			{
				"content":     "First memory: user prefers dark mode",
				"memory_type": "profile",
				"event_date":  evDate1,
				"session_id":  session1,
				"metadata":    map[string]any{"source": "test", "priority": 1},
			},
			{
				"content":     "Second memory: user visited Kyoto in spring",
				"memory_type": "episodic",
				"event_date":  evDate2,
				"session_id":  session1,
				"metadata":    map[string]any{"source": "test", "city": "Kyoto"},
			},
			{
				"content":     "Third memory: Go routines communicate via channels",
				"memory_type": "semantic",
				"session_id":  session2,
				"metadata":    map[string]any{"lang": "go"},
			},
		}

		code, b := e.do(&a, "v2", "POST", "/memories/batch", j(batch), nil)
		e.must(200, code, b, "batch ingest")

		var resp struct {
			IDs      []string `json:"ids"`
			Count    int      `json:"count"`
			Memories []struct {
				ID         string          `json:"id"`
				Content    string          `json:"content"`
				MemoryType string          `json:"memory_type"`
				EventDate  *string         `json:"event_date"`
				SessionID  *string         `json:"session_id"`
				Metadata   json.RawMessage `json:"metadata"`
			} `json:"memories"`
		}
		if err := json.Unmarshal(b, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		if resp.Count != 3 || len(resp.IDs) != 3 || len(resp.Memories) != 3 {
			t.Fatalf("expected 3 memories, got count=%d ids=%d memories=%d", resp.Count, len(resp.IDs), len(resp.Memories))
		}

		// Verify order matches input
		for i, id := range resp.IDs {
			if id != resp.Memories[i].ID {
				t.Fatalf("ID order mismatch at index %d: ids[%d]=%s, memories[%d].id=%s", i, i, id, i, resp.Memories[i].ID)
			}
		}

		if resp.Memories[0].Content != "First memory: user prefers dark mode" || resp.Memories[0].MemoryType != "profile" {
			t.Fatalf("item 0 unexpected: %+v", resp.Memories[0])
		}
		if resp.Memories[0].SessionID == nil || *resp.Memories[0].SessionID != session1 {
			t.Fatalf("item 0 session_id mismatch: expected %s, got %v", session1, resp.Memories[0].SessionID)
		}

		// Verify DB count
		count, err := e.st.CountMemories(t.Context(), a.id)
		if err != nil {
			t.Fatalf("CountMemories: %v", err)
		}
		if count != 3 {
			t.Fatalf("expected DB count 3, got %d", count)
		}
	})

	t.Run("PayloadFormats", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)

		// Object with "memories" key
		obj1 := map[string]any{
			"memories": []map[string]any{
				{"content": "Format test 1"},
				{"content": "Format test 2"},
			},
		}
		code, b := e.do(&a, "v2", "POST", "/memories/batch", j(obj1), nil)
		e.must(200, code, b, "object with memories key")

		// Object with "items" key
		obj2 := map[string]any{
			"items": []map[string]any{
				{"content": "Format test 3"},
			},
		}
		code, b = e.do(&a, "v2", "POST", "/memories/batch", j(obj2), nil)
		e.must(200, code, b, "object with items key")
	})

	t.Run("AtomicRollbackOnInvalidItem", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)

		batch := []map[string]any{
			{"content": "Valid item 1"},
			{"content": "Valid item 2"},
			{"content": ""}, // Invalid: empty content
			{"content": "Valid item 3"},
		}

		code, b := e.do(&a, "v2", "POST", "/memories/batch", j(batch), nil)
		e.must(400, code, b, "empty content should fail batch")

		// Ensure nothing was committed to DB
		count, err := e.st.CountMemories(t.Context(), a.id)
		if err != nil {
			t.Fatalf("CountMemories: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 memories in DB due to atomic rollback, got %d", count)
		}
	})

	t.Run("InvalidMemoryTypeAndDateFormat", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)

		// Invalid memory_type
		badTypeBatch := []map[string]any{
			{"content": "Good content", "memory_type": "nonexistent_type"},
		}
		code, b := e.do(&a, "v2", "POST", "/memories/batch", j(badTypeBatch), nil)
		e.must(400, code, b, "invalid memory_type should fail")

		// Invalid event_date
		badDateBatch := []map[string]any{
			{"content": "Good content", "event_date": "not-a-date"},
		}
		code, b = e.do(&a, "v2", "POST", "/memories/batch", j(badDateBatch), nil)
		e.must(400, code, b, "invalid event_date should fail")
	})

	t.Run("SizeLimits", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)

		// Empty batch
		code, b := e.do(&a, "v2", "POST", "/memories/batch", j([]map[string]any{}), nil)
		e.must(400, code, b, "empty batch should fail")

		// Exceed 500 limit
		tooMany := make([]map[string]any, 501)
		for i := range tooMany {
			tooMany[i] = map[string]any{"content": fmt.Sprintf("Item %d", i)}
		}
		code, b = e.do(&a, "v2", "POST", "/memories/batch", j(tooMany), nil)
		e.must(400, code, b, "batch over 500 should fail")
	})

	t.Run("AgentIDMismatch", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)
		otherAgent := newAgent(t)

		batch := []map[string]any{
			{"content": "Valid item"},
			{"content": "Hacker item", "agent_id": otherAgent.id},
		}
		code, b := e.do(&a, "v2", "POST", "/memories/batch", j(batch), nil)
		e.must(403, code, b, "mismatched agent_id should return 403 Forbidden")
	})

	t.Run("APIKeyAuth", func(t *testing.T) {
		a := newAgent(t)
		e.register(a)

		ident, err := e.st.GetIdentity(t.Context(), a.id)
		if err != nil {
			t.Fatalf("GetIdentity: %v", err)
		}

		rawKey, prefix, hash, err := auth.NewAPIKey()
		if err != nil {
			t.Fatalf("NewAPIKey: %v", err)
		}
		err = e.st.CreateAPIKey(t.Context(), uuid.New(), ident.TenantID, a.id, "Test Key", prefix, hash)
		if err != nil {
			t.Fatalf("CreateAPIKey: %v", err)
		}

		batch := []map[string]any{
			{"content": "Memory ingested with API key 1"},
			{"content": "Memory ingested with API key 2"},
		}

		code, b := e.do(nil, "", "POST", "/memories/batch", j(batch), bearer(rawKey))
		e.must(200, code, b, "batch with API key")

		var resp struct {
			Count int `json:"count"`
		}
		_ = json.Unmarshal(b, &resp)
		if resp.Count != 2 {
			t.Fatalf("expected count 2, got %d", resp.Count)
		}
	})
}
