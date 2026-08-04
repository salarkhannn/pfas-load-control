package migrations

import "embed"

// Files contains the reviewed application migrations.
//
//go:embed *.sql
var Files embed.FS
