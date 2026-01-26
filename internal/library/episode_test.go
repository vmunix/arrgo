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

func TestStore_FindOrCreateEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create test series
	content := &Content{
		Type:           ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(content))

	// First call should create
	ep1, created, err := store.FindOrCreateEpisode(content.ID, 1, 5)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, content.ID, ep1.ContentID)
	assert.Equal(t, 1, ep1.Season)
	assert.Equal(t, 5, ep1.Episode)
	assert.Equal(t, StatusWanted, ep1.Status)

	// Second call should find existing
	ep2, created, err := store.FindOrCreateEpisode(content.ID, 1, 5)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, ep1.ID, ep2.ID)
}

func TestStore_FindOrCreateEpisodes(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	content := &Content{
		Type:           ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(content))

	// Create multiple episodes at once
	episodes, err := store.FindOrCreateEpisodes(content.ID, 1, []int{1, 2, 3})
	require.NoError(t, err)
	assert.Len(t, episodes, 3)

	// Verify each episode
	for i, ep := range episodes {
		assert.Equal(t, content.ID, ep.ContentID)
		assert.Equal(t, 1, ep.Season)
		assert.Equal(t, i+1, ep.Episode)
	}

	// Call again - should return same episodes
	episodes2, err := store.FindOrCreateEpisodes(content.ID, 1, []int{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, episodes[0].ID, episodes2[0].ID)
}

func TestStore_GetSeriesStats(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create test series
	series := &Content{
		Type:           ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	// No episodes - stats should be zero
	stats, err := store.GetSeriesStats(series.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.TotalEpisodes)
	assert.Equal(t, 0, stats.AvailableEpisodes)
	assert.Equal(t, 0, stats.SeasonCount)

	// Add episodes in season 1: 3 total, 2 available
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 1, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 2, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 3, Status: StatusWanted}))

	stats, err = store.GetSeriesStats(series.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.TotalEpisodes)
	assert.Equal(t, 2, stats.AvailableEpisodes)
	assert.Equal(t, 1, stats.SeasonCount)

	// Add episodes in season 2: 2 total, 1 available
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 2, Episode: 1, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 2, Episode: 2, Status: StatusWanted}))

	stats, err = store.GetSeriesStats(series.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, stats.TotalEpisodes)
	assert.Equal(t, 3, stats.AvailableEpisodes)
	assert.Equal(t, 2, stats.SeasonCount)
}

func TestStore_GetSeriesStats_AllAvailable(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	series := &Content{
		Type:           ContentTypeSeries,
		Title:          "Complete Series",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	// Add all available episodes
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 1, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 2, Status: StatusAvailable}))

	stats, err := store.GetSeriesStats(series.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalEpisodes)
	assert.Equal(t, 2, stats.AvailableEpisodes)
	assert.Equal(t, 1, stats.SeasonCount)
}

func TestStore_GetSeriesStatsBatch(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create two series
	series1 := &Content{
		Type:           ContentTypeSeries,
		Title:          "Series One",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series1))

	series2 := &Content{
		Type:           ContentTypeSeries,
		Title:          "Series Two",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series2))

	// Series 1: 3 episodes, 2 available, 1 season
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 1, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 2, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 3, Status: StatusWanted}))

	// Series 2: 4 episodes, 4 available, 2 seasons
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series2.ID, Season: 1, Episode: 1, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series2.ID, Season: 1, Episode: 2, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series2.ID, Season: 2, Episode: 1, Status: StatusAvailable}))
	require.NoError(t, store.AddEpisode(&Episode{ContentID: series2.ID, Season: 2, Episode: 2, Status: StatusAvailable}))

	// Batch query
	statsMap, err := store.GetSeriesStatsBatch([]int64{series1.ID, series2.ID})
	require.NoError(t, err)
	assert.Len(t, statsMap, 2)

	// Verify series 1
	stats1 := statsMap[series1.ID]
	require.NotNil(t, stats1, "stats for series1 should exist")
	assert.Equal(t, 3, stats1.TotalEpisodes)
	assert.Equal(t, 2, stats1.AvailableEpisodes)
	assert.Equal(t, 1, stats1.SeasonCount)

	// Verify series 2
	stats2 := statsMap[series2.ID]
	require.NotNil(t, stats2, "stats for series2 should exist")
	assert.Equal(t, 4, stats2.TotalEpisodes)
	assert.Equal(t, 4, stats2.AvailableEpisodes)
	assert.Equal(t, 2, stats2.SeasonCount)
}

func TestStore_GetSeriesStatsBatch_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Empty input should return empty map
	statsMap, err := store.GetSeriesStatsBatch([]int64{})
	require.NoError(t, err)
	assert.Empty(t, statsMap)
}

func TestStore_GetSeriesStatsBatch_NoEpisodes(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with no episodes
	series := &Content{
		Type:           ContentTypeSeries,
		Title:          "Empty Series",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	// Should return empty map since no episodes exist
	statsMap, err := store.GetSeriesStatsBatch([]int64{series.ID})
	require.NoError(t, err)
	assert.Empty(t, statsMap, "series with no episodes should not appear in batch results")
}

func TestStore_BulkAddEpisodes(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create test series
	series := &Content{
		Type:           ContentTypeSeries,
		Title:          "Bulk Test Show",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	// Prepare 3 episodes for bulk insert
	airDate1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	airDate2 := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	airDate3 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	episodes := []*Episode{
		{ContentID: series.ID, Season: 1, Episode: 1, Title: "Episode One", Status: StatusWanted, AirDate: &airDate1},
		{ContentID: series.ID, Season: 1, Episode: 2, Title: "Episode Two", Status: StatusWanted, AirDate: &airDate2},
		{ContentID: series.ID, Season: 1, Episode: 3, Title: "Episode Three", Status: StatusWanted, AirDate: &airDate3},
	}

	// Bulk add the episodes
	inserted, err := store.BulkAddEpisodes(episodes)
	require.NoError(t, err, "BulkAddEpisodes should succeed")
	assert.Equal(t, 3, inserted, "all 3 episodes should be inserted")

	// Verify episodes are in database
	results, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err)
	assert.Equal(t, 3, total, "should have 3 episodes in database")
	assert.Len(t, results, 3)

	// Verify episode details
	assert.Equal(t, "Episode One", results[0].Title)
	assert.Equal(t, "Episode Two", results[1].Title)
	assert.Equal(t, "Episode Three", results[2].Title)

	// Try to bulk add the same 3 episodes again (duplicates)
	duplicateEpisodes := []*Episode{
		{ContentID: series.ID, Season: 1, Episode: 1, Title: "Episode One Modified", Status: StatusAvailable},
		{ContentID: series.ID, Season: 1, Episode: 2, Title: "Episode Two Modified", Status: StatusAvailable},
		{ContentID: series.ID, Season: 1, Episode: 3, Title: "Episode Three Modified", Status: StatusAvailable},
	}

	inserted, err = store.BulkAddEpisodes(duplicateEpisodes)
	require.NoError(t, err, "BulkAddEpisodes with duplicates should not error")
	assert.Equal(t, 0, inserted, "no episodes should be inserted (all duplicates)")

	// Verify original data is unchanged (INSERT OR IGNORE doesn't update)
	results, _, err = store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err)
	assert.Equal(t, "Episode One", results[0].Title, "original title should be preserved")
	assert.Equal(t, StatusWanted, results[0].Status, "original status should be preserved")
}

func TestStore_BulkAddEpisodes_Empty(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Empty slice should return 0, nil
	inserted, err := store.BulkAddEpisodes([]*Episode{})
	require.NoError(t, err)
	assert.Equal(t, 0, inserted)
}

func TestStore_BulkAddEpisodes_PartialDuplicates(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create test series
	series := &Content{
		Type:           ContentTypeSeries,
		Title:          "Partial Duplicate Test",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	// Add one episode first
	ep1 := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Existing", Status: StatusWanted}
	require.NoError(t, store.AddEpisode(ep1))

	// Bulk add with mix of existing and new
	episodes := []*Episode{
		{ContentID: series.ID, Season: 1, Episode: 1, Title: "Duplicate", Status: StatusWanted},   // exists
		{ContentID: series.ID, Season: 1, Episode: 2, Title: "New Episode 2", Status: StatusWanted}, // new
		{ContentID: series.ID, Season: 1, Episode: 3, Title: "New Episode 3", Status: StatusWanted}, // new
	}

	inserted, err := store.BulkAddEpisodes(episodes)
	require.NoError(t, err)
	assert.Equal(t, 2, inserted, "only 2 new episodes should be inserted")

	// Verify total count
	results, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, results, 3)

	// Verify original was not modified
	assert.Equal(t, "Existing", results[0].Title, "original episode title should be preserved")
}
