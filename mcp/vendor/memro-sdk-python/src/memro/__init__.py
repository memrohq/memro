"""
memro — AI agent memory infrastructure SDK for Python.

Quickstart:
    from memro import Agent

    agent = Agent.create()
    agent.remember("User prefers dark mode", type="profile")
    results = agent.recall("user preferences")
    agent.save("identity.json")
"""
from .agent import Agent
from .client import MemroClient, MemroError
from .models import Memory, SearchResult, ExportData, HealthStatus
from .crypto import generate_keypair

__all__ = [
    "Agent",
    "MemroClient",
    "MemroError",
    "Memory",
    "SearchResult",
    "ExportData",
    "HealthStatus",
    "generate_keypair",
]

__version__ = "0.1.0"
