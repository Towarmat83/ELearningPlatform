//go:build integration

// main_integration_test.go covers seedDatabase (migrations + admin/OIDC/mock
// seeding) against a live PostgreSQL instance. Behind the `integration`
// build tag; run with TEST_DATABASE_URL set.

package main

import (
	"os"
	"testing"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/genesary/pupitre/user-service/internal/config"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// TestSeedDatabase runs the full startup seed against Postgres, once with mock
// data enabled and once without, proving it is idempotent.
func TestSeedDatabase(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	gdb, err := gorm.Open(gormpg.Open(url), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// This suite shares one database with the other integration packages;
	// start from a clean slate so the admin/student counts are deterministic.
	// Guarded by to_regclass so a first run against a not-yet-migrated
	// database (nothing to clean) is a no-op rather than an error.
	err = gdb.WithContext(t.Context()).Exec(`DO $$ BEGIN
		IF to_regclass('public.users') IS NOT NULL THEN
			EXECUTE 'TRUNCATE users, enrollments, platform_settings RESTART IDENTITY CASCADE';
		END IF;
	END $$;`).Error
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	repos := repository.NewGormRepositories(gdb)
	cfg := &config.Config{AdminPassword: "S3cure-Admin-Pass"}

	t.Setenv("SEED_MOCK_DATA", "true")

	err = seedDatabase(t.Context(), gdb, repos, cfg)
	if err != nil {
		t.Fatalf("seedDatabase: %v", err)
	}

	admins, err := repos.Users.CountByRole(t.Context(), "admin")
	if err != nil || admins != 1 {
		t.Fatalf("admin count after seed = %d, %v", admins, err)
	}

	var students int64

	_ = gdb.WithContext(t.Context()).Table("users").Where("role = ?", "student").Count(&students)
	if students == 0 {
		t.Error("SEED_MOCK_DATA=true did not create mock students")
	}

	// Second run must not error or add another admin.
	err = seedDatabase(t.Context(), gdb, repos, cfg)
	if err != nil {
		t.Fatalf("seedDatabase (second run): %v", err)
	}

	admins, _ = repos.Users.CountByRole(t.Context(), "admin")
	if admins != 1 {
		t.Errorf("admin count changed on re-seed: %d", admins)
	}
}
