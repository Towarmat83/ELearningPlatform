// Package db provides the database connection pool and migration runner
// used by course-service to persist optional lab result tracking data.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// maxConns is the maximum number of connections held open in the pool.
const maxConns = 10

// Connect opens a pgx connection pool to connURL and verifies connectivity
// with a ping before returning it.
func Connect(ctx context.Context, connURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connURL)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}

	cfg.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return pool, nil
}

// RunMigrations creates the _migrations bookkeeping table if needed and
// applies any *.sql files under migrationsFS that have not yet been
// recorded as applied, in filename order.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS) error {
	err := ensureMigrationsTable(ctx, pool)
	if err != nil {
		return err
	}

	applied, err := loadAppliedMigrations(ctx, pool)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}

		if applied[name] {
			continue
		}

		err = applyMigration(ctx, pool, migrationsFS, name)
		if err != nil {
			return err
		}
	}

	return nil
}

// ensureMigrationsTable creates the _migrations bookkeeping table if it does
// not already exist.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	return nil
}

// loadAppliedMigrations returns the set of migration filenames that have
// already been recorded in the _migrations table.
func loadAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, "SELECT filename FROM _migrations")
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)

	for rows.Next() {
		var filename string

		err = rows.Scan(&filename)
		if err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}

		applied[filename] = true
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return applied, nil
}

// applyMigration reads the named migration file from migrationsFS, executes
// it, and records it as applied.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS, name string) error {
	content, err := fs.ReadFile(migrationsFS, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	_, err = pool.Exec(ctx, string(content))
	if err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}

	_, err = pool.Exec(ctx, "INSERT INTO _migrations (filename) VALUES ($1)", name)
	if err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}

	slog.Info("migration applied", "file", name)

	return nil
}
