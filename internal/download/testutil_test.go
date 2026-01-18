// internal/download/testutil_test.go
package download

import (
	"database/sql"
	_ "embed"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
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
	if err != nil {
		t.Fatalf("insert test content: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get content id: %v", err)
	}
	return id
}
