// Command memro-api runs the Memro HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/memrohq/memro/server/internal/api"
	"github.com/memrohq/memro/server/internal/config"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/internal/email"
	"github.com/memrohq/memro/server/internal/embed"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/filestore"
	"github.com/memrohq/memro/server/internal/indexer"
	"github.com/memrohq/memro/server/internal/store"
	"github.com/memrohq/memro/server/migrations"
)

func main() {
	_ = godotenv.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()})))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL must be set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	st := store.New(pool)
	provider := embed.New(cfg.EmbeddingProvider, cfg.EmbeddingModel, cfg.OpenAIAPIKey, cfg.VoyageAPIKey, cfg.EmbeddingBaseURL, cfg.EmbeddingCacheMax)
	slog.Info("embedding provider", "name", provider.Name(), "model", provider.Model(), "enabled", provider.Enabled())
	ix := indexer.New(st, provider)
	eng := engine.New(pool, provider, ix)
	files, err := filestore.NewLocal(cfg.FileStoragePath)
	if err != nil {
		return err
	}

	go ix.Run(ctx)
	go indexer.RunGraphMaintenance(ctx, st)

	srv := api.NewServer(api.Deps{
		Config: cfg, Store: st, Engine: eng, Embed: provider, Indexer: ix, Files: files,
		Email: email.New(cfg.EmailHost, cfg.EmailPort, cfg.EmailUser, cfg.EmailPass, cfg.EmailFrom),
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

func logLevel() slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		return slog.LevelInfo
	}
	return l
}
