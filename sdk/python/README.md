# Memro Python SDK

Verified memory infrastructure for AI agents.

## Install

```bash
# PyPI (once published via CI):
pip install memro

# Install from source:
pip install .
```

## Quick Example

```python
from memro import Agent

agent = Agent.create()  # Generates sovereign Ed25519 identity
mem = agent.remember("The billing service was rewritten in Go")
results = agent.recall("billing service language")
print(results[0].content)
```

## Connect

```python
from memro import Agent

agent = Agent.from_env()                       # MEMRO_API_KEY (and optional MEMRO_BASE_URL)
agent = Agent.from_api_key("memro_sk_...")     # explicit key, Memro Cloud
agent = Agent.connect("memro_sk_...", base_url="http://localhost:8081")  # self-hosted
```

Self-owned identity (the private key is generated here and never sent):

```python
agent = Agent.create()                 # registers the public key
agent.save("identity.json")            # keep secret; it can't be recovered
agent = Agent.from_file("identity.json")
```

`Agent.from_env()` also accepts `MEMRO_AGENT_ID` + `MEMRO_PRIVATE_KEY`.

## Memories

```python
m = agent.remember(
    "The billing service was rewritten in Go",
    type="semantic",                     # episodic | semantic | procedural | profile | None (auto)
    visibility="private",
    event_date="2026-03-01T00:00:00Z",   # optional
    session_id=None,                     # optional UUID
    metadata={"source": "standup"},
)
agent.get_recent(limit=20, type="profile")      # list[Memory], newest first
agent.delete_memory(m.id)                       # True, or False if it didn't exist
agent.export()                                  # ExportData(total, memories, ...)
```

## Search

```python
for r in agent.recall("what language is billing in?", limit=5, mode="hybrid", min_score=0.3):
    print(r.score, r.content)                    # SearchResult

agent.recall_temporal("deploy incidents", after_date="2026-09-01T00:00:00Z", temporal_weight=0.6)
```

`mode` is `hybrid` (default), `semantic` or `keyword`.

## Memory engine

```python
ans = agent.reason("why did 500s spike after the billing release?")
print(ans.answer, ans.confidence, ans.based_on)

agent.evolve()                          # EvolutionResult
agent.resolve_conflict(a_id, b_id)      # {"Resolved": id} or {"Ambiguous": [...]}
agent.memory_chain(m.id)                # MemoryChain
agent.timeline(m.id)                    # MemoryTimeline
agent.stats()                           # TemporalStats
agent.usage()                           # dict
```

## Files and webhooks

```python
up = agent.upload_file("notes.pdf")                    # stores the file and a memory of its text
up = agent.upload_file(("notes.txt", b"...", "text/plain"), remember=False)
agent.upload_image("whiteboard.png")                   # Memory with a description
agent.upload_document("handbook.docx")                 # PDF/Word/PowerPoint/Excel/text -> list[Memory]

agent.add_webhook("https://example.com/memro-hook")
agent.webhooks()
agent.remove_webhook("https://example.com/memro-hook")
```

## Errors and retries

```python
from memro import MemroError, RateLimitError

try:
    agent.remember("...")
except RateLimitError:
    ...                     # 429 after the automatic retries
except MemroError as e:
    print(e.status_code, e.message)
```

`MemroClient(timeout=30, max_retries=2)`: rate-limited requests are retried
(honouring `Retry-After`); gateway errors are retried only for `GET` and
`DELETE`, so a memory is never stored twice.

## LangChain

```python
from memro.integrations.langchain import MemroMemory   # pip install "memro[langchain]"
memory = MemroMemory(agent=agent)
```

## Low-level client

`agent.client` is a `MemroClient` with one method per endpoint
(`search`, `search_temporal`, `reason`, `upload_file`, …) and
`request(method, path, params=, json_body=, files=)` for anything else.
Requests are signed with [v2 signatures](https://github.com/memrohq/memro/blob/main/docs/authentication.md#signed-requests)
when there is no API key.

See the [API reference](https://github.com/memrohq/memro/blob/main/docs/api-reference.md) for response fields.

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
