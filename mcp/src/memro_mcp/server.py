import os
import sys
import asyncio
import json
import logging
import uuid
import re
from contextvars import ContextVar
from dotenv import load_dotenv
from urllib.parse import parse_qs

# Load local .env if it exists
load_dotenv()
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.server.sse import SseServerTransport
from mcp.types import Tool, TextContent, CallToolResult, JSONRPCNotification
from memro import Agent, MemroClient, MemroError
from memro_mcp.knowledge_graph import get_graph_for_agent
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.routing import Route, Mount
from starlette.responses import Response, JSONResponse
import uvicorn
from memro_mcp.webhook_handler import process_webhook_event

try:
    import uvloop
    uvloop_available = True
except ImportError:
    uvloop_available = False

# --- Core Objects (Module Level for Uvicorn Workers) ---
server = Server("memro-memory")
# Use a path that will be appended to our mount point
sse = SseServerTransport("/messages/")

# Context variables for request-scoped state
current_session_id: ContextVar[str] = ContextVar("session_id", default="default")
current_auth_info: ContextVar[dict] = ContextVar("auth_info", default={})

# In SaaS/Managed mode, we store agents per session
_session_agents = {}

# Global logging setup
logging.basicConfig(
    filename='/tmp/memro_mcp_debug.log',
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    filemode='a'
)
logger = logging.getLogger("memro_mcp")

# --- Coordination Engine (The Brain) ---

class Decision:
    ALLOW = "allow"
    WARN = "warn"
    BLOCK = "block"

class CoordinationEngine:
    def __init__(self, agent: Agent, session_id: str):
        self.agent = agent
        self.session_id = session_id

    def _get_active_claims(self, resource: str = None):
        """Fetch active procedural memories for claims/intents scoped to session."""
        results = self.agent.get_recent(limit=50) 
        active_claims = []
        active_intents = []
        
        for m in results:
            if m.memory_type != "procedural": continue
            meta = getattr(m, 'metadata', {})
            
            # SESSION ISOLATION: Skip memories from other sessions
            if meta.get("session_id") != self.session_id:
                continue

            action = meta.get("action")
            res = meta.get("resource")
            agent_owner = meta.get("agent_id", "Unknown")
            
            if action == "claim" and meta.get("status") == "active":
                if not resource or res == resource:
                    active_claims.append({
                        "agent": agent_owner, 
                        "resource": res, 
                        "reason": m.content,
                        "timestamp": getattr(m, 'event_date', 'just now')
                    })
            elif action == "intent":
                if not resource or res == resource:
                    active_intents.append({
                        "agent": agent_owner,
                        "intent": m.content,
                        "resource": res
                    })
        
        return active_claims, active_intents

    def can_act(self, resource: str, intent: str) -> dict:
        claims, intents = self._get_active_claims(resource)
        
        # 1. Check for hard blocks (Claims)
        if claims:
            return {
                "decision": Decision.BLOCK,
                "reason": f"Resource '{resource}' is currently claimed.",
                "details": claims[0]
            }
        
        # 2. Check for soft overlaps (Intent)
        # MVP: Simple keyword matching
        words = set(intent.lower().split())
        for existing_intent in intents:
            overlap = words.intersection(set(existing_intent.lower().split()))
            if len(overlap) > 2: # Threshold for "possible overlap"
                return {
                    "decision": Decision.WARN,
                    "reason": f"Potential intent overlap detected with: '{existing_intent}'",
                    "suggestions": ["Coordinate via chat", "Check related files"]
                }
        
        return {"decision": Decision.ALLOW}

# --- Helper Functions ---

def get_agent_for_session(session_id: str = None, agent_id: str = None, token: str = None):
    """Resolves the Agent with Stateless Auth priority."""
    global _session_agents
    
    if not agent_id or not token:
        auth = current_auth_info.get()
        agent_id = agent_id or auth.get("agent_id")
        token = token or auth.get("token")
    
    if session_id and session_id in _session_agents:
        cached_agent = _session_agents[session_id]
        if not agent_id or cached_agent.agent_id == agent_id:
            return cached_agent
        
    base_url = os.getenv("MEMRO_BASE_URL", "http://localhost:8081")
    client = MemroClient(base_url)
    
    if token and agent_id:
        clean_token = token[3:] if token.startswith("sk_") else token
        agent = Agent(agent_id=agent_id, private_key=clean_token, client=client)
        if session_id:
            _session_agents[session_id] = agent
        return agent
            
    env_agent_id = os.getenv("MEMRO_AGENT_ID")
    env_private_key = os.getenv("MEMRO_PRIVATE_KEY")
    
    if env_agent_id and env_private_key:
        agent = Agent.from_env(client=client)
    else:
        agent = Agent.create(client=client)
        
    if session_id:
        _session_agents[session_id] = agent
    return agent

# --- MCP Tool Handlers ---

from memro_mcp.supermemory_tools import get_supermemory_tools

@server.list_tools()
async def list_tools() -> list[Tool]:
    logger.info("Listing tools for client")
    return [
        Tool(
            name="remember",
            description="Store a new memory.",
            inputSchema={
                "type": "object",
                "properties": {
                    "content": {"type": "string"},
                    "type": {"type": "string", "enum": ["episodic", "semantic", "procedural", "profile"], "default": "episodic"},
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
            description="Search memories semantically.",
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
            description="Claim a file or task with intent to work on it.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string", "description": "File path or module"},
                    "intent": {"type": "string", "description": "What you plan to do"}
                },
                "required": ["resource", "intent"],
            },
        ),
        Tool(
            name="release_task",
            description="Release a claimed resource and log outcome.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "status": {"type": "string", "enum": ["completed", "cancelled", "handoff"], "default": "completed"}
                },
                "required": ["resource"],
            },
        ),
        Tool(
            name="log_intent",
            description="Declare your intention for a resource.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "intent": {"type": "string"}
                },
                "required": ["resource", "intent"],
            },
        ),
        Tool(
            name="handoff_task",
            description="Atomically summarizes work and releases a resource claim.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "summary": {"type": "string"},
                    "next_agent_id": {"type": "string"}
                },
                "required": ["resource", "summary"],
            },
        ),
        Tool(
            name="pre_action_check",
            description="Check for conflicts or overlaps before performing an action.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string"},
                    "intent": {"type": "string"}
                },
                "required": ["resource", "intent"],
            },
        ),
        Tool(
            name="delete_memory",
            description="Delete a specific memory by ID.",
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
            description="Get the most recent memories for the agent.",
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
                    "type": {"type": "string", "enum": ["episodic", "semantic", "procedural", "profile"], "default": "episodic"},
                },
                "required": ["content"],
            },
        ),
        Tool(
            name="get_active_work",
            description="Get ALL active claims and intents for the current session.",
            inputSchema={"type": "object", "properties": {}},
        ),
        Tool(
            name="get_active_context",
            description="Get coordination context (claims/intents) for a specific resource.",
            inputSchema={
                "type": "object",
                "properties": {
                    "resource": {"type": "string", "description": "File or resource to check"}
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

@server.call_tool()
async def call_tool(name: str, arguments: dict) -> CallToolResult:
    """Standard Tool Execution with Session/Agent scoping."""
    try:
        session_id = current_session_id.get()
        agent = get_agent_for_session(session_id=session_id)
        logger.info(f"Tool call: {name} | session={session_id} | agent={agent.agent_id}")
        logger.debug(f"Arguments: {arguments}")
        
        if name == "remember":
            content = arguments.get("content")
            m_type = arguments.get("type", "episodic")
            visibility = arguments.get("visibility", "private")
            memory = agent.remember(content=content, type=m_type, visibility=visibility)
            return CallToolResult(content=[TextContent(type="text", text=f"Remembered: {memory.content[:100]}... (ID: {memory.id})")])

        elif name == "recall":
            query = arguments.get("query")
            limit = arguments.get("limit", 5)
            m_type = arguments.get("type")
            results = agent.recall(query=query, limit=limit, type=m_type)
            if not results:
                return CallToolResult(content=[TextContent(type="text", text=json.dumps([]))])
            
            # Return structured list for DevTools
            serializable = [
                {
                    "id": r.memory_id,
                    "content": r.content,
                    "type": r.memory_type,
                    "score": r.score,
                    "metadata": getattr(r, 'metadata', {})
                } for r in results
            ]
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(serializable))])

        elif name == "delete_memory":
            mid = arguments.get("memory_id")
            success = agent.delete_memory(mid)
            return CallToolResult(content=[TextContent(type="text", text="Memory deleted." if success else "Failed to delete memory.")])

        elif name == "export_memories":
            data = agent.export()
            return CallToolResult(content=[TextContent(type="text", text=f"Exported {len(data.memories)} memories.")])

        elif name == "get_recent_memories":
            limit = arguments.get("limit", 10)
            m_type = arguments.get("type")
            results = agent.get_recent(limit=limit, type=m_type)
            if not results:
                return CallToolResult(content=[TextContent(type="text", text=json.dumps([]))])
            
            serializable = [
                {
                    "id": m.id,
                    "content": m.content,
                    "type": m.memory_type,
                    "metadata": getattr(m, 'metadata', {})
                } for m in results
            ]
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(serializable))])

        elif name == "search_temporal":
            params = {
                "agent_id": agent.agent_id,
                "query": arguments.get("query"),
                "limit": arguments.get("limit", 10),
                "after_date": arguments.get("after_date"),
                "before_date": arguments.get("before_date"),
                "temporal_weight": arguments.get("temporal_weight", 0.3),
                "atomic_only": arguments.get("atomic_only"),
            }
            params = {k: v for k, v in params.items() if v is not None}
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.get(f"{agent.client.base_url}/search/temporal", params=params, headers=headers)
                resp.raise_for_status()
                data = resp.json()
            results = data.get("results", [])
            output = "\n".join([f"- {r['content']} ({r['temporal_context']})" for r in results])
            return CallToolResult(content=[TextContent(type="text", text=f"Temporal Search Results:\n{output}")])

        elif name == "get_temporal_stats":
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.get(f"{agent.client.base_url}/stats/temporal", headers=headers)
                resp.raise_for_status()
                stats = resp.json()
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(stats, indent=2))])

        elif name == "remember_with_context":
            content = arguments.get("content")
            memory = agent.remember(
                content=content, 
                type=arguments.get("type", "episodic"),
                event_date=arguments.get("event_date")
            )
            return CallToolResult(content=[TextContent(type="text", text=f"Remembered with context: {memory.content[:100]}... (ID: {memory.id})")])

        elif name == "generate_atomic_facts":
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.post(
                    f"{agent.client.base_url}/memory/generate-atoms", 
                    json={"chunk": arguments.get("text"), "context": arguments.get("context")},
                    headers=headers
                )
                resp.raise_for_status()
                data = resp.json()
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(data, indent=2))])

        elif name == "mee_reason":
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.post(
                    f"{agent.client.base_url}/mee/reason", 
                    json={"query": arguments.get("query")},
                    headers=headers
                )
                resp.raise_for_status()
                data = resp.json()
                # Ensure structure for Reasoning Inspector
                payload = {
                    "answer": data.get("answer", ""),
                    "confidence": data.get("confidence", 0.0),
                    "sources": data.get("sources", []), # List of source memories
                    "logic": data.get("reasoning_path", "Hybrid semantic/recency inference")
                }
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(payload))])

        elif name == "mee_resolve":
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.post(
                    f"{agent.client.base_url}/mee/resolve", 
                    json={"memory_a_id": arguments.get("memory_a_id"), "memory_b_id": arguments.get("memory_b_id")},
                    headers=headers
                )
                resp.raise_for_status()
                data = resp.json()
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(data, indent=2))])

        elif name == "mee_evolve":
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.post(f"{agent.client.base_url}/mee/evolve", headers=headers)
                resp.raise_for_status()
                data = resp.json()
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(data, indent=2))])

        elif name == "get_timeline":
            mid = arguments.get("memory_id")
            headers = agent.client._auth_headers(agent.agent_id, agent._private_key, b"")
            async with httpx.AsyncClient() as client:
                resp = await client.get(f"{agent.client.base_url}/mee/timeline/{mid}", headers=headers)
                resp.raise_for_status()
                data = resp.json()
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(data, indent=2))])

        elif name == "pre_action_check":
            engine = CoordinationEngine(agent, session_id)
            decision = engine.can_act(arguments.get("resource"), arguments.get("intent"))
            return CallToolResult(content=[TextContent(type="text", text=json.dumps(decision))])

        elif name == "claim_task":
            resource = arguments.get("resource")
            intent = arguments.get("intent")
            engine = CoordinationEngine(agent, session_id)
            
            # Auto-check before claim
            check = engine.can_act(resource, intent)
            if check["decision"] == Decision.BLOCK:
                return CallToolResult(isError=True, content=[TextContent(type="text", text=f"Block: {check['reason']}")])
            
            agent.remember(
                content=f"[CLAIMED] {resource}: {intent}",
                type="procedural",
                metadata={
                    "action": "claim", 
                    "resource": resource, 
                    "status": "active",
                    "session_id": session_id,
                    "agent_id": agent.agent_id
                }
            )
            return CallToolResult(content=[TextContent(type="text", text=f"Resource '{resource}' claimed successfully in session '{session_id}'.")])

        elif name == "release_task":
            resource = arguments.get("resource")
            status = arguments.get("status", "completed")
            agent.remember(
                content=f"[RELEASED] {resource} ({status})",
                type="procedural",
                metadata={
                    "action": "release", 
                    "resource": resource, 
                    "status": status,
                    "session_id": session_id,
                    "agent_id": agent.agent_id
                }
            )
            return CallToolResult(content=[TextContent(type="text", text=f"Resource '{resource}' released.")])

        elif name == "log_intent":
            intent = arguments.get("intent")
            resource = arguments.get("resource")
            agent.remember(
                content=f"[INTENT] {resource}: {intent}",
                type="procedural",
                metadata={
                    "action": "intent", 
                    "resource": resource,
                    "session_id": session_id,
                    "agent_id": agent.agent_id
                }
            )
            return CallToolResult(content=[TextContent(type="text", text=f"Intent for {resource} logged.")])

        elif name == "get_active_work":
            engine = CoordinationEngine(agent, session_id)
            claims, intents = engine._get_active_claims(None)
            return CallToolResult(content=[TextContent(type="text", text=json.dumps({"claims": claims, "intents": intents}))])

        elif name == "get_active_context":
            engine = CoordinationEngine(agent, session_id)
            resource = arguments.get("resource")
            claims, intents = engine._get_active_claims(resource)
            return CallToolResult(content=[TextContent(type="text", text=json.dumps({"claims": claims, "intents": intents}))])

        elif name == "handoff_task":
            resource = arguments.get("resource")
            summary = arguments.get("summary")
            next_agent_id = arguments.get("next_agent_id")
            
            agent.remember(
                content=f"[HANDOFF] from {agent.agent_id}: {summary}",
                type="procedural",
                metadata={
                    "action": "handoff",
                    "resource": resource,
                    "from": agent.agent_id,
                    "to": next_agent_id or "Anyone",
                    "session_id": session_id
                }
            )
            
            # Release the claim
            agent.remember(
                content=f"[RELEASED] {resource} (handoff)",
                type="procedural",
                metadata={
                    "action": "release", 
                    "resource": resource, 
                    "status": "handoff",
                    "session_id": session_id,
                    "agent_id": agent.agent_id
                }
            )
            
            return CallToolResult(content=[TextContent(type="text", text=f"Handoff completed for {resource}.")])

        elif name == "get_system_status":
            return CallToolResult(content=[TextContent(type="text", text=json.dumps({
                "status": "online",
                "version": "0.1.0",
                "engine": "CoordinationEngine active",
                "mcp_version": "1.0.0"
            }))])

        elif name in ["recall_multi_session", "get_memory_chain", "find_contradictions", "analyze_conflicts"]:
            # These are legacy aliases or in-prog, handle gracefully
            msg = f"Tool '{name}' is being merged into the newer MEE toolset. Please use 'mee_reason', 'search_temporal', or 'get_timeline' for best results."
            return CallToolResult(content=[TextContent(type="text", text=msg)])

        return CallToolResult(isError=True, content=[TextContent(type="text", text=f"Unknown tool: {name}")])

    except Exception as e:
        logger.exception("Error in call_tool")
        return CallToolResult(isError=True, content=[TextContent(type="text", text=f"Tool execution failed: {str(e)}")])

# --- ASGI / SSE Application Logic ---

import httpx
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
        
        agent_id = params.get("agent_id", [None])[0]
        token = params.get("token", [None])[0]
        session_id = params.get("session_id", [None])[0]
        
        if not agent_id or not token:
            request = Request(scope, receive)
            agent_id = agent_id or request.query_params.get("agent_id")
            token = token or request.query_params.get("token") or request.headers.get("Authorization", "")[7:]

        logger.debug(f"AuthMiddleware: {scope['method']} {path} | agent={agent_id} session={session_id}")

        t_sid = current_session_id.set(session_id or "default")
        t_auth = current_auth_info.set({"agent_id": agent_id, "token": token})
        
        try:
            await self.app(scope, receive, send)
        finally:
            current_auth_info.reset(t_auth)
            current_session_id.reset(t_sid)

# --- ASGI SSE Handler ---

async def handle_sse(scope, receive, send):
    """Refined SSE stream handler with heartbeats."""
    auth = current_auth_info.get()
    agent_id = auth.get("agent_id")
    token = auth.get("token")

    async def wrapped_send(message):
        if message.get("type") == "http.response.body":
            body = message.get("body", b"").decode(errors="ignore")
            if "event: endpoint" in body:
                # Use a cleaner endpoint path
                auth_params = f"&agent_id={agent_id or 'default'}&token={token or ''}"
                # The data portion usually looks like 'data: /messages/?session_id=...'
                # We want it to be '/sse/messages/?session_id=...'
                new_body = re.sub(r"data: /messages/", "data: /sse/messages/", body)
                new_body = re.sub(r"(data: [^\n\s?]+(\?[^\n\s]+)?)", r"\1" + auth_params, new_body)
                message["body"] = new_body.encode()
                logger.debug(f"Sent endpoint event: {new_body.strip()}")
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

async def handle_webhook(request):
    agent_id = request.path_params.get("agent_id")
    try:
        payload = await request.json()
    except Exception:
        payload = {}
    
    result = process_webhook_event(agent_id, payload)
    return JSONResponse(result)

# Final App Assembly
_base_app = Starlette(
    debug=True,
    routes=[
        Mount("/sse/messages", app=handle_messages),
        Mount("/sse", app=handle_sse),
        Route("/webhook/{agent_id}", endpoint=handle_webhook, methods=["POST"]),
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
