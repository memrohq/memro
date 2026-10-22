// Package app builds and runs the Memro server. The community binary
// (cmd/memro-api) calls Run with no options; other distributions, such as
// Memro Cloud, pass options to plug in their own metering, quotas and routes.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/api"
	"github.com/memrohq/memro/server/internal/connectors"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/internal/email"
	"github.com/memrohq/memro/server/internal/embed"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/filestore"
	"github.com/memrohq/memro/server/internal/indexer"
	"github.com/memrohq/memro/server/internal/store"
	"github.com/memrohq/memro/server/migrations"
)

type options struct {
	usage   extension.UsageMeter
	quota   extension.QuotaPolicy
	routes  map[string]http.Handler
	onStart []func(context.Context, *pgxpool.Pool) error
}

// Option customises the server.
type Option func(*options)

// WithUsageMeter replaces the default meter, which writes to tenant_usage.
// Use extension.MultiMeter to keep the default alongside your own.
func WithUsageMeter(m extension.UsageMeter) Option { return func(o *options) { o.usage = m } }

// WithQuota replaces the default per-agent memory cap (MEMORY_SOFT_CAP).
func WithQuota(q extension.QuotaPolicy) Option { return func(o *options) { o.quota = q } }

// WithRoute mounts an extra handler that bypasses agent authentication,
// e.g. "POST /billing/webhook". Patterns use net/http ServeMux syntax.
func WithRoute(pattern string, h http.Handler) Option {
	return func(o *options) {
		if o.routes == nil {
			o.routes = map[string]http.Handler{}
		}
		o.routes[pattern] = h
	}
}

// OnStart runs fn after migrations, before the server accepts requests.
// Distributions use it for their own migrations or background workers.
func OnStart(fn func(ctx context.Context, pool *pgxpool.Pool) error) Option {
	return func(o *options) { o.onStart = append(o.onStart, fn) }
}

// StoreUsageMeter returns the default meter for the given pool, for
// combining with a custom meter via extension.MultiMeter.
func StoreUsageMeter(pool *pgxpool.Pool) extension.UsageMeter {
	return api.StoreUsageMeter(store.New(pool))
}

// Run starts the server and blocks until ctx is cancelled or the listener
// fails, then shuts down gracefully.
func Run(ctx context.Context, cfg config.Config, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL must be set")
	}

	pool, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	slog.Info("database connected", "max_conns", cfg.DBMaxConns)

	if cfg.MigrateOnStartup {
		if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
			return err
		}
		slog.Info("migrations up to date")
	}
	for _, fn := range o.onStart {
		if err := fn(ctx, pool); err != nil {
			return err
		}
	}

	st := store.New(pool)
	provider := embed.New(cfg.EmbeddingProvider, cfg.EmbeddingModel, cfg.OpenAIAPIKey, cfg.VoyageAPIKey, cfg.EmbeddingBaseURL, cfg.EmbeddingCacheMax)
	slog.Info("embedding provider", "name", provider.Name(), "model", provider.Model(), "enabled", provider.Enabled())
	ix := indexer.New(st, provider)
	eng := engine.New(pool, provider, ix)
	ix.OnIndexed = eng.Cache().InvalidateAgent
	files, err := filestore.NewLocal(cfg.FileStoragePath)
	if err != nil {
		return err
	}

	usage, quota := o.usage, o.quota
	if usage == nil {
		usage = api.StoreUsageMeter(st)
	}
	if quota == nil {
		quota = extension.PerAgentCap{Max: cfg.MemorySoftCap}
	}
	conns, err := connectors.New(connectors.Deps{Config: cfg, Pool: pool, Store: st, Indexer: ix,
		Invalidate: eng.Cache().InvalidateAgent, Quota: quota, Usage: usage})
	if err != nil {
		return err
	}

	go ix.Run(ctx)
	go indexer.RunGraphMaintenance(ctx, st)
	go store.RunCoordinationPurge(ctx, st)
	go conns.Run(ctx)

	srv := api.NewServer(api.Deps{
		Config: cfg, Store: st, Engine: eng, Embed: provider, Indexer: ix, Files: files,
		Email: email.New(cfg.EmailHost, cfg.EmailPort, cfg.EmailUser, cfg.EmailPass, cfg.EmailFrom),
		Usage: usage, Quota: quota, Routes: o.routes, Connectors: conns,
	})
	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", httpServer.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
