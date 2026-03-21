"""
High-level Agent interface for the memro protocol.
"""
import os
import json
from typing import Optional, List, Dict, Any

from .client import MemroClient
from .crypto import generate_keypair
from .models import Memory, SearchResult, ExportData, TemporalSearchResult


class Agent:
    """
    An AI agent with persistent, self-owned memory.

    Quickstart:
        agent = Agent.create()
        agent.remember("User prefers dark mode", type="profile")
        results = agent.recall("user preferences")
        agent.save("identity.json")

    Load existing:
        agent = Agent.from_file("identity.json")
        agent = Agent.from_env()   # MEMRO_AGENT_ID + MEMRO_PRIVATE_KEY
    """

    def __init__(
        self,
        agent_id: str,
        private_key: str,
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
    def from_env(cls, client: Optional[MemroClient] = None) -> "Agent":
        """
        Load agent from environment variables.
        Requires: MEMRO_AGENT_ID, MEMRO_PRIVATE_KEY
        """
        agent_id = os.environ.get("MEMRO_AGENT_ID")
        private_key = os.environ.get("MEMRO_PRIVATE_KEY")
        if not agent_id or not private_key:
            raise EnvironmentError(
                "MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY must be set"
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
        """Save agent identity to a JSON file. Keep this file secret."""
        with open(path, "w") as f:
            json.dump(
                {"agent_id": self.agent_id, "private_key": self._private_key},
                f,
                indent=2,
            )

    def to_dict(self) -> Dict[str, str]:
        return {"agent_id": self.agent_id, "private_key": self._private_key}

    # ── Memory operations ───────────────────────────────────────────────────

    def remember(
        self,
        content: str,
        type: str = "episodic",
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
            type:       episodic | semantic | procedural | profile
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

    def delete_agent(self) -> None:
        """Permanently delete this agent and all its memories."""
        self.client.delete_identity(self.agent_id, self._private_key)

    def __repr__(self) -> str:
        return f"Agent(id={self.agent_id[:16]}...)"
