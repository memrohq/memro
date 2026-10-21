package dce

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest defines the declarative contract for an external connector (ADR 0014).
type Manifest struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       Spec     `yaml:"spec" json:"spec"`
}

type Metadata struct {
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Shown in the dashboard's catalog.
	DisplayName string `yaml:"displayName,omitempty" json:"displayName,omitempty"`
	Category    string `yaml:"category,omitempty" json:"category,omitempty"`
	DocsURL     string `yaml:"docsURL,omitempty" json:"docsURL,omitempty"`
}

type Spec struct {
	Type string `yaml:"type" json:"type"`
	// BaseURL, paths, query parameters, headers and bodies may use
	// {{config.<field>}}; operations also {{cursor}}, {{checkpoint}} and
	// {{source.id}} (see template.go).
	BaseURL string      `yaml:"baseURL" json:"baseURL"`
	Fields  []FieldSpec `yaml:"fields,omitempty" json:"fields,omitempty"`
	// Headers are sent with every request (verify, sources, operations),
	// e.g. an API version or a second key ("DD-APPLICATION-KEY").
	Headers    map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Auth       AuthSpec          `yaml:"auth" json:"auth"`
	Verify     *VerifySpec       `yaml:"verify,omitempty" json:"verify,omitempty"`
	Sources    *SourcesSpec      `yaml:"sources,omitempty" json:"sources,omitempty"`
	Operations []OperationSpec   `yaml:"operations" json:"operations"`
	Mapping    MappingSpec       `yaml:"mapping" json:"mapping"`
	Sync       SyncSpec          `yaml:"sync" json:"sync"`
	Policies   PolicySpec        `yaml:"policies,omitempty" json:"policies,omitempty"`
}

// FieldSpec is something the user enters when connecting: the API token and
// account details such as a site or subdomain. Secret fields are sealed and
// never shown again.
type FieldSpec struct {
	Name        string `yaml:"name" json:"name"`
	Label       string `yaml:"label" json:"label"`
	Help        string `yaml:"help,omitempty" json:"help,omitempty"`
	Placeholder string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	Secret      bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
	Optional    bool   `yaml:"optional,omitempty" json:"optional,omitempty"`
	// Pattern restricts the value (a Go regexp, anchored). Fields used in a
	// URL host must have one, so a value cannot point requests elsewhere.
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
}

// VerifySpec is a cheap request that proves the credentials work when the
// user connects, and names the account.
type VerifySpec struct {
	Method      string            `yaml:"method,omitempty" json:"method,omitempty"`
	Path        string            `yaml:"path" json:"path"`
	QueryParams map[string]string `yaml:"queryParams,omitempty" json:"queryParams,omitempty"`
	Body        string            `yaml:"body,omitempty" json:"body,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	AccountID   string            `yaml:"accountId,omitempty" json:"accountId,omitempty"`
	AccountName string            `yaml:"accountName,omitempty" json:"accountName,omitempty"`
}

// SourcesSpec lists what the user can choose to sync (projects, boards,
// repositories). Without it a connection has a single source, "all".
type SourcesSpec struct {
	Operation OperationSpec `yaml:"operation" json:"operation"`
	ID        string        `yaml:"id" json:"id"`
	Name      string        `yaml:"name" json:"name"`
	// Where keeps only items whose value at each path is one of the listed
	// values (e.g. Discord text channels: {type: ["0", "5"]}).
	Where map[string][]string `yaml:"where,omitempty" json:"where,omitempty"`
}

type AuthSpec struct {
	Type      string `yaml:"type" json:"type"` // "bearer", "apiKey", "basic", "query", "none"
	SecretRef string `yaml:"secretRef" json:"secretRef"`
	Header    string `yaml:"header,omitempty" json:"header,omitempty"` // For apiKey: header name, e.g. "X-API-Key"; for query: the parameter name
	Prefix    string `yaml:"prefix,omitempty" json:"prefix,omitempty"` // Header prefix, e.g. "Bearer "
	// Basic auth from templates (e.g. "{{config.email}}" and the token);
	// without them the secret is used as "user:password".
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

type OperationSpec struct {
	Name        string            `yaml:"name" json:"name"`
	Method      string            `yaml:"method" json:"method"` // GET, POST
	Path        string            `yaml:"path" json:"path"`
	QueryParams map[string]string `yaml:"queryParams,omitempty" json:"queryParams,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// Body is a templated request body (POST), e.g. a GraphQL query.
	Body       string         `yaml:"body,omitempty" json:"body,omitempty"`
	Pagination PaginationSpec `yaml:"pagination,omitempty" json:"pagination,omitempty"`
	Response   ResponseSpec   `yaml:"response" json:"response"`
	// Mapping overrides spec.mapping for this operation's items.
	Mapping *MappingSpec `yaml:"mapping,omitempty" json:"mapping,omitempty"`
}

type PaginationSpec struct {
	// "cursor" (a token from the body sent back as CursorParam), "nextURL"
	// (a full URL in the body at CursorPath), "linkHeader" (RFC 8288
	// rel="next"), "page", "offset" or "none". With "cursor" and no
	// CursorParam, the token goes into the body through {{cursor}}.
	Type        string `yaml:"type" json:"type"`
	CursorParam string `yaml:"cursorParam,omitempty" json:"cursorParam,omitempty"`
	CursorPath  string `yaml:"cursorPath,omitempty" json:"cursorPath,omitempty"`
	PageParam   string `yaml:"pageParam,omitempty" json:"pageParam,omitempty"`
	OffsetParam string `yaml:"offsetParam,omitempty" json:"offsetParam,omitempty"`
	LimitParam  string `yaml:"limitParam,omitempty" json:"limitParam,omitempty"`
	Limit       int    `yaml:"limit,omitempty" json:"limit,omitempty"`
	// ZeroBased numbers pages from 0 instead of 1 ("page" pagination).
	ZeroBased bool `yaml:"zeroBased,omitempty" json:"zeroBased,omitempty"`
}

type ResponseSpec struct {
	ItemsPath string `yaml:"itemsPath" json:"itemsPath"`
}

// MappingSpec turns an API item into a memory. Each value is a path into
// the item ("fields.summary") or a template mixing text and paths
// ("{{key}}: {{fields.summary}}"); "a[].b" joins b over the array a.
type MappingSpec struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title,omitempty" json:"title,omitempty"`
	Content string `yaml:"content" json:"content"`
	// MemoryType is "semantic" (default: a document, issue, page) or
	// "episodic" (an event, message, incident).
	MemoryType string `yaml:"memoryType,omitempty" json:"memoryType,omitempty"`
	EventDate  string `yaml:"eventDate,omitempty" json:"eventDate,omitempty"`
	AuthorID   string `yaml:"authorId,omitempty" json:"authorId,omitempty"`
	AuthorName string `yaml:"authorName,omitempty" json:"authorName,omitempty"`
	URL        string `yaml:"url,omitempty" json:"url,omitempty"`
}

type SyncSpec struct {
	Mode       string `yaml:"mode" json:"mode"` // "incremental", "full"
	Checkpoint string `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
	// NewestFirst says items come newest first (deploys, pipelines): a first
	// sync stops paging once a page is older than the backfill window.
	NewestFirst bool `yaml:"newestFirst,omitempty" json:"newestFirst,omitempty"`
	// Overlap is how far before the last checkpoint a newest-first pass keeps
	// reading (default 1h), for items that change after the date they are
	// sorted by (an incident resolved days after it was created).
	Overlap string `yaml:"overlap,omitempty" json:"overlap,omitempty"`
}

// OverlapDuration is the parsed overlap, or the default.
func (s SyncSpec) OverlapDuration() time.Duration {
	if d, err := time.ParseDuration(s.Overlap); err == nil && d > 0 {
		return d
	}
	return time.Hour
}

type PolicySpec struct {
	Timeout         string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	MaxRetries      int    `yaml:"maxRetries,omitempty" json:"maxRetries,omitempty"`
	RateLimitPerMin int    `yaml:"rateLimitPerMin,omitempty" json:"rateLimitPerMin,omitempty"`
}

// ParseManifestYAML loads and validates a YAML connector manifest.
func ParseManifestYAML(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest YAML: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// ParseManifestJSON loads and validates a JSON connector manifest.
func ParseManifestJSON(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks manifest structure and semantics.
func (m *Manifest) Validate() error {
	if m.APIVersion == "" {
		return errors.New("manifest missing apiVersion (expected memro.dev/v1)")
	}
	if m.Kind != "Connector" {
		return fmt.Errorf("manifest kind must be 'Connector', got %q", m.Kind)
	}
	if m.Metadata.Name == "" {
		return errors.New("manifest metadata.name is required")
	}
	if m.Spec.Type != "rest" {
		return fmt.Errorf("unsupported connector type %q (currently 'rest' supported)", m.Spec.Type)
	}
	if m.Spec.BaseURL == "" {
		return errors.New("spec.baseURL is required")
	}
	if _, err := url.Parse(stripTemplates(m.Spec.BaseURL)); err != nil {
		return fmt.Errorf("invalid spec.baseURL: %w", err)
	}

	// Validate Auth
	switch m.Spec.Auth.Type {
	case "bearer", "apiKey", "basic", "query", "none":
	default:
		return fmt.Errorf("unknown auth type %q", m.Spec.Auth.Type)
	}
	if m.Spec.Auth.Type != "none" && m.Spec.Auth.SecretRef == "" && m.Spec.Auth.Password == "" {
		return fmt.Errorf("auth.secretRef is required for auth type %q", m.Spec.Auth.Type)
	}

	// Validate Operations
	if len(m.Spec.Operations) == 0 {
		return errors.New("spec.operations must contain at least one operation")
	}
	for i, op := range m.Spec.Operations {
		if op.Name == "" {
			return fmt.Errorf("operation[%d] missing name", i)
		}
		if op.Path == "" {
			return fmt.Errorf("operation %q missing path", op.Name)
		}
		method := strings.ToUpper(op.Method)
		if method != "GET" && method != "POST" {
			return fmt.Errorf("operation %q has unsupported method %q", op.Name, op.Method)
		}
		if op.Response.ItemsPath == "" {
			return fmt.Errorf("operation %q missing response.itemsPath", op.Name)
		}
		switch op.Pagination.Type {
		case "", "none", "cursor", "nextURL", "linkHeader", "page", "offset":
		default:
			return fmt.Errorf("operation %q has unknown pagination %q", op.Name, op.Pagination.Type)
		}
		if (op.Pagination.Type == "cursor" || op.Pagination.Type == "nextURL") && op.Pagination.CursorPath == "" {
			return fmt.Errorf("operation %q: %s pagination needs cursorPath", op.Name, op.Pagination.Type)
		}
	}
	if err := m.validateFields(); err != nil {
		return err
	}
	switch m.Spec.Mapping.MemoryType {
	case "", "semantic", "episodic":
	default:
		return fmt.Errorf("mapping.memoryType must be semantic or episodic, got %q", m.Spec.Mapping.MemoryType)
	}

	// Validate Mapping
	if m.Spec.Mapping.ID == "" {
		return errors.New("spec.mapping.id is required")
	}
	if m.Spec.Mapping.Content == "" {
		return errors.New("spec.mapping.content is required")
	}

	return nil
}

// Timeout parses the policy timeout duration or returns the default.
func (p PolicySpec) TimeoutDuration(defaultTimeout time.Duration) time.Duration {
	if p.Timeout == "" {
		return defaultTimeout
	}
	d, err := time.ParseDuration(p.Timeout)
	if err != nil {
		return defaultTimeout
	}
	return d
}

var fieldName = regexp.MustCompile(`^[a-z][a-zA-Z0-9_]*$`)

// validateFields checks the user-entered fields and that every
// {{config.x}} the manifest uses is declared; a field placed in the URL's
// host needs a pattern so it cannot redirect requests to another host.
func (m *Manifest) validateFields() error {
	declared := map[string]FieldSpec{}
	for _, f := range m.Spec.Fields {
		if !fieldName.MatchString(f.Name) {
			return fmt.Errorf("field name %q must be a lower-camel identifier", f.Name)
		}
		if f.Label == "" {
			return fmt.Errorf("field %q needs a label", f.Name)
		}
		if f.Pattern != "" {
			if _, err := regexp.Compile("^(?:" + f.Pattern + ")$"); err != nil {
				return fmt.Errorf("field %q has an invalid pattern: %w", f.Name, err)
			}
		}
		declared[f.Name] = f
	}
	if len(m.Spec.Fields) == 0 {
		return nil // legacy manifests resolve secrets through a SecretResolver
	}
	for _, name := range configRefs(m.templates()...) {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("manifest uses config.%s but declares no such field", name)
		}
	}
	if m.Spec.Auth.SecretRef != "" {
		if _, ok := declared[m.Spec.Auth.SecretRef]; !ok {
			return fmt.Errorf("auth.secretRef %q is not a declared field", m.Spec.Auth.SecretRef)
		}
	}
	if host := hostPart(m.Spec.BaseURL); strings.Contains(host, "{{") {
		for _, name := range configRefs(host) {
			if declared[name].Pattern == "" {
				return fmt.Errorf("field %q is used in the URL host and needs a pattern", name)
			}
		}
	}
	return nil
}

// templates are every request template of the manifest, to check the
// fields they use are declared.
func (m *Manifest) templates() []string {
	out := []string{m.Spec.BaseURL, m.Spec.Auth.Username, m.Spec.Auth.Password}
	for _, v := range m.Spec.Headers {
		out = append(out, v)
	}
	ops := append([]OperationSpec{}, m.Spec.Operations...)
	if m.Spec.Sources != nil {
		ops = append(ops, m.Spec.Sources.Operation)
	}
	if v := m.Spec.Verify; v != nil {
		ops = append(ops, OperationSpec{Path: v.Path, QueryParams: v.QueryParams, Body: v.Body})
	}
	for _, op := range ops {
		out = append(out, op.Path, op.Body)
		for _, v := range op.QueryParams {
			out = append(out, v)
		}
		for _, v := range op.Headers {
			out = append(out, v)
		}
	}
	return out
}

// hostPart is the scheme-less authority of a (templated) URL.
func hostPart(raw string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}
