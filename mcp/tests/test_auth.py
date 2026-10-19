"""Every hosted request must bring its own credential, and a connection only
ever acts as the agent it authenticated as."""
import os

os.environ["MCP_TRANSPORT"] = "sse"

import pytest
from starlette.testclient import TestClient

import memro_mcp.server as srv

# The autouse fixture stubs agent_for_api_key; keep the real one for its own test.
srv._real_agent_for_api_key = srv.agent_for_api_key


class FakeAgent:
    def __init__(self, agent_id):
        self.agent_id = agent_id


@pytest.fixture(autouse=True)
def fake_api_keys(monkeypatch):
    def lookup(key):
        if key == "memro_sk_revoked":
            raise srv.MemroError(401, "Invalid or revoked API key")
        return FakeAgent("agent-" + key)

    monkeypatch.setattr(srv, "agent_for_api_key", lookup)
    monkeypatch.setenv("MEMRO_API_KEY", "memro_sk_server_owner")


def test_api_key_resolves_to_its_own_agent():
    assert srv.resolve_agent(token="memro_sk_a").agent_id == "agent-memro_sk_a"
    assert srv.resolve_agent(token="memro_sk_b").agent_id == "agent-memro_sk_b"


def test_no_credential_is_rejected_even_with_env_key():
    with pytest.raises(srv.AuthRequired):
        srv.resolve_agent()


def test_private_key_is_rejected_by_default():
    with pytest.raises(srv.AuthRequired):
        srv.resolve_agent(agent_id="ab" * 32, token="cd" * 32)


@pytest.fixture(scope="module")
def client():
    # The MCP session manager can only start once per process.
    with TestClient(srv.starlette_app) as c:
        yield c


@pytest.mark.parametrize("url", ["/sse/", "/sse/?agent_id=x&token=deadbeef", "/sse/?token=memro_sk_revoked", "/mcp",
                                 "/mcp?token=memro_sk_a"])
def test_unauthenticated_connections_get_401(client, url):
    assert client.get(url).status_code == 401


def test_mcp_needs_a_bearer_scheme(client):
    assert client.post("/mcp", headers={"Authorization": "Basic memro_sk_a"}, json={}).status_code == 401


def test_webhook_stub_is_gone(client):
    assert client.post("/webhook/anyone", json={}).status_code == 404


def test_api_calls_carry_the_end_users_ip():
    client = srv.ForwardingClient("http://api.invalid", api_key="memro_sk_a")
    token = srv.current_client_ip.set("203.0.113.7")
    try:
        assert client._headers("GET", "/search", b"", None, None)["X-Real-IP"] == "203.0.113.7"
    finally:
        srv.current_client_ip.reset(token)
    assert "X-Real-IP" not in client._headers("GET", "/search", b"", None, None)


def test_key_cache_is_bounded_hashed_and_expires(monkeypatch):
    calls = []

    class StubAgent:
        @staticmethod
        def from_api_key(key, client=None):
            calls.append(key)
            return FakeAgent("agent-" + key)

    monkeypatch.setattr(srv, "Agent", StubAgent)
    monkeypatch.setattr(srv, "API_KEY_CACHE_SIZE", 2)
    monkeypatch.setattr(srv, "_api_key_agents", srv.OrderedDict())
    lookup = srv.__dict__["_real_agent_for_api_key"]
    lookup("memro_sk_a"); lookup("memro_sk_a")
    assert calls == ["memro_sk_a"]
    assert all("memro_sk_" not in k for k in srv._api_key_agents)
    lookup("memro_sk_b"); lookup("memro_sk_c")
    assert len(srv._api_key_agents) == 2
    monkeypatch.setattr(srv, "API_KEY_CACHE_TTL", 0)
    lookup("memro_sk_c")
    assert calls.count("memro_sk_c") == 2


@pytest.mark.parametrize("trusted, want", [(True, "203.0.113.9"), (False, "testclient")])
def test_x_real_ip_is_only_trusted_when_configured(monkeypatch, trusted, want):
    monkeypatch.setattr(srv, "TRUST_PROXY_HEADERS", trusted)
    seen = {}

    async def app(scope, receive, send):
        seen["ip"] = srv.current_client_ip.get()
        await send({"type": "http.response.start", "status": 204, "headers": []})
        await send({"type": "http.response.body", "body": b""})

    TestClient(srv.AuthMiddleware(app)).get("/healthz", headers={"X-Real-IP": "203.0.113.9"})
    assert seen["ip"] == want
