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
