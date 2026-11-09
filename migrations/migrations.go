package migrations

import "embed"

// FS embeds all SQL migration scripts for goose.
//
//go:embed *.sql
var FS embed.FS
