package dce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSSRFBlocked   = errors.New("dce: request to private or loopback IP address blocked (SSRF protection)")
	ErrBodyTooLarge  = errors.New("dce: response body exceeded maximum allowed size")
)

// SecretResolver resolves a secret reference into actual credentials.
type SecretResolver interface {
	ResolveSecret(ctx context.Context, secretRef string) (string, error)
}

// MapSecretResolver is a simple map-backed secret resolver for tests and standalone instances.
type MapSecretResolver map[string]string

func (m MapSecretResolver) ResolveSecret(ctx context.Context, secretRef string) (string, error) {
	s, ok := m[secretRef]
	if !ok {
		return "", fmt.Errorf("secret %q not found", secretRef)
	}
	return s, nil
}

// NormalizedItem is an external entity extracted and normalized by DCE into Memro format.
type NormalizedItem struct {
	ID         string
	Title      string
	Content    string
	EventDate  time.Time
	AuthorID   string
	AuthorName string
	URL        string
	Raw        map[string]any
}

// Sink receives normalized items from the DCE runtime.
type Sink interface {
	IngestItem(ctx context.Context, item *NormalizedItem) error
}

// SyncResult captures the outcome of a declarative sync execution.
type SyncResult struct {
	ItemsSynced   int
	NewCheckpoint string
	HasMore       bool
}

// Options configures security boundaries and resource limits for the runtime.
type Options struct {
	AllowPrivateIPs  bool  // Set to true in unit test fixtures using httptest.Server
	MaxResponseBytes int64 // Defaults to 10MB
	MaxPages         int   // Defaults to 50 pages per execution
}

// Runtime executes declarative connector manifests.
type Runtime struct {
	client  *http.Client
	secrets SecretResolver
	opts    Options
}

func NewRuntime(client *http.Client, secrets SecretResolver) *Runtime {
	return NewRuntimeWithOptions(client, secrets, Options{})
}

func NewRuntimeWithOptions(client *http.Client, secrets SecretResolver, opts Options) *Runtime {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.MaxResponseBytes <= 0 {
		opts.MaxResponseBytes = 10 << 20 // 10MB
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 50
	}
	return &Runtime{
		client:  client,
		secrets: secrets,
		opts:    opts,
	}
}

// ValidateDestinationURL checks against SSRF vulnerabilities.
func ValidateDestinationURL(rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q; only http and https allowed", scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return errors.New("missing hostname in target URL")
	}

	if allowPrivate {
		return nil
	}

	// Resolve IP addresses for the hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If hostname resolution fails, return resolution error
		return fmt.Errorf("resolve host %s: %w", hostname, err)
	}

	for _, ip := range ips {
		if isPrivateOrRestrictedIP(ip) {
			return fmt.Errorf("%w: host %s resolves to restricted IP %s", ErrSSRFBlocked, hostname, ip.String())
		}
	}

	return nil
}

func isPrivateOrRestrictedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	// Check for AWS/GCP/Azure link-local metadata (169.254.169.254)
	if ip.String() == "169.254.169.254" {
		return true
	}

	// IPv4 RFC 1918 private ranges
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		case ip4[0] == 127:
			return true
		case ip4[0] == 0:
			return true
		}
	}

	// IPv6 Unique Local Address (ULA) fc00::/7
	if len(ip) == net.IPv6len {
		if (ip[0] & 0xfe) == 0xfc {
			return true
		}
	}

	return false
}

// sanitizeError strips sensitive tokens from error strings before returning them to callers.
func sanitizeError(err error, secret string) error {
	if err == nil {
		return nil
	}
	if secret == "" {
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, secret) {
		msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
		return errors.New(msg)
	}
	return err
}

// Execute runs the declared sync operation on a connector manifest.
func (r *Runtime) Execute(ctx context.Context, m *Manifest, sink Sink, checkpoint string) (*SyncResult, error) {
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	// Validate target BaseURL against SSRF
	if err := ValidateDestinationURL(m.Spec.BaseURL, r.opts.AllowPrivateIPs); err != nil {
		return nil, err
	}

	timeout := m.Spec.Policies.TimeoutDuration(30 * time.Second)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Resolve secret if auth is configured
	secret := ""
	if m.Spec.Auth.Type != "none" {
		s, err := r.secrets.ResolveSecret(ctx, m.Spec.Auth.SecretRef)
		if err != nil {
			return nil, fmt.Errorf("resolve secret %s: %w", m.Spec.Auth.SecretRef, err)
		}
		secret = s
	}

	result := &SyncResult{}
	cursor := checkpoint

	// Primary sync operation (first operation in spec)
	op := m.Spec.Operations[0]

	for page := 0; page < r.opts.MaxPages; page++ {
		reqURL, err := r.buildURL(m.Spec.BaseURL, op, cursor, page)
		if err != nil {
			return nil, err
		}

		// Check full URL in case operation path points elsewhere
		if err := ValidateDestinationURL(reqURL, r.opts.AllowPrivateIPs); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, op.Method, reqURL, nil)
		if err != nil {
			return nil, err
		}

		r.applyAuth(req, m.Spec.Auth, secret)
		for k, v := range op.Headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := r.executeWithRetry(ctx, req, m.Spec.Policies.MaxRetries)
		if err != nil {
			return nil, sanitizeError(fmt.Errorf("http execute %s %s: %w", op.Method, reqURL, err), secret)
		}

		// Read response body with safe size ceiling
		lr := io.LimitReader(resp.Body, r.opts.MaxResponseBytes+1)
		bodyBytes, err := io.ReadAll(lr)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		if int64(len(bodyBytes)) > r.opts.MaxResponseBytes {
			return nil, ErrBodyTooLarge
		}

		if resp.StatusCode >= 400 {
			sanitizedBody := string(bodyBytes)
			if secret != "" {
				sanitizedBody = strings.ReplaceAll(sanitizedBody, secret, "[REDACTED]")
			}
			return nil, fmt.Errorf("api status %d: %s", resp.StatusCode, sanitizedBody)
		}

		var respData any
		if err := json.Unmarshal(bodyBytes, &respData); err != nil {
			return nil, fmt.Errorf("unmarshal api response: %w", err)
		}

		items, ok := ExtractSlice(respData, op.Response.ItemsPath)
		if !ok || len(items) == 0 {
			break
		}

		for _, rawItem := range items {
			itemMap, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			norm := r.normalize(m.Spec.Mapping, itemMap)
			if err := sink.IngestItem(ctx, norm); err != nil {
				return nil, sanitizeError(fmt.Errorf("sink ingest %s: %w", norm.ID, err), secret)
			}
			result.ItemsSynced++

			// Track latest checkpoint if configured
			if m.Spec.Sync.Checkpoint != "" {
				cpVal := ExtractString(itemMap, m.Spec.Sync.Checkpoint)
				if cpVal != "" {
					result.NewCheckpoint = cpVal
				}
			}
		}

		// Handle pagination next cursor
		if op.Pagination.Type == "cursor" && op.Pagination.CursorPath != "" {
			nextCursor := ExtractString(respData, op.Pagination.CursorPath)
			if nextCursor == "" || nextCursor == cursor {
				break
			}
			cursor = nextCursor
			result.HasMore = true
		} else {
			break
		}
	}

	return result, nil
}

func (r *Runtime) buildURL(baseURL string, op OperationSpec, cursor string, page int) (string, error) {
	full := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(op.Path, "/")
	u, err := url.Parse(full)
	if err != nil {
		return "", err
	}

	q := u.Query()
	for k, v := range op.QueryParams {
		q.Set(k, v)
	}

	switch op.Pagination.Type {
	case "cursor":
		if cursor != "" && op.Pagination.CursorParam != "" {
			q.Set(op.Pagination.CursorParam, cursor)
		}
	case "page":
		param := op.Pagination.PageParam
		if param == "" {
			param = "page"
		}
		q.Set(param, strconv.Itoa(page+1))
	case "offset":
		param := op.Pagination.OffsetParam
		if param == "" {
			param = "offset"
		}
		limit := op.Pagination.Limit
		if limit <= 0 {
			limit = 50
		}
		q.Set(param, strconv.Itoa(page*limit))
	}

	if op.Pagination.Limit > 0 && op.Pagination.LimitParam != "" {
		q.Set(op.Pagination.LimitParam, strconv.Itoa(op.Pagination.Limit))
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (r *Runtime) applyAuth(req *http.Request, auth AuthSpec, secret string) {
	switch auth.Type {
	case "bearer":
		prefix := auth.Prefix
		if prefix == "" {
			prefix = "Bearer "
		}
		req.Header.Set("Authorization", prefix+secret)
	case "apiKey":
		hdr := auth.Header
		if hdr == "" {
			hdr = "X-API-Key"
		}
		prefix := auth.Prefix
		req.Header.Set(hdr, prefix+secret)
	case "basic":
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	}
}

func (r *Runtime) executeWithRetry(ctx context.Context, req *http.Request, maxRetries int) (*http.Response, error) {
	if maxRetries <= 0 {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			retryAfter := 1 * time.Second
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if seconds, err := strconv.Atoi(ra); err == nil {
					retryAfter = time.Duration(seconds) * time.Second
				}
			}
			time.Sleep(retryAfter)
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("request failed after retries")
}

func (r *Runtime) normalize(mapping MappingSpec, raw map[string]any) *NormalizedItem {
	dateStr := ExtractString(raw, mapping.EventDate)
	var evDate time.Time
	if dateStr != "" {
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			evDate = t.UTC()
		} else if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			evDate = t.UTC()
		}
	}

	return &NormalizedItem{
		ID:         ExtractString(raw, mapping.ID),
		Title:      ExtractString(raw, mapping.Title),
		Content:    ExtractString(raw, mapping.Content),
		EventDate:  evDate,
		AuthorID:   ExtractString(raw, mapping.AuthorID),
		AuthorName: ExtractString(raw, mapping.AuthorName),
		URL:        ExtractString(raw, mapping.URL),
		Raw:        raw,
	}
}
