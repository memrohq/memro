# Memro vs Mem0, Zep (Graphiti), Supermemory and Cognee: agent memory compared

Memro, Mem0, Zep, Supermemory and Cognee all give AI agents memory that
lasts beyond one conversation. They make different trade-offs. This page sets out those
trade-offs so you can pick the one that fits. It compares design and
features only. We haven't published a head-to-head accuracy benchmark yet,
and we won't quote other projects' numbers next to ours, because they come
from different models, prompts and judges.

*Last checked: November 2026 (Supermemory and Cognee added; Mem0 updated). These projects move quickly. If something here
is out of date, please [open an issue](https://github.com/memrohq/memro/issues).*

## At a glance

| | Memro | Mem0 | Zep / Graphiti | Supermemory | Cognee |
|---|---|---|---|---|---|
| **What it is** | A memory server for agents with a relationship graph between memories | A memory layer that uses an LLM to extract facts | Zep Cloud, a managed service built on Graphiti, an open-source temporal knowledge graph framework | A memory and context engine: a hosted API with SDKs, building a graph of facts | A knowledge engine that turns data into a knowledge graph for agents |
| **License** | [PolyForm Noncommercial 1.0.0](../../LICENSE): source-available, free for noncommercial use, commercial use through Memro Cloud or a commercial license | Apache 2.0 | Graphiti: Apache 2.0. Zep Cloud: commercial. Zep Community Edition is deprecated. | Public repository MIT; SDKs Apache 2.0. The self-hosted server is a separate binary built from a non-public codebase | Apache 2.0 |
| **Self-hosting** | Yes, with `docker compose up` | Yes, with Docker | Graphiti: yes, as a library you build on. Zep itself: cloud only. | A self-hosted server binary, free up to a document limit; otherwise cloud | Yes, as a Python library or server |
| **Storage** | One Postgres database with pgvector for memories, graph and embeddings | A vector store, plus an optional graph store | A graph database: Neo4j, FalkorDB or Amazon Neptune | Not documented publicly; described as vector, full-text and graph built in | Three pluggable stores: relational (SQLite or Postgres), vector (LanceDB, Qdrant, Milvus and others) and graph (Neo4j, Kuzu, FalkorDB and others) |
| **What happens on write** | Stores the text you send. Type, entity tags and dates come from cheap rules; the only model call is the embedding. `generate_atomic_facts` splits text into atomic facts on request, and memories made from files link back to them (`file_id`) | One LLM call per add, extracting facts. Since April 2026 it only adds: nothing is updated or deleted | An LLM extracts entities and relationships from every episode | Fact extraction into a graph of facts linked to their sources | A `cognify` pipeline: classify documents, chunk, extract a graph of entities and relationships, summarise, embed |
| **How changes over time are handled** | Memories link with `updates`, `extends` and `derives`; evolution merges recent episodes into facts, and contradictions are flagged. Facts with validity windows are designed (ADR 0032), not built | Facts accumulate; nothing is overwritten | Bi-temporal facts: each has a validity window and is invalidated when superseded, not deleted | Described as a temporal graph of facts; details aren't public | An optional enrichment step (`memify`) and an `improve` call |
| **MCP** | Built in, with 20+ tools including multi-agent task claiming and handoff | Through Mem0's editor integrations | Graphiti ships an MCP server | An MCP server | An MCP server |
| **SDKs** | Python, TypeScript, Go | Python, TypeScript | Graphiti: Python. Zep Cloud: Python, TypeScript, Go. | Python, TypeScript | Python |
| **Connectors** | 50 sources: 7 native OAuth (Slack, GitHub, Linear, Sentry, Notion, Jira, GitLab) and 43 declarative | Not a core feature | Not a core feature | Imports from documents and apps | Ingests files and data sources through its pipelines |
| **Agent identity** | Each agent is an ed25519 key pair, and self-owned agents sign requests with a key that never leaves the machine | API keys | API keys | API keys | API keys (hosted); none for the embedded library |

## Where Memro is different

**Writes don't need an LLM.** Mem0 and Graphiti call an LLM to process
every memory you add. That gives them structured facts, but it costs money
and time on every write and makes ingestion depend on a model provider.
Memro stores what you give it and does the extra work later: evolution and
reasoning run when you call them. The trade-off is that Memro won't rewrite
messy input into clean facts at write time. You either send good memories,
or call `generate_atomic_facts` first.

**One database.** Memories, the relationship graph and the vectors all live
in Postgres with pgvector, with no separate graph database to run. If you
already run Postgres, you already know how to operate Memro.

**Built for coding agents and teams.** The MCP server includes tools for
several agents working on one codebase (`claim_task`, `pre_action_check`,
`handoff_task`). Connectors bring Slack threads, GitHub PRs, Linear tickets
and Sentry errors into the same memory, linked across sources.

**Agents own their identity.** An agent's ID is its public key. A
self-owned agent signs its requests, so the server never sees its private
key. You can export everything with one call (`GET /export/{agent_id}`).

## Where the others are stronger

- **License.** Mem0, Graphiti and Cognee are Apache 2.0, and Supermemory's
  public repository is MIT, so you can build on them commercially for free.
  Memro is free only for noncommercial use.
- **Temporal modeling.** Graphiti records when each fact became true and
  when it stopped being true. Memro tracks event dates and supersession
  links, but it doesn't yet have a full bi-temporal model.
- **Maturity and ecosystem.** Mem0 and Graphiti have larger communities
  and integrate with many agent frameworks. Memro's LangChain, LangGraph
  and CrewAI adapters are [on the roadmap](../../README.md#roadmap).
- **Fact extraction.** If your input is long, messy conversation and you
  want clean facts with no extra work, the LLM step in Mem0, Graphiti,
  Supermemory and Cognee does that on every write. Memro does it only when
  you call `generate_atomic_facts`.
- **Knowledge graphs from documents.** Cognee's pipeline builds a graph of
  entities and relationships from whole document sets. Memro links memories
  to each other, but doesn't extract entities and relationships from
  documents.

## What Memro is learning from them

Ideas, not code. Each will be measured in Memro's own harness before it
ships (see the roadmap):

- **From Mem0: extracting facts at write time, keeping the original.** One
  model call per write that adds structured facts next to the user's words,
  never replacing them.
- **From Supermemory: atomic facts tied to their source.** Memro already has
  atomic facts and links from memories to files; the step is to deliver the
  source evidence with every fact.
- **From Zep and Graphiti: facts with validity windows.** A fact is closed,
  not deleted, when a newer one replaces it (Memro's ADR 0032).
- **From Cognee: extraction in stages.** Chunking, entity and relationship
  extraction and summaries as separate, testable steps.

## Which one should you pick?

- **Pick Memro** if you want self-hosted memory on Postgres with no LLM
  bill for writes, your agents are coding tools that speak MCP, or you
  want memory fed by Slack, GitHub, Linear and other team tools. Your use
  also has to be noncommercial, or you'll use Memro Cloud.
- **Pick Mem0** if you need an Apache-licensed library to embed in a
  commercial product, and you're happy to pay for an LLM call on each write.
- **Pick Graphiti or Zep** if your problem is mostly about facts that
  change over time and you're comfortable running a graph database, or
  you want it fully managed with Zep Cloud.
- **Pick Supermemory** if you want a hosted memory API with SDKs and
  imports, and don't need the server's source.
- **Pick Cognee** if you want to build a knowledge graph from a body of
  documents, with your choice of graph and vector stores.

## Try Memro

```bash
npx memro-cli connect     # connect Claude Code, Cursor, Codex and other tools
```

Or run it yourself: see [Self-hosting](../self-hosting.md). Read
[Concepts](../concepts.md) for how memories, search and the memory engine
work.
