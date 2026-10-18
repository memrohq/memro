// Package embed talks to the embedding / vision / transcription providers.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrDisabled = errors.New("embedding provider not configured")

type Provider struct {
	name     string
	model    string
	apiKey   string
	endpoint string
	base     string
	http     *http.Client

	mu       sync.Mutex
	cache    map[string]cacheEntry
	maxCache int
	Hits     atomic.Uint64
	Misses   atomic.Uint64
}

type cacheEntry struct {
	vec     []float32
	expires time.Time
}

const queryCacheTTL = time.Hour

// New builds a provider. baseURL overrides the API host (e.g. a proxy or an
// OpenAI-compatible server); leave it empty for the provider default.
func New(provider, model, openAIKey, voyageKey, baseURL string, maxCache int) *Provider {
	p := &Provider{
		http:     &http.Client{Timeout: 30 * time.Second},
		cache:    map[string]cacheEntry{},
		maxCache: maxCache,
	}
	switch provider {
	case "voyageai", "voyage", "anthropic":
		p.name, p.apiKey = "voyageai", voyageKey
		p.model = defaultStr(model, "voyage-3")
	default:
		p.name, p.apiKey = "openai", openAIKey
		p.model = defaultStr(model, "text-embedding-3-small")
	}
	p.base = "https://api.openai.com"
	if p.name == "voyageai" {
		p.base = "https://api.voyageai.com"
	}
	if baseURL != "" {
		p.base = strings.TrimRight(baseURL, "/")
	}
	p.endpoint = p.base + "/v1/embeddings"
	return p
}

func (p *Provider) Name() string  { return p.name }
func (p *Provider) Model() string { return p.model }

// Enabled reports whether an API key is configured.
func (p *Provider) Enabled() bool { return p.apiKey != "" }

func (p *Provider) Dimension() int {
	switch p.model {
	case "text-embedding-3-large":
		return 3072
	case "voyage-3-lite":
		return 512
	}
	if p.name == "voyageai" {
		return 1024
	}
	return 1536
}

// EmbedQuery embeds a single query string, using a TTL cache.
func (p *Provider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	now := time.Now()
	p.mu.Lock()
	if e, ok := p.cache[text]; ok && now.Before(e.expires) {
		p.mu.Unlock()
		p.Hits.Add(1)
		return e.vec, nil
	}
	p.mu.Unlock()
	p.Misses.Add(1)

	vecs, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if len(p.cache) >= p.maxCache {
		// Drop expired entries first, then arbitrary ones until there is room.
		for k, e := range p.cache {
			if now.After(e.expires) {
				delete(p.cache, k)
			}
		}
		for k := range p.cache {
			if len(p.cache) < p.maxCache {
				break
			}
			delete(p.cache, k)
		}
	}
	p.cache[text] = cacheEntry{vec: vecs[0], expires: now.Add(queryCacheTTL)}
	p.mu.Unlock()
	return vecs[0], nil
}

type CacheStats struct {
	Size, MaxSize int
	Hits, Misses  uint64
	TTLSeconds    int
}

func (p *Provider) CacheStats() CacheStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return CacheStats{Size: len(p.cache), MaxSize: p.maxCache, Hits: p.Hits.Load(), Misses: p.Misses.Load(), TTLSeconds: int(queryCacheTTL.Seconds())}
}

// EmbedBatch embeds several texts in one call (no cache).
func (p *Provider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if !p.Enabled() {
		return nil, ErrDisabled
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := p.postJSON(ctx, p.endpoint, map[string]any{"model": p.model, "input": texts}, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("%s returned %d embeddings for %d inputs", p.name, len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for i, d := range out.Data {
		idx := d.Index
		if idx < 0 || idx >= len(vecs) {
			idx = i
		}
		vecs[idx] = d.Embedding
	}
	return vecs, nil
}

// DescribeImage asks GPT-4o for a short description (OpenAI only).
func (p *Provider) DescribeImage(ctx context.Context, base64Image, mime string) (string, error) {
	if p.name != "openai" {
		return "", errors.New("vision analysis requires EMBEDDING_PROVIDER=openai")
	}
	if !p.Enabled() {
		return "", ErrDisabled
	}
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "Describe this image in 1-2 concise sentences. Focus on key objects and context for a semantic memory system."},
				map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + mime + ";base64," + base64Image}},
			},
		}},
		"max_tokens": 300,
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := p.postJSON(ctx, p.base+"/v1/chat/completions", body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("no vision description returned")
	}
	return out.Choices[0].Message.Content, nil
}

// Transcribe sends audio to Whisper (OpenAI only).
func (p *Provider) Transcribe(ctx context.Context, fileName string, audio []byte) (string, error) {
	if p.name != "openai" {
		return "", errors.New("audio transcription requires EMBEDDING_PROVIDER=openai")
	}
	if !p.Enabled() {
		return "", ErrDisabled
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "whisper-1")
	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/v1/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	var out struct {
		Text string `json:"text"`
	}
	if err := p.do(req, &out); err != nil {
		return "", err
	}
	return out.Text, nil
}

func (p *Provider) postJSON(ctx context.Context, url string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return p.do(req, out)
}

func (p *Provider) do(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s API error %d: %s", p.name, resp.StatusCode, msg)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func defaultStr(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
