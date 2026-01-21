// internal/handlers/integration_test.go
package handlers_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/handlers"
	"github.com/vmunix/arrgo/internal/importer"
	_ "modernc.org/sqlite"
)

func setupIntegrationDB(t *testing.T) *sql.DB {
	// Use shared cache mode to allow concurrent access from goroutines
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
			last_transition_at TIMESTAMP NOT NULL
		);
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			payload TEXT NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)
	return db
}

// integrationDownloader is a mock download client for integration tests.
type integrationDownloader struct {
	returnID string
}

func (m *integrationDownloader) Add(_ context.Context, _, _ string) (string, error) {
	return m.returnID, nil
}

func (m *integrationDownloader) Status(_ context.Context, _ string) (*download.ClientStatus, error) {
	return &download.ClientStatus{Status: download.StatusCompleted, Path: "/downloads/test"}, nil
}

func (m *integrationDownloader) List(_ context.Context) ([]*download.ClientStatus, error) {
	return nil, nil
}

func (m *integrationDownloader) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}

// integrationImporter is a mock file importer.
// Note: Status transitions (importing → imported) are now handled by ImportHandler.
type integrationImporter struct{}

func (m *integrationImporter) Import(_ context.Context, _ int64, _ string) (*importer.ImportResult, error) {
	return &importer.ImportResult{
		FileID:    1,
		DestPath:  "/movies/test.mkv",
		SizeBytes: 1000,
	}, nil
}

// TestIntegration_GrabToImport tests the full event-driven flow:
// GrabRequested -> DownloadHandler -> DownloadCreated
// DownloadCompleted -> ImportHandler -> ImportCompleted
func TestIntegration_GrabToImport(t *testing.T) {
	db := setupIntegrationDB(t)
	eventLog := events.NewEventLog(db)
	bus := events.NewBus(eventLog, nil)
	defer bus.Close()

	store := download.NewStore(db)
	client := &integrationDownloader{returnID: "sab-integration"}
	imp := &integrationImporter{}

	// Create handlers
	downloadHandler := handlers.NewDownloadHandler(bus, store, nil, client, nil)
	importHandler := handlers.NewImportHandler(bus, store, nil, imp, nil)

	// Subscribe to events we want to track
	downloadCreated := bus.Subscribe(events.EventDownloadCreated, 10)
	importCompleted := bus.Subscribe(events.EventImportCompleted, 10)

	// Start handlers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = downloadHandler.Start(ctx) }()
	go func() { _ = importHandler.Start(ctx) }()

	// Give handlers time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Step 1: Publish GrabRequested
	err := bus.Publish(ctx, &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	})
	require.NoError(t, err)

	// Step 2: Wait for DownloadCreated event
	var createdEvent *events.DownloadCreated
	select {
	case e := <-downloadCreated:
		createdEvent = e.(*events.DownloadCreated)
		assert.Equal(t, int64(42), createdEvent.ContentID)
		assert.Equal(t, "sab-integration", createdEvent.ClientID)
		assert.Equal(t, "Test.Movie.2024", createdEvent.ReleaseName)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	// Verify download was created in store
	downloads, err := store.List(download.Filter{})
	require.NoError(t, err)
	require.Len(t, downloads, 1, "DownloadHandler should have created a download")
	assert.Equal(t, download.StatusQueued, downloads[0].Status)

	// Step 3: Transition to completed (simulates SABnzbd adapter behavior)
	err = store.Transition(downloads[0], download.StatusCompleted)
	require.NoError(t, err)

	// Step 4: Emit DownloadCompleted (simulates adapter detecting SABnzbd finished)
	err = bus.Publish(ctx, &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, downloads[0].ID),
		DownloadID: downloads[0].ID,
		SourcePath: "/downloads/Test.Movie.2024",
	})
	require.NoError(t, err)

	// Step 4: Wait for ImportCompleted event
	select {
	case e := <-importCompleted:
		ic := e.(*events.ImportCompleted)
		assert.Equal(t, int64(42), ic.ContentID)
		assert.Equal(t, "/movies/test.mkv", ic.FilePath)
		assert.Equal(t, downloads[0].ID, ic.DownloadID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ImportCompleted event")
	}

	// Step 5: Verify final state
	// Allow time for status update to complete
	time.Sleep(100 * time.Millisecond)

	dl, err := store.Get(downloads[0].ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusImported, dl.Status, "download should be in imported status")

	// Step 6: Verify events were persisted
	persistedEvents, err := eventLog.Since(time.Now().Add(-time.Minute))
	require.NoError(t, err)

	// Should have at least: GrabRequested, DownloadCreated, DownloadCompleted, ImportStarted, ImportCompleted
	assert.GreaterOrEqual(t, len(persistedEvents), 4, "should have persisted multiple events")

	// Verify event types in order
	eventTypes := make([]string, len(persistedEvents))
	for i, e := range persistedEvents {
		eventTypes[i] = e.EventType
	}

	assert.Contains(t, eventTypes, events.EventGrabRequested)
	assert.Contains(t, eventTypes, events.EventDownloadCreated)
	assert.Contains(t, eventTypes, events.EventDownloadCompleted)
	assert.Contains(t, eventTypes, events.EventImportStarted)
	assert.Contains(t, eventTypes, events.EventImportCompleted)
}

// TestIntegration_GrabFailure tests that download failures are handled correctly.
func TestIntegration_GrabFailure(t *testing.T) {
	db := setupIntegrationDB(t)
	eventLog := events.NewEventLog(db)
	bus := events.NewBus(eventLog, nil)
	defer bus.Close()

	store := download.NewStore(db)
	client := &failingDownloader{err: assert.AnError}

	downloadHandler := handlers.NewDownloadHandler(bus, store, nil, client, nil)

	// Subscribe to failure event
	failed := bus.Subscribe(events.EventDownloadFailed, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = downloadHandler.Start(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// Publish GrabRequested
	err := bus.Publish(ctx, &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	})
	require.NoError(t, err)

	// Wait for DownloadFailed event
	select {
	case e := <-failed:
		df := e.(*events.DownloadFailed)
		assert.True(t, df.Retryable)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for DownloadFailed event")
	}

	// Verify no download was created
	downloads, err := store.List(download.Filter{})
	require.NoError(t, err)
	assert.Empty(t, downloads, "no download should be created on failure")
}

// failingDownloader always returns an error.
type failingDownloader struct {
	err error
}

func (m *failingDownloader) Add(_ context.Context, _, _ string) (string, error) {
	return "", m.err
}

func (m *failingDownloader) Status(_ context.Context, _ string) (*download.ClientStatus, error) {
	return nil, m.err
}

func (m *failingDownloader) List(_ context.Context) ([]*download.ClientStatus, error) {
	return nil, m.err
}

func (m *failingDownloader) Remove(_ context.Context, _ string, _ bool) error {
	return m.err
}

// TestIntegration_ImportFailure tests that import failures emit the correct event.
func TestIntegration_ImportFailure(t *testing.T) {
	db := setupIntegrationDB(t)
	eventLog := events.NewEventLog(db)
	bus := events.NewBus(eventLog, nil)
	defer bus.Close()

	store := download.NewStore(db)
	imp := &failingImporter{err: assert.AnError}

	// Pre-create a download record (simulating one that was already created)
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	importHandler := handlers.NewImportHandler(bus, store, nil, imp, nil)

	// Subscribe to failure event
	failed := bus.Subscribe(events.EventImportFailed, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = importHandler.Start(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// Publish DownloadCompleted
	err := bus.Publish(ctx, &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: "/downloads/Test.Movie.2024",
	})
	require.NoError(t, err)

	// Wait for ImportFailed event
	select {
	case e := <-failed:
		ifailed := e.(*events.ImportFailed)
		assert.Equal(t, dl.ID, ifailed.DownloadID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ImportFailed event")
	}

	// Verify import failed event was persisted
	persistedEvents, err := eventLog.Since(time.Now().Add(-time.Minute))
	require.NoError(t, err)

	hasImportFailed := false
	for _, e := range persistedEvents {
		if e.EventType == events.EventImportFailed {
			hasImportFailed = true
			break
		}
	}
	assert.True(t, hasImportFailed, "ImportFailed event should be persisted")
}

// failingImporter always returns an error.
type failingImporter struct {
	err error
}

func (m *failingImporter) Import(_ context.Context, _ int64, _ string) (*importer.ImportResult, error) {
	return nil, m.err
}

// TestIntegration_EventPersistence verifies that all events are persisted to the event log.
func TestIntegration_EventPersistence(t *testing.T) {
	db := setupIntegrationDB(t)
	eventLog := events.NewEventLog(db)
	bus := events.NewBus(eventLog, nil)
	defer bus.Close()

	ctx := context.Background()

	// Publish several events
	testEvents := []events.Event{
		&events.GrabRequested{
			BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
			ContentID:   1,
			DownloadURL: "http://example.com/1.nzb",
			ReleaseName: "Test.1",
			Indexer:     "test",
		},
		&events.DownloadCreated{
			BaseEvent:   events.NewBaseEvent(events.EventDownloadCreated, events.EntityDownload, 1),
			DownloadID:  1,
			ContentID:   1,
			ClientID:    "sab-1",
			ReleaseName: "Test.1",
		},
		&events.DownloadCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, 1),
			DownloadID: 1,
			SourcePath: "/downloads/Test.1",
		},
	}

	for _, e := range testEvents {
		err := bus.Publish(ctx, e)
		require.NoError(t, err)
	}

	// Query persisted events
	persistedEvents, err := eventLog.Since(time.Now().Add(-time.Minute))
	require.NoError(t, err)

	assert.Len(t, persistedEvents, 3, "all events should be persisted")

	// Verify event types
	assert.Equal(t, events.EventGrabRequested, persistedEvents[0].EventType)
	assert.Equal(t, events.EventDownloadCreated, persistedEvents[1].EventType)
	assert.Equal(t, events.EventDownloadCompleted, persistedEvents[2].EventType)
}
