package connectors

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/memrohq/memro/server/internal/metrics"
)

// Job kinds. Each job makes a handful of provider calls at most, so a slow
// or rate-limited provider delays only its own work and a restart never
// loses more than one step.
const (
	jobSources = "sources" // refresh the connection's channel / repository list
	jobSync    = "sync"    // one page of a source's history (key: source ID)
	jobEvent   = "event"   // apply one Slack event
	jobThread  = "thread"  // fetch one Slack thread (key: channel/ts)
	jobRender  = "render"  // rebuild one Slack item's memories (key: item key)
	jobItem    = "item"    // fetch one GitHub issue or pull request (key: repo/number)
)

const (
	jobLease       = 5 * time.Minute
	jobTimeout     = 2 * time.Minute
	maxAttempts    = 8
	pollInterval   = 2 * time.Second
	scheduleEvery  = time.Minute
	sourcesRefresh = 6 * time.Hour
)

func (m *Manager) anyEnabled() bool {
	for _, p := range m.providers {
		if p.enabled() {
			return true
		}
	}
	return false
}

// nudge wakes an idle worker.
func (m *Manager) nudge() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Run processes jobs and schedules reconciliation syncs until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	if !m.anyEnabled() {
		return
	}
	for range m.cfg.Workers {
		go m.worker(ctx)
	}
	m.scheduler(ctx)
}

func (m *Manager) worker(ctx context.Context) {
	idle := time.NewTimer(0)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-idle.C:
		case <-m.wake:
		}
		for ctx.Err() == nil {
			j, err := m.claim(ctx, jobLease)
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("claim connector job", "err", err)
				}
				break
			}
			if j == nil {
				break
			}
			m.runJob(ctx, j)
		}
		idle.Reset(pollInterval)
	}
}

// RunOnce runs due jobs until none are left (for tests and tooling).
func (m *Manager) RunOnce(ctx context.Context) error {
	for {
		j, err := m.claim(ctx, jobLease)
		if err != nil || j == nil {
			return err
		}
		m.runJob(ctx, j)
	}
}

func (m *Manager) runJob(ctx context.Context, j *job) {
	jctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	err := m.dispatch(jctx, j)
	result := "ok"
	defer func() { metrics.ConnectorJobs.WithLabelValues(j.Kind, result).Inc() }()

	// Bookkeeping must happen even if the job used up its own deadline.
	bctx, bcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer bcancel()
	if err == nil {
		if ferr := m.finishJob(bctx, j.ID); ferr != nil {
			slog.Error("finish connector job", "id", j.ID, "err", ferr)
		}
		return
	}

	var rl *rateLimitedError
	var rv *revokedError
	var qe *quotaError
	var pe *permanentError
	log := slog.With("job", j.ID, "kind", j.Kind, "key", j.Key, "connection", j.ConnectionID, "attempt", j.Attempts)
	switch {
	case errors.As(err, &rl):
		result = "rate_limited"
		log.Info("connector job rate limited", "retry_in", rl.after)
		_ = m.retryJob(bctx, j.ID, rl.after, false, err.Error())
	case errors.As(err, &rv):
		result = "revoked"
		log.Warn("connector revoked", "err", err)
		_ = m.setStatus(bctx, j.ConnectionID, StatusRevoked, rv.msg)
		_, _ = m.deps.Pool.Exec(bctx, `DELETE FROM connector_jobs WHERE connection_id = $1`, j.ConnectionID)
	case errors.As(err, &qe):
		result = "quota"
		log.Info("connector paused: memory limit", "err", err)
		_ = m.setStatus(bctx, j.ConnectionID, StatusPaused, qe.msg)
		_, _ = m.deps.Pool.Exec(bctx, `DELETE FROM connector_jobs WHERE connection_id = $1`, j.ConnectionID)
	case errors.As(err, &pe):
		result = "dropped"
		log.Warn("connector job dropped", "err", err)
		_ = m.finishJob(bctx, j.ID)
	case j.Attempts >= maxAttempts:
		result = "failed"
		log.Error("connector job failed", "err", err)
		_ = m.finishJob(bctx, j.ID)
		if j.Kind == jobSync || j.Kind == jobSources {
			_ = m.setStatus(bctx, j.ConnectionID, StatusError, "Sync keeps failing: "+err.Error())
		}
	default:
		result = "retry"
		log.Warn("connector job will retry", "err", err)
		_ = m.retryJob(bctx, j.ID, backoff(j.Attempts), true, err.Error())
	}
}

// backoff is 30s, 1m, 2m, … capped at an hour.
func backoff(attempt int) time.Duration {
	d := time.Duration(30*math.Pow(2, float64(attempt-1))) * time.Second
	return min(d, time.Hour)
}

func (m *Manager) dispatch(ctx context.Context, j *job) error {
	c, err := m.connection(ctx, j.ConnectionID)
	if errors.Is(err, ErrNotFound) {
		return nil // disconnected meanwhile
	}
	if err != nil {
		return err
	}
	if c.Status != StatusActive {
		return nil // paused or revoked: work resumes with the next sync
	}
	p, ok := m.providers[c.Provider]
	if !ok || !p.enabled() {
		return &rateLimitedError{after: time.Hour} // configuration may return
	}
	cc, err := m.open(ctx, c)
	if err != nil {
		return err
	}
	if err := p.handle(ctx, cc, j); err != nil {
		return err
	}
	// A connection that recovered on its own clears its old error.
	if j.Kind == jobSync || j.Kind == jobSources {
		_, _ = m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET status_detail = ''
			WHERE id = $1 AND status = 'active' AND status_detail <> ''`, c.ID)
	}
	return nil
}

// scheduler queues reconciliation syncs: every enabled source at the sync
// interval, and every connection's source list every few hours. Webhooks
// keep data current in between; this catches anything they missed.
func (m *Manager) scheduler(ctx context.Context) {
	t := time.NewTicker(scheduleEvery)
	defer t.Stop()
	lastSources := time.Time{}
	lastClean := time.Time{}
	for {
		m.scheduleSyncs(ctx)
		if time.Since(lastSources) >= sourcesRefresh {
			m.scheduleSources(ctx)
			lastSources = time.Now()
		}
		if time.Since(lastClean) >= time.Hour {
			_, _ = m.CleanWebhookDeliveries(ctx, webhookDeliveryTTL)
			lastClean = time.Now()
		}
		var due int
		if err := m.deps.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_jobs WHERE run_at <= NOW()`).Scan(&due); err == nil {
			metrics.ConnectorQueue.Set(float64(due))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) scheduleSyncs(ctx context.Context) {
	tag, err := m.deps.Pool.Exec(ctx, `INSERT INTO connector_jobs (connection_id, kind, key)
		SELECT s.connection_id, 'sync', s.external_id FROM connector_sources s
		JOIN connector_connections c ON c.id = s.connection_id
		WHERE c.status = 'active' AND s.enabled
			AND (s.last_synced_at IS NULL OR s.last_synced_at < NOW() - make_interval(secs => $1))
			AND NOT EXISTS (SELECT 1 FROM connector_jobs j
				WHERE j.connection_id = s.connection_id AND j.kind = 'sync' AND j.key = s.external_id)
		ON CONFLICT DO NOTHING`, m.cfg.SyncInterval.Seconds())
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("schedule connector syncs", "err", err)
		}
		return
	}
	if tag.RowsAffected() > 0 {
		m.nudge()
	}
}

func (m *Manager) scheduleSources(ctx context.Context) {
	_, err := m.deps.Pool.Exec(ctx, `INSERT INTO connector_jobs (connection_id, kind, key)
		SELECT id, 'sources', 'sources' FROM connector_connections WHERE status = 'active'
		ON CONFLICT DO NOTHING`)
	if err != nil && ctx.Err() == nil {
		slog.Error("schedule connector source refresh", "err", err)
	}
}
