package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const connectionColumns = `c.id, c.tenant_id, c.agent_id, c.provider, c.account_id, c.account_name, c.external_user_id,
	c.external_user_name, c.credentials, c.status, c.status_detail, c.created_at, c.last_synced_at`

func scanConnection(row pgx.Row) (*Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.TenantID, &c.AgentID, &c.Provider, &c.AccountID, &c.AccountName, &c.ExternalUserID,
		&c.ExternalUserName, &c.sealed, &c.Status, &c.StatusDetail, &c.CreatedAt, &c.LastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (m *Manager) connection(ctx context.Context, id uuid.UUID) (*Connection, error) {
	return scanConnection(m.deps.Pool.QueryRow(ctx, `SELECT `+connectionColumns+` FROM connector_connections c WHERE c.id = $1`, id))
}

// owned loads a connection that belongs to agentID.
func (m *Manager) owned(ctx context.Context, agentID string, id uuid.UUID) (*Connection, error) {
	return scanConnection(m.deps.Pool.QueryRow(ctx, `SELECT `+connectionColumns+` FROM connector_connections c
		WHERE c.id = $1 AND c.agent_id = $2`, id, agentID))
}

// connectionsForAccount returns the active connections of a Slack team or
// GitHub installation, e.g. to fan out a webhook.
func (m *Manager) connectionsForAccount(ctx context.Context, provider, accountID string, onlyActive bool) ([]*Connection, error) {
	q := `SELECT ` + connectionColumns + ` FROM connector_connections c WHERE c.provider = $1 AND c.account_id = $2`
	if onlyActive {
		q += ` AND c.status = 'active'`
	}
	rows, err := m.deps.Pool.Query(ctx, q, provider, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// connectionsWithSource returns the active connections of a provider that
// sync sourceID (a project, team or repository), optionally only those of
// one account. Webhooks use it to reach exactly the connections whose user
// chose that source.
func (m *Manager) connectionsWithSource(ctx context.Context, provider, accountID, sourceID string) ([]*Connection, error) {
	rows, err := m.deps.Pool.Query(ctx, `SELECT `+connectionColumns+` FROM connector_connections c
		WHERE c.provider = $1 AND c.status = 'active' AND ($2 = '' OR c.account_id = $2)
			AND EXISTS (SELECT 1 FROM connector_sources s WHERE s.connection_id = c.id AND s.external_id = $3 AND s.enabled)`,
		provider, accountID, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// saveConnection creates the connection or, when the agent reconnects the
// same account, replaces its credentials and reactivates it.
func (m *Manager) saveConnection(ctx context.Context, agentID string, tenantID uuid.UUID, provider string, a *authorized) (uuid.UUID, error) {
	id := uuid.New()
	var existing uuid.UUID
	err := m.deps.Pool.QueryRow(ctx, `SELECT id FROM connector_connections WHERE agent_id = $1 AND provider = $2 AND account_id = $3`,
		agentID, provider, a.accountID).Scan(&existing)
	switch {
	case err == nil:
		id = existing
	case !errors.Is(err, pgx.ErrNoRows):
		return uuid.Nil, err
	}
	sealed, err := m.sealer.Seal(a.creds, id.String())
	if err != nil {
		return uuid.Nil, err
	}
	err = m.deps.Pool.QueryRow(ctx, `INSERT INTO connector_connections (id, tenant_id, agent_id, provider, account_id,
			account_name, external_user_id, external_user_name, credentials)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (agent_id, provider, account_id) DO UPDATE SET account_name = EXCLUDED.account_name,
			external_user_id = EXCLUDED.external_user_id, external_user_name = EXCLUDED.external_user_name,
			credentials = EXCLUDED.credentials, status = 'active', status_detail = '', updated_at = NOW()
		RETURNING id`, id, tenantID, agentID, provider, a.accountID, a.accountName, a.userID, a.userName, sealed).Scan(&id)
	return id, err
}

func (m *Manager) setStatus(ctx context.Context, id uuid.UUID, status, detail string) error {
	_, err := m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET status = $2, status_detail = $3, updated_at = NOW()
		WHERE id = $1`, id, status, truncate(detail, 500))
	return err
}

// --- Sources --------------------------------------------------------------------

func (m *Manager) sources(ctx context.Context, connID uuid.UUID) ([]Source, error) {
	rows, err := m.deps.Pool.Query(ctx, `SELECT external_id, name, is_private, enabled, cursor, last_synced_at
		FROM connector_sources WHERE connection_id = $1 ORDER BY lower(name)`, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Name, &s.Private, &s.Enabled, &s.cursor, &s.LastSyncedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (m *Manager) source(ctx context.Context, connID uuid.UUID, id string) (*Source, error) {
	var s Source
	err := m.deps.Pool.QueryRow(ctx, `SELECT external_id, name, is_private, enabled, cursor, last_synced_at
		FROM connector_sources WHERE connection_id = $1 AND external_id = $2`, connID, id).
		Scan(&s.ID, &s.Name, &s.Private, &s.Enabled, &s.cursor, &s.LastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

// replaceSources makes the stored list match the provider's. New sources
// start enabled only when enableNew is set; existing ones keep their choice.
func (m *Manager) replaceSources(ctx context.Context, connID uuid.UUID, list []Source, enableNew bool) error {
	tx, err := m.deps.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ids := make([]string, 0, len(list))
	for _, s := range list {
		ids = append(ids, s.ID)
		if _, err := tx.Exec(ctx, `INSERT INTO connector_sources (connection_id, external_id, name, is_private, enabled)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (connection_id, external_id) DO UPDATE SET name = EXCLUDED.name, is_private = EXCLUDED.is_private`,
			connID, s.ID, s.Name, s.Private, enableNew); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM connector_sources WHERE connection_id = $1 AND NOT (external_id = ANY($2))`, connID, ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *Manager) setSourceEnabled(ctx context.Context, connID uuid.UUID, id string, enabled bool) error {
	_, err := m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET enabled = $3,
		cursor = CASE WHEN $3 THEN cursor ELSE '' END WHERE connection_id = $1 AND external_id = $2`, connID, id, enabled)
	return err
}

func (m *Manager) saveCursor(ctx context.Context, connID uuid.UUID, id, cursor string, finished bool) error {
	q := `UPDATE connector_sources SET cursor = $3 WHERE connection_id = $1 AND external_id = $2`
	if finished {
		q = `UPDATE connector_sources SET cursor = $3, last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`
	}
	if _, err := m.deps.Pool.Exec(ctx, q, connID, id, cursor); err != nil {
		return err
	}
	if finished {
		_, err := m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET last_synced_at = NOW() WHERE id = $1`, connID)
		return err
	}
	return nil
}

// --- OAuth state --------------------------------------------------------------------

const stateTTL = 10 * time.Minute

func (m *Manager) createState(ctx context.Context, provider, agentID string, tenantID uuid.UUID, stateHash string) error {
	if _, err := m.deps.Pool.Exec(ctx, `DELETE FROM connector_oauth_states WHERE expires_at < NOW()`); err != nil {
		return err
	}
	_, err := m.deps.Pool.Exec(ctx, `INSERT INTO connector_oauth_states (state_hash, provider, agent_id, tenant_id, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, stateHash, provider, agentID, tenantID, m.now().Add(stateTTL))
	return err
}

// consumeState returns the agent that started the flow, exactly once.
func (m *Manager) consumeState(ctx context.Context, provider, stateHash string) (string, uuid.UUID, error) {
	var agentID string
	var tenantID uuid.UUID
	err := m.deps.Pool.QueryRow(ctx, `DELETE FROM connector_oauth_states
		WHERE state_hash = $1 AND provider = $2 AND expires_at > NOW() RETURNING agent_id, tenant_id`, stateHash, provider).
		Scan(&agentID, &tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", uuid.Nil, ErrBadState
	}
	return agentID, tenantID, err
}

// --- Jobs ---------------------------------------------------------------------------

type job struct {
	ID           int64
	ConnectionID uuid.UUID
	Kind         string
	Key          string
	Payload      json.RawMessage
	Attempts     int
}

// enqueue adds a job. A job with a non-empty key is dropped if the same work
// is already waiting to start.
func (m *Manager) enqueue(ctx context.Context, q querier, connID uuid.UUID, kind, key string, payload any, delay time.Duration) error {
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = b
	}
	if q == nil {
		q = m.deps.Pool
	}
	_, err := q.Exec(ctx, `INSERT INTO connector_jobs (connection_id, kind, key, payload, run_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, connID, kind, key, raw, m.now().Add(delay))
	if err == nil && delay <= 0 {
		m.nudge()
	}
	return err
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// claim leases the next due job for lease. Rows already leased by another
// worker are skipped, not waited for. A keyed job also waits while the same
// work (connection, kind, key) is leased, so one source never syncs twice at
// once and two passes cannot overwrite each other's position.
func (m *Manager) claim(ctx context.Context, lease time.Duration) (*job, error) {
	var j job
	err := m.deps.Pool.QueryRow(ctx, `UPDATE connector_jobs SET locked_until = $1, attempts = attempts + 1
		WHERE id = (
			SELECT id FROM connector_jobs j
			WHERE run_at <= NOW() AND (locked_until IS NULL OR locked_until < NOW())
				AND (key = '' OR NOT EXISTS (SELECT 1 FROM connector_jobs o
					WHERE o.connection_id = j.connection_id AND o.kind = j.kind AND o.key = j.key
						AND o.id <> j.id AND o.locked_until >= NOW()))
			ORDER BY run_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, connection_id, kind, key, payload, attempts`, m.now().Add(lease)).
		Scan(&j.ID, &j.ConnectionID, &j.Kind, &j.Key, &j.Payload, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}

func (m *Manager) finishJob(ctx context.Context, id int64) error {
	_, err := m.deps.Pool.Exec(ctx, `DELETE FROM connector_jobs WHERE id = $1`, id)
	return err
}

// retryJob keeps the job leased until it is due again, so a fresh copy of
// the same work can still be queued (and coalesced) meanwhile.
func (m *Manager) retryJob(ctx context.Context, id int64, after time.Duration, countAttempt bool, msg string) error {
	at := m.now().Add(after)
	_, err := m.deps.Pool.Exec(ctx, `UPDATE connector_jobs SET run_at = $2, locked_until = $2, last_error = $3,
		attempts = attempts - CASE WHEN $4 THEN 0 ELSE 1 END WHERE id = $1`, id, at, truncate(msg, 1000), countAttempt)
	return err
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
