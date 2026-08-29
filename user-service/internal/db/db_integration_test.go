//go:build integration

// db_integration_test.go exercises Connect and RunMigrations against a live
// PostgreSQL instance. Behind the `integration` build tag; run with
// TEST_DATABASE_URL set.
package db_test

import (
	"os"
	"testing"

	"github.com/genesary/pupitre/user-service/internal/db"
)

// dbURL returns TEST_DATABASE_URL or skips the test.
func dbURL(t *testing.T) string {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	return url
}

// TestConnectAndMigrate connects, runs the migrations, and runs them a second
// time to prove the migration ledger makes the operation idempotent.
func TestConnectAndMigrate(t *testing.T) { //nolint:paralleltest // owns the shared schema
	url := dbURL(t)

	gdb, err := db.Connect(t.Context(), url, 5, 2)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	err = db.RunMigrations(t.Context(), gdb)
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	err = db.RunMigrations(t.Context(), gdb)
	if err != nil {
		t.Fatalf("RunMigrations (second run): %v", err)
	}

	// Migrations must have created the core tables.
	if !gdb.Migrator().HasTable("users") || !gdb.Migrator().HasTable("_schema_migrations") {
		t.Error("expected users and _schema_migrations tables after migration")
	}
}

// TestConnect_BadURL returns an error rather than a usable handle.
func TestConnect_BadURL(t *testing.T) { //nolint:paralleltest // consistent with the file
	_ = dbURL(t) // still skip when no DB configured, to keep the suite hermetic-friendly

	_, err := db.Connect(t.Context(), "postgres://127.0.0.1:1/nope?sslmode=disable&connect_timeout=1", 1, 1)
	if err == nil {
		t.Fatal("expected Connect to fail against an unreachable database")
	}
}

// TestMigrate_CollapsesDuplicateXPEvents exercises the upgrade path for the
// unique index added to user_xp_events.
//
// The index cannot be created while duplicates exist, so a breaking migration
// collapses them first. This reproduces a database in the pre-index state —
// the table without its constraint, carrying repeat awards — and asserts the
// migration both succeeds and keeps exactly one event per (user, source,
// slug), i.e. that the learner's inflated total is corrected rather than the
// upgrade failing.
func TestMigrate_CollapsesDuplicateXPEvents(t *testing.T) { //nolint:paralleltest // owns the shared schema
	url := dbURL(t)

	gdb, err := db.Connect(t.Context(), url, 5, 2)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// The rows seeded below outlive this test's own schema work, and the
	// FK they carry would make a later RunMigrations fail, so clear them.
	t.Cleanup(func() {
		_ = gdb.Exec(`DELETE FROM user_xp_events
		              WHERE userid = '11111111-1111-1111-1111-111111111111'`).Error
		_ = gdb.Exec(`DELETE FROM users
		              WHERE id = '11111111-1111-1111-1111-111111111111'`).Error
	})

	// Rebuild the table as it was before the index, and record that the
	// de-dupe migration has not run yet.
	setup := []string{
		// user_xp_events carries an FK to users, which ensureForeignKeys
		// re-adds after AutoMigrate — so the learner has to exist.
		`INSERT INTO users (id, username, email, role)
		 VALUES ('11111111-1111-1111-1111-111111111111', 'xp-learner', 'xp-learner@test.local', 'student')
		 ON CONFLICT (id) DO NOTHING`,
		`DROP TABLE IF EXISTS user_xp_events`,
		`CREATE TABLE user_xp_events (
			id BIGSERIAL PRIMARY KEY,
			userid UUID NOT NULL,
			source VARCHAR(16) NOT NULL,
			source_slug VARCHAR(255) NOT NULL,
			amount INT NOT NULL,
			earned_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`DELETE FROM _schema_migrations WHERE name = '20260829_dedupe_user_xp_events'`,
		`INSERT INTO user_xp_events (userid, source, source_slug, amount) VALUES
			('11111111-1111-1111-1111-111111111111', 'lesson', 'linux/intro', 10),
			('11111111-1111-1111-1111-111111111111', 'lesson', 'linux/intro', 10),
			('11111111-1111-1111-1111-111111111111', 'lesson', 'linux/intro', 10),
			('11111111-1111-1111-1111-111111111111', 'course', 'linux', 50)`,
	}

	for _, statement := range setup {
		execErr := gdb.WithContext(t.Context()).Exec(statement).Error
		if execErr != nil {
			t.Fatalf("seed pre-index state (%s): %v", statement, execErr)
		}
	}

	err = db.RunMigrations(t.Context(), gdb)
	if err != nil {
		t.Fatalf("RunMigrations over duplicated xp events: %v", err)
	}

	var total int

	err = gdb.WithContext(t.Context()).
		Raw(`SELECT COALESCE(SUM(amount), 0) FROM user_xp_events
		     WHERE userid = '11111111-1111-1111-1111-111111111111'`).
		Scan(&total).Error
	if err != nil {
		t.Fatalf("sum xp: %v", err)
	}

	// 10 for the (collapsed) lesson + 50 for the course; the two extra
	// lesson rows are gone.
	if total != 60 {
		t.Errorf("want 60 XP after collapsing duplicates, got %d", total)
	}

	// And the index now exists, so a repeat award is a no-op rather than a
	// fourth row.
	err = gdb.WithContext(t.Context()).Exec(
		`INSERT INTO user_xp_events (userid, source, source_slug, amount)
		 VALUES ('11111111-1111-1111-1111-111111111111', 'lesson', 'linux/intro', 10)
		 ON CONFLICT (userid, source, source_slug) DO NOTHING`).Error
	if err != nil {
		t.Fatalf("the unique index is missing after migration: %v", err)
	}
}
