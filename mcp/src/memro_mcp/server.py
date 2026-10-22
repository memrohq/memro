import os
import sys
import asyncio
import json
import logging
import uuid
import re
import hashlib
import threading
import time
from collections import OrderedDict
from contextvars import ContextVar
from dotenv import load_dotenv
from urllib.parse import parse_qs

# Load local .env if it exists
load_dotenv()
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.server.sse import SseServerTransport
from mcp.server.streamable_http_manager import StreamableHTTPSessionManager
import contextlib
from mcp.types import Tool, TextContent, CallToolResult, JSONRPCNotification
from memro import Agent, MemroClient, MemroError
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.routing import Route, Mount
from starlette.responses import Response, JSONResponse, PlainTextResponse
import uvicorn

try:
    import uvloop
    uvloop_available = True
except ImportError:
    uvloop_available = False

try:
    from importlib.metadata import version as _version
    SERVER_VERSION = _version("memro-mcp")
except Exception:
    SERVER_VERSION = "0.0.0"

# --- Core Objects (Module Level for Uvicorn Workers) ---
# Sent to every client on connect (MCP "instructions"); clients such as Claude
# Code put it in the agent's context, so tools use Memro without being told.
# memro-cli writes the same guidance into tools' global instruction files for
# clients that ignore this; keep the two in step.
INSTRUCTIONS = """\
Memro is the user's long-term memory, shared across their AI tools and sessions.

Use it without being asked:
- At the start of a task or conversation, call `recall` with a short query about \
the task (project, topic, people involved) and use what comes back.
- Before answering questions about earlier work ("what did we decide about X?"), \
`recall` first.
- Call `remember` with one short, self-contained sentence when something will \
matter in a later session: a decision and why, a fix or workflow worth reusing, \
a preference or correction from the user, or an important project constraint.

Types: "semantic" for facts ("Memro uses PostgreSQL"), "profile" for the user's \
preferences and working style ("User prefers reviewing migrations before \
deploys"), "procedural" for how-tos ("To run integration tests, start PostgreSQL \
first"), "episodic" for significant events ("The billing isolation tests \
passed"). If unsure, leave the type out and Memro chooses.

Don't save routine events (connecting a tool, starting a session), passing \
status, facts Memro already has (recall first), or summaries of a whole \
conversation. Never store secrets (API keys, passwords, tokens, private keys) or \
anything the user asks you not to keep. Don't announce routine saves; mention a \
memory only when it changes your answer."""

server = Server("memro-memory", version=SERVER_VERSION, instructions=INSTRUCTIONS)
# Use a path that will be appended to our mount point
sse = SseServerTransport("/messages/")

# Context variables for request-scoped state
current_session_id: ContextVar[str] = ContextVar("session_id", default="default")
current_auth_info: ContextVar[dict] = ContextVar("auth_info", default={})
# The agent an SSE connection authenticated as; tool calls on that connection
# run in its context.
current_agent: ContextVar = ContextVar("agent", default=None)
# The end user's address, passed on to the API so its per-IP rate limit
# applies per user rather than to the MCP server as a whole.
current_client_ip: ContextVar = ContextVar("client_ip", default=None)

# Over HTTP every request must bring its own credential. The environment
# fallbacks (MEMRO_API_KEY, MEMRO_AGENT_ID/MEMRO_PRIVATE_KEY, a throwaway
# identity) are only for a local stdio server, where there is one user.
HTTP_MODE = os.getenv("MCP_TRANSPORT", "stdio").lower() != "stdio"
# Legacy: accept an agent's private key as the SSE token. Off by default,
# since the key travels in the URL.
ALLOW_PRIVATE_KEY_AUTH = os.getenv("MCP_ALLOW_PRIVATE_KEY_AUTH") == "1"


class AuthRequired(Exception):
    pass

# sha256(API key) -> (Agent, resolved at), so whoami runs once per key per
# TTL rather than on every request. The TTL bounds how long a revoked key can
# still open a session (its tool calls fail at the API right away).
_api_key_agents: "OrderedDict[str, tuple]" = OrderedDict()
_api_key_lock = threading.Lock()
API_KEY_CACHE_SIZE = int(os.getenv("MCP_KEY_CACHE_SIZE", "10000"))
API_KEY_CACHE_TTL = float(os.getenv("MCP_KEY_CACHE_TTL", "300"))
API_KEY_PREFIX = "memro_sk_"
# Read the caller's address from X-Real-IP only behind a proxy that sets it;
# otherwise anyone could pick the address the API rate-limits them by.
TRUST_PROXY_HEADERS = os.getenv("MCP_TRUST_PROXY_HEADERS", "").lower() in ("1", "true")


class ForwardingClient(MemroClient):
    """MemroClient that tells the API which end user a request is for."""

    def _headers(self, method, path_and_query, body, agent_id, private_key) -> dict:
        headers = super()._headers(method, path_and_query, body, agent_id, private_key)
        ip = current_client_ip.get()
        if ip:
            headers["X-Real-IP"] = ip
        return headers


def agent_for_api_key(api_key: str) -> Agent:
    digest = hashlib.sha256(api_key.encode()).hexdigest()
    now = time.monotonic()
    with _api_key_lock:
        hit = _api_key_agents.get(digest)
        if hit and now - hit[1] < API_KEY_CACHE_TTL:
            _api_key_agents.move_to_end(digest)
            return hit[0]
    base_url = os.getenv("MEMRO_BASE_URL", "http://localhost:8081")
    agent = Agent.from_api_key(api_key, client=ForwardingClient(base_url, api_key=api_key))
    with _api_key_lock:
        _api_key_agents[digest] = (agent, now)
        _api_key_agents.move_to_end(digest)
        while len(_api_key_agents) > API_KEY_CACHE_SIZE:
            _api_key_agents.popitem(last=False)
    return agent

# Logs go to stderr (stdout is the stdio transport). Tool arguments contain
# memory content and are never logged.
logging.basicConfig(
    stream=sys.stderr,
    level=os.getenv("MCP_LOG_LEVEL", "INFO").upper(),
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
)
logger = logging.getLogger("memro_mcp")

# --- Coordination ---

class Decision:
    ALLOW = "allow"
    WARN = "warn"
    BLOCK = "block"

# --- Helper Functions ---

def resolve_agent(agent_id: str = None, token: str = None) -> Agent:
    """Return the agent for a credential. Raises AuthRequired over HTTP when
    the request has no usable credential."""
    if token and token.startswith(API_KEY_PREFIX):
        return agent_for_api_key(token)

    base_url = os.getenv("MEMRO_BASE_URL", "http://localhost:8081")
    if HTTP_MODE:
        if ALLOW_PRIVATE_KEY_AUTH and token and agent_id:
            key = token[3:] if token.startswith("sk_") else token
            return Agent(agent_id=agent_id, private_key=key, client=ForwardingClient(base_url))
        raise AuthRequired("Send a memro_sk_ API key (Authorization: Bearer ...). Run `npx memro-cli connect` to get one.")

    if os.getenv("MEMRO_API_KEY"):
        return agent_for_api_key(os.getenv("MEMRO_API_KEY"))
    if os.getenv("MEMRO_AGENT_ID") and os.getenv("MEMRO_PRIVATE_KEY"):
        return Agent.from_env(client=ForwardingClient(base_url))
    global _local_agent
    if _local_agent is None:
        _local_agent = Agent.create(client=ForwardingClient(base_url))
    return _local_agent


_local_agent = None


def current_request_agent() -> Agent:
    agent = current_agent.get()
    if agent is not None:
        return agent
    auth = current_auth_info.get()
    return resolve_agent(auth.get("agent_id"), auth.get("token"))

# --- MCP Tool Handlers ---

from memro_mcp.supermemory_tools import get_supermemory_tools

@server.list_tools()
async def list_tools() -> list[Tool]:
    # Clients reject duplicate tool names; the core definitions win over the
    # supermemory-compatible set, which repeats remember_with_context.
    unique: dict[str, Tool] = {}
    for tool in _all_tools():
        unique.setdefault(tool.name, tool)
    return list(unique.values())


# A memory's type, for the remember tools. No default: an omitted type lets the
# server classify the memory instead of filing everything as episodic.
MEMORY_TYPE = {
    "type": "string",
    "enum": ["semantic", "profile", "procedural", "episodic"],
    "description": (
        "semantic: a fact. profile: the user's preferences or working style. "
        "procedural: how to do something. episodic: a significant event. "
        "Omit if unsure and Memro chooses."
    ),
}


def _memory_type(args: dict):
    """The caller's type, or None so the server classifies the memory."""
    value = args.get("type")
    return value.strip().lower() if isinstance(value, str) and value.strip() else None


def _all_tools() -> list[Tool]:
    logger.info("Listing tools for client")
    return [
        Tool(
            name="remember",
            description=(
                "Save something worth knowing in future sessions: a preference, decision, "
                "correction, or lasting fact about the user or their project. One short, "
                "self-contained sentence per memory. Never store secrets."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "content": {"type": "string"},
                    "type": MEMORY_TYPE,
                    "visibility": {"type": "string", "enum": ["private", "shared", "public"], "default": "private"},
                    "event_date": {"type": "string"},
                    "session_id": {"type": "string"},
                    "is_atomic": {"type": "boolean", "default": False},
                },
                "required": ["content"],
            },
        ),
        Tool(
            name="recall",
            description=(
                "Search the user's long-term memory. Call it at the start of a task and "
                "before answering questions about earlier work, preferences or decisions."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string"},
                    "limit": {"type": "integer", "default": 5},
                    "type": {"type": "string", "enum": ["episodic", "semantic", "procedural", "profile"]},
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="claim_task",
            description=(
                "Claim a file or task with intent to work on it. Claims are shared with "
                "agents in the same account/space and expire after the TTL."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string", "description": "File path or module"},
                    "intent": {"type": "string", "description": "What you plan to do"},
                    "ttl": {"type": "integer", "description": "Lease duration in seconds (default 900, max 7200)"},
                },
                "required": ["resource", "intent"],
            },
        ),
        Tool(
            name="heartbeat_task",
            description="Extend an active lease on a resource so it does not expire.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string", "description": "Resource to heartbeat"},
                    "ttl": {"type": "integer", "description": "New TTL in seconds from now (default 900, max 7200)"},
                },
                "required": ["resource"],
            },
        ),
        Tool(
            name="release_task",
            description="Release a claimed resource and log outcome. Releases the lease in the shared account/space.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "status": {"type": "string", "enum": ["completed", "abandoned", "cancelled", "handoff"], "default": "completed"},
                },
                "required": ["resource"],
            },
        ),
        Tool(
            name="log_intent",
            description="Declare your non-blocking intention for a resource (shared with agents in the same account/space).",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "intent": {"type": "string"},
                    "ttl": {"type": "integer", "description": "Intent duration in seconds (default 900, max 7200)"},
                },
                "required": ["resource", "intent"],
            },
        ),
        Tool(
            name="handoff_task",
            description="Atomically summarizes work and releases a resource claim for the next agent.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "summary": {"type": "string"},
                    "next_agent_id": {"type": "string"},
                },
                "required": ["resource", "summary"],
            },
        ),
        Tool(
            name="pre_action_check",
            description="Check for conflicts or overlaps before performing an action against shared leases and intents.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "intent": {"type": "string"},
                },
                "required": ["resource", "intent"],
            },
        ),
        Tool(
            name="delete_memory",
            description="Delete one memory by ID, e.g. when the user says it is wrong or asks you to forget it.",
            inputSchema={
                "type": "object",
                "properties": {
                    "memory_id": {"type": "string"},
                },
                "required": ["memory_id"],
            },
        ),
        Tool(
            name="export_memories",
            description="Export all memories for the agent.",
            inputSchema={"type": "object", "properties": {}},
        ),
        Tool(
            name="get_recent_memories",
            description="List the most recently saved memories (newest first), e.g. to review what was stored.",
            inputSchema={
                "type": "object",
                "properties": {
                    "limit": {"type": "integer", "default": 10},
                    "type": {"type": "string", "enum": ["episodic", "semantic", "procedural", "profile"]},
                },
            },
        ),
        Tool(
            name="remember_with_context",
            description="Store a new memory with rich metadata context (file, git, function).",
            inputSchema={
                "type": "object",
                "properties": {
                    "content": {"type": "string"},
                    "context_metadata": {"type": "object"},
                    "type": MEMORY_TYPE,
                    "event_date": {"type": "string"},
                    "session_id": {"type": "string"},
                },
                "required": ["content"],
            },
        ),
        Tool(
            name="get_active_work",
            description="Get ALL active claims and intents for the shared account/space.",
            inputSchema={"type": "object", "properties": {}},
        ),
        Tool(
            name="get_active_context",
            description="Get coordination context (claims/intents) for a specific resource in the shared account/space.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string", "description": "File or resource to check"},
                },
                "required": ["resource"],
            },
        ),
        Tool(
            name="get_system_status",
            description="Check the health and status of the Memro MCP server and its connections.",
            inputSchema={"type": "object", "properties": {}},
        ),
        *get_supermemory_tools()
    ]

def _text(value) -> CallToolResult:
    if not isinstance(value, str):
        value = json.dumps(value, indent=2, default=str)
    return CallToolResult(content=[TextContent(type="text", text=value)])


def _dump(model) -> dict:
    return model.model_dump(mode="json") if hasattr(model, "model_dump") else model


MAX_LIMIT = 50


def _limit(args: dict, default: int) -> int:
    """The caller's limit, clamped so one call cannot pull an unbounded page."""
    try:
        return max(1, min(int(args.get("limit", default)), MAX_LIMIT))
    except (TypeError, ValueError):
        return default


def _run_tool(agent: Agent, session_id: str, name: str, args: dict) -> CallToolResult:
    """Executes one tool. Runs in a worker thread: the SDK is synchronous."""
    if name == "remember":
        memory = agent.remember(content=args.get("content"), type=_memory_type(args),
                                visibility=args.get("visibility", "private"),
                                event_date=args.get("event_date"), session_id=args.get("session_id"),
                                is_atomic=bool(args.get("is_atomic", False)))
        return _text(f"Remembered: {memory.content[:100]} (ID: {memory.id})")

    if name == "remember_with_context":
        metadata = args.get("context_metadata")
        memory = agent.remember(content=args.get("content"), type=_memory_type(args),
                                event_date=args.get("event_date"), session_id=args.get("session_id"),
                                metadata=metadata if isinstance(metadata, dict) else None)
        return _text(f"Remembered with context: {memory.content[:100]} (ID: {memory.id})")

    if name in ("recall", "recall_multi_session"):
        results = agent.recall(query=args.get("query"), limit=_limit(args, 5 if name == "recall" else 15),
                               type=args.get("type"))
        return _text([{"id": r.memory_id, "content": r.content, "type": r.memory_type, "score": r.score,
                       "created_at": r.created_at} for r in results])

    if name == "get_recent_memories":
        memories = agent.get_recent(limit=_limit(args, 10), type=args.get("type"))
        return _text([{"id": m.id, "content": m.content, "type": m.memory_type,
                       "created_at": m.created_at.isoformat(), "metadata": m.metadata or {}} for m in memories])

    if name == "delete_memory":
        return _text("Memory deleted." if agent.delete_memory(args.get("memory_id")) else "Memory not found.")

    if name == "export_memories":
        return _text(f"Exported {agent.export().total} memories.")

    if name == "search_temporal":
        results = agent.recall_temporal(
            query=args.get("query"), limit=_limit(args, 10),
            after_date=args.get("after_date"), before_date=args.get("before_date"),
            after_event=args.get("after_event_date"), before_event=args.get("before_event_date"),
            session_id=args.get("session_id"), atomic_only=args.get("atomic_only"),
            temporal_weight=args.get("temporal_weight", 0.3),
        )
        if not results:
            return _text("No matching memories.")
        return _text("\n".join(f"- {r.content} ({r.temporal_context})" for r in results))

    if name == "get_temporal_stats":
        return _text(_dump(agent.stats()))

    if name == "get_memory_chain":
        return _text(_dump(agent.memory_chain(args.get("memory_id"))))

    if name in ("find_contradictions", "analyze_conflicts"):
        return _text(agent.client.analyze_conflicts(agent.agent_id, agent._private_key))

    if name == "generate_atomic_facts":
        return _text(agent.client.generate_atoms(agent.agent_id, agent._private_key, args.get("text", "")))

    if name == "mee_reason":
        r = agent.reason(args.get("query"))
        return _text({"answer": r.answer, "confidence": r.confidence, "why": r.why,
                      "based_on": r.based_on, "paths": r.paths})

    if name == "mee_resolve":
        return _text(agent.resolve_conflict(args.get("memory_a_id"), args.get("memory_b_id")))

    if name == "mee_evolve":
        return _text(_dump(agent.evolve()))

    if name == "get_timeline":
        return _text(_dump(agent.timeline(args.get("memory_id"))))

    if name == "pre_action_check":
        return _text(agent.check(resource=args.get("resource"), intent=args.get("intent", ""), session_id=session_id))

    if name == "claim_task":
        resource, intent = args.get("resource"), args.get("intent")
        ttl = args.get("ttl")
        if ttl is not None:
            try:
                ttl = int(ttl)
            except (ValueError, TypeError):
                ttl = None
        res = agent.claim(resource=resource, intent=intent, session_id=session_id, ttl_seconds=ttl)
        if not res.get("granted"):
            # Tools on one account share an agent id; the session names the tool.
            session = res.get("holder_session")
            who = session if session and session != "default" else f"agent {res.get('holder', 'unknown')}"
            held = res.get("holder_resource")
            where = f"'{held}', which overlaps '{resource}'" if held and held != resource else f"'{resource}'"
            msg = f"Block: {where} is claimed by {who}"
            if res.get("expires_at"):
                msg += f" until {res.get('expires_at')}"
            msg += "."
            if res.get("intent"):
                msg += f" Intent: {res.get('intent')}."
            msg += " Don't edit it; tell the user, or wait for it to be released."
            return CallToolResult(isError=True, content=[TextContent(type="text", text=msg)])
        msg = f"Resource '{resource}' claimed."
        if res.get("handoff_summary"):
            msg += f" Handoff summary from previous agent: {res.get('handoff_summary')}"
        return _text(msg)

    if name == "heartbeat_task":
        resource = args.get("resource")
        ttl = args.get("ttl")
        if ttl is not None:
            try:
                ttl = int(ttl)
            except (ValueError, TypeError):
                ttl = None
        res = agent.heartbeat(resource=resource, ttl_seconds=ttl, session_id=session_id)
        return _text(f"Resource '{resource}' heartbeat renewed until {res.get('expires_at')}.")

    if name == "release_task":
        resource = args.get("resource")
        status = args.get("status", "completed")
        if status == "cancelled":
            status = "abandoned"
        agent.release(resource=resource, status=status, session_id=session_id)
        return _text(f"Resource '{resource}' released.")

    if name == "log_intent":
        intent, resource = args.get("intent"), args.get("resource")
        ttl = args.get("ttl")
        if ttl is not None:
            try:
                ttl = int(ttl)
            except (ValueError, TypeError):
                ttl = None
        agent.log_intent(resource=resource, intent=intent, ttl_seconds=ttl, session_id=session_id)
        return _text(f"Intent for {resource} logged.")

    if name in ("get_active_work", "get_active_context"):
        resource = args.get("resource")
        active = agent.active(prefix=resource)
        return _text({"claims": active.get("leases", []), "intents": active.get("intents", [])})

    if name == "handoff_task":
        resource, summary, nxt = args.get("resource"), args.get("summary"), args.get("next_agent_id")
        agent.handoff(resource=resource, summary=summary, next_agent_id=nxt, session_id=session_id)
        return _text(f"Handoff completed for {resource}.")

    if name == "get_system_status":
        return _text({"status": "online", "version": SERVER_VERSION, "agent_id": agent.agent_id,
                      "api": agent.client.base_url, "api_health": agent.client.health()})

    return CallToolResult(isError=True, content=[TextContent(type="text", text=f"Unknown tool: {name}")])


@server.call_tool()
async def call_tool(name: str, arguments: dict) -> CallToolResult:
    """Runs a tool as the connection's agent, off the event loop so one slow
    API call does not stall other users' requests."""
    try:
        agent = current_request_agent()
        logger.info(f"Tool call: {name} | agent={agent.agent_id[:12]}")
        # to_thread copies the context, so the caller's IP is still forwarded.
        return await asyncio.to_thread(_run_tool, agent, current_session_id.get(), name, arguments or {})
    except MemroError as e:
        return CallToolResult(isError=True, content=[TextContent(type="text", text=f"Memro API error {e.status_code}: {e.message}")])
    except AuthRequired as e:
        return CallToolResult(isError=True, content=[TextContent(type="text", text=str(e))])
    except Exception:
        # The details stay in our logs; the client gets an ID to quote.
        ref = uuid.uuid4().hex[:12]
        logger.exception("Error in call_tool %s (ref %s)", name, ref)
        return CallToolResult(isError=True, content=[TextContent(type="text", text=f"Tool {name} failed (ref {ref}).")])

# --- ASGI / SSE Application Logic ---
# --- Middleware for Auth/Context ---

class AuthMiddleware:
    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            return await self.app(scope, receive, send)

        path = scope.get("path", "")
        query_string = scope.get("query_string", b"").decode()
        params = parse_qs(query_string)
        
        # The coordination session: an explicit ?session_id=, else the MCP
        # transport's per-connection Mcp-Session-Id, so each connected tool
        # (Claude Code, Cursor, ...) is its own holder even on one account.
        session_id = params.get("session_id", [None])[0]
        if not session_id:
            sid = dict(scope.get("headers") or []).get(b"mcp-session-id", b"").decode().strip()
            session_id = f"mcp:{sid}" if sid else None
        if not session_id:
            # Stateless transport and no ?session_id= (tools connected before
            # `memro connect` added it): tell tools apart by User-Agent.
            session_id = _session_from_user_agent(
                dict(scope.get("headers") or []).get(b"user-agent", b"").decode(errors="replace"))
        agent_id = token = None
        # Only the legacy SSE transport takes credentials in the URL, where
        # they end up in proxy logs; everything else needs the header.
        if path.startswith("/sse"):
            agent_id = params.get("agent_id", [None])[0]
            token = params.get("token", [None])[0]
        if not token:
            scheme, _, value = Request(scope, receive).headers.get("Authorization", "").partition(" ")
            if scheme.lower() == "bearer":
                token = value.strip() or None

        # The edge proxy sets X-Real-IP to the caller's address.
        headers = dict(scope.get("headers") or [])
        forwarded = headers.get(b"x-real-ip", b"").decode().strip() if TRUST_PROXY_HEADERS else ""
        client_ip = forwarded or (scope.get("client") or [None])[0]
        t_ip = current_client_ip.set(client_ip)
        t_sid = current_session_id.set(session_id or "default")
        t_auth = current_auth_info.set({"agent_id": agent_id, "token": token})
        
        try:
            await self.app(scope, receive, send)
        finally:
            current_auth_info.reset(t_auth)
            current_session_id.reset(t_sid)
            current_client_ip.reset(t_ip)

# HTTP libraries and browsers that say nothing about which tool is calling.
_GENERIC_AGENTS = {"mozilla", "node", "node-fetch", "undici", "axios", "python-httpx", "python-requests",
                   "python-urllib", "aiohttp", "curl", "wget", "go-http-client", "okhttp", "java", "dart"}


def _session_from_user_agent(ua: str):
    """'claude-code/2.1 (cli)' -> 'ua:claude-code'; None for generic clients."""
    product = re.sub(r"[^a-z0-9._-]", "", ua.strip().split(" ", 1)[0].split("/", 1)[0].lower())[:40]
    if not product or product in _GENERIC_AGENTS:
        return None
    return f"ua:{product}"


# --- ASGI SSE Handler ---

def _unauthorized(message: str) -> JSONResponse:
    return JSONResponse({"error": "unauthorized", "message": message}, status_code=401,
                        headers={"WWW-Authenticate": 'Bearer realm="memro"'})


async def _authenticate_request():
    """Resolve the caller's agent, or return an error response."""
    auth = current_auth_info.get()
    try:
        return await asyncio.to_thread(resolve_agent, auth.get("agent_id"), auth.get("token")), None
    except AuthRequired as e:
        return None, _unauthorized(str(e))
    except MemroError as e:
        if e.status_code in (401, 403):
            return None, _unauthorized(str(e))
        return None, JSONResponse({"error": "upstream_error", "message": str(e)}, status_code=502)
    except Exception:
        logger.exception("Memro API unreachable while checking credentials")
        return None, JSONResponse({"error": "upstream_error", "message": "Memro API is unreachable"}, status_code=502)


async def handle_sse(scope, receive, send):
    """Legacy SSE transport. The connection is bound to the agent it
    authenticated as; tool calls on it run as that agent."""
    agent, error = await _authenticate_request()
    if error is not None:
        return await error(scope, receive, send)
    current_agent.set(agent)
    agent_id = agent.agent_id[:12]

    async def wrapped_send(message):
        if message.get("type") == "http.response.body":
            body = message.get("body", b"").decode(errors="ignore")
            if "event: endpoint" in body:
                # The transport advertises /messages/; it is mounted at /sse/messages/.
                message["body"] = re.sub(r"data: /messages/", "data: /sse/messages/", body).encode()
        await send(message)

    async with sse.connect_sse(scope, receive, wrapped_send) as (read_stream, write_stream):
        async def heartbeat_loop():
            while True:
                try:
                    await asyncio.sleep(15)
                    await write_stream.send(JSONRPCNotification(
                        method="notifications/heartbeat", 
                        params={"note": "keep-alive"}
                    ))
                except Exception: break

        logger.info(f"Starting server session for agent {agent_id}")
        server_task = asyncio.create_task(server.run(
            read_stream, 
            write_stream, 
            server.create_initialization_options()
        ))
        heartbeat_task = asyncio.create_task(heartbeat_loop())
        
        await asyncio.wait([server_task, heartbeat_task], return_when=asyncio.FIRST_COMPLETED)
        heartbeat_task.cancel()
        logger.info(f"Server session terminated for agent {agent_id}")

async def handle_messages(scope, receive, send):
    """Directly proxy POST messages to the transport."""
    logger.debug(f"Proxying POST message for session {current_session_id.get()}")
    await sse.handle_post_message(scope, receive, send)

async def handle_healthz(request):
    """Liveness probe for load balancers and blue-green deploys."""
    return PlainTextResponse("ok")

# --- Streamable HTTP (/mcp): the current MCP transport, authenticated by API key ---

# Stateless: every request carries its own API key, so any instance (blue or
# green) can serve it and nothing is lost when a deploy swaps colors.
session_manager = StreamableHTTPSessionManager(app=server, json_response=False, stateless=True)


class StreamableHTTPApp:
    """ASGI endpoint for /mcp. Requires `Authorization: Bearer memro_sk_...`."""

    async def __call__(self, scope, receive, send):
        token = current_auth_info.get().get("token") or ""
        if not token.startswith(API_KEY_PREFIX):
            return await _unauthorized("Send Authorization: Bearer <memro_sk_ API key>. Run `npx memro-cli connect` to get one.")(scope, receive, send)
        _, error = await _authenticate_request()
        if error is not None:
            return await error(scope, receive, send)
        await session_manager.handle_request(scope, receive, send)


@contextlib.asynccontextmanager
async def lifespan(app):
    async with session_manager.run():
        yield


# Final App Assembly
_base_app = Starlette(
    lifespan=lifespan,
    # Debug pages leak stack traces; enable only for local development.
    debug=os.getenv("MCP_DEBUG") == "1",
    routes=[
        Route("/healthz", endpoint=handle_healthz, methods=["GET"]),
        Route("/mcp", endpoint=StreamableHTTPApp(), methods=["GET", "POST", "DELETE"]),
        Mount("/sse/messages", app=handle_messages),
        Mount("/sse", app=handle_sse),
    ]
)
starlette_app = AuthMiddleware(_base_app)

def main():
    transport_mode = os.getenv("MCP_TRANSPORT", "stdio").lower()
    if transport_mode == "sse":
        port = int(os.getenv("MCP_PORT", "8080"))
        workers = int(os.getenv("MCP_WORKERS", "1"))
        loop = "uvloop" if uvloop_available else "auto"
        # uvicorn.run manages its own event loop
        uvicorn.run("memro_mcp.server:starlette_app", host="0.0.0.0", port=port, workers=workers, loop=loop)
    else:
        # stdio requires an active loop for the context manager
        async def run_stdio():
            async with stdio_server() as (read, write):
                await server.run(read, write, server.create_initialization_options())
        
        if uvloop_available:
            uvloop.install()
        asyncio.run(run_stdio())

if __name__ == "__main__":
    main()
