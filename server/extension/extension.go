// Package extension defines the interfaces a Memro distribution can plug in
// to customise behaviour without forking the server: usage metering and
// quotas. The community server uses the defaults in this package.
package extension

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Usage metrics recorded by the server.
const (
	MetricMemoriesStored   = "memories_stored"
	MetricMemoriesSearched = "memories_searched"

	// Metrics a UsageGate is asked about before an action, and recorded after it.
	MetricQueries      = "queries"       // search, temporal search, reason, conflict analysis, atom generation: 1 each
	MetricIngestBytes  = "ingest_bytes"  // size of each uploaded file
	MetricStoredTokens = "stored_tokens" // estimated tokens of memory text written (see EstimateTokens)
)

// EstimateTokens approximates the tokens in text as one per four bytes,
// rounded up. It is the amount used for MetricStoredTokens.
func EstimateTokens(text string) int64 { return int64((len(text) + 3) / 4) }

// UsageEvent is one billable or reportable action.
type UsageEvent struct {
	TenantID uuid.UUID
	AgentID  string
	Metric   string
	Value    int64
}

// UsageMeter records usage. Record is called on the request path, so
// implementations should be fast or hand events off asynchronously.
type UsageMeter interface {
	Record(ctx context.Context, e UsageEvent) error
}

// UsageMeterFunc adapts a function to UsageMeter.
type UsageMeterFunc func(ctx context.Context, e UsageEvent) error

func (f UsageMeterFunc) Record(ctx context.Context, e UsageEvent) error { return f(ctx, e) }

// MultiMeter sends each event to every meter and returns the first error.
func MultiMeter(meters ...UsageMeter) UsageMeter {
	return UsageMeterFunc(func(ctx context.Context, e UsageEvent) error {
		var first error
		for _, m := range meters {
			if err := m.Record(ctx, e); err != nil && first == nil {
				first = err
			}
		}
		return first
	})
}

// ErrQuotaExceeded is returned (possibly wrapped) by QuotaPolicy when a
// write would exceed the caller's limits. Its message is shown to the client.
var ErrQuotaExceeded = errors.New("quota exceeded")

// QuotaError carries a client-facing message and unwraps to ErrQuotaExceeded.
type QuotaError struct{ Message string }

func (e *QuotaError) Error() string { return e.Message }
func (e *QuotaError) Unwrap() error { return ErrQuotaExceeded }

// QuotaPolicy decides whether an agent may store more memories.
// current is the agent's memory count; adding is how many the request adds.
type QuotaPolicy interface {
	AllowMemoryWrite(ctx context.Context, tenantID uuid.UUID, agentID string, current, adding int) error
	// SoftLimit is the per-agent memory limit reported to clients (0 = none).
	SoftLimit(ctx context.Context, tenantID uuid.UUID, agentID string) int
}

// UsageGate is an optional extension of QuotaPolicy. When the configured
// QuotaPolicy also implements it, the server calls AllowUsage before each
// counted action (see the Metric* constants) and refuses the request with 403
// and the error's message when it returns an error wrapping ErrQuotaExceeded.
// It only decides: the server records a UsageEvent with the same metric and
// amount after the action succeeds. agentID is always the caller's own agent.
type UsageGate interface {
	AllowUsage(ctx context.Context, tenantID uuid.UUID, agentID, metric string, amount int64) error
}

// PerAgentCap allows up to Max memories per agent; Max <= 0 means unlimited.
type PerAgentCap struct{ Max int }

func (c PerAgentCap) AllowMemoryWrite(_ context.Context, _ uuid.UUID, _ string, current, adding int) error {
	if c.Max > 0 && current+adding > c.Max {
		return &QuotaError{Message: fmt.Sprintf("Memory soft cap reached (%d/agent). Please prune old memories to continue in Developer Preview.", c.Max)}
	}
	return nil
}

func (c PerAgentCap) SoftLimit(context.Context, uuid.UUID, string) int { return c.Max }
