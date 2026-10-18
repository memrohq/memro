"""
Supermemory-style enhancements for Memro MCP Server
Implements: temporal reasoning, knowledge chains, atomic memory generation
"""

def get_supermemory_tools():
    """
    Returns list of new Supermemory-enhanced tools for the MCP server.
    These implement temporal grounding, relational versioning, and multi-session reasoning.
    """
    from mcp.types import Tool
    
    return [
        Tool(
            name="search_temporal",
            description=(
                "Search memories with temporal reasoning. Finds relevant memories considering "
                "both when the conversation happened and when events actually occurred. "
                "Perfect for multi-session synthesis and temporal queries."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "What to search for (e.g., 'things I did last summer')",
                    },
                    "after_date": {
                        "type": "string",
                        "description": "ISO 8601 date - only return memories from this date forward",
                    },
                    "before_date": {
                        "type": "string",
                        "description": "ISO 8601 date - only return memories before this date",
                    },
                    "temporal_weight": {
                        "type": "number",
                        "minimum": 0.0,
                        "maximum": 1.0,
                        "default": 0.3,
                        "description": "How much to prioritize recency (0=semantic only, 1=temporal only)",
                    },
                    "atomic_only": {
                        "type": "boolean",
                        "default": False,
                        "description": "Only return atomic facts (true) vs raw chunks (false)",
                    },
                    "limit": {
                        "type": "integer",
                        "default": 10,
                        "maximum": 50,
                    },
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="recall_multi_session",
            description=(
                "Recall memories across multiple sessions with synthesis. "
                "Connects related facts from different conversations and resolves contradictions."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "What to search for across sessions",
                    },
                    "include_all_sessions": {
                        "type": "boolean",
                        "default": True,
                        "description": "Search across all past sessions or current session only",
                    },
                    "limit": {
                        "type": "integer",
                        "default": 15,
                        "description": "Max results from each session",
                    },
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="get_memory_chain",
            description=(
                "Get the knowledge chain for a memory, showing how it evolved. "
                "Returns updates (state changes), extends (refinements), and derives (inferences)."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "memory_id": {
                        "type": "string",
                        "description": "UUID of the memory to trace",
                    },
                },
                "required": ["memory_id"],
            },
        ),
        Tool(
            name="find_contradictions",
            description=(
                "Analyze stored memories for contradictions. Helps identify when beliefs "
                "have changed or conflicting information was stored."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "topic": {
                        "type": "string",
                        "description": "Optional: narrow analysis to a specific topic",
                    },
                    "threshold": {
                        "type": "number",
                        "minimum": 0.0,
                        "maximum": 1.0,
                        "default": 0.6,
                        "description": "Contradiction confidence threshold (0.6 = 60% confidence of contradiction)",
                    },
                },
            },
        ),
        Tool(
            name="get_temporal_stats",
            description=(
                "Get temporal statistics about your memories. Shows distribution over time, "
                "frequency patterns, and memory creation trends."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "days": {
                        "type": "integer",
                        "default": 90,
                        "description": "Analyze memories from the last N days",
                    },
                },
            },
        ),
        Tool(
            name="remember_with_context",
            description=(
                "Store a memory with rich temporal context. Automatically extracts "
                "when events occurred vs when they're discussed, enabling better temporal reasoning."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "content": {
                        "type": "string",
                        "description": "What to remember",
                    },
                    "event_date": {
                        "type": "string",
                        "description": "When the event actually occurred (ISO 8601 or natural language)",
                    },
                    "type": {
                        "type": "string",
                        "enum": ["episodic", "semantic", "procedural", "profile"],
                        "default": "episodic",
                    },
                    "is_atomic": {
                        "type": "boolean",
                        "default": False,
                        "description": "Mark as atomic fact (single statement) vs chunk (narrative)",
                    },
                },
                "required": ["content"],
            },
        ),
        Tool(
            name="generate_atomic_facts",
            description=(
                "Break a longer narrative into atomic facts for better searchability. "
                "Automatically extracts entities and relationships."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "text": {
                        "type": "string",
                        "description": "Text to decompose into facts",
                    },
                    "context": {
                        "type": "string",
                        "description": "Optional context to resolve pronouns",
                    },
                    "auto_save": {
                        "type": "boolean",
                        "default": False,
                        "description": "Automatically save generated facts as memories",
                    },
                },
                "required": ["text"],
            },
        ),
        Tool(
            name="mee_reason",
            description="Trigger the Memory Evolution Engine to reason about a specific query using its internal knowledge graph.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "The question or scenario to reason about"}
                },
                "required": ["query"]
            }
        ),
        Tool(
            name="mee_resolve",
            description="Resolve a detected conflict between two memories using evidence-based scoring.",
            inputSchema={
                "type": "object",
                "properties": {
                    "memory_a_id": {"type": "string", "description": "UUID of the first memory"},
                    "memory_b_id": {"type": "string", "description": "UUID of the second memory"}
                },
                "required": ["memory_a_id", "memory_b_id"]
            }
        ),
        Tool(
            name="mee_evolve",
            description="Trigger a full evolution cycle (decay, consolidation, conflict detection) for the current agent.",
            inputSchema={"type": "object", "properties": {}}
        ),
        Tool(
            name="get_timeline",
            description="Get the full historical timeline of a memory, including all its versions and updates.",
            inputSchema={
                "type": "object",
                "properties": {
                    "memory_id": {"type": "string", "description": "UUID of the memory to trace"}
                },
                "required": ["memory_id"]
            }
        ),
    ]