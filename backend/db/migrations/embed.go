// Package migrations embeds the golang-migrate SQL files so the migration runner
// (and tests) use the exact same on-disk migrations as the CLI / production Job.
package migrations

import "embed"

// FS holds the numbered up/down migration files (000001 schema, 000002 seed).
//
//go:embed *.sql
var FS embed.FS
