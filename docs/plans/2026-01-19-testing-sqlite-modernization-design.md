# Testing & SQLite Modernization Design

**Status:** ✅ Complete

## Overview

Modernize the arrgo codebase with improved testing infrastructure and pure Go SQLite, delivered in two phases to isolate risk.

## Goals

- **Developer experience**: Cleaner test assertions with testify, generated mocks for interfaces
- **Build simplicity**: Eliminate CGo requirement by switching to pure Go SQLite

## Phase 1: Testing Modernization (testify + mockgen)

### Dependencies

```
github.com/stretchr/testify
go.uber.org/mock/mockgen
```

### Mock Generation

Create `internal/api/v1/mocks/` package with generated mocks for all four optional service interfaces from `deps.go`:

- `Searcher`
- `DownloadManager`
- `PlexClient`
- `FileImporter`

Add `//go:generate` directive in `internal/api/v1/generate.go`:

```go
//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/api/v1 Searcher,DownloadManager,PlexClient,FileImporter
```

Regenerate mocks via `go generate ./...`.

### Test Migration Approach

- New tests and modified tests use `assert`/`require` from testify
- Existing tests remain unchanged unless touched
- No bulk rewrite of working tests

### Deliverable

Single PR: "Add testify and mockgen for cleaner tests"

## Phase 2: Pure Go SQLite Migration

### Dependency Swap

```diff
- github.com/mattn/go-sqlite3 v1.14.33
+ modernc.org/sqlite
```

### Code Changes

**Driver name**: All `sql.Open("sqlite3", ...)` become `sql.Open("sqlite", ...)`

Locations:
- `cmd/arrgod/server.go` (main database connection)
- Test setup helpers (any `setupTestDB` patterns)

**Import change**:

```diff
- _ "github.com/mattn/go-sqlite3"
+ _ "modernc.org/sqlite"
```

### Migration-Specific Tests

Add `internal/library/sqlite_compat_test.go` covering edge cases where drivers could differ:

- NULL handling in nullable columns
- Type affinity (storing int vs string)
- `RETURNING` clause behavior
- Concurrent read/write under load
- Transaction isolation semantics

### Deliverable

Single PR: "Migrate to pure Go SQLite (modernc.org/sqlite)"

## Implementation Order

### Phase 1 Steps

1. Add dependencies to go.mod
2. Create mock generation setup in `internal/api/v1/mocks/`
3. Run `go generate ./...` to create mocks
4. Update one or two existing test files to demonstrate usage pattern
5. Verify `task test` passes

### Phase 2 Steps

1. Swap dependency in go.mod
2. Update driver name in all `sql.Open` calls
3. Update imports (remove mattn, add modernc)
4. Add `sqlite_compat_test.go` with edge case tests
5. Run full test suite
6. Manual smoke test of `arrgod` startup and basic operations

## Rollback Plan

**Phase 1**: Remove dependencies and generated files (low risk, additive change)

**Phase 2**: Single commit revert - swap import and driver name back to mattn/go-sqlite3

## Benefits

| Aspect | Before | After |
|--------|--------|-------|
| Test assertions | Verbose `if/t.Errorf` | Clean `assert.Equal` |
| Interface mocking | Manual or none | Generated, type-safe |
| Build requirements | CGo + C compiler | Pure Go |
| Cross-compilation | Complex | Simple |
