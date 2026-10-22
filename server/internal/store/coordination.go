package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	// ErrLeaseHeldByOther means someone else (another agent, or another
	// session of the same agent) holds the resource.
	ErrLeaseHeldByOther = errors.New("resource is held by another agent or session")
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
	Session   string    `json:"session"`
	Intent    string    `json:"intent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ClaimResult struct {
	Granted       bool       `json:"granted"`
	LeaseID       *uuid.UUID `json:"lease_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Holder        string     `json:"holder,omitempty"`
	HolderSession string     `json:"holder_session,omitempty"`
	// HolderResource is the resource the holder claimed when it differs from
	// the one asked for: a folder containing it, or a path inside it.
	HolderResource string `json:"holder_resource,omitempty"`
	Intent         string `json:"intent,omitempty"`
	HandoffSummary string `json:"handoff_summary,omitempty"`
	HandoffFrom    string `json:"handoff_from,omitempty"`
}

type CheckResult struct {
	Decision    string         `json:"decision"` // "allow", "warn", "block"
	Reason      string         `json:"reason,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	Suggestions []string       `json:"suggestions,omitempty"`
}

// NormalizeResource trims whitespace and normalizes path separators.
// NormalizeResource trims r and, when it looks like a file path, cleans it
// ("./src//a.go" -> "src/a.go"). URLs and plain names are kept as given.
func NormalizeResource(r string) string {
	r = strings.TrimSpace(r)
	if r == "" || strings.Contains(r, "://") || !strings.Contains(r, "/") {
		return r
	}
	return path.Clean(r)
}

// DefaultCoordinationSession is the session used when a caller names none.
const DefaultCoordinationSession = "default"

func sessionOrDefault(s string) string {
	if s == "" {
		return DefaultCoordinationSession
	}
	return s
}

// overlapsSQL matches leases and intents on resource $2 itself, a folder
// containing it, or a path inside it: a claim on "bench" covers
// "bench/run.py", and the reverse. URLs only ever match exactly.
const overlapsSQL = `(resource = $2 OR (position('://' in resource) = 0 AND position('://' in $2) = 0 AND
	(starts_with($2, resource || '/') OR starts_with(resource, $2 || '/'))))`

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
	sessionID = sessionOrDefault(sessionID)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Claims in one scope go one at a time, so two overlapping claims (a
	// folder and a file in it) can't both pass the overlap check below.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('coordination:' || $1))`, scopeKey); err != nil {
		return nil, err
	}

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
		if existing.holderAgentID == agentID && existing.holderSession == sessionID {
			// Re-claiming your own active lease extends it (idempotent). Another
			// session of the same agent (say Cursor and Claude Code on one
			// account) is a different holder and is denied below.
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

	// 2b. A folder containing this resource, or a path inside it, held by
	// someone else blocks the claim too.
	var ov struct {
		holder, session, intent, resource string
		expiresAt                         time.Time
	}
	err = tx.QueryRow(ctx, `SELECT holder_agent_id, holder_session, intent, expires_at, resource
		FROM coordination_leases
		WHERE scope_key = $1 AND released_at IS NULL AND expires_at > NOW() AND `+overlapsSQL+`
			AND NOT (holder_agent_id = $3 AND holder_session = $4)
		ORDER BY created_at LIMIT 1`, scopeKey, resource, agentID, sessionID).
		Scan(&ov.holder, &ov.session, &ov.intent, &ov.expiresAt, &ov.resource)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		metrics.CoordinationClaims.WithLabelValues("denied").Inc()
		return &ClaimResult{
			Granted:        false,
			Holder:         ov.holder,
			HolderSession:  ov.session,
			HolderResource: ov.resource,
			Intent:         ov.intent,
			ExpiresAt:      &ov.expiresAt,
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
func (s *Store) HeartbeatLease(ctx context.Context, scopeKey, agentID, sessionID string, leaseID *uuid.UUID, resource string, ttl time.Duration) (*time.Time, error) {
	ttl = clampTTL(ttl)
	sessionID = sessionOrDefault(sessionID)
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
			WHERE resource = $1 AND scope_key = $2 AND holder_agent_id = $3 AND holder_session = $5 AND released_at IS NULL AND expires_at > NOW()
			RETURNING expires_at`, resource, scopeKey, agentID, ttl, sessionID).Scan(&newExpires)
	} else {
		return nil, fmt.Errorf("lease_id or resource is required")
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLeaseNotFound
	}
	return &newExpires, err
}

// ReleaseLease releases the active lease held by the caller.
func (s *Store) ReleaseLease(ctx context.Context, scopeKey, agentID, sessionID string, leaseID *uuid.UUID, resource, status string) error {
	sessionID = sessionOrDefault(sessionID)
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
			WHERE resource = $1 AND scope_key = $2 AND holder_agent_id = $3 AND holder_session = $5 AND released_at IS NULL`,
			resource, scopeKey, agentID, status, sessionID)
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

// HandoffLease releases the caller's lease with status 'handoff' and records
// the summary for the next claimer. With no active lease, the handoff note is
// still recorded (as an already-released lease), so tools can hand off work
// they only logged an intent for. It fails only when someone else holds the
// resource.
func (s *Store) HandoffLease(ctx context.Context, scopeKey, agentID, sessionID string, leaseID *uuid.UUID, resource, summary, nextAgentID string) error {
	resource = NormalizeResource(resource)
	sessionID = sessionOrDefault(sessionID)
	var nextTo *string
	if nextAgentID != "" {
		nextTo = &nextAgentID
	}
	if leaseID == nil && resource == "" {
		return fmt.Errorf("lease_id or resource is required")
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var tag pgconn.CommandTag
	if leaseID != nil {
		tag, err = tx.Exec(ctx, `UPDATE coordination_leases
			SET released_at = NOW(), release_status = 'handoff', handoff_summary = $4, handoff_to = $5
			WHERE id = $1 AND scope_key = $2 AND holder_agent_id = $3 AND released_at IS NULL`,
			*leaseID, scopeKey, agentID, summary, nextTo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrLeaseNotFound
		}
	} else {
		tag, err = tx.Exec(ctx, `UPDATE coordination_leases
			SET released_at = NOW(), release_status = 'handoff', handoff_summary = $4, handoff_to = $5
			WHERE resource = $1 AND scope_key = $2 AND holder_agent_id = $3 AND holder_session = $6 AND released_at IS NULL`,
			resource, scopeKey, agentID, summary, nextTo, sessionID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var held bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM coordination_leases
				WHERE scope_key = $1 AND resource = $2 AND released_at IS NULL AND expires_at > NOW())`,
				scopeKey, resource).Scan(&held); err != nil {
				return err
			}
			if held {
				return ErrLeaseHeldByOther
			}
			if _, err := tx.Exec(ctx, `INSERT INTO coordination_leases
				(id, scope_key, resource, holder_agent_id, holder_session, intent, created_at, heartbeat_at, expires_at,
				 released_at, release_status, handoff_summary, handoff_to)
				VALUES ($1, $2, $3, $4, $5, '', NOW(), NOW(), NOW(), NOW(), 'handoff', $6, $7)`,
				uuid.New(), scopeKey, resource, agentID, sessionID, summary, nextTo); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	metrics.CoordinationHandoffs.Inc()
	return nil
}

// CheckCoordination returns allow, warn (active intent by others), or block (active lease by others).
func (s *Store) CheckCoordination(ctx context.Context, scopeKey, resource, agentID, sessionID, intent string) (*CheckResult, error) {
	sessionID = sessionOrDefault(sessionID)
	resource = NormalizeResource(resource)
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}

	// 1. An active lease by someone else on this resource, a folder
	// containing it, or a path inside it blocks. Your own leases allow.
	var holderID, holderSession, holderIntent, heldResource string
	var expiresAt time.Time
	err := s.Pool.QueryRow(ctx, `SELECT holder_agent_id, holder_session, intent, expires_at, resource
		FROM coordination_leases 
		WHERE scope_key = $1 AND released_at IS NULL AND expires_at > NOW() AND `+overlapsSQL+`
			AND NOT (holder_agent_id = $3 AND holder_session = $4)
		ORDER BY (resource = $2) DESC, created_at LIMIT 1`,
		scopeKey, resource, agentID, sessionID).Scan(&holderID, &holderSession, &holderIntent, &expiresAt, &heldResource)

	if err == nil {
		reason := fmt.Sprintf("Resource '%s' is currently claimed by session '%s'.", resource, holderSession)
		if heldResource != resource {
			reason = fmt.Sprintf("Resource '%s' overlaps '%s', currently claimed by session '%s'.", resource, heldResource, holderSession)
		}
		return &CheckResult{
			Decision: "block",
			Reason:   reason,
			Details: map[string]any{
				"agent":      holderID,
				"session":    holderSession,
				"resource":   heldResource,
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
		WHERE scope_key = $1 AND `+overlapsSQL+` AND NOT (agent_id = $3 AND session = $4) AND expires_at > NOW()`,
		scopeKey, resource, agentID, sessionID)
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
		intentRows, err = s.Pool.Query(ctx, `SELECT id, scope_key, resource, agent_id, session, intent, created_at, expires_at
			FROM coordination_intents 
			WHERE scope_key = $1 AND expires_at > NOW() AND (resource = $2 OR resource LIKE $2 || '/%')
			ORDER BY created_at DESC`, scopeKey, resourcePrefix)
	} else {
		intentRows, err = s.Pool.Query(ctx, `SELECT id, scope_key, resource, agent_id, session, intent, created_at, expires_at
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
		if err := intentRows.Scan(&it.ID, &it.ScopeKey, &it.Resource, &it.AgentID, &it.Session, &it.Intent, &it.CreatedAt, &it.ExpiresAt); err != nil {
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
func (s *Store) LogIntent(ctx context.Context, scopeKey, resource, agentID, sessionID, intent string, ttl time.Duration) (*Intent, error) {
	sessionID = sessionOrDefault(sessionID)
	resource = NormalizeResource(resource)
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}
	ttl = clampTTL(ttl)
	id := uuid.New()
	var createdAt, expiresAt time.Time
	err := s.Pool.QueryRow(ctx, `INSERT INTO coordination_intents 
		(id, scope_key, resource, agent_id, session, intent, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $7, $5, NOW(), NOW() + $6)
		RETURNING created_at, expires_at`,
		id, scopeKey, resource, agentID, intent, ttl, sessionID).Scan(&createdAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	return &Intent{
		ID:        id,
		ScopeKey:  scopeKey,
		Resource:  resource,
		AgentID:   agentID,
		Session:   sessionID,
		Intent:    intent,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	}, nil
}

// coordinationRetention is how long released leases and expired intents are
// kept (for handoff notes and audit) before they are purged.
const coordinationRetention = 30 * 24 * time.Hour

// PurgeCoordination deletes leases released, and intents expired, before
// olderThan ago. Active leases are never touched.
func (s *Store) PurgeCoordination(ctx context.Context, olderThan time.Duration) (int64, error) {
	a, err := s.Pool.Exec(ctx, `DELETE FROM coordination_leases WHERE released_at IS NOT NULL AND released_at < NOW() - $1::interval`, olderThan)
	if err != nil {
		return 0, err
	}
	b, err := s.Pool.Exec(ctx, `DELETE FROM coordination_intents WHERE expires_at < NOW() - $1::interval`, olderThan)
	if err != nil {
		return 0, err
	}
	return a.RowsAffected() + b.RowsAffected(), nil
}

// RunCoordinationPurge purges old coordination rows every hour until ctx ends.
func RunCoordinationPurge(ctx context.Context, s *Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if _, err := s.PurgeCoordination(ctx, coordinationRetention); err != nil && ctx.Err() == nil {
			slog.Warn("coordination purge failed", "err", err)
		}
	}
}
