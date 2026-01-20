package library

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, store.AddContent(c), "create test movie should succeed")
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
	require.NoError(t, store.AddFile(f), "AddFile should succeed")
	after := time.Now()

	// ID should be set
	assert.NotZero(t, f.ID, "ID should be set after AddFile")

	// AddedAt should be set
	assert.False(t, f.AddedAt.Before(before) || f.AddedAt.After(after),
		"AddedAt %v not in expected range [%v, %v]", f.AddedAt, before, after)
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

	require.NoError(t, store.AddFile(f1), "AddFile first should succeed")

	// Try to add duplicate (same path)
	f2 := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 4294967296,
		Quality:   "720p webdl",
		Source:    "torrent",
	}

	err := store.AddFile(f2)
	assert.ErrorIs(t, err, ErrDuplicate, "AddFile duplicate should return ErrDuplicate")
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
	require.NoError(t, store.AddFile(original), "AddFile should succeed")

	retrieved, err := store.GetFile(original.ID)
	require.NoError(t, err, "GetFile should succeed")

	// Verify all fields
	assert.Equal(t, original.ID, retrieved.ID)
	assert.Equal(t, original.ContentID, retrieved.ContentID)
	assert.Nil(t, retrieved.EpisodeID, "EpisodeID should be nil")
	assert.Equal(t, original.Path, retrieved.Path)
	assert.Equal(t, original.SizeBytes, retrieved.SizeBytes)
	assert.Equal(t, original.Quality, retrieved.Quality)
	assert.Equal(t, original.Source, retrieved.Source)
}

func TestStore_GetFile_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetFile(9999)
	assert.ErrorIs(t, err, ErrNotFound, "GetFile(9999) should return ErrNotFound")
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
	require.NoError(t, store.AddContent(movie2), "AddContent should succeed")

	// Add files to both movies
	require.NoError(t, store.AddFile(&File{ContentID: movie1.ID, Path: "/movies/fight1.mkv", Quality: "1080p bluray"}), "AddFile should succeed")
	require.NoError(t, store.AddFile(&File{ContentID: movie1.ID, Path: "/movies/fight2.mkv", Quality: "720p webdl"}), "AddFile should succeed")
	require.NoError(t, store.AddFile(&File{ContentID: movie2.ID, Path: "/movies/pulp1.mkv", Quality: "1080p bluray"}), "AddFile should succeed")

	// Filter by ContentID
	results, total, err := store.ListFiles(FileFilter{ContentID: &movie1.ID})
	require.NoError(t, err, "ListFiles should succeed")

	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	for _, f := range results {
		assert.Equal(t, movie1.ID, f.ContentID, "file ContentID should match")
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
		require.NoError(t, store.AddFile(f), "AddFile should succeed")
	}

	// Get page 1 (first 2)
	results, total, err := store.ListFiles(FileFilter{Limit: 2, Offset: 0})
	require.NoError(t, err, "ListFiles should succeed")

	assert.Equal(t, 5, total)
	assert.Len(t, results, 2)

	// Get page 2 (next 2)
	results2, total2, err := store.ListFiles(FileFilter{Limit: 2, Offset: 2})
	require.NoError(t, err, "ListFiles should succeed")

	assert.Equal(t, 5, total2)
	assert.Len(t, results2, 2)

	// Results should be different
	assert.NotEqual(t, results[0].ID, results2[0].ID, "pagination should return different items")
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
	require.NoError(t, store.AddFile(f), "AddFile should succeed")

	// Update the file
	f.Quality = "1080p bluray"
	f.Source = "usenet"
	f.SizeBytes = 8589934592

	require.NoError(t, store.UpdateFile(f), "UpdateFile should succeed")

	// Verify in database
	retrieved, err := store.GetFile(f.ID)
	require.NoError(t, err, "GetFile should succeed")

	assert.Equal(t, "1080p bluray", retrieved.Quality)
	assert.Equal(t, "usenet", retrieved.Source)
	assert.Equal(t, int64(8589934592), retrieved.SizeBytes)
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
	assert.ErrorIs(t, err, ErrNotFound, "UpdateFile should return ErrNotFound")
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
	require.NoError(t, store.AddFile(f), "AddFile should succeed")

	// Delete
	require.NoError(t, store.DeleteFile(f.ID), "DeleteFile should succeed")

	// Verify deleted
	_, err := store.GetFile(f.ID)
	assert.ErrorIs(t, err, ErrNotFound, "GetFile after delete should return ErrNotFound")
}

func TestStore_DeleteFile_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	assert.NoError(t, store.DeleteFile(9999), "DeleteFile(9999) should be idempotent")
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
	require.NoError(t, store.AddEpisode(e), "AddEpisode should succeed")

	// Add file for the episode
	f := &File{
		ContentID: series.ID,
		EpisodeID: &e.ID,
		Path:      "/tv/Breaking Bad/Season 1/Breaking.Bad.S01E01.Pilot.1080p.BluRay.mkv",
		SizeBytes: 4294967296,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	require.NoError(t, store.AddFile(f), "AddFile should succeed")
	assert.NotZero(t, f.ID, "ID should be set after AddFile")

	// Retrieve and verify EpisodeID
	retrieved, err := store.GetFile(f.ID)
	require.NoError(t, err, "GetFile should succeed")

	require.NotNil(t, retrieved.EpisodeID, "EpisodeID should not be nil")
	assert.Equal(t, e.ID, *retrieved.EpisodeID)
}

func TestStore_ListFiles_FilterByEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	// Create two episodes
	e1 := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	e2 := &Episode{ContentID: series.ID, Season: 1, Episode: 2, Title: "Cat's in the Bag", Status: StatusWanted}
	require.NoError(t, store.AddEpisode(e1), "AddEpisode should succeed")
	require.NoError(t, store.AddEpisode(e2), "AddEpisode should succeed")

	// Add files for each episode
	require.NoError(t, store.AddFile(&File{ContentID: series.ID, EpisodeID: &e1.ID, Path: "/tv/s01e01.mkv", Quality: "1080p bluray"}), "AddFile should succeed")
	require.NoError(t, store.AddFile(&File{ContentID: series.ID, EpisodeID: &e2.ID, Path: "/tv/s01e02.mkv", Quality: "1080p bluray"}), "AddFile should succeed")

	// Filter by EpisodeID
	results, total, err := store.ListFiles(FileFilter{EpisodeID: &e1.ID})
	require.NoError(t, err, "ListFiles should succeed")

	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].EpisodeID)
	assert.Equal(t, e1.ID, *results[0].EpisodeID)
}

func TestStore_ListFiles_FilterByQuality(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	// Add files with different qualities
	require.NoError(t, store.AddFile(&File{ContentID: movie.ID, Path: "/movies/file1.mkv", Quality: "1080p bluray"}), "AddFile should succeed")
	require.NoError(t, store.AddFile(&File{ContentID: movie.ID, Path: "/movies/file2.mkv", Quality: "720p webdl"}), "AddFile should succeed")
	require.NoError(t, store.AddFile(&File{ContentID: movie.ID, Path: "/movies/file3.mkv", Quality: "1080p bluray"}), "AddFile should succeed")

	// Filter by quality
	quality := "1080p bluray"
	results, total, err := store.ListFiles(FileFilter{Quality: &quality})
	require.NoError(t, err, "ListFiles should succeed")

	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	for _, f := range results {
		assert.Equal(t, "1080p bluray", f.Quality)
	}
}

func TestTx_AddFile(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")
	defer func() { _ = tx.Rollback() }()

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	require.NoError(t, tx.AddFile(f), "tx.AddFile should succeed")
	assert.NotZero(t, f.ID, "ID should be set")

	// Should be visible within transaction
	retrieved, err := tx.GetFile(f.ID)
	require.NoError(t, err, "tx.GetFile should succeed")
	assert.Equal(t, f.Path, retrieved.Path)

	// Commit
	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Should be visible after commit
	retrieved, err = store.GetFile(f.ID)
	require.NoError(t, err, "store.GetFile after commit should succeed")
	assert.Equal(t, f.Path, retrieved.Path)
}

func TestTx_Rollback_File(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	movie := createTestMovie(t, store)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	f := &File{
		ContentID: movie.ID,
		Path:      "/movies/Fight Club (1999)/Fight.Club.1999.1080p.BluRay.mkv",
		SizeBytes: 8589934592,
		Quality:   "1080p bluray",
		Source:    "usenet",
	}

	require.NoError(t, tx.AddFile(f), "tx.AddFile should succeed")

	id := f.ID

	// Rollback
	require.NoError(t, tx.Rollback(), "Rollback should succeed")

	// Should NOT be visible after rollback
	_, err = store.GetFile(id)
	assert.ErrorIs(t, err, ErrNotFound, "GetFile after rollback should return ErrNotFound")
}
