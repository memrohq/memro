package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// --- Usage per end user (ADR 0026) ------------------------------------------------

// RecordMemberUsage adds value to this month's running total of metric for
// one agent (for a tenant key, one end user).
func (s *Store) RecordMemberUsage(ctx context.Context, tenantID uuid.UUID, agentID, metric string, value int64) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO usage_monthly (tenant_id, agent_id, month, metric, value)
		VALUES ($1, $2, date_trunc('month', NOW() AT TIME ZONE 'UTC')::date, $3, $4)
		ON CONFLICT (tenant_id, month, agent_id, metric) DO UPDATE SET value = usage_monthly.value + EXCLUDED.value`,
		tenantID, agentID, metric, value)
	return err
}

// MemberUsage is one agent's usage in a month. ExternalID is the customer's
// user ID; nil for an agent that is not an end user, or was deleted since.
type MemberUsage struct {
	AgentID    string           `json:"agent_id"`
	ExternalID *string          `json:"user"`
	Metrics    map[string]int64 `json:"metrics"`
}

// TenantMemberUsage returns every agent's usage in a tenant for the month
// starting at month (first day, UTC).
func (s *Store) TenantMemberUsage(ctx context.Context, tenantID uuid.UUID, month time.Time) ([]MemberUsage, error) {
	rows, err := s.Pool.Query(ctx, `SELECT u.agent_id, i.external_id, u.metric, u.value
		FROM usage_monthly u LEFT JOIN agent_identities i ON i.id = u.agent_id
		WHERE u.tenant_id = $1 AND u.month = $2 ORDER BY u.agent_id`, tenantID, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemberUsage{}
	for rows.Next() {
		var agentID, metric string
		var ext *string
		var v int64
		if err := rows.Scan(&agentID, &ext, &metric, &v); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].AgentID != agentID {
			out = append(out, MemberUsage{AgentID: agentID, ExternalID: ext, Metrics: map[string]int64{}})
		}
		out[len(out)-1].Metrics[metric] = v
	}
	return out, rows.Err()
}

// --- Organisation members (ADR 0026) ----------------------------------------------

// Tenant roles, most powerful first.
const (
	RoleOwner   = "owner"
	RoleAdmin   = "admin"
	RoleBilling = "billing"
	RoleViewer  = "viewer"
)

// Roles lists the valid tenant roles.
var Roles = []string{RoleOwner, RoleAdmin, RoleBilling, RoleViewer}

// ErrLastOwner means the change would leave the tenant without an owner.
var ErrLastOwner = errors.New("a tenant must keep at least one owner")

type Membership struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
}

// Memberships lists the tenants a user belongs to.
func (s *Store) Memberships(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.id, t.name, m.role FROM tenant_members m
		JOIN tenants t ON t.id = m.tenant_id WHERE m.user_id = $1 ORDER BY t.name`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Membership, error) {
		var m Membership
		return m, row.Scan(&m.TenantID, &m.Name, &m.Role)
	})
}

// TenantRole returns a user's role in a tenant, or "" if they are not a member.
func (s *Store) TenantRole(ctx context.Context, tenantID, userID uuid.UUID) (string, error) {
	var role string
	err := s.Pool.QueryRow(ctx, `SELECT role FROM tenant_members WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

type Member struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) Members(ctx context.Context, tenantID uuid.UUID) ([]Member, error) {
	rows, err := s.Pool.Query(ctx, `SELECT u.id, u.email, m.role, m.created_at FROM tenant_members m
		JOIN users u ON u.id = m.user_id WHERE m.tenant_id = $1 ORDER BY m.created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		return m, row.Scan(&m.UserID, &m.Email, &m.Role, &m.CreatedAt)
	})
}

// AddMember makes a user a member; an existing member keeps their role.
func (s *Store) AddMember(ctx context.Context, tenantID, userID uuid.UUID, role string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_members (tenant_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO NOTHING`, tenantID, userID, role)
	return err
}

// SetMemberRole changes a member's role; role "" removes them. It reports
// whether they were a member, and refuses to remove or demote the last owner.
func (s *Store) SetMemberRole(ctx context.Context, tenantID, userID uuid.UUID, role string) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// Locking the owners serialises concurrent changes, so two owners cannot
	// demote each other at once.
	rows, err := tx.Query(ctx, `SELECT user_id FROM tenant_members WHERE tenant_id = $1 AND role = 'owner' FOR UPDATE`, tenantID)
	if err != nil {
		return false, err
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return false, err
	}
	if role != RoleOwner && len(owners) == 1 && owners[0] == userID {
		return false, ErrLastOwner
	}
	q, args := `UPDATE tenant_members SET role = $3 WHERE tenant_id = $1 AND user_id = $2`, []any{tenantID, userID, role}
	if role == "" {
		q, args = `DELETE FROM tenant_members WHERE tenant_id = $1 AND user_id = $2`, args[:2]
	}
	tag, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

// --- Invitations (ADR 0026) -------------------------------------------------------

// ErrInviteInvalid means the link is unknown, used or expired; ErrInviteEmail
// means it was sent to a different address than the one signed in.
var (
	ErrInviteInvalid = errors.New("invite is invalid, expired or already used")
	ErrInviteEmail   = errors.New("invite was sent to a different email address")
)

type Invite struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Store) CreateInvite(ctx context.Context, inv Invite, tokenHash string, invitedBy *uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_invites (id, tenant_id, email, role, token_hash, invited_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		inv.ID, inv.TenantID, inv.Email, inv.Role, tokenHash, invitedBy, inv.CreatedAt, inv.ExpiresAt)
	return err
}

// Invites lists a tenant's pending, unexpired invitations.
func (s *Store) Invites(ctx context.Context, tenantID uuid.UUID) ([]Invite, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, tenant_id, email, role, created_at, expires_at FROM tenant_invites
		WHERE tenant_id = $1 AND accepted_at IS NULL AND expires_at > NOW() ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Invite])
}

func (s *Store) DeleteInvite(ctx context.Context, tenantID, id uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM tenant_invites WHERE id = $1 AND tenant_id = $2 AND accepted_at IS NULL`, id, tenantID)
	return tag.RowsAffected() > 0, err
}

// AcceptInvite adds the signed-in user to the invite's tenant, once, if the
// invite was sent to their email. An existing member keeps their role.
func (s *Store) AcceptInvite(ctx context.Context, tokenHash string, userID uuid.UUID, email string) (Membership, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Membership{}, err
	}
	defer tx.Rollback(ctx)
	var id, tenantID uuid.UUID
	var to, role string
	err = tx.QueryRow(ctx, `SELECT id, tenant_id, email, role FROM tenant_invites
		WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > NOW() FOR UPDATE`, tokenHash).
		Scan(&id, &tenantID, &to, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, ErrInviteInvalid
	}
	if err != nil {
		return Membership{}, err
	}
	if !strings.EqualFold(to, email) {
		return Membership{}, ErrInviteEmail
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tenant_members (tenant_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO NOTHING`, tenantID, userID, role); err != nil {
		return Membership{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tenant_invites SET accepted_at = NOW() WHERE id = $1`, id); err != nil {
		return Membership{}, err
	}
	var m Membership
	if err := tx.QueryRow(ctx, `SELECT t.id, t.name, m.role FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id
		WHERE m.tenant_id = $1 AND m.user_id = $2`, tenantID, userID).Scan(&m.TenantID, &m.Name, &m.Role); err != nil {
		return Membership{}, err
	}
	return m, tx.Commit(ctx)
}

// UserEmail returns a user's email address ("" if there is no such user).
func (s *Store) UserEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	var email string
	err := s.Pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return email, err
}
