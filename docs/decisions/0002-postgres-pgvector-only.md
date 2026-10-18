# 0002. Postgres + pgvector only (no Qdrant, no graph database)
Date: 2026-10-18 · Status: Accepted

## Context
The stack used Postgres, Qdrant for vectors, and an (unused) Neo4j
integration in the MCP server. Three stores meant sync problems and more to
run, back up and pay for.

## Decision
Store memories, the knowledge graph (`memory_relations`) and embeddings
(`memory_embeddings`, pgvector) in one Postgres. Search is exact cosine per
agent (per-agent soft cap keeps it fast); add an HNSW index when single agents
exceed ~100k memories.

## Alternatives considered
- Neo4j / LadybugDB: graph traversal here is shallow (≤3 hops) and always
  scoped to one tenant/agent, which SQL handles well; a separate graph engine
  is only worth it for deep traversals or graph algorithms over one huge graph.
  If needed later, add it as a derived read model, not the source of truth.
- Apache AGE: keeps Cypher inside Postgres; revisit if Cypher becomes useful.

## Consequences
One database to operate; vectors and memories commit together. Scale path:
bigger Postgres → read replicas → shard by tenant (e.g. Citus). Known graph
work: entity nodes, superseded facts instead of edge deletion, bounded fan-out.
