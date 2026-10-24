package memro

import "encoding/json"

// MemoryType is the category of a memory.
type MemoryType string

const (
	MemoryTypeEpisodic   MemoryType = "episodic"   // what happened
	MemoryTypeSemantic   MemoryType = "semantic"   // what is true
	MemoryTypeProcedural MemoryType = "procedural" // how to act
	MemoryTypeProfile    MemoryType = "profile"    // who someone is
	MemoryTypeAuto       MemoryType = "auto"       // let the server classify it
)

// Visibility controls who can read a memory.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityShared  Visibility = "shared"
	VisibilityPublic  Visibility = "public"
)

// SearchMode selects how Recall ranks memories.
type SearchMode string

const (
	SearchHybrid   SearchMode = "hybrid"
	SearchSemantic SearchMode = "semantic"
	SearchKeyword  SearchMode = "keyword"
)

// Memory is a stored memory.
type Memory struct {
	ID           string          `json:"id"`
	AgentID      string          `json:"agent_id"`
	Content      string          `json:"content"`
	MemoryType   MemoryType      `json:"memory_type"`
	Visibility   Visibility      `json:"visibility"`
	CreatedAt    string          `json:"created_at"`
	DocumentDate string          `json:"document_date,omitempty"`
	EventDate    *string         `json:"event_date,omitempty"`
	SessionID    *string         `json:"session_id,omitempty"`
	IsAtomic     bool            `json:"is_atomic"`
	EntityTags   []string        `json:"entity_tags,omitempty"`
	Confidence   float64         `json:"confidence"`
	AccessCount  int             `json:"access_count"`
	FileID       *string         `json:"file_id,omitempty"`
	AssetURL     *string         `json:"asset_url,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

// ResultGraphInfo contains provenance when a search result was discovered through graph expansion.
type ResultGraphInfo struct {
	SeedID        string   `json:"seed_id"`
	Path          []string `json:"path"`
	RelationTypes []string `json:"relation_types"`
	Hops          int      `json:"hops"`
}

// SearchResult is one search hit.
type SearchResult struct {
	MemoryID     string           `json:"memory_id"`
	Content      string           `json:"content"`
	Score        float64          `json:"score"`
	MemoryType   MemoryType       `json:"memory_type"`
	CreatedAt    string           `json:"created_at"`
	DocumentDate *string          `json:"document_date,omitempty"`
	EventDate    *string          `json:"event_date,omitempty"`
	SessionID    *string          `json:"session_id,omitempty"`
	AssetURL     *string          `json:"asset_url,omitempty"`
	IsSuperseded bool             `json:"is_superseded,omitempty"`
	SupersededBy *string          `json:"superseded_by,omitempty"`
	Graph        *ResultGraphInfo `json:"graph,omitempty"`
}

// TemporalResult is one temporal search hit.
type TemporalResult struct {
	MemoryID        string  `json:"memory_id"`
	Content         string  `json:"content"`
	DocumentDate    string  `json:"document_date"`
	EventDate       *string `json:"event_date"`
	IsAtomic        bool    `json:"is_atomic"`
	Score           float64 `json:"score"`
	TemporalContext string  `json:"temporal_context"`
}

// ExportData contains every memory of an agent.
type ExportData struct {
	AgentID    string   `json:"agent_id"`
	ExportedAt string   `json:"exported_at"`
	Total      int      `json:"total"`
	Memories   []Memory `json:"memories"`
}

// ReasoningResult is an answer from memory and the memories it used.
type ReasoningResult struct {
	Answer     string     `json:"answer"`
	Why        string     `json:"why"`
	BasedOn    []string   `json:"based_on"`
	Confidence float64    `json:"confidence"`
	Paths      [][]string `json:"paths"`
}

// ResolutionResult is {"Resolved": id} or {"Ambiguous": [a, b, expires_at]}.
type ResolutionResult struct {
	Resolved  *string           `json:"Resolved,omitempty"`
	Ambiguous []json.RawMessage `json:"Ambiguous,omitempty"`
}

// EvolutionResult reports what Evolve consolidated.
type EvolutionResult struct {
	MemoriesEvolved    int             `json:"memories_evolved"`
	PatternsIdentified []string        `json:"patterns_identified"`
	Diff               json.RawMessage `json:"diff"`
}

// TimelineEvent is one audit event for a memory.
type TimelineEvent struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Details   string `json:"details"`
}

// MemoryTimeline is the history of one memory.
type MemoryTimeline struct {
	MemoryID string          `json:"memory_id"`
	Events   []TimelineEvent `json:"events"`
}

// ChainItem is a memory in a knowledge chain.
type ChainItem struct {
	MemoryID     string   `json:"memory_id"`
	Content      string   `json:"content"`
	RelationType *string  `json:"relation_type"`
	Confidence   *float64 `json:"confidence"`
}

// MemoryChain is a memory and every memory linked to it.
type MemoryChain struct {
	RootMemory      ChainItem   `json:"root_memory"`
	RelatedMemories []ChainItem `json:"related_memories"`
}

// TemporalStats summarizes an agent's memories over time.
type TemporalStats struct {
	TotalMemories              int64            `json:"total_memories"`
	MemoriesToday              int64            `json:"memories_today"`
	MemoriesThisWeek           int64            `json:"memories_this_week"`
	MemoriesThisMonth          int64            `json:"memories_this_month"`
	AverageDaysBetweenMemories float64          `json:"average_days_between_memories"`
	MemoryTypesDistribution    map[string]int64 `json:"memory_types_distribution"`
}

// ConflictAnalysis counts contradictions the engine found.
type ConflictAnalysis struct {
	TotalConflicts       int64   `json:"total_conflicts"`
	UnresolvedConflicts  int64   `json:"unresolved_conflicts"`
	AvgResolutionTimeSec float64 `json:"avg_resolution_time_sec"`
}

// Usage is today's usage for the agent.
type Usage struct {
	ReasonCalls int64   `json:"reason_calls"`
	EvolveCalls int64   `json:"evolve_calls"`
	MemoryNodes int64   `json:"memory_nodes"`
	GraphEdges  int64   `json:"graph_edges"`
	Warning     *string `json:"warning"`
}

// UploadedFile describes a stored file.
type UploadedFile struct {
	FileID        string  `json:"file_id"`
	FileName      string  `json:"file_name"`
	FileType      string  `json:"file_type"`
	FileSize      int64   `json:"file_size"`
	ExtractedText *string `json:"extracted_text"`
	MemoryID      string  `json:"memory_id,omitempty"` // set when also stored as a memory
}

// WhoAmI is the identity behind a credential.
type WhoAmI struct {
	AgentID  string  `json:"agent_id"`
	TenantID string  `json:"tenant_id"`
	UserID   *string `json:"user_id"`
	Auth     string  `json:"auth"` // api_key | session | signature
}

// HealthStatus is the server health response.
type HealthStatus struct {
	Status      string `json:"status"`       // ok | degraded
	DB          string `json:"db"`           // ok | error
	VectorStore string `json:"vector_store"` // ok | unavailable
	Embeddings  string `json:"embeddings"`   // enabled | disabled
}

// AgentIdentity holds a self-owned agent's credentials.
type AgentIdentity struct {
	AgentID    string `json:"agent_id"`
	PrivateKey string `json:"private_key"`
}

// RememberOptions are optional parameters for Agent.Remember.
type RememberOptions struct {
	Type       MemoryType // empty or MemoryTypeAuto: server classifies
	Visibility Visibility // default private
	EventDate  string     // RFC 3339
	SessionID  string     // UUID grouping one conversation
	IsAtomic   bool
	Metadata   map[string]any
}

// RecallOptions are optional parameters for Agent.Recall.
type RecallOptions struct {
	Limit    int // default 10, max 50
	Type     MemoryType
	Mode     SearchMode // default hybrid
	MinScore float64
	Graph    string // "legacy", "bounded", "rerank", "selective"
}

// TemporalOptions are optional parameters for Agent.RecallTemporal.
type TemporalOptions struct {
	Limit           int
	AfterDate       string // RFC 3339
	BeforeDate      string
	AfterEventDate  string
	BeforeEventDate string
	SessionID       string
	AtomicOnly      *bool
	TemporalWeight  *float64 // 0..1, default 0.3
}

// GetRecentOptions are optional parameters for Agent.GetRecent.
type GetRecentOptions struct {
	Limit      int
	Type       MemoryType
	Visibility Visibility
}
