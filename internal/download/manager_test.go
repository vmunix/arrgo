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

// --- Cancel State Tests ---

func TestManager_Cancel_FromQueued(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_queued",
		Status:      download.StatusQueued,
		ReleaseName: "Queued.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Remove(gomock.Any(), "nzo_queued", false).
		Return(nil)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Cancel(context.Background(), d.ID, false)
	require.NoError(t, err)

	// Verify deleted from DB
	_, err = store.Get(d.ID)
	require.ErrorIs(t, err, download.ErrNotFound)
}

func TestManager_Cancel_FromDownloading(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_downloading",
		Status:      download.StatusDownloading,
		ReleaseName: "Downloading.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Remove(gomock.Any(), "nzo_downloading", false).
		Return(nil)

	mgr := download.NewManager(client, store, testLogger())

	err := mgr.Cancel(context.Background(), d.ID, false)
	require.NoError(t, err)

	// Verify deleted from DB
	_, err = store.Get(d.ID)
	require.ErrorIs(t, err, download.ErrNotFound)
}

func TestManager_Cancel_FromCompleted_WithDeleteFiles(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := download.NewStore(db)
	contentID := insertTestContent(t, db)

	d := &download.Download{
		ContentID:   contentID,
		Client:      download.ClientSABnzbd,
		ClientID:    "nzo_completed",
		Status:      download.StatusCompleted,
		ReleaseName: "Completed.Movie",
	}
	require.NoError(t, store.Add(d))

	client := mocks.NewMockDownloader(ctrl)
	// Expect Remove called with deleteFiles=true
	client.EXPECT().
		Remove(gomock.Any(), "nzo_completed", true).
		Return(nil)

	mgr := download.NewManager(client, store, testLogger())

	// Cancel with deleteFiles=true
	err := mgr.Cancel(context.Background(), d.ID, true)
	require.NoError(t, err)

	// Verify deleted from DB
	_, err = store.Get(d.ID)
	require.ErrorIs(t, err, download.ErrNotFound)
}
