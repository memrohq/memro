// Package domain holds the core types shared across the backend. JSON field
// names match the wire format the SDKs and MCP server already use.
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefaultTenantID is the tenant every identity belongs to until multi-tenant
// onboarding exists.
var DefaultTenantID = uuid.Nil

type MemoryType string

const (
	Episodic   MemoryType = "episodic"
	Semantic   MemoryType = "semantic"
	Procedural MemoryType = "procedural"
	Profile    MemoryType = "profile"
)

// ParseMemoryType accepts a type name in any case, with surrounding spaces.
func ParseMemoryType(s string) (MemoryType, bool) {
	switch t := MemoryType(strings.ToLower(strings.TrimSpace(s))); t {
	case Episodic, Semantic, Procedural, Profile:
		return t, true
	}
	return "", false
}

type Visibility string

const (
	Private Visibility = "private"
	Shared  Visibility = "shared"
	Public  Visibility = "public"
)

func ParseVisibility(s string) (Visibility, bool) {
	switch Visibility(s) {
	case Private, Shared, Public:
		return Visibility(s), true
	}
	return "", false
}

type Memory struct {
	ID                uuid.UUID       `json:"id"`
	AgentID           string          `json:"agent_id"`
	TenantID          uuid.UUID       `json:"tenant_id"`
	Content           string          `json:"content"`
	MemoryType        MemoryType      `json:"memory_type"`
	Visibility        Visibility      `json:"visibility"`
	CreatedAt         time.Time       `json:"created_at"`
	FileID            *uuid.UUID      `json:"file_id"`
	AssetURL          *string         `json:"asset_url"`
	DocumentDate      time.Time       `json:"document_date"`
	EventDate         *time.Time      `json:"event_date"`
	SessionID         *uuid.UUID      `json:"session_id"`
	IsAtomic          bool            `json:"is_atomic"`
	EntityTags        []string        `json:"entity_tags"`
	RelationshipTypes []string        `json:"relationship_types"`
	AccessCount       int32           `json:"access_count"`
	LastAccessedAt    time.Time       `json:"last_accessed_at"`
	Confidence        float32         `json:"confidence"`
	Metadata          json.RawMessage `json:"metadata"`
}

type AgentIdentity struct {
	ID        string     `json:"id"`
	UserID    *uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	CreatedAt time.Time  `json:"created_at"`
}

type User struct {
	ID           uuid.UUID
	Email        string
	TenantID     uuid.UUID
	OTPHash      *string
	OTPExpiresAt *time.Time
	OTPAttempts  int
	CreatedAt    time.Time
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TenantID  uuid.UUID
	ExpiresAt time.Time
}

type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Domain    *string   `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RelationType values allowed by the memory_relations CHECK constraint.
type RelationType string

const (
	RelUpdates RelationType = "updates"
	RelExtends RelationType = "extends"
	RelDerives RelationType = "derives"
)

type Relation struct {
	From       uuid.UUID
	To         uuid.UUID
	Type       RelationType
	Confidence float32
}

// --- Search -----------------------------------------------------------------

type SearchQuery struct {
	AgentID    string
	Query      string
	Limit      int
	MemoryType string
	Mode       string // semantic | keyword | hybrid
}

type SearchResult struct {
	MemoryID   string  `json:"memory_id"`
	Content    string  `json:"content"`
	Score      float32 `json:"score"`
	MemoryType string  `json:"memory_type"`
	CreatedAt  string  `json:"created_at"`
	AssetURL   *string `json:"asset_url"`
}

type SearchResponse struct {
	Results   []SearchResult `json:"results"`
	Mode      string         `json:"mode"`
	LatencyMS int64          `json:"latency_ms"`
	Total     int            `json:"total"`
}

// --- Memory engine results ----------------------------------------------------

type ReasoningResult struct {
	Answer     string        `json:"answer"`
	Why        string        `json:"why"`
	BasedOn    []uuid.UUID   `json:"based_on"`
	Confidence float32       `json:"confidence"`
	Paths      [][]uuid.UUID `json:"paths"`
}

// ResolutionResult serializes like the original Rust enum:
// {"Resolved": "<id>"} or {"Ambiguous": ["<a>", "<b>", "<expires_at>"]}.
type ResolutionResult struct {
	Resolved  *uuid.UUID
	A, B      uuid.UUID
	ExpiresAt *string
}

func (r ResolutionResult) MarshalJSON() ([]byte, error) {
	if r.Resolved != nil {
		return json.Marshal(map[string]uuid.UUID{"Resolved": *r.Resolved})
	}
	return json.Marshal(map[string][]any{"Ambiguous": {r.A, r.B, r.ExpiresAt}})
}

func (r *ResolutionResult) UnmarshalJSON(b []byte) error {
	var v struct {
		Resolved  *uuid.UUID
		Ambiguous []json.RawMessage
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Resolved != nil {
		r.Resolved = v.Resolved
		return nil
	}
	if len(v.Ambiguous) != 3 {
		return fmt.Errorf("invalid resolution result")
	}
	if err := json.Unmarshal(v.Ambiguous[0], &r.A); err != nil {
		return err
	}
	if err := json.Unmarshal(v.Ambiguous[1], &r.B); err != nil {
		return err
	}
	return json.Unmarshal(v.Ambiguous[2], &r.ExpiresAt)
}

type EvolutionResult struct {
	MemoriesEvolved    int             `json:"memories_evolved"`
	PatternsIdentified []string        `json:"patterns_identified"`
	Diff               json.RawMessage `json:"diff"`
}

type TimelineEvent struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Details   string `json:"details"`
}

type MemoryTimeline struct {
	MemoryID uuid.UUID       `json:"memory_id"`
	Events   []TimelineEvent `json:"events"`
}
