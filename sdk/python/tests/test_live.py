"""
End-to-end tests against a running Memro server. Skipped unless
MEMRO_TEST_BASE_URL is set, e.g. MEMRO_TEST_BASE_URL=http://localhost:8081.
"""
import os
import time

import pytest

from memro import Agent, MemroClient, MemroError

BASE = os.getenv("MEMRO_TEST_BASE_URL")
pytestmark = pytest.mark.skipif(not BASE, reason="MEMRO_TEST_BASE_URL not set")


@pytest.fixture
def agent():
    a = Agent.create(client=MemroClient(BASE, api_key=None))
    yield a
    try:
        a.delete_agent()
    except MemroError:
        pass


def test_signed_agent_end_to_end(agent):
    m = agent.remember("The billing service is written in Go", type="semantic")
    assert m.content.startswith("The billing")
    agent.remember("Ana prefers short answers", type="profile")

    assert any(r.memory_id == m.id for r in agent.recall("billing service", mode="keyword"))
    assert any(r.memory_id == m.id for r in agent.recall_temporal("billing service"))
    assert {x.id for x in agent.get_recent(limit=10)} >= {m.id}
    assert agent.stats().total_memories == 2
    assert agent.export().total == 2  # before uploads add more
    assert agent.reason("what is the billing service written in").answer
    assert agent.timeline(m.id).memory_id == m.id
    assert agent.memory_chain(m.id).root_memory.memory_id == m.id
    assert agent.evolve().memories_evolved >= 0
    assert agent.usage()["memory_nodes"] == 2

    up = agent.upload_file(("notes.txt", b"Quarterly planning starts in March", "text/plain"))
    assert "Quarterly planning" in (up.extracted_text or "") and up.memory_id

    long_text = ("Retries happen three times before paging on-call. " * 60).encode()
    chunks = agent.upload_document(("runbook.md", long_text, "text/markdown"))
    assert len(chunks) >= 2 and all(len(c.content) <= 1500 and c.file_id == chunks[0].file_id for c in chunks)
    assert chunks[0].metadata["chunks"] == len(chunks)

    agent.add_webhook("https://hooks.example.com/memro")
    assert agent.webhooks() == ["https://hooks.example.com/memro"]
    assert agent.remove_webhook("https://hooks.example.com/memro") is True
    assert agent.webhooks() == []

    assert agent.delete_memory(m.id) is True
    assert agent.delete_memory(m.id) is False


def test_other_agents_cannot_read_my_memories(agent):
    agent.remember("secret orchid canary", type="semantic")
    other = Agent.create(client=MemroClient(BASE))
    try:
        assert not any("orchid" in r.content for r in other.recall("orchid canary", mode="keyword"))
        with pytest.raises(MemroError) as e:
            other.client.get_memories(agent.agent_id, private_key=other._private_key)
        assert e.value.status_code in (401, 403)
    finally:
        other.delete_agent()


def test_api_key_agent(agent):
    # An agent that signs its requests may mint an API key for its tools.
    key = agent.client._json("POST", "/api-keys", json_body={"name": "sdk test"},
                             agent_id=agent.agent_id, private_key=agent._private_key)["key"]
    keyed = Agent.from_api_key(key, client=MemroClient(BASE, api_key=key))
    assert keyed.agent_id == agent.agent_id
    m = keyed.remember("Deploys happen on Tuesdays", type=None)
    assert m.memory_type in ("episodic", "semantic", "procedural", "profile")
    assert any(r.memory_id == m.id for r in keyed.recall("deploys tuesdays", mode="keyword"))
