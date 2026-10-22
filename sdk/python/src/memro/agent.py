"""
High-level Agent interface for the memro protocol.
"""
import os
import json
from typing import Optional, List, Dict, Any

from .client import MemroClient
from .crypto import generate_keypair
from .models import (
    Memory, SearchResult, ExportData, TemporalSearchResult, ReasoningResult,
    EvolutionResult, MemoryTimeline, MemoryChain, TemporalStats, UploadedFile,
)


class Agent:
    """
    An AI agent with persistent, self-owned memory.

    Memro Cloud (API key from `npx memro-cli connect` or the dashboard):
        agent = Agent.from_api_key("memro_sk_...")
        agent = Agent.from_env()            # MEMRO_API_KEY

    Self-owned identity (ed25519 key generated on this machine):
        agent = Agent.create()
        agent.save("identity.json")         # keep this file secret
        agent = Agent.from_file("identity.json")

    Then:
        agent.remember("User prefers dark mode", type="profile")
        results = agent.recall("user preferences")
    """

    def __init__(
        self,
        agent_id: str,
        private_key: Optional[str] = None,
        client: Optional[MemroClient] = None,
    ):
        self.agent_id = agent_id
        self._private_key = private_key
        self.client = client or MemroClient()

    # ── Constructors ────────────────────────────────────────────────────────

    @classmethod
    def create(cls, client: Optional[MemroClient] = None) -> "Agent":
        """Create a new agent identity on the memro server."""
        c = client or MemroClient()
        resp = c.create_identity()
        return cls(agent_id=resp["agent_id"], private_key=resp["private_key"], client=c)

    @classmethod
    def from_file(cls, path: str, client: Optional[MemroClient] = None) -> "Agent":
        """Load agent identity from a JSON file."""
        with open(path, "r") as f:
            data = json.load(f)
        return cls(
            agent_id=data["agent_id"],
            private_key=data["private_key"],
            client=client,
        )

    @classmethod
    def from_api_key(cls, api_key: str, client: Optional[MemroClient] = None) -> "Agent":
        """Use an API key (memro_sk_...) from the dashboard or `memro connect`."""
        c = client or MemroClient(api_key=api_key)
        c.api_key = api_key
        return cls(agent_id=c.whoami()["agent_id"], client=c)

    @classmethod
    def connect(cls, api_key: Optional[str] = None, base_url: Optional[str] = None) -> "Agent":
        """Shortcut for Agent.from_api_key with an optional self-hosted base URL."""
        key = api_key or os.environ.get("MEMRO_API_KEY")
        if not key:
            raise EnvironmentError("Pass an API key or set MEMRO_API_KEY")
        return cls.from_api_key(key, client=MemroClient(base_url, api_key=key))

    @classmethod
    def from_env(cls, client: Optional[MemroClient] = None) -> "Agent":
        """
        Load agent from environment variables:
        MEMRO_API_KEY, or MEMRO_AGENT_ID + MEMRO_PRIVATE_KEY.
        """
        api_key = os.environ.get("MEMRO_API_KEY")
        if api_key:
            return cls.from_api_key(api_key, client=client)
        agent_id = os.environ.get("MEMRO_AGENT_ID")
        private_key = os.environ.get("MEMRO_PRIVATE_KEY")
        if not agent_id or not private_key:
            raise EnvironmentError(
                "Set MEMRO_API_KEY, or MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY"
            )
        return cls(agent_id=agent_id, private_key=private_key, client=client)

    @classmethod
    def from_dict(cls, data: Dict[str, str], client: Optional[MemroClient] = None) -> "Agent":
        """Load agent from a dict with agent_id and private_key."""
        return cls(
            agent_id=data["agent_id"],
            private_key=data["private_key"],
            client=client,
        )

    # ── Persistence ─────────────────────────────────────────────────────────

    def save(self, path: str) -> None:
        """Save agent identity to a JSON file (mode 0600). Keep this file secret."""
        flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
        fd = os.open(path, flags, 0o600)
        with open(fd, "w") as f:
            json.dump(
                {"agent_id": self.agent_id, "private_key": self._private_key},
                f,
                indent=2,
            )
            f.write("\n")

    def to_dict(self) -> Dict[str, str]:
        return {"agent_id": self.agent_id, "private_key": self._private_key}

    # ── Memory operations ───────────────────────────────────────────────────

    def remember(
        self,
        content: str,
        type: Optional[str] = None,
        visibility: str = "private",
        event_date: Optional[str] = None,
        session_id: Optional[str] = None,
        is_atomic: bool = False,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> Memory:
        """
        Store a new memory with optional temporal grounding.

        Args:
            content:    What to remember (max 10,000 chars)
            type:       episodic | semantic | procedural | profile, or None to
                        let the server classify it
            visibility: private | shared | public
            event_date: When the event occurred (ISO 8601)
            session_id: Session identifier for multi-session reasoning
            is_atomic:  Whether this is an atomic fact vs raw chunk
            metadata:   Optional dict of extra context
        """
        data = self.client.create_memory(
            agent_id=self.agent_id,
            private_key=self._private_key,
            content=content,
            memory_type=type,
            visibility=visibility,
            event_date=event_date,
            session_id=session_id,
            is_atomic=is_atomic,
            metadata=metadata,
        )
        return Memory(**data)

    def recall(
        self,
        query: str,
        limit: int = 10,
        type: Optional[str] = None,
        min_score: Optional[float] = None,
        mode: str = "hybrid",
    ) -> List[SearchResult]:
        """
        Search memories semantically or via hybrid retrieval.
        """
        raw = self.client.search(
            agent_id=self.agent_id,
            private_key=self._private_key,
            query=query,
            limit=limit,
            memory_type=type,
            mode=mode,
        )
        results = [SearchResult(**r) for r in raw]
        if min_score is not None:
            results = [r for r in results if r.score >= min_score]
        return results

    def recall_temporal(
        self,
        query: str,
        limit: int = 10,
        after_date: Optional[str] = None,
        before_date: Optional[str] = None,
        after_event: Optional[str] = None,
        before_event: Optional[str] = None,
        session_id: Optional[str] = None,
        atomic_only: bool = False,
        temporal_weight: float = 0.3,
    ) -> List[TemporalSearchResult]:
        """
        Perform temporal search with date-based filtering and recency weighting.
        """
        raw = self.client.search_temporal(
            agent_id=self.agent_id,
            private_key=self._private_key,
            query=query,
            limit=limit,
            after_date=after_date,
            before_date=before_date,
            after_event=after_event,
            before_event=before_event,
            session_id=session_id,
            atomic_only=atomic_only,
            temporal_weight=temporal_weight,
        )
        return [TemporalSearchResult(**r) for r in raw.get("results", [])]

    def get_recent(
        self,
        limit: int = 10,
        type: Optional[str] = None,
        visibility: Optional[str] = None,
    ) -> List[Memory]:
        """Get most recent memories without semantic search."""
        raw = self.client.get_memories(
            agent_id=self.agent_id,
            private_key=self._private_key,
            memory_type=type,
            visibility=visibility,
            limit=limit,
        )
        return [Memory(**m) for m in raw]

    def delete_memory(self, memory_id: str) -> bool:
        """Delete a specific memory by ID. Returns True if deleted."""
        return self.client.delete_memory(
            agent_id=self.agent_id,
            private_key=self._private_key,
            memory_id=memory_id,
        )

    def export(self) -> ExportData:
        """Export all memories for this agent."""
        data = self.client.export(self.agent_id, self._private_key)
        return ExportData(**data)

    # ── Memory engine ───────────────────────────────────────────────────────

    def reason(self, query: str) -> ReasoningResult:
        """Answer a question from memory, with the memories and paths it used."""
        return ReasoningResult(**self.client.reason(self.agent_id, self._private_key, query))

    def resolve_conflict(self, memory_a_id: str, memory_b_id: str) -> dict:
        """{"Resolved": id} or {"Ambiguous": [a, b, expires_at]}."""
        return self.client.resolve_conflict(self.agent_id, self._private_key, memory_a_id, memory_b_id)

    def evolve(self) -> EvolutionResult:
        """Consolidate repeated episodes into semantic memories."""
        return EvolutionResult(**self.client.evolve(self.agent_id, self._private_key))

    def timeline(self, memory_id: str) -> MemoryTimeline:
        return MemoryTimeline(**self.client.timeline(self.agent_id, self._private_key, memory_id))

    def memory_chain(self, memory_id: str) -> MemoryChain:
        return MemoryChain(**self.client.memory_chain(self.agent_id, self._private_key, memory_id))

    def stats(self) -> TemporalStats:
        return TemporalStats(**self.client.stats(self.agent_id, self._private_key))

    def usage(self) -> dict:
        return self.client.usage(self.agent_id, self._private_key)

    # ── Files ───────────────────────────────────────────────────────────────

    def upload_file(self, file, remember: bool = True, type: str = "semantic") -> UploadedFile:
        """Upload a file (path or (name, bytes[, content_type])) and, by default,
        store its extracted text as a memory."""
        up = UploadedFile(**self.client.upload_file(self.agent_id, self._private_key, file))
        if remember:
            res = self.client.memory_from_file(self.agent_id, self._private_key, up.file_id, memory_type=type)
            up.memory_id = res.get("memory_id")
        return up

    def upload_image(self, file) -> Memory:
        """Describe an image with vision and store the description."""
        return Memory(**self.client.upload_image(self.agent_id, self._private_key, file))

    def upload_document(self, file) -> List[Memory]:
        """Store a PDF, Word, PowerPoint, Excel or text document as ~1,500-character memories."""
        return [Memory(**m) for m in self.client.upload_document(self.agent_id, self._private_key, file)]

    # ── Webhooks ────────────────────────────────────────────────────────────

    def add_webhook(self, url: str, event_type: str = "memory_created") -> None:
        """POST each new memory of this agent to url (public http(s) only)."""
        self.client.register_webhook(self.agent_id, self._private_key, url, event_type)

    def remove_webhook(self, url: str, event_type: str = "memory_created") -> bool:
        return self.client.delete_webhook(self.agent_id, self._private_key, url, event_type)

    def webhooks(self, event_type: str = "memory_created") -> List[str]:
        return self.client.list_webhooks(self.agent_id, self._private_key, event_type)

    def delete_agent(self) -> None:
        """Permanently delete this agent and all its memories.

        Needs the agent's private key; API keys cannot delete agents (403).
        """
        self.client.delete_identity(self.agent_id, self._private_key)

    # ── Coordination ────────────────────────────────────────────────────────

    def claim(
        self,
        resource: str,
        intent: str = "",
        session_id: Optional[str] = None,
        ttl_seconds: Optional[int] = None,
    ) -> "ClaimContext":
        """Claim a resource for multi-agent coordination.
        Can be used as a context manager:
            with agent.claim("src/auth.go", intent="refactor") as lease:
                ...
        Or accessed as a dict:
            res = agent.claim("src/auth.go")
            if res["granted"]: ...
        """
        raw = self.client.coordination_claim(
            self.agent_id, self._private_key, resource, intent=intent, session_id=session_id, ttl_seconds=ttl_seconds
        )
        return ClaimContext(self, resource, raw, ttl_seconds)

    def heartbeat(
        self,
        resource: Optional[str] = None,
        lease_id: Optional[str] = None,
        ttl_seconds: Optional[int] = None,
        session_id: Optional[str] = None,
    ) -> dict:
        return self.client.coordination_heartbeat(
            self.agent_id, self._private_key, resource=resource, lease_id=lease_id, ttl_seconds=ttl_seconds,
            session_id=session_id,
        )

    def release(
        self,
        resource: Optional[str] = None,
        lease_id: Optional[str] = None,
        status: str = "completed",
        session_id: Optional[str] = None,
    ) -> dict:
        return self.client.coordination_release(
            self.agent_id, self._private_key, resource=resource, lease_id=lease_id, status=status, session_id=session_id
        )

    def handoff(
        self,
        resource: Optional[str] = None,
        lease_id: Optional[str] = None,
        summary: str = "",
        next_agent_id: Optional[str] = None,
        session_id: Optional[str] = None,
    ) -> dict:
        return self.client.coordination_handoff(
            self.agent_id, self._private_key, resource=resource, lease_id=lease_id, summary=summary,
            next_agent_id=next_agent_id, session_id=session_id,
        )

    def check(self, resource: str, intent: str = "", session_id: Optional[str] = None) -> dict:
        return self.client.coordination_check(
            self.agent_id, self._private_key, resource=resource, intent=intent, session_id=session_id
        )

    def active(self, prefix: Optional[str] = None) -> dict:
        return self.client.coordination_active(self.agent_id, self._private_key, resource=prefix)

    def log_intent(
        self, resource: str, intent: str, ttl_seconds: Optional[int] = None, session_id: Optional[str] = None
    ) -> dict:
        return self.client.coordination_intent(
            self.agent_id, self._private_key, resource=resource, intent=intent, ttl_seconds=ttl_seconds,
            session_id=session_id,
        )

    def __repr__(self) -> str:
        return f"Agent(id={self.agent_id[:16]}...)"


class ClaimContext(dict):
    """Result of agent.claim(...). Behaves as a dictionary, and supports context management:
    with agent.claim('src/auth.go'):
        # auto-heartbeats in background, auto-releases on exit
    """

    def __init__(self, agent: "Agent", resource: str, result: dict, ttl_seconds: Optional[int] = None):
        super().__init__(result)
        self.agent = agent
        self.resource = resource
        self.ttl_seconds = ttl_seconds or 900
        self._stop_heartbeat = None
        self._thread = None

    def __enter__(self):
        if not self.get("granted"):
            raise RuntimeError(f"Resource '{self.resource}' could not be claimed: held by {self.get('holder')}")
        import threading

        self._stop_heartbeat = threading.Event()

        def _beat():
            interval = max(5, self.ttl_seconds // 3)
            while not self._stop_heartbeat.wait(interval):
                try:
                    self.agent.heartbeat(lease_id=self.get("lease_id"), resource=self.resource, ttl_seconds=self.ttl_seconds)
                except Exception:
                    pass

        self._thread = threading.Thread(target=_beat, daemon=True)
        self._thread.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if self._stop_heartbeat:
            self._stop_heartbeat.set()
        status = "abandoned" if exc_type is not None else "completed"
        try:
            self.agent.release(lease_id=self.get("lease_id"), resource=self.resource, status=status)
        except Exception:
            pass
        return False

