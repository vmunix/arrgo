package download

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockDownloader struct {
	addResult    string
	addErr       error
	statusResult *ClientStatus
	statusErr    error
	listResult   []*ClientStatus
	listErr      error
	removeErr    error
	removeCalled bool
}

func (m *mockDownloader) Add(ctx context.Context, url, category string) (string, error) {
	return m.addResult, m.addErr
}

func (m *mockDownloader) Status(ctx context.Context, clientID string) (*ClientStatus, error) {
	return m.statusResult, m.statusErr
}

func (m *mockDownloader) List(ctx context.Context) ([]*ClientStatus, error) {
	return m.listResult, m.listErr
}

func (m *mockDownloader) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	m.removeCalled = true
	return m.removeErr
}

func TestManager_Grab(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	client := &mockDownloader{addResult: "nzo_abc123"}
	mgr := NewManager(client, store)

	d, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024.1080p", "TestIndexer")
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}

	if d.ClientID != "nzo_abc123" {
		t.Errorf("ClientID = %q, want nzo_abc123", d.ClientID)
	}
	if d.Status != StatusQueued {
		t.Errorf("Status = %q, want queued", d.Status)
	}
	if d.ID == 0 {
		t.Error("download should be saved to DB")
	}
}

func TestManager_Grab_WithEpisodeID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series
	result, err := db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('series', 'Test Show', 2024, 'wanted', 'hd', '/tv')`)
	if err != nil {
		t.Fatalf("insert content: %v", err)
	}
	contentID, _ := result.LastInsertId()

	// Create episode
	epResult, err := db.Exec(`INSERT INTO episodes (content_id, season, episode, title, status)
		VALUES (?, 1, 1, 'Pilot', 'wanted')`, contentID)
	if err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	episodeID, _ := epResult.LastInsertId()

	client := &mockDownloader{addResult: "nzo_ep1"}
	mgr := NewManager(client, store)

	d, err := mgr.Grab(context.Background(), contentID, &episodeID, "http://example.com/ep.nzb", "Test.Show.S01E01", "Indexer")
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}

	if d.EpisodeID == nil || *d.EpisodeID != episodeID {
		t.Errorf("EpisodeID = %v, want %d", d.EpisodeID, episodeID)
	}
}

func TestManager_Grab_ClientError(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	client := &mockDownloader{addErr: ErrClientUnavailable}
	mgr := NewManager(client, store)

	_, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie", "Indexer")
	if !errors.Is(err, ErrClientUnavailable) {
		t.Errorf("expected ErrClientUnavailable, got %v", err)
	}

	// Should not have saved to DB
	downloads, _ := store.List(DownloadFilter{})
	if len(downloads) != 0 {
		t.Error("download should not be in DB after client error")
	}
}

func TestManager_Grab_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	client := &mockDownloader{addResult: "nzo_abc123"}
	mgr := NewManager(client, store)

	d1, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024", "Indexer")
	if err != nil {
		t.Fatalf("Grab first: %v", err)
	}

	// Second grab with same content + release should be idempotent
	client.addResult = "nzo_different"
	d2, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024", "Indexer")
	if err != nil {
		t.Fatalf("Grab second: %v", err)
	}

	if d1.ID != d2.ID {
		t.Errorf("expected same download ID %d, got %d", d1.ID, d2.ID)
	}
}

func TestManager_Refresh(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	// Add a download
	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	// Mock client returns completed status
	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:       "nzo_abc123",
			Status:   StatusCompleted,
			Progress: 100,
			Path:     "/complete/Test.Movie",
		},
	}
	mgr := NewManager(client, store)

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Should have updated status in DB
	updated, _ := store.Get(d.ID)
	if updated.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", updated.Status)
	}
	if updated.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestManager_Refresh_NoChange(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	// Mock client returns same status
	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:       "nzo_abc123",
			Status:   StatusDownloading,
			Progress: 50,
		},
	}
	mgr := NewManager(client, store)

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Status should remain unchanged
	updated, _ := store.Get(d.ID)
	if updated.Status != StatusDownloading {
		t.Errorf("Status = %q, want downloading", updated.Status)
	}
	if updated.CompletedAt != nil {
		t.Error("CompletedAt should not be set")
	}
}

func TestManager_Refresh_Failed(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	// Mock client returns failed status
	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:     "nzo_abc123",
			Status: StatusFailed,
		},
	}
	mgr := NewManager(client, store)

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	updated, _ := store.Get(d.ID)
	if updated.Status != StatusFailed {
		t.Errorf("Status = %q, want failed", updated.Status)
	}
	if updated.CompletedAt == nil {
		t.Error("CompletedAt should be set for failed downloads")
	}
}

func TestManager_Cancel(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	client := &mockDownloader{}
	mgr := NewManager(client, store)

	if err := mgr.Cancel(context.Background(), d.ID, false); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if !client.removeCalled {
		t.Error("client.Remove should have been called")
	}

	// Should be deleted from DB
	_, err := store.Get(d.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after cancel, got %v", err)
	}
}

func TestManager_Cancel_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	client := &mockDownloader{}
	mgr := NewManager(client, store)

	err := mgr.Cancel(context.Background(), 9999, false)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestManager_Cancel_ClientError(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	// Client error should not prevent DB deletion
	client := &mockDownloader{removeErr: ErrClientUnavailable}
	mgr := NewManager(client, store)

	if err := mgr.Cancel(context.Background(), d.ID, false); err != nil {
		t.Fatalf("Cancel should succeed despite client error: %v", err)
	}

	// Should still be deleted from DB
	_, err := store.Get(d.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after cancel, got %v", err)
	}
}

func TestManager_GetActive(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:       "nzo_abc123",
			Status:   StatusDownloading,
			Progress: 50,
			Speed:    10000000,
			ETA:      5 * time.Minute,
		},
	}
	mgr := NewManager(client, store)

	active, err := mgr.GetActive(context.Background())
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active download, got %d", len(active))
	}
	if active[0].Download.ID != d.ID {
		t.Errorf("Download.ID = %d, want %d", active[0].Download.ID, d.ID)
	}
	if active[0].Live == nil {
		t.Fatal("Live status should be set")
	}
	if active[0].Live.Progress != 50 {
		t.Errorf("Progress = %f, want 50", active[0].Live.Progress)
	}
}

func TestManager_GetActive_ClientError(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	d := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	// Client error should still return download without live status
	client := &mockDownloader{statusErr: ErrClientUnavailable}
	mgr := NewManager(client, store)

	active, err := mgr.GetActive(context.Background())
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active download, got %d", len(active))
	}
	if active[0].Download.ID != d.ID {
		t.Errorf("Download.ID = %d, want %d", active[0].Download.ID, d.ID)
	}
	if active[0].Live != nil {
		t.Error("Live status should be nil when client errors")
	}
}

func TestManager_GetActive_ExcludesImported(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	// Add active download
	d1 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_1",
		Status:      StatusDownloading,
		ReleaseName: "Active.Movie",
	}
	_ = store.Add(d1)

	// Add imported download
	d2 := &Download{
		ContentID:   contentID,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_2",
		Status:      StatusImported,
		ReleaseName: "Imported.Movie",
	}
	_ = store.Add(d2)

	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:       "nzo_1",
			Status:   StatusDownloading,
			Progress: 50,
		},
	}
	mgr := NewManager(client, store)

	active, err := mgr.GetActive(context.Background())
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active download (excluding imported), got %d", len(active))
	}
	if active[0].Download.ReleaseName != "Active.Movie" {
		t.Errorf("expected Active.Movie, got %s", active[0].Download.ReleaseName)
	}
}
