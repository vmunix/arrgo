# Cleanup Handler Startup Reconciliation

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix cleanup handler to restore pending cleanups from database on startup, preventing orphaned download folders after server restart.

**Architecture:** Add reconcileOnStartup() method that queries "imported" downloads and pre-populates the pending map before event processing begins.

**Tech Stack:** Go, SQLite (via download.Store)

---

### Task 1: Add failing test for reconcileOnStartup

**Files:**
- Modify: `internal/handlers/cleanup_test.go`

**Step 1: Write failing test**

```go
func TestCleanupHandler_ReconcileOnStartup(t *testing.T) {
	// Setup
	db := testutil.NewTestDB(t)
	bus := events.NewBus(slog.Default())
	store := download.NewStore(db)
	logger := slog.Default()

	contentID := int64(123)

	// Create a download in "imported" status (simulating server restart scenario)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_test",
		Status:      download.StatusImported,
		ReleaseName: "Test.Movie.2024.1080p.BluRay",
		Indexer:     "test",
	}
	err := store.Add(dl)
	require.NoError(t, err)

	// Create cleanup handler
	config := CleanupConfig{
		DownloadRoot: t.TempDir(),
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, logger)

	// Call reconcileOnStartup
	ctx := context.Background()
	handler.reconcileOnStartup(ctx)

	// Verify pending map contains the download
	handler.mu.RLock()
	pending, ok := handler.pending[contentID]
	handler.mu.RUnlock()

	assert.True(t, ok, "expected pending entry for content ID")
	assert.Equal(t, dl.ID, pending.DownloadID)
	assert.Equal(t, contentID, pending.ContentID)
	assert.Equal(t, "Test.Movie.2024.1080p.BluRay", pending.ReleaseName)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run TestCleanupHandler_ReconcileOnStartup -v`
Expected: FAIL - reconcileOnStartup method doesn't exist

**Step 3: Commit failing test**

```bash
git add internal/handlers/cleanup_test.go
git commit -m "test: add failing test for cleanup reconciliation (#61)"
```

---

### Task 2: Implement reconcileOnStartup method

**Files:**
- Modify: `internal/handlers/cleanup.go`

**Step 1: Add reconcileOnStartup method**

Add after NewCleanupHandler:

```go
// reconcileOnStartup restores pending cleanups from database.
// This handles the case where server restarted after import but before Plex detection.
func (h *CleanupHandler) reconcileOnStartup(ctx context.Context) {
	// Query downloads in "imported" status
	status := download.StatusImported
	downloads, err := h.store.List(download.Filter{Status: &status})
	if err != nil {
		h.Logger().Error("failed to list imported downloads for reconciliation", "error", err)
		return
	}

	if len(downloads) == 0 {
		h.Logger().Debug("no imported downloads to reconcile for cleanup")
		return
	}

	h.Logger().Info("reconciling imported downloads for cleanup", "count", len(downloads))

	// Pre-populate pending map
	h.mu.Lock()
	for _, dl := range downloads {
		h.pending[dl.ContentID] = &pendingCleanup{
			DownloadID:  dl.ID,
			ContentID:   dl.ContentID,
			ReleaseName: dl.ReleaseName,
		}
	}
	h.mu.Unlock()
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/handlers -run TestCleanupHandler_ReconcileOnStartup -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/handlers/cleanup.go
git commit -m "feat: add reconcileOnStartup to cleanup handler (#61)"
```

---

### Task 3: Call reconcileOnStartup from Start method

**Files:**
- Modify: `internal/handlers/cleanup.go`

**Step 1: Update Start method**

Replace lines 56-60:

```go
// Start begins processing events.
func (h *CleanupHandler) Start(ctx context.Context) error {
	importCompleted := h.Bus().Subscribe(events.EventImportCompleted, 100)
	importSkipped := h.Bus().Subscribe(events.EventImportSkipped, 100)
	plexDetected := h.Bus().Subscribe(events.EventPlexItemDetected, 100)
```

With:

```go
// Start begins processing events.
func (h *CleanupHandler) Start(ctx context.Context) error {
	// Reconcile on startup - restore pending cleanups from database
	h.reconcileOnStartup(ctx)

	importCompleted := h.Bus().Subscribe(events.EventImportCompleted, 100)
	importSkipped := h.Bus().Subscribe(events.EventImportSkipped, 100)
	plexDetected := h.Bus().Subscribe(events.EventPlexItemDetected, 100)
```

**Step 2: Run existing tests to verify no regression**

Run: `go test ./internal/handlers -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add internal/handlers/cleanup.go
git commit -m "feat: call reconcileOnStartup in cleanup handler Start (#61)"
```

---

### Task 4: Add integration test for full reconciliation flow

**Files:**
- Modify: `internal/handlers/cleanup_test.go`

**Step 1: Write integration test**

```go
func TestCleanupHandler_ReconcileOnStartup_FullFlow(t *testing.T) {
	// Setup
	db := testutil.NewTestDB(t)
	bus := events.NewBus(slog.Default())
	store := download.NewStore(db)
	logger := slog.Default()

	contentID := int64(456)
	downloadRoot := t.TempDir()

	// Create source folder (simulating download that was imported but not cleaned)
	releaseName := "Orphaned.Movie.2024.1080p.BluRay"
	sourceDir := filepath.Join(downloadRoot, releaseName)
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "movie.mkv"), []byte("test"), 0644))

	// Create download in "imported" status
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_orphan",
		Status:      download.StatusImported,
		ReleaseName: releaseName,
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	// Create and start cleanup handler
	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start handler in background
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to reconcile and subscribe
	time.Sleep(50 * time.Millisecond)

	// Emit PlexItemDetected (simulating Plex adapter reconciliation)
	err := bus.Publish(ctx, &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, contentID),
		ContentID: contentID,
		PlexKey:   "plex_123",
	})
	require.NoError(t, err)

	// Wait for cleanup to process
	time.Sleep(100 * time.Millisecond)

	// Verify source folder was deleted
	_, err = os.Stat(sourceDir)
	assert.True(t, os.IsNotExist(err), "expected source folder to be deleted")

	// Verify download transitioned to cleaned
	updated, err := store.Get(dl.ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusCleaned, updated.Status)
}
```

**Step 2: Run test**

Run: `go test ./internal/handlers -run TestCleanupHandler_ReconcileOnStartup_FullFlow -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/handlers/cleanup_test.go
git commit -m "test: add integration test for cleanup reconciliation flow (#61)"
```

---

### Task 5: Add test for empty database case

**Files:**
- Modify: `internal/handlers/cleanup_test.go`

**Step 1: Write test**

```go
func TestCleanupHandler_ReconcileOnStartup_NoImportedDownloads(t *testing.T) {
	// Setup
	db := testutil.NewTestDB(t)
	bus := events.NewBus(slog.Default())
	store := download.NewStore(db)
	logger := slog.Default()

	// No downloads in database

	config := CleanupConfig{
		DownloadRoot: t.TempDir(),
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, logger)

	// Call reconcileOnStartup - should not error
	ctx := context.Background()
	handler.reconcileOnStartup(ctx)

	// Verify pending map is empty
	handler.mu.RLock()
	count := len(handler.pending)
	handler.mu.RUnlock()

	assert.Equal(t, 0, count, "expected empty pending map")
}
```

**Step 2: Run test**

Run: `go test ./internal/handlers -run TestCleanupHandler_ReconcileOnStartup_NoImportedDownloads -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/handlers/cleanup_test.go
git commit -m "test: add test for reconciliation with no imported downloads (#61)"
```

---

### Task 6: Final verification

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Run linter**

Run: `golangci-lint run`
Expected: No issues

**Step 3: Manual verification with arrgod**

```bash
go build ./cmd/arrgod
./arrgod
# Check logs for "reconciling imported downloads for cleanup" message
```

**Step 4: Squash commits if desired**

```bash
git rebase -i HEAD~5  # Optional: squash into single commit
```
