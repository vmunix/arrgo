// internal/importer/testutil_test.go
package importer

import (
	"database/sql"
	_ "embed"
	"testing"

	_ "modernc.org/sqlite"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db
}

func insertTestContent(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', ?, 2024, 'wanted', 'hd', '/movies')`,
		title,
	)
	if err != nil {
		t.Fatalf("insert test content: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}
