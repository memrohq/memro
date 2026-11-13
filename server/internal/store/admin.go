package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The staff console (ADR 0029): Memro-wide figures and every account.

// AdminOverview is Memro at a glance.
type AdminOverview struct {
	Users            int   `json:"users"`
	Staff            int   `json:"staff"`
	Organisations    int   `json:"organisations"`
	OrgsTrial        int   `json:"orgs_on_trial"`
	OrgsTrialEnded   int   `json:"orgs_trial_ended"`
	OrgsTrialEnding  int   `json:"orgs_trial_ending_7d"`
	OrgsContract     int   `json:"orgs_on_contract"` // enterprise or custom
	Agents           int   `json:"agents"`
	EndUsers         int   `json:"end_users"`
	Memories         int64 `json:"memories"`
	StoredTokens     int64 `json:"stored_tokens"`
	QueriesThisMonth int64 `json:"queries_this_month"`
	SignInsLast30d   int   `json:"active_users_30d"`
}

// shortcut: whole-table counts per load, fine for a staff page; precompute
// once the memories table is large enough to make it slow.
func (s *Store) AdminOverview(ctx context.Context) (AdminOverview, error) {
	var o AdminOverview
	err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM platform_admins),
		(SELECT COUNT(*) FROM tenants WHERE id <> '00000000-0000-0000-0000-000000000000'),
		(SELECT COUNT(*) FROM tenant_limits WHERE plan = 'trial' AND (trial_ends_at IS NULL OR trial_ends_at > NOW())),
		(SELECT COUNT(*) FROM tenant_limits WHERE plan = 'trial' AND trial_ends_at <= NOW()),
		(SELECT COUNT(*) FROM tenant_limits WHERE plan = 'trial' AND trial_ends_at > NOW() AND trial_ends_at <= NOW() + interval '7 days'),
		(SELECT COUNT(*) FROM tenant_limits WHERE plan IN ('enterprise', 'custom')),
		(SELECT COUNT(*) FROM agent_identities),
		(SELECT COUNT(*) FROM agent_identities WHERE external_id IS NOT NULL),
		(SELECT COUNT(*) FROM memories),
		(SELECT `+storedTokensExpr+` FROM memories),
		(SELECT COALESCE(SUM(value), 0)::BIGINT FROM usage_monthly WHERE metric = 'queries' AND month = date_trunc('month', NOW() AT TIME ZONE 'UTC')::date),
		(SELECT COUNT(DISTINCT user_id) FROM sessions WHERE created_at > NOW() - interval '30 days')`).
		Scan(&o.Users, &o.Staff, &o.Organisations, &o.OrgsTrial, &o.OrgsTrialEnded, &o.OrgsTrialEnding, &o.OrgsContract,
			&o.Agents, &o.EndUsers, &o.Memories, &o.StoredTokens, &o.QueriesThisMonth, &o.SignInsLast30d)
	return o, err
}

// AdminUser is one person's account as Memro staff see it.
type AdminUser struct {
	ID           uuid.UUID    `json:"id"`
	Email        string       `json:"email"`
	CreatedAt    time.Time    `json:"created_at"`
	LastSignIn   *time.Time   `json:"last_sign_in"`
	Agents       int          `json:"agents"`
	Memories     int64        `json:"memories"`
	StoredTokens int64        `json:"stored_tokens"`
	Staff        bool         `json:"staff"`
	Orgs         []Membership `json:"orgs"`
}

// AdminUsers lists people, newest first, or those whose email contains q.
// Agents and storage are their own (personal) agents, not organisations'.
// shortcut: correlated counts over at most 500 rows, fine for staff use.
func (s *Store) AdminUsers(ctx context.Context, q string) ([]AdminUser, error) {
	rows, err := s.Pool.Query(ctx, `SELECT u.id, u.email, u.created_at,
			(SELECT MAX(created_at) FROM sessions se WHERE se.user_id = u.id),
			(SELECT COUNT(*) FROM agent_identities a WHERE a.user_id = u.id),
			(SELECT COUNT(*) FROM memories m JOIN agent_identities a ON a.id = m.agent_id WHERE a.user_id = u.id),
			(SELECT `+storedTokensExpr+` FROM memories m JOIN agent_identities a ON a.id = m.agent_id WHERE a.user_id = u.id),
			EXISTS (SELECT 1 FROM platform_admins p WHERE p.user_id = u.id)
		FROM users u WHERE $1 = '' OR u.email ILIKE '%' || $1 || '%'
		ORDER BY u.created_at DESC LIMIT 500`, q)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminUser, error) {
		var u AdminUser
		return u, row.Scan(&u.ID, &u.Email, &u.CreatedAt, &u.LastSignIn, &u.Agents, &u.Memories, &u.StoredTokens, &u.Staff)
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Orgs, err = s.Memberships(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetPlatformAdmin grants or revokes staff rights; it reports whether
// anything changed. It refuses to remove the last staff member.
func (s *Store) SetPlatformAdmin(ctx context.Context, userID uuid.UUID, admin bool, note string) (bool, error) {
	if admin {
		tag, err := s.Pool.Exec(ctx, `INSERT INTO platform_admins (user_id, note) SELECT id, $2 FROM users WHERE id = $1
			ON CONFLICT DO NOTHING`, userID, note)
		return tag.RowsAffected() > 0, err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM platform_admins WHERE user_id = $1
		AND (SELECT COUNT(*) FROM platform_admins) > 1`, userID)
	return tag.RowsAffected() > 0, err
}

// --- Account actions (ADR 0029) -----------------------------------------------------

// Suspended reports whether a person or organisation is suspended, with the
// reason. Either ID may be nil.
func (s *Store) Suspended(ctx context.Context, userID *uuid.UUID, tenantID uuid.UUID) (bool, string, error) {
	var reason *string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(
			(SELECT COALESCE(suspended_reason, '') FROM users WHERE id = $1 AND suspended_at IS NOT NULL),
			(SELECT COALESCE(suspended_reason, '') FROM tenants WHERE id = $2 AND suspended_at IS NOT NULL))`,
		userID, tenantID).Scan(&reason)
	if err != nil || reason == nil {
		return false, "", err
	}
	return true, *reason, nil
}

// SetSuspended suspends (reason set) or unsuspends a person ("user") or an
// organisation ("tenant"); it reports whether the row exists.
func (s *Store) SetSuspended(ctx context.Context, kind string, id uuid.UUID, suspend bool, reason string) (bool, error) {
	table := map[string]string{"user": "users", "tenant": "tenants"}[kind]
	q := `UPDATE ` + table + ` SET suspended_at = NULL, suspended_reason = NULL WHERE id = $1`
	args := []any{id}
	if suspend {
		q, args = `UPDATE `+table+` SET suspended_at = NOW(), suspended_reason = $2 WHERE id = $1`, []any{id, reason}
	}
	tag, err := s.Pool.Exec(ctx, q, args...)
	return tag.RowsAffected() > 0, err
}

// DeleteSessions signs a person out everywhere; it returns how many sessions
// ended.
func (s *Store) DeleteSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return tag.RowsAffected(), err
}

// RevokeUserKeys revokes every API key of a person's own agents.
func (s *Store) RevokeUserKeys(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE api_keys SET revoked_at = NOW() WHERE revoked_at IS NULL
		AND agent_id IN (SELECT id FROM agent_identities WHERE user_id = $1)`, userID)
	return tag.RowsAffected(), err
}

// RevokeTenantKeys revokes every key in an organisation: server keys and its
// agents' keys.
func (s *Store) RevokeTenantKeys(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE api_keys SET revoked_at = NOW() WHERE revoked_at IS NULL AND tenant_id = $1`, tenantID)
	return tag.RowsAffected(), err
}

// SoleOwnerOrgs names the organisations a person is the only owner of: they
// would be left without an owner if the person were deleted.
func (s *Store) SoleOwnerOrgs(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.name FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id
		WHERE m.user_id = $1 AND m.role = 'owner'
		  AND NOT EXISTS (SELECT 1 FROM tenant_members o WHERE o.tenant_id = m.tenant_id AND o.role = 'owner' AND o.user_id <> $1)
		ORDER BY t.name`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// UserAgents lists a person's own agents.
func (s *Store) UserAgents(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM agent_identities WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// DeleteUser deletes a person; their sessions and memberships go with them.
// Delete their agents' data first (the API does, with their files).
func (s *Store) DeleteUser(ctx context.Context, userID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return tag.RowsAffected() > 0, err
}

// DeleteTenant deletes an organisation with its members, invitations, keys,
// domains, limits and its own audit log. Delete its agents' data first.
func (s *Store) DeleteTenant(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM api_keys WHERE tenant_id = $1`, tenantID); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1 AND id <> '00000000-0000-0000-0000-000000000000'`, tenantID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

// --- Detail pages (ADR 0029) --------------------------------------------------------

type AdminAgent struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Memories     int64     `json:"memories"`
	StoredTokens int64     `json:"stored_tokens"`
}

type AdminKey struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	AgentID    *string    `json:"agent_id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

type AdminAuditEntry struct {
	CreatedAt time.Time `json:"created_at"`
	Action    string    `json:"action"`
	Resource  *string   `json:"resource"`
	Actor     *string   `json:"actor"` // the person's email, when a person did it
	AgentID   string    `json:"agent_id"`
	Org       *string   `json:"org"`
	IP        *string   `json:"ip"`
}

// MonthUsage is one month of a metric.
type MonthUsage struct {
	Month  time.Time `json:"month"`
	Metric string    `json:"metric"`
	Value  int64     `json:"value"`
}

const auditCols = `l.created_at, l.action, l.resource_id, u.email, l.agent_id, t.name, l.ip_addr
	FROM audit_log l LEFT JOIN agent_identities a ON a.id = l.agent_id LEFT JOIN users u ON u.id = a.user_id
	LEFT JOIN tenants t ON t.id = l.tenant_id AND t.id <> '00000000-0000-0000-0000-000000000000'`

func collectAudit(rows pgx.Rows, err error) ([]AdminAuditEntry, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[AdminAuditEntry])
}

func collectUsage(rows pgx.Rows, err error) ([]MonthUsage, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MonthUsage])
}

// AdminUserDetail is everything about one person.
type AdminUserDetail struct {
	AdminUser
	Suspended       *time.Time        `json:"suspended_at"`
	SuspendedReason *string           `json:"suspended_reason"`
	AgentList       []AdminAgent      `json:"agent_list"`
	Keys            []AdminKey        `json:"keys"`
	SignIns         []time.Time       `json:"sign_ins"`
	Usage           []MonthUsage      `json:"usage"`
	Audit           []AdminAuditEntry `json:"audit"`
}

func (s *Store) AdminUserDetail(ctx context.Context, id uuid.UUID) (*AdminUserDetail, error) {
	var d AdminUserDetail
	err := s.Pool.QueryRow(ctx, `SELECT u.id, u.email, u.created_at, u.suspended_at, u.suspended_reason,
			(SELECT MAX(created_at) FROM sessions se WHERE se.user_id = u.id),
			EXISTS (SELECT 1 FROM platform_admins p WHERE p.user_id = u.id)
		FROM users u WHERE u.id = $1`, id).
		Scan(&d.ID, &d.Email, &d.CreatedAt, &d.Suspended, &d.SuspendedReason, &d.LastSignIn, &d.Staff)
	if p, err := nilIfNoRows(&d, err); p == nil || err != nil {
		return nil, err
	}
	if d.Orgs, err = s.Memberships(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT a.id, a.created_at, COUNT(m.id), `+storedTokensExpr+`
		FROM agent_identities a LEFT JOIN memories m ON m.agent_id = a.id
		WHERE a.user_id = $1 GROUP BY a.id, a.created_at ORDER BY a.created_at`, id)
	if err != nil {
		return nil, err
	}
	if d.AgentList, err = pgx.CollectRows(rows, pgx.RowToStructByPos[AdminAgent]); err != nil {
		return nil, err
	}
	d.Agents = len(d.AgentList)
	for _, a := range d.AgentList {
		d.Memories += a.Memories
		d.StoredTokens += a.StoredTokens
	}
	rows, err = s.Pool.Query(ctx, `SELECT k.id, k.name, k.prefix, k.agent_id, k.created_at, k.last_used_at, k.revoked_at
		FROM api_keys k JOIN agent_identities a ON a.id = k.agent_id WHERE a.user_id = $1 ORDER BY k.created_at DESC`, id)
	if err != nil {
		return nil, err
	}
	if d.Keys, err = pgx.CollectRows(rows, pgx.RowToStructByPos[AdminKey]); err != nil {
		return nil, err
	}
	rows, err = s.Pool.Query(ctx, `SELECT created_at FROM sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 10`, id)
	if err != nil {
		return nil, err
	}
	if d.SignIns, err = pgx.CollectRows(rows, pgx.RowTo[time.Time]); err != nil {
		return nil, err
	}
	if d.Usage, err = collectUsage(s.Pool.Query(ctx, `SELECT u.month::timestamptz, u.metric, SUM(u.value)::BIGINT FROM usage_monthly u
		JOIN agent_identities a ON a.id = u.agent_id WHERE a.user_id = $1 AND u.month > NOW() - interval '6 months'
		GROUP BY u.month, u.metric ORDER BY u.month, u.metric`, id)); err != nil {
		return nil, err
	}
	d.Audit, err = collectAudit(s.Pool.Query(ctx, `SELECT `+auditCols+` WHERE a.user_id = $1 ORDER BY l.created_at DESC LIMIT 50`, id))
	return &d, err
}

// AdminTenantDetail is everything about one organisation.
type AdminTenantDetail struct {
	AdminTenant
	Suspended       *time.Time        `json:"suspended_at"`
	SuspendedReason *string           `json:"suspended_reason"`
	Members         []Member          `json:"member_list"`
	Invites         []Invite          `json:"invites"`
	DomainClaims    []TenantDomain    `json:"domain_claims"`
	Keys            []AdminKey        `json:"keys"`
	Usage           []MonthUsage      `json:"usage"`
	Audit           []AdminAuditEntry `json:"audit"`
}

func (s *Store) AdminTenantDetail(ctx context.Context, id uuid.UUID) (*AdminTenantDetail, error) {
	all, err := s.adminTenants(ctx, &id)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	d := AdminTenantDetail{AdminTenant: all[0]}
	if err := s.Pool.QueryRow(ctx, `SELECT suspended_at, suspended_reason FROM tenants WHERE id = $1`, id).
		Scan(&d.Suspended, &d.SuspendedReason); err != nil {
		return nil, err
	}
	if d.Members, err = s.Members(ctx, id); err != nil {
		return nil, err
	}
	if d.Invites, err = s.Invites(ctx, id); err != nil {
		return nil, err
	}
	if d.DomainClaims, err = s.TenantDomains(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id, name, prefix, agent_id, created_at, last_used_at, revoked_at
		FROM api_keys WHERE tenant_id = $1 ORDER BY created_at DESC`, id)
	if err != nil {
		return nil, err
	}
	if d.Keys, err = pgx.CollectRows(rows, pgx.RowToStructByPos[AdminKey]); err != nil {
		return nil, err
	}
	if d.Usage, err = collectUsage(s.Pool.Query(ctx, `SELECT month::timestamptz, metric, SUM(value)::BIGINT FROM usage_monthly
		WHERE tenant_id = $1 AND month > NOW() - interval '6 months' GROUP BY month, metric ORDER BY month, metric`, id)); err != nil {
		return nil, err
	}
	d.Audit, err = collectAudit(s.Pool.Query(ctx, `SELECT `+auditCols+` WHERE l.tenant_id = $1 ORDER BY l.created_at DESC LIMIT 50`, id))
	return &d, err
}

// --- Activity (ADR 0029) ------------------------------------------------------------

// DayCount is a count on one day (UTC).
type DayCount struct {
	Day   time.Time `json:"day"`
	Count int64     `json:"count"`
}

type AdminActivity struct {
	SignUps []DayCount        `json:"sign_ups"`      // people, last 30 days
	NewOrgs []DayCount        `json:"organisations"` // last 30 days
	Queries []MonthUsage      `json:"queries"`       // per month, last 6
	Feed    []AdminAuditEntry `json:"feed"`          // newest first
}

// AdminActivity is what has been happening across Memro; action filters the
// feed by prefix (e.g. "staff." or "tenant_limits.").
func (s *Store) AdminActivity(ctx context.Context, action string) (AdminActivity, error) {
	var a AdminActivity
	days := func(q string) ([]DayCount, error) {
		rows, err := s.Pool.Query(ctx, `SELECT d::timestamptz, (SELECT COUNT(*) FROM `+q+`)
			FROM generate_series(date_trunc('day', NOW() AT TIME ZONE 'UTC') - interval '29 days', date_trunc('day', NOW() AT TIME ZONE 'UTC'), interval '1 day') d`)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowToStructByPos[DayCount])
	}
	var err error
	if a.SignUps, err = days(`users x WHERE (x.created_at AT TIME ZONE 'UTC') >= d AND (x.created_at AT TIME ZONE 'UTC') < d + interval '1 day'`); err != nil {
		return a, err
	}
	if a.NewOrgs, err = days(`tenants x WHERE x.id <> '00000000-0000-0000-0000-000000000000' AND (x.created_at AT TIME ZONE 'UTC') >= d AND (x.created_at AT TIME ZONE 'UTC') < d + interval '1 day'`); err != nil {
		return a, err
	}
	if a.Queries, err = collectUsage(s.Pool.Query(ctx, `SELECT month::timestamptz, 'queries', SUM(value)::BIGINT FROM usage_monthly
		WHERE metric = 'queries' AND month > NOW() - interval '6 months' GROUP BY month ORDER BY month`)); err != nil {
		return a, err
	}
	a.Feed, err = collectAudit(s.Pool.Query(ctx, `SELECT `+auditCols+` WHERE $1 = '' OR l.action LIKE $1 || '%'
		ORDER BY l.created_at DESC LIMIT 100`, action))
	return a, err
}
