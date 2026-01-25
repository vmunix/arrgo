# API Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Standardize pagination, remove duplicate endpoints, fix response codes, and optimize dashboard queries.

**Architecture:** All changes are to the v1 API layer. No changes to business logic. We add limit/offset to filters, add CountByStatus to download store, and fix response patterns.

**Tech Stack:** Go, net/http, SQLite

---

## Task 1: Add pagination to downloads endpoint

Add `limit` and `offset` query params to `GET /api/v1/downloads`, return consistent response with `{items, total, limit, offset}`.

**Files:**
- Modify: `internal/download/download.go` - Add Limit/Offset to Filter, update List() to support pagination and return total
- Modify: `internal/api/v1/types.go` - Update listDownloadsResponse with Limit/Offset fields
- Modify: `internal/api/v1/api.go` - Parse limit/offset params in listDownloads handler
- Test: `internal/download/download_test.go`
- Test: `internal/api/v1/api_test.go`

**Step 1: Write failing test for store pagination**

Add to `internal/download/download_test.go`:
```go
func TestStore_List_Pagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create 5 downloads
	for i := 0; i < 5; i++ {
		d := &Download{
			ContentID:   1,
			Client:      ClientSABnzbd,
			ClientID:    fmt.Sprintf("nzo_%d", i),
			Status:      StatusQueued,
			ReleaseName: fmt.Sprintf("Release.%d", i),
			Indexer:     "test",
		}
		require.NoError(t, store.Add(d))
	}

	// Test limit
	downloads, total, err := store.List(Filter{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, downloads, 2)

	// Test offset
	downloads, total, err = store.List(Filter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, downloads, 2)

	// Test offset beyond results
	downloads, total, err = store.List(Filter{Limit: 10, Offset: 10})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Empty(t, downloads)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download -run TestStore_List_Pagination -v`
Expected: FAIL (List returns 2 values, not 3)

**Step 3: Update Filter and List() in store**

In `internal/download/download.go`, update Filter struct:
```go
type Filter struct {
	ContentID *int64
	EpisodeID *int64
	Status    *Status
	Client    *Client
	Active    bool // If true, exclude terminal states (cleaned, failed)
	Limit     int
	Offset    int
}
```

Update List() signature and implementation:
```go
// List returns downloads matching the specified filter.
// Returns (downloads, total count, error).
func (s *Store) List(f Filter) ([]*Download, int, error) {
	// Build WHERE clause (existing code)
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 6)

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}
	if f.Client != nil {
		conditions = append(conditions, "client = ?")
		args = append(args, *f.Client)
	}
	if f.Active {
		conditions = append(conditions, "status NOT IN (?, ?)")
		args = append(args, StatusCleaned, StatusFailed)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count first
	countQuery := "SELECT COUNT(*) FROM downloads " + whereClause
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count downloads: %w", err)
	}

	// Build main query with pagination
	query := "SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at FROM downloads " +
		whereClause + " ORDER BY id"

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
		if f.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", f.Offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list downloads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*Download
	for rows.Next() {
		d := &Download{}
		if err := rows.Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt); err != nil {
			return nil, 0, fmt.Errorf("scan download: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate downloads: %w", err)
	}

	return results, total, nil
}
```

**Step 4: Fix all callers of List()**

Update all callers to handle the new (downloads, total, error) return signature. Search for `List(` in files that use the download store.

**Step 5: Update API types**

In `internal/api/v1/types.go`, update:
```go
type listDownloadsResponse struct {
	Items  []downloadResponse `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}
```

**Step 6: Update API handler**

In `internal/api/v1/api.go`, update listDownloads:
```go
func (s *Server) listDownloads(w http.ResponseWriter, r *http.Request) {
	filter := download.Filter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if activeStr := r.URL.Query().Get("active"); activeStr == queryTrue {
		filter.Active = true
	}
	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		st := download.Status(statusStr)
		filter.Status = &st
	}

	downloads, total, err := s.deps.Downloads.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Build live status map (existing code)
	liveStatus := make(map[int64]*download.ClientStatus)
	if s.deps.Manager != nil {
		active, err := s.deps.Manager.GetActive(r.Context())
		if err == nil {
			for _, a := range active {
				if a.Live != nil {
					liveStatus[a.Download.ID] = a.Live
				}
			}
		}
	}

	resp := listDownloadsResponse{
		Items:  make([]downloadResponse, len(downloads)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, d := range downloads {
		resp.Items[i] = downloadToResponse(d, liveStatus[d.ID])
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 7: Run all tests**

Run: `go test ./internal/download/... ./internal/api/v1/... -v`
Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/download/download.go internal/api/v1/api.go internal/api/v1/types.go internal/download/download_test.go
git commit -m "feat(api): add pagination to downloads endpoint

- Add Limit/Offset to download.Filter
- Update List() to return (items, total, error)
- Update listDownloads handler to parse limit/offset params
- Response now includes limit/offset fields for consistency"
```

---

## Task 2: Add pagination to history endpoint

Add `offset` query param to `GET /api/v1/history`, return consistent response with `{items, total, limit, offset}`.

**Files:**
- Modify: `internal/importer/history.go` - Add Offset to HistoryFilter, update List() to return total
- Modify: `internal/api/v1/types.go` - Update listHistoryResponse with Limit/Offset fields
- Modify: `internal/api/v1/api.go` - Parse offset param, include limit/offset in response
- Test: `internal/api/v1/api_test.go`

**Step 1: Update HistoryFilter and List()**

In `internal/importer/history.go`:
```go
type HistoryFilter struct {
	ContentID *int64
	EpisodeID *int64
	Event     *string
	Limit     int
	Offset    int
}
```

Update List() to return total:
```go
// List returns history entries matching the filter.
// Returns (entries, total count, error).
func (s *HistoryStore) List(f HistoryFilter) ([]*HistoryEntry, int, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Event != nil {
		conditions = append(conditions, "event = ?")
		args = append(args, *f.Event)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM history " + whereClause
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count history: %w", err)
	}

	query := `SELECT id, content_id, episode_id, event, data, created_at ` +
		`FROM history ` + whereClause + ` ORDER BY created_at DESC`

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
		if f.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", f.Offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*HistoryEntry
	for rows.Next() {
		h := &HistoryEntry{}
		if err := rows.Scan(&h.ID, &h.ContentID, &h.EpisodeID, &h.Event, &h.Data, &h.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan history: %w", err)
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate history: %w", err)
	}

	return results, total, nil
}
```

**Step 2: Update API types**

In `internal/api/v1/types.go`:
```go
type listHistoryResponse struct {
	Items  []historyResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}
```

**Step 3: Update API handler**

In `internal/api/v1/api.go`, update listHistory:
```go
func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	filter := importer.HistoryFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}

	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	entries, total, err := s.deps.History.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listHistoryResponse{
		Items:  make([]historyResponse, len(entries)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, h := range entries {
		resp.Items[i] = historyResponse{
			ID:        h.ID,
			ContentID: h.ContentID,
			EpisodeID: h.EpisodeID,
			Event:     h.Event,
			Data:      h.Data,
			CreatedAt: h.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Run tests and commit**

Run: `go test ./internal/importer/... ./internal/api/v1/... -v`

```bash
git add internal/importer/history.go internal/api/v1/api.go internal/api/v1/types.go
git commit -m "feat(api): add pagination to history endpoint

- Add Offset to HistoryFilter
- Update List() to return (items, total, error)
- Response now includes limit/offset fields"
```

---

## Task 3: Add pagination to files endpoint

Add `limit` and `offset` query params to `GET /api/v1/files`.

**Files:**
- Modify: `internal/library/files.go` - Verify FileFilter has Limit/Offset, update ListFiles to use them
- Modify: `internal/api/v1/types.go` - Update listFilesResponse with Limit/Offset fields
- Modify: `internal/api/v1/api.go` - Parse limit/offset params in listFiles handler

**Step 1: Check FileFilter**

FileFilter already has Limit/Offset fields. Verify ListFiles uses them and returns total.

**Step 2: Update API types**

In `internal/api/v1/types.go`:
```go
type listFilesResponse struct {
	Items  []fileResponse `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}
```

**Step 3: Update API handler**

In `internal/api/v1/api.go`, update listFiles:
```go
func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	filter := library.FileFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	files, total, err := s.deps.Library.ListFiles(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listFilesResponse{
		Items:  make([]fileResponse, len(files)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, f := range files {
		resp.Items[i] = fileResponse{
			ID:        f.ID,
			ContentID: f.ContentID,
			EpisodeID: f.EpisodeID,
			Path:      f.Path,
			SizeBytes: f.SizeBytes,
			Quality:   f.Quality,
			Source:    f.Source,
			AddedAt:   f.AddedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Run tests and commit**

Run: `go test ./internal/api/v1/... -v`

```bash
git add internal/api/v1/api.go internal/api/v1/types.go
git commit -m "feat(api): add pagination to files endpoint

- Parse limit/offset query params
- Response now includes limit/offset fields"
```

---

## Task 4: Add pagination to events endpoint

Add `offset` query param to `GET /api/v1/events`.

**Files:**
- Modify: `internal/events/log.go` - Add RecentWithPagination method or update Recent
- Modify: `internal/api/v1/types.go` - Update listEventsResponse with Limit/Offset fields
- Modify: `internal/api/v1/api.go` - Parse limit/offset params in listEvents handler

**Step 1: Update EventLog**

In `internal/events/log.go`, update Recent to support offset:
```go
// Recent returns events with pagination, ordered by most recent first.
func (l *EventLog) Recent(limit, offset int) ([]RawEvent, int, error) {
	// Get total count
	var total int
	if err := l.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	rows, err := l.db.Query(`
		SELECT id, event_type, entity_type, entity_id, payload, occurred_at, created_at
		FROM events
		ORDER BY id DESC
		LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	return events, total, err
}
```

**Step 2: Update API types**

In `internal/api/v1/types.go`:
```go
type listEventsResponse struct {
	Items  []EventResponse `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}
```

**Step 3: Update API handler**

Find and update listEvents handler to use new Recent signature.

**Step 4: Run tests and commit**

```bash
git add internal/events/log.go internal/api/v1/api.go internal/api/v1/types.go
git commit -m "feat(api): add pagination to events endpoint

- Update Recent() to accept limit/offset and return total
- Response now includes limit/offset fields"
```

---

## Task 5: Remove duplicate /scan endpoint

Remove `POST /api/v1/scan`, keep only `POST /api/v1/plex/scan`.

**Files:**
- Modify: `internal/api/v1/api.go` - Remove route and handler
- Modify: `internal/api/v1/types.go` - Remove scanRequest type
- Test: Verify no tests depend on /scan

**Step 1: Remove route registration**

In `internal/api/v1/api.go`, delete line:
```go
mux.HandleFunc("POST /api/v1/scan", s.requirePlex(s.triggerScan))
```

**Step 2: Remove handler**

Delete the `triggerScan` function entirely.

**Step 3: Remove type**

In `internal/api/v1/types.go`, delete:
```go
// scanRequest is the request body for POST /scan.
type scanRequest struct {
	Path string `json:"path,omitempty"`
}
```

**Step 4: Run tests and commit**

Run: `go test ./internal/api/v1/... -v`

```bash
git add internal/api/v1/api.go internal/api/v1/types.go
git commit -m "refactor(api): remove duplicate /scan endpoint

Keep only /plex/scan, which has the same functionality."
```

---

## Task 6: Fix /plex/status response codes

Return 503 when Plex is unavailable instead of 200 with error field.

**Files:**
- Modify: `internal/api/v1/api.go` - Update getPlexStatus handler
- Test: `internal/api/v1/api_test.go`

**Step 1: Write failing test**

Add to `internal/api/v1/api_test.go`:
```go
func TestGetPlexStatus_Unavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	mockPlex.EXPECT().GetIdentity(gomock.Any()).Return(nil, errors.New("connection refused"))

	deps := ServerDeps{
		Library:   &mockLibraryStore{},
		Downloads: &mockDownloadStore{},
		History:   &mockHistoryStore{},
		Plex:      mockPlex,
	}
	server, _ := NewWithDeps(deps, Config{})
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp plexStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Connected)
	assert.Contains(t, resp.Error, "connection")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1 -run TestGetPlexStatus_Unavailable -v`
Expected: FAIL (returns 200, not 503)

**Step 3: Update handler**

In `internal/api/v1/api.go`, update getPlexStatus:
```go
func (s *Server) getPlexStatus(w http.ResponseWriter, r *http.Request) {
	resp := plexStatusResponse{}

	if s.deps.Plex == nil {
		resp.Error = "Plex not configured"
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}

	ctx := r.Context()

	// Get identity
	identity, err := s.deps.Plex.GetIdentity(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("connection failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}

	resp.Connected = true
	resp.ServerName = identity.Name
	resp.Version = identity.Version

	// Get sections
	sections, err := s.deps.Plex.GetSections(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("failed to get libraries: %v", err)
		writeJSON(w, http.StatusOK, resp) // Connected but partial failure
		return
	}

	resp.Libraries = make([]plexLibrary, len(sections))
	for i, sec := range sections {
		location := ""
		if len(sec.Locations) > 0 {
			location = sec.Locations[0].Path
		}

		count, _ := s.deps.Plex.GetLibraryCount(ctx, sec.Key)

		resp.Libraries[i] = plexLibrary{
			Key:        sec.Key,
			Title:      sec.Title,
			Type:       sec.Type,
			ItemCount:  count,
			Location:   location,
			ScannedAt:  sec.ScannedAt,
			Refreshing: sec.Refreshing(),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Run tests and commit**

Run: `go test ./internal/api/v1/... -v`

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go
git commit -m "fix(api): return 503 when Plex unavailable

/plex/status now returns 503 Service Unavailable when:
- Plex is not configured
- Connection to Plex fails

Returns 200 when connected, even if fetching libraries fails."
```

---

## Task 7: Remove unused new_download_id from retry response

Remove the unused field from retryResponse.

**Files:**
- Modify: `internal/api/v1/types.go` - Remove field from retryResponse

**Step 1: Remove field**

In `internal/api/v1/types.go`, update:
```go
// retryResponse is the response for POST /downloads/{id}/retry.
type retryResponse struct {
	ReleaseName string `json:"release_name"`
	Message     string `json:"message"`
}
```

**Step 2: Run tests and commit**

Run: `go test ./internal/api/v1/... -v`

```bash
git add internal/api/v1/types.go
git commit -m "refactor(api): remove unused new_download_id from retry response

Field was never populated. Retry uses event-driven flow where
download ID is assigned asynchronously."
```

---

## Task 8: Add CountByStatus to download store

Replace 7 separate queries in dashboard with a single GROUP BY query.

**Files:**
- Modify: `internal/download/download.go` - Add CountByStatus method
- Modify: `internal/api/v1/api.go` - Use CountByStatus in getDashboard
- Modify: `internal/api/v1/deps.go` - Add CountByStatus to interface if needed
- Test: `internal/download/download_test.go`

**Step 1: Write failing test**

Add to `internal/download/download_test.go`:
```go
func TestStore_CountByStatus(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create downloads with different statuses
	statuses := []Status{StatusQueued, StatusQueued, StatusDownloading, StatusCompleted, StatusFailed}
	for i, status := range statuses {
		d := &Download{
			ContentID:   1,
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
```

**Step 2: Implement CountByStatus**

In `internal/download/download.go`:
```go
// CountByStatus returns a map of status to count for all downloads.
func (s *Store) CountByStatus() (map[Status]int, error) {
	rows, err := s.db.Query(`
		SELECT status, COUNT(*) as count
		FROM downloads
		GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[Status]int)
	for rows.Next() {
		var status Status
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		counts[status] = count
	}

	return counts, rows.Err()
}
```

**Step 3: Update dashboard handler**

In `internal/api/v1/api.go`, update getDashboard:
```go
func (s *Server) getDashboard(w http.ResponseWriter, _ *http.Request) {
	resp := DashboardResponse{
		Version: "0.1.0",
	}

	// Connection status
	resp.Connections.Server = true
	resp.Connections.Plex = s.deps.Plex != nil
	resp.Connections.SABnzbd = s.deps.Manager != nil

	// Download counts by status (single query)
	counts, err := s.deps.Downloads.CountByStatus()
	if err == nil {
		resp.Downloads.Queued = counts[download.StatusQueued]
		resp.Downloads.Downloading = counts[download.StatusDownloading]
		resp.Downloads.Completed = counts[download.StatusCompleted]
		resp.Downloads.Importing = counts[download.StatusImporting]
		resp.Downloads.Imported = counts[download.StatusImported]
		resp.Downloads.Cleaned = counts[download.StatusCleaned]
		resp.Downloads.Failed = counts[download.StatusFailed]
	}

	// Stuck count (existing code)
	resp.Stuck.Threshold = 60
	thresholds := map[download.Status]time.Duration{
		download.StatusQueued:      time.Hour,
		download.StatusDownloading: time.Hour,
		download.StatusCompleted:   time.Hour,
		download.StatusImporting:   time.Hour,
	}
	stuck, _ := s.deps.Downloads.ListStuck(thresholds)
	resp.Stuck.Count = len(stuck)

	// Library counts
	movieType := library.ContentTypeMovie
	seriesType := library.ContentTypeSeries
	movies, _, _ := s.deps.Library.ListContent(library.ContentFilter{Type: &movieType})
	series, _, _ := s.deps.Library.ListContent(library.ContentFilter{Type: &seriesType})
	resp.Library.Movies = len(movies)
	resp.Library.Series = len(series)

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Update interface if needed**

Check if Downloads in deps.go needs CountByStatus added to its interface.

**Step 5: Run tests and commit**

Run: `go test ./internal/download/... ./internal/api/v1/... -v`

```bash
git add internal/download/download.go internal/download/download_test.go internal/api/v1/api.go internal/api/v1/deps.go
git commit -m "perf(api): optimize dashboard with CountByStatus

Replace 7 individual queries with single GROUP BY query.
Reduces dashboard query count from 10+ to ~4."
```

---

## Task 9: Document /library/check rationale

Add comment explaining why /library/check exists when /library doesn't.

**Files:**
- Modify: `internal/api/v1/api.go` - Add documentation comment

**Step 1: Add comment**

In `internal/api/v1/api.go`, above the route registration:
```go
// Library check - validates content records against actual files and Plex.
// Note: There is no /library resource. "Library" represents the validated state
// of content + files + Plex awareness, not a standalone entity. This endpoint
// performs cross-system health checks rather than CRUD operations.
mux.HandleFunc("GET /api/v1/library/check", s.checkLibrary)
```

**Step 2: Commit**

```bash
git add internal/api/v1/api.go
git commit -m "docs(api): document /library/check path rationale

Explains why /library/check exists without a /library resource:
it validates cross-system state, not a standalone entity."
```

---

## Task 10: Update GitHub issue

Update issue #64 to reflect completed work and defer grab tracking to v2.

**Step 1: Update issue**

```bash
gh issue edit 64 --body "$(cat <<'EOF'
## Summary

API design review identified several consistency issues and improvements to address before building more on the v1 API.

## Completed

- [x] **Standardize pagination on all list endpoints** - All list endpoints now support `?limit=N&offset=N` with response containing `{items, total, limit, offset}`
- [x] **Remove duplicate scan endpoint** - Removed `/scan`, kept `/plex/scan`
- [x] **Fix `/plex/status` response codes** - Returns 503 when Plex unavailable
- [x] **Change `POST /search` to `GET /search`** - Done in earlier commit
- [x] **Remove unused `new_download_id` from retry response**
- [x] **Add `CountByStatus()` to Downloads store** - Dashboard now uses single GROUP BY query
- [x] **Document `/library/check` path rationale**

## Deferred to v2 (TUI work)

- [ ] **Grab tracking** - Add correlation IDs or SSE for real-time updates when TUI is implemented

## Compat API Notes (Not Blocking)

- Tags are fake stubs (always returns ID=1)
- Language profiles stub returns only "English"
- Root folders lack type metadata (movies vs series)
- Dual `qualityprofile`/`qualityProfile` routes for Radarr compat
EOF
)"
```

**Step 2: Close issue if all v1 items done**

```bash
gh issue close 64 --comment "All v1 API cleanup items completed. Grab tracking deferred to v2 (TUI work)."
```
