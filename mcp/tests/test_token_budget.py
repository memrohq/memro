"""recall with token_budget packs the best memories that fit, minus superseded and duplicate ones."""
import json
import os
from types import SimpleNamespace

os.environ["MCP_TRANSPORT"] = "sse"

import memro_mcp.server as srv


def hit(i, content, score, superseded=False):
    return {"memory_id": f"m{i}", "content": content, "memory_type": "semantic", "score": score,
            "created_at": "2026-11-16T00:00:00Z", "is_superseded": superseded,
            "superseded_by": "m0" if superseded else None}


class StubClient:
    def __init__(self, results):
        self.results, self.kw = results, None

    def search(self, **kw):
        self.kw = kw
        return self.results


def recall(results, **args):
    client = StubClient(results)
    agent = SimpleNamespace(client=client, agent_id="a1", _private_key=None)
    out = srv._run_tool(agent, "s", "recall", {"query": "billing", **args})
    return json.loads(out.content[0].text), client.kw


def test_budget_drops_superseded_and_duplicates_and_fits():
    results = [
        hit(1, "Billing lives in memro-cloud.", 0.9),
        hit(2, "billing lives in  memro-cloud.", 0.8),          # duplicate after normalising
        hit(3, "Billing lived in the public repo.", 0.7, True),  # superseded
        hit(4, "x" * 2000, 0.6),                                 # too big for the budget
        hit(5, "Dodo handles payments.", 0.5),                   # smaller, still fits
    ]
    items, kw = recall(results, token_budget=100)
    assert [i["id"] for i in items] == ["m1", "m5"]
    assert kw["limit"] == srv.MAX_LIMIT
    assert sum(srv._tokens(i) for i in items) <= 100


def test_no_budget_keeps_old_behaviour():
    results = [hit(1, "a", 0.9), hit(2, "a", 0.8, True)]
    items, kw = recall(results)
    assert len(items) == 2 and items[1]["superseded_by"] == "m0"
    assert kw["limit"] == 5


def test_explicit_limit_wins_and_bad_budget_is_ignored():
    _, kw = recall([], token_budget=500, limit=10)
    assert kw["limit"] == 10
    _, kw = recall([], token_budget="lots")
    assert kw["limit"] == 5
