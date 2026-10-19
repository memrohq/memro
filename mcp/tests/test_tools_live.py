"""
Calls every MCP tool through the Streamable HTTP transport against a running
Memro API. Skipped unless MEMRO_TEST_BASE_URL is set.
"""
import asyncio
import json
import os
import socket
import subprocess
import urllib.request
import sys
import time

import pytest

BASE = os.getenv("MEMRO_TEST_BASE_URL")
pytestmark = pytest.mark.skipif(not BASE, reason="MEMRO_TEST_BASE_URL not set")


def test_every_tool_works_end_to_end(monkeypatch):
    from mcp import ClientSession
    from mcp.client.streamable_http import streamablehttp_client
    from memro import Agent, MemroClient

    owner = Agent.create(client=MemroClient(BASE))
    key = owner.client._json("POST", "/api-keys", json_body={"name": "mcp test"},
                             agent_id=owner.agent_id, private_key=owner._private_key)["key"]
    # A separate process: the MCP session manager starts once per process.
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    env = {**os.environ, "MEMRO_BASE_URL": BASE, "MCP_TRANSPORT": "sse"}
    proc = subprocess.Popen([sys.executable, "-m", "uvicorn", "memro_mcp.server:starlette_app", "--port", str(port),
                             "--log-level", "warning"], env=env)
    for _ in range(100):
        try:
            urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz", timeout=1)
            break
        except OSError:
            time.sleep(0.1)

    async def run():
        async with streamablehttp_client(f"http://127.0.0.1:{port}/mcp", headers={"Authorization": f"Bearer {key}"}) as (r, w, _):
            async with ClientSession(r, w) as s:
                await s.initialize()
                tools = {t.name for t in (await s.list_tools()).tools}
                out = {}

                async def call(name, **args):
                    res = await s.call_tool(name, args)
                    assert not res.isError, f"{name}: {res.content[0].text}"
                    out[name] = res.content[0].text
                    return out[name]

                text = await call("remember", content="The billing service is written in Go", type="semantic")
                mid = text.split("ID: ")[1].rstrip(")")
                await call("remember_with_context", content="Shipped billing v2", event_date="2026-10-01T00:00:00Z")
                assert "billing" in await call("recall", query="billing service")
                await call("recall_multi_session", query="billing")
                assert "billing" in await call("get_recent_memories", limit=5)
                await call("search_temporal", query="billing")
                await call("get_temporal_stats")
                await call("get_memory_chain", memory_id=mid)
                await call("find_contradictions")
                await call("generate_atomic_facts", text="Ana leads platform. She joined in 2024.")
                await call("mee_reason", query="what is the billing service written in?")
                await call("mee_evolve")
                await call("get_timeline", memory_id=mid)
                await call("pre_action_check", resource="billing", intent="refactor invoices")
                await call("claim_task", resource="billing", intent="refactor invoices")
                await call("log_intent", resource="docs", intent="write the quickstart")
                await call("get_active_work")
                await call("get_active_context", resource="billing")
                await call("release_task", resource="billing")
                await call("claim_task", resource="billing", intent="second pass")  # released claims don't block
                await call("release_task", resource="billing")
                await call("handoff_task", resource="docs", summary="quickstart drafted")
                await call("export_memories")
                await call("get_system_status")
                await call("delete_memory", memory_id=mid)
                return tools, out

    try:
        tools, out = asyncio.run(run())
    finally:
        proc.terminate()
        owner.delete_agent()
    assert len(tools) == len(set(tools))
    assert json.loads(out["get_system_status"])["agent_id"] == owner.agent_id
