// internal/handlers/download_test.go
package handlers

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/library"
	_ "modernc.org/sqlite"
)

func setupDownloadTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
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

// mockDownloader is a test implementation
type mockDownloader struct {
	addCalled   bool
	lastURL     string
	returnID    string
	returnError error
}

func (m *mockDownloader) Add(ctx context.Context, url, category string) (string, error) {
	m.addCalled = true
	m.lastURL = url
	if m.returnError != nil {
		return "", m.returnError
	}
	return m.returnID, nil
}

func (m *mockDownloader) Status(ctx context.Context, clientID string) (*download.ClientStatus, error) {
	return nil, nil
}

func (m *mockDownloader) List(ctx context.Context) ([]*download.ClientStatus, error) {
	return nil, nil
}

func (m *mockDownloader) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	return nil
}

func TestDownloadHandler_GrabRequested(t *testing.T) {
	db := setupDownloadTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)
	client := &mockDownloader{returnID: "sab-123"}

	handler := NewDownloadHandler(bus, store, nil, client, nil)

	// Subscribe to DownloadCreated before starting
	created := bus.Subscribe(events.EventDownloadCreated, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = handler.Start(ctx)
	}()

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish GrabRequested
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	err := bus.Publish(ctx, grab)
	require.NoError(t, err)

	// Wait for DownloadCreated event
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(42), dc.ContentID)
		assert.Equal(t, "sab-123", dc.ClientID)
		assert.Equal(t, "Test.Movie.2024.1080p", dc.ReleaseName)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	// Verify client was called
	assert.True(t, client.addCalled)
	assert.Equal(t, "https://example.com/test.nzb", client.lastURL)
}

// setupDownloadTestDBWithLibrary creates a test DB with both download and library schemas.
func setupDownloadTestDBWithLibrary(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
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
		);
		CREATE TABLE content (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			year INTEGER,
			status TEXT NOT NULL DEFAULT 'wanted',
			quality_profile TEXT NOT NULL DEFAULT 'hd',
			root_path TEXT NOT NULL
		);
		CREATE TABLE files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			path TEXT NOT NULL UNIQUE,
			size_bytes INTEGER,
			quality TEXT,
			source TEXT,
			added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)
	return db
}

func TestDownloadHandler_GrabSkipped_ExistingQualityEqual(t *testing.T) {
	db := setupDownloadTestDBWithLibrary(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create content and existing file with 1080p
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (42, 'movie', 'Test Movie', 2024, '/movies')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO files (content_id, path, quality, size_bytes, source) VALUES (42, '/movies/test.mkv', '1080p', 5000000000, 'webdl')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	libraryStore := library.NewStore(db)
	client := &mockDownloader{returnID: "sab-123"}

	handler := NewDownloadHandler(bus, downloadStore, libraryStore, client, nil)

	// Subscribe to events
	skipped := bus.Subscribe(events.EventGrabSkipped, 10)
	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Try to grab 1080p (same as existing)
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Should get GrabSkipped, not DownloadCreated
	select {
	case e := <-skipped:
		gs := e.(*events.GrabSkipped)
		assert.Equal(t, int64(42), gs.ContentID)
		assert.Equal(t, "1080p", gs.ReleaseQuality)
		assert.Equal(t, "1080p", gs.ExistingQuality)
		assert.Equal(t, "existing_quality_equal_or_better", gs.Reason)
	case <-created:
		t.Fatal("should not create download when existing quality is equal")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Verify client was NOT called
	assert.False(t, client.addCalled, "download client should not be called when grab is skipped")
}

func TestDownloadHandler_GrabSkipped_ExistingQualityBetter(t *testing.T) {
	db := setupDownloadTestDBWithLibrary(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create content and existing file with 4K
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (42, 'movie', 'Test Movie', 2024, '/movies')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO files (content_id, path, quality, size_bytes, source) VALUES (42, '/movies/test.mkv', '2160p', 10000000000, 'bluray')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	libraryStore := library.NewStore(db)
	client := &mockDownloader{returnID: "sab-123"}

	handler := NewDownloadHandler(bus, downloadStore, libraryStore, client, nil)

	skipped := bus.Subscribe(events.EventGrabSkipped, 10)
	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Try to grab 1080p (worse than existing 4K)
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024.1080p.BluRay",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Should get GrabSkipped
	select {
	case e := <-skipped:
		gs := e.(*events.GrabSkipped)
		assert.Equal(t, "1080p", gs.ReleaseQuality)
		assert.Equal(t, "2160p", gs.ExistingQuality)
	case <-created:
		t.Fatal("should not create download when existing quality is better")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	assert.False(t, client.addCalled)
}

func TestDownloadHandler_GrabProceeds_QualityUpgrade(t *testing.T) {
	db := setupDownloadTestDBWithLibrary(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create content and existing file with 720p
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (42, 'movie', 'Test Movie', 2024, '/movies')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO files (content_id, path, quality, size_bytes, source) VALUES (42, '/movies/test.mkv', '720p', 3000000000, 'webdl')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	libraryStore := library.NewStore(db)
	client := &mockDownloader{returnID: "sab-123"}

	handler := NewDownloadHandler(bus, downloadStore, libraryStore, client, nil)

	skipped := bus.Subscribe(events.EventGrabSkipped, 10)
	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Try to grab 1080p (upgrade from 720p)
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024.1080p.BluRay",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Should get DownloadCreated (upgrade allowed)
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(42), dc.ContentID)
	case <-skipped:
		t.Fatal("should not skip grab when it's a quality upgrade")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	assert.True(t, client.addCalled, "download client should be called for upgrade")
}

func TestDownloadHandler_GrabProceeds_NoExistingFiles(t *testing.T) {
	db := setupDownloadTestDBWithLibrary(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create content with NO existing files
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (42, 'movie', 'Test Movie', 2024, '/movies')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	libraryStore := library.NewStore(db)
	client := &mockDownloader{returnID: "sab-123"}

	handler := NewDownloadHandler(bus, downloadStore, libraryStore, client, nil)

	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024.1080p.BluRay",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Should get DownloadCreated (no existing files)
	select {
	case <-created:
		// Success
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	assert.True(t, client.addCalled)
}

// setupDownloadTestDBWithEpisodes creates a test DB with download, library, and episode schemas.
func setupDownloadTestDBWithEpisodes(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
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
		);
		CREATE TABLE content (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			year INTEGER,
			status TEXT NOT NULL DEFAULT 'wanted',
			quality_profile TEXT NOT NULL DEFAULT 'hd',
			root_path TEXT NOT NULL
		);
		CREATE TABLE episodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
			season INTEGER NOT NULL,
			episode INTEGER NOT NULL,
			title TEXT,
			status TEXT NOT NULL DEFAULT 'wanted',
			air_date DATE,
			UNIQUE(content_id, season, episode)
		);
		CREATE TABLE files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			path TEXT NOT NULL UNIQUE,
			size_bytes INTEGER,
			quality TEXT,
			source TEXT,
			added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)
	return db
}

func TestDownloadHandler_GrabWithEpisodeIDs(t *testing.T) {
	db := setupDownloadTestDBWithEpisodes(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create series content
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (1, 'series', 'Breaking Bad', 2008, '/tv')`)
	require.NoError(t, err)

	// Create episodes
	_, err = db.Exec(`INSERT INTO episodes (id, content_id, season, episode, title, status) VALUES (101, 1, 1, 1, 'Pilot', 'wanted')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO episodes (id, content_id, season, episode, title, status) VALUES (102, 1, 1, 2, 'Cat in the Bag', 'wanted')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	client := &mockDownloader{returnID: "sab-456"}

	handler := NewDownloadHandler(bus, downloadStore, nil, client, nil)

	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit GrabRequested with multiple episode IDs
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   1,
		EpisodeIDs:  []int64{101, 102},
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Breaking.Bad.S01E01E02.1080p.BluRay",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Wait for DownloadCreated event
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(1), dc.ContentID)
		assert.Equal(t, "sab-456", dc.ClientID)
		assert.Equal(t, []int64{101, 102}, dc.EpisodeIDs)
		assert.Equal(t, "Breaking.Bad.S01E01E02.1080p.BluRay", dc.ReleaseName)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	// Verify client was called
	assert.True(t, client.addCalled)

	// Verify download was created in DB with correct episode links
	var downloadID int64
	err = db.QueryRow("SELECT id FROM downloads WHERE content_id = 1").Scan(&downloadID)
	require.NoError(t, err)

	// Verify episode IDs were linked in junction table
	rows, err := db.Query("SELECT episode_id FROM download_episodes WHERE download_id = ? ORDER BY episode_id", downloadID)
	require.NoError(t, err)
	defer rows.Close()

	var linkedEpisodes []int64
	for rows.Next() {
		var epID int64
		require.NoError(t, rows.Scan(&epID))
		linkedEpisodes = append(linkedEpisodes, epID)
	}
	assert.Equal(t, []int64{101, 102}, linkedEpisodes)
}

func TestDownloadHandler_GrabSeasonPack(t *testing.T) {
	db := setupDownloadTestDBWithEpisodes(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create series content
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (1, 'series', 'Breaking Bad', 2008, '/tv')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	client := &mockDownloader{returnID: "sab-789"}

	handler := NewDownloadHandler(bus, downloadStore, nil, client, nil)

	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit GrabRequested with IsCompleteSeason=true (season pack)
	season := 1
	grab := &events.GrabRequested{
		BaseEvent:        events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:        1,
		Season:           &season,
		IsCompleteSeason: true,
		DownloadURL:      "https://example.com/test.nzb",
		ReleaseName:      "Breaking.Bad.S01.1080p.BluRay",
		Indexer:          "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Wait for DownloadCreated event
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(1), dc.ContentID)
		assert.Equal(t, "sab-789", dc.ClientID)
		require.NotNil(t, dc.Season)
		assert.Equal(t, 1, *dc.Season)
		assert.True(t, dc.IsCompleteSeason)
		assert.Empty(t, dc.EpisodeIDs) // No specific episodes for season pack
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	assert.True(t, client.addCalled)

	// Verify download was created with Season and IsCompleteSeason
	var storedSeason sql.NullInt64
	var isCompleteSeason bool
	err = db.QueryRow("SELECT season, is_complete_season FROM downloads WHERE content_id = 1").Scan(&storedSeason, &isCompleteSeason)
	require.NoError(t, err)
	assert.True(t, storedSeason.Valid)
	assert.Equal(t, int64(1), storedSeason.Int64)
	assert.True(t, isCompleteSeason)
}

func TestDownloadHandler_GrabSingleEpisodeBackwardCompat(t *testing.T) {
	db := setupDownloadTestDBWithEpisodes(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create series content and episode
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, root_path) VALUES (1, 'series', 'Breaking Bad', 2008, '/tv')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO episodes (id, content_id, season, episode, title, status) VALUES (101, 1, 1, 1, 'Pilot', 'wanted')`)
	require.NoError(t, err)

	downloadStore := download.NewStore(db)
	client := &mockDownloader{returnID: "sab-111"}

	handler := NewDownloadHandler(bus, downloadStore, nil, client, nil)

	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = handler.Start(ctx) }()

	time.Sleep(10 * time.Millisecond)

	// Emit GrabRequested with single EpisodeIDs (should set EpisodeID for backward compat)
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   1,
		EpisodeIDs:  []int64{101},
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Breaking.Bad.S01E01.1080p.BluRay",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, bus.Publish(ctx, grab))

	// Wait for DownloadCreated event
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(1), dc.ContentID)
		// Should have both EpisodeID (backward compat) and EpisodeIDs
		require.NotNil(t, dc.EpisodeID)
		assert.Equal(t, int64(101), *dc.EpisodeID)
		assert.Equal(t, []int64{101}, dc.EpisodeIDs)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	// Verify download was created with EpisodeID set (backward compat)
	var episodeID sql.NullInt64
	err = db.QueryRow("SELECT episode_id FROM downloads WHERE content_id = 1").Scan(&episodeID)
	require.NoError(t, err)
	assert.True(t, episodeID.Valid)
	assert.Equal(t, int64(101), episodeID.Int64)
}
