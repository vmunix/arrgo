// Package plex provides an adapter that verifies imported content in Plex.
package plex

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	_ "modernc.org/sqlite"
)

// mockChecker implements Checker for testing.
type mockChecker struct {
	mu         sync.RWMutex
	hasContent map[int64]bool // contentID -> exists
	plexKeys   map[int64]string
	calls      int
}

func (m *mockChecker) HasContentByID(ctx context.Context, contentID int64) (bool, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.hasContent[contentID] {
		return true, m.plexKeys[contentID], nil
	}
	return false, "", nil
}

func TestAdapter_Name(t *testing.T) {
	adapter := &Adapter{}
	assert.Equal(t, "plex", adapter.Name())
}

func TestAdapter_EmitsPlexItemDetected(t *testing.T) {
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	client := &mockChecker{
		hasContent: map[int64]bool{42: true},
		plexKeys:   map[int64]string{42: "/library/metadata/12345"},
	}

	adapter := New(bus, client, nil, 10*time.Millisecond, nil)

	// Subscribe to events
	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	// Start adapter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit ImportCompleted to register pending
	ic := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, 1),
		DownloadID: 1,
		ContentID:  42,
		FilePath:   "/movies/test.mkv",
		FileSize:   1000000,
	}
	_ = bus.Publish(ctx, ic)

	// Wait for PlexItemDetected
	select {
	case e := <-detected:
		pid := e.(*events.PlexItemDetected)
		assert.Equal(t, int64(42), pid.ContentID)
		assert.Equal(t, "/library/metadata/12345", pid.PlexKey)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlexItemDetected")
	}
}

func TestAdapter_TracksPendingVerifications(t *testing.T) {
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	// Client that doesn't have the content yet
	client := &mockChecker{
		hasContent: map[int64]bool{},
		plexKeys:   map[int64]string{},
	}

	adapter := New(bus, client, nil, 10*time.Millisecond, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit ImportCompleted
	ic := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, 1),
		DownloadID: 1,
		ContentID:  99,
		FilePath:   "/movies/pending.mkv",
		FileSize:   2000000,
	}
	_ = bus.Publish(ctx, ic)

	// Wait a bit for it to process
	time.Sleep(50 * time.Millisecond)

	// Check pending count
	adapter.mu.RLock()
	pendingCount := len(adapter.pending)
	adapter.mu.RUnlock()

	assert.Equal(t, 1, pendingCount, "should have 1 pending verification")
}

func TestAdapter_RemovesPendingAfterDetection(t *testing.T) {
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	client := &mockChecker{
		hasContent: map[int64]bool{},
		plexKeys:   map[int64]string{},
	}

	adapter := New(bus, client, nil, 10*time.Millisecond, slog.Default())
	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit ImportCompleted
	ic := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, 1),
		DownloadID: 1,
		ContentID:  100,
		FilePath:   "/movies/todetect.mkv",
		FileSize:   3000000,
	}
	_ = bus.Publish(ctx, ic)

	// Wait for pending to be tracked
	time.Sleep(30 * time.Millisecond)

	// Verify it's pending
	adapter.mu.RLock()
	_, exists := adapter.pending[100]
	adapter.mu.RUnlock()
	require.True(t, exists, "content should be pending")

	// Now make client return the content as found
	client.mu.Lock()
	client.hasContent[100] = true
	client.plexKeys[100] = "/library/metadata/999"
	client.mu.Unlock()

	// Wait for detection
	select {
	case <-detected:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for detection")
	}

	// Wait a bit for cleanup
	time.Sleep(20 * time.Millisecond)

	// Verify it's removed from pending
	adapter.mu.RLock()
	_, exists = adapter.pending[100]
	adapter.mu.RUnlock()
	assert.False(t, exists, "content should no longer be pending after detection")
}

func TestAdapter_NoDuplicatePlexItemDetected(t *testing.T) {
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	client := &mockChecker{
		hasContent: map[int64]bool{42: true},
		plexKeys:   map[int64]string{42: "/library/metadata/12345"},
	}

	adapter := New(bus, client, nil, 10*time.Millisecond, slog.Default())
	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit ImportCompleted
	ic := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, 1),
		DownloadID: 1,
		ContentID:  42,
		FilePath:   "/movies/test.mkv",
		FileSize:   1000000,
	}
	_ = bus.Publish(ctx, ic)

	// Wait for the first PlexItemDetected
	select {
	case <-detected:
		// Good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for first PlexItemDetected")
	}

	// Wait a bit longer to ensure no duplicates
	time.Sleep(50 * time.Millisecond)

	// Check for any additional events
	select {
	case <-detected:
		t.Fatal("received duplicate PlexItemDetected event")
	default:
		// Good - no duplicate
	}
}

func TestAdapter_HandlesMultiplePendingVerifications(t *testing.T) {
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	client := &mockChecker{
		hasContent: map[int64]bool{},
		plexKeys:   map[int64]string{},
	}

	adapter := New(bus, client, nil, 10*time.Millisecond, slog.Default())
	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit multiple ImportCompleted events
	for i := int64(1); i <= 3; i++ {
		ic := &events.ImportCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, i),
			DownloadID: i,
			ContentID:  i * 10,
			FilePath:   "/movies/test" + string(rune('0'+i)) + ".mkv",
			FileSize:   1000000 * i,
		}
		_ = bus.Publish(ctx, ic)
	}

	// Wait for pending to be tracked
	time.Sleep(30 * time.Millisecond)

	adapter.mu.RLock()
	pendingCount := len(adapter.pending)
	adapter.mu.RUnlock()
	assert.Equal(t, 3, pendingCount, "should have 3 pending verifications")

	// Make one item be found
	client.mu.Lock()
	client.hasContent[20] = true
	client.plexKeys[20] = "/library/metadata/20"
	client.mu.Unlock()

	// Wait for PlexItemDetected for content ID 20
	select {
	case e := <-detected:
		pid := e.(*events.PlexItemDetected)
		assert.Equal(t, int64(20), pid.ContentID)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for PlexItemDetected")
	}

	// Verify only 2 remaining pending
	time.Sleep(20 * time.Millisecond)
	adapter.mu.RLock()
	pendingCount = len(adapter.pending)
	adapter.mu.RUnlock()
	assert.Equal(t, 2, pendingCount, "should have 2 pending verifications remaining")
}

func TestAdapter_StopsOnContextCancel(t *testing.T) {
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	client := &mockChecker{
		hasContent: map[int64]bool{},
		plexKeys:   map[int64]string{},
	}

	adapter := New(bus, client, nil, 10*time.Millisecond, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- adapter.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	// Cancel context
	cancel()

	// Should complete quickly
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("adapter did not stop after context cancel")
	}
}

func setupPlexTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

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

func TestAdapter_ReconcileOnStartup_FoundInPlex(t *testing.T) {
	db := setupPlexTestDB(t)
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	store := download.NewStore(db)

	// Create download in imported status (stuck from previous run)
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusImported,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Client reports content is in Plex
	client := &mockChecker{
		hasContent: map[int64]bool{42: true},
		plexKeys:   map[int64]string{42: "/library/metadata/12345"},
	}

	adapter := New(bus, client, store, 10*time.Millisecond, slog.Default())

	// Subscribe to PlexItemDetected
	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	// Should emit PlexItemDetected during startup reconciliation
	select {
	case e := <-detected:
		pid := e.(*events.PlexItemDetected)
		assert.Equal(t, int64(42), pid.ContentID)
		assert.Equal(t, "/library/metadata/12345", pid.PlexKey)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for PlexItemDetected from reconciliation")
	}
}

func TestAdapter_ReconcileOnStartup_NotInPlex(t *testing.T) {
	db := setupPlexTestDB(t)
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	store := download.NewStore(db)

	// Create download in imported status
	dl := &download.Download{
		ContentID:   99,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-456",
		Status:      download.StatusImported,
		ReleaseName: "Another.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Client reports content NOT in Plex yet
	client := &mockChecker{
		hasContent: map[int64]bool{},
		plexKeys:   map[int64]string{},
	}

	adapter := New(bus, client, store, 10*time.Millisecond, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	// Give time for reconciliation
	time.Sleep(50 * time.Millisecond)

	// Should have added to pending
	adapter.mu.RLock()
	_, exists := adapter.pending[99]
	adapter.mu.RUnlock()
	assert.True(t, exists, "download should be added to pending during reconciliation")
}

func TestAdapter_ReconcileOnStartup_NoImportedDownloads(t *testing.T) {
	db := setupPlexTestDB(t)
	bus := events.NewBus(nil, slog.Default())
	defer bus.Close()

	store := download.NewStore(db)

	// No downloads in imported status
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCleaned, // Already cleaned, not imported
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	client := &mockChecker{
		hasContent: map[int64]bool{},
		plexKeys:   map[int64]string{},
	}

	adapter := New(bus, client, store, 10*time.Millisecond, slog.Default())

	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = adapter.Start(ctx) }()

	// Give time for reconciliation
	time.Sleep(50 * time.Millisecond)

	// Should have no pending (nothing to reconcile)
	adapter.mu.RLock()
	pendingCount := len(adapter.pending)
	adapter.mu.RUnlock()
	assert.Equal(t, 0, pendingCount, "should have no pending when no imported downloads")

	// Should not have emitted any events
	select {
	case <-detected:
		t.Fatal("should not emit PlexItemDetected when no imported downloads")
	default:
		// Good
	}
}
