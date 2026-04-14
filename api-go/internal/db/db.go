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

func Connect(ctx context.Context, connURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connURL)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

// RunMigrations applies SQL files from migrationsFS (root directory) in alphabetical order.
// Applied files are tracked in _go_migrations.
// On first run against an existing Rust/SQLx database, it seeds _go_migrations from
// _sqlx_migrations to avoid re-applying already-executed migrations.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _go_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// If table is empty, check for a pre-existing Rust/SQLx database and seed accordingly.
	var count int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM _go_migrations").Scan(&count) //nolint:errcheck
	if count == 0 {
		seedFromSQLxMigrations(ctx, pool, migrationsFS)
	}

	// Load applied filenames.
	rows, err := pool.Query(ctx, "SELECT filename FROM _go_migrations")
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	applied := make(map[string]bool)
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return err
		}
		applied[f] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Read SQL files from the FS root directory.
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
		content, err := fs.ReadFile(migrationsFS, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx,
			"INSERT INTO _go_migrations (filename) VALUES ($1)", name); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}
		slog.Info("migration applied", "file", name)
	}
	return nil
}

// seedFromSQLxMigrations seeds _go_migrations from _sqlx_migrations when migrating
// from the Rust/SQLx API to the Go API on an existing database.
func seedFromSQLxMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS) {
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = '_sqlx_migrations'
		)`).Scan(&exists); err != nil || !exists {
		return
	}

	rows, err := pool.Query(ctx,
		"SELECT version FROM _sqlx_migrations WHERE success = TRUE ORDER BY version")
	if err != nil {
		return
	}
	defer rows.Close()

	var versions []int64
	for rows.Next() {
		var v int64
		if rows.Scan(&v) == nil {
			versions = append(versions, v)
		}
	}
	if rows.Err() != nil || len(versions) == 0 {
		return
	}

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return
	}
	versionToFile := make(map[int64]string)
	for _, e := range entries {
		var v int64
		if n, _ := fmt.Sscanf(e.Name(), "%d_", &v); n == 1 {
			versionToFile[v] = e.Name()
		}
	}

	seeded := 0
	for _, v := range versions {
		if filename, ok := versionToFile[v]; ok {
			pool.Exec(ctx, //nolint:errcheck
				"INSERT INTO _go_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING",
				filename)
			seeded++
		}
	}
	if seeded > 0 {
		slog.Info("seeded _go_migrations from _sqlx_migrations", "count", seeded)
	}
}
