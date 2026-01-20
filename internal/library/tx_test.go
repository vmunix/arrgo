// internal/library/tx_test.go
package library

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTx_Commit(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	c := &Content{Type: ContentTypeMovie, Title: "TX Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	require.NoError(t, tx.AddContent(c), "AddContent in tx should succeed")

	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Should be visible outside transaction
	got, err := store.GetContent(c.ID)
	require.NoError(t, err, "GetContent after commit should succeed")
	assert.Equal(t, "TX Movie", got.Title)
}

func TestTx_Rollback_Comprehensive(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	c := &Content{Type: ContentTypeMovie, Title: "TX Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	require.NoError(t, tx.AddContent(c), "AddContent in tx should succeed")
	id := c.ID

	require.NoError(t, tx.Rollback(), "Rollback should succeed")

	// Should NOT be visible outside transaction
	_, err = store.GetContent(id)
	require.ErrorIs(t, err, ErrNotFound, "expected ErrNotFound after rollback")
}

func TestTx_MultipleOperations(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	// Add series with episodes in one transaction
	series := &Content{Type: ContentTypeSeries, Title: "TX Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	require.NoError(t, tx.AddContent(series), "AddContent should succeed")

	for i := 1; i <= 3; i++ {
		ep := &Episode{ContentID: series.ID, Season: 1, Episode: i, Title: "Episode", Status: StatusWanted}
		require.NoError(t, tx.AddEpisode(ep), "AddEpisode should succeed")
	}

	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Verify all episodes exist
	eps, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err, "ListEpisodes should succeed")
	assert.Equal(t, 3, total, "expected 3 episodes")
	assert.Len(t, eps, 3, "expected 3 results")
}

func TestTx_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	require.NoError(t, store.AddContent(series), "setup AddContent should succeed")
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	require.NoError(t, store.AddEpisode(ep), "setup AddEpisode should succeed")
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/series/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	require.NoError(t, store.AddFile(f), "setup AddFile should succeed")

	// Delete content - should cascade
	require.NoError(t, store.DeleteContent(series.ID), "DeleteContent should succeed")

	// Episode should be gone
	_, err := store.GetEpisode(ep.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected episode ErrNotFound after cascade")

	// File should be gone
	_, err = store.GetFile(f.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected file ErrNotFound after cascade")
}

func TestTx_CascadeDelete_InTransaction(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file outside transaction
	series := &Content{Type: ContentTypeSeries, Title: "Cascade TX Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	require.NoError(t, store.AddContent(series), "setup AddContent should succeed")
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	require.NoError(t, store.AddEpisode(ep), "setup AddEpisode should succeed")
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/cascade/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	require.NoError(t, store.AddFile(f), "setup AddFile should succeed")

	// Delete content in a transaction
	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	require.NoError(t, tx.DeleteContent(series.ID), "tx.DeleteContent should succeed")

	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Episode should be gone after commit
	_, err = store.GetEpisode(ep.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected episode ErrNotFound after cascade delete")

	// File should be gone after commit
	_, err = store.GetFile(f.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected file ErrNotFound after cascade delete")
}

func TestTx_CascadeDelete_Rollback(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Rollback Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	require.NoError(t, store.AddContent(series), "setup AddContent should succeed")
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	require.NoError(t, store.AddEpisode(ep), "setup AddEpisode should succeed")
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/rollback/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	require.NoError(t, store.AddFile(f), "setup AddFile should succeed")

	// Delete content in a transaction, then rollback
	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	require.NoError(t, tx.DeleteContent(series.ID), "tx.DeleteContent should succeed")

	// Rollback the delete
	require.NoError(t, tx.Rollback(), "Rollback should succeed")

	// Content should still exist
	_, err = store.GetContent(series.ID)
	require.NoError(t, err, "expected content to exist after rollback")

	// Episode should still exist
	_, err = store.GetEpisode(ep.ID)
	require.NoError(t, err, "expected episode to exist after rollback")

	// File should still exist
	_, err = store.GetFile(f.ID)
	assert.NoError(t, err, "expected file to exist after rollback")
}

func TestTx_EpisodeDelete_CascadeFiles(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Episode Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	require.NoError(t, store.AddContent(series), "setup AddContent should succeed")
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	require.NoError(t, store.AddEpisode(ep), "setup AddEpisode should succeed")
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/episode-cascade/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	require.NoError(t, store.AddFile(f), "setup AddFile should succeed")

	// Delete episode - file should cascade
	require.NoError(t, store.DeleteEpisode(ep.ID), "DeleteEpisode should succeed")

	// Episode should be gone
	_, err := store.GetEpisode(ep.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected episode ErrNotFound after delete")

	// File should be gone (cascade from episode)
	_, err = store.GetFile(f.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected file ErrNotFound after episode cascade delete")

	// Series should still exist
	_, err = store.GetContent(series.ID)
	assert.NoError(t, err, "expected series to still exist")
}

func TestTx_MultipleEpisodesAndFiles(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with multiple episodes and files in a transaction
	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	series := &Content{Type: ContentTypeSeries, Title: "Multi Episode Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	require.NoError(t, tx.AddContent(series), "tx.AddContent should succeed")

	for i := 1; i <= 3; i++ {
		ep := &Episode{ContentID: series.ID, Season: 1, Episode: i, Title: "Episode", Status: StatusWanted}
		require.NoError(t, tx.AddEpisode(ep), "tx.AddEpisode should succeed")

		// Add a file for each episode
		f := &File{
			ContentID: series.ID,
			EpisodeID: &ep.ID,
			Path:      "/tv/multi/s01e0" + string(rune('0'+i)) + ".mkv",
			SizeBytes: 1000,
			Quality:   "1080p",
			Source:    "webdl",
		}
		require.NoError(t, tx.AddFile(f), "tx.AddFile should succeed")
	}

	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Verify all episodes exist
	_, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err, "ListEpisodes should succeed")
	assert.Equal(t, 3, total, "expected 3 episodes")

	// Verify all files exist
	files, total, err := store.ListFiles(FileFilter{ContentID: &series.ID})
	require.NoError(t, err, "ListFiles should succeed")
	assert.Equal(t, 3, total, "expected 3 files")
	assert.Len(t, files, 3, "expected 3 file results")

	// Delete series - all should cascade
	require.NoError(t, store.DeleteContent(series.ID), "DeleteContent should succeed")

	// Verify all episodes are gone
	_, total, err = store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err, "ListEpisodes after delete should succeed")
	assert.Equal(t, 0, total, "expected 0 episodes after cascade delete")

	// Verify all files are gone
	_, total, err = store.ListFiles(FileFilter{ContentID: &series.ID})
	require.NoError(t, err, "ListFiles after delete should succeed")
	assert.Equal(t, 0, total, "expected 0 files after cascade delete")
}

func TestTx_MovieWithFile_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create movie with file
	movie := &Content{Type: ContentTypeMovie, Title: "Cascade Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	require.NoError(t, store.AddContent(movie), "setup AddContent should succeed")
	f := &File{ContentID: movie.ID, Path: "/movies/cascade-movie.mkv", SizeBytes: 5000, Quality: "1080p", Source: "usenet"}
	require.NoError(t, store.AddFile(f), "setup AddFile should succeed")

	// Delete movie - file should cascade
	require.NoError(t, store.DeleteContent(movie.ID), "DeleteContent should succeed")

	// File should be gone
	_, err := store.GetFile(f.ID)
	require.ErrorIs(t, err, ErrNotFound, "expected file ErrNotFound after cascade delete")
}
