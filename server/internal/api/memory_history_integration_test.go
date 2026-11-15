package api_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
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

// "What did the agent know at time T?": live memories created by T plus those
// archived after T.
func TestListMemoriesAsOf(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	before := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	pune := e.remember(a, "The office is in Pune.", "semantic")
	time.Sleep(1100 * time.Millisecond)
	known := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(1100 * time.Millisecond)
	code, b := e.do(&a, "v2", "DELETE", "/memory/"+pune+"?reason=moved", nil, nil)
	e.must(204, code, b, "archive")
	delhi := e.remember(a, "The office is in Delhi.", "semantic")

	list := func(asOf string) string {
		path := "/memory/agent/" + a.id
		if asOf != "" {
			path += "?as_of=" + url.QueryEscape(asOf)
		}
		code, b := e.do(&a, "v2", "GET", path, nil, nil)
		e.must(200, code, b, "list as of "+asOf)
		return string(b)
	}
	if got := list(known); !strings.Contains(got, pune) || strings.Contains(got, delhi) {
		t.Fatalf("as of %s: want Pune only, got %s", known, got)
	}
	if got := list(""); strings.Contains(got, pune) || !strings.Contains(got, delhi) {
		t.Fatalf("now: want Delhi only, got %s", got)
	}
	if got := list(before); strings.Contains(got, pune) || strings.Contains(got, delhi) {
		t.Fatalf("before any write: want nothing, got %s", got)
	}
	code, b = e.do(&a, "v2", "GET", "/memory/agent/"+a.id+"?as_of=yesterday", nil, nil)
	e.must(400, code, b, "bad as_of")
}

// ADR 0032: an agent corrects a memory; the old version is archived and both
// histories say why.
func TestUpdateMemoryReplacesAndRecords(t *testing.T) {
	e := setup(t)
	a, other := newAgent(t), newAgent(t)
	e.register(a)
	e.register(other)
	id := e.remember(a, "The billing contact is Marco.", "semantic")

	upd := j(map[string]any{"content": "The billing contact is Ana.", "reason": "Marco left in October"})
	code, b := e.do(&other, "v2", "PUT", "/memory/"+id, upd, nil)
	e.must(404, code, b, "another agent updates")
	code, b = e.do(&a, "v2", "PUT", "/memory/"+id, j(map[string]any{"content": " "}), nil)
	e.must(400, code, b, "empty content")

	code, b = e.do(&a, "v2", "PUT", "/memory/"+id, upd, nil)
	e.must(200, code, b, "update")
	var got struct {
		Memory struct {
			ID         string `json:"id"`
			Content    string `json:"content"`
			MemoryType string `json:"memory_type"`
		} `json:"memory"`
		Replaced string `json:"replaced"`
	}
	_ = json.Unmarshal(b, &got)
	if got.Replaced != id || got.Memory.Content != "The billing contact is Ana." || got.Memory.MemoryType != "semantic" {
		t.Fatalf("update = %s", b)
	}

	code, b = e.do(&a, "v2", "GET", "/memory/agent/"+a.id, nil, nil)
	if strings.Contains(string(b), "Marco") || !strings.Contains(string(b), got.Memory.ID) {
		t.Fatalf("after update, memories = %s", b)
	}
	code, b = e.do(&a, "v2", "GET", "/memory/history/"+id, nil, nil)
	e.must(200, code, b, "old history")
	if !strings.Contains(string(b), `"action":"memory.update"`) || !strings.Contains(string(b), `"replaced_by":"`+got.Memory.ID) ||
		!strings.Contains(string(b), "Marco left in October") || !strings.Contains(string(b), `"archived":true`) {
		t.Fatalf("old history = %s", b)
	}
	code, b = e.do(&a, "v2", "GET", "/memory/history/"+got.Memory.ID, nil, nil)
	if !strings.Contains(string(b), `"action":"memory.replaces"`) || !strings.Contains(string(b), "The billing contact is Marco.") {
		t.Fatalf("new history = %s", b)
	}
	code, b = e.do(&a, "v2", "PUT", "/memory/"+id, upd, nil)
	e.must(404, code, b, "update an archived memory")

	// Two updates at once: exactly one wins and only one version stays live.
	id2 := e.remember(a, "The office is in Pune.", "semantic")
	codes := make(chan int, 2)
	for _, c := range []string{"The office is in Delhi.", "The office is in Goa."} {
		go func(c string) {
			code, _ := e.do(&a, "v2", "PUT", "/memory/"+id2, j(map[string]any{"content": c, "reason": "moved"}), nil)
			codes <- code
		}(c)
	}
	c1, c2 := <-codes, <-codes
	if c1+c2 != 200+409 && c1+c2 != 200+404 {
		t.Fatalf("concurrent updates = %d, %d", c1, c2)
	}
	_, b = e.do(&a, "v2", "GET", "/memory/agent/"+a.id, nil, nil)
	if strings.Contains(string(b), "Delhi") == strings.Contains(string(b), "Goa") {
		t.Fatalf("want exactly one live version, memories = %s", b)
	}
}
