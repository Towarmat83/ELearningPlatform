package migrations

import "embed"

// FS embeds all SQL migration files at compile time.
//
//go:embed *.sql
var FS embed.FS
