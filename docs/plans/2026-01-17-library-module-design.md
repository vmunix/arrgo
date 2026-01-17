# Library Module Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:writing-plans to create the implementation plan from this design.

**Goal:** Implement the library module providing full CRUD operations for content (movies/series), episodes, and files with transaction support.

**Architecture:** Data access layer over SQLite using the existing schema from `migrations/001_initial.sql`. Exposes `Store` for regular operations and `Tx` for transactional operations, both sharing the same method signatures via an internal `querier` interface.

**Tech Stack:** Go, database/sql, github.com/mattn/go-sqlite3

---

## Error Types

```go
// errors.go
package library

import "errors"

var (
    // ErrNotFound indicates the requested entity doesn't exist
    ErrNotFound = errors.New("not found")

    // ErrDuplicate indicates a unique constraint violation
    ErrDuplicate = errors.New("duplicate entry")

    // ErrConstraint indicates a foreign key or check constraint violation
    ErrConstraint = errors.New("constraint violation")
)
```

SQLite error codes are detected and mapped to these custom errors. Callers use `errors.Is()` to check error types.

---

## Filter Structs

```go
// filter.go
package library

// ContentFilter specifies criteria for listing content.
type ContentFilter struct {
    Type           *ContentType
    Status         *ContentStatus
    QualityProfile *string
    TMDBID         *int64
    TVDBID         *int64
    Limit          int  // 0 = no limit
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

---

## Transaction Support

```go
// store.go
package library

import "database/sql"

// querier abstracts *sql.DB and *sql.Tx
type querier interface {
    QueryRow(query string, args ...any) *sql.Row
    Query(query string, args ...any) (*sql.Rows, error)
    Exec(query string, args ...any) (sql.Result, error)
}

// Store provides access to content data.
type Store struct {
    db *sql.DB
}

func NewStore(db *sql.DB) *Store {
    return &Store{db: db}
}

func (s *Store) Begin() (*Tx, error) {
    tx, err := s.db.Begin()
    if err != nil {
        return nil, fmt.Errorf("begin transaction: %w", err)
    }
    return &Tx{tx: tx}, nil
}

// Tx wraps a database transaction.
type Tx struct {
    tx *sql.Tx
}

func (t *Tx) Commit() error   { return t.tx.Commit() }
func (t *Tx) Rollback() error { return t.tx.Rollback() }
```

Both `Store` and `Tx` implement the same CRUD methods using shared helper functions that accept a `querier`.

---

## Method Signatures

Both `Store` and `Tx` provide these methods:

### Content CRUD

```go
AddContent(c *Content) error              // Sets c.ID on success
GetContent(id int64) (*Content, error)    // Returns ErrNotFound if missing
ListContent(f ContentFilter) ([]*Content, int, error)
UpdateContent(c *Content) error           // Updates all fields, sets UpdatedAt
DeleteContent(id int64) error             // Cascades to episodes/files via FK
```

### Episode CRUD

```go
AddEpisode(e *Episode) error
GetEpisode(id int64) (*Episode, error)
ListEpisodes(f EpisodeFilter) ([]*Episode, int, error)
UpdateEpisode(e *Episode) error
DeleteEpisode(id int64) error
```

### File CRUD

```go
AddFile(f *File) error
GetFile(id int64) (*File, error)
ListFiles(f FileFilter) ([]*File, int, error)
UpdateFile(f *File) error
DeleteFile(id int64) error
```

### Behavior Notes

- `Add*` methods populate the `ID` field on the passed struct after insert
- `Update*` methods update all fields (full replacement, not partial patch)
- `Delete*` methods return `nil` if entity doesn't exist (idempotent)
- Foreign key cascades handle cleanup (delete content → deletes episodes/files)
- List methods return `(results, totalCount, error)` for pagination

---

## Testing Approach

Use real SQLite with in-memory databases:

```go
func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    t.Cleanup(func() { db.Close() })

    // Apply schema from migrations/001_initial.sql
    if _, err := db.Exec(schema); err != nil {
        t.Fatalf("apply schema: %v", err)
    }
    return db
}
```

### Test Files

- `store_test.go` - Content CRUD tests
- `episode_test.go` - Episode CRUD tests
- `file_test.go` - File CRUD tests
- `tx_test.go` - Transaction behavior tests

### Key Test Cases

- Basic CRUD operations for each entity
- Filter combinations and pagination
- Error cases: `ErrNotFound`, `ErrDuplicate`, `ErrConstraint`
- Transaction commit/rollback behavior
- Foreign key cascades (delete content → episodes/files removed)

---

## File Structure

```
internal/library/
├── library.go       # Existing: types (Content, Episode, File, ContentType, etc.)
├── errors.go        # New: ErrNotFound, ErrDuplicate, ErrConstraint
├── filter.go        # New: ContentFilter, EpisodeFilter, FileFilter
├── store.go         # Modified: Store, Tx, querier interface, Begin()
├── content.go       # New: Content CRUD implementation
├── episode.go       # New: Episode CRUD implementation
├── file.go          # New: File CRUD implementation
├── store_test.go    # New: Content tests
├── episode_test.go  # New: Episode tests
├── file_test.go     # New: File tests
└── tx_test.go       # New: Transaction tests
```
