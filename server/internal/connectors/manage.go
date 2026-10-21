package connectors

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/connectors/dce"
)

// StateCookie holds the OAuth state in the browser that started the flow.
// The callback only proceeds when the state in the URL matches it, so a
// link crafted by someone else cannot connect their account to yours.
const StateCookie = "memro_connector_state"

// StartAuthorization begins connecting a provider for agentID and returns
// the provider URL to send the browser to, plus the state for StateCookie.
func (m *Manager) StartAuthorization(ctx context.Context, providerName, agentID string, tenantID uuid.UUID) (string, string, error) {
	p, err := m.provider(providerName)
	if err != nil {
		return "", "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	state := base64.RawURLEncoding.EncodeToString(buf)
	if err := m.createState(ctx, providerName, agentID, tenantID, auth.HashToken(state)); err != nil {
		return "", "", err
	}
	return p.authorizeURL(state), state, nil
}

// CompleteAuthorization handles the provider's redirect back and saves the
// connection for the agent that started the flow.
func (m *Manager) CompleteAuthorization(ctx context.Context, providerName string, q url.Values, cookieState string) (*Connection, error) {
	p, err := m.provider(providerName)
	if err != nil {
		return nil, err
	}
	state := q.Get("state")
	if state == "" || cookieState == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return nil, ErrBadState
	}
	agentID, tenantID, err := m.consumeState(ctx, providerName, auth.HashToken(state))
	if err != nil {
		return nil, err
	}
	if e := q.Get("error"); e != "" {
		if e == "access_denied" {
			return nil, &UserError{"Connection cancelled."}
		}
		return nil, &UserError{"The provider returned an error: " + truncate(e, 100)}
	}
	a, err := p.complete(ctx, q)
	if err != nil {
		return nil, err
	}
	id, err := m.saveConnection(ctx, agentID, tenantID, providerName, a)
	if err != nil {
		return nil, err
	}
	if err := m.enqueue(ctx, nil, id, jobSources, jobSources, nil, 0); err != nil {
		return nil, err
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	c.ManageURL = p.manageURL(c)
	return c, nil
}

// ConnectToken connects a token-based connector directly for the agent.
func (m *Manager) ConnectToken(ctx context.Context, providerName, agentID string, tenantID uuid.UUID, token string) (*Connection, error) {
	p, err := m.provider(providerName)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("token", token)
	a, err := p.complete(ctx, values)
	if err != nil {
		return nil, err
	}
	id, err := m.saveConnection(ctx, agentID, tenantID, providerName, a)
	if err != nil {
		return nil, err
	}
	_, _ = m.deps.Pool.Exec(ctx, `INSERT INTO connector_sources (connection_id, external_id, name, is_private, enabled)
		VALUES ($1, $2, $3, false, true)
		ON CONFLICT (connection_id, external_id) DO UPDATE SET enabled = true`, id, providerName, dce.FormattedDisplayName(providerName))
	if err := m.enqueue(ctx, nil, id, jobSources, jobSources, nil, 0); err != nil {
		return nil, err
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	c.ManageURL = p.manageURL(c)
	return c, nil
}

// List returns the agent's connections with sync statistics.
func (m *Manager) List(ctx context.Context, agentID string) ([]Connection, error) {
	rows, err := m.deps.Pool.Query(ctx, `SELECT `+connectionColumns+`,
			(SELECT COUNT(*) FROM connector_sources s WHERE s.connection_id = c.id AND s.enabled),
			(SELECT COUNT(*) FROM connector_sources s WHERE s.connection_id = c.id),
			(SELECT COUNT(*) FROM memories mm WHERE mm.connection_id = c.id),
			(SELECT COUNT(*) FROM connector_jobs j WHERE j.connection_id = c.id)
		FROM connector_connections c WHERE c.agent_id = $1 ORDER BY c.created_at`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.TenantID, &c.AgentID, &c.Provider, &c.AccountID, &c.AccountName, &c.ExternalUserID,
			&c.ExternalUserName, &c.sealed, &c.Status, &c.StatusDetail, &c.CreatedAt, &c.LastSyncedAt,
			&c.SourcesEnabled, &c.SourcesTotal, &c.Memories, &c.PendingJobs); err != nil {
			return nil, err
		}
		if p, ok := m.providers[c.Provider]; ok {
			c.ManageURL = p.manageURL(&c)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Sources lists a connection's channels or repositories.
func (m *Manager) Sources(ctx context.Context, agentID string, id uuid.UUID) ([]Source, error) {
	if _, err := m.owned(ctx, agentID, id); err != nil {
		return nil, err
	}
	return m.sources(ctx, id)
}

// UpdateSources turns syncing on or off per source (Slack channels).
func (m *Manager) UpdateSources(ctx context.Context, agentID string, id uuid.UUID, enabled map[string]bool) error {
	c, err := m.owned(ctx, agentID, id)
	if err != nil {
		return err
	}
	if c.Status == StatusRevoked {
		return &UserError{"This connection was revoked. Reconnect it first."}
	}
	p, err := m.provider(c.Provider)
	if err != nil {
		return err
	}
	cc, err := m.open(ctx, c)
	if err != nil {
		return err
	}
	return p.setSources(ctx, cc, enabled)
}

// SyncNow resumes a paused or failed connection and syncs it immediately.
func (m *Manager) SyncNow(ctx context.Context, agentID string, id uuid.UUID) error {
	c, err := m.owned(ctx, agentID, id)
	if err != nil {
		return err
	}
	if c.Status == StatusRevoked {
		return &UserError{"This connection was revoked. Reconnect it to sync again."}
	}
	if _, err := m.provider(c.Provider); err != nil {
		return err
	}
	if err := m.setStatus(ctx, id, StatusActive, ""); err != nil {
		return err
	}
	if err := m.enqueue(ctx, nil, id, jobSources, jobSources, nil, 0); err != nil {
		return err
	}
	srcs, err := m.sources(ctx, id)
	if err != nil {
		return err
	}
	for _, s := range srcs {
		if s.Enabled {
			if err := m.enqueue(ctx, nil, id, jobSync, s.ID, nil, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// Disconnect removes a connection, releases its provider credentials and,
// if asked, deletes the memories it created. Otherwise those memories stay.
func (m *Manager) Disconnect(ctx context.Context, agentID string, id uuid.UUID, deleteMemories bool) (int, error) {
	c, err := m.owned(ctx, agentID, id)
	if err != nil {
		return 0, err
	}
	if p, ok := m.providers[c.Provider]; ok && p.enabled() && c.Status != StatusRevoked {
		if cc, err := m.open(ctx, c); err == nil {
			var others int
			_ = m.deps.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_connections
				WHERE provider = $1 AND account_id = $2 AND id <> $3 AND status <> 'revoked'`, c.Provider, c.AccountID, c.ID).Scan(&others)
			rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := p.revoke(rctx, cc, others == 0); err != nil {
				slog.Warn("revoke connector credentials", "connection", c.ID, "provider", c.Provider, "err", err)
			}
			cancel()
		}
	}
	tx, err := m.deps.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	deleted := 0
	if deleteMemories {
		tag, err := tx.Exec(ctx, `DELETE FROM memories WHERE connection_id = $1 AND agent_id = $2`, id, agentID)
		if err != nil {
			return 0, err
		}
		deleted = int(tag.RowsAffected())
	}
	if _, err := tx.Exec(ctx, `DELETE FROM connector_connections WHERE id = $1`, id); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	if m.deps.Invalidate != nil {
		m.deps.Invalidate(agentID)
	}
	return deleted, nil
}

// revokeAccount marks every connection of a provider account revoked, e.g.
// when the app is uninstalled from a Slack workspace or GitHub account.
func (m *Manager) revokeAccount(ctx context.Context, provider, accountID, reason string) error {
	if _, err := m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET status = 'revoked', status_detail = $3, updated_at = NOW()
		WHERE provider = $1 AND account_id = $2`, provider, accountID, reason); err != nil {
		return err
	}
	_, err := m.deps.Pool.Exec(ctx, `DELETE FROM connector_jobs WHERE connection_id IN
		(SELECT id FROM connector_connections WHERE provider = $1 AND account_id = $2)`, provider, accountID)
	return err
}
