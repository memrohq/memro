// Package db opens the Postgres pool and applies schema migrations.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

func Open(ctx context.Context, url string, maxConns, minConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnIdleTime = 10 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// The vector type only exists once migration 0021 has run; ignore the
		// error on the very first connection and register on later ones.
		_ = pgxvec.RegisterTypes(ctx, conn)
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// Migrate applies every embedded NNNN_name.sql file that has not run yet,
// each in its own transaction. Databases previously migrated by the Rust
// backend (sqlx) are detected and their applied versions are carried over.
// migrateLockKey is the advisory lock that serialises Migrate across processes
// (server instances starting together, test packages sharing a database).
const migrateLockKey = 0x6d656d726f // "memro"

func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS) error {
	// Two migrators running at once both see a migration as unapplied and race
	// on CREATE TABLE IF NOT EXISTS (duplicate pg_type rows). Hold a session
	// advisory lock on one connection for the whole run.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lockConn.Release()
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockKey); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer lockConn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrateLockKey)

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return err
	}
	if err := importSqlxHistory(ctx, pool); err != nil {
		return err
	}

	applied := map[int64]bool{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	files, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, name := range files {
		version, err := versionOf(name)
		if err != nil {
			return err
		}
		if applied[version] {
			continue
		}
		sqlBytes, err := fs.ReadFile(migrations, name)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, version, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		slog.Info("applied migration", "name", name)
	}

	// Re-register the vector type on pooled connections opened before 0021.
	pool.Reset()
	return nil
}

func importSqlxHistory(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('_sqlx_migrations') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		return err
	}
	_, err := pool.Exec(ctx, `INSERT INTO schema_migrations (version, name)
		SELECT version, description FROM _sqlx_migrations WHERE success
		ON CONFLICT (version) DO NOTHING`)
	return err
}

func versionOf(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must be named NNNN_description.sql", name)
	}
	return strconv.ParseInt(prefix, 10, 64)
}
