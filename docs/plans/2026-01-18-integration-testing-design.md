# Integration Testing Design

**Date:** 2026-01-18
**Status:** Approved

## Overview

End-to-end integration tests with mocked external services (Prowlarr, SABnzbd, Plex). Tests exercise the full wiring between components without subprocess coordination.

## Design Decisions

1. **In-process testing** - API handler created directly, mock servers via `httptest.NewServer()`, in-memory SQLite
2. **Co-located with API** - `internal/api/v1/integration_test.go` with `//go:build integration` tag
3. **Builder helpers** - Type-safe `mockRelease()`, `mockContent()` functions for test data
4. **State verification** - Check DB state after each step, not just HTTP responses

## Test Files

```
internal/api/v1/
├── integration_test.go       # Integration tests (build tagged)
└── testdata/
    └── schema.sql            # Existing DB schema
```

Run with: `go test -tags=integration ./internal/api/v1/`

## Test Environment

```go
type testEnv struct {
    api         *httptest.Server  // arrgo API under test
    prowlarr    *httptest.Server  // Mock Prowlarr
    sabnzbd     *httptest.Server  // Mock SABnzbd
    plex        *httptest.Server  // Mock Plex
    db          *sql.DB
    manager     *download.Manager

    // Control mock responses
    prowlarrReleases []search.ProwlarrRelease
    sabnzbdClientID  string
    sabnzbdStatus    *download.ClientStatus
    sabnzbdErr       error
    plexNotifyCalled bool
}

func setupIntegrationTest(t *testing.T) *testEnv
func (e *testEnv) cleanup()
```

## Test Cases

### Test 1: API-to-Download (`TestIntegration_SearchAndGrab`)

Verifies: Search → Add content → Grab → Download record created

1. Configure mock Prowlarr with releases
2. POST `/api/v1/search` - verify results returned
3. POST `/api/v1/content` - create content entry
4. POST `/api/v1/grab` - verify SABnzbd called
5. Verify DB: download record exists with correct client_id and status

### Test 2: Download-to-Import (`TestIntegration_DownloadComplete`)

Verifies: Download completes → File imported → Plex notified

1. Seed DB with content + download record
2. Create mock completed download directory with video file
3. Configure SABnzbd mock to report "completed" with path
4. Trigger `manager.CheckDownloads()`
5. Verify DB: download status = "imported", file record created
6. Verify: Plex notification called

### Test 3: Full Happy Path (`TestIntegration_FullHappyPath`)

Combines Test 1 and Test 2 into single end-to-end flow:

Search → Add → Grab → (simulate completion) → Import → Plex notify

## Builder Helpers

```go
func mockRelease(title string, size int64, indexer string) search.ProwlarrRelease
func mockContent(title string, year int) library.Content
func insertTestContent(t *testing.T, db *sql.DB, title string, year int) int64
func insertTestDownload(t *testing.T, db *sql.DB, contentID int64, clientID, status string) int64
func createTestFile(t *testing.T, path string, size int64)
```

## Future (v2)

- Subprocess smoke test starting `arrgo serve` on real ports for live system health validation
