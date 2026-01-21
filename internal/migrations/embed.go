// Package migrations provides embedded SQL migration files.
package migrations

import (
	_ "embed"
)

//go:embed sql/001_initial.sql
var InitialSQL string

//go:embed sql/002_last_transition_at.sql
var Migration002LastTransitionAt string

//go:embed sql/003_downloads_status_cleaned.sql
var Migration003DownloadsStatusCleaned string

//go:embed sql/005_events.sql
var Migration005Events string
