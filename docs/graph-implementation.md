# Graph implementation

How Memro's memory graph is stored, written and read, what it's used for today,
and what the benchmarks say about it. Written 2026-11-19 from the code on
`main`; the design is ADR 0023 (edges and search expansion), building on
ADR 0002 (no separate graph database) and ADR 0021 (temporal ordering).

**In short:** the graph is one Postgres table of links between memories. It's
cheap and well bounded, but mostly switched off: ordinary saves write no
edges, and graph expansion in search is behind a flag. Its main live use is
marking superseded results in every search. Where it has been measured, it
hasn't improved recall yet.

## Storage

No graph database. Memories are the nodes; `memory_relations` (migration
`0013`, extended by `0031`) holds the edges:

```
memories (nodes)                     memory_relations (edges)
┌────────────────────────┐          ┌──────────────────────────────────┐
│ id                     │◄─────────│ source_memory_id                 │
│ content                │◄─────────│ target_memory_id                 │
│ entity_tags  {...}     │          │ relation_type                    │
│ event_date, ...        │          │ confidence, metadata, created_by │
└────────────────────────┘          └──────────────────────────────────┘
```

Indexes on source and target make following an edge an indexed lookup; a walk
is a loop of those lookups in Go. Edges are deleted with either memory
(`ON DELETE CASCADE`), and a unique index on (source, target, type) stops
duplicates.

### Edge types (ADR 0023)

| Type | Direction | Meaning | Example |
|---|---|---|---|
| `updates` | newer → older | The newer memory replaces a fact in the older one | "DB moved to Postgres 17" → "DB is Postgres 16" |
| `associated` | undirected, stored once | Both are about the same specific thing | two memories mentioning issue MEM-12 |
| `causes` | cause → effect | One is a reason for the other; only by explicit assertion, never inferred | "outage" → "rollback" |
| `derives` | source → product | Provenance only: the target was produced from the source | episodes → an `Evolve()` summary |
| `extends` | detail → base | Adds detail without contradicting | — |

Sharing a session is not an edge (`session_id` already records it), and
sharing an entity gives at most `associated`.

## Who writes edges

| Writer | Code | On in production? |
|---|---|---|
| `Evolve()` consolidation: consolidation and contradiction links | `server/internal/engine/engine.go` (`Evolve`) | Yes, when it runs |
| Connectors: cross-references between items that share an issue key | `server/internal/connectors/links.go` | Yes |
| Save-time linker: for a new memory, the 3 most recent memories sharing each of its first 5 specific entities; `updates` if `DetectContradiction ≥ 0.7` (newer → older by `event_date`, then `document_date`), otherwise `associated`; at most 8 edges, 200 ms budget, best effort | `server/internal/engine/linker.go` (`LinkOnSave`) | **No**: `GRAPH_WRITE_EDGES` defaults to `false` |

An ordinary `POST /memory`, which agents, the MCP tools and the hooks use,
writes no edges, so on most accounts the graph is close to empty.

## Who reads edges

1. **Every search: marking superseded results.** After hybrid search,
   `relationsTouching` loads every edge touching the results and
   `resolveSearchContradictions` (`engine.go`) marks the older end of each
   `updates` edge `IsSuperseded`, with `SupersededBy` pointing to the newer
   one (when both are in the results, their dates decide). This is the graph's
   main live use.
2. **Graph expansion in search** (`ExpandGraph`, `graph.go`), behind the
   `GRAPH_RETRIEVAL` flag (default off) and the `graph=legacy|bounded|rerank|selective`
   parameter:
   - seeds: the top `min(limit, 10)` hybrid results;
   - walk up to 2 hops, only over edges whose both ends belong to the same
     agent and tenant;
   - score `seed_score × path_confidence × 0.7^(hops−1) × type_weight`
     (`updates`, `causes` 1.0; `derives`, `extends` 0.8; `associated` 0.6);
   - graph results take at most a third of the slots and never displace the
     top 5;
   - budgets: 2,000 edges, 200 paths, 150 ms; past them it returns the hybrid
     results unchanged.
3. **`Reason()`** (`POST /mee/reason`) walks the graph to build reasoning chains.
4. **History views**: `MemoryChain`, `Timeline` and memory history follow
   `updates` chains.
5. **Dashboard 3D view** (`memgraph.go`): memories, the entity tags they share,
   and their edges.

## Why it's built this way: the 2026-10-24 ablation

The bounds and the flags come from a graph ablation on LongMemEval-100
(ADR 0023 §6–7; results in `docs/benchmarks/results/longmemeval/ablation-100/`).
Every arm queried the same stored memories (ingested with
`GRAPH_WRITE_EDGES=true`), server, reader (`gpt-4o-mini`) and judge. Accuracy
below is recomputed from each arm's saved evaluation file; latency from
ADR 0023.

| Arm | What it tested | Answer accuracy | p95 search |
|---|---|---|---|
| A | Hybrid search, no graph (baseline) | 80/100 | 356 ms |
| B | + the original walk (`graph=legacy`) | 78/100 | 808 ms |
| C | + PR #43's bounded walk (`graph=bounded`) | 76/100 | 662 ms |
| D | bounded walk + temporal resolution | 76/100 | 511 ms |
| E | graph reranking, no new memories (`graph=rerank`) | 81/100 | 428 ms |
| F | selective: rerank or expand by query intent (`graph=selective`) | 80/100 | 866 ms |
| G | E + adaptive session diversity | 80/100 | — |
| H | F + adaptive session diversity | 81/100 | — |

What it decided:
- **Blind expansion was rejected.** B–D added linked memories about the same
  entity to every answer; on "how many / total" questions the reader
  double-counted, and multi-session accuracy fell to 55.6%. Hence the cap of
  a third of the slots, never displacing the top 5, and the edge, path and
  time budgets.
- **Reranking is the safer use.** E only reorders what hybrid search already
  found, so it adds no distractors (multi-session 70.4%). `rerank` and
  `selective` stay as opt-in modes.
- **Both flags stay off.** The gate set in advance (ADR 0023 §6) asked for
  accuracy above run-to-run noise, no loss in recall, at most 50 ms added to
  p95 and at least 80% correct `updates` edges. E added 72 ms and lowered
  Recall@5 from 99% to 95%, and 81 vs 80 is one question.
- **No graph database.** Walks are at most 2 hops within one agent (ADR 0002).

Limits of that evidence:
- Each arm ran once; ADR 0023 planned three repeats, and later reruns
  (journal 2026-11-15) showed identical runs differing by a few points, so
  E's 81 vs A's 80 is within noise.
- Arms G and H aren't in ADR 0023's table, and arm I (selective + diversity
  + intent-aware supersession) has no saved evaluation.
- The edge-precision audit (do `updates` edges actually hold?) has no
  recorded result.
- There is no journal write-up of the run, only ADR 0023 §7.

## What the benchmarks say

From journal 2026-11-15 (evolution engine) and 2026-10-23 (LongMemEval):

- LongMemEval-100 with save-time edges on: 78%, against 80% without; no
  reliable gain, so `GRAPH_WRITE_EDGES` stays off.
- Search-time contradiction resolution: 81.0% vs 81.0%, no net difference.
- 14 of 19 LongMemEval failures already had the evidence in the top 15: the
  misses are in answering, which a graph doesn't change.
- `updates` edges come from `DetectContradiction`, a wording rule; on
  LongMemEval knowledge updates Memro is 12/16 against plain RAG's 14/16.
- The relay benchmark's hard case (a rule sharing no words with the task)
  isn't reachable through the graph either: no edge links the task to the
  rule unless something created one.

## Where it could earn its place

- **Enforcement (roadmap P2):** edges from a rule to the files and
  identifiers it governs, so an edit can be checked against the rules linked
  to it.
- **Supersession and entities (P4):** a model writing `updates` and
  `associated` edges at save time, instead of wording rules, is the test of
  whether a populated graph helps.
- **ADR 0032** would move "is this still true?" from walking `updates` edges
  to a lookup on time-bounded facts (`valid_from` / `valid_to`); the graph
  would remain for association and provenance.

The structure is sound; what limits it is how few edges get written and the
rules that write them.
