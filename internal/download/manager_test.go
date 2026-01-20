package download_test

import (
	"context"
	"database/sql"
	_ "embed"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/download/mocks"
	"go.uber.org/mock/gomock"
	_ "modernc.org/sqlite"
)

//go:embed testdata/schema.sql
var testSchema string

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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

func TestManager_Grab(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Add(gomock.Any(), "http://example.com/test.nzb", "").
		Return("nzo_abc123", nil)

	mgr := download.NewManager(client, store, testLogger())

	d, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024.1080p", "TestIndexer")

	require.NoError(t, err)
	assert.Equal(t, "nzo_abc123", d.ClientID)
	assert.Equal(t, download.StatusQueued, d.Status)
	assert.NotZero(t, d.ID, "download should be saved to DB")
}

func TestManager_Grab_WithEpisodeID(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)

	// Create series
	result, err := db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', 'Test Show', 2024, 'wanted', 'hd', '/tv')`)
	require.NoError(t, err)
	contentID, _ := result.LastInsertId()

	// Create episode
	epResult, err := db.Exec(`INSERT INTO episodes (content_id, season, episode, title, status)
		VALUES (?, 1, 1, 'Pilot', 'wanted')`, contentID)
	require.NoError(t, err)
	episodeID, _ := epResult.LastInsertId()

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Add(gomock.Any(), "http://example.com/ep.nzb", "").
		Return("nzo_ep1", nil)

	mgr := download.NewManager(client, store, testLogger())

	d, err := mgr.Grab(context.Background(), contentID, &episodeID, "http://example.com/ep.nzb", "Test.Show.S01E01", "Indexer")

	require.NoError(t, err)
	require.NotNil(t, d.EpisodeID)
	assert.Equal(t, episodeID, *d.EpisodeID)
}

func TestManager_Grab_ClientError(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Add(gomock.Any(), "http://example.com/test.nzb", "").
		Return("", download.ErrClientUnavailable)

	mgr := download.NewManager(client, store, testLogger())

	_, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie", "Indexer")

	require.ErrorIs(t, err, download.ErrClientUnavailable)

	// Should not have saved to DB
	downloads, _ := store.List(download.Filter{})
	assert.Empty(t, downloads, "download should not be in DB after client error")
}

func TestManager_Grab_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	client := mocks.NewMockDownloader(ctrl)
	// First grab returns nzo_abc123
	client.EXPECT().
		Add(gomock.Any(), "http://example.com/test.nzb", "").
		Return("nzo_abc123", nil)
	// Second grab calls Add again (returns different ID), but store returns existing record
	client.EXPECT().
		Add(gomock.Any(), "http://example.com/test.nzb", "").
		Return("nzo_different", nil)

	mgr := download.NewManager(client, store, testLogger())

	d1, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024", "Indexer")
	require.NoError(t, err)

	// Second grab with same content + release should be idempotent (returns same DB record)
	d2, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024", "Indexer")
	require.NoError(t, err)

	assert.Equal(t, d1.ID, d2.ID, "expected same download ID")
}

func TestManager_Refresh(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	// Add a download
	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Status(gomock.Any(), "nzo_abc123").
		Return(&download.ClientStatus{
			ID:       "nzo_abc123",
			Status:   download.StatusCompleted,
			Progress: 100,
			Path:     "/complete/Test.Movie",
		}, nil)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Refresh(context.Background())
	require.NoError(t, err)

	// Should have updated status in DB
	updated, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusCompleted, updated.Status)
	assert.NotNil(t, updated.CompletedAt, "CompletedAt should be set")
}

func TestManager_Refresh_NoChange(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Status(gomock.Any(), "nzo_abc123").
		Return(&download.ClientStatus{
			ID:       "nzo_abc123",
			Status:   download.StatusDownloading,
			Progress: 50,
		}, nil)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Refresh(context.Background())
	require.NoError(t, err)

	// Status should remain unchanged
	updated, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusDownloading, updated.Status)
	assert.Nil(t, updated.CompletedAt, "CompletedAt should not be set")
}

func TestManager_Refresh_Failed(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Status(gomock.Any(), "nzo_abc123").
		Return(&download.ClientStatus{
			ID:     "nzo_abc123",
			Status: download.StatusFailed,
		}, nil)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Refresh(context.Background())
	require.NoError(t, err)

	updated, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusFailed, updated.Status)
	assert.NotNil(t, updated.CompletedAt, "CompletedAt should be set for failed downloads")
}

func TestManager_Cancel(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Remove(gomock.Any(), "nzo_abc123", false).
		Return(nil)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Cancel(context.Background(), d.ID, false)
	require.NoError(t, err)

	// Should be deleted from DB
	_, err = store.Get(d.ID)
	require.ErrorIs(t, err, download.ErrNotFound)
}

func TestManager_Cancel_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)

	client := mocks.NewMockDownloader(ctrl)
	// No expectations - Remove should not be called

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Cancel(context.Background(), 9999, false)
	require.ErrorIs(t, err, download.ErrNotFound)
}

func TestManager_Cancel_ClientError(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	// Client error should not prevent DB deletion
	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Remove(gomock.Any(), "nzo_abc123", false).
		Return(download.ErrClientUnavailable)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Cancel(context.Background(), d.ID, false)
	require.NoError(t, err, "Cancel should succeed despite client error")

	// Should still be deleted from DB
	_, err = store.Get(d.ID)
	require.ErrorIs(t, err, download.ErrNotFound)
}

func TestManager_GetActive(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Status(gomock.Any(), "nzo_abc123").
		Return(&download.ClientStatus{
			ID:       "nzo_abc123",
			Status:   download.StatusDownloading,
			Progress: 50,
			Speed:    10000000,
			ETA:      5 * time.Minute,
		}, nil)

	mgr := download.NewManager(client, store, testLogger())

	active, err := mgr.GetActive(context.Background())
	require.NoError(t, err)
	require.Len(t, active, 1, "expected 1 active download")

	assert.Equal(t, d.ID, active[0].Download.ID)
	require.NotNil(t, active[0].Live, "Live status should be set")
	assert.InDelta(t, 50, active[0].Live.Progress, 0.001)
}

func TestManager_GetActive_ClientError(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	require.NoError(t, store.Add(d))

	// Client error should still return download without live status
	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Status(gomock.Any(), "nzo_abc123").
		Return(nil, download.ErrClientUnavailable)

	mgr := download.NewManager(client, store, testLogger())

	active, err := mgr.GetActive(context.Background())
	require.NoError(t, err)
	require.Len(t, active, 1, "expected 1 active download")

	assert.Equal(t, d.ID, active[0].Download.ID)
	assert.Nil(t, active[0].Live, "Live status should be nil when client errors")
}

func TestManager_GetActive_ExcludesTerminal(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	// Add active download
	d1 := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_1",
		Status:      download.StatusDownloading,
		ReleaseName: "Active.Movie",
	}
	require.NoError(t, store.Add(d1))

	// Add imported download (should be included - not terminal)
	d2 := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_2",
		Status:      download.StatusImported,
		ReleaseName: "Imported.Movie",
	}
	require.NoError(t, store.Add(d2))

	// Add cleaned download (should be excluded - terminal)
	d3 := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_3",
		Status:      download.StatusCleaned,
		ReleaseName: "Cleaned.Movie",
	}
	require.NoError(t, store.Add(d3))

	client := mocks.NewMockDownloader(ctrl)
	// Expect Status calls for non-terminal downloads only
	client.EXPECT().
		Status(gomock.Any(), "nzo_1").
		Return(&download.ClientStatus{
			ID:       "nzo_1",
			Status:   download.StatusDownloading,
			Progress: 50,
		}, nil)
	client.EXPECT().
		Status(gomock.Any(), "nzo_2").
		Return(&download.ClientStatus{
			ID:       "nzo_2",
			Status:   download.StatusImported,
			Progress: 100,
		}, nil)

	mgr := download.NewManager(client, store, testLogger())

	active, err := mgr.GetActive(context.Background())
	require.NoError(t, err)
	require.Len(t, active, 2, "expected 2 active downloads (excluding terminal states)")

	// Verify no terminal status in results
	for _, a := range active {
		assert.NotEqual(t, download.StatusCleaned, a.Download.Status, "GetActive should exclude cleaned status")
		assert.NotEqual(t, download.StatusFailed, a.Download.Status, "GetActive should exclude failed status")
	}
}
