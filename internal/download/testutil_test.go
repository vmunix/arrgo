// internal/download/testutil_test.go
package download

import (
	"database/sql"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(testSchema)
	require.NoError(t, err)
	return db
}

// insertTestContent inserts a test content row and returns its ID.
// This is needed because downloads reference content via foreign key.
func insertTestContent(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', ?, 2000, 'wanted', 'hd', '/movies')`,
		title,
	)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}
