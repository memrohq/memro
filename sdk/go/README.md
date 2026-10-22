# Go SDK

Verified memory infrastructure for AI agents.

## Install

```bash
go get github.com/memrohq/memro/sdk/go@v0.1.0
```

```go
import memro "github.com/memrohq/memro/sdk/go"
```

No dependencies outside the standard library. Go 1.21+.

## Quick Example

```go
ctx := context.Background()
agent, _ := memro.CreateAgent(nil) // local Ed25519 sovereign identity
mem, _ := agent.Remember(ctx, "The billing service was rewritten in Go", nil)
results, _ := agent.Recall(ctx, "billing service language", nil)
fmt.Println(results[0].Content)
```

## Connect

```go
ctx := context.Background()

agent, err := memro.AgentFromAPIKey(ctx, os.Getenv("MEMRO_API_KEY"), nil)       // Memro Cloud
agent, err := memro.AgentFromAPIKey(ctx, key, &memro.ClientOptions{BaseURL: "http://localhost:8081"})
agent, err := memro.AgentFromEnv(nil)  // MEMRO_API_KEY, or MEMRO_AGENT_ID + MEMRO_PRIVATE_KEY
```

Self-owned identity:

```go
agent, err := memro.CreateAgent(nil)        // key pair generated locally
err = agent.Save("identity.json")           // 0600; keep secret
agent, err = memro.AgentFromFile("identity.json", nil)
```

## Memories and search

```go
m, err := agent.Remember(ctx, "The billing service was rewritten in Go", &memro.RememberOptions{
	Type:      memro.MemoryTypeSemantic, // empty or MemoryTypeAuto: server classifies
	EventDate: "2026-03-01T00:00:00Z",
	Metadata:  map[string]any{"source": "standup"},
})

results, err := agent.Recall(ctx, "what language is billing in?", &memro.RecallOptions{Limit: 5, Mode: memro.SearchHybrid})
recent, err := agent.RecallTemporal(ctx, "deploy incidents", &memro.TemporalOptions{AfterDate: "2026-09-01T00:00:00Z"})

mems, err := agent.GetRecent(ctx, &memro.GetRecentOptions{Limit: 20})
ok, err := agent.DeleteMemory(ctx, m.ID) // false if it didn't exist
export, err := agent.Export(ctx)
```

## Memory engine

```go
ans, err := agent.Reason(ctx, "why did 500s spike after the billing release?")
evo, err := agent.Evolve(ctx)
res, err := agent.ResolveConflict(ctx, aID, bID)
chain, err := agent.MemoryChain(ctx, m.ID)
tl, err := agent.Timeline(ctx, m.ID)
st, err := agent.Stats(ctx)
```

## Files and webhooks

```go
data, _ := os.ReadFile("notes.pdf")
up, err := agent.UploadFile(ctx, "notes.pdf", "application/pdf", data, true) // true: also store a memory
mem, err := agent.UploadImage(ctx, "board.png", "image/png", png)
chunks, err := agent.UploadDocument(ctx, "handbook.pdf", pdf)

err = agent.AddWebhook(ctx, "https://example.com/memro-hook")
hooks, err := agent.Webhooks(ctx)
ok, err = agent.RemoveWebhook(ctx, "https://example.com/memro-hook")
```

## Errors and retries

```go
if memro.IsRateLimited(err) { /* 429 after retries */ }
if memro.IsNotFound(err) { … }
var apiErr *memro.MemroError
if errors.As(err, &apiErr) { log.Println(apiErr.StatusCode, apiErr.Message) }
```

`ClientOptions{BaseURL, APIKey, HTTPClient, MaxRetries}`. The default HTTP
client has a 30-second timeout. 429s are retried (honouring `Retry-After`);
gateway errors only for `GET` and `DELETE`. Without an API key, requests are
signed with [v2 signatures](https://github.com/memrohq/memro/blob/main/docs/authentication.md#signed-requests)
(`memro.SignRequest`).

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
