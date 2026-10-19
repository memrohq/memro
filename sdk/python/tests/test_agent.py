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


class _Recorder:
    """Stands in for requests.Session.request and records what was sent."""

    def __init__(self, status=200, body=b"{}", headers=None):
        self.calls = []
        self.status, self.body, self.headers = status, body, headers or {}

    def __call__(self, method, url, data=None, headers=None, timeout=None):
        self.calls.append({"method": method, "url": url, "data": data or b"", "headers": headers})
        resp = MagicMock(status_code=self.status, ok=200 <= self.status < 300, content=self.body,
                         text=self.body.decode(), reason="", headers=self.headers)
        resp.json.side_effect = lambda: json.loads(self.body)
        return resp


def _verify_v2(agent_id, call, path_and_query):
    import nacl.signing
    h = call["headers"]
    assert h["X-Signature-Version"] == "2" and h["X-Agent-Id"] == agent_id
    payload = f"{h['X-Timestamp']}\n{call['method']}\n{path_and_query}\n".encode() + (call["data"] or b"")
    nacl.signing.VerifyKey(bytes.fromhex(agent_id)).verify(payload, bytes.fromhex(h["X-Signature"]))


def test_delete_is_signed_with_v2_over_an_empty_body():
    agent_id, private_key = generate_keypair()
    client = MemroClient(base_url="http://example.test")
    client._session.request = rec = _Recorder(status=204, body=b"")
    assert client.delete_memory(agent_id, private_key, "11111111-1111-1111-1111-111111111111")
    _verify_v2(agent_id, rec.calls[0], "/memory/11111111-1111-1111-1111-111111111111")


def test_signature_covers_the_exact_query_string():
    agent_id, private_key = generate_keypair()
    client = MemroClient(base_url="http://example.test/api")
    client._session.request = rec = _Recorder(body=b'{"results": []}')
    client.search(agent_id, "what & why?", limit=5, private_key=private_key, mode="keyword")
    url = rec.calls[0]["url"]
    assert url.startswith("http://example.test/api/search?")
    _verify_v2(agent_id, rec.calls[0], url[len("http://example.test/api"):])


def test_signature_covers_the_json_body():
    agent_id, private_key = generate_keypair()
    client = MemroClient(base_url="http://example.test")
    client._session.request = rec = _Recorder(body=json.dumps({**FAKE_MEMORY, "agent_id": agent_id}).encode())
    client.create_memory(agent_id, private_key, "hello", "semantic")
    _verify_v2(agent_id, rec.calls[0], "/memory")
    assert json.loads(rec.calls[0]["data"])["content"] == "hello"


def test_multipart_upload_is_signed():
    agent_id, private_key = generate_keypair()
    client = MemroClient(base_url="http://example.test")
    client._session.request = rec = _Recorder(body=b'{"file_id": "f", "file_name": "a.txt", "file_type": "text", "file_size": 2}')
    client.upload_file(agent_id, private_key, ("a.txt", b"hi"))
    assert rec.calls[0]["headers"]["Content-Type"].startswith("multipart/form-data")
    _verify_v2(agent_id, rec.calls[0], "/upload")


def test_api_key_uses_bearer_header_and_no_signature():
    client = MemroClient(base_url="http://example.test", api_key="memro_sk_test")
    client._session.request = rec = _Recorder(body=b'{"agent_id": "a"}')
    client.whoami()
    h = rec.calls[0]["headers"]
    assert h["Authorization"] == "Bearer memro_sk_test" and "X-Signature" not in h


def test_rejects_keys_that_are_not_api_keys():
    with pytest.raises(ValueError):
        MemroClient(api_key="0" * 64)


def test_default_base_url_is_memro_cloud(monkeypatch):
    monkeypatch.delenv("MEMRO_BASE_URL", raising=False)
    assert MemroClient().base_url == "https://api.memro.co"


def test_rate_limited_requests_are_retried():
    client = MemroClient(base_url="http://example.test", api_key="memro_sk_test", max_retries=2)
    responses = [_Recorder(status=429, body=b"Rate limit exceeded", headers={"Retry-After": "0"}), _Recorder(body=b'{"agent_id": "a"}')]
    calls = []
    def fake(*a, **kw):
        calls.append(1)
        return responses[min(len(calls), 2) - 1](*a, **kw)
    client._session.request = fake
    assert client.whoami()["agent_id"] == "a" and len(calls) == 2


def test_post_is_not_retried_after_a_gateway_error():
    from memro import MemroError
    client = MemroClient(base_url="http://example.test", api_key="memro_sk_test")
    client._session.request = rec = _Recorder(status=502, body=b"bad gateway")
    with pytest.raises(MemroError):
        client.create_memory("a", None, "x")
    assert len(rec.calls) == 1
