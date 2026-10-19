"""
memro — AI agent memory infrastructure SDK for Python.

Quickstart:
    from memro import Agent

    agent = Agent.from_env()            # MEMRO_API_KEY=memro_sk_...
    agent.remember("User prefers dark mode", type="profile")
    results = agent.recall("user preferences")
"""
from .agent import Agent
from .client import MemroClient, MemroError, RateLimitError
from .models import (
    Memory, SearchResult, ExportData, HealthStatus, TemporalSearchResult,
    ReasoningResult, EvolutionResult, MemoryTimeline, MemoryChain, TemporalStats, UploadedFile,
)
from .crypto import generate_keypair, sign_request

__all__ = [
    "Agent",
    "MemroClient",
    "MemroError",
    "RateLimitError",
    "TemporalSearchResult",
    "ReasoningResult",
    "EvolutionResult",
    "MemoryTimeline",
    "MemoryChain",
    "TemporalStats",
    "UploadedFile",
    "sign_request",
    "Memory",
    "SearchResult",
    "ExportData",
    "HealthStatus",
    "generate_keypair",
]

__version__ = "0.2.0"
