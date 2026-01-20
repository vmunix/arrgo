package library

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestSeries creates a series Content for episode tests
func createTestSeries(t *testing.T, store *Store) *Content {
	t.Helper()
	c := &Content{
		Type:           ContentTypeSeries,
		TVDBID:         ptr(int64(81189)),
		Title:          "Breaking Bad",
		Year:           2008,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(c), "create test series should succeed")
	return c
}

func TestStore_AddEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	airDate := time.Date(2008, 1, 20, 0, 0, 0, 0, time.UTC)
	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
		AirDate:   &airDate,
	}

	require.NoError(t, store.AddEpisode(e), "AddEpisode should succeed")

	// ID should be set
	assert.NotZero(t, e.ID, "ID should be set after AddEpisode")
}

func TestStore_AddEpisode_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	e1 := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}

	require.NoError(t, store.AddEpisode(e1), "AddEpisode first should succeed")

	// Try to add duplicate (same content_id, season, episode)
	e2 := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot Duplicate",
		Status:    StatusWanted,
	}

	err := store.AddEpisode(e2)
	assert.ErrorIs(t, err, ErrDuplicate, "AddEpisode duplicate should return ErrDuplicate")
}

func TestStore_GetEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	airDate := time.Date(2008, 1, 20, 0, 0, 0, 0, time.UTC)
	original := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
		AirDate:   &airDate,
	}
	require.NoError(t, store.AddEpisode(original), "AddEpisode should succeed")

	retrieved, err := store.GetEpisode(original.ID)
	require.NoError(t, err, "GetEpisode should succeed")

	// Verify all fields
	assert.Equal(t, original.ID, retrieved.ID)
	assert.Equal(t, original.ContentID, retrieved.ContentID)
	assert.Equal(t, original.Season, retrieved.Season)
	assert.Equal(t, original.Episode, retrieved.Episode)
	assert.Equal(t, original.Title, retrieved.Title)
	assert.Equal(t, original.Status, retrieved.Status)
	require.NotNil(t, retrieved.AirDate, "AirDate should not be nil")
	assert.True(t, retrieved.AirDate.Equal(*original.AirDate), "AirDate = %v, want %v", retrieved.AirDate, original.AirDate)
}

func TestStore_GetEpisode_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetEpisode(9999)
	assert.ErrorIs(t, err, ErrNotFound, "GetEpisode(9999) should return ErrNotFound")
}

func TestStore_ListEpisodes_FilterByContentID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create two series
	series1 := createTestSeries(t, store)
	series2 := &Content{
		Type:           ContentTypeSeries,
		TVDBID:         ptr(int64(12345)),
		Title:          "The Wire",
		Year:           2002,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series2), "AddContent should succeed")

	// Add episodes to both
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 1, Title: "BB S01E01", Status: StatusWanted}), "AddEpisode should succeed")
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 2, Title: "BB S01E02", Status: StatusWanted}), "AddEpisode should succeed")
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series2.ID, Season: 1, Episode: 1, Title: "Wire S01E01", Status: StatusWanted}), "AddEpisode should succeed")

	// Filter by ContentID
	results, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series1.ID})
	require.NoError(t, err, "ListEpisodes should succeed")

	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	for _, ep := range results {
		assert.Equal(t, series1.ID, ep.ContentID, "episode ContentID should match")
	}
}

func TestStore_ListEpisodes_FilterBySeason(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	// Add episodes in different seasons
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "S01E01", Status: StatusWanted}), "AddEpisode should succeed")
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 2, Title: "S01E02", Status: StatusWanted}), "AddEpisode should succeed")
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 2, Episode: 1, Title: "S02E01", Status: StatusWanted}), "AddEpisode should succeed")

	// Filter by season 1
	season := 1
	results, total, err := store.ListEpisodes(EpisodeFilter{Season: &season})
	require.NoError(t, err, "ListEpisodes should succeed")

	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	for _, ep := range results {
		assert.Equal(t, 1, ep.Season, "episode Season should be 1")
	}
}

func TestStore_ListEpisodes_Pagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	// Add 5 episodes
	for i := 1; i <= 5; i++ {
		e := &Episode{
			ContentID: series.ID,
			Season:    1,
			Episode:   i,
			Title:     "Episode",
			Status:    StatusWanted,
		}
		require.NoError(t, store.AddEpisode(e), "AddEpisode should succeed")
	}

	// Get page 1 (first 2)
	results, total, err := store.ListEpisodes(EpisodeFilter{Limit: 2, Offset: 0})
	require.NoError(t, err, "ListEpisodes should succeed")

	assert.Equal(t, 5, total)
	assert.Len(t, results, 2)

	// Get page 2 (next 2)
	results2, total2, err := store.ListEpisodes(EpisodeFilter{Limit: 2, Offset: 2})
	require.NoError(t, err, "ListEpisodes should succeed")

	assert.Equal(t, 5, total2)
	assert.Len(t, results2, 2)

	// Results should be different
	assert.NotEqual(t, results[0].ID, results2[0].ID, "pagination should return different items")
}

func TestStore_UpdateEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}
	require.NoError(t, store.AddEpisode(e), "AddEpisode should succeed")

	// Update the episode
	e.Title = "Pilot (Renamed)"
	e.Status = StatusAvailable

	require.NoError(t, store.UpdateEpisode(e), "UpdateEpisode should succeed")

	// Verify in database
	retrieved, err := store.GetEpisode(e.ID)
	require.NoError(t, err, "GetEpisode should succeed")

	assert.Equal(t, "Pilot (Renamed)", retrieved.Title)
	assert.Equal(t, StatusAvailable, retrieved.Status)
}

func TestStore_UpdateEpisode_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	e := &Episode{
		ID:        9999,
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Nonexistent",
		Status:    StatusWanted,
	}

	err := store.UpdateEpisode(e)
	assert.ErrorIs(t, err, ErrNotFound, "UpdateEpisode should return ErrNotFound")
}

func TestStore_DeleteEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}
	require.NoError(t, store.AddEpisode(e), "AddEpisode should succeed")

	// Delete
	require.NoError(t, store.DeleteEpisode(e.ID), "DeleteEpisode should succeed")

	// Verify deleted
	_, err := store.GetEpisode(e.ID)
	assert.ErrorIs(t, err, ErrNotFound, "GetEpisode after delete should return ErrNotFound")
}

func TestStore_DeleteEpisode_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	assert.NoError(t, store.DeleteEpisode(9999), "DeleteEpisode(9999) should be idempotent")
}

func TestTx_AddEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")
	defer func() { _ = tx.Rollback() }()

	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}

	require.NoError(t, tx.AddEpisode(e), "tx.AddEpisode should succeed")
	assert.NotZero(t, e.ID, "ID should be set")

	// Should be visible within transaction
	retrieved, err := tx.GetEpisode(e.ID)
	require.NoError(t, err, "tx.GetEpisode should succeed")
	assert.Equal(t, e.Title, retrieved.Title)

	// Commit
	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Should be visible after commit
	retrieved, err = store.GetEpisode(e.ID)
	require.NoError(t, err, "store.GetEpisode after commit should succeed")
	assert.Equal(t, e.Title, retrieved.Title)
}

func TestTx_Rollback_Episode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}

	require.NoError(t, tx.AddEpisode(e), "tx.AddEpisode should succeed")

	id := e.ID

	// Rollback
	require.NoError(t, tx.Rollback(), "Rollback should succeed")

	// Should NOT be visible after rollback
	_, err = store.GetEpisode(id)
	assert.ErrorIs(t, err, ErrNotFound, "GetEpisode after rollback should return ErrNotFound")
}
