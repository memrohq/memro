"""
Low-level HTTP client for the memro protocol API.
"""
import os
import json
from typing import Optional, List, Tuple

import requests
from requests import Response

from .crypto import sign_body, generate_keypair


class MemroError(Exception):
    """Raised when the memro API returns an error."""
    def __init__(self, status_code: int, message: str):
        self.status_code = status_code
        super().__init__(f"[{status_code}] {message}")


class MemroClient:
    """
    HTTP client for the memro backend API.

    Usage:
        client = MemroClient()                          # uses MEMRO_BASE_URL or localhost:8081
        client = MemroClient("https://api.memro.co")   # custom server
    """

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = (
            base_url
            or os.getenv("MEMRO_BASE_URL", "http://localhost:8081")
        ).rstrip("/")
        self._session = requests.Session()

    def _raise(self, resp: Response) -> None:
        if not resp.ok:
            try:
                msg = resp.text
            except Exception:
                msg = str(resp.status_code)
            raise MemroError(resp.status_code, msg)

    def _auth_headers(self, agent_id: str, private_key: str, body: bytes) -> dict:
        headers = sign_body(private_key, body)
        headers["X-Agent-Id"] = agent_id
        headers["Content-Type"] = "application/json"
        return headers

    # ── Identity ────────────────────────────────────────────────────────────

    def create_identity(self) -> dict:
        """
        Generate a keypair client-side and register the public key with the server.
        Returns {"agent_id": str, "private_key": str}.
        The private key is generated locally and never sent to the server.
        """
        public_key, private_key = generate_keypair()
        body = json.dumps({"public_key": public_key}).encode()
        resp = self._session.post(
            f"{self.base_url}/identity",
            data=body,
            headers={"Content-Type": "application/json"},
        )
        self._raise(resp)
        data = resp.json()
        # Augment response with the locally-generated private key
        data["private_key"] = private_key
        return data

    def get_identity(self, agent_id: str) -> dict:
        resp = self._session.get(f"{self.base_url}/identity/{agent_id}")
        self._raise(resp)
        return resp.json()

    def delete_identity(self, agent_id: str, private_key: str) -> None:
        body = agent_id.encode()
        headers = self._auth_headers(agent_id, private_key, body)
        resp = self._session.delete(
            f"{self.base_url}/identity/{agent_id}", headers=headers
        )
        self._raise(resp)

    # ── Memory ──────────────────────────────────────────────────────────────

    def create_memory(
        self,
        agent_id: str,
        private_key: str,
        content: str,
        memory_type: str = "episodic",
        visibility: str = "private",
        event_date: Optional[str] = None,
        session_id: Optional[str] = None,
        is_atomic: bool = False,
        metadata: Optional[dict] = None,
    ) -> dict:
        body_dict = {
            "agent_id": agent_id,
            "content": content,
            "memory_type": memory_type,
            "visibility": visibility,
            "event_date": event_date,
            "session_id": session_id,
            "is_atomic": is_atomic,
        }
        if metadata:
            body_dict["metadata"] = metadata
        
        body = json.dumps(body_dict).encode()
        headers = self._auth_headers(agent_id, private_key, body)
        resp = self._session.post(f"{self.base_url}/memory", data=body, headers=headers)
        self._raise(resp)
        return resp.json()

    def get_memories(
        self,
        agent_id: str,
        private_key: Optional[str] = None,
        memory_type: Optional[str] = None,
        visibility: Optional[str] = None,
        limit: Optional[int] = None,
    ) -> List[dict]:
        """
        Retrieve memories for an agent.

        When private_key is provided the request is authenticated and all
        memories (including private/shared) are returned.  Without a key only
        public memories are returned.
        """
        params: dict = {}
        if memory_type:
            params["memory_type"] = memory_type
        if visibility:
            params["visibility"] = visibility
        if limit is not None:
            params["limit"] = limit

        headers: dict = {}
        if private_key:
            # For GET requests with no body, the signature payload is TIMESTAMP + b""
            headers = self._auth_headers(agent_id, private_key, b"")

        resp = self._session.get(
            f"{self.base_url}/memory/agent/{agent_id}", params=params, headers=headers
        )
        self._raise(resp)
        return resp.json()

    def delete_memory(self, agent_id: str, private_key: str, memory_id: str) -> bool:
        body = memory_id.encode()
        headers = self._auth_headers(agent_id, private_key, body)
        resp = self._session.delete(
            f"{self.base_url}/memory/{memory_id}", headers=headers
        )
        return resp.status_code == 204

    # ── Search ──────────────────────────────────────────────────────────────

    def search(
        self,
        agent_id: str,
        query: str,
        limit: int = 10,
        private_key: Optional[str] = None,
        memory_type: Optional[str] = None,
        mode: str = "hybrid",
    ) -> List[dict]:
        params: dict = {
            "agent_id": agent_id, 
            "query": query, 
            "limit": min(limit, 50),
            "mode": mode
        }
        if memory_type:
            params["memory_type"] = memory_type

        headers: dict = {}
        if private_key:
            # For GET requests with no body, the signature payload is TIMESTAMP + b""
            headers = self._auth_headers(agent_id, private_key, b"")

        resp = self._session.get(f"{self.base_url}/search", params=params, headers=headers)
        self._raise(resp)
        return resp.json().get("results", [])

    def search_temporal(
        self,
        agent_id: str,
        private_key: str,
        query: str,
        limit: int = 10,
        after_date: Optional[str] = None,
        before_date: Optional[str] = None,
        after_event: Optional[str] = None,
        before_event: Optional[str] = None,
        session_id: Optional[str] = None,
        atomic_only: bool = False,
        temporal_weight: float = 0.3,
    ) -> dict:
        params = {
            "agent_id": agent_id,
            "query": query,
            "limit": limit,
            "after_date": after_date,
            "before_date": before_date,
            "after_event_date": after_event,
            "before_event_date": before_event,
            "session_id": session_id,
            "atomic_only": atomic_only,
            "temporal_weight": temporal_weight,
        }
        # Filter out None values
        params = {k: v for k, v in params.items() if v is not None}
        
        headers = self._auth_headers(agent_id, private_key, b"")
        resp = self._session.get(f"{self.base_url}/search/temporal", params=params, headers=headers)
        self._raise(resp)
        return resp.json()

    # ── Export ──────────────────────────────────────────────────────────────

    def export(self, agent_id: str, private_key: str) -> dict:
        """Export all memories. Requires authentication."""
        # For GET requests with no body, the signature payload is TIMESTAMP + b""
        headers = self._auth_headers(agent_id, private_key, b"")
        resp = self._session.get(f"{self.base_url}/export/{agent_id}", headers=headers)
        self._raise(resp)
        return resp.json()

    # ── Health ──────────────────────────────────────────────────────────────

    def health(self) -> dict:
        resp = self._session.get(f"{self.base_url}/health")
        self._raise(resp)
        return resp.json()
