# Series Episode Handling Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the disconnect between release parsing and episode tracking so that season packs and multi-episode releases import correctly.

**Architecture:** Add download_episodes junction table, parse release names at grab time to detect episodes, and map individual files to episodes during import.

**Tech Stack:** Go, SQLite, testify, gomock

---

## Task 1: Add download_episodes migration

**Files:**
- Create: `migrations/002_download_episodes.sql`

**Step 1: Write the migration**

```sql
-- Migration: Add download_episodes junction table
-- This allows a single download to reference multiple episodes (for season packs, multi-episode releases)

-- Create junction table
CREATE TABLE IF NOT EXISTS download_episodes (
    download_id INTEGER NOT NULL REFERENCES downloads(id) ON DELETE CASCADE,
    episode_id  INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    PRIMARY KEY (download_id, episode_id)
);

-- Index for efficient lookups by episode
CREATE INDEX IF NOT EXISTS idx_download_episodes_episode_id ON download_episodes(episode_id);

-- Migrate existing episode_id data from downloads table
INSERT INTO download_episodes (download_id, episode_id)
SELECT id, episode_id FROM downloads WHERE episode_id IS NOT NULL;

-- Add season tracking columns to downloads (for season packs)
ALTER TABLE downloads ADD COLUMN season INTEGER;
ALTER TABLE downloads ADD COLUMN is_complete_season INTEGER DEFAULT 0;
```

**Step 2: Verify migration syntax**

Run: `sqlite3 :memory: < migrations/002_download_episodes.sql`
Expected: No output (clean execution)

**Step 3: Commit**

```bash
git add migrations/002_download_episodes.sql
git commit -m "feat(db): add download_episodes junction table (#67)"
```

---

## Task 2: Update Download struct and Store for episode IDs

**Files:**
- Modify: `internal/download/download.go`
- Modify: `internal/download/store.go`

**Step 1: Write failing test for EpisodeIDs field**

Add to `internal/download/store_test.go`:

```go
func TestStore_AddWithEpisodeIDs(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)

	// Create test content and episodes first
	_, err := db.Exec(`INSERT INTO content (id, type, title, year, status, quality_profile, root_path) VALUES (1, 'series', 'Test Show', 2024, 'wanted', 'hd', '/tv')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO episodes (id, content_id, season, episode, title, status) VALUES (10, 1, 1, 1, 'Ep1', 'wanted'), (11, 1, 1, 2, 'Ep2', 'wanted')`)
	require.NoError(t, err)

	dl := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_test",
		Status:      StatusQueued,
		ReleaseName: "Test.Show.S01E01E02.1080p",
		Indexer:     "test",
	}

	err = store.Add(dl)
	require.NoError(t, err)
	require.NotZero(t, dl.ID)

	// Link episodes
	err = store.SetEpisodeIDs(dl.ID, []int64{10, 11})
	require.NoError(t, err)

	// Retrieve and verify
	got, err := store.Get(dl.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{10, 11}, got.EpisodeIDs)
}

func TestStore_AddWithSeasonPack(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)

	_, err := db.Exec(`INSERT INTO content (id, type, title, year, status, quality_profile, root_path) VALUES (1, 'series', 'Test Show', 2024, 'wanted', 'hd', '/tv')`)
	require.NoError(t, err)

	dl := &Download{
		ContentID:        1,
		Client:           ClientSABnzbd,
		ClientID:         "nzo_season",
		Status:           StatusQueued,
		ReleaseName:      "Test.Show.S01.1080p",
		Indexer:          "test",
		Season:           intPtr(1),
		IsCompleteSeason: true,
	}

	err = store.Add(dl)
	require.NoError(t, err)

	got, err := store.Get(dl.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, *got.Season)
	assert.True(t, got.IsCompleteSeason)
}

func intPtr(i int) *int { return &i }
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestStore_AddWithEpisodeIDs -v`
Expected: FAIL - SetEpisodeIDs method doesn't exist, EpisodeIDs field doesn't exist

**Step 3: Update Download struct**

In `internal/download/download.go`, update the Download struct (around line 37):

```go
// Download represents an active or recent download.
type Download struct {
	ID               int64
	ContentID        int64
	EpisodeID        *int64  // Deprecated: use EpisodeIDs. Kept for backward compat.
	EpisodeIDs       []int64 // Episode IDs from junction table
	Season           *int    // For season packs: which season
	IsCompleteSeason bool    // True if this is a complete season pack
	Client           Client
	ClientID         string // ID in the download client
	Status           Status
	ReleaseName      string
	Indexer          string
	AddedAt          time.Time
	CompletedAt      *time.Time
	LastTransitionAt time.Time
}
```

**Step 4: Update Store.Add to handle season columns**

In `internal/download/store.go`, update the Add method's INSERT (around line 127):

```go
	result, err := s.db.Exec(`
		INSERT INTO downloads (content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at, season, is_complete_season)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ContentID, d.EpisodeID, d.Client, d.ClientID, d.Status, d.ReleaseName, d.Indexer, now, d.CompletedAt, now, d.Season, d.IsCompleteSeason,
	)
```

**Step 5: Update Store.Get to load EpisodeIDs**

In `internal/download/store.go`, update the Get method:

```go
func (s *Store) Get(id int64) (*Download, error) {
	d := &Download{}
	err := s.db.QueryRow(`
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at, season, is_complete_season
		FROM downloads WHERE id = ?`, id,
	).Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt, &d.Season, &d.IsCompleteSeason)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("get download %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get download %d: %w", id, err)
	}

	// Load episode IDs from junction table
	d.EpisodeIDs, err = s.getEpisodeIDs(id)
	if err != nil {
		return nil, fmt.Errorf("get episode IDs for download %d: %w", id, err)
	}

	return d, nil
}

func (s *Store) getEpisodeIDs(downloadID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT episode_id FROM download_episodes WHERE download_id = ? ORDER BY episode_id`, downloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

**Step 6: Add SetEpisodeIDs method**

In `internal/download/store.go`:

```go
// SetEpisodeIDs sets the episode IDs for a download using the junction table.
// This replaces any existing episode associations.
func (s *Store) SetEpisodeIDs(downloadID int64, episodeIDs []int64) error {
	// Delete existing associations
	if _, err := s.db.Exec(`DELETE FROM download_episodes WHERE download_id = ?`, downloadID); err != nil {
		return fmt.Errorf("clear episode IDs: %w", err)
	}

	// Insert new associations
	for _, epID := range episodeIDs {
		if _, err := s.db.Exec(`INSERT INTO download_episodes (download_id, episode_id) VALUES (?, ?)`, downloadID, epID); err != nil {
			return fmt.Errorf("insert episode ID %d: %w", epID, err)
		}
	}

	return nil
}
```

**Step 7: Update other Get* methods similarly**

Update `GetByClientID` to also load EpisodeIDs and season columns.

**Step 8: Update List method to load season columns**

Update the SELECT and Scan in List method to include season, is_complete_season columns.

**Step 9: Run tests to verify they pass**

Run: `task test -- -run TestStore_AddWith -v`
Expected: PASS

**Step 10: Commit**

```bash
git add internal/download/download.go internal/download/store.go internal/download/store_test.go
git commit -m "feat(download): add EpisodeIDs junction table support (#67)"
```

---

## Task 3: Add FindOrCreateEpisode to library

**Files:**
- Modify: `internal/library/episode.go`
- Modify: `internal/library/episode_test.go`

**Step 1: Write failing test**

Add to `internal/library/episode_test.go`:

```go
func TestStore_FindOrCreateEpisode(t *testing.T) {
	db := testutil.NewTestDB(t)
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
	db := testutil.NewTestDB(t)
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
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestStore_FindOrCreateEpisode -v`
Expected: FAIL - FindOrCreateEpisode method doesn't exist

**Step 3: Implement FindOrCreateEpisode**

Add to `internal/library/episode.go`:

```go
// FindOrCreateEpisode finds an existing episode or creates a new one.
// Returns (episode, created, error) where created is true if a new episode was created.
func (s *Store) FindOrCreateEpisode(contentID int64, season, episode int) (*Episode, bool, error) {
	// Try to find existing
	eps, _, err := s.ListEpisodes(EpisodeFilter{
		ContentID: &contentID,
		Season:    &season,
	})
	if err != nil {
		return nil, false, fmt.Errorf("list episodes: %w", err)
	}

	for _, ep := range eps {
		if ep.Episode == episode {
			return ep, false, nil
		}
	}

	// Not found, create new
	ep := &Episode{
		ContentID: contentID,
		Season:    season,
		Episode:   episode,
		Status:    StatusWanted,
	}
	if err := s.AddEpisode(ep); err != nil {
		return nil, false, fmt.Errorf("add episode: %w", err)
	}

	return ep, true, nil
}

// FindOrCreateEpisodes finds or creates multiple episodes for a season.
// Returns the episodes in the same order as the input episode numbers.
func (s *Store) FindOrCreateEpisodes(contentID int64, season int, episodeNums []int) ([]*Episode, error) {
	result := make([]*Episode, 0, len(episodeNums))

	for _, epNum := range episodeNums {
		ep, _, err := s.FindOrCreateEpisode(contentID, season, epNum)
		if err != nil {
			return nil, err
		}
		result = append(result, ep)
	}

	return result, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `task test -- -run TestStore_FindOrCreateEpisode -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/library/episode.go internal/library/episode_test.go
git commit -m "feat(library): add FindOrCreateEpisode helper (#67)"
```

---

## Task 4: Update GrabRequested event for multi-episode support

**Files:**
- Modify: `internal/events/download.go`

**Step 1: Update GrabRequested struct**

In `internal/events/download.go`, update GrabRequested (around line 31):

```go
// GrabRequested is emitted when a user/API requests a download.
type GrabRequested struct {
	BaseEvent
	ContentID        int64   `json:"content_id"`
	EpisodeID        *int64  `json:"episode_id,omitempty"`         // Deprecated: use EpisodeIDs
	EpisodeIDs       []int64 `json:"episode_ids,omitempty"`        // Episode IDs for multi-episode grabs
	Season           *int    `json:"season,omitempty"`             // Season number (for season packs)
	IsCompleteSeason bool    `json:"is_complete_season,omitempty"` // True if grabbing complete season
	DownloadURL      string  `json:"download_url"`
	ReleaseName      string  `json:"release_name"`
	Indexer          string  `json:"indexer"`
}
```

**Step 2: Update DownloadCreated similarly**

```go
// DownloadCreated is emitted when a download record is created.
type DownloadCreated struct {
	BaseEvent
	DownloadID       int64   `json:"download_id"`
	ContentID        int64   `json:"content_id"`
	EpisodeID        *int64  `json:"episode_id,omitempty"`  // Deprecated
	EpisodeIDs       []int64 `json:"episode_ids,omitempty"` // Episode IDs
	Season           *int    `json:"season,omitempty"`
	IsCompleteSeason bool    `json:"is_complete_season,omitempty"`
	ClientID         string  `json:"client_id"`
	ReleaseName      string  `json:"release_name"`
}
```

**Step 3: Commit**

```bash
git add internal/events/download.go
git commit -m "feat(events): add multi-episode fields to GrabRequested (#67)"
```

---

## Task 5: Update grab endpoint to parse release and detect episodes

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/requests.go` (if exists, otherwise in api.go)

**Step 1: Write failing test**

Add to `internal/api/v1/api_test.go`:

```go
func TestGrab_SeriesWithEpisodeDetection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	bus := events.NewBus(slog.Default())
	mockManager := mocks.NewMockDownloadManager(ctrl)

	// Create series content
	store := library.NewStore(db)
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(content))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Subscribe to capture event
	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Grab with release name containing S01E05
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Test.Show.S01E05.1080p.WEB",
		"indexer": "TestIndexer"
	}`, content.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	// Verify event has episode info
	select {
	case e := <-eventCh:
		grab := e.(*events.GrabRequested)
		assert.Len(t, grab.EpisodeIDs, 1)
		assert.Equal(t, 1, *grab.Season)
		assert.False(t, grab.IsCompleteSeason)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Verify episode was created
	eps, _, err := store.ListEpisodes(library.EpisodeFilter{ContentID: &content.ID})
	require.NoError(t, err)
	require.Len(t, eps, 1)
	assert.Equal(t, 1, eps[0].Season)
	assert.Equal(t, 5, eps[0].Episode)
}

func TestGrab_SeasonPack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	bus := events.NewBus(slog.Default())
	mockManager := mocks.NewMockDownloadManager(ctrl)

	store := library.NewStore(db)
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(content))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Grab season pack
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Test.Show.S01.1080p.WEB",
		"indexer": "TestIndexer"
	}`, content.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	// Verify event is season pack
	select {
	case e := <-eventCh:
		grab := e.(*events.GrabRequested)
		assert.Empty(t, grab.EpisodeIDs) // No episodes yet for season pack
		assert.Equal(t, 1, *grab.Season)
		assert.True(t, grab.IsCompleteSeason)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestGrab_SeriesNoEpisodeInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	bus := events.NewBus(slog.Default())
	mockManager := mocks.NewMockDownloadManager(ctrl)

	store := library.NewStore(db)
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(content))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab without episode info in title
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Test.Show.1080p.WEB",
		"indexer": "TestIndexer"
	}`, content.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "episode")
}
```

**Step 2: Run tests to verify they fail**

Run: `task test -- -run "TestGrab_Series|TestGrab_SeasonPack" -v`
Expected: FAIL - Episode detection not implemented

**Step 3: Update grab handler to detect episodes**

In `internal/api/v1/api.go`, update the `grab` function (around line 531):

```go
func (s *Server) grab(w http.ResponseWriter, r *http.Request) {
	var req grabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Validate required fields
	if req.ContentID == 0 {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "content_id is required")
		return
	}
	if req.DownloadURL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "download_url is required")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "title is required")
		return
	}

	// Get content to check type
	content, err := s.deps.Library.GetContent(req.ContentID)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Require event bus for grab operations
	if s.deps.Bus == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_DOWNLOAD_CLIENT", "download client not configured")
		return
	}

	// Build grab event
	event := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   req.ContentID,
		DownloadURL: req.DownloadURL,
		ReleaseName: req.Title,
		Indexer:     req.Indexer,
	}

	// For series, parse release name to detect episodes
	if content.Type == library.ContentTypeSeries {
		episodeInfo, err := s.parseSeriesRelease(req.ContentID, req.Title, req.Season, req.Episodes)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_RELEASE", err.Error())
			return
		}
		event.EpisodeIDs = episodeInfo.EpisodeIDs
		event.Season = episodeInfo.Season
		event.IsCompleteSeason = episodeInfo.IsCompleteSeason

		// Backward compat: set EpisodeID if single episode
		if len(episodeInfo.EpisodeIDs) == 1 {
			event.EpisodeID = &episodeInfo.EpisodeIDs[0]
		}
	}

	if err := s.deps.Bus.Publish(r.Context(), event); err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

type episodeInfo struct {
	EpisodeIDs       []int64
	Season           *int
	IsCompleteSeason bool
}

// parseSeriesRelease parses a release name and creates/finds episodes.
// Returns error if no episode info can be determined.
func (s *Server) parseSeriesRelease(contentID int64, releaseName string, overrideSeason *int, overrideEpisodes []int) (*episodeInfo, error) {
	parsed := release.Parse(releaseName)

	// Use override if provided
	season := parsed.Season
	if overrideSeason != nil {
		season = *overrideSeason
	}

	episodes := parsed.Episodes
	if len(overrideEpisodes) > 0 {
		episodes = overrideEpisodes
	}

	// Validate we have at least season info
	if season == 0 {
		return nil, fmt.Errorf("cannot determine season from release title: %s", releaseName)
	}

	result := &episodeInfo{
		Season: &season,
	}

	// Season pack: no specific episodes
	if parsed.IsCompleteSeason && len(episodes) == 0 {
		result.IsCompleteSeason = true
		return result, nil
	}

	// Specific episodes: find or create
	if len(episodes) == 0 {
		return nil, fmt.Errorf("cannot determine episodes from release title: %s", releaseName)
	}

	eps, err := s.deps.Library.FindOrCreateEpisodes(contentID, season, episodes)
	if err != nil {
		return nil, fmt.Errorf("create episodes: %w", err)
	}

	for _, ep := range eps {
		result.EpisodeIDs = append(result.EpisodeIDs, ep.ID)
	}

	return result, nil
}
```

**Step 4: Update grabRequest struct**

In `internal/api/v1/api.go` (or requests.go), add override fields:

```go
type grabRequest struct {
	ContentID   int64  `json:"content_id"`
	DownloadURL string `json:"download_url"`
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
	EpisodeID   *int64 `json:"episode_id,omitempty"` // Deprecated
	Season      *int   `json:"season,omitempty"`     // Override: season number
	Episodes    []int  `json:"episodes,omitempty"`   // Override: episode numbers
}
```

**Step 5: Run tests to verify they pass**

Run: `task test -- -run "TestGrab_Series|TestGrab_SeasonPack" -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/v1/api.go
git commit -m "feat(api): parse release name for episode detection in grab (#67)"
```

---

## Task 6: Update DownloadHandler to use EpisodeIDs

**Files:**
- Modify: `internal/handlers/download.go`

**Step 1: Write failing test**

Add to `internal/handlers/download_test.go`:

```go
func TestDownloadHandler_GrabWithEpisodeIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := testutil.NewTestDB(t)
	bus := events.NewBus(slog.Default())
	store := download.NewStore(db)
	lib := library.NewStore(db)
	mockClient := mocks.NewMockDownloader(ctrl)
	logger := slog.Default()

	// Create series with episodes
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, lib.AddContent(content))

	ep1 := &library.Episode{ContentID: content.ID, Season: 1, Episode: 5, Status: library.StatusWanted}
	ep2 := &library.Episode{ContentID: content.ID, Season: 1, Episode: 6, Status: library.StatusWanted}
	require.NoError(t, lib.AddEpisode(ep1))
	require.NoError(t, lib.AddEpisode(ep2))

	mockClient.EXPECT().Add(gomock.Any(), "http://test.com/nzb", "").Return("nzo_123", nil)

	handler := NewDownloadHandler(bus, store, lib, mockClient, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = handler.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Emit grab with multiple episodes
	season := 1
	err := bus.Publish(ctx, &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   content.ID,
		EpisodeIDs:  []int64{ep1.ID, ep2.ID},
		Season:      &season,
		DownloadURL: "http://test.com/nzb",
		ReleaseName: "Test.Show.S01E05E06.1080p",
		Indexer:     "test",
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify download created with episode IDs
	downloads, _, err := store.List(download.Filter{ContentID: &content.ID})
	require.NoError(t, err)
	require.Len(t, downloads, 1)

	dl := downloads[0]
	assert.Equal(t, season, *dl.Season)
	assert.ElementsMatch(t, []int64{ep1.ID, ep2.ID}, dl.EpisodeIDs)
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestDownloadHandler_GrabWithEpisodeIDs -v`
Expected: FAIL - Handler doesn't use EpisodeIDs

**Step 3: Update handleGrabRequested**

In `internal/handlers/download.go`, update `handleGrabRequested`:

```go
func (h *DownloadHandler) handleGrabRequested(ctx context.Context, e *events.GrabRequested) {
	h.Logger().Info("processing grab request",
		"content_id", e.ContentID,
		"release", e.ReleaseName,
		"indexer", e.Indexer,
		"episode_ids", e.EpisodeIDs,
		"season", e.Season,
		"is_complete_season", e.IsCompleteSeason)

	// Check for existing files before grabbing (duplicate prevention)
	// ... (existing quality check code stays the same) ...

	// Send to download client
	clientID, err := h.client.Add(ctx, e.DownloadURL, "")
	if err != nil {
		h.Logger().Error("failed to add download", "error", err)
		if pubErr := h.Bus().Publish(ctx, &events.DownloadFailed{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadFailed, events.EntityDownload, 0),
			DownloadID: 0,
			Reason:     err.Error(),
			Retryable:  true,
		}); pubErr != nil {
			h.Logger().Error("failed to publish DownloadFailed event", "error", pubErr)
		}
		return
	}

	// Create DB record
	dl := &download.Download{
		ContentID:        e.ContentID,
		Season:           e.Season,
		IsCompleteSeason: e.IsCompleteSeason,
		Client:           download.ClientSABnzbd,
		ClientID:         clientID,
		Status:           download.StatusQueued,
		ReleaseName:      e.ReleaseName,
		Indexer:          e.Indexer,
	}

	// Backward compat: set EpisodeID if provided
	if e.EpisodeID != nil {
		dl.EpisodeID = e.EpisodeID
	} else if len(e.EpisodeIDs) == 1 {
		dl.EpisodeID = &e.EpisodeIDs[0]
	}

	if err := h.store.Add(dl); err != nil {
		h.Logger().Error("failed to save download", "error", err)
		return
	}

	// Set episode IDs in junction table
	if len(e.EpisodeIDs) > 0 {
		if err := h.store.SetEpisodeIDs(dl.ID, e.EpisodeIDs); err != nil {
			h.Logger().Error("failed to set episode IDs", "error", err)
			// Continue - download is created, just missing episode links
		}
	}

	// Emit success event
	if err := h.Bus().Publish(ctx, &events.DownloadCreated{
		BaseEvent:        events.NewBaseEvent(events.EventDownloadCreated, events.EntityDownload, dl.ID),
		DownloadID:       dl.ID,
		ContentID:        e.ContentID,
		EpisodeID:        dl.EpisodeID,
		EpisodeIDs:       e.EpisodeIDs,
		Season:           e.Season,
		IsCompleteSeason: e.IsCompleteSeason,
		ClientID:         clientID,
		ReleaseName:      e.ReleaseName,
	}); err != nil {
		h.Logger().Error("failed to publish DownloadCreated event", "error", err)
	}

	h.Logger().Info("download created",
		"download_id", dl.ID,
		"client_id", clientID,
		"episode_ids", e.EpisodeIDs)
}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestDownloadHandler_GrabWithEpisodeIDs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/handlers/download.go internal/handlers/download_test.go
git commit -m "feat(handlers): update DownloadHandler for multi-episode grabs (#67)"
```

---

## Task 7: Update ImportCompleted event for multi-episode results

**Files:**
- Modify: `internal/events/import.go`

**Step 1: Update ImportCompleted struct**

In `internal/events/import.go`:

```go
// EpisodeImportResult tracks the outcome of importing a single episode.
type EpisodeImportResult struct {
	EpisodeID int64  `json:"episode_id"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Success   bool   `json:"success"`
	FilePath  string `json:"file_path,omitempty"` // Empty if failed
	Error     string `json:"error,omitempty"`     // Empty if success
}

// ImportCompleted is emitted when import succeeds.
type ImportCompleted struct {
	BaseEvent
	DownloadID     int64                 `json:"download_id"`
	ContentID      int64                 `json:"content_id"`
	EpisodeID      *int64                `json:"episode_id,omitempty"`       // Deprecated: use EpisodeResults
	EpisodeResults []EpisodeImportResult `json:"episode_results,omitempty"`  // Per-episode outcomes
	FilePath       string                `json:"file_path,omitempty"`        // Deprecated: use EpisodeResults
	FileSize       int64                 `json:"file_size"`                  // Total size
}

// AllSucceeded returns true if all episode imports succeeded.
func (e *ImportCompleted) AllSucceeded() bool {
	for _, r := range e.EpisodeResults {
		if !r.Success {
			return false
		}
	}
	return true
}

// SuccessCount returns the number of successfully imported episodes.
func (e *ImportCompleted) SuccessCount() int {
	count := 0
	for _, r := range e.EpisodeResults {
		if r.Success {
			count++
		}
	}
	return count
}
```

**Step 2: Commit**

```bash
git add internal/events/import.go
git commit -m "feat(events): add EpisodeResults to ImportCompleted (#67)"
```

---

## Task 8: Add episode matcher utility

**Files:**
- Create: `internal/importer/episode_matcher.go`
- Create: `internal/importer/episode_matcher_test.go`

**Step 1: Write failing test**

Create `internal/importer/episode_matcher_test.go`:

```go
package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/library"
)

func TestMatchFileToEpisode(t *testing.T) {
	episodes := []*library.Episode{
		{ID: 1, Season: 1, Episode: 1},
		{ID: 2, Season: 1, Episode: 2},
		{ID: 3, Season: 1, Episode: 3},
	}

	tests := []struct {
		name     string
		filename string
		wantID   int64
		wantErr  bool
	}{
		{
			name:     "standard format",
			filename: "Show.S01E02.1080p.mkv",
			wantID:   2,
		},
		{
			name:     "lowercase",
			filename: "show.s01e01.720p.mkv",
			wantID:   1,
		},
		{
			name:     "no match",
			filename: "Show.S01E05.1080p.mkv",
			wantErr:  true,
		},
		{
			name:     "multi-episode returns first",
			filename: "Show.S01E01E02.1080p.mkv",
			wantID:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, err := MatchFileToEpisode(tt.filename, episodes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, ep.ID)
		})
	}
}

func TestMatchFilesToEpisodes(t *testing.T) {
	episodes := []*library.Episode{
		{ID: 1, Season: 1, Episode: 1},
		{ID: 2, Season: 1, Episode: 2},
		{ID: 3, Season: 1, Episode: 3},
	}

	files := []string{
		"/downloads/Show.S01/Show.S01E01.mkv",
		"/downloads/Show.S01/Show.S01E02.mkv",
		"/downloads/Show.S01/Show.S01E03.mkv",
		"/downloads/Show.S01/sample.mkv", // Should be skipped
	}

	matches, unmatched := MatchFilesToEpisodes(files, episodes)

	assert.Len(t, matches, 3)
	assert.Len(t, unmatched, 1)
	assert.Equal(t, "/downloads/Show.S01/sample.mkv", unmatched[0])

	// Verify correct matching
	for _, m := range matches {
		assert.NotNil(t, m.Episode)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestMatchFileToEpisode -v`
Expected: FAIL - MatchFileToEpisode doesn't exist

**Step 3: Implement episode matcher**

Create `internal/importer/episode_matcher.go`:

```go
package importer

import (
	"fmt"
	"path/filepath"

	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/pkg/release"
)

// FileMatch represents a matched file-to-episode pairing.
type FileMatch struct {
	FilePath string
	Episode  *library.Episode
}

// MatchFileToEpisode finds the episode that matches a filename.
// Returns error if no match is found.
func MatchFileToEpisode(filename string, episodes []*library.Episode) (*library.Episode, error) {
	info := release.Parse(filepath.Base(filename))

	if info.Season == 0 || len(info.Episodes) == 0 {
		return nil, fmt.Errorf("cannot parse episode info from %s", filename)
	}

	// Match first episode in the file (for multi-episode files)
	targetEp := info.Episodes[0]

	for _, ep := range episodes {
		if ep.Season == info.Season && ep.Episode == targetEp {
			return ep, nil
		}
	}

	return nil, fmt.Errorf("no matching episode for S%02dE%02d in %s", info.Season, targetEp, filename)
}

// MatchFilesToEpisodes matches multiple files to episodes.
// Returns matched pairs and a list of unmatched files.
func MatchFilesToEpisodes(files []string, episodes []*library.Episode) ([]FileMatch, []string) {
	var matches []FileMatch
	var unmatched []string

	for _, f := range files {
		ep, err := MatchFileToEpisode(f, episodes)
		if err != nil {
			unmatched = append(unmatched, f)
			continue
		}
		matches = append(matches, FileMatch{FilePath: f, Episode: ep})
	}

	return matches, unmatched
}

// MatchFileToSeason parses a file and returns episode info for season pack handling.
// Unlike MatchFileToEpisode, this doesn't require pre-existing episode records.
// Returns (season, episodeNumber, error).
func MatchFileToSeason(filename string) (int, int, error) {
	info := release.Parse(filepath.Base(filename))

	if info.Season == 0 {
		return 0, 0, fmt.Errorf("cannot parse season from %s", filename)
	}
	if len(info.Episodes) == 0 {
		return 0, 0, fmt.Errorf("cannot parse episode number from %s", filename)
	}

	return info.Season, info.Episodes[0], nil
}
```

**Step 4: Run tests to verify they pass**

Run: `task test -- -run "TestMatchFileToEpisode|TestMatchFilesToEpisodes" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/episode_matcher.go internal/importer/episode_matcher_test.go
git commit -m "feat(importer): add episode matcher utility (#67)"
```

---

## Task 9: Update ImportHandler for multi-file season pack imports

**Files:**
- Modify: `internal/handlers/import.go`
- Modify: `internal/importer/importer.go`

**Step 1: Write failing integration test**

Add to `internal/handlers/import_test.go`:

```go
func TestImportHandler_SeasonPack(t *testing.T) {
	db := testutil.NewTestDB(t)
	bus := events.NewBus(slog.Default())
	dlStore := download.NewStore(db)
	lib := library.NewStore(db)
	logger := slog.Default()

	// Create series
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Show",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, lib.AddContent(content))

	// Create download (season pack, no episodes yet)
	season := 1
	dl := &download.Download{
		ContentID:        content.ID,
		Client:           download.ClientSABnzbd,
		ClientID:         "nzo_seasonpack",
		Status:           download.StatusCompleted,
		ReleaseName:      "Test.Show.S01.1080p",
		Indexer:          "test",
		Season:           &season,
		IsCompleteSeason: true,
	}
	require.NoError(t, dlStore.Add(dl))

	// Create temp directory with season pack files
	tmpDir := t.TempDir()
	seasonDir := filepath.Join(tmpDir, "Test.Show.S01.1080p")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	// Create fake episode files
	for i := 1; i <= 3; i++ {
		filename := fmt.Sprintf("Test.Show.S01E%02d.1080p.mkv", i)
		require.NoError(t, os.WriteFile(filepath.Join(seasonDir, filename), []byte("video data"), 0644))
	}

	// Create mock importer that tracks calls
	mockImporter := &mockMultiFileImporter{
		results: make(map[int64]*importer.ImportResult),
	}

	handler := NewImportHandler(bus, dlStore, lib, mockImporter, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to ImportCompleted
	completedCh := bus.Subscribe(events.EventImportCompleted, 10)

	go func() { _ = handler.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Emit download completed
	err := bus.Publish(ctx, &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: seasonDir,
	})
	require.NoError(t, err)

	// Wait for import completed
	select {
	case e := <-completedCh:
		completed := e.(*events.ImportCompleted)
		assert.Equal(t, dl.ID, completed.DownloadID)
		assert.Len(t, completed.EpisodeResults, 3)
		assert.True(t, completed.AllSucceeded())
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ImportCompleted")
	}

	// Verify episodes were created
	eps, _, err := lib.ListEpisodes(library.EpisodeFilter{ContentID: &content.ID})
	require.NoError(t, err)
	assert.Len(t, eps, 3)
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestImportHandler_SeasonPack -v`
Expected: FAIL - Handler doesn't handle season packs

**Step 3: Add ImportSeasonPack to importer**

In `internal/importer/importer.go`, add a new method:

```go
// ImportSeasonPack processes a season pack download with multiple files.
// It finds or creates episodes for each video file and imports them.
func (i *Importer) ImportSeasonPack(ctx context.Context, downloadID int64, downloadPath string) (*SeasonPackResult, error) {
	i.log.Info("season pack import started", "download_id", downloadID, "path", downloadPath)

	// Get download record
	dl, err := i.downloads.Get(downloadID)
	if err != nil {
		return nil, fmt.Errorf("get download: %w", err)
	}

	if dl.Season == nil {
		return nil, fmt.Errorf("download %d is not a season pack (no season set)", downloadID)
	}

	// Get content record
	content, err := i.library.GetContent(dl.ContentID)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}

	// Find all video files
	videoFiles, err := FindAllVideos(downloadPath)
	if err != nil {
		return nil, err
	}

	if len(videoFiles) == 0 {
		return nil, ErrNoVideoFiles
	}

	i.log.Info("found video files for season pack", "count", len(videoFiles))

	result := &SeasonPackResult{
		DownloadID: downloadID,
		ContentID:  dl.ContentID,
	}

	// Process each file
	for _, videoPath := range videoFiles {
		epResult := i.importSeasonPackFile(ctx, dl, content, videoPath)
		result.Episodes = append(result.Episodes, epResult)
		result.TotalSize += epResult.SizeBytes
	}

	return result, nil
}

func (i *Importer) importSeasonPackFile(ctx context.Context, dl *download.Download, content *library.Content, videoPath string) EpisodeImportResult {
	result := EpisodeImportResult{
		SourcePath: videoPath,
	}

	// Parse episode info from filename
	season, epNum, err := MatchFileToSeason(videoPath)
	if err != nil {
		result.Error = err.Error()
		i.log.Warn("skipping unrecognized file", "path", videoPath, "error", err)
		return result
	}

	result.Season = season
	result.Episode = epNum

	// Find or create episode
	episode, _, err := i.library.FindOrCreateEpisode(dl.ContentID, season, epNum)
	if err != nil {
		result.Error = fmt.Sprintf("create episode: %v", err)
		return result
	}
	result.EpisodeID = episode.ID

	// Build destination path
	quality := extractQuality(dl.ReleaseName)
	ext := strings.TrimPrefix(filepath.Ext(videoPath), ".")
	relPath := i.renamer.EpisodePath(content.Title, season, epNum, quality, ext)
	destPath := filepath.Join(i.seriesRoot, relPath)

	// Validate path
	if err := ValidatePath(destPath, i.seriesRoot); err != nil {
		result.Error = err.Error()
		return result
	}

	// Copy file
	size, err := CopyFile(videoPath, destPath)
	if err != nil {
		result.Error = fmt.Sprintf("copy file: %v", err)
		return result
	}

	result.DestPath = destPath
	result.SizeBytes = size

	// Update database
	tx, err := i.library.Begin()
	if err != nil {
		result.Error = fmt.Sprintf("begin transaction: %v", err)
		return result
	}
	defer func() { _ = tx.Rollback() }()

	file := &library.File{
		ContentID: content.ID,
		EpisodeID: &episode.ID,
		Path:      destPath,
		SizeBytes: size,
		Quality:   quality,
		Source:    dl.Indexer,
	}
	if err := tx.AddFile(file); err != nil {
		result.Error = fmt.Sprintf("add file: %v", err)
		return result
	}

	episode.Status = library.StatusAvailable
	if err := tx.UpdateEpisode(episode); err != nil {
		result.Error = fmt.Sprintf("update episode: %v", err)
		return result
	}

	if err := tx.Commit(); err != nil {
		result.Error = fmt.Sprintf("commit: %v", err)
		return result
	}

	result.Success = true
	result.FileID = file.ID

	i.log.Info("imported episode", "episode_id", episode.ID, "season", season, "episode", epNum, "dest", destPath)
	return result
}

// SeasonPackResult is the result of importing a season pack.
type SeasonPackResult struct {
	DownloadID int64
	ContentID  int64
	Episodes   []EpisodeImportResult
	TotalSize  int64
}

// EpisodeImportResult tracks the outcome of importing a single episode file.
type EpisodeImportResult struct {
	EpisodeID  int64
	Season     int
	Episode    int
	Success    bool
	SourcePath string
	DestPath   string
	SizeBytes  int64
	FileID     int64
	Error      string
}

// AllSucceeded returns true if all episodes imported successfully.
func (r *SeasonPackResult) AllSucceeded() bool {
	for _, ep := range r.Episodes {
		if !ep.Success {
			return false
		}
	}
	return true
}
```

**Step 4: Add FindAllVideos helper**

In `internal/importer/files.go`:

```go
// FindAllVideos finds all video files in a directory (recursive).
func FindAllVideos(root string) ([]string, error) {
	var videos []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if isVideoFile(path) {
			videos = append(videos, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return videos, nil
}
```

**Step 5: Update ImportHandler to detect season packs**

In `internal/handlers/import.go`, update `handleDownloadCompleted`:

```go
func (h *ImportHandler) handleDownloadCompleted(ctx context.Context, e *events.DownloadCompleted) {
	// ... existing lock acquisition ...

	dl, err := h.store.Get(e.DownloadID)
	if err != nil {
		// ... existing error handling ...
	}

	// ... existing quality check ...

	// Transition to importing
	if err := h.store.Transition(dl, download.StatusImporting); err != nil {
		// ... existing error handling ...
	}

	// Emit ImportStarted
	// ... existing code ...

	// Determine import type
	if dl.IsCompleteSeason {
		h.handleSeasonPackImport(ctx, dl, e.SourcePath)
	} else {
		h.handleSingleFileImport(ctx, dl, e.SourcePath)
	}
}

func (h *ImportHandler) handleSeasonPackImport(ctx context.Context, dl *download.Download, sourcePath string) {
	// Cast importer to check for season pack support
	seasonImporter, ok := h.importer.(interface {
		ImportSeasonPack(ctx context.Context, downloadID int64, downloadPath string) (*importer.SeasonPackResult, error)
	})
	if !ok {
		h.Logger().Error("importer does not support season packs")
		h.publishImportFailed(ctx, dl.ID, "importer does not support season packs")
		return
	}

	result, err := seasonImporter.ImportSeasonPack(ctx, dl.ID, sourcePath)
	if err != nil {
		h.Logger().Error("season pack import failed", "download_id", dl.ID, "error", err)
		h.publishImportFailed(ctx, dl.ID, err.Error())
		return
	}

	// Transition to imported
	if err := h.store.Transition(dl, download.StatusImported); err != nil {
		h.Logger().Error("failed to transition to imported", "download_id", dl.ID, "error", err)
	}

	// Convert results to event format
	var episodeResults []events.EpisodeImportResult
	for _, r := range result.Episodes {
		episodeResults = append(episodeResults, events.EpisodeImportResult{
			EpisodeID: r.EpisodeID,
			Season:    r.Season,
			Episode:   r.Episode,
			Success:   r.Success,
			FilePath:  r.DestPath,
			Error:     r.Error,
		})
	}

	// Emit ImportCompleted
	if err := h.Bus().Publish(ctx, &events.ImportCompleted{
		BaseEvent:      events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID:     dl.ID,
		ContentID:      dl.ContentID,
		EpisodeResults: episodeResults,
		FileSize:       result.TotalSize,
	}); err != nil {
		h.Logger().Error("failed to publish ImportCompleted event", "error", err)
	}

	h.Logger().Info("season pack import completed",
		"download_id", dl.ID,
		"content_id", dl.ContentID,
		"episodes", len(result.Episodes),
		"total_size", result.TotalSize)
}

func (h *ImportHandler) handleSingleFileImport(ctx context.Context, dl *download.Download, sourcePath string) {
	// Existing single-file import logic (refactored from handleDownloadCompleted)
	result, err := h.importer.Import(ctx, dl.ID, sourcePath)
	if err != nil {
		h.Logger().Error("import failed", "download_id", dl.ID, "error", err)
		h.publishImportFailed(ctx, dl.ID, err.Error())
		return
	}

	// Transition to imported
	if err := h.store.Transition(dl, download.StatusImported); err != nil {
		h.Logger().Error("failed to transition to imported", "download_id", dl.ID, "error", err)
	}

	// Build episode results for single file
	var episodeResults []events.EpisodeImportResult
	if dl.EpisodeID != nil {
		ep, _ := h.library.GetEpisode(*dl.EpisodeID)
		if ep != nil {
			episodeResults = append(episodeResults, events.EpisodeImportResult{
				EpisodeID: ep.ID,
				Season:    ep.Season,
				Episode:   ep.Episode,
				Success:   true,
				FilePath:  result.DestPath,
			})
		}
	}

	// Emit ImportCompleted
	if err := h.Bus().Publish(ctx, &events.ImportCompleted{
		BaseEvent:      events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID:     dl.ID,
		ContentID:      dl.ContentID,
		EpisodeID:      dl.EpisodeID,
		EpisodeResults: episodeResults,
		FilePath:       result.DestPath,
		FileSize:       result.SizeBytes,
	}); err != nil {
		h.Logger().Error("failed to publish ImportCompleted event", "error", err)
	}

	h.Logger().Info("import completed",
		"download_id", dl.ID,
		"content_id", dl.ContentID,
		"dest", result.DestPath,
		"size_bytes", result.SizeBytes)
}
```

**Step 6: Run tests to verify they pass**

Run: `task test -- -run TestImportHandler_SeasonPack -v`
Expected: PASS

**Step 7: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/handlers/import.go internal/importer/importer.go internal/importer/files.go
git commit -m "feat(import): add season pack import support (#67)"
```

---

## Task 10: Integration test with Peaky Blinders test case

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Write integration test**

Add to `internal/api/v1/integration_test.go`:

```go
func TestIntegration_SeasonPackGrabAndImport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupIntegrationEnv(t)
	defer env.cleanup()

	// Create series
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Peaky Blinders",
		Year:           2013,
		TVDBID:         270915,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, env.library.AddContent(content))

	// Grab season pack
	grabBody := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://test.com/peaky.s01.nzb",
		"title": "Peaky.Blinders.S01.1080p.BluRay.x264",
		"indexer": "TestIndexer"
	}`, content.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(grabBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	// Wait for download to be created
	time.Sleep(100 * time.Millisecond)

	// Verify download was created with season pack flags
	downloads, _, err := env.downloads.List(download.Filter{ContentID: &content.ID})
	require.NoError(t, err)
	require.Len(t, downloads, 1)

	dl := downloads[0]
	assert.True(t, dl.IsCompleteSeason)
	assert.Equal(t, 1, *dl.Season)
	assert.Empty(t, dl.EpisodeIDs) // No episodes until import

	// Simulate download completion with season pack files
	tmpDir := t.TempDir()
	seasonDir := filepath.Join(tmpDir, "Peaky.Blinders.S01.1080p.BluRay.x264")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	for i := 1; i <= 6; i++ {
		filename := fmt.Sprintf("Peaky.Blinders.S01E%02d.1080p.BluRay.x264.mkv", i)
		require.NoError(t, os.WriteFile(filepath.Join(seasonDir, filename), []byte("video"), 0644))
	}

	// Emit download completed
	err = env.bus.Publish(context.Background(), &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: seasonDir,
	})
	require.NoError(t, err)

	// Wait for import
	time.Sleep(500 * time.Millisecond)

	// Verify episodes were created and marked available
	eps, _, err := env.library.ListEpisodes(library.EpisodeFilter{ContentID: &content.ID})
	require.NoError(t, err)
	assert.Len(t, eps, 6)

	for _, ep := range eps {
		assert.Equal(t, library.StatusAvailable, ep.Status)
	}

	// Verify download status
	dl, err = env.downloads.Get(dl.ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusImported, dl.Status)
}
```

**Step 2: Run integration test**

Run: `task test:integration -- -run TestIntegration_SeasonPackGrabAndImport -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add integration test for season pack flow (#67)"
```

---

## Task 11: Final verification and cleanup

**Step 1: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 2: Run linter**

Run: `task lint`
Expected: No issues

**Step 3: Update migration index**

If your project tracks migration versions, update accordingly.

**Step 4: Manual test with test case #28 (Peaky Blinders S01)**

```bash
# Start server
task dev

# Check download #28
./arrgo downloads show 28

# If still stuck, retry it
./arrgo downloads retry 28

# Watch logs for season pack handling
tail -f /tmp/arrgod.log
```

**Step 5: Close issue #67**

```bash
gh issue close 67 --comment "Implemented season pack and multi-episode support:

- Added download_episodes junction table
- Parse release names at grab time to detect episodes
- Season packs import all video files and create episodes on-demand
- Multi-episode releases (S01E05E06E07) link to multiple episodes

Tested with Peaky Blinders S01 (test case #28)."
```

**Step 6: Final commit (if any cleanup)**

```bash
git add -A
git commit -m "chore: final cleanup for series episode handling (#67)"
```
