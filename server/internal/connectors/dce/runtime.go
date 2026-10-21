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
	ErrSSRFBlocked  = errors.New("dce: request to private or loopback IP address blocked (SSRF protection)")
	ErrBodyTooLarge = errors.New("dce: response body exceeded maximum allowed size")
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
	ItemsSynced    int           `json:"itemsSynced"`
	PagesProcessed int           `json:"pagesProcessed"`
	NewCheckpoint  string        `json:"newCheckpoint"`
	HasMore        bool          `json:"hasMore"`
	Duration       time.Duration `json:"duration"`
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
	startTime := time.Now()
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

	baseURL := m.Spec.BaseURL
	if strings.Contains(baseURL, "{") && strings.Contains(secret, ":") {
		prefix, actualSecret, _ := strings.Cut(secret, ":")
		baseURL = strings.ReplaceAll(baseURL, "{subdomain}", prefix)
		baseURL = strings.ReplaceAll(baseURL, "{domain}", prefix)
		baseURL = strings.ReplaceAll(baseURL, "{account_id}", prefix)
		baseURL = strings.ReplaceAll(baseURL, "{cloudId}", prefix)
		secret = actualSecret
	}

	result := &SyncResult{
		NewCheckpoint: checkpoint,
	}
	cursor := checkpoint

	// Primary sync operation (first operation in spec)
	op := m.Spec.Operations[0]

	for page := 0; page < r.opts.MaxPages; page++ {
		// Respect proactive rate limiting
		if page > 0 && m.Spec.Policies.RateLimitPerMin > 0 {
			paceInterval := time.Minute / time.Duration(m.Spec.Policies.RateLimitPerMin)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(paceInterval):
			}
		}

		reqURL, err := r.buildURL(baseURL, op, cursor, page)
		if err != nil {
			return nil, err
		}

		// Check full URL in case operation path points elsewhere
		if err := ValidateDestinationURL(reqURL, r.opts.AllowPrivateIPs); err != nil {
			return nil, err
		}

		var bodyReader io.Reader
		if op.Body != "" {
			bodyReader = strings.NewReader(op.Body)
		}
		req, err := http.NewRequestWithContext(ctx, op.Method, reqURL, bodyReader)
		if err != nil {
			return nil, err
		}
		if op.Body != "" {
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(op.Body)), nil
			}
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

		result.PagesProcessed++

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
			result.HasMore = false
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

			// Track latest monotonic checkpoint
			if m.Spec.Sync.Checkpoint != "" {
				cpVal := ExtractString(itemMap, m.Spec.Sync.Checkpoint)
				if cpVal != "" {
					result.NewCheckpoint = updateCheckpoint(result.NewCheckpoint, cpVal)
				}
			}
		}

		// Determine pagination continuation
		hasMorePages := false
		switch op.Pagination.Type {
		case "cursor":
			if op.Pagination.CursorPath != "" {
				nextCursor := ExtractString(respData, op.Pagination.CursorPath)
				if nextCursor != "" && nextCursor != cursor {
					cursor = nextCursor
					hasMorePages = true
				}
			}
		case "page":
			limit := op.Pagination.Limit
			if limit <= 0 || len(items) >= limit {
				hasMorePages = true
			}
		case "offset":
			limit := op.Pagination.Limit
			if limit <= 0 {
				limit = 50
			}
			if len(items) >= limit {
				hasMorePages = true
			}
		default: // "none"
			hasMorePages = false
		}

		if !hasMorePages {
			result.HasMore = false
			break
		}
		result.HasMore = true
	}

	result.Duration = time.Since(startTime)
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
		if strings.Contains(secret, ":") {
			// e.g. Trello apiKey:token
			key, token, _ := strings.Cut(secret, ":")
			req.Header.Set("Authorization", fmt.Sprintf("OAuth oauth_consumer_key=\"%s\", oauth_token=\"%s\"", key, token))
			return
		}
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
		if hdr == "DD-API-KEY" && strings.Contains(secret, ":") {
			apiKey, appKey, _ := strings.Cut(secret, ":")
			req.Header.Set("DD-API-KEY", apiKey)
			req.Header.Set("DD-APPLICATION-KEY", appKey)
			return
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
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}

		// Fast fail on permanent client errors (400, 401, 403, 404, 422) - do not retry
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound ||
			resp.StatusCode == http.StatusUnprocessableEntity {
			return resp, nil
		}

		// 429 Too Many Requests -> respect Retry-After header
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("rate limit exceeded (HTTP 429) after %d retries", maxRetries)
			}
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryAfter):
			}
			continue
		}

		// Transient server errors (500, 502, 503, 504) -> retry with backoff
		if resp.StatusCode == http.StatusInternalServerError ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout {
			if attempt < maxRetries {
				resp.Body.Close()
				backoff := time.Duration(attempt+1) * 150 * time.Millisecond
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
				continue
			}
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("request failed after retries")
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 500 * time.Millisecond
	}
	if sec, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && sec >= 0 {
		if sec > 60 {
			sec = 60
		}
		return time.Duration(sec) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d > 60*time.Second {
			return 60 * time.Second
		}
		if d > 0 {
			return d
		}
	}
	return 500 * time.Millisecond
}

func updateCheckpoint(current, candidate string) string {
	if current == "" {
		return candidate
	}
	if candidate == "" {
		return current
	}
	t1 := parseTimestamp(current)
	t2 := parseTimestamp(candidate)
	if !t1.IsZero() && !t2.IsZero() {
		if t2.After(t1) {
			return candidate
		}
		return current
	}
	if n1, err1 := strconv.ParseInt(current, 10, 64); err1 == nil {
		if n2, err2 := strconv.ParseInt(candidate, 10, 64); err2 == nil {
			if n2 > n1 {
				return candidate
			}
			return current
		}
	}
	if candidate > current {
		return candidate
	}
	return current
}

func parseTimestamp(val any) time.Time {
	if val == nil {
		return time.Time{}
	}
	switch v := val.(type) {
	case time.Time:
		return v.UTC()
	case float64:
		if v > 1e11 {
			return time.UnixMilli(int64(v)).UTC()
		}
		sec := int64(v)
		nsec := int64((v - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC()
	case int64:
		if v > 1e11 {
			return time.UnixMilli(v).UTC()
		}
		return time.Unix(v, 0).UTC()
	case int:
		iv := int64(v)
		if iv > 1e11 {
			return time.UnixMilli(iv).UTC()
		}
		return time.Unix(iv, 0).UTC()
	case string:
		str := strings.TrimSpace(v)
		if str == "" {
			return time.Time{}
		}
		if num, err := strconv.ParseFloat(str, 64); err == nil {
			return parseTimestamp(num)
		}
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
			time.RFC1123,
			time.RFC1123Z,
		}
		for _, f := range formats {
			if t, err := time.Parse(f, str); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

func (r *Runtime) normalize(mapping MappingSpec, raw map[string]any) *NormalizedItem {
	var evDate time.Time
	if mapping.EventDate != "" {
		if val, ok := ExtractValue(raw, mapping.EventDate); ok {
			evDate = parseTimestamp(val)
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
