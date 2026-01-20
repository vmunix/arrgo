// internal/library/testutil_test.go
package library

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

// ptr is a helper to create pointer to value
func ptr[T any](v T) *T {
	return &v
}
