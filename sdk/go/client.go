package memro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is Memro Cloud's API.
	DefaultBaseURL = "https://api.memro.co"
	// APIKeyPrefix starts every Memro API key.
	APIKeyPrefix = "memro_sk_"
)

// MemroError is returned when the server responds with a non-2xx status.
type MemroError struct {
	StatusCode int
	Message    string
}

func (e *MemroError) Error() string { return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message) }

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var e *MemroError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// IsRateLimited reports whether err is a 429 from the API.
func IsRateLimited(err error) bool {
	var e *MemroError
	return errors.As(err, &e) && e.StatusCode == http.StatusTooManyRequests
}

// ClientOptions configures a Client.
type ClientOptions struct {
	BaseURL    string       // default MEMRO_BASE_URL, then https://api.memro.co
	APIKey     string       // memro_sk_...; default MEMRO_API_KEY
	HTTPClient *http.Client // default: 30 s timeout
	MaxRetries *int         // retries for rate limits and gateway errors on safe requests (default 2)
}

// Client is the low-level HTTP client: one method per endpoint. Most code
// should use Agent, which keeps the credentials.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
}

// Signer is a self-owned agent's credentials, used when there is no API key.
type Signer struct {
	AgentID    string
	PrivateKey string
}

// NewClient creates a client. opts may be nil.
func NewClient(opts *ClientOptions) *Client {
	if opts == nil {
		opts = &ClientOptions{}
	}
	c := &Client{baseURL: opts.BaseURL, apiKey: opts.APIKey, httpClient: opts.HTTPClient, maxRetries: 2}
	if c.baseURL == "" {
		c.baseURL = os.Getenv("MEMRO_BASE_URL")
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	if c.apiKey == "" {
		c.apiKey = os.Getenv("MEMRO_API_KEY")
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.MaxRetries != nil {
		c.maxRetries = *opts.MaxRetries
	}
	return c
}

// BaseURL returns the API base URL.
func (c *Client) BaseURL() string { return c.baseURL }

type request struct {
	method string
	path   string
	query  url.Values
	body   []byte
	ctype  string
	signer *Signer
	noAuth bool
}

// do sends a request and decodes a JSON response into out (if non-nil). The
// signature covers the exact path, query string and body sent.
func (c *Client) do(ctx context.Context, r request, out any) error {
	if c.apiKey != "" && !strings.HasPrefix(c.apiKey, APIKeyPrefix) {
		return errors.New("memro: API keys start with memro_sk_; create one with `npx memro-cli connect` or in the dashboard")
	}
	pathAndQuery := r.path
	if qs := r.query.Encode(); qs != "" {
		pathAndQuery += "?" + qs
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, r.method, c.baseURL+pathAndQuery, bytes.NewReader(r.body))
		if err != nil {
			return fmt.Errorf("memro: build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if r.ctype != "" {
			req.Header.Set("Content-Type", r.ctype)
		}
		signed := false
		switch {
		case r.noAuth:
		case c.apiKey != "":
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		case r.signer != nil:
			h, err := SignRequest(r.signer.AgentID, r.signer.PrivateKey, r.method, pathAndQuery, r.body)
			if err != nil {
				return err
			}
			for k, v := range h {
				req.Header.Set(k, v)
			}
			signed = true
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("memro: %s %s: %w", r.method, r.path, err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("memro: read response: %w", err)
		}
		if resp.StatusCode < 300 {
			if out != nil && len(data) > 0 {
				return json.Unmarshal(data, out)
			}
			return nil
		}
		if attempt < c.maxRetries {
			var wait time.Duration
			retry := true
			switch {
			case resp.StatusCode == http.StatusUnauthorized && signed && bytes.Contains(data, []byte("already used")):
				// Signatures are deterministic: re-sign an identical request in the next second.
				wait = time.Second - time.Duration(time.Now().Nanosecond()) + 50*time.Millisecond
			case resp.StatusCode == http.StatusTooManyRequests,
				(resp.StatusCode >= 502 && resp.StatusCode <= 504) && (r.method == http.MethodGet || r.method == http.MethodDelete):
				wait = time.Duration(500<<attempt) * time.Millisecond
				if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
					wait = min(time.Duration(s)*time.Second, 30*time.Second)
				}
			default:
				retry = false
			}
			if retry {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
				continue
			}
		}
		msg := strings.TrimSpace(string(data))
		var j struct{ Message, Error string }
		if json.Unmarshal(data, &j) == nil && (j.Message != "" || j.Error != "") {
			msg = j.Message
			if msg == "" {
				msg = j.Error
			}
		}
		return &MemroError{StatusCode: resp.StatusCode, Message: msg}
	}
}

func (c *Client) jsonReq(method, path string, body any, signer *Signer) (request, error) {
	r := request{method: method, path: path, signer: signer}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return r, err
		}
		r.body, r.ctype = b, "application/json"
	}
	return r, nil
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body any, signer *Signer, out any) error {
	r, err := c.jsonReq(method, path, body, signer)
	if err != nil {
		return err
	}
	r.query = query
	return c.do(ctx, r, out)
}

// ── Account ───────────────────────────────────────────────────────────────

// WhoAmI returns the agent a credential acts as.
func (c *Client) WhoAmI(ctx context.Context, signer *Signer) (*WhoAmI, error) {
	var out WhoAmI
	return &out, c.call(ctx, http.MethodGet, "/auth/whoami", nil, nil, signer, &out)
}

// Health reports server, database and embedding status.
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	var out HealthStatus
	return &out, c.do(ctx, request{method: http.MethodGet, path: "/health", noAuth: true}, &out)
}

// ── Identity ──────────────────────────────────────────────────────────────

// CreateIdentity generates a key pair locally and registers only the public key.
func (c *Client) CreateIdentity(ctx context.Context) (*AgentIdentity, error) {
	kp, err := GenerateKeypair()
	if err != nil {
		return nil, err
	}
	body := map[string]any{"public_key": kp.PublicKey}
	var pow struct {
		Difficulty int `json:"difficulty"`
	}
	switch err := c.do(ctx, request{method: http.MethodGet, path: "/identity/pow", noAuth: true}, &pow); {
	case err == nil:
		body["pow"] = SolvePow(kp.PublicKey, pow.Difficulty)
	case !IsNotFound(err): // servers older than ADR 0013 have no /identity/pow
		return nil, err
	}
	r, _ := c.jsonReq(http.MethodPost, "/identity", body, nil)
	r.noAuth = true
	if err := c.do(ctx, r, nil); err != nil {
		return nil, err
	}
	return &AgentIdentity{AgentID: kp.PublicKey, PrivateKey: kp.PrivateKey}, nil
}

// DeleteIdentity deletes the agent and all of its memories, files, keys and webhooks.
func (c *Client) DeleteIdentity(ctx context.Context, agentID string, signer *Signer) error {
	return c.call(ctx, http.MethodDelete, "/identity/"+agentID, nil, nil, signer, nil)
}

// ── Memories ──────────────────────────────────────────────────────────────

// CreateMemory stores a memory (POST /memory).
func (c *Client) CreateMemory(ctx context.Context, body map[string]any, signer *Signer) (*Memory, error) {
	var out Memory
	return &out, c.call(ctx, http.MethodPost, "/memory", nil, body, signer, &out)
}

// ListMemories returns an agent's recent memories.
func (c *Client) ListMemories(ctx context.Context, agentID string, query url.Values, signer *Signer) ([]Memory, error) {
	var out []Memory
	return out, c.call(ctx, http.MethodGet, "/memory/agent/"+agentID, query, nil, signer, &out)
}

// DeleteMemory deletes a memory; it returns false if the memory does not exist.
func (c *Client) DeleteMemory(ctx context.Context, memoryID string, signer *Signer) (bool, error) {
	err := c.call(ctx, http.MethodDelete, "/memory/"+memoryID, nil, nil, signer, nil)
	if IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// Export returns every memory of an agent.
func (c *Client) Export(ctx context.Context, agentID string, signer *Signer) (*ExportData, error) {
	var out ExportData
	return &out, c.call(ctx, http.MethodGet, "/export/"+agentID, nil, nil, signer, &out)
}

// ── Search ────────────────────────────────────────────────────────────────

// Search runs GET /search.
func (c *Client) Search(ctx context.Context, query url.Values, signer *Signer) ([]SearchResult, error) {
	var out struct {
		Results []SearchResult `json:"results"`
	}
	return out.Results, c.call(ctx, http.MethodGet, "/search", query, nil, signer, &out)
}

// SearchTemporal runs GET /search/temporal.
func (c *Client) SearchTemporal(ctx context.Context, query url.Values, signer *Signer) ([]TemporalResult, error) {
	var out struct {
		Results []TemporalResult `json:"results"`
	}
	return out.Results, c.call(ctx, http.MethodGet, "/search/temporal", query, nil, signer, &out)
}

// ── Memory engine ─────────────────────────────────────────────────────────

func (c *Client) Reason(ctx context.Context, query string, signer *Signer) (*ReasoningResult, error) {
	var out ReasoningResult
	return &out, c.call(ctx, http.MethodPost, "/mee/reason", nil, map[string]string{"query": query}, signer, &out)
}

func (c *Client) ResolveConflict(ctx context.Context, a, b string, signer *Signer) (*ResolutionResult, error) {
	var out ResolutionResult
	return &out, c.call(ctx, http.MethodPost, "/mee/resolve", nil, map[string]string{"memory_a_id": a, "memory_b_id": b}, signer, &out)
}

func (c *Client) Evolve(ctx context.Context, signer *Signer) (*EvolutionResult, error) {
	var out EvolutionResult
	return &out, c.call(ctx, http.MethodPost, "/mee/evolve", nil, nil, signer, &out)
}

func (c *Client) Timeline(ctx context.Context, memoryID string, signer *Signer) (*MemoryTimeline, error) {
	var out MemoryTimeline
	return &out, c.call(ctx, http.MethodGet, "/mee/timeline/"+memoryID, nil, nil, signer, &out)
}

func (c *Client) MemoryChain(ctx context.Context, memoryID string, signer *Signer) (*MemoryChain, error) {
	var out MemoryChain
	return &out, c.call(ctx, http.MethodGet, "/memory/chain/"+memoryID, nil, nil, signer, &out)
}

func (c *Client) AnalyzeConflicts(ctx context.Context, signer *Signer) (*ConflictAnalysis, error) {
	var out ConflictAnalysis
	return &out, c.call(ctx, http.MethodGet, "/analyze/conflicts", nil, nil, signer, &out)
}

func (c *Client) Stats(ctx context.Context, signer *Signer) (*TemporalStats, error) {
	var out TemporalStats
	return &out, c.call(ctx, http.MethodGet, "/stats/temporal", nil, nil, signer, &out)
}

func (c *Client) Usage(ctx context.Context, signer *Signer) (*Usage, error) {
	var out Usage
	return &out, c.call(ctx, http.MethodGet, "/usage", nil, nil, signer, &out)
}

// ── Files ─────────────────────────────────────────────────────────────────

func multipartBody(field, name, contentType string, data []byte, extra map[string]string) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	for k, v := range extra {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func (c *Client) upload(ctx context.Context, path, field, name, contentType string, data []byte, extra map[string]string, signer *Signer, out any) error {
	body, ctype, err := multipartBody(field, name, contentType, data, extra)
	if err != nil {
		return err
	}
	return c.do(ctx, request{method: http.MethodPost, path: path, body: body, ctype: ctype, signer: signer}, out)
}

// UploadFile stores a file and extracts its text.
func (c *Client) UploadFile(ctx context.Context, agentID, name, contentType string, data []byte, signer *Signer) (*UploadedFile, error) {
	var out UploadedFile
	return &out, c.upload(ctx, "/upload", "file", name, contentType, data, map[string]string{"agent_id": agentID}, signer, &out)
}

// MemoryFromFile stores an uploaded file's text as a memory and returns its ID.
func (c *Client) MemoryFromFile(ctx context.Context, agentID, fileID string, memoryType MemoryType, signer *Signer) (string, error) {
	var out struct {
		MemoryID string `json:"memory_id"`
	}
	body := map[string]any{"agent_id": agentID, "file_id": fileID, "memory_type": memoryType}
	return out.MemoryID, c.call(ctx, http.MethodPost, "/memory/from-file", nil, body, signer, &out)
}

// UploadImage describes an image with vision and stores the description.
func (c *Client) UploadImage(ctx context.Context, name, contentType string, data []byte, signer *Signer) (*Memory, error) {
	var out Memory
	return &out, c.upload(ctx, "/upload/image", "image", name, contentType, data, nil, signer, &out)
}

// UploadDocument stores a PDF, Word, PowerPoint, Excel or text document as
// ~1,500-character overlapping memories linked to the stored file.
func (c *Client) UploadDocument(ctx context.Context, name string, data []byte, signer *Signer) ([]Memory, error) {
	var out []Memory
	return out, c.upload(ctx, "/upload/document", "document", name, "", data, nil, signer, &out)
}

// ── Webhooks ──────────────────────────────────────────────────────────────

func (c *Client) RegisterWebhook(ctx context.Context, eventType, hookURL string, signer *Signer) error {
	return c.call(ctx, http.MethodPost, "/hooks", nil, map[string]string{"event_type": eventType, "url": hookURL}, signer, nil)
}

// DeleteWebhook removes a webhook; it returns false if it was not registered.
func (c *Client) DeleteWebhook(ctx context.Context, eventType, hookURL string, signer *Signer) (bool, error) {
	err := c.call(ctx, http.MethodDelete, "/hooks", nil, map[string]string{"event_type": eventType, "url": hookURL}, signer, nil)
	if IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (c *Client) ListWebhooks(ctx context.Context, eventType string, signer *Signer) ([]string, error) {
	var out []string
	return out, c.call(ctx, http.MethodGet, "/hooks/"+eventType, nil, nil, signer, &out)
}

// CreateAPIKey mints an API key for a self-owned agent (signed requests only;
// API keys cannot create keys).
func (c *Client) CreateAPIKey(ctx context.Context, name string, signer *Signer) (string, error) {
	var out struct {
		Key string `json:"key"`
	}
	return out.Key, c.call(ctx, http.MethodPost, "/api-keys", nil, map[string]string{"name": name}, signer, &out)
}

// ── Coordination ──────────────────────────────────────────────────────────

func (c *Client) CoordinationClaim(ctx context.Context, body map[string]any, signer *Signer) (map[string]any, error) {
	var out map[string]any
	return out, c.call(ctx, http.MethodPost, "/coordination/claim", nil, body, signer, &out)
}

func (c *Client) CoordinationHeartbeat(ctx context.Context, body map[string]any, signer *Signer) (map[string]any, error) {
	var out map[string]any
	return out, c.call(ctx, http.MethodPost, "/coordination/heartbeat", nil, body, signer, &out)
}

func (c *Client) CoordinationRelease(ctx context.Context, body map[string]any, signer *Signer) error {
	return c.call(ctx, http.MethodPost, "/coordination/release", nil, body, signer, nil)
}

func (c *Client) CoordinationHandoff(ctx context.Context, body map[string]any, signer *Signer) error {
	return c.call(ctx, http.MethodPost, "/coordination/handoff", nil, body, signer, nil)
}

func (c *Client) CoordinationCheck(ctx context.Context, body map[string]any, signer *Signer) (map[string]any, error) {
	var out map[string]any
	return out, c.call(ctx, http.MethodPost, "/coordination/check", nil, body, signer, &out)
}

func (c *Client) CoordinationActive(ctx context.Context, q url.Values, signer *Signer) (map[string]any, error) {
	var out map[string]any
	return out, c.call(ctx, http.MethodGet, "/coordination/active", q, nil, signer, &out)
}

func (c *Client) CoordinationIntent(ctx context.Context, body map[string]any, signer *Signer) (map[string]any, error) {
	var out map[string]any
	return out, c.call(ctx, http.MethodPost, "/coordination/intent", nil, body, signer, &out)
}
