// Package migrations provides embedded SQL migration files.
package migrations

import (
	_ "embed"
)

//go:embed sql/001_initial.sql
var InitialSQL string

//go:embed sql/002_last_transition_at.sql
var Migration002LastTransitionAt string
