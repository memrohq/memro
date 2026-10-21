package dce

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest defines the declarative contract for an external connector (ADR 0012).
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
}

type Spec struct {
	Type       string          `yaml:"type" json:"type"`
	BaseURL    string          `yaml:"baseURL" json:"baseURL"`
	Auth       AuthSpec        `yaml:"auth" json:"auth"`
	Operations []OperationSpec `yaml:"operations" json:"operations"`
	Mapping    MappingSpec     `yaml:"mapping" json:"mapping"`
	Sync       SyncSpec        `yaml:"sync" json:"sync"`
	Policies   PolicySpec      `yaml:"policies,omitempty" json:"policies,omitempty"`
}

type AuthSpec struct {
	Type      string `yaml:"type" json:"type"` // "bearer", "apiKey", "basic", "none"
	SecretRef string `yaml:"secretRef" json:"secretRef"`
	Header    string `yaml:"header,omitempty" json:"header,omitempty"` // For apiKey: header name, e.g. "X-API-Key"
	Prefix    string `yaml:"prefix,omitempty" json:"prefix,omitempty"` // Header prefix, e.g. "Bearer "
}

type OperationSpec struct {
	Name        string            `yaml:"name" json:"name"`
	Method      string            `yaml:"method" json:"method"` // GET, POST
	Path        string            `yaml:"path" json:"path"`
	QueryParams map[string]string `yaml:"queryParams,omitempty" json:"queryParams,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Pagination  PaginationSpec    `yaml:"pagination,omitempty" json:"pagination,omitempty"`
	Response    ResponseSpec      `yaml:"response" json:"response"`
}

type PaginationSpec struct {
	Type        string `yaml:"type" json:"type"` // "cursor", "page", "offset", "none"
	CursorParam string `yaml:"cursorParam,omitempty" json:"cursorParam,omitempty"`
	CursorPath  string `yaml:"cursorPath,omitempty" json:"cursorPath,omitempty"`
	PageParam   string `yaml:"pageParam,omitempty" json:"pageParam,omitempty"`
	OffsetParam string `yaml:"offsetParam,omitempty" json:"offsetParam,omitempty"`
	LimitParam  string `yaml:"limitParam,omitempty" json:"limitParam,omitempty"`
	Limit       int    `yaml:"limit,omitempty" json:"limit,omitempty"`
}

type ResponseSpec struct {
	ItemsPath string `yaml:"itemsPath" json:"itemsPath"`
}

type MappingSpec struct {
	ID         string `yaml:"id" json:"id"`
	Title      string `yaml:"title,omitempty" json:"title,omitempty"`
	Content    string `yaml:"content" json:"content"`
	EventDate  string `yaml:"eventDate,omitempty" json:"eventDate,omitempty"`
	AuthorID   string `yaml:"authorId,omitempty" json:"authorId,omitempty"`
	AuthorName string `yaml:"authorName,omitempty" json:"authorName,omitempty"`
	URL        string `yaml:"url,omitempty" json:"url,omitempty"`
}

type SyncSpec struct {
	Mode       string `yaml:"mode" json:"mode"` // "incremental", "full"
	Checkpoint string `yaml:"checkpoint,omitempty" json:"checkpoint,omitempty"`
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
	if _, err := url.Parse(m.Spec.BaseURL); err != nil {
		return fmt.Errorf("invalid spec.baseURL: %w", err)
	}

	// Validate Auth
	switch m.Spec.Auth.Type {
	case "bearer", "apiKey", "basic", "none":
	default:
		return fmt.Errorf("unknown auth type %q", m.Spec.Auth.Type)
	}
	if m.Spec.Auth.Type != "none" && m.Spec.Auth.SecretRef == "" {
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
