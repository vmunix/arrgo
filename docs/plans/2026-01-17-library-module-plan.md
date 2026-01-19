# Library Module Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement full CRUD operations for Content, Episode, and File entities with transaction support and custom error types.

**Architecture:** Data access layer over SQLite. `Store` wraps `*sql.DB`, `Tx` wraps `*sql.Tx`. Both share CRUD logic via internal helper functions that accept a `querier` interface. Error mapping converts SQLite errors to custom sentinel errors.

**Tech Stack:** Go 1.21+, database/sql, github.com/mattn/go-sqlite3

---

### Task 1: Error Types

**Files:**
- Create: `internal/library/errors.go`
- Create: `internal/library/errors_test.go`

**Step 1: Write the test file**

```go
// internal/library/errors_test.go
package library

import (
	"errors"
	"testing"
)

func TestErrors_AreDistinct(t *testing.T) {
	if errors.Is(ErrNotFound, ErrDuplicate) {
		t.Error("ErrNotFound should not match ErrDuplicate")
	}
	if errors.Is(ErrNotFound, ErrConstraint) {
		t.Error("ErrNotFound should not match ErrConstraint")
	}
	if errors.Is(ErrDuplicate, ErrConstraint) {
		t.Error("ErrDuplicate should not match ErrConstraint")
	}
}

func TestErrors_CanBeWrapped(t *testing.T) {
	wrapped := fmt.Errorf("content 123: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("wrapped error should match ErrNotFound")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/library/... -run TestErrors`
Expected: FAIL (ErrNotFound undefined)

**Step 3: Write the implementation**

```go
// internal/library/errors.go
package library

import "errors"

var (
	// ErrNotFound indicates the requested entity doesn't exist.
	ErrNotFound = errors.New("not found")

	// ErrDuplicate indicates a unique constraint violation.
	ErrDuplicate = errors.New("duplicate entry")

	// ErrConstraint indicates a foreign key or check constraint violation.
	ErrConstraint = errors.New("constraint violation")
)
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/library/... -run TestErrors`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/library/errors.go internal/library/errors_test.go
git commit -m "feat(library): add custom error types"
```

---

### Task 2: Filter Structs

**Files:**
- Create: `internal/library/filter.go`

**Step 1: Create the filter types**

```go
// internal/library/filter.go
package library

// ContentFilter specifies criteria for listing content.
type ContentFilter struct {
	Type           *ContentType
	Status         *ContentStatus
	QualityProfile *string
	TMDBID         *int64
	TVDBID         *int64
	Limit          int // 0 = no limit
	Offset         int
}

// EpisodeFilter specifies criteria for listing episodes.
type EpisodeFilter struct {
	ContentID *int64
	Season    *int
	Status    *ContentStatus
	Limit     int
	Offset    int
}

// FileFilter specifies criteria for listing files.
type FileFilter struct {
	ContentID *int64
	EpisodeID *int64
	Quality   *string
	Limit     int
	Offset    int
}
```

**Step 2: Verify it compiles**

Run: `go build ./internal/library/...`
Expected: Success (no output)

**Step 3: Commit**

```bash
git add internal/library/filter.go
git commit -m "feat(library): add filter structs for list operations"
```

---

### Task 3: Store and Transaction Infrastructure

**Files:**
- Modify: `internal/library/library.go` (remove stub methods, keep types)
- Create: `internal/library/store.go`

**Step 1: Clean up library.go - remove stub methods, keep only types**

Edit `internal/library/library.go` to remove everything from line 64 onwards (Store struct and methods). Keep only the imports, types, and constants.

The file should end after the File struct definition (line 62).

**Step 2: Create store.go with Store, Tx, and querier**

```go
// internal/library/store.go
package library

import (
	"database/sql"
	"fmt"
)

// querier abstracts *sql.DB and *sql.Tx for shared query logic.
type querier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

// Store provides access to content data.
type Store struct {
	db *sql.DB
}

// NewStore creates a new library store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Begin starts a transaction.
func (s *Store) Begin() (*Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &Tx{tx: tx}, nil
}

// Tx wraps a database transaction with the same methods as Store.
type Tx struct {
	tx *sql.Tx
}

// Commit commits the transaction.
func (t *Tx) Commit() error {
	return t.tx.Commit()
}

// Rollback aborts the transaction.
func (t *Tx) Rollback() error {
	return t.tx.Rollback()
}
```

**Step 3: Verify it compiles**

Run: `go build ./internal/library/...`
Expected: Success

**Step 4: Commit**

```bash
git add internal/library/library.go internal/library/store.go
git commit -m "feat(library): add Store/Tx infrastructure with querier interface"
```

---

### Task 4: Test Helpers and Schema Embedding

**Files:**
- Create: `internal/library/testutil_test.go`

**Step 1: Create test utilities with embedded schema**

```go
// internal/library/testutil_test.go
package library

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

// helper to create pointer to value
func ptr[T any](v T) *T {
	return &v
}
```

**Step 2: Create testdata directory and schema file**

Create `internal/library/testdata/schema.sql` with the relevant tables from migrations:

```sql
-- Test schema for library module
PRAGMA foreign_keys = ON;

CREATE TABLE content (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    type            TEXT NOT NULL CHECK (type IN ('movie', 'series')),
    tmdb_id         INTEGER,
    tvdb_id         INTEGER,
    title           TEXT NOT NULL,
    year            INTEGER,
    status          TEXT NOT NULL DEFAULT 'wanted' CHECK (status IN ('wanted', 'available', 'unmonitored')),
    quality_profile TEXT NOT NULL DEFAULT 'hd',
    root_path       TEXT NOT NULL,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_content_type ON content(type);
CREATE INDEX idx_content_status ON content(status);
CREATE INDEX idx_content_tmdb ON content(tmdb_id);
CREATE INDEX idx_content_tvdb ON content(tvdb_id);

CREATE TABLE episodes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    season          INTEGER NOT NULL,
    episode         INTEGER NOT NULL,
    title           TEXT,
    status          TEXT NOT NULL DEFAULT 'wanted' CHECK (status IN ('wanted', 'available', 'unmonitored')),
    air_date        DATE,
    UNIQUE(content_id, season, episode)
);

CREATE INDEX idx_episodes_content ON episodes(content_id);
CREATE INDEX idx_episodes_status ON episodes(status);

CREATE TABLE files (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    path            TEXT NOT NULL UNIQUE,
    size_bytes      INTEGER,
    quality         TEXT,
    source          TEXT,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_files_content ON files(content_id);
CREATE INDEX idx_files_episode ON files(episode_id);
```

**Step 3: Verify the test helper compiles**

Run: `go build ./internal/library/...`
Expected: Success

**Step 4: Commit**

```bash
git add internal/library/testutil_test.go internal/library/testdata/schema.sql
git commit -m "test(library): add test helpers with embedded schema"
```

---

### Task 5: Content CRUD Implementation

**Files:**
- Create: `internal/library/content.go`
- Create: `internal/library/store_test.go`

**Step 5a: Write AddContent test**

```go
// internal/library/store_test.go
package library

import (
	"errors"
	"testing"
	"time"
)

func TestStore_AddContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(12345)),
		Title:          "Test Movie",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	err := store.AddContent(c)
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}
	if c.ID == 0 {
		t.Error("expected ID to be set")
	}
	if c.AddedAt.IsZero() {
		t.Error("expected AddedAt to be set")
	}
}
```

Run: `go test -v ./internal/library/... -run TestStore_AddContent`
Expected: FAIL (method not implemented)

**Step 5b: Write AddContent implementation**

```go
// internal/library/content.go
package library

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

// mapSQLiteError converts SQLite errors to custom error types.
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

func addContent(q querier, c *Content) error {
	now := time.Now()
	result, err := q.Exec(`
		INSERT INTO content (type, tmdb_id, tvdb_id, title, year, status, quality_profile, root_path, added_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Type, c.TMDBID, c.TVDBID, c.Title, c.Year, c.Status, c.QualityProfile, c.RootPath, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert content: %w", mapSQLiteError(err))
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	c.ID = id
	c.AddedAt = now
	c.UpdatedAt = now
	return nil
}

func (s *Store) AddContent(c *Content) error {
	return addContent(s.db, c)
}

func (t *Tx) AddContent(c *Content) error {
	return addContent(t.tx, c)
}
```

Run: `go test -v ./internal/library/... -run TestStore_AddContent`
Expected: PASS

**Step 5c: Write GetContent test**

Add to `store_test.go`:

```go
func TestStore_GetContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add content first
	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(12345)),
		Title:          "Test Movie",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Test get
	got, err := store.GetContent(c.ID)
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}
	if got.Title != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got %q", got.Title)
	}
	if got.TMDBID == nil || *got.TMDBID != 12345 {
		t.Errorf("expected TMDBID 12345, got %v", got.TMDBID)
	}
}

func TestStore_GetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetContent(999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

Run: `go test -v ./internal/library/... -run TestStore_GetContent`
Expected: FAIL

**Step 5d: Write GetContent implementation**

Add to `content.go`:

```go
func getContent(q querier, id int64) (*Content, error) {
	c := &Content{}
	err := q.QueryRow(`
		SELECT id, type, tmdb_id, tvdb_id, title, year, status, quality_profile, root_path, added_at, updated_at
		FROM content WHERE id = ?`, id,
	).Scan(&c.ID, &c.Type, &c.TMDBID, &c.TVDBID, &c.Title, &c.Year, &c.Status, &c.QualityProfile, &c.RootPath, &c.AddedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get content %d: %w", id, mapSQLiteError(err))
	}
	return c, nil
}

func (s *Store) GetContent(id int64) (*Content, error) {
	return getContent(s.db, id)
}

func (t *Tx) GetContent(id int64) (*Content, error) {
	return getContent(t.tx, id)
}
```

Run: `go test -v ./internal/library/... -run TestStore_GetContent`
Expected: PASS

**Step 5e: Write ListContent test**

Add to `store_test.go`:

```go
func TestStore_ListContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add test data
	movies := []string{"Movie A", "Movie B", "Movie C"}
	for _, title := range movies {
		c := &Content{Type: ContentTypeMovie, Title: title, Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
		if err := store.AddContent(c); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	series := &Content{Type: ContentTypeSeries, Title: "Series A", Year: 2024, Status: StatusAvailable, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Test: list all
	results, total, err := store.ListContent(ContentFilter{})
	if err != nil {
		t.Fatalf("ListContent failed: %v", err)
	}
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}

	// Test: filter by type
	results, total, err = store.ListContent(ContentFilter{Type: ptr(ContentTypeMovie)})
	if err != nil {
		t.Fatalf("ListContent failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 movies, got %d", total)
	}

	// Test: pagination
	results, total, err = store.ListContent(ContentFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListContent failed: %v", err)
	}
	if total != 4 {
		t.Errorf("expected total 4, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit, got %d", len(results))
	}
}
```

Run: `go test -v ./internal/library/... -run TestStore_ListContent`
Expected: FAIL

**Step 5f: Write ListContent implementation**

Add to `content.go`:

```go
func listContent(q querier, f ContentFilter) ([]*Content, int, error) {
	// Build WHERE clause
	var conditions []string
	var args []any

	if f.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *f.Type)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}
	if f.QualityProfile != nil {
		conditions = append(conditions, "quality_profile = ?")
		args = append(args, *f.QualityProfile)
	}
	if f.TMDBID != nil {
		conditions = append(conditions, "tmdb_id = ?")
		args = append(args, *f.TMDBID)
	}
	if f.TVDBID != nil {
		conditions = append(conditions, "tvdb_id = ?")
		args = append(args, *f.TVDBID)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM content " + whereClause
	if err := q.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count content: %w", err)
	}

	// Get results
	query := "SELECT id, type, tmdb_id, tvdb_id, title, year, status, quality_profile, root_path, added_at, updated_at FROM content " + whereClause + " ORDER BY id"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list content: %w", err)
	}
	defer rows.Close()

	var results []*Content
	for rows.Next() {
		c := &Content{}
		if err := rows.Scan(&c.ID, &c.Type, &c.TMDBID, &c.TVDBID, &c.Title, &c.Year, &c.Status, &c.QualityProfile, &c.RootPath, &c.AddedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan content: %w", err)
		}
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate content: %w", err)
	}

	return results, total, nil
}

func (s *Store) ListContent(f ContentFilter) ([]*Content, int, error) {
	return listContent(s.db, f)
}

func (t *Tx) ListContent(f ContentFilter) ([]*Content, int, error) {
	return listContent(t.tx, f)
}
```

Run: `go test -v ./internal/library/... -run TestStore_ListContent`
Expected: PASS

**Step 5g: Write UpdateContent test**

Add to `store_test.go`:

```go
func TestStore_UpdateContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{Type: ContentTypeMovie, Title: "Original", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("setup: %v", err)
	}

	c.Title = "Updated"
	c.Status = StatusAvailable
	if err := store.UpdateContent(c); err != nil {
		t.Fatalf("UpdateContent failed: %v", err)
	}

	got, _ := store.GetContent(c.ID)
	if got.Title != "Updated" {
		t.Errorf("expected title 'Updated', got %q", got.Title)
	}
	if got.Status != StatusAvailable {
		t.Errorf("expected status Available, got %v", got.Status)
	}
}

func TestStore_UpdateContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{ID: 999, Type: ContentTypeMovie, Title: "Test", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	err := store.UpdateContent(c)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

Run: `go test -v ./internal/library/... -run TestStore_UpdateContent`
Expected: FAIL

**Step 5h: Write UpdateContent implementation**

Add to `content.go`:

```go
func updateContent(q querier, c *Content) error {
	now := time.Now()
	result, err := q.Exec(`
		UPDATE content SET type = ?, tmdb_id = ?, tvdb_id = ?, title = ?, year = ?, status = ?, quality_profile = ?, root_path = ?, updated_at = ?
		WHERE id = ?`,
		c.Type, c.TMDBID, c.TVDBID, c.Title, c.Year, c.Status, c.QualityProfile, c.RootPath, now, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update content %d: %w", c.ID, mapSQLiteError(err))
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update content %d: %w", c.ID, ErrNotFound)
	}
	c.UpdatedAt = now
	return nil
}

func (s *Store) UpdateContent(c *Content) error {
	return updateContent(s.db, c)
}

func (t *Tx) UpdateContent(c *Content) error {
	return updateContent(t.tx, c)
}
```

Run: `go test -v ./internal/library/... -run TestStore_UpdateContent`
Expected: PASS

**Step 5i: Write DeleteContent test**

Add to `store_test.go`:

```go
func TestStore_DeleteContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{Type: ContentTypeMovie, Title: "To Delete", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := store.DeleteContent(c.ID); err != nil {
		t.Fatalf("DeleteContent failed: %v", err)
	}

	_, err := store.GetContent(c.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_DeleteContent_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Deleting non-existent content should not error
	err := store.DeleteContent(999)
	if err != nil {
		t.Errorf("expected nil for non-existent delete, got %v", err)
	}
}
```

Run: `go test -v ./internal/library/... -run TestStore_DeleteContent`
Expected: FAIL

**Step 5j: Write DeleteContent implementation**

Add to `content.go`:

```go
func deleteContent(q querier, id int64) error {
	_, err := q.Exec("DELETE FROM content WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete content %d: %w", id, mapSQLiteError(err))
	}
	// Idempotent: don't error if not found
	return nil
}

func (s *Store) DeleteContent(id int64) error {
	return deleteContent(s.db, id)
}

func (t *Tx) DeleteContent(id int64) error {
	return deleteContent(t.tx, id)
}
```

Run: `go test -v ./internal/library/... -run TestStore_DeleteContent`
Expected: PASS

**Step 5k: Run all content tests and commit**

Run: `go test -v ./internal/library/... -run "TestStore_.*Content"`
Expected: All PASS

```bash
git add internal/library/content.go internal/library/store_test.go
git commit -m "feat(library): implement Content CRUD operations"
```

---

### Task 6: Episode CRUD Implementation

**Files:**
- Create: `internal/library/episode.go`
- Create: `internal/library/episode_test.go`

**Step 6a: Write Episode tests**

```go
// internal/library/episode_test.go
package library

import (
	"errors"
	"testing"
	"time"
)

func TestStore_AddEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series first
	series := &Content{Type: ContentTypeSeries, Title: "Test Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}

	airDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	ep := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
		AirDate:   &airDate,
	}

	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("AddEpisode failed: %v", err)
	}
	if ep.ID == 0 {
		t.Error("expected ID to be set")
	}
}

func TestStore_AddEpisode_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	series := &Content{Type: ContentTypeSeries, Title: "Test Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup: %v", err)
	}

	dup := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Duplicate", Status: StatusWanted}
	err := store.AddEpisode(dup)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}
}

func TestStore_GetEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	series := &Content{Type: ContentTypeSeries, Title: "Test Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := store.GetEpisode(ep.ID)
	if err != nil {
		t.Fatalf("GetEpisode failed: %v", err)
	}
	if got.Title != "Pilot" {
		t.Errorf("expected title 'Pilot', got %q", got.Title)
	}
}

func TestStore_GetEpisode_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetEpisode(999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_ListEpisodes(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	series := &Content{Type: ContentTypeSeries, Title: "Test Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Add episodes
	for s := 1; s <= 2; s++ {
		for e := 1; e <= 3; e++ {
			ep := &Episode{ContentID: series.ID, Season: s, Episode: e, Title: "Episode", Status: StatusWanted}
			if err := store.AddEpisode(ep); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}
	}

	// List all
	results, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if total != 6 {
		t.Errorf("expected total 6, got %d", total)
	}

	// Filter by season
	results, total, err = store.ListEpisodes(EpisodeFilter{ContentID: &series.ID, Season: ptr(1)})
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 for season 1, got %d", total)
	}

	// Pagination
	results, total, err = store.ListEpisodes(EpisodeFilter{ContentID: &series.ID, Limit: 2})
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if total != 6 {
		t.Errorf("expected total 6, got %d", total)
	}
}

func TestStore_UpdateEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	series := &Content{Type: ContentTypeSeries, Title: "Test Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Original", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ep.Title = "Updated"
	ep.Status = StatusAvailable
	if err := store.UpdateEpisode(ep); err != nil {
		t.Fatalf("UpdateEpisode failed: %v", err)
	}

	got, _ := store.GetEpisode(ep.ID)
	if got.Title != "Updated" {
		t.Errorf("expected title 'Updated', got %q", got.Title)
	}
	if got.Status != StatusAvailable {
		t.Errorf("expected status Available, got %v", got.Status)
	}
}

func TestStore_DeleteEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	series := &Content{Type: ContentTypeSeries, Title: "Test Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "To Delete", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := store.DeleteEpisode(ep.ID); err != nil {
		t.Fatalf("DeleteEpisode failed: %v", err)
	}

	_, err := store.GetEpisode(ep.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
```

Run: `go test -v ./internal/library/... -run "TestStore_.*Episode"`
Expected: FAIL

**Step 6b: Write Episode implementation**

```go
// internal/library/episode.go
package library

import (
	"fmt"
	"strings"
)

func addEpisode(q querier, e *Episode) error {
	result, err := q.Exec(`
		INSERT INTO episodes (content_id, season, episode, title, status, air_date)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.ContentID, e.Season, e.Episode, e.Title, e.Status, e.AirDate,
	)
	if err != nil {
		return fmt.Errorf("insert episode: %w", mapSQLiteError(err))
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	e.ID = id
	return nil
}

func (s *Store) AddEpisode(e *Episode) error {
	return addEpisode(s.db, e)
}

func (t *Tx) AddEpisode(e *Episode) error {
	return addEpisode(t.tx, e)
}

func getEpisode(q querier, id int64) (*Episode, error) {
	e := &Episode{}
	err := q.QueryRow(`
		SELECT id, content_id, season, episode, title, status, air_date
		FROM episodes WHERE id = ?`, id,
	).Scan(&e.ID, &e.ContentID, &e.Season, &e.Episode, &e.Title, &e.Status, &e.AirDate)
	if err != nil {
		return nil, fmt.Errorf("get episode %d: %w", id, mapSQLiteError(err))
	}
	return e, nil
}

func (s *Store) GetEpisode(id int64) (*Episode, error) {
	return getEpisode(s.db, id)
}

func (t *Tx) GetEpisode(id int64) (*Episode, error) {
	return getEpisode(t.tx, id)
}

func listEpisodes(q querier, f EpisodeFilter) ([]*Episode, int, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.Season != nil {
		conditions = append(conditions, "season = ?")
		args = append(args, *f.Season)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := q.QueryRow("SELECT COUNT(*) FROM episodes "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count episodes: %w", err)
	}

	query := "SELECT id, content_id, season, episode, title, status, air_date FROM episodes " + whereClause + " ORDER BY season, episode"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()

	var results []*Episode
	for rows.Next() {
		e := &Episode{}
		if err := rows.Scan(&e.ID, &e.ContentID, &e.Season, &e.Episode, &e.Title, &e.Status, &e.AirDate); err != nil {
			return nil, 0, fmt.Errorf("scan episode: %w", err)
		}
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate episodes: %w", err)
	}

	return results, total, nil
}

func (s *Store) ListEpisodes(f EpisodeFilter) ([]*Episode, int, error) {
	return listEpisodes(s.db, f)
}

func (t *Tx) ListEpisodes(f EpisodeFilter) ([]*Episode, int, error) {
	return listEpisodes(t.tx, f)
}

func updateEpisode(q querier, e *Episode) error {
	result, err := q.Exec(`
		UPDATE episodes SET content_id = ?, season = ?, episode = ?, title = ?, status = ?, air_date = ?
		WHERE id = ?`,
		e.ContentID, e.Season, e.Episode, e.Title, e.Status, e.AirDate, e.ID,
	)
	if err != nil {
		return fmt.Errorf("update episode %d: %w", e.ID, mapSQLiteError(err))
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update episode %d: %w", e.ID, ErrNotFound)
	}
	return nil
}

func (s *Store) UpdateEpisode(e *Episode) error {
	return updateEpisode(s.db, e)
}

func (t *Tx) UpdateEpisode(e *Episode) error {
	return updateEpisode(t.tx, e)
}

func deleteEpisode(q querier, id int64) error {
	_, err := q.Exec("DELETE FROM episodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete episode %d: %w", id, mapSQLiteError(err))
	}
	return nil
}

func (s *Store) DeleteEpisode(id int64) error {
	return deleteEpisode(s.db, id)
}

func (t *Tx) DeleteEpisode(id int64) error {
	return deleteEpisode(t.tx, id)
}
```

Run: `go test -v ./internal/library/... -run "TestStore_.*Episode"`
Expected: All PASS

**Step 6c: Commit**

```bash
git add internal/library/episode.go internal/library/episode_test.go
git commit -m "feat(library): implement Episode CRUD operations"
```

---

### Task 7: File CRUD Implementation

**Files:**
- Create: `internal/library/file.go`
- Create: `internal/library/file_test.go`

**Step 7a: Write File tests**

```go
// internal/library/file_test.go
package library

import (
	"errors"
	"testing"
)

func TestStore_AddFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	movie := &Content{Type: ContentTypeMovie, Title: "Test Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup: %v", err)
	}

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Test Movie (2024)/movie.mkv",
		SizeBytes: 5000000000,
		Quality:   "1080p",
		Source:    "bluray",
	}

	if err := store.AddFile(f); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if f.ID == 0 {
		t.Error("expected ID to be set")
	}
	if f.AddedAt.IsZero() {
		t.Error("expected AddedAt to be set")
	}
}

func TestStore_AddFile_DuplicatePath(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	movie := &Content{Type: ContentTypeMovie, Title: "Test Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup: %v", err)
	}

	f := &File{ContentID: movie.ID, Path: "/movies/test.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup: %v", err)
	}

	dup := &File{ContentID: movie.ID, Path: "/movies/test.mkv", SizeBytes: 2000, Quality: "720p", Source: "hdtv"}
	err := store.AddFile(dup)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}
}

func TestStore_GetFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	movie := &Content{Type: ContentTypeMovie, Title: "Test Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f := &File{ContentID: movie.ID, Path: "/movies/test.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := store.GetFile(f.ID)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	if got.Path != "/movies/test.mkv" {
		t.Errorf("expected path '/movies/test.mkv', got %q", got.Path)
	}
	if got.Quality != "1080p" {
		t.Errorf("expected quality '1080p', got %q", got.Quality)
	}
}

func TestStore_GetFile_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetFile(999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_ListFiles(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	movie := &Content{Type: ContentTypeMovie, Title: "Test Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup: %v", err)
	}

	for i := 1; i <= 3; i++ {
		f := &File{ContentID: movie.ID, Path: fmt.Sprintf("/movies/file%d.mkv", i), SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
		if err := store.AddFile(f); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	results, total, err := store.ListFiles(FileFilter{ContentID: &movie.ID})
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestStore_UpdateFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	movie := &Content{Type: ContentTypeMovie, Title: "Test Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f := &File{ContentID: movie.ID, Path: "/movies/test.mkv", SizeBytes: 1000, Quality: "720p", Source: "hdtv"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup: %v", err)
	}

	f.Quality = "1080p"
	f.Source = "bluray"
	if err := store.UpdateFile(f); err != nil {
		t.Fatalf("UpdateFile failed: %v", err)
	}

	got, _ := store.GetFile(f.ID)
	if got.Quality != "1080p" {
		t.Errorf("expected quality '1080p', got %q", got.Quality)
	}
}

func TestStore_DeleteFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	movie := &Content{Type: ContentTypeMovie, Title: "Test Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f := &File{ContentID: movie.ID, Path: "/movies/test.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := store.DeleteFile(f.ID); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	_, err := store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
```

Run: `go test -v ./internal/library/... -run "TestStore_.*File"`
Expected: FAIL

**Step 7b: Write File implementation**

```go
// internal/library/file.go
package library

import (
	"fmt"
	"strings"
	"time"
)

func addFile(q querier, f *File) error {
	now := time.Now()
	result, err := q.Exec(`
		INSERT INTO files (content_id, episode_id, path, size_bytes, quality, source, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ContentID, f.EpisodeID, f.Path, f.SizeBytes, f.Quality, f.Source, now,
	)
	if err != nil {
		return fmt.Errorf("insert file: %w", mapSQLiteError(err))
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	f.ID = id
	f.AddedAt = now
	return nil
}

func (s *Store) AddFile(f *File) error {
	return addFile(s.db, f)
}

func (t *Tx) AddFile(f *File) error {
	return addFile(t.tx, f)
}

func getFile(q querier, id int64) (*File, error) {
	f := &File{}
	err := q.QueryRow(`
		SELECT id, content_id, episode_id, path, size_bytes, quality, source, added_at
		FROM files WHERE id = ?`, id,
	).Scan(&f.ID, &f.ContentID, &f.EpisodeID, &f.Path, &f.SizeBytes, &f.Quality, &f.Source, &f.AddedAt)
	if err != nil {
		return nil, fmt.Errorf("get file %d: %w", id, mapSQLiteError(err))
	}
	return f, nil
}

func (s *Store) GetFile(id int64) (*File, error) {
	return getFile(s.db, id)
}

func (t *Tx) GetFile(id int64) (*File, error) {
	return getFile(t.tx, id)
}

func listFiles(q querier, f FileFilter) ([]*File, int, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Quality != nil {
		conditions = append(conditions, "quality = ?")
		args = append(args, *f.Quality)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := q.QueryRow("SELECT COUNT(*) FROM files "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count files: %w", err)
	}

	query := "SELECT id, content_id, episode_id, path, size_bytes, quality, source, added_at FROM files " + whereClause + " ORDER BY id"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var results []*File
	for rows.Next() {
		file := &File{}
		if err := rows.Scan(&file.ID, &file.ContentID, &file.EpisodeID, &file.Path, &file.SizeBytes, &file.Quality, &file.Source, &file.AddedAt); err != nil {
			return nil, 0, fmt.Errorf("scan file: %w", err)
		}
		results = append(results, file)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate files: %w", err)
	}

	return results, total, nil
}

func (s *Store) ListFiles(f FileFilter) ([]*File, int, error) {
	return listFiles(s.db, f)
}

func (t *Tx) ListFiles(f FileFilter) ([]*File, int, error) {
	return listFiles(t.tx, f)
}

func updateFile(q querier, f *File) error {
	result, err := q.Exec(`
		UPDATE files SET content_id = ?, episode_id = ?, path = ?, size_bytes = ?, quality = ?, source = ?
		WHERE id = ?`,
		f.ContentID, f.EpisodeID, f.Path, f.SizeBytes, f.Quality, f.Source, f.ID,
	)
	if err != nil {
		return fmt.Errorf("update file %d: %w", f.ID, mapSQLiteError(err))
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update file %d: %w", f.ID, ErrNotFound)
	}
	return nil
}

func (s *Store) UpdateFile(f *File) error {
	return updateFile(s.db, f)
}

func (t *Tx) UpdateFile(f *File) error {
	return updateFile(t.tx, f)
}

func deleteFile(q querier, id int64) error {
	_, err := q.Exec("DELETE FROM files WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete file %d: %w", id, mapSQLiteError(err))
	}
	return nil
}

func (s *Store) DeleteFile(id int64) error {
	return deleteFile(s.db, id)
}

func (t *Tx) DeleteFile(id int64) error {
	return deleteFile(t.tx, id)
}
```

Run: `go test -v ./internal/library/... -run "TestStore_.*File"`
Expected: All PASS

**Step 7c: Commit**

```bash
git add internal/library/file.go internal/library/file_test.go
git commit -m "feat(library): implement File CRUD operations"
```

---

### Task 8: Transaction Tests

**Files:**
- Create: `internal/library/tx_test.go`

**Step 8a: Write transaction tests**

```go
// internal/library/tx_test.go
package library

import (
	"errors"
	"testing"
)

func TestTx_Commit(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	c := &Content{Type: ContentTypeMovie, Title: "TX Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := tx.AddContent(c); err != nil {
		t.Fatalf("AddContent in tx failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Should be visible outside transaction
	got, err := store.GetContent(c.ID)
	if err != nil {
		t.Fatalf("GetContent after commit failed: %v", err)
	}
	if got.Title != "TX Movie" {
		t.Errorf("expected title 'TX Movie', got %q", got.Title)
	}
}

func TestTx_Rollback(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	c := &Content{Type: ContentTypeMovie, Title: "TX Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := tx.AddContent(c); err != nil {
		t.Fatalf("AddContent in tx failed: %v", err)
	}
	id := c.ID

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Should NOT be visible outside transaction
	_, err = store.GetContent(id)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after rollback, got %v", err)
	}
}

func TestTx_MultipleOperations(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Add series with episodes in one transaction
	series := &Content{Type: ContentTypeSeries, Title: "TX Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := tx.AddContent(series); err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	for i := 1; i <= 3; i++ {
		ep := &Episode{ContentID: series.ID, Season: 1, Episode: i, Title: "Episode", Status: StatusWanted}
		if err := tx.AddEpisode(ep); err != nil {
			t.Fatalf("AddEpisode failed: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify all episodes exist
	eps, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 episodes, got %d", total)
	}
	if len(eps) != 3 {
		t.Errorf("expected 3 results, got %d", len(eps))
	}
}

func TestTx_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/series/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Delete content - should cascade
	if err := store.DeleteContent(series.ID); err != nil {
		t.Fatalf("DeleteContent failed: %v", err)
	}

	// Episode should be gone
	_, err := store.GetEpisode(ep.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected episode ErrNotFound after cascade, got %v", err)
	}

	// File should be gone
	_, err = store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected file ErrNotFound after cascade, got %v", err)
	}
}
```

Run: `go test -v ./internal/library/... -run "TestTx_"`
Expected: All PASS

**Step 8b: Commit**

```bash
git add internal/library/tx_test.go
git commit -m "test(library): add transaction tests including cascade delete"
```

---

### Task 9: Final Verification

**Step 1: Run all tests**

Run: `go test -v ./internal/library/...`
Expected: All PASS

**Step 2: Run linter**

Run: `golangci-lint run ./internal/library/...`
Expected: No issues

**Step 3: Verify build**

Run: `go build ./...`
Expected: Success

**Step 4: Final commit if any fixes needed**

If linter fixes needed, commit them:
```bash
git add -A
git commit -m "fix(library): resolve linter issues"
```
