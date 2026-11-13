package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Plan limits per organisation (ADR 0028), in Memro Cloud's public units:
// stored tokens (bytes of memory text / 4) and queries.

// TenantLimits are an organisation's hard limits; nil means none.
type TenantLimits struct {
	Plan                   string     `json:"plan"`
	MaxStoredTokensPerUser *int64     `json:"max_stored_tokens_per_user"`
	MaxStoredTokensTotal   *int64     `json:"max_stored_tokens_total"`
	MaxQueriesMonthly      *int64     `json:"max_queries_monthly"`
	MaxMembers             *int       `json:"max_members"`
	AllowImages            bool       `json:"allow_images"`
	TrialEndsAt            *time.Time `json:"trial_ends_at"`
	TrialNotice            *string    `json:"trial_notice,omitempty"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

const limitCols = `plan, max_stored_tokens_per_user, max_stored_tokens_total, max_queries_monthly, max_members,
	allow_images, trial_ends_at, trial_notice, updated_at`

func (l *TenantLimits) dest() []any {
	return []any{&l.Plan, &l.MaxStoredTokensPerUser, &l.MaxStoredTokensTotal, &l.MaxQueriesMonthly, &l.MaxMembers,
		&l.AllowImages, &l.TrialEndsAt, &l.TrialNotice, &l.UpdatedAt}
}

// GetTenantLimits returns the tenant's limits, or nil when it has none and
// the server's quota policy applies.
func (s *Store) GetTenantLimits(ctx context.Context, tenantID uuid.UUID) (*TenantLimits, error) {
	var l TenantLimits
	err := s.Pool.QueryRow(ctx, `SELECT `+limitCols+` FROM tenant_limits WHERE tenant_id = $1`, tenantID).Scan(l.dest()...)
	return nilIfNoRows(&l, err)
}

const insertLimits = `INSERT INTO tenant_limits (tenant_id, plan, max_stored_tokens_per_user, max_stored_tokens_total,
		max_queries_monthly, max_members, allow_images, trial_ends_at, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

func limitArgs(tenantID uuid.UUID, l TenantLimits, by *uuid.UUID) []any {
	return []any{tenantID, l.Plan, l.MaxStoredTokensPerUser, l.MaxStoredTokensTotal, l.MaxQueriesMonthly, l.MaxMembers,
		l.AllowImages, l.TrialEndsAt, by}
}

// SetTenantLimits replaces the tenant's limits and restarts its warnings
// and trial emails; by is nil for the system.
func (s *Store) SetTenantLimits(ctx context.Context, tenantID uuid.UUID, l TenantLimits, by *uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, insertLimits+` ON CONFLICT (tenant_id) DO UPDATE SET plan = $2,
		max_stored_tokens_per_user = $3, max_stored_tokens_total = $4, max_queries_monthly = $5, max_members = $6,
		allow_images = $7, trial_ends_at = $8, updated_by = $9, updated_at = NOW(), trial_notice = NULL, warned_at = NULL`,
		limitArgs(tenantID, l, by)...)
	return err
}

// DeleteTenantLimits returns the tenant to the server's quota policy.
func (s *Store) DeleteTenantLimits(ctx context.Context, tenantID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM tenant_limits WHERE tenant_id = $1`, tenantID)
	return err
}

// The estimate matches extension.EstimateTokens summed per memory.
const storedTokensExpr = `COALESCE(SUM((octet_length(content) + 3) / 4), 0)::BIGINT`

// TenantStoredTokens is the stored text of every agent in a tenant, in tokens.
// shortcut: a SUM per write, fine for trial-sized pools (the only hard total
// limit); keep a running total if paid plans get hard limits in the millions.
func (s *Store) TenantStoredTokens(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT `+storedTokensExpr+` FROM memories WHERE tenant_id = $1`, tenantID).Scan(&n)
	return n, err
}

// AgentStoredTokens is one agent's stored text in tokens.
func (s *Store) AgentStoredTokens(ctx context.Context, agentID string) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT `+storedTokensExpr+` FROM memories WHERE agent_id = $1`, agentID).Scan(&n)
	return n, err
}

// TenantMonthUsage is a metric's total across a tenant's agents this month (UTC).
func (s *Store) TenantMonthUsage(ctx context.Context, tenantID uuid.UUID, metric string) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(value), 0)::BIGINT FROM usage_monthly
		WHERE tenant_id = $1 AND metric = $2 AND month = date_trunc('month', NOW() AT TIME ZONE 'UTC')::date`,
		tenantID, metric).Scan(&n)
	return n, err
}

// Seats counts members plus pending invitations.
func (s *Store) Seats(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM tenant_members WHERE tenant_id = $1)
		+ (SELECT COUNT(*) FROM tenant_invites WHERE tenant_id = $1 AND accepted_at IS NULL AND expires_at > NOW())`,
		tenantID).Scan(&n)
	return n, err
}

// MarkWarned records that owners were told the tenant passed 80% of a limit.
// It reports false when they already were, so only one caller emails.
func (s *Store) MarkWarned(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE tenant_limits SET warned_at = NOW() WHERE tenant_id = $1 AND warned_at IS NULL`, tenantID)
	return tag.RowsAffected() > 0, err
}

// OwnerEmails lists the addresses of a tenant's owners.
func (s *Store) OwnerEmails(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT u.email FROM tenant_members m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1 AND m.role = 'owner' ORDER BY u.email`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// TrialState is a trial organisation the lifecycle job looks at.
type TrialState struct {
	TenantID    uuid.UUID
	Name        string
	TrialEndsAt time.Time
	Notice      *string
}

// EndedTrials lists organisations still on a trial whose trial has ended.
func (s *Store) EndedTrials(ctx context.Context) ([]TrialState, error) {
	rows, err := s.Pool.Query(ctx, `SELECT l.tenant_id, t.name, l.trial_ends_at, l.trial_notice
		FROM tenant_limits l JOIN tenants t ON t.id = l.tenant_id
		WHERE l.plan = 'trial' AND l.trial_ends_at < NOW() AND l.trial_notice IS DISTINCT FROM 'deleted'`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[TrialState])
}

// SetTrialNotice records the last trial email stage, if the trial still
// ends when the caller saw it (staff may have extended it meanwhile).
func (s *Store) SetTrialNotice(ctx context.Context, tenantID uuid.UUID, endsAt time.Time, notice string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE tenant_limits SET trial_notice = $3
		WHERE tenant_id = $1 AND plan = 'trial' AND trial_ends_at = $2`, tenantID, endsAt, notice)
	return tag.RowsAffected() > 0, err
}

// TenantAgents lists every agent in a tenant.
func (s *Store) TenantAgents(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM agent_identities WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// ClaimTrialDomain records that a company (email domain) started its free
// trial; false means it already had one.
func (s *Store) ClaimTrialDomain(ctx context.Context, tx pgx.Tx, domain string, tenantID uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO trial_domains (domain, tenant_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, domain, tenantID)
	return tag.RowsAffected() > 0, err
}

// SnapshotStoredTokens records each dedicated-tenant agent's stored tokens
// as this month's stored_tokens_held, keeping the month's peak. Billing
// charges for storage from it (ADR 0028).
// shortcut: one GROUP BY over all tenant memories per run (hourly); move to
// a running total per agent once that takes more than a few seconds.
func (s *Store) SnapshotStoredTokens(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO usage_monthly (tenant_id, agent_id, month, metric, value)
		SELECT tenant_id, agent_id, date_trunc('month', NOW() AT TIME ZONE 'UTC')::date, 'stored_tokens_held', `+storedTokensExpr+`
		FROM memories WHERE tenant_id <> '00000000-0000-0000-0000-000000000000'
		GROUP BY tenant_id, agent_id
		ON CONFLICT (tenant_id, month, agent_id, metric) DO UPDATE SET value = GREATEST(usage_monthly.value, EXCLUDED.value)`)
	return err
}

// AdminTenant is one organisation as Memro staff see it.
type AdminTenant struct {
	ID           uuid.UUID     `json:"id"`
	Name         string        `json:"name"`
	CreatedAt    time.Time     `json:"created_at"`
	Limits       *TenantLimits `json:"limits"`
	StoredTokens int64         `json:"stored_tokens"`
	QueriesMonth int64         `json:"queries_this_month"`
	EndUsers     int           `json:"end_users"`
	Members      int           `json:"members"`
	Domains      []string      `json:"domains"`
	SelfServe    bool          `json:"self_serve"`
}

// AdminTenants lists organisations, newest first, with their limits and size.
// shortcut: correlated sums over the newest 500, fine for staff use;
// paginate and precompute when there are thousands of organisations.
func (s *Store) AdminTenants(ctx context.Context) ([]AdminTenant, error) {
	return s.adminTenants(ctx, nil)
}

// adminTenants is AdminTenants, or just the one organisation when id is set.
func (s *Store) adminTenants(ctx context.Context, id *uuid.UUID) ([]AdminTenant, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.id, t.name, t.created_at, t.created_by IS NOT NULL,
			l.plan, l.max_stored_tokens_per_user, l.max_stored_tokens_total, l.max_queries_monthly, l.max_members,
			l.allow_images, l.trial_ends_at, l.trial_notice, l.updated_at,
			(SELECT `+storedTokensExpr+` FROM memories m WHERE m.tenant_id = t.id),
			(SELECT COALESCE(SUM(value), 0)::BIGINT FROM usage_monthly u WHERE u.tenant_id = t.id AND u.metric = 'queries'
				AND u.month = date_trunc('month', NOW() AT TIME ZONE 'UTC')::date),
			(SELECT COUNT(*) FROM agent_identities a WHERE a.tenant_id = t.id AND a.external_id IS NOT NULL),
			(SELECT COUNT(*) FROM tenant_members tm WHERE tm.tenant_id = t.id),
			COALESCE((SELECT array_agg(d.domain ORDER BY d.domain) FROM tenant_domains d WHERE d.tenant_id = t.id AND d.verified_at IS NOT NULL), '{}')
		FROM tenants t LEFT JOIN tenant_limits l ON l.tenant_id = t.id
		WHERE t.id <> '00000000-0000-0000-0000-000000000000' AND ($1::uuid IS NULL OR t.id = $1)
		ORDER BY t.created_at DESC LIMIT 500`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminTenant{}
	for rows.Next() {
		var t AdminTenant
		var plan *string
		var images *bool
		var updated *time.Time
		var l TenantLimits
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.SelfServe, &plan, &l.MaxStoredTokensPerUser, &l.MaxStoredTokensTotal,
			&l.MaxQueriesMonthly, &l.MaxMembers, &images, &l.TrialEndsAt, &l.TrialNotice, &updated,
			&t.StoredTokens, &t.QueriesMonth, &t.EndUsers, &t.Members, &t.Domains); err != nil {
			return nil, err
		}
		if plan != nil {
			l.Plan, l.AllowImages, l.UpdatedAt = *plan, *images, *updated
			t.Limits = &l
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
