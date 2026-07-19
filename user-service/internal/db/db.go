// Package db provides database connection setup and schema migrations for
// the user service.
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

// maxConns caps the number of open/idle connections in the pool.
const maxConns = 20

// Connect opens a GORM connection to connURL and verifies connectivity with
// a ping before returning it.
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

// loadAppliedMigrations returns the set of migration filenames already
// recorded in the _migrations table.
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

// applyMigration executes a single migration file's SQL and records it as
// applied in the _migrations table.
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

// RunMigrations applies any .sql files in migrationsFS that are not yet
// recorded in the _migrations table, in filename order.
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
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || applied[name] {
			continue
		}

		err := applyMigration(ctx, gdb, migrationsFS, name)
		if err != nil {
			return err
		}
	}

	return nil
}
