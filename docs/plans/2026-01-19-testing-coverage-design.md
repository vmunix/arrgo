# Download Lifecycle Testing Coverage Design

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Comprehensive test coverage for the download lifecycle state machine to enable safe refactoring.

**Architecture:** Add boundary tests, retry tests, and a full lifecycle integration test to catch state machine bugs and integration breaks.

**Tech Stack:** Go testing, testify, gomock, in-memory SQLite

---

## Background

After fixing a state machine bug (missing `queued → completed` transition), we identified gaps in test coverage that could allow similar regressions. This design adds tests that ensure:

1. Invalid state transitions are rejected (not just valid ones accepted)
2. The full download lifecycle works end-to-end
3. Error recovery (retry) paths are exercised
4. Edge cases (orphans, partial failures) are handled

## Current State

**Well-tested:**
- State machine valid/invalid transitions (20 paths)
- Basic CRUD operations
- Idempotency
- Integration happy path (search → grab → complete)

**Gaps:**
- No test for Refresh() rejecting invalid transitions
- No full lifecycle test (queued → downloading → completed → imported)
- No retry flow test (failed → queued)
- No orphan detection test

## Design Decisions

**Orphan Handling:** When SABnzbd returns "not found" for a tracked download, mark it as `StatusFailed`. This uses the existing state machine and allows retry.

## New Tests

### 1. State Machine Boundary Tests (manager_test.go)

#### TestManager_Refresh_RejectsInvalidTransition
Ensures Refresh() doesn't overwrite good state with stale client data.

```
Setup:
  - Insert download: status=completed, client_id="nzo_123"
  - Mock SABnzbd: returns status=downloading for "nzo_123"

Assert:
  - Download status STILL "completed" (not overwritten)
  - No error returned
```

#### TestManager_Refresh_OrphanDetection
Ensures orphaned downloads are marked as failed.

```
Setup:
  - Insert download: status=downloading, client_id="nzo_orphan"
  - Mock SABnzbd: Status() returns ErrDownloadNotFound

Assert:
  - Download status = "failed"
  - Refresh continues with other downloads
```

#### TestManager_Refresh_PartialFailures
Ensures one failure doesn't stop processing of other downloads.

```
Setup:
  - Insert 3 downloads: nzo_1, nzo_2, nzo_3 (all downloading)
  - Mock SABnzbd:
    - nzo_1 → completed
    - nzo_2 → ErrClientUnavailable
    - nzo_3 → completed

Assert:
  - nzo_1 status = completed
  - nzo_2 status = downloading (unchanged)
  - nzo_3 status = completed
  - Returns lastErr
```

#### TestManager_Refresh_EmptyActiveList
Boundary case - nothing to refresh.

```
Setup:
  - No active downloads in DB

Assert:
  - No error
  - No mock calls
```

### 2. Retry Flow Test (store_test.go)

#### TestStore_Transition_FailedToQueued
Validates the retry path works.

```
Setup:
  - Insert download: status=failed

Action:
  - store.Transition(download, StatusQueued)

Assert:
  - No error
  - Download status = queued
  - LastTransitionAt updated
```

### 3. Cancel State Tests (manager_test.go)

#### TestManager_Cancel_FromQueued
```
Setup: download status=queued
Assert: deleted from DB, Remove() called
```

#### TestManager_Cancel_FromDownloading
```
Setup: download status=downloading
Assert: deleted from DB, Remove() called
```

#### TestManager_Cancel_FromCompleted
```
Setup: download status=completed
Action: Cancel with deleteFiles=true
Assert: deleted from DB, Remove(deleteFiles=true) called
```

### 4. Full Lifecycle Integration Test (integration_test.go)

#### TestIntegration_FullLifecycle_MovieDownload

```
Phase 1 - Content Creation:
  POST /api/v1/content → status=wanted

Phase 2 - Search:
  POST /api/v1/search → releases returned

Phase 3 - Grab:
  POST /api/v1/grab → status=queued

Phase 4 - Download Progress:
  manager.Refresh() → status=downloading

Phase 5 - Download Complete:
  manager.Refresh() → status=completed

Phase 6 - Import:
  importer.Import() → status=imported, content=available

Phase 7 - Verify:
  content.status = available
  file record exists
  history entry exists
```

## Code Changes

### manager.go - Orphan Detection

Add to `Refresh()` method:

```go
status, err := m.client.Status(ctx, d.ClientID)
if err != nil {
    if errors.Is(err, ErrDownloadNotFound) {
        // Download disappeared from client - mark as failed
        m.log.Warn("download orphaned", "download_id", d.ID, "client_id", d.ClientID)
        _ = m.store.Transition(d, StatusFailed)
        continue
    }
    m.log.Error("refresh error", "download_id", d.ID, "error", err)
    lastErr = err
    continue
}
```

## Files Modified

| File | Changes |
|------|---------|
| `internal/download/manager_test.go` | +7 tests (~180 lines) |
| `internal/download/store_test.go` | +1 test (~30 lines) |
| `internal/api/v1/integration_test.go` | +1 test (~100 lines) |
| `internal/download/manager.go` | +8 lines (orphan detection) |

## Deferred Work (GitHub Issues)

1. **Production Readiness Testing** - Concurrent operations, race conditions, context timeouts
2. **Manual Import / Import Existing Library** - Feature + tests for importing existing media

## Success Criteria

- All 9 new tests pass
- Existing tests still pass
- `task check` passes (fmt, lint, test)
- Coverage for download lifecycle state machine is comprehensive
