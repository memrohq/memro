"""The remember tools pass the caller's memory type through, and leave it out
when the caller does, so the server classifies the memory."""
import os

os.environ["MCP_TRANSPORT"] = "sse"

from types import SimpleNamespace

import memro_mcp.server as srv


class RecordingAgent:
    agent_id = "agent-a"

    def __init__(self):
        self.types = []

    def remember(self, content, type=None, **kw):
        self.types.append(type)
        return SimpleNamespace(id="m1", content=content, memory_type=type)


def _tool(name):
    return next(t for t in srv._all_tools() if t.name == name)


def test_remember_tools_have_no_default_type():
    for name in ("remember", "remember_with_context"):
        schema = _tool(name).inputSchema["properties"]["type"]
        assert "default" not in schema, name
        assert set(schema["enum"]) == {"semantic", "profile", "procedural", "episodic"}


def test_omitted_or_blank_type_reaches_the_server_as_none():
    agent = RecordingAgent()
    srv._run_tool(agent, "s", "remember", {"content": "To deploy, run make release."})
    srv._run_tool(agent, "s", "remember", {"content": "x", "type": "  "})
    srv._run_tool(agent, "s", "remember_with_context", {"content": "x", "context_metadata": {"file": "a.go"}})
    assert agent.types == [None, None, None]


def test_explicit_types_are_kept():
    agent = RecordingAgent()
    for t in ("semantic", "profile", "procedural", "episodic"):
        srv._run_tool(agent, "s", "remember", {"content": "x", "type": t})
    srv._run_tool(agent, "s", "remember", {"content": "x", "type": " Profile "})
    assert agent.types == ["semantic", "profile", "procedural", "episodic", "profile"]


def test_instructions_explain_the_types_and_what_to_skip():
    text = srv.INSTRUCTIONS
    for word in ('"semantic"', '"profile"', '"procedural"', '"episodic"', "leave the type out",
                 "routine events", "summaries"):
        assert word in text, word
