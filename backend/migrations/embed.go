// Package migrations embeds the SQL migration files so the API binary is self-contained.
package migrations

import "embed"

// FS holds every *.sql file in this directory. Files are applied in lexical order
// (0001_..., 0002_...), so the numeric prefix defines the order.
//
//go:embed *.sql
var FS embed.FS
