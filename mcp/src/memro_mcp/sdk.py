import httpx
import asyncio
from typing import List, Optional, Dict, Any

class MemroSDK:
    """
    Simplified Developer SDK for Memro: The Active Knowledge Layer for AI Agents.
    
    This SDK abstracts away the complexity of the MEE (Memory Evolution Engine) 
    and providing a human-first interface for AI agent memory.
    """
    def __init__(self, agent_id: str, token: str, base_url: str = "http://localhost:8081"):
        self.agent_id = agent_id
        self.token = token
        self.base_url = base_url
        self.headers = {
            "Authorization": f"Bearer {token}",
            "X-Agent-Id": agent_id,
            "Content-Type": "application/json"
        }

    async def remember(self, content: str, memory_type: str = "episodic", metadata: Dict[str, Any] = None):
        """
        Store a new memory for the agent.
        
        Args:
            content: The text content of the memory.
            memory_type: 'episodic' (event) or 'semantic' (fact). Defaults to 'episodic'.
            metadata: Optional dictionary of additional context.
        """
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.base_url}/memories",
                json={"content": content, "memory_type": memory_type, "metadata": metadata or {}},
                headers=self.headers
            )
            response.raise_for_status()
            return response.json()

    async def recall(self, query: str, limit: int = 5):
        """
        Retrieve the most relevant memories based on semantic similarity.
        
        Args:
            query: The search string.
            limit: Maximum number of memories to return.
        """
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.base_url}/search",
                json={"query": query, "limit": limit},
                headers=self.headers
            )
            response.raise_for_status()
            return response.json()

    async def reason(self, query: str, complex: bool = False):
        """
        Ask the Memory Evolution Engine (MEE) to reason over stored knowledge.
        
        Args:
            query: The question or reasoning prompt.
            complex: If True, uses the 'Deep Path' (causal analysis). Defaults to False (Fast Path).
        """
        endpoint = "/mee/reason"
        # The engine uses heuristics to decide between Fast and Deep paths based on query keywords,
        # but the SDK can eventually support explicit mode selection.
        async with httpx.AsyncClient() as client:
            response = await client.post(
                f"{self.base_url}{endpoint}",
                json={"query": query},
                headers=self.headers
            )
            response.raise_for_status()
            return response.json()

    async def get_usage(self):
        """
        Fetch real-time usage metrics for the current agent.
        Includes reasoning call counts and graph statistics.
        """
        async with httpx.AsyncClient() as client:
            response = await client.get(
                f"{self.base_url}/usage",
                headers=self.headers
            )
            response.raise_for_status()
            return response.json()
