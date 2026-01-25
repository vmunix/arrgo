// internal/importer/importer_test.go
package importer

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/library"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupTestImporter(t *testing.T) (*Importer, *sql.DB, string, string) {
	t.Helper()

	db := setupTestDB(t)
	downloadDir := t.TempDir()
	movieRoot := t.TempDir()

	imp := &Importer{
		downloads:   download.NewStore(db),
		library:     library.NewStore(db),
		history:     NewHistoryStore(db),
		renamer:     NewRenamer("", ""),
		mediaServer: nil, // No Plex in tests
		movieRoot:   movieRoot,
		seriesRoot:  t.TempDir(),
		log:         testLogger(),
	}

	return imp, db, downloadDir, movieRoot
}

func createTestDownload(t *testing.T, db *sql.DB, contentID int64, status download.Status) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer, added_at, last_transition_at)
		VALUES (?, 'sabnzbd', 'nzo_test', ?, 'Test.Movie.2024.1080p.BluRay', 'TestIndexer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		contentID, status,
	)
	require.NoError(t, err, "create download")
	id, _ := result.LastInsertId()
	return id
}

func TestImporter_Import_Movie(t *testing.T) {
	imp, db, downloadDir, movieRoot := setupTestImporter(t)

	// Create content
	contentID := insertTestContent(t, db)

	// Create completed download
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "Test.Movie.2024.1080p.BluRay")
	require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")

	videoPath := filepath.Join(downloadPath, "test.movie.mkv")
	require.NoError(t, os.WriteFile(videoPath, make([]byte, 1000), 0644), "create video")

	// Import
	result, err := imp.Import(context.Background(), downloadID, downloadPath)
	require.NoError(t, err, "Import")

	// Verify result
	assert.NotZero(t, result.FileID, "FileID should be set")
	assert.Equal(t, videoPath, result.SourcePath)
	expectedDest := filepath.Join(movieRoot, "Test Movie (2024)", "Test Movie (2024) - 1080p.mkv")
	assert.Equal(t, expectedDest, result.DestPath)

	// Verify file was copied
	_, statErr := os.Stat(result.DestPath)
	assert.False(t, os.IsNotExist(statErr), "destination file should exist")

	// Verify download status updated
	var status string
	require.NoError(t, db.QueryRow("SELECT status FROM downloads WHERE id = ?", downloadID).Scan(&status), "query download status")
	assert.Equal(t, "imported", status)

	// Verify content status updated
	require.NoError(t, db.QueryRow("SELECT status FROM content WHERE id = ?", contentID).Scan(&status), "query content status")
	assert.Equal(t, "available", status)

	// Verify history entry
	entries, _, _ := imp.history.List(HistoryFilter{ContentID: &contentID})
	require.Len(t, entries, 1, "expected 1 history entry")
	assert.Equal(t, EventImported, entries[0].Event)

	// Verify file record
	var filePath string
	require.NoError(t, db.QueryRow("SELECT path FROM files WHERE content_id = ?", contentID).Scan(&filePath), "query file path")
	assert.Equal(t, result.DestPath, filePath)
}

func TestImporter_Import_DownloadNotFound(t *testing.T) {
	imp, _, _, _ := setupTestImporter(t)

	_, err := imp.Import(context.Background(), 9999, "/some/path")
	assert.ErrorIs(t, err, ErrDownloadNotFound)
}

func TestImporter_Import_NotCompleted(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	contentID := insertTestContent(t, db)
	downloadID := createTestDownload(t, db, contentID, download.StatusDownloading)

	_, err := imp.Import(context.Background(), downloadID, downloadDir)
	assert.ErrorIs(t, err, ErrDownloadNotReady)
}

func TestImporter_Import_NoVideoFile(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	contentID := insertTestContent(t, db)
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	downloadPath := filepath.Join(downloadDir, "empty")
	require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	assert.ErrorIs(t, err, ErrNoVideoFile)
}

func TestImporter_Import_PathTraversal(t *testing.T) {
	// Test that ValidatePath catches path traversal attempts.
	// Note: SanitizeFilename prevents most traversal via title,
	// so we test the ValidatePath function directly.
	t.Run("ValidatePath rejects traversal", func(t *testing.T) {
		root := "/movies"
		badPath := "/movies/../../../etc/passwd"
		err := ValidatePath(badPath, root)
		assert.ErrorIs(t, err, ErrPathTraversal)
	})

	t.Run("SanitizeFilename prevents traversal in title", func(t *testing.T) {
		imp, db, downloadDir, movieRoot := setupTestImporter(t)

		// Create content with malicious title - sanitization should handle it
		result, _ := db.Exec(`
			INSERT INTO content (type, title, year, status, quality_profile, root_path)
			VALUES ('movie', '../../../etc/passwd', 2024, 'wanted', 'hd', '/movies')`)
		contentID, _ := result.LastInsertId()

		downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

		downloadPath := filepath.Join(downloadDir, "download")
		require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")
		require.NoError(t, os.WriteFile(filepath.Join(downloadPath, "movie.mkv"), make([]byte, 100), 0644), "create video")

		// Import should succeed because SanitizeFilename cleans the title
		importResult, err := imp.Import(context.Background(), downloadID, downloadPath)
		require.NoError(t, err, "Import should succeed with sanitized title")

		// Verify the file is inside the movie root (not escaped)
		assert.True(t, strings.HasPrefix(importResult.DestPath, movieRoot),
			"DestPath %q should be inside movieRoot %q", importResult.DestPath, movieRoot)

		// Verify no file was created at a dangerous location
		_, statErr := os.Stat("/etc/passwd/etc passwd (2024)")
		assert.True(t, os.IsNotExist(statErr) || statErr != nil, "file should not be created outside movie root")
	})
}

func TestImporter_Import_DestinationExists(t *testing.T) {
	imp, db, downloadDir, movieRoot := setupTestImporter(t)

	contentID := insertTestContent(t, db)
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	// Create download with video
	downloadPath := filepath.Join(downloadDir, "download")
	require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")
	require.NoError(t, os.WriteFile(filepath.Join(downloadPath, "movie.mkv"), make([]byte, 100), 0644), "create video")

	// Pre-create destination
	destDir := filepath.Join(movieRoot, "Test Movie (2024)")
	require.NoError(t, os.MkdirAll(destDir, 0755), "create dest dir")
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "Test Movie (2024) - 1080p.mkv"), []byte("existing"), 0644), "create existing file")

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	assert.ErrorIs(t, err, ErrDestinationExists)
}

// Helper to create series content
func insertTestSeries(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', ?, 2024, 'wanted', 'hd', '/tv')`,
		title,
	)
	require.NoError(t, err, "insert test series")
	id, _ := result.LastInsertId()
	return id
}

// Helper to create episode
func insertTestEpisode(t *testing.T, db *sql.DB, contentID int64, season, episode int) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO episodes (content_id, season, episode, title, status)
		VALUES (?, ?, ?, 'Test Episode', 'wanted')`,
		contentID, season, episode,
	)
	require.NoError(t, err, "insert test episode")
	id, _ := result.LastInsertId()
	return id
}

// Helper to create download with episode ID
func createTestEpisodeDownload(t *testing.T, db *sql.DB, contentID, episodeID int64, status download.Status) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO downloads (content_id, episode_id, client, client_id, status, release_name, indexer, added_at, last_transition_at)
		VALUES (?, ?, 'sabnzbd', 'nzo_test', ?, 'Test.Show.S01E05.1080p.WEB', 'TestIndexer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		contentID, episodeID, status,
	)
	require.NoError(t, err, "create episode download")
	id, _ := result.LastInsertId()
	return id
}

func TestImporter_Import_Episode(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	// Create series and episode
	seriesID := insertTestSeries(t, db, "Test Show")
	episodeID := insertTestEpisode(t, db, seriesID, 1, 5)

	// Create completed download with episode ID
	downloadID := createTestEpisodeDownload(t, db, seriesID, episodeID, download.StatusCompleted)

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "Test.Show.S01E05.1080p.WEB")
	require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")
	videoPath := filepath.Join(downloadPath, "test.show.s01e05.mkv")
	require.NoError(t, os.WriteFile(videoPath, make([]byte, 1000), 0644), "create video")

	// Import
	result, err := imp.Import(context.Background(), downloadID, downloadPath)
	require.NoError(t, err, "Import")

	// Verify destination path uses series template
	assert.Contains(t, result.DestPath, "Test Show")
	assert.Contains(t, result.DestPath, "Season 01")
	assert.Contains(t, result.DestPath, "S01E05")

	// Verify file was copied
	_, statErr := os.Stat(result.DestPath)
	assert.False(t, os.IsNotExist(statErr), "destination file should exist")

	// Verify episode status updated to available
	var epStatus string
	require.NoError(t, db.QueryRow("SELECT status FROM episodes WHERE id = ?", episodeID).Scan(&epStatus), "query episode status")
	assert.Equal(t, "available", epStatus)

	// Verify series status unchanged (still wanted)
	var seriesStatus string
	require.NoError(t, db.QueryRow("SELECT status FROM content WHERE id = ?", seriesID).Scan(&seriesStatus), "query series status")
	assert.Equal(t, "wanted", seriesStatus, "series status should remain unchanged")

	// Verify file record has episode ID
	var fileEpisodeID sql.NullInt64
	require.NoError(t, db.QueryRow("SELECT episode_id FROM files WHERE content_id = ?", seriesID).Scan(&fileEpisodeID), "query file episode_id")
	assert.True(t, fileEpisodeID.Valid)
	assert.Equal(t, episodeID, fileEpisodeID.Int64)

	// Verify history entry has episode ID
	entries, _, _ := imp.history.List(HistoryFilter{ContentID: &seriesID})
	require.Len(t, entries, 1, "expected 1 history entry")
	require.NotNil(t, entries[0].EpisodeID)
	assert.Equal(t, episodeID, *entries[0].EpisodeID)
}

func TestImporter_Import_Episode_NoEpisodeID(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	// Create series (no episode)
	seriesID := insertTestSeries(t, db, "Test Show")

	// Create download WITHOUT episode ID
	result, err := db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer, added_at, last_transition_at)
		VALUES (?, 'sabnzbd', 'nzo_test', 'completed', 'Test.Show.S01E05.1080p', 'Indexer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		seriesID,
	)
	require.NoError(t, err, "create download")
	downloadID, _ := result.LastInsertId()

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "download")
	require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")
	require.NoError(t, os.WriteFile(filepath.Join(downloadPath, "episode.mkv"), make([]byte, 100), 0644), "create video")

	_, err = imp.Import(context.Background(), downloadID, downloadPath)
	assert.ErrorIs(t, err, ErrEpisodeNotSpecified)
}

func TestImporter_Import_Episode_EpisodeNotFound(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	// Create series and episode
	seriesID := insertTestSeries(t, db, "Test Show")
	episodeID := insertTestEpisode(t, db, seriesID, 1, 5)

	// Create download with valid episode ID
	downloadID := createTestEpisodeDownload(t, db, seriesID, episodeID, download.StatusCompleted)

	// Disable foreign keys temporarily to simulate data inconsistency
	_, err := db.Exec("PRAGMA foreign_keys = OFF")
	require.NoError(t, err, "disable foreign keys")
	defer func() { _, _ = db.Exec("PRAGMA foreign_keys = ON") }()

	// Delete the episode (with FK disabled, download stays)
	_, err = db.Exec("DELETE FROM episodes WHERE id = ?", episodeID)
	require.NoError(t, err, "delete episode")

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "download")
	require.NoError(t, os.MkdirAll(downloadPath, 0755), "create download dir")
	require.NoError(t, os.WriteFile(filepath.Join(downloadPath, "episode.mkv"), make([]byte, 100), 0644), "create video")

	_, err = imp.Import(context.Background(), downloadID, downloadPath)
	require.Error(t, err, "expected error for non-existent episode")
	assert.Contains(t, err.Error(), "get episode", "expected 'get episode' error")
}
