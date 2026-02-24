# memro Go SDK

Official Go SDK for the memro protocol — AI agent memory infrastructure.

## Installation

```bash
go get github.com/memrohq/memro-sdk-go
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"

	memro "github.com/memrohq/memro-sdk-go"
)

func main() {
	ctx := context.Background()

	// Create a new agent identity
	agent, err := memro.CreateAgent(nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Agent ID: %s\n", agent.AgentID)

	// Store a memory
	mem, err := agent.Remember(ctx, "User prefers Go over Python", &memro.RememberOptions{
		Type: memro.MemoryTypeProfile,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Stored: %s\n", mem.ID)

	// Recall memories semantically
	results, err := agent.Recall(ctx, "what does the user prefer?", &memro.RecallOptions{
		Limit: 5,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Printf("[%.2f] %s\n", r.Score, r.Content)
	}

	// Get recent memories
	recent, err := agent.GetRecent(ctx, &memro.GetRecentOptions{
		Limit: 10,
		Type:  memro.MemoryTypeEpisodic,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range recent {
		fmt.Printf("• %s\n", m.Content)
	}

	// Save identity to file (keep secret!)
	if err := agent.Save("identity.json"); err != nil {
		log.Fatal(err)
	}
}
```

## Features

- **Cryptographic Identity**: ed25519 key generation and management
- **Memory Storage**: Create and manage episodic, semantic, procedural, and profile memories
- **Semantic Search**: Vector-based similarity search
- **Request Signing**: Automatic ed25519 signature generation for authenticated requests
- **Data Export**: Full data portability
- **Zero Dependencies**: Only uses `golang.org/x/crypto` (standard ed25519)

## Configuration

### Connect to memro server

```go
// Custom server URL
client := memro.NewClient(&memro.ClientOptions{
    BaseURL: "https://api.memro.co",
})
agent, err := memro.CreateAgent(client)

// Or use environment variable
// export MEMRO_BASE_URL=https://api.memro.co
agent, err := memro.CreateAgent(nil)
```

### Load existing identity

```go
// From file
agent, err := memro.AgentFromFile("identity.json", nil)

// From environment variables
// export MEMRO_AGENT_ID=...
// export MEMRO_PRIVATE_KEY=...
agent, err := memro.AgentFromEnv(nil)

// From identity struct
agent := memro.AgentFromIdentity(memro.AgentIdentity{
    AgentID:    "5bd80dd5...",
    PrivateKey: "x1y2z3...",
}, nil)
```

## API Reference

### Agent (high-level)

| Method | Description |
|--------|-------------|
| `CreateAgent(client)` | Create a new agent identity |
| `AgentFromFile(path, client)` | Load from JSON file |
| `AgentFromEnv(client)` | Load from env vars |
| `agent.Remember(ctx, content, opts)` | Store a memory |
| `agent.Recall(ctx, query, opts)` | Semantic search |
| `agent.GetRecent(ctx, opts)` | Get recent memories |
| `agent.DeleteMemory(ctx, id)` | Delete a memory |
| `agent.Export(ctx)` | Export all data |
| `agent.DeleteAgent(ctx)` | Delete agent + data |
| `agent.Save(path)` | Save identity to file |

### Client (low-level)

| Method | Description |
|--------|-------------|
| `CreateIdentity(ctx)` | Register new identity |
| `GetIdentity(ctx, agentID)` | Look up identity |
| `DeleteIdentity(ctx, agentID, key)` | Delete identity |
| `CreateMemory(ctx, ...)` | Store memory |
| `GetMemories(ctx, agentID, opts)` | List memories |
| `DeleteMemory(ctx, ...)` | Delete memory |
| `Search(ctx, agentID, query, ...)` | Semantic search |
| `Export(ctx, agentID)` | Export data |
| `Health(ctx)` | Health check |

### Types

```go
type MemoryType string // "episodic", "semantic", "procedural", "profile"
type Visibility string // "private", "shared", "public"

type Memory struct {
    ID         string     `json:"id"`
    AgentID    string     `json:"agent_id"`
    Content    string     `json:"content"`
    MemoryType MemoryType `json:"memory_type"`
    Visibility Visibility `json:"visibility"`
    CreatedAt  string     `json:"created_at"`
}

type SearchResult struct {
    MemoryID   string     `json:"memory_id"`
    Content    string     `json:"content"`
    Score      float64    `json:"score"`
    MemoryType MemoryType `json:"memory_type"`
    CreatedAt  string     `json:"created_at"`
}
```

## Development

```bash
# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Check for issues
go vet ./...
```

## Requirements

- Go 1.21+
- `golang.org/x/crypto` (for ed25519, included via go.mod)

## License

MIT
