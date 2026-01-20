# arrgo Architecture Review

A comprehensive review applying Eskil Steenberg's black-box principles and Go best practices.

**Last Updated:** 2026-01-19
**Last Review PR:** #23

---

## Executive Summary

arrgo demonstrates solid architectural foundations as a modular monolith. The codebase excels in interface-based design, testing practices, and domain modeling.

**Overall Assessment: 4.5/5** - Production-ready with clean architecture.

---

## 1. Module Boundary Analysis

### Current Structure

```
arrgo/
├── cmd/
│   ├── arrgo/           # CLI client (Cobra)
│   └── arrgod/          # Server daemon
├── internal/
│   ├── library/         # Content tracking ✅ Excellent
│   ├── search/          # Indexer queries  ✅ Excellent
│   ├── download/        # Download clients ✅ Good
│   ├── importer/        # File import      ✅ Refactored
│   ├── api/v1/          # REST API         ✅ Refactored
│   ├── api/compat/      # Radarr shim      ✅ Good
│   ├── config/          # Configuration    ✅ Excellent
│   └── ai/              # LLM integration  📋 Minimal
└── pkg/
    ├── newznab/         # Protocol client  ✅ Excellent
    └── release/         # Name parsing     ✅ Excellent
```

### Black-Box Assessment by Module

| Module | Interface Quality | Coupling | Testability | Status |
|--------|------------------|----------|-------------|--------|
| `pkg/release` | Pure, no deps | None | Excellent | ✅ Model |
| `pkg/newznab` | Clean HTTP client | None | Excellent | ✅ Model |
| `library` | `querier` interface | Low | Excellent | ✅ Solid |
| `search` | `IndexerAPI` interface | Moderate | Good | ✅ Solid |
| `download` | `Downloader` interface | Low | Good | ✅ Solid |
| `config` | Pure loading | Low | Excellent | ✅ Solid |
| `importer` | `MediaServer` interface | Low | Good | ✅ Refactored |
| `api/v1` | `ServerDeps` injection | Low | Good | ✅ Refactored |

---

## 2. Completed Improvements (PR #23)

### 2.1 API Server Dependency Injection ✅

**Problem (was):** Setter injection with nil checks scattered throughout handlers.

**Solution (implemented):**
- `ServerDeps` struct with required/optional separation
- `NewWithDeps()` constructor with validation
- Middleware wrappers: `requireSearcher()`, `requireManager()`, `requirePlex()`, `requireImporter()`
- Fail-fast validation at construction time

```go
// internal/api/v1/deps.go
type ServerDeps struct {
    // Required dependencies
    Library   *library.Store
    Downloads *download.Store
    History   *importer.HistoryStore

    // Optional dependencies (nil if not configured)
    Searcher  Searcher
    Manager   DownloadManager
    Plex      PlexClient
    Importer  FileImporter
}
```

### 2.2 Importer Refactor ✅

**Problem (was):** `Import()` at 165 lines handling too many responsibilities.

**Solution (implemented):**
- `ImportJob` struct for import operation data
- Three-phase split: `prepareImport()`, `executeImport()`, `notifyMediaServer()`
- Main `Import()` function now ~25 lines

```go
// internal/importer/importer.go
func (i *Importer) Import(ctx context.Context, downloadID int64, downloadPath string) (*ImportResult, error) {
    // Phase 1: Prepare
    job, err := i.prepareImport(downloadID, downloadPath)

    // Phase 2: Execute
    result, err := i.executeImport(job)

    // Phase 3: Notify (best-effort)
    i.notifyMediaServer(ctx, job, result)

    return result, nil
}
```

### 2.3 Media Server Abstraction ✅

**Problem (was):** Plex client directly coupled in importer.

**Solution (implemented):**
- `MediaServer` interface for future extensibility
- `PlexClient` implements `MediaServer`
- Importer uses interface, not concrete type

```go
// internal/importer/mediaserver.go
type MediaServer interface {
    HasContent(ctx context.Context, title string, year int) (bool, error)
    ScanPath(ctx context.Context, path string) error
    RefreshLibrary(ctx context.Context, libraryName string) error
}
```

---

## 3. Current Strengths

### 3.1 Interface-Based Design

The codebase demonstrates excellent use of Go interfaces for testability:

```go
// search/search.go - Clean interface boundary
type IndexerAPI interface {
    Search(ctx context.Context, q Query) ([]Release, []error)
}

// download/download.go - Allows client swaps
type Downloader interface {
    Add(ctx context.Context, url, category string) (clientID string, err error)
    Status(ctx context.Context, clientID string) (*ClientStatus, error)
    List(ctx context.Context) ([]*ClientStatus, error)
    Remove(ctx context.Context, clientID string, deleteFiles bool) error
}
```

### 3.2 Transaction Abstraction

The `querier` pattern elegantly shares code between DB and transaction contexts:

```go
// library/store.go
type querier interface {
    QueryRow(query string, args ...any) *sql.Row
    Query(query string, args ...any) (*sql.Rows, error)
    Exec(query string, args ...any) (sql.Result, error)
}
```

### 3.3 State Machine Rigor

Download status transitions are validated:

```go
StatusQueued → StatusDownloading → StatusCompleted → StatusImported → StatusCleaned
                                 ↘ StatusFailed ↙
```

### 3.4 Testing Excellence

- **45+ test files** with comprehensive coverage
- Table-driven tests for complex scenarios
- Mock implementations via interfaces
- Integration tests covering full workflows

### 3.5 Error Handling

Consistent sentinel error pattern with proper wrapping.

---

## 4. Future Improvement Areas

### 4.1 Scorer Config Coupling (LOW PRIORITY)

**Current:** Scorer reads config types directly.

**Recommendation:** Define profile type in search package to decouple from config.

### 4.2 Test Utility Duplication (LOW PRIORITY)

**Current:** `setupTestDB()` duplicated across test files.

**Recommendation:** Create shared `internal/testutil/db.go`.

### 4.3 Series Support in MediaServer (FUTURE)

**Current:** `HasContent()` only checks movies via `HasMovie()`.

**Recommendation:** Extend to support series when v2 adds series verification.

---

## 5. Dependency Graph

```
                    ┌─────────────┐
                    │   config    │
                    └──────┬──────┘
                           │ reads
              ┌────────────┼────────────┐
              │            │            │
              ▼            ▼            ▼
        ┌─────────┐  ┌─────────┐  ┌─────────┐
        │ library │  │ search  │  │download │
        └────┬────┘  └────┬────┘  └────┬────┘
             │            │            │
             │       ┌────┴────┐       │
             │       │IndexerAPI│      │
             │       └────┬────┘       │
             │            │            │
             │       ┌────▼────┐       │
             │       │newznab  │       │
             │       │(pkg)    │       │
             │       └─────────┘       │
             │                         │
             ▼     MediaServer         ▼
        ┌────────────────────────────────┐
        │           importer             │
        │   (clean interface deps)       │
        └────────────────┬───────────────┘
                         │
                         ▼
        ┌────────────────────────────────┐
        │            api/v1              │
        │   (ServerDeps injection)       │
        └────────────────────────────────┘
```

**No circular dependencies** ✅

---

## 6. Review Checklist

Use this checklist when re-running the architecture review:

- [ ] All tests pass (`go test ./...`)
- [ ] Linter clean (`golangci-lint run`)
- [ ] `go vet ./...` clean
- [ ] No new circular dependencies
- [ ] New modules follow interface-based design
- [ ] New handlers use middleware for optional deps
- [ ] Import function remains under 50 lines
- [ ] Media server operations use `MediaServer` interface

---

## 7. Metrics

| Metric | Target | Current |
|--------|--------|---------|
| Test files | 40+ | 45+ ✅ |
| `Import()` lines | <50 | ~25 ✅ |
| Circular deps | 0 | 0 ✅ |
| Linter warnings | 0 | 0 ✅ |

---

## Changelog

| Date | PR | Changes |
|------|-----|---------|
| 2026-01-19 | #23 | Initial review implementation: API deps injection, importer refactor, MediaServer interface |
