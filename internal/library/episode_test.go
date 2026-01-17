package library

import (
	"errors"
	"testing"
	"time"
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
	if err := store.AddContent(c); err != nil {
		t.Fatalf("create test series: %v", err)
	}
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

	if err := store.AddEpisode(e); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// ID should be set
	if e.ID == 0 {
		t.Error("ID should be set after AddEpisode")
	}
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

	if err := store.AddEpisode(e1); err != nil {
		t.Fatalf("AddEpisode first: %v", err)
	}

	// Try to add duplicate (same content_id, season, episode)
	e2 := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot Duplicate",
		Status:    StatusWanted,
	}

	err := store.AddEpisode(e2)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("AddEpisode duplicate error = %v, want ErrDuplicate", err)
	}
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
	if err := store.AddEpisode(original); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	retrieved, err := store.GetEpisode(original.ID)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}

	// Verify all fields
	if retrieved.ID != original.ID {
		t.Errorf("ID = %d, want %d", retrieved.ID, original.ID)
	}
	if retrieved.ContentID != original.ContentID {
		t.Errorf("ContentID = %d, want %d", retrieved.ContentID, original.ContentID)
	}
	if retrieved.Season != original.Season {
		t.Errorf("Season = %d, want %d", retrieved.Season, original.Season)
	}
	if retrieved.Episode != original.Episode {
		t.Errorf("Episode = %d, want %d", retrieved.Episode, original.Episode)
	}
	if retrieved.Title != original.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, original.Title)
	}
	if retrieved.Status != original.Status {
		t.Errorf("Status = %q, want %q", retrieved.Status, original.Status)
	}
	if retrieved.AirDate == nil {
		t.Error("AirDate should not be nil")
	} else if !retrieved.AirDate.Equal(*original.AirDate) {
		t.Errorf("AirDate = %v, want %v", retrieved.AirDate, original.AirDate)
	}
}

func TestStore_GetEpisode_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetEpisode(9999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEpisode(9999) error = %v, want ErrNotFound", err)
	}
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
	if err := store.AddContent(series2); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	// Add episodes to both
	if err := store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 1, Title: "BB S01E01", Status: StatusWanted}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := store.AddEpisode(&Episode{ContentID: series1.ID, Season: 1, Episode: 2, Title: "BB S01E02", Status: StatusWanted}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := store.AddEpisode(&Episode{ContentID: series2.ID, Season: 1, Episode: 1, Title: "Wire S01E01", Status: StatusWanted}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// Filter by ContentID
	results, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series1.ID})
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	for _, ep := range results {
		if ep.ContentID != series1.ID {
			t.Errorf("episode ContentID = %d, want %d", ep.ContentID, series1.ID)
		}
	}
}

func TestStore_ListEpisodes_FilterBySeason(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	// Add episodes in different seasons
	if err := store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "S01E01", Status: StatusWanted}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := store.AddEpisode(&Episode{ContentID: series.ID, Season: 1, Episode: 2, Title: "S01E02", Status: StatusWanted}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := store.AddEpisode(&Episode{ContentID: series.ID, Season: 2, Episode: 1, Title: "S02E01", Status: StatusWanted}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// Filter by season 1
	season := 1
	results, total, err := store.ListEpisodes(EpisodeFilter{Season: &season})
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	for _, ep := range results {
		if ep.Season != 1 {
			t.Errorf("episode Season = %d, want 1", ep.Season)
		}
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
		if err := store.AddEpisode(e); err != nil {
			t.Fatalf("AddEpisode: %v", err)
		}
	}

	// Get page 1 (first 2)
	results, total, err := store.ListEpisodes(EpisodeFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}

	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Get page 2 (next 2)
	results2, total2, err := store.ListEpisodes(EpisodeFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
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
	if err := store.AddEpisode(e); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// Update the episode
	e.Title = "Pilot (Renamed)"
	e.Status = StatusAvailable

	if err := store.UpdateEpisode(e); err != nil {
		t.Fatalf("UpdateEpisode: %v", err)
	}

	// Verify in database
	retrieved, err := store.GetEpisode(e.ID)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}

	if retrieved.Title != "Pilot (Renamed)" {
		t.Errorf("Title = %q, want Pilot (Renamed)", retrieved.Title)
	}
	if retrieved.Status != StatusAvailable {
		t.Errorf("Status = %q, want available", retrieved.Status)
	}
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
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateEpisode error = %v, want ErrNotFound", err)
	}
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
	if err := store.AddEpisode(e); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	// Delete
	if err := store.DeleteEpisode(e.ID); err != nil {
		t.Fatalf("DeleteEpisode: %v", err)
	}

	// Verify deleted
	_, err := store.GetEpisode(e.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEpisode after delete: error = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteEpisode_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	if err := store.DeleteEpisode(9999); err != nil {
		t.Errorf("DeleteEpisode(9999) = %v, want nil (idempotent)", err)
	}
}

func TestTx_AddEpisode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}

	if err := tx.AddEpisode(e); err != nil {
		t.Fatalf("tx.AddEpisode: %v", err)
	}

	if e.ID == 0 {
		t.Error("ID should be set")
	}

	// Should be visible within transaction
	retrieved, err := tx.GetEpisode(e.ID)
	if err != nil {
		t.Fatalf("tx.GetEpisode: %v", err)
	}
	if retrieved.Title != e.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, e.Title)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Should be visible after commit
	retrieved, err = store.GetEpisode(e.ID)
	if err != nil {
		t.Fatalf("store.GetEpisode after commit: %v", err)
	}
	if retrieved.Title != e.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, e.Title)
	}
}

func TestTx_Rollback_Episode(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	series := createTestSeries(t, store)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	e := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}

	if err := tx.AddEpisode(e); err != nil {
		t.Fatalf("tx.AddEpisode: %v", err)
	}

	id := e.ID

	// Rollback
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Should NOT be visible after rollback
	_, err = store.GetEpisode(id)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEpisode after rollback: error = %v, want ErrNotFound", err)
	}
}
