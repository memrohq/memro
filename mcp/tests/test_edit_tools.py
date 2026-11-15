"""update_memory and delete_memory send the right requests (ADR 0032)."""
import os
from types import SimpleNamespace

os.environ["MCP_TRANSPORT"] = "sse"

import memro_mcp.server as srv
from memro import MemroError


class StubClient:
    def __init__(self, status=200):
        self.calls, self.status = [], status

    def request(self, method, path, **kw):
        self.calls.append((method, path, kw))
        if self.status != 200:
            raise MemroError(self.status, "nope")

    def _json(self, method, path, **kw):
        self.request(method, path, **kw)
        return {"memory": {"id": "new-1"}, "replaced": "old/1"}


def agent(client):
    return SimpleNamespace(client=client, agent_id="a1", _private_key=None)


def text(result):
    return result.content[0].text


def test_update_memory():
    c = StubClient()
    out = srv._run_tool(agent(c), "s", "update_memory",
                        {"memory_id": "old/1", "content": "The billing contact is Ana.", "reason": "Marco left"})
    method, path, kw = c.calls[0]
    assert (method, path) == ("PUT", "/memory/old%2F1")
    assert kw["json_body"] == {"content": "The billing contact is Ana.", "reason": "Marco left"}
    assert "new-1" in text(out)


def test_delete_memory_archives_with_reason():
    c = StubClient()
    out = srv._run_tool(agent(c), "s", "delete_memory", {"memory_id": "m1", "reason": "user asked"})
    assert c.calls[0][:2] == ("DELETE", "/memory/m1") and c.calls[0][2]["params"] == {"reason": "user asked"}
    assert "archived" in text(out)


def test_missing_memory():
    for tool, args in [("update_memory", {"memory_id": "x", "content": "c", "reason": "r"}), ("delete_memory", {"memory_id": "x"})]:
        assert text(srv._run_tool(agent(StubClient(404)), "s", tool, args)) == "Memory not found."


def test_recall_ranks_and_marks_superseded():
    class SearchClient:
        def search(self, **kw):
            self.kw = kw
            return [{"memory_id": "new", "content": "Ana lives in Berlin.", "memory_type": "semantic", "score": 0.9,
                     "created_at": "2026-11-15T00:00:00Z"},
                    {"memory_id": "old", "content": "Ana lives in Paris.", "memory_type": "semantic", "score": 0.2,
                     "created_at": "2026-10-01T00:00:00Z", "is_superseded": True, "superseded_by": "new"}]
    c = SearchClient()
    out = text(srv._run_tool(agent(c), "s", "recall", {"query": "where does Ana live"}))
    assert c.kw["resolve_conflicts"] is True
    assert '"superseded_by": "new"' in out and out.count("superseded_by") == 1
