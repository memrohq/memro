package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- Self-service organisations, verified domains, join requests (ADR 0027) ------

// ErrDomainTaken means another organisation already holds the domain verified.
var ErrDomainTaken = errors.New("domain is verified by another organisation")

// Join policies: who may become a member without an invitation.
const (
	JoinByInvite  = "invite"
	JoinByRequest = "request"
)

// CreateOrg makes a dedicated tenant with the user as its owner, on trial
// limits (ADR 0028). A company gets one free trial: when emailDomain already
// had one, the trial starts ended (read-only) and trialStarted is false.
func (s *Store) CreateOrg(ctx context.Context, name string, userID uuid.UUID, emailDomain string, trial TenantLimits) (m Membership, trialStarted bool, err error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Membership{}, false, err
	}
	defer tx.Rollback(ctx)
	m = Membership{TenantID: uuid.New(), Name: name, Role: RoleOwner}
	if _, err := tx.Exec(ctx, `INSERT INTO tenants (id, name, created_by) VALUES ($1, $2, $3)`, m.TenantID, name, userID); err != nil {
		return Membership{}, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tenant_members (tenant_id, user_id, role) VALUES ($1, $2, 'owner')`, m.TenantID, userID); err != nil {
		return Membership{}, false, err
	}
	if trialStarted, err = s.ClaimTrialDomain(ctx, tx, emailDomain, m.TenantID); err != nil {
		return Membership{}, false, err
	}
	if !trialStarted {
		now := time.Now().UTC()
		trial.TrialEndsAt = &now
	}
	if _, err := tx.Exec(ctx, insertLimits, limitArgs(m.TenantID, trial, nil)...); err != nil {
		return Membership{}, false, err
	}
	return m, trialStarted, tx.Commit(ctx)
}

// OrgsCreatedBy counts the self-service organisations a user has created.
func (s *Store) OrgsCreatedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM tenants WHERE created_by = $1`, userID).Scan(&n)
	return n, err
}

type TenantDomain struct {
	Domain     string     `json:"domain"`
	Token      *string    `json:"-"`
	VerifiedAt *time.Time `json:"verified_at"`
	CheckedAt  *time.Time `json:"checked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ClaimDomain records an unverified claim; claiming the same domain again
// keeps the existing token. It returns the claim.
func (s *Store) ClaimDomain(ctx context.Context, tenantID uuid.UUID, domain, token string) (TenantDomain, error) {
	if _, err := s.Pool.Exec(ctx, `INSERT INTO tenant_domains (tenant_id, domain, token) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, domain) DO NOTHING`, tenantID, domain, token); err != nil {
		return TenantDomain{}, err
	}
	d, err := s.TenantDomainClaim(ctx, tenantID, domain)
	if d == nil && err == nil {
		err = pgx.ErrNoRows
	}
	if err != nil {
		return TenantDomain{}, err
	}
	return *d, nil
}

// AddVerifiedDomain records a domain Memro staff checked by hand.
func (s *Store) AddVerifiedDomain(ctx context.Context, tenantID uuid.UUID, domain string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_domains (tenant_id, domain, verified_at, checked_at) VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (tenant_id, domain) DO UPDATE SET verified_at = NOW(), token = NULL`, tenantID, domain)
	return domainErr(err)
}

// TenantDomainClaim returns one claim, or nil.
func (s *Store) TenantDomainClaim(ctx context.Context, tenantID uuid.UUID, domain string) (*TenantDomain, error) {
	var d TenantDomain
	err := s.Pool.QueryRow(ctx, `SELECT domain, token, verified_at, checked_at, created_at FROM tenant_domains
		WHERE tenant_id = $1 AND domain = $2`, tenantID, domain).Scan(&d.Domain, &d.Token, &d.VerifiedAt, &d.CheckedAt, &d.CreatedAt)
	return nilIfNoRows(&d, err)
}

func (s *Store) TenantDomains(ctx context.Context, tenantID uuid.UUID) ([]TenantDomain, error) {
	rows, err := s.Pool.Query(ctx, `SELECT domain, token, verified_at, checked_at, created_at FROM tenant_domains
		WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TenantDomain, error) {
		var d TenantDomain
		return d, row.Scan(&d.Domain, &d.Token, &d.VerifiedAt, &d.CheckedAt, &d.CreatedAt)
	})
}

func (s *Store) CountTenantDomains(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM tenant_domains WHERE tenant_id = $1`, tenantID).Scan(&n)
	return n, err
}

func (s *Store) DeleteTenantDomain(ctx context.Context, tenantID uuid.UUID, domain string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM tenant_domains WHERE tenant_id = $1 AND domain = $2`, tenantID, domain)
	return tag.RowsAffected() > 0, err
}

// SetDomainVerified records the outcome of a DNS check: verified keeps or
// sets verified_at, otherwise it is cleared. ErrDomainTaken if another
// organisation holds the domain.
func (s *Store) SetDomainVerified(ctx context.Context, tenantID uuid.UUID, domain string, verified bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE tenant_domains SET checked_at = NOW(),
		verified_at = CASE WHEN $3 THEN COALESCE(verified_at, NOW()) ELSE NULL END
		WHERE tenant_id = $1 AND domain = $2`, tenantID, domain, verified)
	return domainErr(err)
}

func domainErr(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrDomainTaken
	}
	return err
}

// DomainCheck is a verified, DNS-proven domain due for re-checking.
type DomainCheck struct {
	TenantID uuid.UUID
	Domain   string
	Token    string
}

// DomainsToRecheck lists DNS-verified domains last checked before cutoff.
func (s *Store) DomainsToRecheck(ctx context.Context, cutoff time.Time, limit int) ([]DomainCheck, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id, domain, token FROM tenant_domains
		WHERE verified_at IS NOT NULL AND token IS NOT NULL AND (checked_at IS NULL OR checked_at < $1)
		ORDER BY checked_at NULLS FIRST LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[DomainCheck])
}

// HasVerifiedDomain reports whether the tenant holds at least one verified domain.
func (s *Store) HasVerifiedDomain(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_domains WHERE tenant_id = $1 AND verified_at IS NOT NULL)`,
		tenantID).Scan(&ok)
	return ok, err
}

func (s *Store) JoinPolicy(ctx context.Context, tenantID uuid.UUID) (string, error) {
	var p string
	err := s.Pool.QueryRow(ctx, `SELECT join_policy FROM tenants WHERE id = $1`, tenantID).Scan(&p)
	return p, err
}

func (s *Store) SetJoinPolicy(ctx context.Context, tenantID uuid.UUID, policy string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE tenants SET join_policy = $2, updated_at = NOW() WHERE id = $1`, tenantID, policy)
	return err
}

// DiscoverableOrg is an organisation a person may ask to join.
type DiscoverableOrg struct {
	TenantID  uuid.UUID `json:"tenant_id"`
	Name      string    `json:"name"`
	Requested bool      `json:"requested"`
}

// DiscoverableOrgs lists organisations holding domain verified that take
// join requests and that the user is not a member of.
func (s *Store) DiscoverableOrgs(ctx context.Context, domain string, userID uuid.UUID) ([]DiscoverableOrg, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.id, t.name,
			EXISTS (SELECT 1 FROM tenant_join_requests r WHERE r.tenant_id = t.id AND r.user_id = $2)
		FROM tenant_domains d JOIN tenants t ON t.id = d.tenant_id
		WHERE d.domain = $1 AND d.verified_at IS NOT NULL AND t.join_policy = 'request'
		  AND NOT EXISTS (SELECT 1 FROM tenant_members m WHERE m.tenant_id = t.id AND m.user_id = $2)
		ORDER BY t.name`, domain, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[DiscoverableOrg])
}

// CreateJoinRequest records a request; asking again is a no-op.
func (s *Store) CreateJoinRequest(ctx context.Context, tenantID, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_join_requests (id, tenant_id, user_id) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO NOTHING`, uuid.New(), tenantID, userID)
	return err
}

type JoinRequest struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) JoinRequests(ctx context.Context, tenantID uuid.UUID) ([]JoinRequest, error) {
	rows, err := s.Pool.Query(ctx, `SELECT r.id, r.user_id, u.email, r.created_at FROM tenant_join_requests r
		JOIN users u ON u.id = r.user_id WHERE r.tenant_id = $1 ORDER BY r.created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[JoinRequest])
}

// DecideJoinRequest removes a pending request and, when role is set, makes
// the requester a member with it. It returns the requester, or nil if there
// was no such request.
func (s *Store) DecideJoinRequest(ctx context.Context, tenantID, id uuid.UUID, role string) (*uuid.UUID, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var userID uuid.UUID
	err = tx.QueryRow(ctx, `DELETE FROM tenant_join_requests WHERE id = $1 AND tenant_id = $2 RETURNING user_id`, id, tenantID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if role != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO tenant_members (tenant_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, user_id) DO NOTHING`, tenantID, userID, role); err != nil {
			return nil, err
		}
	}
	return &userID, tx.Commit(ctx)
}
