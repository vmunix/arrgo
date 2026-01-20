// internal/importer/testutil_test.go
package importer

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
	require.NoError(t, err, "open db")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(testSchema)
	require.NoError(t, err, "apply schema")
	return db
}

func insertTestContent(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`,
	)
	require.NoError(t, err, "insert test content")
	id, _ := result.LastInsertId()
	return id
}
