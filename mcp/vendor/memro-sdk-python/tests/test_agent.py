"""
Unit tests for memro Python SDK.
Run: pytest tests/
"""
import json
import pytest
from unittest.mock import patch, MagicMock
from datetime import datetime, timezone

from memro import Agent, MemroClient, Memory, SearchResult
from memro.crypto import generate_keypair, sign_body


# ── Crypto tests ─────────────────────────────────────────────────────────────

def test_generate_keypair():
    pub, priv = generate_keypair()
    assert len(pub) == 64   # 32 bytes hex
    assert len(priv) == 64  # 32 bytes hex
    assert pub != priv

def test_sign_body_returns_headers():
    _, priv = generate_keypair()
    headers = sign_body(priv, b"hello world")
    assert "X-Signature" in headers
    assert "X-Timestamp" in headers
    assert len(headers["X-Signature"]) == 128  # 64 bytes hex

def test_two_signs_same_body_same_signature():
    _, priv = generate_keypair()
    body = b"deterministic"
    h1 = sign_body(priv, body)
    h2 = sign_body(priv, body)
    assert h1["X-Signature"] == h2["X-Signature"]


# ── Agent constructor tests ───────────────────────────────────────────────────

def test_agent_from_dict():
    pub, priv = generate_keypair()
    agent = Agent.from_dict({"agent_id": pub, "private_key": priv})
    assert agent.agent_id == pub

def test_agent_save_and_load(tmp_path):
    pub, priv = generate_keypair()
    agent = Agent.from_dict({"agent_id": pub, "private_key": priv})
    path = str(tmp_path / "identity.json")
    agent.save(path)

    loaded = Agent.from_file(path)
    assert loaded.agent_id == pub

def test_agent_from_env(monkeypatch):
    pub, priv = generate_keypair()
    monkeypatch.setenv("MEMRO_AGENT_ID", pub)
    monkeypatch.setenv("MEMRO_PRIVATE_KEY", priv)
    agent = Agent.from_env()
    assert agent.agent_id == pub

def test_agent_from_env_missing(monkeypatch):
    monkeypatch.delenv("MEMRO_AGENT_ID", raising=False)
    monkeypatch.delenv("MEMRO_PRIVATE_KEY", raising=False)
    with pytest.raises(EnvironmentError):
        Agent.from_env()


# ── Agent method tests (mocked HTTP) ─────────────────────────────────────────

FAKE_MEMORY = {
    "id": "11111111-1111-1111-1111-111111111111",
    "agent_id": "abc123",
    "content": "User likes Python",
    "memory_type": "profile",
    "visibility": "private",
    "created_at": "2026-02-24T00:00:00Z",
}

@pytest.fixture
def mock_agent():
    pub, priv = generate_keypair()
    client = MagicMock(spec=MemroClient)
    client.create_memory.return_value = {**FAKE_MEMORY, "agent_id": pub}
    client.get_memories.return_value = [{**FAKE_MEMORY, "agent_id": pub}]
    client.search.return_value = [{
        "memory_id": FAKE_MEMORY["id"],
        "content": FAKE_MEMORY["content"],
        "score": 0.95,
        "memory_type": "profile",
        "created_at": FAKE_MEMORY["created_at"],
    }]
    client.export.return_value = {
        "agent_id": pub,
        "exported_at": "2026-02-24T00:00:00Z",
        "total": 1,
        "memories": [{**FAKE_MEMORY, "agent_id": pub}],
    }
    return Agent(agent_id=pub, private_key=priv, client=client)

def test_remember(mock_agent):
    memory = mock_agent.remember("User likes Python", type="profile")
    assert isinstance(memory, Memory)
    assert memory.memory_type == "profile"
    mock_agent.client.create_memory.assert_called_once()

def test_recall(mock_agent):
    results = mock_agent.recall("what does user like?")
    assert len(results) == 1
    assert isinstance(results[0], SearchResult)
    assert results[0].score == 0.95

def test_recall_min_score_filter(mock_agent):
    results = mock_agent.recall("what does user like?", min_score=0.99)
    assert len(results) == 0  # 0.95 < 0.99

def test_get_recent(mock_agent):
    memories = mock_agent.get_recent(limit=5)
    assert len(memories) == 1
    assert isinstance(memories[0], Memory)

def test_export(mock_agent):
    export = mock_agent.export()
    assert export.total == 1
    assert len(export.memories) == 1
