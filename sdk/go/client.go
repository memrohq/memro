package memro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

// MemroError is returned when the server responds with a non-2xx status.
type MemroError struct {
	StatusCode int
	Message    string
}

func (e *MemroError) Error() string {
	return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
}

// ClientOptions configures the low-level HTTP client.
type ClientOptions struct {
	BaseURL    string       // defaults to MEMRO_BASE_URL or http://localhost:8081
	HTTPClient *http.Client // defaults to http.DefaultClient
}

// Client is the low-level HTTP client for the memro protocol API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new low-level memro API client.
func NewClient(opts *ClientOptions) *Client {
	baseURL := "http://localhost:8081"
	var httpClient *http.Client

	if opts != nil {
		if opts.BaseURL != "" {
			baseURL = opts.BaseURL
		}
		if opts.HTTPClient != nil {
			httpClient = opts.HTTPClient
		}
	}
	if baseURL == "http://localhost:8081" {
		if envURL := os.Getenv("MEMRO_BASE_URL"); envURL != "" {
			baseURL = envURL
		}
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// --- helpers ---

func (c *Client) doRequest(ctx context.Context, method, path string, headers map[string]string, body []byte) ([]byte, int, error) {
	u := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("memro: build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("memro: request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("memro: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, &MemroError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	}

	return respBody, resp.StatusCode, nil
}

func (c *Client) authHeaders(agentID, privateKey string, body []byte) (map[string]string, error) {
	sh, err := SignBody(agentID, privateKey, body)
	if err != nil {
		return nil, err
	}
	return sh.Headers(), nil
}

// ── Identity ────────────────────────────────────────────────────────────────

// CreateIdentity registers a new agent identity on the server.
func (c *Client) CreateIdentity(ctx context.Context) (*AgentIdentity, error) {
	data, _, err := c.doRequest(ctx, http.MethodPost, "/identity", nil, nil)
	if err != nil {
		return nil, err
	}
	var resp createIdentityResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("memro: decode identity: %w", err)
	}
	return &AgentIdentity{AgentID: resp.AgentID, PrivateKey: resp.PrivateKey}, nil
}

// GetIdentity looks up an identity by agent ID.
func (c *Client) GetIdentity(ctx context.Context, agentID string) (*getIdentityResponse, error) {
	data, _, err := c.doRequest(ctx, http.MethodGet, "/identity/"+agentID, nil, nil)
	if err != nil {
		return nil, err
	}
	var resp getIdentityResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("memro: decode identity: %w", err)
	}
	return &resp, nil
}

// DeleteIdentity permanently deletes an agent and all its memories.
func (c *Client) DeleteIdentity(ctx context.Context, agentID, privateKey string) error {
	headers, err := c.authHeaders(agentID, privateKey, []byte(agentID))
	if err != nil {
		return err
	}
	_, _, err = c.doRequest(ctx, http.MethodDelete, "/identity/"+agentID, headers, nil)
	return err
}

// ── Memory ──────────────────────────────────────────────────────────────────

// CreateMemory stores a new memory.
func (c *Client) CreateMemory(ctx context.Context, agentID, privateKey, content string, memoryType MemoryType, visibility Visibility) (*Memory, error) {
	reqBody := createMemoryRequest{
		AgentID:    agentID,
		Content:    content,
		MemoryType: memoryType,
		Visibility: visibility,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("memro: marshal request: %w", err)
	}

	headers, err := c.authHeaders(agentID, privateKey, bodyBytes)
	if err != nil {
		return nil, err
	}

	data, _, err := c.doRequest(ctx, http.MethodPost, "/memory", headers, bodyBytes)
	if err != nil {
		return nil, err
	}

	var mem Memory
	if err := json.Unmarshal(data, &mem); err != nil {
		return nil, fmt.Errorf("memro: decode memory: %w", err)
	}
	return &mem, nil
}

// GetMemories retrieves memories for an agent with optional filters.
func (c *Client) GetMemories(ctx context.Context, agentID string, opts *GetRecentOptions) ([]Memory, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Type != "" {
			params.Set("memory_type", string(opts.Type))
		}
		if opts.Visibility != "" {
			params.Set("visibility", string(opts.Visibility))
		}
		if opts.Limit > 0 {
			limit := opts.Limit
			if limit > 100 {
				limit = 100
			}
			params.Set("limit", strconv.Itoa(limit))
		}
	}

	path := "/memory/" + agentID
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	data, _, err := c.doRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}

	var memories []Memory
	if err := json.Unmarshal(data, &memories); err != nil {
		return nil, fmt.Errorf("memro: decode memories: %w", err)
	}
	return memories, nil
}

// DeleteMemory deletes a specific memory. Returns true if deleted, false if not found.
func (c *Client) DeleteMemory(ctx context.Context, agentID, privateKey, memoryID string) (bool, error) {
	headers, err := c.authHeaders(agentID, privateKey, []byte(memoryID))
	if err != nil {
		return false, err
	}

	_, status, err := c.doRequest(ctx, http.MethodDelete, "/memory/"+memoryID, headers, nil)
	if err != nil {
		if me, ok := err.(*MemroError); ok && me.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return status == 204 || status == 200, nil
}

// ── Search ──────────────────────────────────────────────────────────────────

// Search performs a semantic search over an agent's memories.
func (c *Client) Search(ctx context.Context, agentID, query string, limit int, memoryType MemoryType) ([]SearchResult, error) {
	params := url.Values{
		"agent_id": {agentID},
		"query":    {query},
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	params.Set("limit", strconv.Itoa(limit))
	if memoryType != "" {
		params.Set("memory_type", string(memoryType))
	}

	data, _, err := c.doRequest(ctx, http.MethodGet, "/search?"+params.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}

	var resp searchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("memro: decode search: %w", err)
	}
	return resp.Results, nil
}

// ── Export ───────────────────────────────────────────────────────────────────

// Export retrieves all memories for an agent.
func (c *Client) Export(ctx context.Context, agentID string) (*ExportData, error) {
	data, _, err := c.doRequest(ctx, http.MethodGet, "/export/"+agentID, nil, nil)
	if err != nil {
		return nil, err
	}
	var export ExportData
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("memro: decode export: %w", err)
	}
	return &export, nil
}

// ── Health ───────────────────────────────────────────────────────────────────

// Health checks the server health.
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	data, _, err := c.doRequest(ctx, http.MethodGet, "/health", nil, nil)
	if err != nil {
		return nil, err
	}
	var status HealthStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("memro: decode health: %w", err)
	}
	return &status, nil
}
