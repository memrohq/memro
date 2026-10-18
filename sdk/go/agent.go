package memro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// Agent is the high-level interface for the memro protocol.
// It wraps a Client with a specific agent identity for convenience.
type Agent struct {
	AgentID    string
	privateKey string
	Client     *Client
}

// NewAgent creates an Agent from existing credentials.
func NewAgent(agentID, privateKey string, client *Client) *Agent {
	if client == nil {
		client = NewClient(nil)
	}
	return &Agent{AgentID: agentID, privateKey: privateKey, Client: client}
}

// ── Constructors ────────────────────────────────────────────────────────────

// CreateAgent registers a new agent identity on the memro server.
func CreateAgent(client *Client) (*Agent, error) {
	if client == nil {
		client = NewClient(nil)
	}
	identity, err := client.CreateIdentity(context.Background())
	if err != nil {
		return nil, fmt.Errorf("memro: create agent: %w", err)
	}
	return &Agent{
		AgentID:    identity.AgentID,
		privateKey: identity.PrivateKey,
		Client:     client,
	}, nil
}

// AgentFromFile loads an agent identity from a JSON file.
func AgentFromFile(path string, client *Client) (*Agent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("memro: read identity file: %w", err)
	}
	var identity AgentIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("memro: decode identity file: %w", err)
	}
	if client == nil {
		client = NewClient(nil)
	}
	return &Agent{
		AgentID:    identity.AgentID,
		privateKey: identity.PrivateKey,
		Client:     client,
	}, nil
}

// AgentFromEnv loads an agent from MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY
// environment variables.
func AgentFromEnv(client *Client) (*Agent, error) {
	agentID := os.Getenv("MEMRO_AGENT_ID")
	privateKey := os.Getenv("MEMRO_PRIVATE_KEY")
	if agentID == "" || privateKey == "" {
		return nil, fmt.Errorf("memro: MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY must be set")
	}
	if client == nil {
		client = NewClient(nil)
	}
	return &Agent{
		AgentID:    agentID,
		privateKey: privateKey,
		Client:     client,
	}, nil
}

// AgentFromIdentity creates an Agent from an AgentIdentity struct.
func AgentFromIdentity(identity AgentIdentity, client *Client) *Agent {
	if client == nil {
		client = NewClient(nil)
	}
	return &Agent{
		AgentID:    identity.AgentID,
		privateKey: identity.PrivateKey,
		Client:     client,
	}
}

// ── Persistence ─────────────────────────────────────────────────────────────

// Save writes the agent identity to a JSON file. Keep this file secret.
func (a *Agent) Save(path string) error {
	data, err := json.MarshalIndent(AgentIdentity{
		AgentID:    a.AgentID,
		PrivateKey: a.privateKey,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("memro: marshal identity: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// Identity returns the agent's identity (agent_id + private_key).
func (a *Agent) Identity() AgentIdentity {
	return AgentIdentity{AgentID: a.AgentID, PrivateKey: a.privateKey}
}

// ── Memory operations ───────────────────────────────────────────────────────

// Remember stores a new memory. Pass nil for opts to use defaults (episodic, private).
func (a *Agent) Remember(ctx context.Context, content string, opts *RememberOptions) (*Memory, error) {
	memType := MemoryTypeEpisodic
	vis := VisibilityPrivate
	if opts != nil {
		if opts.Type != "" {
			memType = opts.Type
		}
		if opts.Visibility != "" {
			vis = opts.Visibility
		}
	}
	return a.Client.CreateMemory(ctx, a.AgentID, a.privateKey, content, memType, vis)
}

// Recall performs a semantic search over the agent's memories.
// Pass nil for opts to use defaults (limit 10, no type filter).
func (a *Agent) Recall(ctx context.Context, query string, opts *RecallOptions) ([]SearchResult, error) {
	limit := 10
	var memType MemoryType
	var minScore float64

	if opts != nil {
		if opts.Limit > 0 {
			limit = opts.Limit
		}
		memType = opts.Type
		minScore = opts.MinScore
	}

	results, err := a.Client.Search(ctx, a.AgentID, query, limit, memType)
	if err != nil {
		return nil, err
	}

	if minScore > 0 {
		filtered := make([]SearchResult, 0, len(results))
		for _, r := range results {
			if r.Score >= minScore {
				filtered = append(filtered, r)
			}
		}
		return filtered, nil
	}
	return results, nil
}

// GetRecent retrieves recent memories without semantic search.
// Pass nil for opts to use defaults.
func (a *Agent) GetRecent(ctx context.Context, opts *GetRecentOptions) ([]Memory, error) {
	return a.Client.GetMemories(ctx, a.AgentID, opts)
}

// DeleteMemory deletes a specific memory by ID. Returns true if deleted.
func (a *Agent) DeleteMemory(ctx context.Context, memoryID string) (bool, error) {
	return a.Client.DeleteMemory(ctx, a.AgentID, a.privateKey, memoryID)
}

// Export retrieves all memories for this agent.
func (a *Agent) Export(ctx context.Context) (*ExportData, error) {
	return a.Client.Export(ctx, a.AgentID)
}

// DeleteAgent permanently deletes this agent and all its memories.
func (a *Agent) DeleteAgent(ctx context.Context) error {
	return a.Client.DeleteIdentity(ctx, a.AgentID, a.privateKey)
}

// String returns a short representation of the agent.
func (a *Agent) String() string {
	id := a.AgentID
	if len(id) > 16 {
		id = id[:16]
	}
	return fmt.Sprintf("Agent(id=%s...)", id)
}
