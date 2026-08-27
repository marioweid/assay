// Package migrations exposes Assay's embedded database migrations.
package migrations

import "embed"

// Files contains every SQL migration compiled into assayd.
//
//go:embed *.sql
var Files embed.FS
