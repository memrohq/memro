package store

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/memrohq/memro/server/internal/metrics"
)

var (
	ErrLeaseNotFound = errors.New("coordination lease not found, expired, or not owned by caller")
)

const (
	DefaultLeaseTTL = 15 * time.Minute
	MaxLeaseTTL     = 2 * time.Hour
	MinLeaseTTL     = 1 * time.Second
)

type Lease struct {
	ID             uuid.UUID  `json:"id"`
	ScopeKey       string     `json:"scope_key"`
	Resource       string     `json:"resource"`
	HolderAgentID  string     `json:"holder_agent_id"`
	HolderSession  string     `json:"holder_session"`
	Intent         string     `json:"intent"`
	CreatedAt      time.Time  `json:"created_at"`
	HeartbeatAt    time.Time  `json:"heartbeat_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
	ReleaseStatus  *string    `json:"release_status,omitempty"`
	HandoffSummary *string    `json:"handoff_summary,omitempty"`
	HandoffTo      *string    `json:"handoff_to,omitempty"`
}

type Intent struct {
	ID        uuid.UUID `json:"id"`
	ScopeKey  string    `json:"scope_key"`
	Resource  string    `json:"resource"`
	AgentID   string    `json:"agent_id"`
	Intent    string    `json:"intent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ClaimResult struct {
	Granted        bool       `json:"granted"`
	LeaseID        *uuid.UUID `json:"lease_id,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Holder         string     `json:"holder,omitempty"`
	HolderSession  string     `json:"holder_session,omitempty"`
	Intent         string     `json:"intent,omitempty"`
	HandoffSummary string     `json:"handoff_summary,omitempty"`
	HandoffFrom    string     `json:"handoff_from,omitempty"`
}

type CheckResult struct {
	Decision    string         `json:"decision"` // "allow", "warn", "block"
	Reason      string         `json:"reason,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	Suggestions []string       `json:"suggestions,omitempty"`
}

// NormalizeResource trims whitespace and normalizes path separators.
func NormalizeResource(r string) string {
	r = strings.TrimSpace(r)
	if r == "" {
		return ""
	}
	return path.Clean(r)
}

// CoordinationScopeKey derives the isolation scope:
// 1. Tenant/Space UUID when TenantID != uuid.Nil
// 2. Owning User UUID when UserID != nil
// 3. Otherwise Agent ID (for unclaimed self-signed-up agents)
func CoordinationScopeKey(tenantID uuid.UUID, userID *uuid.UUID, agentID string) string {
	if tenantID != uuid.Nil {
		return "tenant:" + tenantID.String()
	}
	if userID != nil && *userID != uuid.Nil {
		return "user:" + userID.String()
	}
	return "agent:" + agentID
}

func clampTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return DefaultLeaseTTL
	}
	if ttl < MinLeaseTTL {
		return MinLeaseTTL
	}
	if ttl > MaxLeaseTTL {
		return MaxLeaseTTL
	}
	return ttl
}

// ClaimLease attempts to atomically acquire or extend a lease.
func (s *Store) ClaimLease(ctx context.Context, scopeKey, resource, agentID, sessionID, intent string, ttl time.Duration) (*ClaimResult, error) {
	resource = NormalizeResource(resource)
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}
	ttl = clampTTL(ttl)
	if sessionID == "" {
		sessionID = "default"
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Atomically mark expired leases released (status 'expired') in this scope & resource
	tag, err := tx.Exec(ctx, `UPDATE coordination_leases 
		SET released_at = NOW(), release_status = 'expired'
		WHERE scope_key = $1 AND resource = $2 AND released_at IS NULL AND expires_at <= NOW()`,
		scopeKey, resource)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() > 0 {
		metrics.CoordinationExpirations.Add(float64(tag.RowsAffected()))
	}

	// 2. Check for active lease
	var existing struct {
		id            uuid.UUID
		holderAgentID string
		holderSession string
		intent        string
		expiresAt     time.Time
	}
	err = tx.QueryRow(ctx, `SELECT id, holder_agent_id, holder_session, intent, expires_at 
		FROM coordination_leases 
		WHERE scope_key = $1 AND resource = $2 AND released_at IS NULL 
		FOR UPDATE`, scopeKey, resource).Scan(&existing.id, &existing.holderAgentID, &existing.holderSession, &existing.intent, &existing.expiresAt)

	if err == nil {
		// Active lease exists
		if existing.holderAgentID == agentID {
			// Re-claiming your own active lease extends it (idempotent)
			var newExpires time.Time
			updateSQL := `UPDATE coordination_leases 
				SET expires_at = NOW() + $2, heartbeat_at = NOW(),
					intent = CASE WHEN $3 != '' THEN $3 ELSE intent END
				WHERE id = $1 RETURNING expires_at`
			if err := tx.QueryRow(ctx, updateSQL, existing.id, ttl, intent).Scan(&newExpires); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			metrics.CoordinationClaims.WithLabelValues("granted").Inc()
			return &ClaimResult{
				Granted:   true,
				LeaseID:   &existing.id,
				ExpiresAt: &newExpires,
			}, nil
		}

		// Held by another agent
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		metrics.CoordinationClaims.WithLabelValues("denied").Inc()
		return &ClaimResult{
			Granted:       false,
			Holder:        existing.holderAgentID,
			HolderSession: existing.holderSession,
			Intent:        existing.intent,
			ExpiresAt:     &existing.expiresAt,
		}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// 3. No active lease: check if there was a previous handoff for this resource
	var handoffSummary, handoffFrom string
	var prevHandoffSummary, prevHolder, prevHandoffTo *string
	err = tx.QueryRow(ctx, `SELECT handoff_summary, holder_agent_id, handoff_to 
		FROM coordination_leases 
		WHERE scope_key = $1 AND resource = $2 AND release_status = 'handoff' 
		ORDER BY released_at DESC LIMIT 1`, scopeKey, resource).Scan(&prevHandoffSummary, &prevHolder, &prevHandoffTo)
	if err == nil {
		if prevHandoffSummary != nil && (prevHandoffTo == nil || *prevHandoffTo == "" || *prevHandoffTo == agentID) {
			handoffSummary = *prevHandoffSummary
			if prevHolder != nil {
				handoffFrom = *prevHolder
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// 4. Insert new lease
	leaseID := uuid.New()
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO coordination_leases 
		(id, scope_key, resource, holder_agent_id, holder_session, intent, created_at, heartbeat_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW(), NOW() + $7)
		ON CONFLICT (scope_key, resource) WHERE released_at IS NULL DO NOTHING
		RETURNING expires_at`,
		leaseID, scopeKey, resource, agentID, sessionID, intent, ttl).Scan(&expiresAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pgErr) && pgErr.Code == "23505") {
			// Another concurrent transaction won the race and inserted the lease
			var conflictHolder, conflictSession, conflictIntent string
			var conflictExpires time.Time
			queryErr := tx.QueryRow(ctx, `SELECT holder_agent_id, holder_session, intent, expires_at 
				FROM coordination_leases 
				WHERE scope_key = $1 AND resource = $2 AND released_at IS NULL`, scopeKey, resource).
				Scan(&conflictHolder, &conflictSession, &conflictIntent, &conflictExpires)
			if queryErr != nil {
				return nil, queryErr
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			metrics.CoordinationClaims.WithLabelValues("denied").Inc()
			return &ClaimResult{
				Granted:       false,
				Holder:        conflictHolder,
				HolderSession: conflictSession,
				Intent:        conflictIntent,
				ExpiresAt:     &conflictExpires,
			}, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	metrics.CoordinationClaims.WithLabelValues("granted").Inc()
	return &ClaimResult{
		Granted:        true,
		LeaseID:        &leaseID,
		ExpiresAt:      &expiresAt,
		HandoffSummary: handoffSummary,
		HandoffFrom:    handoffFrom,
	}, nil
}

// HeartbeatLease extends expires_at for the holder's lease.
func (s *Store) HeartbeatLease(ctx context.Context, scopeKey, agentID string, leaseID *uuid.UUID, resource string, ttl time.Duration) (*time.Time, error) {
	ttl = clampTTL(ttl)
	resource = NormalizeResource(resource)

	var newExpires time.Time
	var err error
	if leaseID != nil {
		err = s.Pool.QueryRow(ctx, `UPDATE coordination_leases 
			SET expires_at = NOW() + $4, heartbeat_at = NOW()
			WHERE id = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL AND expires_at > NOW()
			RETURNING expires_at`, *leaseID, scopeKey, agentID, ttl).Scan(&newExpires)
	} else if resource != "" {
		err = s.Pool.QueryRow(ctx, `UPDATE coordination_leases 
			SET expires_at = NOW() + $4, heartbeat_at = NOW()
			WHERE resource = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL AND expires_at > NOW()
			RETURNING expires_at`, resource, scopeKey, agentID, ttl).Scan(&newExpires)
	} else {
		return nil, fmt.Errorf("lease_id or resource is required")
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLeaseNotFound
	}
	return &newExpires, err
}

// ReleaseLease releases the active lease held by the caller.
func (s *Store) ReleaseLease(ctx context.Context, scopeKey, agentID string, leaseID *uuid.UUID, resource, status string) error {
	if status != "completed" && status != "abandoned" {
		status = "completed"
	}
	resource = NormalizeResource(resource)

	var tag pgconn.CommandTag
	var err error
	if leaseID != nil {
		tag, err = s.Pool.Exec(ctx, `UPDATE coordination_leases 
			SET released_at = NOW(), release_status = $4
			WHERE id = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL`,
			*leaseID, scopeKey, agentID, status)
	} else if resource != "" {
		tag, err = s.Pool.Exec(ctx, `UPDATE coordination_leases 
			SET released_at = NOW(), release_status = $4
			WHERE resource = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL`,
			resource, scopeKey, agentID, status)
	} else {
		return fmt.Errorf("lease_id or resource is required")
	}

	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseNotFound
	}
	return nil
}

// HandoffLease releases a lease with status 'handoff' and records summary for next claimer.
func (s *Store) HandoffLease(ctx context.Context, scopeKey, agentID string, leaseID *uuid.UUID, resource, summary, nextAgentID string) error {
	resource = NormalizeResource(resource)
	var nextTo *string
	if nextAgentID != "" {
		nextTo = &nextAgentID
	}

	var tag pgconn.CommandTag
	var err error
	if leaseID != nil {
		tag, err = s.Pool.Exec(ctx, `UPDATE coordination_leases 
			SET released_at = NOW(), release_status = 'handoff', handoff_summary = $4, handoff_to = $5
			WHERE id = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL`,
			*leaseID, scopeKey, agentID, summary, nextTo)
	} else if resource != "" {
		tag, err = s.Pool.Exec(ctx, `UPDATE coordination_leases 
			SET released_at = NOW(), release_status = 'handoff', handoff_summary = $4, handoff_to = $5
			WHERE resource = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL`,
			resource, scopeKey, agentID, summary, nextTo)
	} else {
		return fmt.Errorf("lease_id or resource is required")
	}

	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseNotFound
	}
	metrics.CoordinationHandoffs.Inc()
	return nil
}

// CheckCoordination returns allow, warn (active intent by others), or block (active lease by others).
func (s *Store) CheckCoordination(ctx context.Context, scopeKey, resource, agentID, intent string) (*CheckResult, error) {
	resource = NormalizeResource(resource)
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}

	// 1. Check for active lease by anyone
	var holderID, holderSession, holderIntent string
	var expiresAt time.Time
	err := s.Pool.QueryRow(ctx, `SELECT holder_agent_id, holder_session, intent, expires_at 
		FROM coordination_leases 
		WHERE scope_key = $1 AND resource = $2 AND released_at IS NULL AND expires_at > NOW()`,
		scopeKey, resource).Scan(&holderID, &holderSession, &holderIntent, &expiresAt)

	if err == nil {
		if holderID == agentID {
			// Your own lease returns allow
			return &CheckResult{Decision: "allow"}, nil
		}
		return &CheckResult{
			Decision: "block",
			Reason:   fmt.Sprintf("Resource '%s' is currently claimed.", resource),
			Details: map[string]any{
				"agent":      holderID,
				"session":    holderSession,
				"resource":   resource,
				"reason":     holderIntent,
				"expires_at": expiresAt,
			},
		}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// 2. Check for active intents by others
	rows, err := s.Pool.Query(ctx, `SELECT agent_id, intent, expires_at 
		FROM coordination_intents 
		WHERE scope_key = $1 AND resource = $2 AND agent_id != $3 AND expires_at > NOW()`,
		scopeKey, resource, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var intents []map[string]any
	for rows.Next() {
		var a, it string
		var exp time.Time
		if err := rows.Scan(&a, &it, &exp); err != nil {
			return nil, err
		}
		intents = append(intents, map[string]any{
			"agent":      a,
			"intent":     it,
			"resource":   resource,
			"expires_at": exp,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(intents) > 0 {
		return &CheckResult{
			Decision: "warn",
			Reason:   fmt.Sprintf("Active intent(s) logged by other agents for '%s'", resource),
			Details: map[string]any{
				"intents": intents,
			},
			Suggestions: []string{"Coordinate via chat", "Check related files"},
		}, nil
	}

	return &CheckResult{Decision: "allow"}, nil
}

// ActiveCoordination lists active leases and intents in the scope, optionally filtered by resource prefix.
func (s *Store) ActiveCoordination(ctx context.Context, scopeKey, resourcePrefix string) ([]Lease, []Intent, error) {
	resourcePrefix = NormalizeResource(resourcePrefix)

	var leaseRows pgx.Rows
	var err error
	if resourcePrefix != "" {
		leaseRows, err = s.Pool.Query(ctx, `SELECT id, scope_key, resource, holder_agent_id, holder_session, intent, created_at, heartbeat_at, expires_at, released_at, release_status, handoff_summary, handoff_to
			FROM coordination_leases 
			WHERE scope_key = $1 AND released_at IS NULL AND expires_at > NOW() AND (resource = $2 OR resource LIKE $2 || '/%')
			ORDER BY created_at DESC`, scopeKey, resourcePrefix)
	} else {
		leaseRows, err = s.Pool.Query(ctx, `SELECT id, scope_key, resource, holder_agent_id, holder_session, intent, created_at, heartbeat_at, expires_at, released_at, release_status, handoff_summary, handoff_to
			FROM coordination_leases 
			WHERE scope_key = $1 AND released_at IS NULL AND expires_at > NOW()
			ORDER BY created_at DESC`, scopeKey)
	}
	if err != nil {
		return nil, nil, err
	}
	defer leaseRows.Close()

	leases := []Lease{}
	for leaseRows.Next() {
		var l Lease
		if err := leaseRows.Scan(&l.ID, &l.ScopeKey, &l.Resource, &l.HolderAgentID, &l.HolderSession, &l.Intent,
			&l.CreatedAt, &l.HeartbeatAt, &l.ExpiresAt, &l.ReleasedAt, &l.ReleaseStatus, &l.HandoffSummary, &l.HandoffTo); err != nil {
			return nil, nil, err
		}
		leases = append(leases, l)
	}
	if err := leaseRows.Err(); err != nil {
		return nil, nil, err
	}

	var intentRows pgx.Rows
	if resourcePrefix != "" {
		intentRows, err = s.Pool.Query(ctx, `SELECT id, scope_key, resource, agent_id, intent, created_at, expires_at
			FROM coordination_intents 
			WHERE scope_key = $1 AND expires_at > NOW() AND (resource = $2 OR resource LIKE $2 || '/%')
			ORDER BY created_at DESC`, scopeKey, resourcePrefix)
	} else {
		intentRows, err = s.Pool.Query(ctx, `SELECT id, scope_key, resource, agent_id, intent, created_at, expires_at
			FROM coordination_intents 
			WHERE scope_key = $1 AND expires_at > NOW()
			ORDER BY created_at DESC`, scopeKey)
	}
	if err != nil {
		return nil, nil, err
	}
	defer intentRows.Close()

	intents := []Intent{}
	for intentRows.Next() {
		var it Intent
		if err := intentRows.Scan(&it.ID, &it.ScopeKey, &it.Resource, &it.AgentID, &it.Intent, &it.CreatedAt, &it.ExpiresAt); err != nil {
			return nil, nil, err
		}
		intents = append(intents, it)
	}
	if err := intentRows.Err(); err != nil {
		return nil, nil, err
	}

	return leases, intents, nil
}

// LogIntent records a soft, non-blocking intent.
func (s *Store) LogIntent(ctx context.Context, scopeKey, resource, agentID, intent string, ttl time.Duration) (*Intent, error) {
	resource = NormalizeResource(resource)
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}
	ttl = clampTTL(ttl)
	id := uuid.New()
	var createdAt, expiresAt time.Time
	err := s.Pool.QueryRow(ctx, `INSERT INTO coordination_intents 
		(id, scope_key, resource, agent_id, intent, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW() + $6)
		RETURNING created_at, expires_at`,
		id, scopeKey, resource, agentID, intent, ttl).Scan(&createdAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	return &Intent{
		ID:        id,
		ScopeKey:  scopeKey,
		Resource:  resource,
		AgentID:   agentID,
		Intent:    intent,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	}, nil
}
