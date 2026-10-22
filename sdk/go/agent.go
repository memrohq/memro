package memro

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Agent is an AI agent with persistent memory. Create one with
// AgentFromAPIKey (Memro Cloud), AgentFromEnv, or CreateAgent for a
// self-owned identity.
type Agent struct {
	AgentID string
	Client  *Client
	signer  *Signer
}

// NewAgent wraps existing credentials. privateKey may be empty when the
// client has an API key.
func NewAgent(agentID, privateKey string, client *Client) *Agent {
	if client == nil {
		client = NewClient(nil)
	}
	a := &Agent{AgentID: agentID, Client: client}
	if privateKey != "" {
		a.signer = &Signer{AgentID: agentID, PrivateKey: privateKey}
	}
	return a
}

// AgentFromAPIKey connects with an API key (memro_sk_...) from
// `npx memro-cli connect` or the dashboard. opts.APIKey is ignored.
func AgentFromAPIKey(ctx context.Context, apiKey string, opts *ClientOptions) (*Agent, error) {
	o := ClientOptions{}
	if opts != nil {
		o = *opts
	}
	o.APIKey = apiKey
	c := NewClient(&o)
	who, err := c.WhoAmI(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Agent{AgentID: who.AgentID, Client: c}, nil
}

// CreateAgent creates a self-owned identity: the key pair is generated here
// and only the public key is sent. Save it; it can't be recovered.
func CreateAgent(client *Client) (*Agent, error) {
	if client == nil {
		client = NewClient(nil)
	}
	id, err := client.CreateIdentity(context.Background())
	if err != nil {
		return nil, err
	}
	return NewAgent(id.AgentID, id.PrivateKey, client), nil
}

// AgentFromEnv uses MEMRO_API_KEY, or MEMRO_AGENT_ID + MEMRO_PRIVATE_KEY.
func AgentFromEnv(client *Client) (*Agent, error) {
	if key := os.Getenv("MEMRO_API_KEY"); key != "" {
		opts := &ClientOptions{}
		if client != nil {
			opts.BaseURL, opts.HTTPClient = client.baseURL, client.httpClient
		}
		return AgentFromAPIKey(context.Background(), key, opts)
	}
	id, key := os.Getenv("MEMRO_AGENT_ID"), os.Getenv("MEMRO_PRIVATE_KEY")
	if id == "" || key == "" {
		return nil, fmt.Errorf("memro: set MEMRO_API_KEY, or MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY")
	}
	return NewAgent(id, key, client), nil
}

// AgentFromIdentity wraps a saved self-owned identity.
func AgentFromIdentity(identity AgentIdentity, client *Client) *Agent {
	return NewAgent(identity.AgentID, identity.PrivateKey, client)
}

// AgentFromFile loads an identity saved with Save.
func AgentFromFile(path string, client *Client) (*Agent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("memro: read identity file: %w", err)
	}
	var id AgentIdentity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("memro: parse identity file: %w", err)
	}
	return AgentFromIdentity(id, client), nil
}

// Save writes a self-owned identity with owner-only permissions. Keep it secret.
func (a *Agent) Save(path string) error {
	if a.signer == nil {
		return fmt.Errorf("memro: this agent uses an API key and has no private key to save")
	}
	data, _ := json.MarshalIndent(a.Identity(), "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Identity returns the self-owned credentials (empty PrivateKey with an API key).
func (a *Agent) Identity() AgentIdentity {
	id := AgentIdentity{AgentID: a.AgentID}
	if a.signer != nil {
		id.PrivateKey = a.signer.PrivateKey
	}
	return id
}

// Signer returns the agent's signing credentials, or nil with an API key.
func (a *Agent) Signer() *Signer { return a.signer }

// ── Memories ──────────────────────────────────────────────────────────────

// Remember stores a memory (max 10,000 characters). opts may be nil.
func (a *Agent) Remember(ctx context.Context, content string, opts *RememberOptions) (*Memory, error) {
	if opts == nil {
		opts = &RememberOptions{}
	}
	body := map[string]any{"agent_id": a.AgentID, "content": content, "visibility": VisibilityPrivate, "is_atomic": opts.IsAtomic}
	if opts.Type != "" {
		body["memory_type"] = opts.Type
	}
	if opts.Visibility != "" {
		body["visibility"] = opts.Visibility
	}
	if opts.EventDate != "" {
		body["event_date"] = opts.EventDate
	}
	if opts.SessionID != "" {
		body["session_id"] = opts.SessionID
	}
	if opts.Metadata != nil {
		body["metadata"] = opts.Metadata
	}
	return a.Client.CreateMemory(ctx, body, a.signer)
}

// Recall searches memories (hybrid by default). opts may be nil.
func (a *Agent) Recall(ctx context.Context, query string, opts *RecallOptions) ([]SearchResult, error) {
	if opts == nil {
		opts = &RecallOptions{}
	}
	q := url.Values{"agent_id": {a.AgentID}, "query": {query}, "limit": {strconv.Itoa(clamp(opts.Limit, 10, 50))}}
	if opts.Mode != "" {
		q.Set("mode", string(opts.Mode))
	}
	if opts.Type != "" {
		q.Set("memory_type", string(opts.Type))
	}
	res, err := a.Client.Search(ctx, q, a.signer)
	if err != nil || opts.MinScore <= 0 {
		return res, err
	}
	kept := res[:0]
	for _, r := range res {
		if r.Score >= opts.MinScore {
			kept = append(kept, r)
		}
	}
	return kept, nil
}

// RecallTemporal searches with date filters and a recency boost. opts may be nil.
func (a *Agent) RecallTemporal(ctx context.Context, query string, opts *TemporalOptions) ([]TemporalResult, error) {
	if opts == nil {
		opts = &TemporalOptions{}
	}
	q := url.Values{"agent_id": {a.AgentID}, "query": {query}, "limit": {strconv.Itoa(clamp(opts.Limit, 10, 50))}}
	for k, v := range map[string]string{"after_date": opts.AfterDate, "before_date": opts.BeforeDate,
		"after_event_date": opts.AfterEventDate, "before_event_date": opts.BeforeEventDate, "session_id": opts.SessionID} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if opts.AtomicOnly != nil {
		q.Set("atomic_only", strconv.FormatBool(*opts.AtomicOnly))
	}
	if opts.TemporalWeight != nil {
		q.Set("temporal_weight", strconv.FormatFloat(*opts.TemporalWeight, 'f', -1, 64))
	}
	return a.Client.SearchTemporal(ctx, q, a.signer)
}

// GetRecent returns the most recent memories, newest first. opts may be nil.
func (a *Agent) GetRecent(ctx context.Context, opts *GetRecentOptions) ([]Memory, error) {
	if opts == nil {
		opts = &GetRecentOptions{}
	}
	q := url.Values{"limit": {strconv.Itoa(clamp(opts.Limit, 10, 100))}}
	if opts.Type != "" {
		q.Set("memory_type", string(opts.Type))
	}
	if opts.Visibility != "" {
		q.Set("visibility", string(opts.Visibility))
	}
	return a.Client.ListMemories(ctx, a.AgentID, q, a.signer)
}

// DeleteMemory deletes a memory; false means it did not exist.
func (a *Agent) DeleteMemory(ctx context.Context, memoryID string) (bool, error) {
	return a.Client.DeleteMemory(ctx, memoryID, a.signer)
}

// Export returns every memory.
func (a *Agent) Export(ctx context.Context) (*ExportData, error) {
	return a.Client.Export(ctx, a.AgentID, a.signer)
}

// ── Memory engine ─────────────────────────────────────────────────────────

// Reason answers a question from memory, following the knowledge graph for why/how questions.
func (a *Agent) Reason(ctx context.Context, query string) (*ReasoningResult, error) {
	return a.Client.Reason(ctx, query, a.signer)
}

// ResolveConflict picks the more trustworthy of two memories.
func (a *Agent) ResolveConflict(ctx context.Context, memoryAID, memoryBID string) (*ResolutionResult, error) {
	return a.Client.ResolveConflict(ctx, memoryAID, memoryBID, a.signer)
}

// Evolve consolidates repeated episodes into semantic memories.
func (a *Agent) Evolve(ctx context.Context) (*EvolutionResult, error) {
	return a.Client.Evolve(ctx, a.signer)
}

// Timeline returns a memory's history.
func (a *Agent) Timeline(ctx context.Context, memoryID string) (*MemoryTimeline, error) {
	return a.Client.Timeline(ctx, memoryID, a.signer)
}

// MemoryChain returns a memory and every memory linked to it.
func (a *Agent) MemoryChain(ctx context.Context, memoryID string) (*MemoryChain, error) {
	return a.Client.MemoryChain(ctx, memoryID, a.signer)
}

// AnalyzeConflicts counts contradictions the engine found.
func (a *Agent) AnalyzeConflicts(ctx context.Context) (*ConflictAnalysis, error) {
	return a.Client.AnalyzeConflicts(ctx, a.signer)
}

// Stats summarizes memories over time.
func (a *Agent) Stats(ctx context.Context) (*TemporalStats, error) {
	return a.Client.Stats(ctx, a.signer)
}

// Usage returns today's usage.
func (a *Agent) Usage(ctx context.Context) (*Usage, error) { return a.Client.Usage(ctx, a.signer) }

// ── Files ─────────────────────────────────────────────────────────────────

// UploadFile stores a file and, when remember is true, stores its extracted
// text as a memory (UploadedFile.MemoryID).
func (a *Agent) UploadFile(ctx context.Context, name, contentType string, data []byte, remember bool) (*UploadedFile, error) {
	up, err := a.Client.UploadFile(ctx, a.AgentID, name, contentType, data, a.signer)
	if err != nil || !remember {
		return up, err
	}
	up.MemoryID, err = a.Client.MemoryFromFile(ctx, a.AgentID, up.FileID, MemoryTypeSemantic, a.signer)
	return up, err
}

// UploadImage describes an image with vision and stores the description.
func (a *Agent) UploadImage(ctx context.Context, name, contentType string, data []byte) (*Memory, error) {
	return a.Client.UploadImage(ctx, name, contentType, data, a.signer)
}

// UploadDocument stores a PDF, Word, PowerPoint, Excel or text document as
// ~1,500-character overlapping memories.
func (a *Agent) UploadDocument(ctx context.Context, name string, data []byte) ([]Memory, error) {
	return a.Client.UploadDocument(ctx, name, data, a.signer)
}

// ── Webhooks ──────────────────────────────────────────────────────────────

// AddWebhook POSTs each new memory of this agent to hookURL (public http(s) only).
func (a *Agent) AddWebhook(ctx context.Context, hookURL string) error {
	return a.Client.RegisterWebhook(ctx, "memory_created", hookURL, a.signer)
}

// RemoveWebhook removes a memory_created webhook; false if it was not registered.
func (a *Agent) RemoveWebhook(ctx context.Context, hookURL string) (bool, error) {
	return a.Client.DeleteWebhook(ctx, "memory_created", hookURL, a.signer)
}

// Webhooks lists this agent's memory_created webhooks.
func (a *Agent) Webhooks(ctx context.Context) ([]string, error) {
	return a.Client.ListWebhooks(ctx, "memory_created", a.signer)
}

// DeleteAgent permanently deletes this agent and everything it stored. It
// needs the agent's private key; API keys cannot delete agents (403).
func (a *Agent) DeleteAgent(ctx context.Context) error {
	return a.Client.DeleteIdentity(ctx, a.AgentID, a.signer)
}

// ── Coordination ──────────────────────────────────────────────────────────

type ClaimOptions struct {
	Intent     string
	SessionID  string
	TTLSeconds int
}

type HeartbeatOptions struct {
	LeaseID    string
	TTLSeconds int
}

func (a *Agent) Claim(ctx context.Context, resource string, opts *ClaimOptions) (map[string]any, error) {
	body := map[string]any{"resource": resource}
	if opts != nil {
		if opts.Intent != "" {
			body["intent"] = opts.Intent
		}
		if opts.SessionID != "" {
			body["session_id"] = opts.SessionID
		}
		if opts.TTLSeconds > 0 {
			body["ttl_seconds"] = opts.TTLSeconds
		}
	}
	return a.Client.CoordinationClaim(ctx, body, a.signer)
}

func (a *Agent) Heartbeat(ctx context.Context, resource string, opts *HeartbeatOptions) (map[string]any, error) {
	body := map[string]any{}
	if resource != "" {
		body["resource"] = resource
	}
	if opts != nil {
		if opts.LeaseID != "" {
			body["lease_id"] = opts.LeaseID
		}
		if opts.TTLSeconds > 0 {
			body["ttl_seconds"] = opts.TTLSeconds
		}
	}
	return a.Client.CoordinationHeartbeat(ctx, body, a.signer)
}

func (a *Agent) Release(ctx context.Context, resource, status string) error {
	if status == "" {
		status = "completed"
	}
	return a.Client.CoordinationRelease(ctx, map[string]any{"resource": resource, "status": status}, a.signer)
}

func (a *Agent) Handoff(ctx context.Context, resource, summary, nextAgentID string) error {
	body := map[string]any{"resource": resource, "summary": summary}
	if nextAgentID != "" {
		body["next_agent_id"] = nextAgentID
	}
	return a.Client.CoordinationHandoff(ctx, body, a.signer)
}

func (a *Agent) Check(ctx context.Context, resource, intent string) (map[string]any, error) {
	return a.Client.CoordinationCheck(ctx, map[string]any{"resource": resource, "intent": intent}, a.signer)
}

func (a *Agent) Active(ctx context.Context, prefix string) (map[string]any, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("resource", prefix)
	}
	return a.Client.CoordinationActive(ctx, q, a.signer)
}

func (a *Agent) LogIntent(ctx context.Context, resource, intent string, ttlSeconds int) (map[string]any, error) {
	body := map[string]any{"resource": resource, "intent": intent}
	if ttlSeconds > 0 {
		body["ttl_seconds"] = ttlSeconds
	}
	return a.Client.CoordinationIntent(ctx, body, a.signer)
}

func (a *Agent) String() string {
	if len(a.AgentID) > 16 {
		return "Agent(" + a.AgentID[:16] + "…)"
	}
	return "Agent(" + a.AgentID + ")"
}

func clamp(v, def, maxV int) int {
	if v <= 0 {
		return def
	}
	return min(v, maxV)
}
