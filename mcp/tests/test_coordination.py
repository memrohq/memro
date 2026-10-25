"""Server-side multi-agent coordination tests for MCP tools.

Tests atomic leases, expiry, heartbeats, handoffs, and scope isolation between
agents in the same scope.
"""
import os
import time
from types import SimpleNamespace

os.environ["MCP_TRANSPORT"] = "sse"

import memro_mcp.server as srv


class CoordinationScope:
    """Simulates server-side lease and intent state for a single scope."""

    def __init__(self):
        self.leases = {}     # resource -> dict
        self.intents = {}    # resource -> list of dict
        self.handoffs = {}   # resource -> dict

    def reap_expired(self, resource):
        now = time.time()
        if resource in self.leases:
            if self.leases[resource]["expires_at"] <= now:
                del self.leases[resource]
        if resource in self.intents:
            self.intents[resource] = [
                i for i in self.intents[resource] if i["expires_at"] > now
            ]


class FakeScopedAgent:
    """Agent in a shared scope that delegates coordination to the shared state."""

    def __init__(self, agent_id: str, scope: CoordinationScope):
        self.agent_id = agent_id
        self.scope = scope
        self.memories = []
        self.calls = []

    def remember(self, content, type="episodic", metadata=None, **kw):
        m = SimpleNamespace(id=str(len(self.memories)), content=content, memory_type=type, metadata=metadata)
        self.memories.insert(0, m)
        return m

    def get_recent(self, limit=10, type=None):
        self.calls.append(limit)
        return [m for m in self.memories if type is None or m.memory_type == type][:limit]

    def claim(self, resource: str, intent: str = "", session_id: str = "default", ttl_seconds: int = None):
        res = resource.strip()
        self.scope.reap_expired(res)
        ttl = ttl_seconds or 900
        now = time.time()

        if res in self.scope.leases:
            lease = self.scope.leases[res]
            if lease["holder"] == self.agent_id:
                # Idempotent extension
                lease["expires_at"] = now + ttl
                if intent:
                    lease["intent"] = intent
                return {"granted": True, "lease_id": lease["id"], "expires_at": lease["expires_at"]}
            return {
                "granted": False,
                "holder": lease["holder"],
                "holder_session": lease["session_id"],
                "intent": lease["intent"],
                "expires_at": lease["expires_at"],
            }

        # Check for handoff
        handoff_summary = None
        if res in self.scope.handoffs:
            ho = self.scope.handoffs[res]
            if not ho.get("next_agent_id") or ho.get("next_agent_id") == self.agent_id:
                handoff_summary = ho.get("summary")
                del self.scope.handoffs[res]

        lease_id = f"lease-{len(self.scope.leases) + 1}"
        self.scope.leases[res] = {
            "id": lease_id,
            "resource": res,
            "holder": self.agent_id,
            "session_id": session_id or "default",
            "intent": intent,
            "created_at": now,
            "heartbeat_at": now,
            "expires_at": now + ttl,
        }
        out = {"granted": True, "lease_id": lease_id, "expires_at": now + ttl}
        if handoff_summary:
            out["handoff_summary"] = handoff_summary
        return out

    def heartbeat(self, resource: str = None, lease_id: str = None, ttl_seconds: int = None, session_id: str = None):
        res = resource.strip()
        self.scope.reap_expired(res)
        if res not in self.scope.leases or self.scope.leases[res]["holder"] != self.agent_id:
            raise RuntimeError("lease not held by this agent")
        ttl = ttl_seconds or 900
        now = time.time()
        self.scope.leases[res]["expires_at"] = now + ttl
        self.scope.leases[res]["heartbeat_at"] = now
        return {"renewed": True, "expires_at": now + ttl}

    def release(self, resource: str = None, lease_id: str = None, status: str = "completed", session_id: str = None):
        res = resource.strip()
        self.scope.reap_expired(res)
        if res not in self.scope.leases or self.scope.leases[res]["holder"] != self.agent_id:
            raise RuntimeError("lease not held by this agent")
        del self.scope.leases[res]
        return {"released": True}

    def handoff(self, resource: str = None, lease_id: str = None, summary: str = "", next_agent_id: str = None, session_id: str = None):
        res = resource.strip()
        self.scope.reap_expired(res)
        if res not in self.scope.leases or self.scope.leases[res]["holder"] != self.agent_id:
            raise RuntimeError("lease not held by this agent")
        del self.scope.leases[res]
        self.scope.handoffs[res] = {
            "summary": summary,
            "from_agent_id": self.agent_id,
            "next_agent_id": next_agent_id,
            "created_at": time.time(),
        }
        return {"handoff": True}

    def check(self, resource: str, intent: str = "", session_id: str = None):
        res = resource.strip()
        self.scope.reap_expired(res)
        if res in self.scope.leases:
            lease = self.scope.leases[res]
            if lease["holder"] == self.agent_id:
                return {"decision": "allow", "reason": "Held by calling agent", "details": lease}
            return {
                "decision": "block",
                "reason": f"Resource '{res}' is currently claimed by agent {lease['holder']}.",
                "details": lease,
            }

        # Check intent overlaps by others
        intents = [
            i for i in self.scope.intents.get(res, [])
            if i["agent_id"] != self.agent_id
        ]
        if intents:
            return {
                "decision": "warn",
                "reason": f"Active intent by agent {intents[0]['agent_id']}: '{intents[0]['intent']}'",
                "intents": intents,
            }

        return {"decision": "allow"}

    def log_intent(self, resource: str, intent: str, ttl_seconds: int = None, session_id: str = None):
        res = resource.strip()
        ttl = ttl_seconds or 900
        self.scope.intents.setdefault(res, []).append({
            "resource": res,
            "agent_id": self.agent_id,
            "intent": intent,
            "expires_at": time.time() + ttl,
        })
        return {"logged": True}

    def active(self, prefix: str = None):
        now = time.time()
        leases = [
            l for r, l in self.scope.leases.items()
            if (not prefix or r.startswith(prefix)) and l["expires_at"] > now
        ]
        intents = [
            i for r, ilist in self.scope.intents.items()
            if not prefix or r.startswith(prefix)
            for i in ilist if i["expires_at"] > now
        ]
        return {"leases": leases, "intents": intents}


def call(agent, name, **args):
    return srv._run_tool(agent, "default", name, args)


def test_two_agents_in_one_scope_block_each_other():
    """Two agents sharing the same scope see each other's claims and block."""
    scope = CoordinationScope()
    agent_a = FakeScopedAgent("agent-a", scope)
    agent_b = FakeScopedAgent("agent-b", scope)

    # Agent A claims billing
    claim_a = call(agent_a, "claim_task", resource="billing/invoice.go", intent="refactor")
    assert not claim_a.isError
    assert "claimed" in claim_a.content[0].text.lower()

    # Agent B pre_action_check is blocked
    check_b = call(agent_b, "pre_action_check", resource="billing/invoice.go", intent="edit")
    assert "block" in check_b.content[0].text.lower()

    # Agent B claim_task fails
    claim_b = call(agent_b, "claim_task", resource="billing/invoice.go", intent="edit")
    assert claim_b.isError
    assert "claimed by agent agent-a" in claim_b.content[0].text

    # Agent A calling pre_action_check allows because A holds the lease
    check_a = call(agent_a, "pre_action_check", resource="billing/invoice.go", intent="edit")
    assert "allow" in check_a.content[0].text.lower()

    # Agent A re-claiming is idempotent
    reclaim_a = call(agent_a, "claim_task", resource="billing/invoice.go", intent="continue refactor")
    assert not reclaim_a.isError


def test_handoff_passes_summary_to_next_claimer():
    """Agent A hands off to Agent B; B's claim receives the handoff summary."""
    scope = CoordinationScope()
    agent_a = FakeScopedAgent("agent-a", scope)
    agent_b = FakeScopedAgent("agent-b", scope)

    call(agent_a, "claim_task", resource="docs/api.md", intent="drafting")
    handoff_res = call(agent_a, "handoff_task", resource="docs/api.md", summary="Draft completed; needs review", next_agent_id="agent-b")
    assert not handoff_res.isError

    # Agent B pre-check is now allowed
    check_b = call(agent_b, "pre_action_check", resource="docs/api.md", intent="review")
    assert "allow" in check_b.content[0].text.lower()

    # Agent B claims and sees the handoff summary
    claim_b = call(agent_b, "claim_task", resource="docs/api.md", intent="review")
    assert not claim_b.isError
    assert "Draft completed; needs review" in claim_b.content[0].text


def test_heartbeat_and_release():
    """Heartbeat extends lease; release allows another agent to claim."""
    scope = CoordinationScope()
    agent_a = FakeScopedAgent("agent-a", scope)
    agent_b = FakeScopedAgent("agent-b", scope)

    call(agent_a, "claim_task", resource="auth/token.go", intent="fix jwt")
    hb = call(agent_a, "heartbeat_task", resource="auth/token.go", ttl=1800)
    assert not hb.isError
    assert "heartbeat renewed" in hb.content[0].text

    # Release by A frees the resource
    rel = call(agent_a, "release_task", resource="auth/token.go")
    assert not rel.isError

    # B can now claim
    claim_b = call(agent_b, "claim_task", resource="auth/token.go", intent="inspect")
    assert not claim_b.isError


def test_intent_warns_other_agents():
    """Intent creates a soft warning without blocking claims."""
    scope = CoordinationScope()
    agent_a = FakeScopedAgent("agent-a", scope)
    agent_b = FakeScopedAgent("agent-b", scope)

    call(agent_a, "log_intent", resource="router/mux.go", intent="explore chi router")

    # Check by B yields warn
    check_b = call(agent_b, "pre_action_check", resource="router/mux.go", intent="rewrite")
    assert "warn" in check_b.content[0].text.lower()

    # Claim by B still succeeds because intents are non-blocking
    claim_b = call(agent_b, "claim_task", resource="router/mux.go", intent="rewrite")
    assert not claim_b.isError


def test_expiry_reaps_lease():
    """An expired lease is reaped when a new agent claims."""
    scope = CoordinationScope()
    agent_a = FakeScopedAgent("agent-a", scope)
    agent_b = FakeScopedAgent("agent-b", scope)

    # A claims with 1 second TTL
    call(agent_a, "claim_task", resource="cache/redis.go", intent="flush", ttl=1)
    # Manually expire the lease timestamp
    scope.leases["cache/redis.go"]["expires_at"] = time.time() - 5

    # B claims successfully because A's lease expired
    claim_b = call(agent_b, "claim_task", resource="cache/redis.go", intent="reconfigure")
    assert not claim_b.isError
    assert "claimed" in claim_b.content[0].text.lower()


def test_active_work_lists_claims_and_intents():
    """get_active_work and get_active_context return active coordination items."""
    scope = CoordinationScope()
    agent_a = FakeScopedAgent("agent-a", scope)

    call(agent_a, "claim_task", resource="pkg/a.go", intent="task a")
    call(agent_a, "log_intent", resource="pkg/b.go", intent="intent b")

    res = call(agent_a, "get_active_work")
    assert not res.isError
    assert "pkg/a.go" in res.content[0].text
    assert "pkg/b.go" in res.content[0].text

    res_ctx = call(agent_a, "get_active_context", resource="pkg/a.go")
    assert not res_ctx.isError
    assert "pkg/a.go" in res_ctx.content[0].text


def test_limits_are_clamped():
    scope = CoordinationScope()
    agent = FakeScopedAgent("agent-a", scope)
    call(agent, "get_recent_memories", limit=10_000)
    call(agent, "get_recent_memories", limit="nonsense")
    assert agent.calls == [srv.MAX_LIMIT, 10]


def test_block_message_names_the_holding_tool_and_overlapping_folder():
    """Tools on one account share an agent id, so the message names the session."""
    agent = SimpleNamespace(claim=lambda **kw: {
        "granted": False, "holder": "ff87", "holder_session": "claude-code",
        "holder_resource": "memro/bench", "intent": "refactor harness",
        "expires_at": "2026-10-23T21:07:32Z"})
    out = call(agent, "claim_task", resource="memro/bench/run.py", intent="edit")
    text = out.content[0].text
    assert out.isError
    assert "'memro/bench', which overlaps 'memro/bench/run.py'" in text
    assert "claimed by claude-code" in text and "refactor harness" in text


def test_session_from_user_agent():
    assert srv._session_from_user_agent("claude-code/2.1.4 (external, cli)") == "ua:claude-code"
    assert srv._session_from_user_agent("Antigravity/1.11 Electron") == "ua:antigravity"
    for generic in ("node", "python-httpx/0.27", "Mozilla/5.0 (Macintosh)", "", "  "):
        assert srv._session_from_user_agent(generic) is None
