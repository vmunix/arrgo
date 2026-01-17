package library

import (
	"errors"
	"testing"
	"time"
)

// createTestMovie creates a movie Content for file tests
func createTestMovie(t *testing.T, store *Store) *Content {
	t.Helper()
	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("create test movie: %v", err)
	}
	return c
}

func TestStore_AddFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	before := time.Now()
	if err := store.AddFile(f); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	after := time.Now()

	// ID should be set
	if f.ID == 0 {
		t.Error("ID should be set after AddFile")
	}

	// AddedAt should be set
	if f.AddedAt.Before(before) || f.AddedAt.After(after) {
		t.Errorf("AddedAt %v not in expected range [%v, %v]", f.AddedAt, before, after)
	}
}

func TestStore_AddFile_DuplicatePath(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	f1 := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	if err := store.AddFile(f1); err != nil {
		t.Fatalf("AddFile first: %v", err)
	}

	// Try to add duplicate (same path)
	f2 := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 4294967296,
		Quality:   "720p webdl",
		Source:    "torrent",
	}

	err := store.AddFile(f2)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("AddFile duplicate error = %v, want ErrDuplicate", err)
	}
}

func TestStore_GetFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	original := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}
	if err := store.AddFile(original); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	retrieved, err := store.GetFile(original.ID)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	// Verify all fields
	if retrieved.ID != original.ID {
		t.Errorf("ID = %d, want %d", retrieved.ID, original.ID)
	}
	if retrieved.ContentID != original.ContentID {
		t.Errorf("ContentID = %d, want %d", retrieved.ContentID, original.ContentID)
	}
	if retrieved.EpisodeID != nil {
		t.Errorf("EpisodeID = %v, want nil", retrieved.EpisodeID)
	}
	if retrieved.Path != original.Path {
		t.Errorf("Path = %q, want %q", retrieved.Path, original.Path)
	}
	if retrieved.SizeBytes != original.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", retrieved.SizeBytes, original.SizeBytes)
	}
	if retrieved.Quality != original.Quality {
		t.Errorf("Quality = %q, want %q", retrieved.Quality, original.Quality)
	}
	if retrieved.Source != original.Source {
		t.Errorf("Source = %q, want %q", retrieved.Source, original.Source)
	}
}

func TestStore_GetFile_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetFile(9999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFile(9999) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListFiles_FilterByContentID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create two movies
	movie1 := createTestMovie(t, store)
	movie2 := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(680)),
		Title:          "Pulp Fiction",
		Year:           1994,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(movie2); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	// Add files to both movies
	if err := store.AddFile(&File{ContentID: movie1.ID, Path: "/movies/fight1.mkv", Quality: "1080p bluray"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := store.AddFile(&File{ContentID: movie1.ID, Path: "/movies/fight2.mkv", Quality: "720p webdl"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := store.AddFile(&File{ContentID: movie2.ID, Path: "/movies/pulp1.mkv", Quality: "1080p bluray"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Filter by ContentID
	results, total, err := store.ListFiles(FileFilter{ContentID: &movie1.ID})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	for _, f := range results {
		if f.ContentID != movie1.ID {
			t.Errorf("file ContentID = %d, want %d", f.ContentID, movie1.ID)
		}
	}
}

func TestStore_ListFiles_Pagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	// Add 5 files
	for i := 1; i <= 5; i++ {
		f := &File{
			ContentID: movie.ID,
			Path:      "/movies/file" + string(rune('0'+i)) + ".mkv",
			Quality:   "1080p bluray",
		}
		if err := store.AddFile(f); err != nil {
			t.Fatalf("AddFile: %v", err)
		}
	}

	// Get page 1 (first 2)
	results, total, err := store.ListFiles(FileFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Get page 2 (next 2)
	results2, total2, err := store.ListFiles(FileFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if total2 != 5 {
		t.Errorf("total = %d, want 5", total2)
	}
	if len(results2) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results2))
	}

	// Results should be different
	if results[0].ID == results2[0].ID {
		t.Error("pagination should return different items")
	}
}

func TestStore_UpdateFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.720p.WebDL.mkv",
		SizeBytes: 4294967296,
		Quality:   "720p webdl",
		Source:    "torrent",
	}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Update the file
	f.Quality = "1080p bluray"
	f.Source = "usenet"
	f.SizeBytes = 8589934592

	if err := store.UpdateFile(f); err != nil {
		t.Fatalf("UpdateFile: %v", err)
	}

	// Verify in database
	retrieved, err := store.GetFile(f.ID)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if retrieved.Quality != "1080p bluray" {
		t.Errorf("Quality = %q, want 1080p bluray", retrieved.Quality)
	}
	if retrieved.Source != "usenet" {
		t.Errorf("Source = %q, want usenet", retrieved.Source)
	}
	if retrieved.SizeBytes != 8589934592 {
		t.Errorf("SizeBytes = %d, want 8589934592", retrieved.SizeBytes)
	}
}

func TestStore_UpdateFile_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	f := &File{
		ID:        9999,
		ContentID: movie.ID,
		Path:      "/movies/nonexistent.mkv",
		Quality:   "1080p bluray",
	}

	err := store.UpdateFile(f)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateFile error = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Delete
	if err := store.DeleteFile(f.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	// Verify deleted
	_, err := store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFile after delete: error = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteFile_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	if err := store.DeleteFile(9999); err != nil {
		t.Errorf("DeleteFile(9999) = %v, want nil (idempotent)", err)
	}
}

func TestStore_AddFile_ForEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	// Create an episode
	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}
	if err := store.AddEpisode(e); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// Add file for the episode
	f := &File{
		ContentID: series.ID,
		EpisodeID: &e.ID,
		Path:      "/tv/Breaking Bad/Season 1/Breaking.Bad.S01E01.Pilot.1080p.BluRay.mkv",
		SizeBytes: 4294967296,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	if err := store.AddFile(f); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	if f.ID == 0 {
		t.Error("ID should be set after AddFile")
	}

	// Retrieve and verify EpisodeID
	retrieved, err := store.GetFile(f.ID)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if retrieved.EpisodeID == nil {
		t.Error("EpisodeID should not be nil")
	} else if *retrieved.EpisodeID != e.ID {
		t.Errorf("EpisodeID = %d, want %d", *retrieved.EpisodeID, e.ID)
	}
}

func TestStore_ListFiles_FilterByEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	// Create two episodes
	e1 := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	e2 := &Episode{ContentID: series.ID, Season: 1, Episode: 2, Title: "Cat's in the Bag", Status: StatusWanted}
	if err := store.AddEpisode(e1); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := store.AddEpisode(e2); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// Add files for each episode
	if err := store.AddFile(&File{ContentID: series.ID, EpisodeID: &e1.ID, Path: "/tv/s01e01.mkv", Quality: "1080p bluray"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := store.AddFile(&File{ContentID: series.ID, EpisodeID: &e2.ID, Path: "/tv/s01e02.mkv", Quality: "1080p bluray"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Filter by EpisodeID
	results, total, err := store.ListFiles(FileFilter{EpisodeID: &e1.ID})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].EpisodeID == nil || *results[0].EpisodeID != e1.ID {
		t.Errorf("EpisodeID = %v, want %d", results[0].EpisodeID, e1.ID)
	}
}

func TestStore_ListFiles_FilterByQuality(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	// Add files with different qualities
	if err := store.AddFile(&File{ContentID: movie.ID, Path: "/movies/file1.mkv", Quality: "1080p bluray"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := store.AddFile(&File{ContentID: movie.ID, Path: "/movies/file2.mkv", Quality: "720p webdl"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := store.AddFile(&File{ContentID: movie.ID, Path: "/movies/file3.mkv", Quality: "1080p bluray"}); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Filter by quality
	quality := "1080p bluray"
	results, total, err := store.ListFiles(FileFilter{Quality: &quality})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	for _, f := range results {
		if f.Quality != "1080p bluray" {
			t.Errorf("Quality = %q, want 1080p bluray", f.Quality)
		}
	}
}

func TestTx_AddFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	if err := tx.AddFile(f); err != nil {
		t.Fatalf("tx.AddFile: %v", err)
	}

	if f.ID == 0 {
		t.Error("ID should be set")
	}

	// Should be visible within transaction
	retrieved, err := tx.GetFile(f.ID)
	if err != nil {
		t.Fatalf("tx.GetFile: %v", err)
	}
	if retrieved.Path != f.Path {
		t.Errorf("Path = %q, want %q", retrieved.Path, f.Path)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Should be visible after commit
	retrieved, err = store.GetFile(f.ID)
	if err != nil {
		t.Fatalf("store.GetFile after commit: %v", err)
	}
	if retrieved.Path != f.Path {
		t.Errorf("Path = %q, want %q", retrieved.Path, f.Path)
	}
}

func TestTx_Rollback_File(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	if err := tx.AddFile(f); err != nil {
		t.Fatalf("tx.AddFile: %v", err)
	}

	id := f.ID

	// Rollback
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Should NOT be visible after rollback
	_, err = store.GetFile(id)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFile after rollback: error = %v, want ErrNotFound", err)
	}
}
