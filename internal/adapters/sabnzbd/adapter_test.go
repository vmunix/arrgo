package sabnzbd

import (
	"context"
	"database/sql"
	_ "embed"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/download/mocks"
	"github.com/vmunix/arrgo/internal/events"
	"go.uber.org/mock/gomock"
	_ "modernc.org/sqlite"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(testSchema)
	require.NoError(t, err)
	return db
}

// insertTestContent inserts a test content row and returns its ID.
func insertTestContent(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2000, 'wanted', 'hd', '/movies')`,
	)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestAdapter_Name(t *testing.T) {
	adapter := &Adapter{}
	assert.Equal(t, "sabnzbd", adapter.Name())
}

func TestAdapter_EmitsDownloadCompleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Subscribe to completed events
	completedCh := bus.Subscribe(events.EventDownloadCompleted, 10)

	// Create tracked download in store
	contentID := insertTestContent(t, db)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Mock client reports completed
	mockClient.EXPECT().
		Status(gomock.Any(), "nzo_abc123").
		Return(&download.ClientStatus{
			ID:       "nzo_abc123",
			Name:     "Test.Movie.2024.1080p.WEB-DL",
			Status:   download.StatusCompleted,
			Progress: 100,
			Path:     "/downloads/complete/Test.Movie.2024.1080p.WEB-DL",
		}, nil)

	// Create adapter with short interval for testing
	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	// Start adapter and let it poll once
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = adapter.Start(ctx)
	}()

	// Wait for completed event
	select {
	case evt := <-completedCh:
		completed, ok := evt.(*events.DownloadCompleted)
		require.True(t, ok, "expected DownloadCompleted event")
		assert.Equal(t, dl.ID, completed.DownloadID)
		assert.Equal(t, "/downloads/complete/Test.Movie.2024.1080p.WEB-DL", completed.SourcePath)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DownloadCompleted event")
	}
}

func TestAdapter_EmitsDownloadProgressed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Subscribe to progressed events
	progressCh := bus.Subscribe(events.EventDownloadProgressed, 10)

	// Create tracked download in store
	contentID := insertTestContent(t, db)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_xyz789",
		Status:      download.StatusQueued,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Mock client reports in-progress with progress/speed
	mockClient.EXPECT().
		Status(gomock.Any(), "nzo_xyz789").
		Return(&download.ClientStatus{
			ID:       "nzo_xyz789",
			Name:     "Test.Movie.2024.1080p.WEB-DL",
			Status:   download.StatusDownloading,
			Progress: 45.5,
			Size:     5000000000, // 5GB
			Speed:    10000000,   // 10 MB/s
			ETA:      5 * time.Minute,
		}, nil)

	// Create adapter with short interval for testing
	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = adapter.Start(ctx)
	}()

	// Wait for progress event
	select {
	case evt := <-progressCh:
		progressed, ok := evt.(*events.DownloadProgressed)
		require.True(t, ok, "expected DownloadProgressed event")
		assert.Equal(t, dl.ID, progressed.DownloadID)
		assert.InDelta(t, 45.5, progressed.Progress, 0.001)
		assert.Equal(t, int64(10000000), progressed.Speed)
		assert.Equal(t, int64(5000000000), progressed.Size)
		assert.Equal(t, 300, progressed.ETA) // 5 minutes = 300 seconds
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DownloadProgressed event")
	}
}

func TestAdapter_EmitsDownloadFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Subscribe to failed events
	failedCh := bus.Subscribe(events.EventDownloadFailed, 10)

	// Create tracked download in store
	contentID := insertTestContent(t, db)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_fail456",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Mock client reports failed
	mockClient.EXPECT().
		Status(gomock.Any(), "nzo_fail456").
		Return(&download.ClientStatus{
			ID:     "nzo_fail456",
			Name:   "Test.Movie.2024.1080p.WEB-DL",
			Status: download.StatusFailed,
		}, nil)

	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = adapter.Start(ctx)
	}()

	// Wait for failed event
	select {
	case evt := <-failedCh:
		failed, ok := evt.(*events.DownloadFailed)
		require.True(t, ok, "expected DownloadFailed event")
		assert.Equal(t, dl.ID, failed.DownloadID)
		assert.Equal(t, "download reported failed by client", failed.Reason)
		assert.True(t, failed.Retryable)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DownloadFailed event")
	}
}

func TestAdapter_EmitsDownloadFailed_WhenDisappeared(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Subscribe to failed events
	failedCh := bus.Subscribe(events.EventDownloadFailed, 10)

	// Create tracked download in store
	contentID := insertTestContent(t, db)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_gone123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Mock client returns nil (download disappeared)
	mockClient.EXPECT().
		Status(gomock.Any(), "nzo_gone123").
		Return(nil, nil)

	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = adapter.Start(ctx)
	}()

	// Wait for failed event
	select {
	case evt := <-failedCh:
		failed, ok := evt.(*events.DownloadFailed)
		require.True(t, ok, "expected DownloadFailed event")
		assert.Equal(t, dl.ID, failed.DownloadID)
		assert.Equal(t, "download disappeared from client", failed.Reason)
		assert.False(t, failed.Retryable)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DownloadFailed event")
	}
}

func TestAdapter_NoDuplicateStateTransitionEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Subscribe to completed events
	completedCh := bus.Subscribe(events.EventDownloadCompleted, 10)

	// Create tracked download already completed
	contentID := insertTestContent(t, db)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_dup123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Mock client reports completed multiple times - use AnyTimes since the
	// download stays in "downloading" status in DB (adapter doesn't update store),
	// so it will be re-polled on each tick
	mockClient.EXPECT().
		Status(gomock.Any(), "nzo_dup123").
		Return(&download.ClientStatus{
			ID:       "nzo_dup123",
			Name:     "Test.Movie.2024.1080p.WEB-DL",
			Status:   download.StatusCompleted,
			Progress: 100,
			Path:     "/downloads/complete/Test.Movie.2024.1080p.WEB-DL",
		}, nil).
		AnyTimes()

	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() {
		_ = adapter.Start(ctx)
	}()

	// Should only receive one completed event despite multiple polls
	var eventCount int
	timeout := time.After(100 * time.Millisecond)
loop:
	for {
		select {
		case <-completedCh:
			eventCount++
		case <-timeout:
			break loop
		}
	}

	assert.Equal(t, 1, eventCount, "should only emit one DownloadCompleted event despite multiple polls")
}

func TestAdapter_OnlyPollsSABnzbdDownloads(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Create a manual download (should be ignored)
	contentID := insertTestContent(t, db)
	manualDL := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientManual,
		ClientID:    "manual123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "manual",
	}
	require.NoError(t, store.Add(manualDL))

	// No expectations on mockClient - it should not be called

	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// This should complete without errors and without calling the client
	err := adapter.Start(ctx)
	assert.NoError(t, err)
}

func TestAdapter_SkipsTerminalStates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Create downloads in various terminal states
	contentID := insertTestContent(t, db)
	states := []download.Status{
		download.StatusCompleted,
		download.StatusImported,
		download.StatusCleaned,
		download.StatusFailed,
	}

	for i, status := range states {
		dl := &download.Download{
			ContentID:   contentID,
			Client:      download.ClientSABnzbd,
			ClientID:    "nzo_terminal" + string(rune('0'+i)),
			Status:      status,
			ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
			Indexer:     "nzbgeek",
		}
		require.NoError(t, store.Add(dl))
	}

	// No expectations on mockClient - it should not be called for terminal states

	adapter := New(bus, mockClient, store, Config{Interval: 10 * time.Millisecond}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := adapter.Start(ctx)
	assert.NoError(t, err)
}

func TestAdapter_RemapsPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockDownloader(ctrl)

	db := setupTestDB(t)
	store := download.NewStore(db)
	bus := events.NewBus(nil, slog.Default())
	t.Cleanup(func() { _ = bus.Close() })

	// Subscribe to completed events
	completedCh := bus.Subscribe(events.EventDownloadCompleted, 10)

	// Create tracked download in store
	contentID := insertTestContent(t, db)
	dl := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_remap",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(dl))

	// Mock client reports completed with remote path
	mockClient.EXPECT().
		Status(gomock.Any(), "nzo_remap").
		Return(&download.ClientStatus{
			ID:       "nzo_remap",
			Name:     "Test.Movie.2024.1080p.WEB-DL",
			Status:   download.StatusCompleted,
			Progress: 100,
			Path:     "/data/usenet/Test.Movie.2024.1080p.WEB-DL/movie.mkv", // SABnzbd's view
		}, nil)

	// Create adapter with path remapping
	adapter := New(bus, mockClient, store, Config{
		Interval:   10 * time.Millisecond,
		RemotePath: "/data/usenet",
		LocalPath:  "/srv/data/usenet",
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = adapter.Start(ctx)
	}()

	// Wait for completed event and verify path is remapped
	select {
	case evt := <-completedCh:
		completed, ok := evt.(*events.DownloadCompleted)
		require.True(t, ok, "expected DownloadCompleted event")
		assert.Equal(t, dl.ID, completed.DownloadID)
		// Path should be remapped from /data/usenet to /srv/data/usenet
		assert.Equal(t, "/srv/data/usenet/Test.Movie.2024.1080p.WEB-DL/movie.mkv", completed.SourcePath)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DownloadCompleted event")
	}
}

func TestAdapter_RemapPath_NoConfig(t *testing.T) {
	adapter := &Adapter{}
	// Without path mapping config, path should pass through unchanged
	assert.Equal(t, "/some/path/file.mkv", adapter.remapPath("/some/path/file.mkv"))
}

func TestAdapter_RemapPath_NoMatch(t *testing.T) {
	adapter := &Adapter{
		config: Config{
			RemotePath: "/data/usenet",
			LocalPath:  "/srv/data/usenet",
		},
	}
	// Path that doesn't match remote prefix should pass through unchanged
	assert.Equal(t, "/other/path/file.mkv", adapter.remapPath("/other/path/file.mkv"))
}
