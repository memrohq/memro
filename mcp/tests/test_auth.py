"""Every hosted request must bring its own credential, and a connection only
ever acts as the agent it authenticated as."""
import os

os.environ["MCP_TRANSPORT"] = "sse"

import pytest
from starlette.testclient import TestClient

import memro_mcp.server as srv


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


@pytest.mark.parametrize("url", ["/sse/", "/sse/?agent_id=x&token=deadbeef", "/sse/?token=memro_sk_revoked", "/mcp"])
def test_unauthenticated_connections_get_401(client, url):
    assert client.get(url).status_code == 401


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
