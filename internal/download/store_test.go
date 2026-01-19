package download

import (
	"errors"
	"testing"
	"time"
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
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}
	after := time.Now()

	// ID should be set
	if d.ID == 0 {
		t.Error("ID should be set after Add")
	}

	// AddedAt should be set
	if d.AddedAt.Before(before) || d.AddedAt.After(after) {
		t.Errorf("AddedAt %v not in expected range [%v, %v]", d.AddedAt, before, after)
	}
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

	if err := store.Add(d1); err != nil {
		t.Fatalf("Add first: %v", err)
	}
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

	if err := store.Add(d2); err != nil {
		t.Fatalf("Add second: %v", err)
	}

	// Should return the existing record's ID
	if d2.ID != firstID {
		t.Errorf("idempotent Add: got ID %d, want %d", d2.ID, firstID)
	}
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

	if err := store.Add(d1); err != nil {
		t.Fatalf("Add first: %v", err)
	}

	// Add same content_id but different release_name - should create new record
	d2 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "SABnzbd_nzo_xyz789",
		Status:      StatusQueued,
		ReleaseName: "Fight.Club.1999.720p.BluRay.x264",
		Indexer:     "nzbgeek",
	}

	if err := store.Add(d2); err != nil {
		t.Fatalf("Add second: %v", err)
	}

	// Should create a new record with different ID
	if d2.ID == d1.ID {
		t.Error("different release_name should create new record")
	}
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
	if err := store.Add(original); err != nil {
		t.Fatalf("Add: %v", err)
	}

	retrieved, err := store.Get(original.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify all fields
	if retrieved.ID != original.ID {
		t.Errorf("ID = %d, want %d", retrieved.ID, original.ID)
	}
	if retrieved.ContentID != original.ContentID {
		t.Errorf("ContentID = %d, want %d", retrieved.ContentID, original.ContentID)
	}
	if retrieved.Client != original.Client {
		t.Errorf("Client = %q, want %q", retrieved.Client, original.Client)
	}
	if retrieved.ClientID != original.ClientID {
		t.Errorf("ClientID = %q, want %q", retrieved.ClientID, original.ClientID)
	}
	if retrieved.Status != original.Status {
		t.Errorf("Status = %q, want %q", retrieved.Status, original.Status)
	}
	if retrieved.ReleaseName != original.ReleaseName {
		t.Errorf("ReleaseName = %q, want %q", retrieved.ReleaseName, original.ReleaseName)
	}
	if retrieved.Indexer != original.Indexer {
		t.Errorf("Indexer = %q, want %q", retrieved.Indexer, original.Indexer)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.Get(9999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(9999) error = %v, want ErrNotFound", err)
	}
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
	if err := store.Add(original); err != nil {
		t.Fatalf("Add: %v", err)
	}

	retrieved, err := store.GetByClientID(ClientSABnzbd, "SABnzbd_nzo_abc123")
	if err != nil {
		t.Fatalf("GetByClientID: %v", err)
	}

	if retrieved.ID != original.ID {
		t.Errorf("ID = %d, want %d", retrieved.ID, original.ID)
	}
}

func TestStore_GetByClientID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetByClientID(ClientSABnzbd, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByClientID(nonexistent) error = %v, want ErrNotFound", err)
	}
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
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Update status and completed_at
	d.Status = StatusCompleted
	now := time.Now()
	d.CompletedAt = &now

	if err := store.Update(d); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Verify in database
	retrieved, err := store.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if retrieved.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", retrieved.Status)
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
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
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Update error = %v, want ErrNotFound", err)
	}
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
		if err := store.Add(d); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// List all
	results, err := store.List(DownloadFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
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
		if err := store.Add(d); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// List active (excludes terminal states: cleaned, failed)
	results, err := store.List(DownloadFilter{Active: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("len(results) = %d, want 4 (excludes cleaned and failed)", len(results))
	}

	// Verify no terminal status in results
	for _, d := range results {
		if d.Status == StatusCleaned || d.Status == StatusFailed {
			t.Errorf("Active filter should exclude terminal status, found: %v", d)
		}
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

	if err := store.Add(d1); err != nil {
		t.Fatalf("Add d1: %v", err)
	}
	if err := store.Add(d2); err != nil {
		t.Fatalf("Add d2: %v", err)
	}

	// Filter by content ID
	results, err := store.List(DownloadFilter{ContentID: &contentID1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].ContentID != contentID1 {
		t.Errorf("ContentID = %d, want %d", results[0].ContentID, contentID1)
	}
}

func TestStore_List_FilterByStatus(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add downloads with different statuses
	d1 := &Download{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"}
	d2 := &Download{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusDownloading, ReleaseName: "release2", Indexer: "idx2"}

	if err := store.Add(d1); err != nil {
		t.Fatalf("Add d1: %v", err)
	}
	if err := store.Add(d2); err != nil {
		t.Fatalf("Add d2: %v", err)
	}

	// Filter by status
	status := StatusDownloading
	results, err := store.List(DownloadFilter{Status: &status})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != StatusDownloading {
		t.Errorf("Status = %q, want downloading", results[0].Status)
	}
}

func TestStore_List_FilterByClient(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Fight Club")

	// Add downloads for different clients
	d1 := &Download{ContentID: contentID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "release1", Indexer: "idx1"}
	d2 := &Download{ContentID: contentID, Client: ClientQBittorrent, ClientID: "hash123", Status: StatusQueued, ReleaseName: "release2", Indexer: "idx2"}

	if err := store.Add(d1); err != nil {
		t.Fatalf("Add d1: %v", err)
	}
	if err := store.Add(d2); err != nil {
		t.Fatalf("Add d2: %v", err)
	}

	// Filter by client
	client := ClientSABnzbd
	results, err := store.List(DownloadFilter{Client: &client})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].Client != ClientSABnzbd {
		t.Errorf("Client = %q, want sabnzbd", results[0].Client)
	}
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
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Delete
	if err := store.Delete(d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	_, err := store.Get(d.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: error = %v, want ErrNotFound", err)
	}
}

func TestStore_Delete_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	if err := store.Delete(9999); err != nil {
		t.Errorf("Delete(9999) = %v, want nil (idempotent)", err)
	}
}

func TestStore_Add_WithEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series content
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', 'Breaking Bad', 2008, 'wanted', 'hd', '/tv')`)
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}
	contentID, _ := result.LastInsertId()

	// Create episode
	result, err = db.Exec(`
		INSERT INTO episodes (content_id, season, episode, title, status)
		VALUES (?, 1, 1, 'Pilot', 'wanted')`, contentID)
	if err != nil {
		t.Fatalf("insert episode: %v", err)
	}
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

	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	retrieved, err := store.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if retrieved.EpisodeID == nil {
		t.Error("EpisodeID should not be nil")
	} else if *retrieved.EpisodeID != episodeID {
		t.Errorf("EpisodeID = %d, want %d", *retrieved.EpisodeID, episodeID)
	}
}

func TestStore_List_FilterByEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series content
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', 'Breaking Bad', 2008, 'wanted', 'hd', '/tv')`)
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}
	contentID, _ := result.LastInsertId()

	// Create episodes
	result, _ = db.Exec(`INSERT INTO episodes (content_id, season, episode, title, status) VALUES (?, 1, 1, 'Pilot', 'wanted')`, contentID)
	ep1ID, _ := result.LastInsertId()
	result, _ = db.Exec(`INSERT INTO episodes (content_id, season, episode, title, status) VALUES (?, 1, 2, 'Cat in the Bag', 'wanted')`, contentID)
	ep2ID, _ := result.LastInsertId()

	// Add downloads for different episodes
	d1 := &Download{ContentID: contentID, EpisodeID: &ep1ID, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "S01E01", Indexer: "idx"}
	d2 := &Download{ContentID: contentID, EpisodeID: &ep2ID, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusQueued, ReleaseName: "S01E02", Indexer: "idx"}

	if err := store.Add(d1); err != nil {
		t.Fatalf("Add d1: %v", err)
	}
	if err := store.Add(d2); err != nil {
		t.Fatalf("Add d2: %v", err)
	}

	// Filter by episode ID
	results, err := store.List(DownloadFilter{EpisodeID: &ep1ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].EpisodeID == nil || *results[0].EpisodeID != ep1ID {
		t.Errorf("EpisodeID = %v, want %d", results[0].EpisodeID, ep1ID)
	}
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
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}
	after := time.Now()

	// LastTransitionAt should be set on Add
	if d.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set after Add")
	}

	// Verify it's within expected time range
	if d.LastTransitionAt.Before(before) || d.LastTransitionAt.After(after) {
		t.Errorf("LastTransitionAt %v not in expected range [%v, %v]", d.LastTransitionAt, before, after)
	}

	// Retrieve and verify via Get
	got, err := store.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set after Get")
	}

	// Retrieve and verify via GetByClientID
	gotByClient, err := store.GetByClientID(ClientManual, "test-123")
	if err != nil {
		t.Fatalf("GetByClientID: %v", err)
	}
	if gotByClient.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set after GetByClientID")
	}

	// Retrieve and verify via List
	downloads, err := store.List(DownloadFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(downloads))
	}
	if downloads[0].LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set in List results")
	}
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
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Valid transition
	oldTime := d.LastTransitionAt
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	if err := store.Transition(d, StatusDownloading); err != nil {
		t.Fatalf("Transition: %v", err)
	}

	if d.Status != StatusDownloading {
		t.Errorf("Status = %s, want downloading", d.Status)
	}
	if !d.LastTransitionAt.After(oldTime) {
		t.Error("LastTransitionAt should be updated")
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].From != StatusQueued || events[0].To != StatusDownloading {
		t.Errorf("event = %v, want queued->downloading", events[0])
	}

	// Invalid transition
	if err := store.Transition(d, StatusCleaned); err == nil {
		t.Error("should reject invalid transition downloading->cleaned")
	}
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
	if err := store.Add(d1); err != nil {
		t.Fatalf("Add d1: %v", err)
	}
	// Manually set old timestamp
	if _, err := db.Exec("UPDATE downloads SET last_transition_at = ? WHERE id = ?", oldTime, d1.ID); err != nil {
		t.Fatalf("update d1 timestamp: %v", err)
	}

	// Not stuck - recently added
	d2 := &Download{
		ContentID:   contentID2,
		Client:      ClientManual,
		ClientID:    "recent-queued",
		Status:      StatusQueued,
		ReleaseName: "Recent.Queued",
		Indexer:     "manual",
	}
	if err := store.Add(d2); err != nil {
		t.Fatalf("Add d2: %v", err)
	}

	// Stuck in downloading (> 24 hours would be stuck, but 2 hours is not)
	d3 := &Download{
		ContentID:   contentID3,
		Client:      ClientManual,
		ClientID:    "downloading",
		Status:      StatusDownloading,
		ReleaseName: "Downloading",
		Indexer:     "manual",
	}
	if err := store.Add(d3); err != nil {
		t.Fatalf("Add d3: %v", err)
	}
	if _, err := db.Exec("UPDATE downloads SET status = 'downloading', last_transition_at = ? WHERE id = ?", oldTime, d3.ID); err != nil {
		t.Fatalf("update d3 timestamp: %v", err)
	}

	thresholds := map[Status]time.Duration{
		StatusQueued:      1 * time.Hour,
		StatusDownloading: 24 * time.Hour,
		StatusCompleted:   1 * time.Hour,
		StatusImported:    24 * time.Hour,
	}

	stuck, err := store.ListStuck(thresholds)
	if err != nil {
		t.Fatalf("ListStuck: %v", err)
	}

	// Only d1 should be stuck (queued for > 1 hour)
	// d2 is recent, d3 is downloading but only 2 hours (threshold is 24)
	if len(stuck) != 1 {
		t.Fatalf("got %d stuck, want 1", len(stuck))
	}
	if stuck[0].ID != d1.ID {
		t.Errorf("stuck[0].ID = %d, want %d", stuck[0].ID, d1.ID)
	}
}
