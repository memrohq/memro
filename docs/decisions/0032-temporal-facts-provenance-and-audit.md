# 0032. Temporal facts, source-aware writes, agent edits and an auditable history
Date: 2026-11-15 · Status: Proposed

## Context
Memro stores each memory as text with optional dates (`event_date`,
`document_date`), loose entity tags, and typed links in `memory_relations`
(ADR 0002, 0023). Deciding what a new memory means for the old ones is done by
rules in `engine/knowledge.go` (`DetectContradiction`, `NearDuplicate`), at
query time in search and, behind `GRAPH_WRITE_EDGES`, at save time.

What we learned on 2026-11-15:

- **Rules guess from wording.** The evolution benchmark (`TestEvolutionBench`)
  went from 4/9 updates unresolved to 0/9 with sharper rules (PR #60), but each
  new rule needed traps to stop it hiding true facts ("Ana moved to the Berlin
  office" vs "Ana is in the design team"). A rule written for short facts (the
  negation rule) marked about 1 in 10 pairs of LongMemEval chat messages as
  contradicting, demoting real evidence in search and producing ~1,870 bogus
  `updates` edges for ~6 questions' memories with save-time linking on.
- **Additive vs replacing facts can't be told apart from text alone.**
  "Memro supports Cursor" and "Memro supports Codex" are both true; "the billing
  contact is Marco" and "… is Ana" are not. Today that is a regex judgement.
- **There is no edit.** The API and MCP tools can create and delete memories,
  not update or replace them, so an agent that learns a correction can only add
  a new memory and hope search ranks it first.
- **Not every write is distilled.** Agents using the MCP tools write one short
  self-contained sentence (their own model does the extraction, at no cost to
  us). Dashboard entries, SDK calls, file imports (`/memory/from-file`) and
  connector syncs (ADR 0011) arrive as whatever the user or source provides.
- **The audit log records too little.** `audit_log` (migration 0005) stores
  agent, action, resource id, IP and time. It doesn't record where a write came
  from, the content before and after, why, or anything done by automated
  processes (Evolve, the save-time linker).
- **LLM use today** is limited to embeddings on every write
  (`text-embedding-3-small`), image descriptions (`gpt-4o`) and audio
  transcription (`whisper-1`). No model decides anything about memories.

Other memory systems, from their published designs: Mem0 runs an LLM on every
add to extract facts and choose add/update/delete; Zep's Graphiti extracts
entities and relationships into a bi-temporal graph and invalidates edges with
an LLM; Letta lets the agent edit its own memory through tools.

## Decision

### 1. Facts with time, derived from memories
Memories stay as written (the "episode"). Facts are derived from them, in
Postgres (ADR 0002 still holds: no separate graph database).

- `entities`: id, tenant, agent, kind (person, organisation, project, tool,
  place, other), canonical name, aliases. Replaces string entity tags as the
  thing facts point at; tags stay as a search hint.
- `facts`: id, tenant, agent, subject entity, predicate, value (text, number,
  date or entity), **valid_from / valid_to** (when it was true in the world),
  **recorded_at / retired_at** (when Memro learned it and stopped believing it),
  source memory, confidence, `decided_by` (rule, llm, agent, user), reason.
- `predicates`: name and **cardinality**, single-valued (`billing_contact`,
  `database`, `port`, `deploy_day`) or multi-valued (`supports`, `uses_tool`,
  `member_of`). Seeded with common predicates; new ones default to
  multi-valued until a rule, the user or the LLM judge says otherwise.

A new value for a single-valued (subject, predicate) **closes** the open fact:
its `valid_to` is set to the new fact's `valid_from`, and an `updates` edge
links the two memories, so search keeps working unchanged (ADR 0021). Facts
are never deleted by this; history stays queryable. Multi-valued predicates
never conflict. This replaces most of the wording rules: conflict becomes a
lookup on (subject, predicate), not a guess.

New reads: current facts (`valid_to IS NULL AND retired_at IS NULL`), facts
as of a date, and the history of a (subject, predicate). Search attaches the
current facts for the entities in its results; the temporal ledger (ADR 0021)
reads facts when they exist.

### 2. Every write carries its source, and the source decides the processing
Writes store immediately and are enriched in the background, as embeddings
already are; enrichment never blocks a write.

| Source | Treated as | Enrichment |
|---|---|---|
| MCP tools (Claude Code, Cursor, Codex, Copilot…) | distilled by the agent's model | rule extraction; LLM judge only for unclear pairs (§4) |
| Dashboard (a person typed it) | authoritative | rule extraction; no LLM |
| SDKs | the developer chooses | `extract`: `none`, `rules` (default) or `llm` (opt-in, billed); `remember_raw` for transcripts |
| File import, connectors | raw, unsummarised | chunks kept with provenance (file, page, connector item); LLM fact extraction in the background, within the plan's quota |

Each memory records `source` (e.g. `mcp:remember`, `sdk:python`, `dashboard`,
`import:pdf`, `connector:slack`), `author_type` (agent, user, system) and the
model used for any LLM step.

### 3. Agents can edit their own memory
New API endpoints and MCP tools, each requiring a reason and limited to the
caller's own memories:

- `update_memory(id, new_content, reason)`: writes a new version, closes the
  old one's facts and links new → old with `updates`. Nothing is overwritten.
- `forget_memory(id, reason)`: archives (soft delete). Hidden from search,
  kept in history.
- `correct_fact(subject, predicate, value, reason)`: a structured fact edit,
  no extraction needed.

Hard deletion stays a separate, explicit operation (privacy requests), and is
itself logged (content excluded).

### 4. An LLM only where rules can't decide
1. Rules run on every write (free, milliseconds), restricted to short single
   statements; long multi-sentence text goes to extraction instead.
2. When a new memory has close neighbours (embedding similarity above a
   threshold) and rules can't classify the pair, a small model answers one
   structured question: duplicate, update, contradiction, extends or unrelated,
   plus the predicate and its cardinality when it's new. Async, cached by
   content hash, with a per-plan monthly budget.
3. Extraction of facts from raw text (§2) uses the same model with a fixed
   schema.
4. Evolve's nightly consolidation writes real summaries with source links,
   using batch requests.

Measured on 2026-11-15, LongMemEval's 200 answer calls (~1.1M tokens) cost
$0.17 with `gpt-4o-mini`; a judge call with two short memories is roughly a
twentieth of an answer call. The model is a configuration choice per
deployment.

### 5. Every memory auditable
- **Append-only memories.** Content is never changed in place: edits create a
  version, deletes are soft unless a hard purge is requested.
- **A richer audit log.** New columns: `actor_type` (agent, user, system, llm),
  `actor_id`, `source`, `before` / `after` (content or a diff, JSONB),
  `reason`, `request_id`, and for LLM decisions `model`, `prompt_hash` and
  cost. Automated changes (save-time links, Evolve, the judge, fact closing)
  write entries too; link metadata already stores a `basis`.
- **Tamper evidence (optional, per tenant).** Each audit row stores a hash of
  the previous row for the tenant, so editing history is detectable.
- **Visible.** `GET /memory/{id}/history` (extending `Timeline`), a History
  panel per memory in the dashboard, and audit export (CSV, JSON).

## Rollout
Each step is its own PR, behind a flag where it changes behaviour, and must
pass the evolution benchmark gate and not regress LongMemEval (three-arm local
run: main, change, change + `GRAPH_WRITE_EDGES`). Cost per 1,000 memories is
reported alongside accuracy for any LLM step.

1. Provenance (`source`, `author_type`), the richer audit log, soft delete,
   `GET /memory/{id}/history`.
2. Agent edit tools: `update_memory`, `forget_memory`; MCP and SDKs.
3. Source-aware enrichment: SDK `extract` option, background LLM extraction
   for imports and connectors.
4. Entities, facts and predicates; rule extraction for short statements;
   current / as-of / history reads; `correct_fact`.
5. The LLM judge for unclear pairs; then turn save-time linking on in staging
   if the benchmarks support it.

## Alternatives considered
- **An LLM on every write (Mem0-style).** Simplest and most accurate on messy
  text, but adds cost and latency to every write, including the agent-written
  facts that are already distilled. Rejected as the default; available per
  write through the SDK `extract: llm` option.
- **A graph database (Neo4j, as Graphiti uses).** Rejected by ADR 0002; facts
  and entities are relational and fit Postgres, and one store keeps tenancy,
  backups and auditing in one place.
- **Keep improving the wording rules.** Cheap, but each rule needs traps and
  still can't tell additive from replacing facts. Kept as the fast path for
  short statements only.
- **Let agents edit memories in place.** Simpler, but loses history and makes
  audit impossible. Rejected in favour of versions.

## Consequences
- Contradiction handling moves from wording to structure; the rules in
  `knowledge.go` shrink to a fast path and the benchmarks become the guard.
- Storage grows: memory versions, facts and a larger audit log. Retention
  settings for audit and archived memories are needed per plan.
- LLM costs appear on the bill for imports, connectors and opted-in SDK
  writes, bounded by plan quotas; MCP writes stay free of LLM cost unless the
  judge runs.
- Search can answer "what is true now" and "what was true on <date>" from
  facts, and show why a memory was replaced.
- Hard deletion must reach facts, versions and audit content for privacy
  requests; the audit row of the deletion itself remains, without content.
- Builds on ADR 0015 (verified code memory): code anchors become one more kind
  of provenance on facts.
