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
                "Summarize contradictions the memory engine has found: how many beliefs were "
                "updated by newer memories and how many are still unresolved. Use mee_resolve "
                "to settle a specific pair."
            ),
            inputSchema={"type": "object", "properties": {}},
        ),
        Tool(
            name="get_temporal_stats",
            description=(
                "Get statistics about your memories: totals for today, this week and this "
                "month, average days between memories, and the count per memory type."
            ),
            inputSchema={"type": "object", "properties": {}},
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