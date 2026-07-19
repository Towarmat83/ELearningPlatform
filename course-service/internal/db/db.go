// Package db provides the database connection and migration runner used by
// course-service to persist optional lab result tracking data.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go.uber.org/zap"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// maxConns is the maximum number of connections held open in the pool.
const maxConns = 10

// Connect opens a GORM/Postgres connection and verifies connectivity with a
// ping before returning it. Schema is managed exclusively through
// RunMigrations below (GORM's AutoMigrate is never used).
func Connect(ctx context.Context, connURL string) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(connURL), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Warn),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("unwrap sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxConns)
	sqlDB.SetMaxIdleConns(maxConns)

	err = sqlDB.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return gdb, nil
}

// RunMigrations creates the _migrations bookkeeping table if needed and
// applies any *.sql files under migrationsFS that have not yet been
// recorded as applied, in filename order.
func RunMigrations(ctx context.Context, gdb *gorm.DB, migrationsFS embed.FS) error {
	err := ensureMigrationsTable(ctx, gdb)
	if err != nil {
		return err
	}

	applied, err := loadAppliedMigrations(ctx, gdb)
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

		err = applyMigration(ctx, gdb, migrationsFS, name)
		if err != nil {
			return err
		}
	}

	return nil
}

// ensureMigrationsTable creates the _migrations bookkeeping table if it does
// not already exist.
func ensureMigrationsTable(ctx context.Context, gdb *gorm.DB) error {
	err := gdb.WithContext(ctx).Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`).Error
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	return nil
}

// loadAppliedMigrations returns the set of migration filenames that have
// already been recorded in the _migrations table.
func loadAppliedMigrations(ctx context.Context, gdb *gorm.DB) (map[string]bool, error) {
	var filenames []string

	err := gdb.WithContext(ctx).Raw("SELECT filename FROM _migrations").Scan(&filenames).Error
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}

	applied := make(map[string]bool, len(filenames))
	for _, filename := range filenames {
		applied[filename] = true
	}

	return applied, nil
}

// applyMigration reads the named migration file from migrationsFS, executes
// it, and records it as applied.
func applyMigration(ctx context.Context, gdb *gorm.DB, migrationsFS embed.FS, name string) error {
	content, err := fs.ReadFile(migrationsFS, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	err = gdb.WithContext(ctx).Exec(string(content)).Error
	if err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}

	err = gdb.WithContext(ctx).Exec("INSERT INTO _migrations (filename) VALUES (?)", name).Error
	if err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}

	zap.L().Info("migration applied", zap.String("file", name))

	return nil
}
