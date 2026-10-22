package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/api"
	"github.com/memrohq/memro/server/internal/connectors"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/store"
)

// TestCrossAgentIsolation_Search tests that search queries (hybrid, semantic,
// keyword, and temporal) never return memories belonging to another agent, even
// when both agents share the default tenant (uuid.Nil).
func TestCrossAgentIsolation_Search(t *testing.T) {
	e := setup(t)

	agentA := newAgent(t)
	agentB := newAgent(t)
	e.register(agentA)
	e.register(agentB)

	// Agent A stores a distinct secret memory
	memIDA := e.remember(agentA, "Agent A confidential quantum encryption key for vault access", "semantic")
	e.waitEmbedded(agentA, 1)

	// 1. Agent A searches for their secret: finds it
	code, b := e.do(&agentA, "v2", "GET", "/search?query=quantum+encryption+key&mode=hybrid", nil, nil)
	e.must(http.StatusOK, code, b, "agent A hybrid search")
	var respA domain.SearchResponse
	if err := json.Unmarshal(b, &respA); err != nil {
		t.Fatal(err)
	}
	if len(respA.Results) == 0 || respA.Results[0].MemoryID != memIDA {
		t.Fatalf("agent A search failed to find their memory: %+v", respA)
	}

	// 2. Before Agent B stores any memories, Agent B searches: must return 0 results
	code, b = e.do(&agentB, "v2", "GET", "/search?query=quantum+encryption+key&mode=hybrid", nil, nil)
	e.must(http.StatusOK, code, b, "agent B empty hybrid search")
	var respBEmpty domain.SearchResponse
	if err := json.Unmarshal(b, &respBEmpty); err != nil {
		t.Fatal(err)
	}
	if len(respBEmpty.Results) != 0 {
		t.Fatalf("cross-agent isolation leak: Agent B with no memories got results: %+v", respBEmpty.Results)
	}

	// 3. Agent B stores their own memory
	memIDB := e.remember(agentB, "Agent B stellar parallax telescope observation report", "semantic")
	e.waitEmbedded(agentB, 1)

	// 4. Agent B searches for Agent A's exact phrase in hybrid mode: must NEVER return Agent A's memory
	code, b = e.do(&agentB, "v2", "GET", "/search?query=quantum+encryption+key&mode=hybrid", nil, nil)
	e.must(http.StatusOK, code, b, "agent B hybrid search")
	var respB domain.SearchResponse
	if err := json.Unmarshal(b, &respB); err != nil {
		t.Fatal(err)
	}
	for _, res := range respB.Results {
		if res.MemoryID == memIDA {
			t.Fatalf("cross-agent isolation leak: Agent B found Agent A memory %s", memIDA)
		}
	}

	// 3. Agent B searches in semantic-only mode
	code, b = e.do(&agentB, "v2", "GET", "/search?query=quantum+encryption+key&mode=semantic", nil, nil)
	e.must(http.StatusOK, code, b, "agent B semantic search")
	var respBSem domain.SearchResponse
	_ = json.Unmarshal(b, &respBSem)
	for _, res := range respBSem.Results {
		if res.MemoryID == memIDA {
			t.Fatalf("cross-agent isolation leak in semantic mode: Agent B found Agent A memory %s", memIDA)
		}
	}

	// 4. Agent B searches in keyword-only mode
	code, b = e.do(&agentB, "v2", "GET", "/search?query=quantum+encryption+key&mode=keyword", nil, nil)
	e.must(http.StatusOK, code, b, "agent B keyword search")
	var respBKw domain.SearchResponse
	_ = json.Unmarshal(b, &respBKw)
	for _, res := range respBKw.Results {
		if res.MemoryID == memIDA {
			t.Fatalf("cross-agent isolation leak in keyword mode: Agent B found Agent A memory %s", memIDA)
		}
	}

	// 5. Agent B searches with temporal search
	code, b = e.do(&agentB, "v2", "GET", "/search/temporal?query=quantum+encryption+key", nil, nil)
	e.must(http.StatusOK, code, b, "agent B temporal search")
	var tempResults []map[string]any
	_ = json.Unmarshal(b, &tempResults)
	for _, res := range tempResults {
		if res["memory_id"] == memIDA {
			t.Fatalf("cross-agent isolation leak in temporal search: Agent B found Agent A memory %s", memIDA)
		}
	}

	// 6. Agent B attempts to query specifying agent_id=agentA
	code, _ = e.do(&agentB, "v2", "GET", "/search?query=quantum&agent_id="+agentA.id, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when Agent B supplies Agent A's agent_id, got %d", code)
	}

	_ = memIDB
}

// TestCrossAgentIsolation_Get tests that an agent cannot read another agent's
// memories, memory lists, chains, timelines, exports, or files.
func TestCrossAgentIsolation_Get(t *testing.T) {
	e := setup(t)

	agentA := newAgent(t)
	agentB := newAgent(t)
	e.register(agentA)
	e.register(agentB)

	memIDA := e.remember(agentA, "Agent A confidential briefing notes", "semantic")

	// 1. Agent B attempts to list Agent A's memories
	code, _ := e.do(&agentB, "v2", "GET", "/memory/agent/"+agentA.id, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-agent memory listing, got %d", code)
	}

	// 2. Agent B attempts to export Agent A's data
	code, _ = e.do(&agentB, "v2", "GET", "/export/"+agentA.id, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-agent export, got %d", code)
	}

	// 3. Agent B attempts to fetch memory chain for Agent A's memory
	code, _ = e.do(&agentB, "v2", "GET", "/memory/chain/"+memIDA, nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-agent memory chain, got %d", code)
	}

	// 4. Agent B attempts to fetch timeline for Agent A's memory
	code, _ = e.do(&agentB, "v2", "GET", "/mee/timeline/"+memIDA, nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-agent timeline, got %d", code)
	}

	// 5. File scoping: Agent A uploads a file, Agent B tries to create memory from it
	ctx := context.Background()
	fileID := uuid.New()
	filePath := t.TempDir() + "/agentA_doc.txt"
	txt := "Super Secret Document Content"
	_ = e.st.InsertFile(ctx, &store.File{
		ID:            fileID,
		AgentID:       agentA.id,
		TenantID:      domain.DefaultTenantID,
		FileName:      "secret.txt",
		FileType:      "text/plain",
		FilePath:      filePath,
		FileSize:      100,
		MimeType:      "text/plain",
		ExtractedText: &txt,
	})

	code, _ = e.do(&agentB, "v2", "POST", "/memory/from-file", j(map[string]any{
		"file_id":     fileID.String(),
		"memory_type": "semantic",
	}), nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when creating memory from another agent's file, got %d", code)
	}
}

// TestCrossAgentIsolation_Delete tests that an agent cannot delete another
// agent's memories or identities.
func TestCrossAgentIsolation_Delete(t *testing.T) {
	e := setup(t)

	agentA := newAgent(t)
	agentB := newAgent(t)
	e.register(agentA)
	e.register(agentB)

	memIDA := e.remember(agentA, "Agent A permanent architectural record", "semantic")

	// 1. Agent B attempts to delete Agent A's memory
	code, _ := e.do(&agentB, "v2", "DELETE", "/memory/"+memIDA, nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when Agent B deletes Agent A memory, got %d", code)
	}

	// Verify Agent A's memory is still intact
	mem, err := e.st.GetMemory(context.Background(), uuid.MustParse(memIDA), agentA.id, domain.DefaultTenantID)
	if err != nil || mem == nil {
		t.Fatalf("Agent A memory was unexpectedly deleted or corrupted: %v", err)
	}

	// 2. Agent B attempts to delete Agent A's identity
	code, _ = e.do(&agentB, "v2", "DELETE", "/identity/"+agentA.id, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when Agent B deletes Agent A identity, got %d", code)
	}

	// Verify Agent A's identity is still registered
	ident, err := e.st.GetIdentity(context.Background(), agentA.id)
	if err != nil || ident == nil {
		t.Fatalf("Agent A identity was unexpectedly removed: %v", err)
	}
}

// TestCrossAgentIsolation_Connectors tests that connector management endpoints
// (sources, update sources, sync, disconnect) enforce ownership and never
// allow cross-agent access.
func TestCrossAgentIsolation_Connectors(t *testing.T) {
	var mgr *connectors.Manager
	e := setup(t, func(d *api.Deps) {
		d.Config.AppURL = "http://dash.test"
		d.Config.Connectors.EncryptionKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
		d.Config.Connectors.CallbackBaseURL = "http://dash.test/api"
		cd := connectors.Deps{Config: d.Config, Pool: d.Store.Pool, Store: d.Store, Indexer: d.Indexer,
			Invalidate: d.Engine.Cache().InvalidateAgent}
		m, err := connectors.New(cd)
		if err != nil {
			t.Fatal(err)
		}
		mgr = m
		d.Connectors = m
	})

	agentA, sessionA := e.dashboardSession(fmt.Sprintf("agentA-%d@example.com", time.Now().UnixNano()))
	agentB, sessionB := e.dashboardSession(fmt.Sprintf("agentB-%d@example.com", time.Now().UnixNano()))
	_ = agentB

	ctx := context.Background()

	// Create a connector connection owned by Agent A
	connIDA := uuid.New()
	_, err := e.st.Pool.Exec(ctx, `
		INSERT INTO connector_connections (id, tenant_id, agent_id, provider, account_id, account_name, credentials, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'slack', 'T_AGENT_A', 'Workspace A', 'sealed_dummy', 'active', NOW(), NOW())`,
		connIDA, domain.DefaultTenantID, agentA.id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = e.st.Pool.Exec(context.Background(), `DELETE FROM connector_connections WHERE id = $1`, connIDA)
	})

	doDashboard := func(session, method, path string, body any) (int, []byte) {
		return e.do(nil, "", method, path, j(body), map[string]string{"Authorization": "Bearer " + session})
	}

	// 1. Agent A lists connectors: sees connection A
	code, b := doDashboard(sessionA, "GET", "/connectors", nil)
	e.must(http.StatusOK, code, b, "agent A list connectors")
	var listA struct {
		Connections []map[string]any `json:"connections"`
	}
	_ = json.Unmarshal(b, &listA)
	if len(listA.Connections) != 1 || listA.Connections[0]["id"] != connIDA.String() {
		t.Fatalf("agent A expected connection %s, got %+v", connIDA, listA.Connections)
	}

	// 2. Agent B lists connectors: connection list must NOT contain Connection A
	code, b = doDashboard(sessionB, "GET", "/connectors", nil)
	e.must(http.StatusOK, code, b, "agent B list connectors")
	var listB struct {
		Connections []map[string]any `json:"connections"`
	}
	_ = json.Unmarshal(b, &listB)
	for _, c := range listB.Connections {
		if c["id"] == connIDA.String() {
			t.Fatalf("cross-agent isolation leak: Agent B sees Agent A's connection %s", connIDA)
		}
	}
	if len(listB.Connections) != 0 {
		t.Fatalf("expected 0 connections for agent B, got %d", len(listB.Connections))
	}

	// 3. Agent B attempts to get sources of Connection A
	code, _ = doDashboard(sessionB, "GET", fmt.Sprintf("/connectors/%s/sources", connIDA), nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-agent get sources, got %d", code)
	}

	// 4. Agent B attempts to update sources of Connection A
	code, _ = doDashboard(sessionB, "PUT", fmt.Sprintf("/connectors/%s/sources", connIDA), map[string]any{
		"enabled": map[string]bool{"C123": true},
	})
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-agent update sources, got %d", code)
	}

	// 5. Agent B attempts to trigger sync on Connection A
	code, _ = doDashboard(sessionB, "POST", fmt.Sprintf("/connectors/%s/sync", connIDA), nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-agent sync, got %d", code)
	}

	// 6. Agent B attempts to disconnect Connection A
	code, _ = doDashboard(sessionB, "DELETE", fmt.Sprintf("/connectors/%s", connIDA), nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-agent disconnect, got %d", code)
	}

	// Verify Connection A still exists and is untouched
	var count int
	_ = e.st.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_connections WHERE id = $1`, connIDA).Scan(&count)
	if count != 1 {
		t.Fatalf("Agent A connection was unexpectedly deleted by Agent B")
	}

	_ = mgr
}
