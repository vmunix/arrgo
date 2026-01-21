// internal/handlers/import_test.go
package handlers

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	_ "modernc.org/sqlite"
)

func setupImportTestDB(t *testing.T) *sql.DB {
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
			last_transition_at TIMESTAMP NOT NULL
		)
	`)
	require.NoError(t, err)
	return db
}

// mockImporter is a test implementation
type mockImporter struct {
	mu           sync.Mutex
	importCalled bool
	callCount    int
	lastID       int64
	lastPath     string
	returnResult *importer.ImportResult
	returnError  error
	delay        time.Duration // Artificial delay for concurrency tests
}

func (m *mockImporter) Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.importCalled = true
	m.callCount++
	m.lastID = downloadID
	m.lastPath = path

	if m.returnError != nil {
		return nil, m.returnError
	}
	return m.returnResult, nil
}

func (m *mockImporter) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestImportHandler_Name(t *testing.T) {
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	handler := NewImportHandler(bus, nil, nil, nil)
	assert.Equal(t, "import", handler.Name())
}

func TestImportHandler_DownloadCompleted(t *testing.T) {
	db := setupImportTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create download record
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	imp := &mockImporter{
		returnResult: &importer.ImportResult{
			FileID:     1,
			SourcePath: "/downloads/Test.Movie.2024.1080p/movie.mkv",
			DestPath:   "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv",
			SizeBytes:  5000000000,
			Quality:    "1080p",
		},
	}

	handler := NewImportHandler(bus, store, imp, nil)

	// Subscribe to ImportStarted and ImportCompleted before starting
	started := bus.Subscribe(events.EventImportStarted, 10)
	completed := bus.Subscribe(events.EventImportCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish DownloadCompleted
	downloadCompleted := &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: "/downloads/Test.Movie.2024.1080p",
	}
	err := bus.Publish(ctx, downloadCompleted)
	require.NoError(t, err)

	// Wait for ImportStarted event
	select {
	case e := <-started:
		is := e.(*events.ImportStarted)
		assert.Equal(t, dl.ID, is.DownloadID)
		assert.Equal(t, "/downloads/Test.Movie.2024.1080p", is.SourcePath)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ImportStarted event")
	}

	// Wait for ImportCompleted event
	select {
	case e := <-completed:
		ic := e.(*events.ImportCompleted)
		assert.Equal(t, dl.ID, ic.DownloadID)
		assert.Equal(t, int64(42), ic.ContentID)
		assert.Equal(t, "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv", ic.FilePath)
		assert.Equal(t, int64(5000000000), ic.FileSize)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ImportCompleted event")
	}

	// Verify importer was called
	assert.True(t, imp.importCalled)
	assert.Equal(t, dl.ID, imp.lastID)
	assert.Equal(t, "/downloads/Test.Movie.2024.1080p", imp.lastPath)
}

func TestImportHandler_ImportFailed(t *testing.T) {
	db := setupImportTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create download record
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	imp := &mockImporter{
		returnError: errors.New("no video file found"),
	}

	handler := NewImportHandler(bus, store, imp, nil)

	// Subscribe to ImportFailed before starting
	failed := bus.Subscribe(events.EventImportFailed, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish DownloadCompleted
	downloadCompleted := &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: "/downloads/Test.Movie.2024.1080p",
	}
	err := bus.Publish(ctx, downloadCompleted)
	require.NoError(t, err)

	// Wait for ImportFailed event
	select {
	case e := <-failed:
		ifailed := e.(*events.ImportFailed)
		assert.Equal(t, dl.ID, ifailed.DownloadID)
		assert.Contains(t, ifailed.Reason, "no video file found")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ImportFailed event")
	}

	// Verify importer was called
	assert.True(t, imp.importCalled)
}

func TestImportHandler_PreventsConcurrentImport(t *testing.T) {
	db := setupImportTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create download record
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Slow importer to test concurrency
	imp := &mockImporter{
		delay: 200 * time.Millisecond,
		returnResult: &importer.ImportResult{
			FileID:     1,
			SourcePath: "/downloads/Test.Movie.2024.1080p/movie.mkv",
			DestPath:   "/movies/Test Movie (2024)/Test Movie (2024) - 1080p.mkv",
			SizeBytes:  5000000000,
			Quality:    "1080p",
		},
	}

	handler := NewImportHandler(bus, store, imp, nil)

	// Subscribe to ImportCompleted to track completion
	completed := bus.Subscribe(events.EventImportCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Send two completion events for the same download in quick succession
	for i := 0; i < 2; i++ {
		downloadCompleted := &events.DownloadCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
			DownloadID: dl.ID,
			SourcePath: "/downloads/Test.Movie.2024.1080p",
		}
		err := bus.Publish(ctx, downloadCompleted)
		require.NoError(t, err)
	}

	// Wait for import to complete
	select {
	case <-completed:
		// Success
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ImportCompleted event")
	}

	// Give time for second event to be processed (if it were to be)
	time.Sleep(100 * time.Millisecond)

	// Verify importer was only called once (per-download lock worked)
	assert.Equal(t, 1, imp.getCallCount(), "importer should only be called once due to per-download lock")
}

func TestImportHandler_AllowsDifferentDownloads(t *testing.T) {
	db := setupImportTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create two download records
	dl1 := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl1))

	dl2 := &download.Download{
		ContentID:   43,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-456",
		Status:      download.StatusCompleted,
		ReleaseName: "Other.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl2))

	var importCount atomic.Int32
	imp := &mockImporter{
		delay: 50 * time.Millisecond,
		returnResult: &importer.ImportResult{
			FileID:     1,
			SourcePath: "/downloads/movie.mkv",
			DestPath:   "/movies/movie.mkv",
			SizeBytes:  5000000000,
			Quality:    "1080p",
		},
	}

	// Wrap importer to count calls
	wrappedImp := &countingImporter{
		inner:   imp,
		counter: &importCount,
	}

	handler := NewImportHandler(bus, store, wrappedImp, nil)

	// Subscribe to ImportCompleted to track completion
	completed := bus.Subscribe(events.EventImportCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Send completion events for both downloads
	for _, dl := range []*download.Download{dl1, dl2} {
		downloadCompleted := &events.DownloadCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
			DownloadID: dl.ID,
			SourcePath: "/downloads/movie",
		}
		err := bus.Publish(ctx, downloadCompleted)
		require.NoError(t, err)
	}

	// Wait for both imports to complete
	receivedCount := 0
	timeout := time.After(time.Second)
	for receivedCount < 2 {
		select {
		case <-completed:
			receivedCount++
		case <-timeout:
			t.Fatalf("timeout waiting for ImportCompleted events, got %d", receivedCount)
		}
	}

	// Verify importer was called twice (once for each download)
	assert.Equal(t, int32(2), importCount.Load(), "importer should be called for each different download")
}

func TestImportHandler_DownloadNotFound(t *testing.T) {
	db := setupImportTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)
	imp := &mockImporter{}

	handler := NewImportHandler(bus, store, imp, nil)

	// Subscribe to ImportFailed before starting
	failed := bus.Subscribe(events.EventImportFailed, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish DownloadCompleted for non-existent download
	downloadCompleted := &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, 9999),
		DownloadID: 9999,
		SourcePath: "/downloads/nonexistent",
	}
	err := bus.Publish(ctx, downloadCompleted)
	require.NoError(t, err)

	// Wait for ImportFailed event
	select {
	case e := <-failed:
		ifailed := e.(*events.ImportFailed)
		assert.Equal(t, int64(9999), ifailed.DownloadID)
		assert.Contains(t, ifailed.Reason, "not found")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ImportFailed event")
	}

	// Verify importer was NOT called
	assert.False(t, imp.importCalled)
}

// countingImporter wraps an importer and counts calls
type countingImporter struct {
	inner   FileImporter
	counter *atomic.Int32
}

func (c *countingImporter) Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error) {
	c.counter.Add(1)
	return c.inner.Import(ctx, downloadID, path)
}
