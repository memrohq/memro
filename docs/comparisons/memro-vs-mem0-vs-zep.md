# Memro vs Mem0 vs Zep (Graphiti): agent memory compared

Memro, Mem0 and Zep all give AI agents memory that lasts beyond one
conversation. They make different trade-offs. This page sets out those
trade-offs so you can pick the one that fits. It compares design and
features only. We haven't published a head-to-head accuracy benchmark yet,
and we won't quote other projects' numbers next to ours, because they come
from different models, prompts and judges.

*Last checked: October 2026. These projects move quickly. If something here
is out of date, please [open an issue](https://github.com/memrohq/memro/issues).*

## At a glance

| | Memro | Mem0 | Zep / Graphiti |
|---|---|---|---|
| **What it is** | A memory server for agents with a relationship graph between memories | A memory layer that uses an LLM to extract and update facts | Zep Cloud, a managed service built on Graphiti, an open-source temporal knowledge graph framework |
| **License** | [PolyForm Noncommercial 1.0.0](../../LICENSE): free for noncommercial use, commercial use through Memro Cloud or a commercial license | Apache 2.0 | Graphiti: Apache 2.0. Zep Cloud: commercial. Zep Community Edition is deprecated. |
| **Self-hosting** | Yes, with `docker compose up` | Yes, with Docker | Graphiti: yes, as a library you build on. Zep itself: cloud only. |
| **Storage** | One Postgres database with pgvector for memories, graph and embeddings | A vector store, plus an optional graph store | A graph database: Neo4j, FalkorDB or Amazon Neptune |
| **What happens on write** | Stores the text you send. Type, entity tags and dates are found with cheap rules. The only model call is the embedding. | One LLM call per add, to extract memories | An LLM extracts entities and relationships from every episode |
| **How changes over time are handled** | Memories link with `updates`, `extends` and `derives`. Evolution merges recent episodes into semantic facts, and contradictions are flagged and can be resolved. | The LLM extraction step decides what to add | Bi-temporal facts: each one has a validity window and is invalidated when superseded, not deleted |
| **MCP** | Built in, with 20+ tools including multi-agent task claiming and handoff | Through Mem0's editor integrations | Graphiti ships an MCP server |
| **SDKs** | Python, TypeScript, Go | Python, TypeScript | Graphiti: Python. Zep Cloud: Python, TypeScript, Go. |
| **Connectors** | 50 sources: 7 native OAuth (Slack, GitHub, Linear, Sentry, Notion, Jira, GitLab) and 43 declarative | Not a core feature | Not a core feature |
| **Agent identity** | Each agent is an ed25519 key pair, and self-owned agents sign requests with a key that never leaves the machine | API keys | API keys |

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

- **License.** Mem0 and Graphiti are Apache 2.0, so you can build a
  commercial product on them for free. Memro is free only for noncommercial
  use.
- **Temporal modeling.** Graphiti records when each fact became true and
  when it stopped being true. Memro tracks event dates and supersession
  links, but it doesn't yet have a full bi-temporal model.
- **Maturity and ecosystem.** Mem0 and Graphiti have larger communities
  and integrate with many agent frameworks. Memro's LangChain, LangGraph
  and CrewAI adapters are [on the roadmap](../../README.md#roadmap).
- **Fact extraction.** If your input is long, messy conversation and you
  want clean facts with no extra work, the LLM step in Mem0 and Graphiti
  does that for you.

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

## Try Memro

```bash
npx memro-cli connect     # connect Claude Code, Cursor, Codex and other tools
```

Or run it yourself: see [Self-hosting](../self-hosting.md). Read
[Concepts](../concepts.md) for how memories, search and the memory engine
work.
