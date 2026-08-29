//go:build integration

// export_integration_test.go covers the ExportRepository SQL builder,
// FetchRows and WriteCSV against a live PostgreSQL instance. Behind the
// `integration` build tag; run with TEST_DATABASE_URL set.

package repository_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/genesary/pupitre/user-service/internal/repository"
)

// TestExportRepository_Categories lists the available categories with their
// field and filter descriptors.
func TestExportRepository_Categories(t *testing.T) { //nolint:paralleltest // shared db
	gdb := crudDB(t)
	repo := repository.NewGormExportRepository(gdb)

	cats := repo.Categories()
	if len(cats) == 0 {
		t.Fatal("Categories returned nothing")
	}

	var haveUsers bool

	for _, c := range cats {
		if c.ID == "users" {
			haveUsers = true

			if len(c.Fields) == 0 || len(c.DefaultFields) == 0 {
				t.Errorf("users category missing field metadata: %+v", c)
			}
		}
	}

	if !haveUsers {
		t.Error("expected a \"users\" category")
	}
}

// TestExportRepository_FetchRows_Users returns the requested columns and a
// total count for the users category.
func TestExportRepository_FetchRows_Users(t *testing.T) { //nolint:paralleltest // shared db
	gdb := crudDB(t)
	repo := repository.NewGormExportRepository(gdb)
	ctx := t.Context()

	mkUser(t, gdb)
	mkUser(t, gdb)
	mkUser(t, gdb)

	headers, rows, total, err := repo.FetchRows(ctx, "users", []string{"email", "role"}, nil, 2)
	if err != nil {
		t.Fatalf("FetchRows: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}

	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2 (limit)", len(rows))
	}

	if len(headers) != 2 || headers[0] != "email" {
		t.Errorf("headers = %v", headers)
	}

	for _, r := range rows {
		if r["email"] == "" || r["role"] != "student" {
			t.Errorf("unexpected row: %+v", r)
		}
	}
}

// TestExportRepository_FetchRows_UnknownCategory returns an "unknown ..."
// error the handler maps to 400.
func TestExportRepository_FetchRows_UnknownCategory(t *testing.T) { //nolint:paralleltest // shared db
	gdb := crudDB(t)
	repo := repository.NewGormExportRepository(gdb)

	headers, rows, total, err := repo.FetchRows(t.Context(), "does-not-exist", []string{"x"}, nil, 0)
	if err == nil || !strings.HasPrefix(err.Error(), "unknown") {
		t.Fatalf("want an \"unknown\" error, got %v", err)
	}

	if headers != nil || rows != nil || total != 0 {
		t.Errorf("expected zero values on error, got headers=%v rows=%v total=%d", headers, rows, total)
	}
}

// TestExportRepository_WriteCSV streams the users dataset as semicolon CSV
// with a header row and one line per user.
func TestExportRepository_WriteCSV(t *testing.T) { //nolint:paralleltest // shared db
	gdb := crudDB(t)
	repo := repository.NewGormExportRepository(gdb)
	ctx := t.Context()

	mkUser(t, gdb)
	mkUser(t, gdb)

	var buf bytes.Buffer

	n, err := repo.WriteCSV(ctx, &buf, "users", []string{"email", "role"}, nil)
	if err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	if n != 2 {
		t.Errorf("WriteCSV wrote %d rows, want 2", n)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 rows, got %d lines: %q", len(lines), buf.String())
	}

	if !strings.Contains(lines[0], ";") {
		t.Errorf("expected a semicolon-delimited header, got %q", lines[0])
	}
}

// TestExportRepository_FetchRowsWithFilters exercises buildWhere and
// combineWhere by requesting the users category with an active-status and a
// role filter applied.
func TestExportRepository_FetchRowsWithFilters(t *testing.T) { //nolint:paralleltest // shared db
	gdb := crudDB(t)
	repo := repository.NewGormExportRepository(gdb)
	ctx := t.Context()

	mkUser(t, gdb) // one student, active

	filters := map[string]string{"role": "student", "active": "true"}

	_, rows, total, err := repo.FetchRows(ctx, "users", []string{"email", "role"}, filters, 50)
	if err != nil {
		t.Fatalf("FetchRows(filtered): %v", err)
	}

	if total != 1 || len(rows) != 1 || rows[0]["role"] != "student" {
		t.Fatalf("filtered fetch = total:%d rows:%+v", total, rows)
	}

	// A filter that matches nothing.
	_, rows, total, err = repo.FetchRows(ctx, "users", []string{"email"}, map[string]string{"role": "admin"}, 50)
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("no-match filter = total:%d rows:%d err:%v", total, len(rows), err)
	}

	// WriteCSV with a date filter (a category with a base FROM/JOIN).
	n, err := repo.WriteCSV(ctx, io.Discard, "enrollments", []string{"email", "courseslug"},
		map[string]string{"enrolled_from": "2000-01-01"})
	if err != nil {
		t.Fatalf("WriteCSV(filtered): %v", err)
	}

	if n != 0 {
		t.Errorf("expected 0 enrollment rows, got %d", n)
	}
}
