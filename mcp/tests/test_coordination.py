"""Claims follow the newest action on each resource, and coordination never
trips over memories without metadata."""
import os

os.environ["MCP_TRANSPORT"] = "sse"

from types import SimpleNamespace

import memro_mcp.server as srv


class FakeAgent:
    """Stores memories in memory, newest first like the API."""

    agent_id = "agent-a"

    def __init__(self):
        self.memories = []
        self.calls = []

    def remember(self, content, type="episodic", metadata=None, **kw):
        m = SimpleNamespace(id=str(len(self.memories)), content=content, memory_type=type, metadata=metadata)
        self.memories.insert(0, m)
        return m

    def get_recent(self, limit=10, type=None):
        self.calls.append(limit)
        return [m for m in self.memories if type is None or m.memory_type == type][:limit]


def call(agent, name, **args):
    return srv._run_tool(agent, "default", name, args)


def test_release_frees_the_resource_for_a_new_claim():
    agent = FakeAgent()
    assert not call(agent, "claim_task", resource="billing", intent="refactor").isError
    assert call(agent, "claim_task", resource="billing", intent="again").isError
    call(agent, "release_task", resource="billing")
    assert srv.CoordinationEngine(agent, "default").can_act("billing", "x")["decision"] == srv.Decision.ALLOW
    assert not call(agent, "claim_task", resource="billing", intent="second pass").isError


def test_handoff_releases_the_claim():
    agent = FakeAgent()
    call(agent, "claim_task", resource="docs", intent="write")
    call(agent, "handoff_task", resource="docs", summary="drafted")
    claims, _ = srv.CoordinationEngine(agent, "default")._get_active_claims("docs")
    assert claims == []


def test_release_clears_older_intents():
    agent = FakeAgent()
    call(agent, "log_intent", resource="api", intent="rewrite the router")
    call(agent, "release_task", resource="api")
    assert srv.CoordinationEngine(agent, "default")._get_active_claims() == ([], [])


def test_memories_without_metadata_are_ignored():
    agent = FakeAgent()
    agent.remember("a plain procedural note", type="procedural", metadata=None)
    assert not call(agent, "claim_task", resource="billing", intent="refactor").isError
    assert call(agent, "pre_action_check", resource="billing", intent="refactor").content[0].text


def test_other_sessions_do_not_block():
    agent = FakeAgent()
    srv._run_tool(agent, "other", "claim_task", {"resource": "billing", "intent": "refactor"})
    assert not call(agent, "claim_task", resource="billing", intent="refactor").isError


def test_limits_are_clamped():
    agent = FakeAgent()
    call(agent, "get_recent_memories", limit=10_000)
    call(agent, "get_recent_memories", limit="nonsense")
    assert agent.calls == [srv.MAX_LIMIT, 10]
