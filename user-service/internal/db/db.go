// Package db provides database connection setup and schema management for
// the user service.
package db

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// Connect opens a GORM connection to connURL and verifies connectivity with
// a ping before returning it. maxOpenConns and maxIdleConns cap the
// connection pool size.
func Connect(ctx context.Context, connURL string, maxOpenConns, maxIdleConns int) (*gorm.DB, error) {
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

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)

	err = sqlDB.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return gdb, nil
}

// allModels lists every GORM model whose table AutoMigrate manages. Order
// doesn't matter for AutoMigrate itself (it creates all tables before
// ensureForeignKeys wires up cross-table constraints), but grouping
// user-anchored tables first keeps the list readable.
//
//nolint:gochecknoglobals // static migration configuration, populated once at init
var allModels = []any{
	&models.User{},
	&models.PlatformSetting{},
	&models.Enrollment{},
	&models.LessonProgress{},
	&models.ModuleProgress{},
	&models.Group{},
	&models.UserGroup{},
	&models.GroupRoleMapping{},
	&models.GroupEnrollment{},
	&models.MarkdownPattern{},
	&models.PathEnrollment{},
	&models.UserBadge{},
}

// RunMigrations brings the database schema up to date. Schema management is
// split in two:
//
//   - Additive changes (new tables, columns, indexes) are handled entirely
//     by GORM's AutoMigrate, driven by the `gorm` tags on internal/models
//     structs. This runs on every startup and is safe to run repeatedly.
//   - Breaking changes that AutoMigrate cannot express (renaming/dropping a
//     column, transforming data, splitting a table) go in breakingMigrations
//     below, tracked in _schema_migrations so each runs exactly once. The
//     list is empty today — there is no stable release yet, so the schema is
//     still free to be reshaped directly through model changes. Once a
//     breaking change is needed after a stable release, add an entry there
//     instead of hand-editing already-deployed columns.
//
// A small set of constraints (foreign keys with ON DELETE behavior) aren't
// expressible via AutoMigrate without modeling full GORM associations, which
// this codebase's flat repository style deliberately avoids. ensureForeignKeys
// adds them idempotently after AutoMigrate runs.
func RunMigrations(ctx context.Context, gdb *gorm.DB) error {
	err := applyBreakingMigrations(ctx, gdb)
	if err != nil {
		return err
	}

	err = gdb.WithContext(ctx).AutoMigrate(allModels...)
	if err != nil {
		return fmt.Errorf("auto-migrate schema: %w", err)
	}

	err = ensureForeignKeys(ctx, gdb)
	if err != nil {
		return err
	}

	zap.L().Info("schema migrated")

	return nil
}

// breakingMigration is a one-off Go function for a schema change AutoMigrate
// cannot express safely (column rename/drop, type transform, data backfill).
// Each runs at most once, tracked by Name in _schema_migrations.
type breakingMigration struct {
	Name  string
	Apply func(ctx context.Context, gdb *gorm.DB) error
}

// breakingMigrations is empty for now — see the RunMigrations doc comment.
// To add one:
//
//	{
//		Name: "2026xxxx_rename_foo_to_bar",
//		Apply: func(ctx context.Context, gdb *gorm.DB) error {
//			return gdb.WithContext(ctx).Migrator().RenameColumn(&models.X{}, "foo", "bar")
//		},
//	}
//
//nolint:gochecknoglobals // static migration configuration, populated once at init
var breakingMigrations = []breakingMigration{}

// applyBreakingMigrations runs any breakingMigrations entries not yet
// recorded in _schema_migrations, in slice order, each in its own
// transaction so a failure partway through doesn't record it as applied.
func applyBreakingMigrations(ctx context.Context, gdb *gorm.DB) error {
	err := ensureSchemaMigrationsTable(ctx, gdb)
	if err != nil {
		return err
	}

	applied, err := loadAppliedSchemaMigrations(ctx, gdb)
	if err != nil {
		return err
	}

	for _, migration := range breakingMigrations {
		if applied[migration.Name] {
			continue
		}

		err := gdb.WithContext(ctx).Transaction(func(migrationTx *gorm.DB) error {
			applyErr := migration.Apply(ctx, migrationTx)
			if applyErr != nil {
				return fmt.Errorf("apply %s: %w", migration.Name, applyErr)
			}

			return migrationTx.Exec("INSERT INTO _schema_migrations (name) VALUES (?)", migration.Name).Error
		})
		if err != nil {
			return fmt.Errorf("breaking migration %s: %w", migration.Name, err)
		}

		zap.L().Info("breaking migration applied", zap.String("name", migration.Name))
	}

	return nil
}

// ensureSchemaMigrationsTable creates the _schema_migrations bookkeeping
// table if it does not already exist.
func ensureSchemaMigrationsTable(ctx context.Context, gdb *gorm.DB) error {
	err := gdb.WithContext(ctx).Exec(`
		CREATE TABLE IF NOT EXISTS _schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`).Error
	if err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}

	return nil
}

// loadAppliedSchemaMigrations returns the set of breaking-migration names
// already recorded in _schema_migrations.
func loadAppliedSchemaMigrations(ctx context.Context, gdb *gorm.DB) (map[string]bool, error) {
	var names []string

	err := gdb.WithContext(ctx).Raw("SELECT name FROM _schema_migrations").Scan(&names).Error
	if err != nil {
		return nil, fmt.Errorf("query applied schema migrations: %w", err)
	}

	applied := make(map[string]bool, len(names))
	for _, name := range names {
		applied[name] = true
	}

	return applied, nil
}

// foreignKey describes a FK constraint AutoMigrate doesn't create on its
// own (this codebase's models use raw ID columns, not GORM associations).
type foreignKey struct {
	Name string
	DDL  string
}

// foreignKeys lists every FK constraint the live schema needs. Referential
// integrity for cascading deletes (e.g. DeleteUser) relies on these existing
// in the database — the application does not manually clean up dependent
// rows.
//
//nolint:gochecknoglobals // static migration configuration, populated once at init
var foreignKeys = []foreignKey{
	{
		Name: "enrollments_userid_fkey",
		DDL:  "ALTER TABLE enrollments ADD CONSTRAINT enrollments_userid_fkey FOREIGN KEY (userid) REFERENCES users(id) ON DELETE CASCADE",
	},
	{
		Name: "lesson_progress_userid_fkey",
		DDL:  "ALTER TABLE lesson_progress ADD CONSTRAINT lesson_progress_userid_fkey FOREIGN KEY (userid) REFERENCES users(id) ON DELETE CASCADE",
	},
	{
		Name: "module_progress_userid_fkey",
		DDL:  "ALTER TABLE module_progress ADD CONSTRAINT module_progress_userid_fkey FOREIGN KEY (userid) REFERENCES users(id) ON DELETE CASCADE",
	},
	{
		Name: "user_groups_userid_fkey",
		DDL:  "ALTER TABLE user_groups ADD CONSTRAINT user_groups_userid_fkey FOREIGN KEY (userid) REFERENCES users(id) ON DELETE CASCADE",
	},
	{
		Name: "user_groups_groupid_fkey",
		DDL:  "ALTER TABLE user_groups ADD CONSTRAINT user_groups_groupid_fkey FOREIGN KEY (groupid) REFERENCES groups(id) ON DELETE CASCADE",
	},
	{
		Name: "group_enrollments_groupid_fkey",
		DDL:  "ALTER TABLE group_enrollments ADD CONSTRAINT group_enrollments_groupid_fkey FOREIGN KEY (groupid) REFERENCES groups(id) ON DELETE CASCADE",
	},
	{
		Name: "markdown_patterns_createdby_fkey",
		DDL:  "ALTER TABLE markdown_patterns ADD CONSTRAINT markdown_patterns_createdby_fkey FOREIGN KEY (createdby) REFERENCES users(id) ON DELETE SET NULL",
	},
	{
		Name: "path_enrollments_userid_fkey",
		DDL:  "ALTER TABLE path_enrollments ADD CONSTRAINT path_enrollments_userid_fkey FOREIGN KEY (userid) REFERENCES users(id) ON DELETE CASCADE",
	},
}

// ensureForeignKeys adds every entry in foreignKeys that isn't already
// present, keyed by constraint name via pg_constraint. Safe to run on every
// startup.
func ensureForeignKeys(ctx context.Context, gdb *gorm.DB) error {
	for _, constraint := range foreignKeys {
		var count int64

		err := gdb.WithContext(ctx).Raw(
			"SELECT count(*) FROM pg_constraint WHERE conname = ?", constraint.Name,
		).Scan(&count).Error
		if err != nil {
			return fmt.Errorf("check constraint %s: %w", constraint.Name, err)
		}

		if count > 0 {
			continue
		}

		err = gdb.WithContext(ctx).Exec(constraint.DDL).Error
		if err != nil {
			return fmt.Errorf("add constraint %s: %w", constraint.Name, err)
		}

		zap.L().Info("foreign key constraint added", zap.String("name", constraint.Name))
	}

	return nil
}
