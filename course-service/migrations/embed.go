// Package migrations embeds the SQL migration files applied by
// internal/db.RunMigrations.
package migrations

import "embed"

// FS holds the embedded *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
