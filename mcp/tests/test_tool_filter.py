"""MEMRO_MCP_TOOLS lists only the named tools; unset, every tool is listed."""
import anyio

import memro_mcp.server as srv


def names(monkeypatch, value):
    if value is None:
        monkeypatch.delenv("MEMRO_MCP_TOOLS", raising=False)
    else:
        monkeypatch.setenv("MEMRO_MCP_TOOLS", value)
    return [t.name for t in anyio.run(srv.list_tools)]


def test_unset_lists_every_tool(monkeypatch):
    all_tools = names(monkeypatch, None)
    assert {"recall", "remember", "claim_task", "mee_evolve"} <= set(all_tools)
    assert len(all_tools) == len(set(all_tools))


def test_listed_names_only(monkeypatch):
    assert sorted(names(monkeypatch, " recall, remember ,update_memory,")) == ["recall", "remember", "update_memory"]
