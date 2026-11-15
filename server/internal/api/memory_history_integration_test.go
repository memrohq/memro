package api_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// ADR 0032: deleting archives a memory, it can be restored, purging removes
// it and its audited content, and every step shows in its history.
func TestMemoryArchiveRestorePurgeHistory(t *testing.T) {
	e := setup(t)
	a, other := newAgent(t), newAgent(t)
	e.register(a)
	e.register(other)
	id := e.remember(a, "The billing contact is Ana.", "semantic")

	listed := func() bool {
		code, b := e.do(&a, "v2", "GET", "/memory/agent/"+a.id, nil, nil)
		e.must(200, code, b, "list memories")
		return strings.Contains(string(b), id)
	}
	history := func(who *agent) (int, map[string]any) {
		code, b := e.do(who, "v2", "GET", "/memory/history/"+id, nil, nil)
		var h map[string]any
		_ = json.Unmarshal(b, &h)
		return code, h
	}

	code, b := e.do(&a, "v2", "DELETE", "/memory/"+id+"?reason=wrong+person", nil, nil)
	e.must(204, code, b, "archive")
	if listed() {
		t.Fatal("archived memory still listed")
	}
	code, h := history(&a)
	e.must(200, code, nil, "history of archived memory")
	if h["archived"] != true {
		t.Fatalf("archived = %v", h["archived"])
	}
	steps := h["history"].([]any)
	if len(steps) != 2 {
		t.Fatalf("history = %v", steps)
	}
	created, deleted := steps[0].(map[string]any), steps[1].(map[string]any)
	if created["action"] != "memory.create" || created["source"] != "api" || created["actor_type"] != "agent" ||
		created["after"].(map[string]any)["content"] != "The billing contact is Ana." {
		t.Fatalf("create entry = %v", created)
	}
	if deleted["action"] != "memory.delete" || deleted["reason"] != "wrong person" ||
		deleted["before"].(map[string]any)["content"] != "The billing contact is Ana." {
		t.Fatalf("delete entry = %v", deleted)
	}
	if code, _ := history(&other); code != 404 {
		t.Fatalf("another agent read the history: %d", code)
	}
	code, b = e.do(&other, "v2", "POST", "/memory/restore/"+id, nil, nil)
	e.must(404, code, b, "another agent restores")

	code, b = e.do(&a, "v2", "POST", "/memory/restore/"+id, nil, nil)
	e.must(200, code, b, "restore")
	if !listed() {
		t.Fatal("restored memory not listed")
	}

	code, b = e.do(&a, "v2", "DELETE", "/memory/"+id+"?purge=true", nil, nil)
	e.must(204, code, b, "purge")
	code, b = e.do(&a, "v2", "POST", "/memory/restore/"+id, nil, nil)
	e.must(404, code, b, "restore after purge")
	_, h = history(&a)
	steps = h["history"].([]any)
	if last := steps[len(steps)-1].(map[string]any); last["action"] != "memory.purge" {
		t.Fatalf("last entry = %v", last)
	}
	if raw, _ := json.Marshal(steps); strings.Contains(string(raw), "billing contact") {
		t.Fatalf("purged content still in history: %s", raw)
	}

	// The MCP server and SDKs name themselves; batches are audited per memory.
	code, b = e.do(&a, "v2", "POST", "/memories/batch", j([]map[string]any{{"content": "Deploys happen on Fridays."}}),
		map[string]string{"X-Memro-Source": "mcp"})
	e.must(200, code, b, "batch")
	code, b = e.do(&a, "v2", "GET", "/memory/agent/"+a.id, nil, nil)
	if !strings.Contains(string(b), `"source":"mcp"`) {
		t.Fatalf("batch memory source not recorded: %s", b)
	}
}
