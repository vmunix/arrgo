// internal/handlers/cleanup_test.go
package handlers

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	_ "modernc.org/sqlite"
)

func setupCleanupTestDB(t *testing.T) *sql.DB {
	// Use shared cache mode for in-memory database to allow concurrent access
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create minimal schema
	_, err = db.Exec(`
		CREATE TABLE downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			client TEXT NOT NULL,
			client_id TEXT NOT NULL,
			status TEXT NOT NULL,
			release_name TEXT NOT NULL,
			indexer TEXT NOT NULL,
			added_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			last_transition_at TIMESTAMP NOT NULL,
			season INTEGER,
			is_complete_season INTEGER DEFAULT 0,
			progress REAL DEFAULT 0,
			speed INTEGER DEFAULT 0,
			eta_seconds INTEGER DEFAULT 0,
			size_bytes INTEGER DEFAULT 0
		);
		CREATE TABLE download_episodes (
			download_id INTEGER NOT NULL,
			episode_id  INTEGER NOT NULL,
			PRIMARY KEY (download_id, episode_id)
		)
	`)
	require.NoError(t, err)
	return db
}

func TestCleanupHandler_Name(t *testing.T) {
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	handler := NewCleanupHandler(bus, nil, CleanupConfig{}, nil)
	assert.Equal(t, "cleanup", handler.Name())
}

func TestCleanupHandler_PlexItemDetected(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create temp dir with test file
	tmpDir := t.TempDir()
	downloadRoot := filepath.Join(tmpDir, "downloads")
	require.NoError(t, os.MkdirAll(downloadRoot, 0755))

	releaseDir := filepath.Join(downloadRoot, "Test.Movie.2024.1080p")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))

	testFile := filepath.Join(releaseDir, "movie.mkv")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Create download record in imported state
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusImported,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupStarted and CleanupCompleted before starting
	started := bus.Subscribe(events.EventCleanupStarted, 10)
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish ImportCompleted (registers pending cleanup)
	importCompleted := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		ContentID:  42,
		FilePath:   "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv",
		FileSize:   5000000000,
	}
	err := bus.Publish(ctx, importCompleted)
	require.NoError(t, err)

	// Give handler time to process ImportCompleted
	time.Sleep(10 * time.Millisecond)

	// Publish PlexItemDetected (triggers cleanup)
	plexDetected := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}
	err = bus.Publish(ctx, plexDetected)
	require.NoError(t, err)

	// Wait for CleanupStarted event
	select {
	case e := <-started:
		cs := e.(*events.CleanupStarted)
		assert.Equal(t, dl.ID, cs.DownloadID)
		assert.Equal(t, releaseDir, cs.SourcePath)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CleanupStarted event")
	}

	// Wait for CleanupCompleted event
	select {
	case e := <-completed:
		cc := e.(*events.CleanupCompleted)
		assert.Equal(t, dl.ID, cc.DownloadID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CleanupCompleted event")
	}

	// Verify files were deleted
	_, err = os.Stat(releaseDir)
	assert.True(t, os.IsNotExist(err), "release directory should be deleted")

	// Verify download status updated to cleaned
	updated, err := store.Get(dl.ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusCleaned, updated.Status)
}

func TestCleanupHandler_Disabled(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create temp dir with test file
	tmpDir := t.TempDir()
	downloadRoot := filepath.Join(tmpDir, "downloads")
	require.NoError(t, os.MkdirAll(downloadRoot, 0755))

	releaseDir := filepath.Join(downloadRoot, "Test.Movie.2024.1080p")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))

	testFile := filepath.Join(releaseDir, "movie.mkv")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Create download record in imported state
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusImported,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      false, // Disabled
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupCompleted before starting
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish ImportCompleted
	importCompleted := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		ContentID:  42,
		FilePath:   "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv",
		FileSize:   5000000000,
	}
	err := bus.Publish(ctx, importCompleted)
	require.NoError(t, err)

	// Give handler time to process ImportCompleted
	time.Sleep(10 * time.Millisecond)

	// Publish PlexItemDetected
	plexDetected := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}
	err = bus.Publish(ctx, plexDetected)
	require.NoError(t, err)

	// No cleanup should happen, wait briefly to be sure
	select {
	case <-completed:
		t.Fatal("should not receive CleanupCompleted when disabled")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event
	}

	// Verify files still exist
	_, err = os.Stat(releaseDir)
	assert.NoError(t, err, "release directory should still exist")
}

func TestCleanupHandler_SafetyRefusesOutsideRoot(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create temp dirs
	tmpDir := t.TempDir()
	downloadRoot := filepath.Join(tmpDir, "downloads")
	require.NoError(t, os.MkdirAll(downloadRoot, 0755))

	// Create a file OUTSIDE the download root
	outsideDir := filepath.Join(tmpDir, "outside")
	require.NoError(t, os.MkdirAll(outsideDir, 0755))
	outsideFile := filepath.Join(outsideDir, "important.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("important"), 0644))

	// Create download record with release name that would resolve outside root
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusImported,
		ReleaseName: "../outside", // Attempt path traversal
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupCompleted before starting
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish ImportCompleted
	importCompleted := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		ContentID:  42,
		FilePath:   "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv",
		FileSize:   5000000000,
	}
	err := bus.Publish(ctx, importCompleted)
	require.NoError(t, err)

	// Give handler time to process ImportCompleted
	time.Sleep(10 * time.Millisecond)

	// Publish PlexItemDetected
	plexDetected := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}
	err = bus.Publish(ctx, plexDetected)
	require.NoError(t, err)

	// Cleanup should not complete (safety check should fail)
	select {
	case <-completed:
		t.Fatal("should not complete cleanup for path outside download root")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event
	}

	// Verify outside file still exists
	_, err = os.Stat(outsideFile)
	assert.NoError(t, err, "file outside download root should still exist")
}

func TestCleanupHandler_NoPendingCleanup(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)
	tmpDir := t.TempDir()

	config := CleanupConfig{
		DownloadRoot: tmpDir,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupCompleted before starting
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish PlexItemDetected WITHOUT prior ImportCompleted
	plexDetected := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}
	err := bus.Publish(ctx, plexDetected)
	require.NoError(t, err)

	// No cleanup should happen
	select {
	case <-completed:
		t.Fatal("should not receive CleanupCompleted when no pending cleanup")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event
	}
}

func TestCleanupHandler_DownloadNotFound(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)
	tmpDir := t.TempDir()

	config := CleanupConfig{
		DownloadRoot: tmpDir,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupCompleted before starting
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish ImportCompleted for non-existent download
	importCompleted := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, 9999),
		DownloadID: 9999,
		ContentID:  42,
		FilePath:   "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv",
		FileSize:   5000000000,
	}
	err := bus.Publish(ctx, importCompleted)
	require.NoError(t, err)

	// Give handler time to process ImportCompleted
	time.Sleep(10 * time.Millisecond)

	// Publish PlexItemDetected
	plexDetected := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}
	err = bus.Publish(ctx, plexDetected)
	require.NoError(t, err)

	// Cleanup should not happen (download not found)
	select {
	case <-completed:
		t.Fatal("should not receive CleanupCompleted when download not found")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event (will fail to get download from store)
	}
}

func TestCleanupHandler_MultipleContentIDs(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create temp dirs with test files
	tmpDir := t.TempDir()
	downloadRoot := filepath.Join(tmpDir, "downloads")
	require.NoError(t, os.MkdirAll(downloadRoot, 0755))

	// Release 1
	releaseDir1 := filepath.Join(downloadRoot, "Movie.One.2024.1080p")
	require.NoError(t, os.MkdirAll(releaseDir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir1, "movie.mkv"), []byte("content1"), 0644))

	// Release 2
	releaseDir2 := filepath.Join(downloadRoot, "Movie.Two.2024.1080p")
	require.NoError(t, os.MkdirAll(releaseDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir2, "movie.mkv"), []byte("content2"), 0644))

	// Create download records
	dl1 := &download.Download{
		ContentID:   100,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-100",
		Status:      download.StatusImported,
		ReleaseName: "Movie.One.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl1))

	dl2 := &download.Download{
		ContentID:   200,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-200",
		Status:      download.StatusImported,
		ReleaseName: "Movie.Two.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl2))

	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupCompleted before starting
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish ImportCompleted for both
	err := bus.Publish(ctx, &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl1.ID),
		DownloadID: dl1.ID,
		ContentID:  100,
		FilePath:   "/movies/Movie One (2024)/movie.mkv",
		FileSize:   5000000000,
	})
	require.NoError(t, err)

	err = bus.Publish(ctx, &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl2.ID),
		DownloadID: dl2.ID,
		ContentID:  200,
		FilePath:   "/movies/Movie Two (2024)/movie.mkv",
		FileSize:   5000000000,
	})
	require.NoError(t, err)

	// Give handler time to process
	time.Sleep(20 * time.Millisecond)

	// Publish PlexItemDetected for content 200 only
	err = bus.Publish(ctx, &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 200),
		ContentID: 200,
		PlexKey:   "/library/metadata/200",
	})
	require.NoError(t, err)

	// Wait for CleanupCompleted event
	select {
	case e := <-completed:
		cc := e.(*events.CleanupCompleted)
		assert.Equal(t, dl2.ID, cc.DownloadID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CleanupCompleted event")
	}

	// Verify only release 2 was deleted
	_, err = os.Stat(releaseDir1)
	require.NoError(t, err, "release 1 should still exist")

	_, err = os.Stat(releaseDir2)
	assert.True(t, os.IsNotExist(err), "release 2 should be deleted")
}

func TestCleanupHandler_ImportSkipped_ImmediateCleanup(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create download record in skipped status
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusSkipped,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Create temp directory structure
	downloadRoot := t.TempDir()
	releaseDir := filepath.Join(downloadRoot, dl.ReleaseName)
	require.NoError(t, os.MkdirAll(releaseDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, "movie.mkv"), []byte("test"), 0644))

	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Subscribe to CleanupCompleted
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Publish ImportSkipped event (should trigger immediate cleanup, no Plex detection needed)
	err := bus.Publish(ctx, &events.ImportSkipped{
		BaseEvent:       events.NewBaseEvent(events.EventImportSkipped, events.EntityDownload, dl.ID),
		DownloadID:      dl.ID,
		ContentID:       42,
		SourcePath:      releaseDir,
		ReleaseQuality:  "1080p",
		ExistingQuality: "2160p",
		Reason:          "existing_quality_equal_or_better",
	})
	require.NoError(t, err)

	// Should get CleanupCompleted (immediate, without waiting for Plex)
	select {
	case e := <-completed:
		cc := e.(*events.CleanupCompleted)
		assert.Equal(t, dl.ID, cc.DownloadID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CleanupCompleted event")
	}

	// Verify files were deleted
	_, err = os.Stat(releaseDir)
	assert.True(t, os.IsNotExist(err), "release directory should be deleted")
}

func TestCleanupHandler_ImportSkipped_Disabled(t *testing.T) {
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusSkipped,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	downloadRoot := t.TempDir()
	releaseDir := filepath.Join(downloadRoot, dl.ReleaseName)
	require.NoError(t, os.MkdirAll(releaseDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, "movie.mkv"), []byte("test"), 0644))

	config := CleanupConfig{
		DownloadRoot: downloadRoot,
		Enabled:      false, // Disabled
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	err := bus.Publish(ctx, &events.ImportSkipped{
		BaseEvent:       events.NewBaseEvent(events.EventImportSkipped, events.EntityDownload, dl.ID),
		DownloadID:      dl.ID,
		ContentID:       42,
		SourcePath:      releaseDir,
		ReleaseQuality:  "1080p",
		ExistingQuality: "2160p",
		Reason:          "existing_quality_equal_or_better",
	})
	require.NoError(t, err)

	// Should NOT get CleanupCompleted (disabled)
	select {
	case <-completed:
		t.Fatal("should not cleanup when disabled")
	case <-time.After(100 * time.Millisecond):
		// Expected - no cleanup
	}

	// Verify files still exist
	_, err = os.Stat(releaseDir)
	assert.NoError(t, err, "release directory should still exist when cleanup is disabled")
}

func TestCleanupHandler_ReconcileOnStartup(t *testing.T) {
	// Setup
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

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
	handler := NewCleanupHandler(bus, store, config, nil)

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

func TestCleanupHandler_ReconcileOnStartup_FullFlow(t *testing.T) {
	// Setup
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

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
	handler := NewCleanupHandler(bus, store, config, nil)

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

func TestCleanupHandler_ReconcileOnStartup_NoImportedDownloads(t *testing.T) {
	// Setup
	db := setupCleanupTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// No downloads in database

	config := CleanupConfig{
		DownloadRoot: t.TempDir(),
		Enabled:      true,
	}
	handler := NewCleanupHandler(bus, store, config, nil)

	// Call reconcileOnStartup - should not error
	ctx := context.Background()
	handler.reconcileOnStartup(ctx)

	// Verify pending map is empty
	handler.mu.RLock()
	count := len(handler.pending)
	handler.mu.RUnlock()

	assert.Equal(t, 0, count, "expected empty pending map")
}
