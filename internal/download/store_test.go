package download

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Add(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}

	before := time.Now()
	err := store.Add(d)
	after := time.Now()

	require.NoError(t, err)
	assert.NotZero(t, d.ID, "ID should be set after Add")
	assert.False(t, d.AddedAt.Before(before) || d.AddedAt.After(after),
		"AddedAt %v not in expected range [%v, %v]", d.AddedAt, before, after)
}

func TestStore_Add_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	d1 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}

	require.NoError(t, store.Add(d1))
	firstID := d1.ID

	// Add same content_id + release_name again
	d2 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_different",
		Status:      StatusDownloading,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "dognzb",
	}

	require.NoError(t, store.Add(d2))
	// Should return the existing record's ID
	assert.Equal(t, firstID, d2.ID, "idempotent Add should return same ID")
}

func TestStore_Add_DifferentReleaseName(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	d1 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}

	require.NoError(t, store.Add(d1))

	// Add same content_id but different release_name - should create new record
	d2 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_xyz789",
		Status:      StatusQueued,
		ReleaseName: "Fight.Club.1999.720p.BluRay.x264",
		Indexer:     "nzbgeek",
	}

	require.NoError(t, store.Add(d2))
	// Should create a new record with different ID
	assert.NotEqual(t, d1.ID, d2.ID, "different release_name should create new record")
}

func TestStore_Get(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	original := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(original))

	retrieved, err := store.Get(original.ID)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, original.ID, retrieved.ID)
	assert.Equal(t, original.ContentID, retrieved.ContentID)
	assert.Equal(t, original.Client, retrieved.Client)
	assert.Equal(t, original.ClientID, retrieved.ClientID)
	assert.Equal(t, original.Status, retrieved.Status)
	assert.Equal(t, original.ReleaseName, retrieved.ReleaseName)
	assert.Equal(t, original.Indexer, retrieved.Indexer)
}

func TestStore_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.Get(9999)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStore_GetByClientID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	original := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(original))

	retrieved, err := store.GetByClientID(ClientSABnzbd, "SABnzbd_nzo_abc123")
	require.NoError(t, err)
	assert.Equal(t, original.ID, retrieved.ID)
}

func TestStore_GetByClientID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetByClientID(ClientSABnzbd, "nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStore_Update(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(d))

	// Update status and completed_at
	d.Status = StatusCompleted
	now := time.Now()
	d.CompletedAt = &now

	require.NoError(t, store.Update(d))

	// Verify in database
	retrieved, err := store.Get(d.ID)
	require.NoError(t, err)

	assert.Equal(t, StatusCompleted, retrieved.Status)
	assert.NotNil(t, retrieved.CompletedAt, "CompletedAt should not be nil")
}

func TestStore_Update_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	d := &Download{
		ID:          9999,
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nonexistent",
		Status:      StatusCompleted,
		ReleaseName: "test",
	}

	err := store.Update(d)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStore_List_All(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add multiple downloads
	downloads := []*Download{
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusDownloading, ReleaseName: "release2", Indexer: "idx2"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_3", Status: StatusImported, ReleaseName: "release3", Indexer: "idx3"},
	}

	for _, d := range downloads {
		require.NoError(t, store.Add(d))
	}

	// List all
	results, total, err := store.List(Filter{})
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, 3, total)
}

func TestStore_List_Active(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add downloads with various statuses
	downloads := []*Download{
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusDownloading, ReleaseName: "release2", Indexer: "idx2"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_3", Status: StatusCompleted, ReleaseName: "release3", Indexer: "idx3"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_4", Status: StatusImported, ReleaseName: "release4", Indexer: "idx4"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_5", Status: StatusFailed, ReleaseName: "release5", Indexer: "idx5"},
		{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_6", Status: StatusCleaned, ReleaseName: "release6", Indexer: "idx6"},
	}

	for _, d := range downloads {
		require.NoError(t, store.Add(d))
	}

	// List active (excludes terminal states: cleaned, failed)
	results, total, err := store.List(Filter{Active: true})
	require.NoError(t, err)
	assert.Len(t, results, 4, "should exclude cleaned and failed")
	assert.Equal(t, 4, total)

	// Verify no terminal status in results
	for _, d := range results {
		assert.NotEqual(t, StatusCleaned, d.Status, "Active filter should exclude cleaned")
		assert.NotEqual(t, StatusFailed, d.Status, "Active filter should exclude failed")
	}
}

func TestStore_List_FilterByContentID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID1 := insertTestContent(t, db, "Fight Club")
	contentID2 := insertTestContent(t, db, "Pulp Fiction")

	// Add downloads for different content
	d1 := &Download{ContentID: contentID1, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"}
	d2 := &Download{ContentID: contentID2, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusQueued, ReleaseName: "release2", Indexer: "idx2"}

	require.NoError(t, store.Add(d1))
	require.NoError(t, store.Add(d2))

	// Filter by content ID
	results, total, err := store.List(Filter{ContentID: &contentID1})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, contentID1, results[0].ContentID)
}

func TestStore_List_FilterByStatus(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add downloads with different statuses
	d1 := &Download{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"}
	d2 := &Download{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusDownloading, ReleaseName: "release2", Indexer: "idx2"}

	require.NoError(t, store.Add(d1))
	require.NoError(t, store.Add(d2))

	// Filter by status
	status := StatusDownloading
	results, total, err := store.List(Filter{Status: &status})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, StatusDownloading, results[0].Status)
}

func TestStore_List_FilterByClient(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add downloads for different clients
	d1 := &Download{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"}
	d2 := &Download{ContentID: contentID, Client: ClientQBittorrent, ClientID: "hash123", Status: StatusQueued, ReleaseName: "release2", Indexer: "idx2"}

	require.NoError(t, store.Add(d1))
	require.NoError(t, store.Add(d2))

	// Filter by client
	client := ClientSABnzbd
	results, total, err := store.List(Filter{Client: &client})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, 1, total)
	assert.Equal(t, ClientSABnzbd, results[0].Client)
}

func TestStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Fight.Club.1999.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}
	require.NoError(t, store.Add(d))

	// Delete
	require.NoError(t, store.Delete(d.ID))

	// Verify deleted
	_, err := store.Get(d.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStore_Delete_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	err := store.Delete(9999)
	assert.NoError(t, err, "Delete should be idempotent")
}

func TestStore_Add_WithEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series content
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', 'Breaking Bad', 2008, 'wanted', 'hd', '/tv')`)
	require.NoError(t, err)
	contentID, _ := result.LastInsertId()

	// Create episode
	result, err = db.Exec(`
		INSERT INTO episodes (content_id, season, episode, title, status)
		VALUES (?, 1, 1, 'Pilot', 'wanted')`, contentID)
	require.NoError(t, err)
	episodeID, _ := result.LastInsertId()

	d := &Download{
		ContentID:   contentID,
		EpisodeID:   &episodeID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Breaking.Bad.S01E01.1080p.BluRay.x264",
		Indexer:     "nzbgeek",
	}

	require.NoError(t, store.Add(d))

	retrieved, err := store.Get(d.ID)
	require.NoError(t, err)

	require.NotNil(t, retrieved.EpisodeID, "EpisodeID should not be nil")
	assert.Equal(t, episodeID, *retrieved.EpisodeID)
}

func TestStore_List_FilterByEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series content
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', 'Breaking Bad', 2008, 'wanted', 'hd', '/tv')`)
	require.NoError(t, err)
	contentID, _ := result.LastInsertId()

	// Create episodes
	result, _ = db.Exec(`INSERT INTO episodes (content_id, season, episode, title, status) VALUES (?, 1, 1, 'Pilot', 'wanted')`, contentID)
	ep1ID, _ := result.LastInsertId()
	result, _ = db.Exec(`INSERT INTO episodes (content_id, season, episode, title, status) VALUES (?, 1, 2, 'Cat in the Bag', 'wanted')`, contentID)
	ep2ID, _ := result.LastInsertId()

	// Add downloads for different episodes
	d1 := &Download{ContentID: contentID, EpisodeID: &ep1ID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "S01E01", Indexer: "idx"}
	d2 := &Download{ContentID: contentID, EpisodeID: &ep2ID, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusQueued, ReleaseName: "S01E02", Indexer: "idx"}

	require.NoError(t, store.Add(d1))
	require.NoError(t, store.Add(d2))

	// Filter by episode ID
	results, total, err := store.List(Filter{EpisodeID: &ep1ID})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, 1, total)
	require.NotNil(t, results[0].EpisodeID)
	assert.Equal(t, ep1ID, *results[0].EpisodeID)
}

func TestStore_LastTransitionAt(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add a download
	d := &Download{
		ContentID:   contentID,
		Client:      ClientManual,
		ClientID:    "test-123",
		Status:      StatusQueued,
		ReleaseName: "Test.Release",
		Indexer:     "manual",
	}

	before := time.Now()
	require.NoError(t, store.Add(d))
	after := time.Now()

	// LastTransitionAt should be set on Add
	assert.False(t, d.LastTransitionAt.IsZero(), "LastTransitionAt should be set after Add")

	// Verify it's within expected time range
	assert.False(t, d.LastTransitionAt.Before(before) || d.LastTransitionAt.After(after),
		"LastTransitionAt %v not in expected range [%v, %v]", d.LastTransitionAt, before, after)

	// Retrieve and verify via Get
	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.False(t, got.LastTransitionAt.IsZero(), "LastTransitionAt should be set after Get")

	// Retrieve and verify via GetByClientID
	gotByClient, err := store.GetByClientID(ClientManual, "test-123")
	require.NoError(t, err)
	assert.False(t, gotByClient.LastTransitionAt.IsZero(), "LastTransitionAt should be set after GetByClientID")

	// Retrieve and verify via List
	downloads, _, err := store.List(Filter{})
	require.NoError(t, err)
	require.Len(t, downloads, 1)
	assert.False(t, downloads[0].LastTransitionAt.IsZero(), "LastTransitionAt should be set in List results")
}

func TestStore_Transition(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Track events
	var events []TransitionEvent
	store.OnTransition(func(e TransitionEvent) {
		events = append(events, e)
	})

	// Add a download
	d := &Download{
		ContentID:   contentID,
		Client:      ClientManual,
		ClientID:    "test-456",
		Status:      StatusQueued,
		ReleaseName: "Test.Release",
		Indexer:     "manual",
	}
	require.NoError(t, store.Add(d))

	// Valid transition
	oldTime := d.LastTransitionAt
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	require.NoError(t, store.Transition(d, StatusDownloading))

	assert.Equal(t, StatusDownloading, d.Status)
	assert.True(t, d.LastTransitionAt.After(oldTime), "LastTransitionAt should be updated")
	require.Len(t, events, 1)
	assert.Equal(t, StatusQueued, events[0].From)
	assert.Equal(t, StatusDownloading, events[0].To)

	// Invalid transition
	err := store.Transition(d, StatusCleaned)
	assert.Error(t, err, "should reject invalid transition downloading->cleaned")
}

func TestStore_ListStuck(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create content records for foreign key constraints
	contentID1 := insertTestContent(t, db, "Stuck Movie")
	contentID2 := insertTestContent(t, db, "Recent Movie")
	contentID3 := insertTestContent(t, db, "Downloading Movie")

	// Add downloads with old timestamps
	oldTime := time.Now().Add(-2 * time.Hour)

	// Stuck in queued (> 1 hour)
	d1 := &Download{
		ContentID:   contentID1,
		Client:      ClientManual,
		ClientID:    "stuck-queued",
		Status:      StatusQueued,
		ReleaseName: "Stuck.Queued",
		Indexer:     "manual",
	}
	require.NoError(t, store.Add(d1))
	// Manually set old timestamp
	_, err := db.Exec("UPDATE downloads SET last_transition_at = ? WHERE id = ?", oldTime, d1.ID)
	require.NoError(t, err)

	// Not stuck - recently added
	d2 := &Download{
		ContentID:   contentID2,
		Client:      ClientManual,
		ClientID:    "recent-queued",
		Status:      StatusQueued,
		ReleaseName: "Recent.Queued",
		Indexer:     "manual",
	}
	require.NoError(t, store.Add(d2))

	// Stuck in downloading (> 24 hours would be stuck, but 2 hours is not)
	d3 := &Download{
		ContentID:   contentID3,
		Client:      ClientManual,
		ClientID:    "downloading",
		Status:      StatusDownloading,
		ReleaseName: "Downloading",
		Indexer:     "manual",
	}
	require.NoError(t, store.Add(d3))
	_, err = db.Exec("UPDATE downloads SET status = 'downloading', last_transition_at = ? WHERE id = ?", oldTime, d3.ID)
	require.NoError(t, err)

	thresholds := map[Status]time.Duration{
		StatusQueued:      1 * time.Hour,
		StatusDownloading: 24 * time.Hour,
		StatusCompleted:   1 * time.Hour,
		StatusImported:    24 * time.Hour,
	}

	stuck, err := store.ListStuck(thresholds)
	require.NoError(t, err)

	// Only d1 should be stuck (queued for > 1 hour)
	// d2 is recent, d3 is downloading but only 2 hours (threshold is 24)
	require.Len(t, stuck, 1)
	assert.Equal(t, d1.ID, stuck[0].ID)
}

func TestStore_Transition_FailedToQueued(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Retry Movie")

	// Add a failed download
	d := &Download{
		ContentID:   contentID,
		Client:      ClientManual,
		ClientID:    "test-retry",
		Status:      StatusFailed,
		ReleaseName: "Failed.Movie",
		Indexer:     "manual",
	}
	require.NoError(t, store.Add(d))

	// Retry: transition from failed to queued
	beforeRetry := time.Now()
	err := store.Transition(d, StatusQueued)
	require.NoError(t, err, "failed -> queued should be valid (retry)")

	assert.Equal(t, StatusQueued, d.Status, "status should be queued after retry")
	assert.True(t, d.LastTransitionAt.After(beforeRetry) || d.LastTransitionAt.Equal(beforeRetry),
		"LastTransitionAt should be updated on retry")

	// Verify in DB
	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status)
}

func TestStore_List_Pagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Pagination Test")

	// Add 5 downloads
	for i := 0; i < 5; i++ {
		d := &Download{
			ContentID:   contentID,
			Client:      ClientSABnzbd,
			ClientID:    fmt.Sprintf("nzo_%d", i),
			Status:      StatusQueued,
			ReleaseName: fmt.Sprintf("release%d", i),
			Indexer:     "idx",
		}
		require.NoError(t, store.Add(d))
	}

	// Test: List all without pagination
	results, total, err := store.List(Filter{})
	require.NoError(t, err)
	assert.Len(t, results, 5)
	assert.Equal(t, 5, total)

	// Test: Limit to 2
	results, total, err = store.List(Filter{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, 5, total, "total should be all matching, not just page size")

	// Test: Limit 2, Offset 2 (second page)
	results, total, err = store.List(Filter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, 5, total)

	// Test: Limit 2, Offset 4 (third page - partial)
	results, total, err = store.List(Filter{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 5, total)

	// Test: Offset beyond data
	results, total, err = store.List(Filter{Limit: 2, Offset: 10})
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, 5, total)

	// Test: Pagination with filter
	status := StatusQueued
	results, total, err = store.List(Filter{Status: &status, Limit: 2, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, 5, total)
}

func TestStore_CountByStatus(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Count Test")

	// Create downloads with different statuses
	statuses := []Status{StatusQueued, StatusQueued, StatusDownloading, StatusCompleted, StatusFailed}
	for i, status := range statuses {
		d := &Download{
			ContentID:   contentID,
			Client:      ClientSABnzbd,
			ClientID:    fmt.Sprintf("nzo_%d", i),
			Status:      status,
			ReleaseName: fmt.Sprintf("Release.%d", i),
			Indexer:     "test",
		}
		require.NoError(t, store.Add(d))
	}

	counts, err := store.CountByStatus()
	require.NoError(t, err)

	assert.Equal(t, 2, counts[StatusQueued])
	assert.Equal(t, 1, counts[StatusDownloading])
	assert.Equal(t, 1, counts[StatusCompleted])
	assert.Equal(t, 1, counts[StatusFailed])
	assert.Equal(t, 0, counts[StatusImported])
}

func TestStore_CountByStatus_Empty(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	counts, err := store.CountByStatus()
	require.NoError(t, err)
	assert.Empty(t, counts, "should return empty map when no downloads exist")
}
