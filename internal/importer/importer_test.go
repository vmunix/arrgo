// internal/importer/importer_test.go
package importer

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		downloads:  download.NewStore(db),
		library:    library.NewStore(db),
		history:    NewHistoryStore(db),
		renamer:    NewRenamer("", ""),
		plex:       nil, // No Plex in tests
		movieRoot:  movieRoot,
		seriesRoot: t.TempDir(),
		log:        testLogger(),
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
	if err != nil {
		t.Fatalf("create download: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func TestImporter_Import_Movie(t *testing.T) {
	imp, db, downloadDir, movieRoot := setupTestImporter(t)

	// Create content
	contentID := insertTestContent(t, db, "Test Movie")

	// Create completed download
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "Test.Movie.2024.1080p.BluRay")
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}

	videoPath := filepath.Join(downloadPath, "test.movie.mkv")
	if err := os.WriteFile(videoPath, make([]byte, 1000), 0644); err != nil {
		t.Fatalf("create video: %v", err)
	}

	// Import
	result, err := imp.Import(context.Background(), downloadID, downloadPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Verify result
	if result.FileID == 0 {
		t.Error("FileID should be set")
	}
	if result.SourcePath != videoPath {
		t.Errorf("SourcePath = %q, want %q", result.SourcePath, videoPath)
	}
	expectedDest := filepath.Join(movieRoot, "Test Movie (2024)", "Test Movie (2024) - 1080p.mkv")
	if result.DestPath != expectedDest {
		t.Errorf("DestPath = %q, want %q", result.DestPath, expectedDest)
	}

	// Verify file was copied
	if _, err := os.Stat(result.DestPath); os.IsNotExist(err) {
		t.Error("destination file should exist")
	}

	// Verify download status updated
	var status string
	if err := db.QueryRow("SELECT status FROM downloads WHERE id = ?", downloadID).Scan(&status); err != nil {
		t.Fatalf("query download status: %v", err)
	}
	if status != "imported" {
		t.Errorf("download status = %q, want imported", status)
	}

	// Verify content status updated
	if err := db.QueryRow("SELECT status FROM content WHERE id = ?", contentID).Scan(&status); err != nil {
		t.Fatalf("query content status: %v", err)
	}
	if status != "available" {
		t.Errorf("content status = %q, want available", status)
	}

	// Verify history entry
	entries, _ := imp.history.List(HistoryFilter{ContentID: &contentID})
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	if entries[0].Event != EventImported {
		t.Errorf("history event = %q, want imported", entries[0].Event)
	}

	// Verify file record
	var filePath string
	if err := db.QueryRow("SELECT path FROM files WHERE content_id = ?", contentID).Scan(&filePath); err != nil {
		t.Fatalf("query file path: %v", err)
	}
	if filePath != result.DestPath {
		t.Errorf("file path = %q, want %q", filePath, result.DestPath)
	}
}

func TestImporter_Import_DownloadNotFound(t *testing.T) {
	imp, _, _, _ := setupTestImporter(t)

	_, err := imp.Import(context.Background(), 9999, "/some/path")
	if !errors.Is(err, ErrDownloadNotFound) {
		t.Errorf("expected ErrDownloadNotFound, got %v", err)
	}
}

func TestImporter_Import_NotCompleted(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	contentID := insertTestContent(t, db, "Test Movie")
	downloadID := createTestDownload(t, db, contentID, download.StatusDownloading)

	_, err := imp.Import(context.Background(), downloadID, downloadDir)
	if !errors.Is(err, ErrDownloadNotReady) {
		t.Errorf("expected ErrDownloadNotReady, got %v", err)
	}
}

func TestImporter_Import_NoVideoFile(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	contentID := insertTestContent(t, db, "Test Movie")
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	downloadPath := filepath.Join(downloadDir, "empty")
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	if !errors.Is(err, ErrNoVideoFile) {
		t.Errorf("expected ErrNoVideoFile, got %v", err)
	}
}

func TestImporter_Import_PathTraversal(t *testing.T) {
	// Test that ValidatePath catches path traversal attempts.
	// Note: SanitizeFilename prevents most traversal via title,
	// so we test the ValidatePath function directly.
	t.Run("ValidatePath rejects traversal", func(t *testing.T) {
		root := "/movies"
		badPath := "/movies/../../../etc/passwd"
		err := ValidatePath(badPath, root)
		if !errors.Is(err, ErrPathTraversal) {
			t.Errorf("ValidatePath should reject traversal, got %v", err)
		}
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
		if err := os.MkdirAll(downloadPath, 0755); err != nil {
			t.Fatalf("create download dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(downloadPath, "movie.mkv"), make([]byte, 100), 0644); err != nil {
			t.Fatalf("create video: %v", err)
		}

		// Import should succeed because SanitizeFilename cleans the title
		importResult, err := imp.Import(context.Background(), downloadID, downloadPath)
		if err != nil {
			t.Fatalf("Import should succeed with sanitized title: %v", err)
		}

		// Verify the file is inside the movie root (not escaped)
		if !strings.HasPrefix(importResult.DestPath, movieRoot) {
			t.Errorf("DestPath %q should be inside movieRoot %q", importResult.DestPath, movieRoot)
		}

		// Verify no file was created at a dangerous location
		if _, err := os.Stat("/etc/passwd/etc passwd (2024)"); err == nil {
			t.Error("file should not be created outside movie root")
		}
	})
}

func TestImporter_Import_DestinationExists(t *testing.T) {
	imp, db, downloadDir, movieRoot := setupTestImporter(t)

	contentID := insertTestContent(t, db, "Test Movie")
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	// Create download with video
	downloadPath := filepath.Join(downloadDir, "download")
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(downloadPath, "movie.mkv"), make([]byte, 100), 0644); err != nil {
		t.Fatalf("create video: %v", err)
	}

	// Pre-create destination
	destDir := filepath.Join(movieRoot, "Test Movie (2024)")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("create dest dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "Test Movie (2024) - 1080p.mkv"), []byte("existing"), 0644); err != nil {
		t.Fatalf("create existing file: %v", err)
	}

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	if !errors.Is(err, ErrDestinationExists) {
		t.Errorf("expected ErrDestinationExists, got %v", err)
	}
}

// Helper to create series content
func insertTestSeries(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', ?, 2024, 'wanted', 'hd', '/tv')`,
		title,
	)
	if err != nil {
		t.Fatalf("insert test series: %v", err)
	}
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
	if err != nil {
		t.Fatalf("insert test episode: %v", err)
	}
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
	if err != nil {
		t.Fatalf("create episode download: %v", err)
	}
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
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}
	videoPath := filepath.Join(downloadPath, "test.show.s01e05.mkv")
	if err := os.WriteFile(videoPath, make([]byte, 1000), 0644); err != nil {
		t.Fatalf("create video: %v", err)
	}

	// Import
	result, err := imp.Import(context.Background(), downloadID, downloadPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Verify destination path uses series template
	if !strings.Contains(result.DestPath, "Test Show") {
		t.Errorf("DestPath should contain series title: %s", result.DestPath)
	}
	if !strings.Contains(result.DestPath, "Season 01") {
		t.Errorf("DestPath should contain season folder: %s", result.DestPath)
	}
	if !strings.Contains(result.DestPath, "S01E05") {
		t.Errorf("DestPath should contain episode number: %s", result.DestPath)
	}

	// Verify file was copied
	if _, err := os.Stat(result.DestPath); os.IsNotExist(err) {
		t.Error("destination file should exist")
	}

	// Verify episode status updated to available
	var epStatus string
	if err := db.QueryRow("SELECT status FROM episodes WHERE id = ?", episodeID).Scan(&epStatus); err != nil {
		t.Fatalf("query episode status: %v", err)
	}
	if epStatus != "available" {
		t.Errorf("episode status = %q, want available", epStatus)
	}

	// Verify series status unchanged (still wanted)
	var seriesStatus string
	if err := db.QueryRow("SELECT status FROM content WHERE id = ?", seriesID).Scan(&seriesStatus); err != nil {
		t.Fatalf("query series status: %v", err)
	}
	if seriesStatus != "wanted" {
		t.Errorf("series status = %q, want wanted (unchanged)", seriesStatus)
	}

	// Verify file record has episode ID
	var fileEpisodeID sql.NullInt64
	if err := db.QueryRow("SELECT episode_id FROM files WHERE content_id = ?", seriesID).Scan(&fileEpisodeID); err != nil {
		t.Fatalf("query file episode_id: %v", err)
	}
	if !fileEpisodeID.Valid || fileEpisodeID.Int64 != episodeID {
		t.Errorf("file episode_id = %v, want %d", fileEpisodeID, episodeID)
	}

	// Verify history entry has episode ID
	entries, _ := imp.history.List(HistoryFilter{ContentID: &seriesID})
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	if entries[0].EpisodeID == nil || *entries[0].EpisodeID != episodeID {
		t.Errorf("history episode_id = %v, want %d", entries[0].EpisodeID, episodeID)
	}
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
	if err != nil {
		t.Fatalf("create download: %v", err)
	}
	downloadID, _ := result.LastInsertId()

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "download")
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(downloadPath, "episode.mkv"), make([]byte, 100), 0644); err != nil {
		t.Fatalf("create video: %v", err)
	}

	_, err = imp.Import(context.Background(), downloadID, downloadPath)
	if !errors.Is(err, ErrEpisodeNotSpecified) {
		t.Errorf("expected ErrEpisodeNotSpecified, got %v", err)
	}
}

func TestImporter_Import_Episode_EpisodeNotFound(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	// Create series and episode
	seriesID := insertTestSeries(t, db, "Test Show")
	episodeID := insertTestEpisode(t, db, seriesID, 1, 5)

	// Create download with valid episode ID
	downloadID := createTestEpisodeDownload(t, db, seriesID, episodeID, download.StatusCompleted)

	// Disable foreign keys temporarily to simulate data inconsistency
	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	defer func() { _, _ = db.Exec("PRAGMA foreign_keys = ON") }()

	// Delete the episode (with FK disabled, download stays)
	if _, err := db.Exec("DELETE FROM episodes WHERE id = ?", episodeID); err != nil {
		t.Fatalf("delete episode: %v", err)
	}

	// Create download directory with video
	downloadPath := filepath.Join(downloadDir, "download")
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(downloadPath, "episode.mkv"), make([]byte, 100), 0644); err != nil {
		t.Fatalf("create video: %v", err)
	}

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	if err == nil {
		t.Error("expected error for non-existent episode")
	}
	if !strings.Contains(err.Error(), "get episode") {
		t.Errorf("expected 'get episode' error, got %v", err)
	}
}
