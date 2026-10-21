package dce

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
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
	MemoryType string // "semantic" or "episodic"
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

// NewRuntimeWithOptions builds a runtime. Without a client it makes one that
// refuses private and loopback addresses when it connects (so DNS changes and
// redirects cannot reach internal services) and checks every redirect.
func NewRuntimeWithOptions(client *http.Client, secrets SecretResolver, opts Options) *Runtime {
	if opts.MaxResponseBytes <= 0 {
		opts.MaxResponseBytes = 10 << 20 // 10MB
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 50
	}
	if client == nil {
		client = SafeClient(opts.AllowPrivateIPs)
	}
	return &Runtime{client: client, secrets: secrets, opts: opts}
}

// SafeClient is an HTTP client for third-party APIs: it refuses private,
// loopback and metadata addresses at connect time and allows at most five
// redirects, each re-checked.
func SafeClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || isPrivateOrRestrictedIP(ip) {
				return fmt.Errorf("%w: %s", ErrSSRFBlocked, host)
			}
			return nil
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	tr.Proxy = nil
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Host != via[0].URL.Host {
				return errors.New("redirect to another host refused")
			}
			return ValidateDestinationURL(req.URL.String(), allowPrivate)
		},
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
	if scheme != "https" {
		return fmt.Errorf("third-party APIs must use https: %s", hostname)
	}

	// Resolve IP addresses for the hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("resolve host %s: %w", hostname, err)
	}

	for _, ip := range ips {
		if isPrivateOrRestrictedIP(ip) {
			return fmt.Errorf("%w: host %s resolves to restricted IP %s", ErrSSRFBlocked, hostname, ip.String())
		}
	}

	return nil
}

// restrictedNets are ranges a third-party API never lives in: shared and
// benchmarking space, and IPv6 prefixes that embed an IPv4 address (NAT64,
// 6to4, Teredo) and could reach a private one.
var restrictedNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func isPrivateOrRestrictedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.IsPrivate() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 0 || ip4[0] == 127 {
			return true
		}
		ip = ip4
	}
	for _, n := range restrictedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// sanitizeError strips sensitive tokens from error strings before returning them to callers.
func sanitizeError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	changed := false
	for _, s := range secrets {
		if len(s) >= 4 && strings.Contains(msg, s) {
			msg = strings.ReplaceAll(msg, s, "[REDACTED]")
			changed = true
		}
	}
	if changed {
		return errors.New(msg)
	}
	return err
}

// --- Calls --------------------------------------------------------------------

// Call is everything one request needs: the manifest, the user's fields
// (token and account details) and the sync position.
type Call struct {
	Manifest *Manifest
	Config   map[string]string
	SourceID string
	Source   string // source display name
	Cursor   string // pagination position within the current pass
	// Checkpoint is the high-water mark from the last finished pass, sent as
	// {{checkpoint}} so a pass fetches only what changed since.
	Checkpoint string
}

func (c Call) vars() Vars {
	return Vars{Config: c.Config, Cursor: c.Cursor, Checkpoint: c.Checkpoint, SourceID: c.SourceID, SourceName: c.Source}
}

// secrets are the values to redact from errors: every secret field.
func (c Call) secrets() []string {
	var out []string
	for _, f := range c.Manifest.Spec.Fields {
		if f.Secret {
			out = append(out, c.Config[f.Name])
		}
	}
	if ref := c.Manifest.Spec.Auth.SecretRef; ref != "" {
		out = append(out, c.Config[ref])
	}
	return out
}

// CheckConfig validates the user's fields against the manifest: required
// fields are present and patterns match.
func CheckConfig(m *Manifest, cfg map[string]string) error {
	for _, f := range m.Spec.Fields {
		v := strings.TrimSpace(cfg[f.Name])
		if v == "" {
			if !f.Optional {
				return fmt.Errorf("%s is required", f.Label)
			}
			continue
		}
		if f.Pattern != "" && !regexp.MustCompile("^(?:"+f.Pattern+")$").MatchString(v) {
			return fmt.Errorf("%s does not look right", f.Label)
		}
	}
	return nil
}

// PageResult is one page of an operation.
type PageResult struct {
	Items      []*NormalizedItem
	NextCursor string // "" when the pass is finished
	// MaxCheckpoint is the largest checkpoint value seen on this page.
	MaxCheckpoint string
}

// Page fetches one page of an operation.
func (r *Runtime) Page(ctx context.Context, c Call, op OperationSpec) (*PageResult, error) {
	m := c.Manifest
	data, resp, reqURL, err := r.do(ctx, c, op.Method, op.Path, op.QueryParams, op.Headers, op.Body, &op.Pagination)
	if err != nil {
		return nil, err
	}
	res := &PageResult{}
	items, _ := ExtractSlice(data, op.Response.ItemsPath)
	mapping := m.Spec.Mapping
	if op.Mapping != nil {
		mapping = *op.Mapping
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		n := normalize(mapping, item)
		if n.ID == "" {
			continue
		}
		res.Items = append(res.Items, n)
		if cp := m.Spec.Sync.Checkpoint; cp != "" {
			res.MaxCheckpoint = laterCheckpoint(res.MaxCheckpoint, ExtractString(item, cp))
		}
	}
	if len(items) == 0 {
		return res, nil
	}
	switch op.Pagination.Type {
	case "cursor":
		if next := ExtractString(data, op.Pagination.CursorPath); next != c.Cursor {
			res.NextCursor = next
		}
	case "nextURL":
		res.NextCursor = sameHostURL(reqURL, ExtractString(data, op.Pagination.CursorPath))
	case "linkHeader":
		res.NextCursor = sameHostURL(reqURL, nextLink(resp.Header.Values("Link")))
	case "page", "offset":
		limit := op.Pagination.Limit
		if limit <= 0 || len(items) >= limit {
			n, _ := strconv.Atoi(c.Cursor)
			res.NextCursor = strconv.Itoa(n + 1)
		}
	}
	return res, nil
}

// Verify checks the user's credentials and names the account.
func (r *Runtime) Verify(ctx context.Context, c Call) (accountID, accountName string, err error) {
	v := c.Manifest.Spec.Verify
	if v == nil {
		op := c.Manifest.Spec.Operations[0]
		_, _, _, err = r.do(ctx, c, op.Method, op.Path, op.QueryParams, op.Headers, op.Body, nil)
		return "", "", err
	}
	data, _, _, err := r.do(ctx, c, v.Method, v.Path, v.QueryParams, v.Headers, v.Body, nil)
	if err != nil {
		return "", "", err
	}
	return renderItem(v.AccountID, data), renderItem(v.AccountName, data), nil
}

// Source is something the user can choose to sync.
type Source struct{ ID, Name string }

// ListSources returns the manifest's sources (at most MaxPages pages), or a
// single "all" source when it declares none.
func (r *Runtime) ListSources(ctx context.Context, c Call) ([]Source, error) {
	s := c.Manifest.Spec.Sources
	if s == nil {
		return []Source{{ID: "all", Name: "Everything this token can read"}}, nil
	}
	var out []Source
	seen := map[string]bool{}
	c.Cursor = ""
	for page := 0; page < r.opts.MaxPages; page++ {
		data, resp, reqURL, err := r.do(ctx, c, s.Operation.Method, s.Operation.Path, s.Operation.QueryParams,
			s.Operation.Headers, s.Operation.Body, &s.Operation.Pagination)
		if err != nil {
			return nil, err
		}
		items, _ := ExtractSlice(data, s.Operation.Response.ItemsPath)
		for _, raw := range items {
			id, name := renderItem(s.ID, raw), renderItem(s.Name, raw)
			if id != "" && !seen[id] && matchesWhere(raw, s.Where) {
				seen[id] = true
				out = append(out, Source{ID: id, Name: name})
			}
		}
		next := ""
		switch s.Operation.Pagination.Type {
		case "cursor":
			next = ExtractString(data, s.Operation.Pagination.CursorPath)
		case "nextURL":
			next = sameHostURL(reqURL, ExtractString(data, s.Operation.Pagination.CursorPath))
		case "linkHeader":
			next = sameHostURL(reqURL, nextLink(resp.Header.Values("Link")))
		case "page", "offset":
			if len(items) > 0 && s.Operation.Pagination.Limit > 0 && len(items) >= s.Operation.Pagination.Limit {
				n, _ := strconv.Atoi(c.Cursor)
				next = strconv.Itoa(n + 1)
			}
		}
		if next == "" || next == c.Cursor || len(items) == 0 {
			break
		}
		c.Cursor = next
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// matchesWhere reports whether item has, at each path, one of the values.
func matchesWhere(item any, where map[string][]string) bool {
	for path, allowed := range where {
		v := ExtractString(item, path)
		ok := false
		for _, a := range allowed {
			ok = ok || v == a
		}
		if !ok {
			return false
		}
	}
	return true
}

// do sends one templated request and decodes its JSON response. For page and
// offset pagination the cursor is the zero-based page number; for nextURL and
// linkHeader it is the full URL of the next page (already checked to be on
// the same host).
func (r *Runtime) do(ctx context.Context, c Call, method, path string, query, headers map[string]string, body string,
	pg *PaginationSpec) (any, *http.Response, string, error) {
	m := c.Manifest
	if err := CheckConfig(m, c.Config); err != nil && len(m.Spec.Fields) > 0 {
		return nil, nil, "", err
	}
	vars := c.vars()
	secrets := c.secrets()
	if pg != nil && (pg.Type == "page" || pg.Type == "offset") {
		n, _ := strconv.Atoi(c.Cursor)
		vars.Page = strconv.Itoa(n + pageBase(pg))
		vars.Offset = strconv.Itoa(n * pageLimit(pg))
	}

	var reqURL string
	if pg != nil && (pg.Type == "nextURL" || pg.Type == "linkHeader") && c.Cursor != "" {
		reqURL = c.Cursor
	} else {
		base := render(m.Spec.BaseURL, vars)
		full := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(render(path, vars), "/")
		u, err := url.Parse(full)
		if err != nil {
			return nil, nil, "", err
		}
		q := u.Query()
		for k, v := range query {
			if val, ok := renderAll(v, vars); ok && val != "" {
				q.Set(k, val)
			}
		}
		if pg != nil {
			switch pg.Type {
			case "cursor":
				if c.Cursor != "" && pg.CursorParam != "" {
					q.Set(pg.CursorParam, c.Cursor)
				}
			case "page":
				if pg.PageParam != "-" { // "-": the page number goes in the body
					param := pg.PageParam
					if param == "" {
						param = "page"
					}
					q.Set(param, vars.Page)
				}
			case "offset":
				if pg.OffsetParam != "-" {
					param := pg.OffsetParam
					if param == "" {
						param = "offset"
					}
					q.Set(param, vars.Offset)
				}
			}
			if pg.Limit > 0 && pg.LimitParam != "" {
				q.Set(pg.LimitParam, strconv.Itoa(pg.Limit))
			}
		}
		if m.Spec.Auth.Type == "query" {
			q.Set(m.Spec.Auth.Header, c.Config[m.Spec.Auth.SecretRef])
		}
		u.RawQuery = q.Encode()
		reqURL = u.String()
	}
	if err := ValidateDestinationURL(reqURL, r.opts.AllowPrivateIPs); err != nil {
		return nil, nil, "", sanitizeError(err, secrets...)
	}

	method = strings.ToUpper(method)
	if method == "" {
		method = http.MethodGet
	}
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(renderJSON(body, vars))
	}
	ctx, cancel := context.WithTimeout(ctx, m.Spec.Policies.TimeoutDuration(30*time.Second))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, nil, "", sanitizeError(err, secrets...)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "memro-connectors/1")
	if err := r.applyAuth(ctx, req, c); err != nil {
		return nil, nil, "", err
	}
	for k, v := range m.Spec.Headers {
		if val, ok := renderAll(v, vars); ok && val != "" {
			req.Header.Set(k, val)
		}
	}
	for k, v := range headers { // may override Accept (vendor media types)
		if val, ok := renderAll(v, vars); ok && val != "" {
			req.Header.Set(k, val)
		}
	}

	resp, err := r.executeWithRetry(ctx, req, m.Spec.Policies.MaxRetries)
	if err != nil {
		return nil, nil, "", sanitizeError(fmt.Errorf("%s %s: %w", method, redactURL(reqURL), err), secrets...)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, r.opts.MaxResponseBytes+1))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read response body: %w", err)
	}
	if int64(len(raw)) > r.opts.MaxResponseBytes {
		return nil, nil, "", ErrBodyTooLarge
	}
	if resp.StatusCode >= 400 {
		return nil, resp, reqURL, sanitizeError(&HTTPError{Status: resp.StatusCode, Body: truncate(string(raw), 300)}, secrets...)
	}
	var data any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, nil, "", fmt.Errorf("unmarshal api response: %w", err)
		}
	}
	return data, resp, reqURL, nil
}

func pageBase(pg *PaginationSpec) int {
	if pg.ZeroBased {
		return 0
	}
	return 1
}

func pageLimit(pg *PaginationSpec) int {
	if pg.Limit > 0 {
		return pg.Limit
	}
	return 50
}

// HTTPError is an error status from the API.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("api status %d: %s", e.Status, e.Body) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// redactURL drops the query, which may carry a key (auth type "query").
func redactURL(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}

func (r *Runtime) applyAuth(ctx context.Context, req *http.Request, c Call) error {
	a := c.Manifest.Spec.Auth
	secret := ""
	if a.SecretRef != "" {
		if v, ok := c.Config[a.SecretRef]; ok {
			secret = v
		} else if r.secrets != nil {
			s, err := r.secrets.ResolveSecret(ctx, a.SecretRef)
			if err != nil {
				return fmt.Errorf("resolve secret %s: %w", a.SecretRef, err)
			}
			secret = s
		}
	}
	switch a.Type {
	case "bearer":
		prefix := a.Prefix
		if prefix == "" {
			prefix = "Bearer "
		}
		req.Header.Set("Authorization", prefix+secret)
	case "apiKey":
		hdr := a.Header
		if hdr == "" {
			hdr = "X-API-Key"
		}
		req.Header.Set(hdr, a.Prefix+secret)
	case "basic":
		cred := secret
		if a.Username != "" || a.Password != "" {
			cred = render(a.Username, c.vars()) + ":" + render(a.Password, c.vars())
		}
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cred)))
	}
	return nil
}

func (r *Runtime) executeWithRetry(ctx context.Context, req *http.Request, maxRetries int) (*http.Response, error) {
	if maxRetries <= 0 {
		maxRetries = 2
	}
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			if errors.Is(err, ErrSSRFBlocked) || ctx.Err() != nil {
				return nil, err
			}
			sleep(ctx, time.Duration(attempt+1)*500*time.Millisecond)
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			wait := time.Duration(attempt+1) * time.Second
			if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && ra > 0 {
				wait = min(time.Duration(ra)*time.Second, 30*time.Second)
			}
			if attempt < maxRetries {
				resp.Body.Close()
				sleep(ctx, wait)
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

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// --- Pagination helpers -----------------------------------------------------------

var linkNext = regexp.MustCompile(`<([^>]+)>\s*;[^,]*rel="?next"?`)

// nextLink is the rel="next" URL of RFC 8288 Link headers.
func nextLink(values []string) string {
	for _, v := range values {
		if m := linkNext.FindStringSubmatch(v); m != nil {
			return m[1]
		}
	}
	return ""
}

// sameHostURL resolves next against the current URL and returns it only if
// it stays on the same scheme and host, so credentials never go elsewhere.
func sameHostURL(current, next string) string {
	if next == "" {
		return ""
	}
	cur, err := url.Parse(current)
	if err != nil {
		return ""
	}
	n, err := cur.Parse(next)
	if err != nil || n.Scheme != cur.Scheme || n.Host != cur.Host {
		return ""
	}
	return n.String()
}

// --- Mapping ----------------------------------------------------------------------

func normalize(mapping MappingSpec, raw map[string]any) *NormalizedItem {
	memType := mapping.MemoryType
	if memType == "" {
		memType = "semantic"
	}
	return &NormalizedItem{
		ID:         renderItem(mapping.ID, raw),
		Title:      renderItem(mapping.Title, raw),
		Content:    renderItem(mapping.Content, raw),
		EventDate:  ParseDate(renderItem(mapping.EventDate, raw)),
		AuthorID:   renderItem(mapping.AuthorID, raw),
		AuthorName: renderItem(mapping.AuthorName, raw),
		URL:        renderItem(mapping.URL, raw),
		MemoryType: memType,
		Raw:        raw,
	}
}

var dateLayouts = []string{
	time.RFC3339Nano, time.RFC3339,
	"2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700", // Jira, Confluence
	"2006-01-02T15:04:05.000Z", "2006-01-02T15:04:05", "2006-01-02 15:04:05",
	time.RFC1123Z, time.RFC1123, "2006/01/02 15:04:05 -0700", "2006-01-02",
}

// ParseDate reads the date formats APIs use, including Unix seconds and
// milliseconds. Unknown formats give the zero time.
func ParseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 0 {
		if n > 1e12 {
			return time.UnixMilli(int64(n)).UTC()
		}
		return time.Unix(int64(n), 0).UTC()
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// laterCheckpoint returns the later of two checkpoint values: compared as
// dates when both parse, otherwise as numbers or strings.
func laterCheckpoint(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	// Plain numbers (versions, IDs, epoch times) compare as numbers.
	if na, err := strconv.ParseFloat(a, 64); err == nil {
		if nb, err := strconv.ParseFloat(b, 64); err == nil {
			if nb > na {
				return b
			}
			return a
		}
	}
	ta, tb := ParseDate(a), ParseDate(b)
	if !ta.IsZero() && !tb.IsZero() {
		if tb.After(ta) {
			return b
		}
		return a
	}
	if b > a {
		return b
	}
	return a
}

// --- Legacy entry point -----------------------------------------------------------

// Execute runs the manifest's first operation to the end (up to MaxPages),
// sending items to sink. checkpoint is the high-water mark of the previous
// run, available to the operation as {{checkpoint}}.
func (r *Runtime) Execute(ctx context.Context, m *Manifest, sink Sink, checkpoint string) (*SyncResult, error) {
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}
	cfg := map[string]string{}
	if ref := m.Spec.Auth.SecretRef; ref != "" && r.secrets != nil && m.Spec.Auth.Type != "none" {
		s, err := r.secrets.ResolveSecret(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve secret %s: %w", ref, err)
		}
		cfg[ref] = s
	}
	call := Call{Manifest: m, Config: cfg, Checkpoint: checkpoint}
	op := m.Spec.Operations[0]
	res := &SyncResult{}
	high := checkpoint
	for page := 0; page < r.opts.MaxPages; page++ {
		p, err := r.Page(ctx, call, op)
		if err != nil {
			return nil, err
		}
		for _, it := range p.Items {
			if err := sink.IngestItem(ctx, it); err != nil {
				return nil, sanitizeError(fmt.Errorf("sink ingest %s: %w", it.ID, err), call.secrets()...)
			}
			res.ItemsSynced++
		}
		high = laterCheckpoint(high, p.MaxCheckpoint)
		if p.NextCursor == "" {
			break
		}
		call.Cursor = p.NextCursor
		res.HasMore = page+1 < r.opts.MaxPages
	}
	if high != checkpoint {
		res.NewCheckpoint = high
	}
	return res, nil
}
