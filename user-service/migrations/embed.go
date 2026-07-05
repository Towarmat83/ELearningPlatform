// Package migrations embeds the SQL migration files applied at startup.
package migrations

import "embed"

// FS holds the embedded SQL migration files.
//
//go:embed *.sql
var FS embed.FS
