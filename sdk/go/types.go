package memro

// MemoryType represents the category of a memory.
type MemoryType string

const (
	MemoryTypeEpisodic   MemoryType = "episodic"
	MemoryTypeSemantic   MemoryType = "semantic"
	MemoryTypeProcedural MemoryType = "procedural"
	MemoryTypeProfile    MemoryType = "profile"
)

// Visibility controls who can read a memory.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityShared  Visibility = "shared"
	VisibilityPublic  Visibility = "public"
)

// Memory is a single stored memory returned by the server.
type Memory struct {
	ID         string     `json:"id"`
	AgentID    string     `json:"agent_id"`
	Content    string     `json:"content"`
	MemoryType MemoryType `json:"memory_type"`
	Visibility Visibility `json:"visibility"`
	CreatedAt  string     `json:"created_at"`
}

// SearchResult is a single result from a semantic search.
type SearchResult struct {
	MemoryID   string     `json:"memory_id"`
	Content    string     `json:"content"`
	Score      float64    `json:"score"`
	MemoryType MemoryType `json:"memory_type"`
	CreatedAt  string     `json:"created_at"`
}

// ExportData contains all memories for an agent.
type ExportData struct {
	AgentID    string   `json:"agent_id"`
	ExportedAt string   `json:"exported_at"`
	Total      int      `json:"total"`
	Memories   []Memory `json:"memories"`
}

// HealthStatus is the server health response.
type HealthStatus struct {
	Status      string `json:"status"`       // "ok" or "degraded"
	DB          string `json:"db"`           // "ok" or "error"
	VectorStore string `json:"vector_store"` // "ok" or "unavailable"
}

// AgentIdentity holds the credentials needed to authenticate an agent.
type AgentIdentity struct {
	AgentID    string `json:"agent_id"`
	PrivateKey string `json:"private_key"`
}

// RememberOptions are optional parameters for Agent.Remember.
type RememberOptions struct {
	Type       MemoryType
	Visibility Visibility
}

// RecallOptions are optional parameters for Agent.Recall.
type RecallOptions struct {
	Limit    int
	Type     MemoryType
	MinScore float64
}

// GetRecentOptions are optional parameters for Agent.GetRecent.
type GetRecentOptions struct {
	Limit      int
	Type       MemoryType
	Visibility Visibility
}

// createIdentityResponse is the server response when creating a new identity.
type createIdentityResponse struct {
	AgentID    string `json:"agent_id"`
	PrivateKey string `json:"private_key"`
}

// getIdentityResponse is the server response for identity lookup.
type getIdentityResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

// searchResponse wraps the array returned by /search.
type searchResponse struct {
	Results []SearchResult `json:"results"`
}

// createMemoryRequest is the JSON body for POST /memory.
type createMemoryRequest struct {
	AgentID    string     `json:"agent_id"`
	Content    string     `json:"content"`
	MemoryType MemoryType `json:"memory_type"`
	Visibility Visibility `json:"visibility"`
}
