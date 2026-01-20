# Pure Go SQLite Migration (Phase 2) Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `mattn/go-sqlite3` (CGo) with `modernc.org/sqlite` (pure Go) for easier cross-compilation and no C compiler requirement.

**Architecture:** Direct dependency swap with driver name change from "sqlite3" to "sqlite". Update error handling in `mapSQLiteError` to use the new library's error types. All existing tests validate the migration.

**Tech Stack:** `modernc.org/sqlite` (pure Go SQLite implementation)

---

### Task 1: Swap Dependency in go.mod

**Files:**
- Modify: `go.mod`

**Step 1: Remove old dependency and add new one**

```bash
go get modernc.org/sqlite
go mod edit -droprequire github.com/mattn/go-sqlite3
go mod tidy
```

**Step 2: Verify dependency swap**

Run: `grep -E "sqlite" go.mod`

Expected: Shows `modernc.org/sqlite` but NOT `github.com/mattn/go-sqlite3`.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: swap mattn/go-sqlite3 for modernc.org/sqlite"
```

---

### Task 2: Update Server Database Connection

**Files:**
- Modify: `cmd/arrgod/server.go`

**Step 1: Change import**

Find line 17:
```go
_ "github.com/mattn/go-sqlite3"
```

Replace with:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change driver name**

Find line 88:
```go
db, err := sql.Open("sqlite3", cfg.Database.Path)
```

Replace with:
```go
db, err := sql.Open("sqlite", cfg.Database.Path)
```

**Step 3: Verify build**

Run: `go build ./cmd/arrgod/...`

Expected: Clean build.

**Step 4: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "refactor: update server to use pure Go sqlite driver"
```

---

### Task 3: Update Error Handling in Library

**Files:**
- Modify: `internal/library/content.go`

**Step 1: Change import**

Find line 10:
```go
"github.com/mattn/go-sqlite3"
```

Replace with:
```go
sqlite3 "modernc.org/sqlite/lib"
```

**Step 2: Update mapSQLiteError function**

The modernc.org/sqlite library uses different error types. Replace the entire `mapSQLiteError` function (lines 14-31):

Before:
```go
func mapSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.ExtendedCode {
		case sqlite3.ErrConstraintUnique, sqlite3.ErrConstraintPrimaryKey:
			return ErrDuplicate
		case sqlite3.ErrConstraintForeignKey, sqlite3.ErrConstraintCheck:
			return ErrConstraint
		}
	}
	return err
}
```

After:
```go
func mapSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	// modernc.org/sqlite wraps errors; check error message for constraint violations
	errStr := err.Error()
	if strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "PRIMARY KEY constraint failed") {
		return ErrDuplicate
	}
	if strings.Contains(errStr, "FOREIGN KEY constraint failed") ||
		strings.Contains(errStr, "CHECK constraint failed") {
		return ErrConstraint
	}
	return err
}
```

**Step 3: Remove unused import**

After replacing the function, the `sqlite3` import is no longer needed. Remove line 10:
```go
sqlite3 "modernc.org/sqlite/lib"
```

The `strings` import is already present (line 7).

**Step 4: Verify build**

Run: `go build ./internal/library/...`

Expected: Clean build.

**Step 5: Commit**

```bash
git add internal/library/content.go
git commit -m "refactor: update error handling for pure Go sqlite"
```

---

### Task 4: Update Test Utilities - Library

**Files:**
- Modify: `internal/library/testutil_test.go`

**Step 1: Change import**

Find line 9:
```go
_ "github.com/mattn/go-sqlite3"
```

Replace with:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change driver name**

Find line 17:
```go
db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
```

Replace with:
```go
db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
```

**Step 3: Run library tests**

Run: `go test ./internal/library/... -v`

Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/library/testutil_test.go
git commit -m "test: update library tests for pure Go sqlite"
```

---

### Task 5: Update Test Utilities - Download

**Files:**
- Modify: `internal/download/testutil_test.go`

**Step 1: Change import (line 9)**

```go
_ "github.com/mattn/go-sqlite3"
```
to:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change driver name (line 17)**

```go
db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
```
to:
```go
db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
```

**Step 3: Run download tests**

Run: `go test ./internal/download/... -v`

Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/download/testutil_test.go
git commit -m "test: update download tests for pure Go sqlite"
```

---

### Task 6: Update Test Utilities - Importer

**Files:**
- Modify: `internal/importer/testutil_test.go`

**Step 1: Change import (line 9)**

```go
_ "github.com/mattn/go-sqlite3"
```
to:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change driver name (line 17)**

```go
db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
```
to:
```go
db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
```

**Step 3: Run importer tests**

Run: `go test ./internal/importer/... -v`

Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/importer/testutil_test.go
git commit -m "test: update importer tests for pure Go sqlite"
```

---

### Task 7: Update API Compat Tests

**Files:**
- Modify: `internal/api/compat/radarr_test.go`

**Step 1: Change import (line 13)**

```go
_ "github.com/mattn/go-sqlite3"
```
to:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change driver name (line 62)**

```go
db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
```
to:
```go
db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
```

**Step 3: Run compat tests**

Run: `go test ./internal/api/compat/... -v`

Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/api/compat/radarr_test.go
git commit -m "test: update compat tests for pure Go sqlite"
```

---

### Task 8: Update API v1 Tests

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Change import (line 15)**

```go
_ "github.com/mattn/go-sqlite3"
```
to:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change driver name (line 29)**

```go
db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
```
to:
```go
db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
```

**Step 3: Run API v1 tests**

Run: `go test ./internal/api/v1/... -v`

Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test: update api v1 tests for pure Go sqlite"
```

---

### Task 9: Update Integration Tests

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Change import (line 21)**

```go
_ "github.com/mattn/go-sqlite3"
```
to:
```go
_ "modernc.org/sqlite"
```

**Step 2: Change all driver names**

There are 3 occurrences at lines 130, 412, 569. Change all from:
```go
db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
```
to:
```go
db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
```

**Step 3: Run integration tests**

Run: `go test ./internal/api/v1/... -v -tags=integration`

Note: Integration tests may be skipped if not tagged. Run unit tests at minimum:
Run: `go test ./internal/api/v1/... -v`

Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: update integration tests for pure Go sqlite"
```

---

### Task 10: Add SQLite Compatibility Tests

**Files:**
- Create: `internal/library/sqlite_compat_test.go`

**Step 1: Create compatibility test file**

```go
package library

import (
	"database/sql"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestSQLiteCompat_NullHandling verifies NULL handling works correctly.
func TestSQLiteCompat_NullHandling(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create content with nil optional fields
	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         nil, // NULL
		Title:          "Test Movie",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	err := store.AddContent(c)
	require.NoError(t, err)

	// Retrieve and verify NULL preserved
	retrieved, err := store.GetContent(c.ID)
	require.NoError(t, err)
	assert.Nil(t, retrieved.TMDBID)
}

// TestSQLiteCompat_TypeAffinity verifies type coercion works.
func TestSQLiteCompat_TypeAffinity(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Store with int64 ID
	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(12345)),
		Title:          "Type Test",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	err := store.AddContent(c)
	require.NoError(t, err)

	retrieved, err := store.GetContent(c.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), *retrieved.TMDBID)
}

// TestSQLiteCompat_ConcurrentWrites verifies concurrent operations.
func TestSQLiteCompat_ConcurrentWrites(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := &Content{
				Type:           ContentTypeMovie,
				Title:          "Concurrent Test",
				Year:           2024 + idx,
				Status:         StatusWanted,
				QualityProfile: "hd",
				RootPath:       "/movies",
			}
			if err := store.AddContent(c); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent write failed: %v", err)
	}

	// Verify all 10 were inserted
	contents, err := store.ListContent(nil, nil, 20, 0)
	require.NoError(t, err)
	assert.Len(t, contents, 10)
}

// TestSQLiteCompat_ConstraintErrors verifies error mapping works.
func TestSQLiteCompat_ConstraintErrors(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add first content
	c1 := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(999)),
		Title:          "Original",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, store.AddContent(c1))

	// Try to add duplicate TMDB ID - should get ErrDuplicate
	c2 := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(999)), // Same TMDB ID
		Title:          "Duplicate",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	err := store.AddContent(c2)
	assert.ErrorIs(t, err, ErrDuplicate)
}

// TestSQLiteCompat_TransactionIsolation verifies transactions work.
func TestSQLiteCompat_TransactionIsolation(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Start transaction
	tx, err := store.Begin()
	require.NoError(t, err)

	// Add content in transaction
	c := &Content{
		Type:           ContentTypeMovie,
		Title:          "Transaction Test",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	err = tx.AddContent(c)
	require.NoError(t, err)

	// Before commit, content should not be visible outside transaction
	contents, err := store.ListContent(nil, nil, 10, 0)
	require.NoError(t, err)
	assert.Len(t, contents, 0)

	// Commit
	require.NoError(t, tx.Commit())

	// After commit, content should be visible
	contents, err = store.ListContent(nil, nil, 10, 0)
	require.NoError(t, err)
	assert.Len(t, contents, 1)
}
```

**Step 2: Run compatibility tests**

Run: `go test ./internal/library/... -v -run TestSQLiteCompat`

Expected: All 5 tests pass.

**Step 3: Commit**

```bash
git add internal/library/sqlite_compat_test.go
git commit -m "test: add sqlite compatibility tests for driver migration"
```

---

### Task 11: Final Verification

**Step 1: Run full test suite**

Run: `task test`

Expected: All tests pass.

**Step 2: Run linter**

Run: `task lint`

Expected: No lint errors.

**Step 3: Verify no mattn/go-sqlite3 references remain**

Run: `grep -r "mattn/go-sqlite3" --include="*.go" .`

Expected: No matches (only docs/plans may have references).

**Step 4: Verify build**

Run: `go build ./...`

Expected: Clean build.

**Step 5: Smoke test server startup**

Run: `./arrgod --help` or start server briefly if database path configured.

Expected: Server starts without errors.

---

## Summary

After completing these tasks:

1. `modernc.org/sqlite` replaces `mattn/go-sqlite3` (no CGo required)
2. Driver name changed from `"sqlite3"` to `"sqlite"` everywhere
3. Error handling updated to string-based constraint detection
4. All existing tests validate the migration
5. New compatibility tests verify edge cases
6. Cross-compilation is now simpler (pure Go)
