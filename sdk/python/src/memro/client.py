"""
Low-level HTTP client for the Memro API.

Every method maps to one endpoint (see docs/api-reference.md). Most code
should use :class:`memro.Agent`, which keeps the credentials for you.
"""
import json
import mimetypes
import os
import time
from typing import Any, Dict, List, Optional, Tuple, Union
from urllib.parse import urlencode

import requests
from requests import Response

from .crypto import generate_keypair, sign_body, sign_request, solve_pow

DEFAULT_BASE_URL = "https://api.memro.co"
API_KEY_PREFIX = "memro_sk_"

FileInput = Union[str, os.PathLike, Tuple[str, bytes], Tuple[str, bytes, str]]


class MemroError(Exception):
    """Raised when the Memro API returns an error."""

    def __init__(self, status_code: int, message: str):
        self.status_code = status_code
        self.message = message
        super().__init__(f"[{status_code}] {message}")


class RateLimitError(MemroError):
    """HTTP 429: slow down and retry later."""


class MemroClient:
    """
    HTTP client for the Memro API.

        MemroClient(api_key="memro_sk_...")             # Memro Cloud
        MemroClient("http://localhost:8081")            # self-hosted, signed requests
        MemroClient()                                   # MEMRO_BASE_URL / MEMRO_API_KEY from env

    Authentication is either an API key (``Authorization: Bearer``) or an
    ed25519 signature made with the agent's private key, passed per call.
    """

    def __init__(
        self,
        base_url: Optional[str] = None,
        api_key: Optional[str] = None,
        timeout: float = 30.0,
        max_retries: int = 2,
    ):
        self.base_url = (base_url or os.getenv("MEMRO_BASE_URL") or DEFAULT_BASE_URL).rstrip("/")
        self.api_key = api_key or os.getenv("MEMRO_API_KEY") or None
        if self.api_key and not self.api_key.startswith(API_KEY_PREFIX):
            raise ValueError("API keys start with memro_sk_. Create one with `npx memro-cli connect` or in the dashboard.")
        self.timeout = timeout
        self.max_retries = max_retries
        self._session = requests.Session()

    # ── Plumbing ────────────────────────────────────────────────────────────

    def _headers(self, method: str, path_and_query: str, body: bytes,
                 agent_id: Optional[str], private_key: Optional[str]) -> Dict[str, str]:
        """Authentication headers for one request. Override to add headers."""
        if self.api_key:
            return {"Authorization": f"Bearer {self.api_key}"}
        if agent_id and private_key:
            headers = sign_request(private_key, method, path_and_query, body)
            headers["X-Agent-Id"] = agent_id
            return headers
        return {}

    def _auth_headers(self, agent_id: str, private_key: str, body: bytes) -> dict:
        """Deprecated: legacy v1 signing headers. Use the client methods instead."""
        if self.api_key:
            return {"Authorization": f"Bearer {self.api_key}", "Content-Type": "application/json"}
        headers = sign_body(private_key, body)
        headers["X-Agent-Id"] = agent_id
        headers["Content-Type"] = "application/json"
        return headers

    def _authenticated(self, private_key: Optional[str]) -> bool:
        return bool(self.api_key or private_key)

    @staticmethod
    def _raise(resp: Response) -> None:
        if resp.ok:
            return
        try:
            body = resp.json()
            msg = body.get("message") or body.get("error") or resp.text
        except ValueError:
            msg = resp.text or resp.reason
        cls = RateLimitError if resp.status_code == 429 else MemroError
        raise cls(resp.status_code, msg)

    def request(
        self,
        method: str,
        path: str,
        *,
        params: Optional[Dict[str, Any]] = None,
        json_body: Any = None,
        files: Optional[Dict[str, Tuple[str, bytes, str]]] = None,
        data: Optional[Dict[str, str]] = None,
        agent_id: Optional[str] = None,
        private_key: Optional[str] = None,
        auth: bool = True,
    ) -> Response:
        """Send a request and return the response, raising MemroError on failure.

        The signature covers the exact path, query string and body that are
        sent, so the URL is built here rather than by ``requests``.
        """
        query = {k: _param(v) for k, v in (params or {}).items() if v is not None}
        path_and_query = path + ("?" + urlencode(query) if query else "")
        url = self.base_url + path_and_query

        for attempt in range(self.max_retries + 1):
            headers: Dict[str, str] = {"Accept": "application/json"}
            if files is not None:
                prepared = requests.Request(method, url, files=files, data=data).prepare()
                body = prepared.body or b""
                headers["Content-Type"] = prepared.headers["Content-Type"]
            elif json_body is not None:
                body = json.dumps(json_body).encode()
                headers["Content-Type"] = "application/json"
            else:
                body = b""
            if auth:
                headers.update(self._headers(method, path_and_query, body, agent_id, private_key))
            resp = self._session.request(method, url, data=body or None, headers=headers, timeout=self.timeout)
            # Signed requests carry a fresh timestamp and signature on each attempt.
            # 429 means the request was not processed; gateway errors are only
            # retried for requests that are safe to repeat.
            retry = resp.status_code == 429 or (resp.status_code in (502, 503, 504) and method in ("GET", "DELETE"))
            # Signatures are deterministic, so an identical signed request in the
            # same second is refused as a replay; re-sign in the next second.
            if resp.status_code == 401 and "already used" in resp.text and "X-Signature" in headers and attempt < self.max_retries:
                time.sleep(1.05 - (time.time() % 1))
                continue
            if retry and attempt < self.max_retries:
                time.sleep(_retry_delay(resp, attempt))
                continue
            self._raise(resp)
            return resp
        raise AssertionError("unreachable")

    def _json(self, method: str, path: str, **kw) -> Any:
        resp = self.request(method, path, **kw)
        return resp.json() if resp.content else None

    # ── Account ─────────────────────────────────────────────────────────────

    def whoami(self, agent_id: Optional[str] = None, private_key: Optional[str] = None) -> dict:
        """The agent this client's credential acts as: {agent_id, tenant_id, user_id, auth}."""
        return self._json("GET", "/auth/whoami", agent_id=agent_id, private_key=private_key)

    def health(self) -> dict:
        return self._json("GET", "/health", auth=False)

    # ── Identity ────────────────────────────────────────────────────────────

    def create_identity(self) -> dict:
        """
        Generate an ed25519 key pair locally and register the public key.
        Returns {"agent_id": str, "private_key": str}; the private key never
        leaves this machine.
        """
        public_key, private_key = generate_keypair()
        body: Dict[str, Any] = {"public_key": public_key}
        pow_params = self._pow_params()
        if pow_params is not None:
            timestamp, nonce = solve_pow(public_key, int(pow_params.get("difficulty", 0)))
            body["pow"] = {"timestamp": timestamp, "nonce": nonce}
        data = self._json("POST", "/identity", json_body=body, auth=False)
        data["private_key"] = private_key
        return data

    def _pow_params(self) -> Optional[dict]:
        """The server's registration proof-of-work settings, or None if it has none."""
        try:
            return self._json("GET", "/identity/pow", auth=False)
        except MemroError as e:
            if e.status_code == 404:
                return None
            raise

    def get_identity(self, agent_id: str) -> dict:
        """Whether an agent is registered: {id, created_at}. The owner is not public."""
        return self._json("GET", f"/identity/{agent_id}", auth=False)

    def delete_identity(self, agent_id: str, private_key: Optional[str] = None) -> None:
        """Delete the agent and all of its memories, files, keys and webhooks."""
        self.request("DELETE", f"/identity/{agent_id}", agent_id=agent_id, private_key=private_key)

    # ── Memories ────────────────────────────────────────────────────────────

    def create_memory(
        self,
        agent_id: str,
        private_key: Optional[str],
        content: str,
        memory_type: Optional[str] = None,
        visibility: str = "private",
        event_date: Optional[str] = None,
        session_id: Optional[str] = None,
        is_atomic: bool = False,
        metadata: Optional[dict] = None,
    ) -> dict:
        """Store a memory. memory_type None (or "auto") lets the server classify it."""
        body: Dict[str, Any] = {"agent_id": agent_id, "content": content, "visibility": visibility, "is_atomic": is_atomic}
        if memory_type:
            body["memory_type"] = memory_type
        if event_date:
            body["event_date"] = event_date
        if session_id:
            body["session_id"] = session_id
        if metadata:
            body["metadata"] = metadata
        return self._json("POST", "/memory", json_body=body, agent_id=agent_id, private_key=private_key)

    def get_memories(
        self,
        agent_id: str,
        private_key: Optional[str] = None,
        memory_type: Optional[str] = None,
        visibility: Optional[str] = None,
        limit: Optional[int] = None,
    ) -> List[dict]:
        """The agent's most recent memories, newest first (at most 100)."""
        params = {"memory_type": memory_type, "visibility": visibility, "limit": limit}
        return self._json("GET", f"/memory/agent/{agent_id}", params=params, agent_id=agent_id, private_key=private_key)

    def delete_memory(self, agent_id: str, private_key: Optional[str], memory_id: str) -> bool:
        """Delete a memory. Returns False if it does not exist."""
        try:
            self.request("DELETE", f"/memory/{memory_id}", agent_id=agent_id, private_key=private_key)
            return True
        except MemroError as e:
            if e.status_code == 404:
                return False
            raise

    def export(self, agent_id: str, private_key: Optional[str] = None) -> dict:
        """Every memory the agent has: {agent_id, exported_at, total, memories}."""
        return self._json("GET", f"/export/{agent_id}", agent_id=agent_id, private_key=private_key)

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
        """Search memories. mode is hybrid (default), semantic or keyword."""
        params = {"agent_id": agent_id, "query": query, "limit": min(limit, 50), "mode": mode, "memory_type": memory_type}
        return self._json("GET", "/search", params=params, agent_id=agent_id, private_key=private_key).get("results", [])

    def search_temporal(
        self,
        agent_id: str,
        private_key: Optional[str],
        query: str,
        limit: int = 10,
        after_date: Optional[str] = None,
        before_date: Optional[str] = None,
        after_event: Optional[str] = None,
        before_event: Optional[str] = None,
        session_id: Optional[str] = None,
        atomic_only: Optional[bool] = None,
        temporal_weight: float = 0.3,
    ) -> dict:
        """Search with date filters (RFC 3339) and a recency boost (0..1)."""
        params = {
            "agent_id": agent_id, "query": query, "limit": min(limit, 50),
            "after_date": after_date, "before_date": before_date,
            "after_event_date": after_event, "before_event_date": before_event,
            "session_id": session_id, "atomic_only": atomic_only, "temporal_weight": temporal_weight,
        }
        return self._json("GET", "/search/temporal", params=params, agent_id=agent_id, private_key=private_key)

    # ── Memory engine ───────────────────────────────────────────────────────

    def reason(self, agent_id: str, private_key: Optional[str], query: str) -> dict:
        """Answer from memory, following the knowledge graph for why/how questions."""
        return self._json("POST", "/mee/reason", json_body={"query": query}, agent_id=agent_id, private_key=private_key)

    def resolve_conflict(self, agent_id: str, private_key: Optional[str], memory_a_id: str, memory_b_id: str) -> dict:
        """Pick the more trustworthy of two memories."""
        body = {"memory_a_id": memory_a_id, "memory_b_id": memory_b_id}
        return self._json("POST", "/mee/resolve", json_body=body, agent_id=agent_id, private_key=private_key)

    def evolve(self, agent_id: str, private_key: Optional[str] = None) -> dict:
        """Consolidate repeated episodes into semantic memories and link updates."""
        return self._json("POST", "/mee/evolve", agent_id=agent_id, private_key=private_key)

    def timeline(self, agent_id: str, private_key: Optional[str], memory_id: str) -> dict:
        return self._json("GET", f"/mee/timeline/{memory_id}", agent_id=agent_id, private_key=private_key)

    def memory_chain(self, agent_id: str, private_key: Optional[str], memory_id: str) -> dict:
        return self._json("GET", f"/memory/chain/{memory_id}", agent_id=agent_id, private_key=private_key)

    def analyze_conflicts(self, agent_id: str, private_key: Optional[str] = None) -> dict:
        return self._json("GET", "/analyze/conflicts", agent_id=agent_id, private_key=private_key)

    def generate_atoms(self, agent_id: str, private_key: Optional[str], text: str) -> dict:
        """Split text into atomic facts (does not store them)."""
        return self._json("POST", "/memory/generate-atoms", json_body={"chunk": text}, agent_id=agent_id, private_key=private_key)

    def stats(self, agent_id: str, private_key: Optional[str] = None) -> dict:
        return self._json("GET", "/stats/temporal", agent_id=agent_id, private_key=private_key)

    def usage(self, agent_id: str, private_key: Optional[str] = None) -> dict:
        return self._json("GET", "/usage", agent_id=agent_id, private_key=private_key)

    # ── Files ───────────────────────────────────────────────────────────────

    def upload_file(self, agent_id: str, private_key: Optional[str], file: FileInput) -> dict:
        """Store a file and extract its text: {file_id, file_name, file_type, file_size, extracted_text}."""
        return self._json("POST", "/upload", files={"file": _file(file)}, data={"agent_id": agent_id},
                          agent_id=agent_id, private_key=private_key)

    def memory_from_file(self, agent_id: str, private_key: Optional[str], file_id: str,
                         memory_type: str = "semantic", visibility: str = "private") -> dict:
        body = {"agent_id": agent_id, "file_id": file_id, "memory_type": memory_type, "visibility": visibility}
        return self._json("POST", "/memory/from-file", json_body=body, agent_id=agent_id, private_key=private_key)

    def upload_image(self, agent_id: str, private_key: Optional[str], file: FileInput) -> dict:
        """Describe an image with vision and store the description as a memory."""
        return self._json("POST", "/upload/image", files={"image": _file(file)}, agent_id=agent_id, private_key=private_key)

    def upload_document(self, agent_id: str, private_key: Optional[str], file: FileInput) -> List[dict]:
        """Store a PDF, Word, PowerPoint, Excel or text document as ~1,500-character
        overlapping memories linked to the file."""
        return self._json("POST", "/upload/document", files={"document": _file(file)}, agent_id=agent_id, private_key=private_key)

    # ── Webhooks ────────────────────────────────────────────────────────────

    def register_webhook(self, agent_id: str, private_key: Optional[str], url: str, event_type: str = "memory_created") -> None:
        self.request("POST", "/hooks", json_body={"event_type": event_type, "url": url}, agent_id=agent_id, private_key=private_key)

    def delete_webhook(self, agent_id: str, private_key: Optional[str], url: str, event_type: str = "memory_created") -> bool:
        """Remove a webhook. Returns False if it was not registered."""
        try:
            self.request("DELETE", "/hooks", json_body={"event_type": event_type, "url": url}, agent_id=agent_id, private_key=private_key)
            return True
        except MemroError as e:
            if e.status_code == 404:
                return False
            raise

    def list_webhooks(self, agent_id: str, private_key: Optional[str] = None, event_type: str = "memory_created") -> List[str]:
        return self._json("GET", f"/hooks/{event_type}", agent_id=agent_id, private_key=private_key)

    # ── Coordination ────────────────────────────────────────────────────────

    def coordination_claim(
        self,
        agent_id: str,
        private_key: Optional[str],
        resource: str,
        intent: str = "",
        session_id: Optional[str] = None,
        ttl_seconds: Optional[int] = None,
    ) -> dict:
        body: Dict[str, Any] = {"resource": resource, "intent": intent}
        if session_id:
            body["session_id"] = session_id
        if ttl_seconds:
            body["ttl_seconds"] = ttl_seconds
        return self._json("POST", "/coordination/claim", json_body=body, agent_id=agent_id, private_key=private_key)

    def coordination_heartbeat(
        self,
        agent_id: str,
        private_key: Optional[str],
        resource: Optional[str] = None,
        lease_id: Optional[str] = None,
        ttl_seconds: Optional[int] = None,
        session_id: Optional[str] = None,
    ) -> dict:
        body: Dict[str, Any] = {}
        if session_id:
            body["session_id"] = session_id
        if resource:
            body["resource"] = resource
        if lease_id:
            body["lease_id"] = lease_id
        if ttl_seconds:
            body["ttl_seconds"] = ttl_seconds
        return self._json("POST", "/coordination/heartbeat", json_body=body, agent_id=agent_id, private_key=private_key)

    def coordination_release(
        self,
        agent_id: str,
        private_key: Optional[str],
        resource: Optional[str] = None,
        lease_id: Optional[str] = None,
        status: str = "completed",
        session_id: Optional[str] = None,
    ) -> dict:
        body: Dict[str, Any] = {"status": status}
        if session_id:
            body["session_id"] = session_id
        if resource:
            body["resource"] = resource
        if lease_id:
            body["lease_id"] = lease_id
        return self._json("POST", "/coordination/release", json_body=body, agent_id=agent_id, private_key=private_key)

    def coordination_handoff(
        self,
        agent_id: str,
        private_key: Optional[str],
        resource: Optional[str] = None,
        lease_id: Optional[str] = None,
        summary: str = "",
        next_agent_id: Optional[str] = None,
        session_id: Optional[str] = None,
    ) -> dict:
        body: Dict[str, Any] = {"summary": summary}
        if session_id:
            body["session_id"] = session_id
        if resource:
            body["resource"] = resource
        if lease_id:
            body["lease_id"] = lease_id
        if next_agent_id:
            body["next_agent_id"] = next_agent_id
        return self._json("POST", "/coordination/handoff", json_body=body, agent_id=agent_id, private_key=private_key)

    def coordination_check(
        self,
        agent_id: str,
        private_key: Optional[str],
        resource: str,
        intent: str = "",
        session_id: Optional[str] = None,
    ) -> dict:
        body: Dict[str, Any] = {"resource": resource, "intent": intent}
        if session_id:
            body["session_id"] = session_id
        return self._json("POST", "/coordination/check", json_body=body, agent_id=agent_id, private_key=private_key)

    def coordination_active(
        self,
        agent_id: str,
        private_key: Optional[str] = None,
        resource: Optional[str] = None,
    ) -> dict:
        params = {"resource": resource} if resource else None
        return self._json("GET", "/coordination/active", params=params, agent_id=agent_id, private_key=private_key)

    def coordination_intent(
        self,
        agent_id: str,
        private_key: Optional[str],
        resource: str,
        intent: str,
        ttl_seconds: Optional[int] = None,
        session_id: Optional[str] = None,
    ) -> dict:
        body: Dict[str, Any] = {"resource": resource, "intent": intent}
        if session_id:
            body["session_id"] = session_id
        if ttl_seconds:
            body["ttl_seconds"] = ttl_seconds
        return self._json("POST", "/coordination/intent", json_body=body, agent_id=agent_id, private_key=private_key)


def _param(v: Any) -> str:
    if isinstance(v, bool):
        return "true" if v else "false"
    return str(v)


def _retry_delay(resp: Response, attempt: int) -> float:
    try:
        return min(float(resp.headers.get("Retry-After", "")), 30.0)
    except ValueError:
        return 0.5 * (2 ** attempt)


def _file(file: FileInput) -> Tuple[str, bytes, str]:
    if isinstance(file, tuple):
        name, data = file[0], file[1]
        ctype = file[2] if len(file) > 2 else (mimetypes.guess_type(name)[0] or "application/octet-stream")
        return name, data, ctype
    path = os.fspath(file)
    with open(path, "rb") as f:
        data = f.read()
    name = os.path.basename(path)
    return name, data, mimetypes.guess_type(name)[0] or "application/octet-stream"
