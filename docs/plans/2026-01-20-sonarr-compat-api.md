# Sonarr Compat API Completion Plan

**Status:** Pending

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the Sonarr compatibility API so Overseerr can request TV series through arrgo. Based on analysis of Overseerr's `server/api/servarr/sonarr.ts` (MIT licensed).

**Architecture:** Extend `internal/api/compat/compat.go` with missing Sonarr endpoints. The key insight from Overseerr's code is that it calls `/series/lookup?term=tvdb:ID` before adding, and expects a rich response with seasons array.

**Tech Stack:** Go net/http, existing arrgo stores

---

## Task 1: Add Language Profile Endpoint

Overseerr requires language profiles for Sonarr v3+. We'll return a single default.

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Add route registration**

In `RegisterRoutes()`, add:
```go
mux.HandleFunc("GET /api/v3/languageprofile", s.authMiddleware(s.listLanguageProfiles))
```

**Step 2: Implement handler**

```go
func (s *Server) listLanguageProfiles(w http.ResponseWriter, r *http.Request) {
	// Return a single "English" profile - arrgo doesn't filter by language
	writeJSON(w, http.StatusOK, []map[string]any{
		{
			"id":   1,
			"name": "English",
		},
	})
}
```

**Step 3: Verify**

```bash
curl -H "X-Api-Key: test" http://localhost:8484/api/v3/languageprofile
```
Expected: `[{"id":1,"name":"English"}]`

**Step 4: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): add languageprofile endpoint for Sonarr"
```

---

## Task 2: Expand Sonarr Series Response Types

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Replace sonarrSeriesResponse with full structure**

```go
// sonarrSeason represents a season in Sonarr format.
type sonarrSeason struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
	Statistics   *struct {
		EpisodeFileCount   int `json:"episodeFileCount"`
		EpisodeCount       int `json:"episodeCount"`
		TotalEpisodeCount  int `json:"totalEpisodeCount"`
		SizeOnDisk         int `json:"sizeOnDisk"`
		PercentOfEpisodes  int `json:"percentOfEpisodes"`
	} `json:"statistics,omitempty"`
}

// sonarrSeriesResponse is the full Sonarr format for a series.
type sonarrSeriesResponse struct {
	ID                int64          `json:"id,omitempty"`
	TVDBID            int64          `json:"tvdbId"`
	Title             string         `json:"title"`
	SortTitle         string         `json:"sortTitle"`
	Year              int            `json:"year"`
	SeasonCount       int            `json:"seasonCount"`
	Seasons           []sonarrSeason `json:"seasons"`
	Status            string         `json:"status"`
	Overview          string         `json:"overview,omitempty"`
	Network           string         `json:"network,omitempty"`
	Runtime           int            `json:"runtime,omitempty"`
	Images            []struct {
		CoverType string `json:"coverType"`
		URL       string `json:"url"`
	} `json:"images,omitempty"`
	SeriesType        string `json:"seriesType"`
	Monitored         bool   `json:"monitored"`
	QualityProfileID  int    `json:"qualityProfileId"`
	LanguageProfileID int    `json:"languageProfileId"`
	SeasonFolder      bool   `json:"seasonFolder"`
	Path              string `json:"path,omitempty"`
	RootFolderPath    string `json:"rootFolderPath,omitempty"`
	TitleSlug         string `json:"titleSlug"`
	Certification     string `json:"certification,omitempty"`
	Genres            []string `json:"genres,omitempty"`
	Tags              []int  `json:"tags"`
	Added             string `json:"added,omitempty"`
	FirstAired        string `json:"firstAired,omitempty"`
	CleanTitle        string `json:"cleanTitle"`
	ImdbID            string `json:"imdbId,omitempty"`
	Statistics        *struct {
		SeasonCount       int `json:"seasonCount"`
		EpisodeFileCount  int `json:"episodeFileCount"`
		EpisodeCount      int `json:"episodeCount"`
		TotalEpisodeCount int `json:"totalEpisodeCount"`
		SizeOnDisk        int `json:"sizeOnDisk"`
		PercentOfEpisodes int `json:"percentOfEpisodes"`
	} `json:"statistics,omitempty"`
}

// sonarrAddRequest updated with all Overseerr fields.
type sonarrAddRequest struct {
	TVDBID            int64          `json:"tvdbId"`
	Title             string         `json:"title"`
	Year              int            `json:"year"`
	QualityProfileID  int            `json:"qualityProfileId"`
	LanguageProfileID int            `json:"languageProfileId"`
	Seasons           []int          `json:"seasons"` // Season numbers to monitor
	SeasonFolder      bool           `json:"seasonFolder"`
	RootFolderPath    string         `json:"rootFolderPath"`
	SeriesType        string         `json:"seriesType"` // standard, daily, anime
	Monitored         bool           `json:"monitored"`
	Tags              []int          `json:"tags"`
	AddOptions        struct {
		IgnoreEpisodesWithFiles    bool `json:"ignoreEpisodesWithFiles"`
		SearchForMissingEpisodes   bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}
```

**Step 2: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): expand Sonarr series types for Overseerr"
```

---

## Task 3: Add Series Lookup Endpoint

This is the critical endpoint Overseerr calls before adding a series.

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Add route**

In `RegisterRoutes()`, add:
```go
mux.HandleFunc("GET /api/v3/series/lookup", s.authMiddleware(s.lookupSeries))
```

**Step 2: Implement handler**

```go
func (s *Server) lookupSeries(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Parse tvdb:12345 format
	var tvdbID int64
	if _, err := fmt.Sscanf(term, "tvdb:%d", &tvdbID); err != nil {
		// Not a TVDB lookup - could be title search, return empty for now
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Check if we have this series in library
	contents, _, err := s.library.ListContent(library.ContentFilter{TVDBID: &tvdbID, Limit: 1})
	if err == nil && len(contents) > 0 {
		// Found in library - return with ID (signals "already exists")
		writeJSON(w, http.StatusOK, []sonarrSeriesResponse{s.contentToSonarrSeries(contents[0])})
		return
	}

	// Not in library - return stub that Overseerr can use to add
	// Overseerr will fill in metadata from TMDB/TVDB on its side
	response := sonarrSeriesResponse{
		TVDBID:            tvdbID,
		Title:             "",
		SortTitle:         "",
		Year:              0,
		SeasonCount:       1,
		Seasons:           []sonarrSeason{{SeasonNumber: 1, Monitored: false}},
		Status:            "continuing",
		SeriesType:        "standard",
		Monitored:         false,
		QualityProfileID:  1,
		LanguageProfileID: 1,
		SeasonFolder:      true,
		TitleSlug:         fmt.Sprintf("tvdb-%d", tvdbID),
		Tags:              []int{},
		CleanTitle:        "",
	}

	writeJSON(w, http.StatusOK, []sonarrSeriesResponse{response})
}
```

**Step 3: Add helper to convert library content to Sonarr format**

```go
// contentToSonarrSeries converts library content to Sonarr response format.
func (s *Server) contentToSonarrSeries(c *library.Content) sonarrSeriesResponse {
	var tvdbID int64
	if c.TVDBID != nil {
		tvdbID = *c.TVDBID
	}

	// Determine profile ID from name
	profileID := 1
	for name, id := range s.cfg.QualityProfiles {
		if name == c.QualityProfile {
			profileID = id
			break
		}
	}

	// Build path
	path := fmt.Sprintf("%s/%s", c.RootPath, c.Title)

	// Default to 1 season if we don't have episode data
	seasons := []sonarrSeason{{SeasonNumber: 1, Monitored: true}}

	return sonarrSeriesResponse{
		ID:                c.ID,
		TVDBID:            tvdbID,
		Title:             c.Title,
		SortTitle:         strings.ToLower(c.Title),
		Year:              c.Year,
		SeasonCount:       len(seasons),
		Seasons:           seasons,
		Status:            "continuing",
		SeriesType:        "standard",
		Monitored:         c.Status == library.StatusWanted,
		QualityProfileID:  profileID,
		LanguageProfileID: 1,
		SeasonFolder:      true,
		Path:              path,
		RootFolderPath:    c.RootPath,
		TitleSlug:         fmt.Sprintf("tvdb-%d", tvdbID),
		Tags:              []int{},
		Added:             c.AddedAt.Format("2006-01-02T15:04:05Z"),
		CleanTitle:        strings.ToLower(strings.ReplaceAll(c.Title, " ", "")),
	}
}
```

**Step 4: Add strings import if needed**

```go
import "strings"
```

**Step 5: Verify**

```bash
curl -H "X-Api-Key: test" "http://localhost:8484/api/v3/series/lookup?term=tvdb:12345"
```
Expected: Returns series stub with tvdbId: 12345

**Step 6: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): add series lookup endpoint for Overseerr"
```

---

## Task 4: Update listSeries and getSeries Handlers

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Implement listSeries properly**

```go
func (s *Server) listSeries(w http.ResponseWriter, r *http.Request) {
	contentType := library.ContentTypeSeries
	contents, _, err := s.library.ListContent(library.ContentFilter{Type: &contentType})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	series := make([]sonarrSeriesResponse, 0, len(contents))
	for _, c := range contents {
		series = append(series, s.contentToSonarrSeries(c))
	}
	writeJSON(w, http.StatusOK, series)
}
```

**Step 2: Implement getSeries properly**

```go
func (s *Server) getSeries(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	content, err := s.library.GetContent(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	if content.Type != library.ContentTypeSeries {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	writeJSON(w, http.StatusOK, s.contentToSonarrSeries(content))
}
```

**Step 3: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): implement listSeries and getSeries handlers"
```

---

## Task 5: Update addSeries to Handle Full Request

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Update addSeries handler**

```go
func (s *Server) addSeries(w http.ResponseWriter, r *http.Request) {
	var req sonarrAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Check if series already exists
	contents, _, err := s.library.ListContent(library.ContentFilter{TVDBID: &req.TVDBID, Limit: 1})
	if err == nil && len(contents) > 0 {
		existing := contents[0]
		// Update monitoring status if needed
		if req.Monitored && existing.Status != library.StatusWanted {
			existing.Status = library.StatusWanted
			if err := s.library.UpdateContent(existing); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, s.contentToSonarrSeries(existing))
		return
	}

	// Map quality profile ID to name
	profileName := "hd"
	for name, id := range s.cfg.QualityProfiles {
		if id == req.QualityProfileID {
			profileName = name
			break
		}
	}

	// Determine root path
	rootPath := req.RootFolderPath
	if rootPath == "" {
		rootPath = s.cfg.SeriesRoot
	}

	// Add to library
	tvdbID := req.TVDBID
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		TVDBID:         &tvdbID,
		Title:          req.Title,
		Year:           req.Year,
		Status:         library.StatusWanted,
		QualityProfile: profileName,
		RootPath:       rootPath,
	}

	if err := s.library.AddContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Auto-search if requested
	if req.AddOptions.SearchForMissingEpisodes && s.searcher != nil && s.manager != nil {
		go s.searchAndGrabSeries(content.ID, req.Title, req.Year, profileName)
	}

	writeJSON(w, http.StatusCreated, s.contentToSonarrSeries(content))
}
```

**Step 2: Add series search helper**

```go
// searchAndGrabSeries performs a background search for series (season pack or latest episode).
func (s *Server) searchAndGrabSeries(contentID int64, title string, year int, profile string) {
	ctx := context.Background()

	// Search for season 1 pack first
	query := search.Query{
		Text: fmt.Sprintf("%s S01", title),
		Type: "series",
	}

	result, err := s.searcher.Search(ctx, query, profile)
	if err != nil || len(result.Releases) == 0 {
		return
	}

	// Grab the best match
	best := result.Releases[0]
	_, _ = s.manager.Grab(ctx, contentID, nil, best.DownloadURL, best.Title, best.Indexer)
}
```

**Step 3: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): update addSeries to handle full Overseerr request"
```

---

## Task 6: Add PUT Handlers for Updates

Overseerr calls PUT to update existing movies/series (e.g., to enable monitoring).

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Add routes**

In `RegisterRoutes()`, add:
```go
mux.HandleFunc("PUT /api/v3/movie", s.authMiddleware(s.updateMovie))
mux.HandleFunc("PUT /api/v3/series", s.authMiddleware(s.updateSeries))
```

**Step 2: Implement updateMovie**

```go
func (s *Server) updateMovie(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        int64 `json:"id"`
		Monitored bool  `json:"monitored"`
		Tags      []int `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	content, err := s.library.GetContent(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Movie not found"})
		return
	}

	// Update monitoring status
	if req.Monitored {
		content.Status = library.StatusWanted
	}

	if err := s.library.UpdateContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, s.contentToRadarrMovie(content))
}
```

**Step 3: Implement updateSeries**

```go
func (s *Server) updateSeries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        int64          `json:"id"`
		Monitored bool           `json:"monitored"`
		Seasons   []sonarrSeason `json:"seasons"`
		Tags      []int          `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	content, err := s.library.GetContent(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	// Update monitoring status
	if req.Monitored {
		content.Status = library.StatusWanted
	}

	if err := s.library.UpdateContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, s.contentToSonarrSeries(content))
}
```

**Step 4: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): add PUT handlers for movie/series updates"
```

---

## Task 7: Add Integration Test

**Files:**
- Modify: `internal/api/compat/compat_test.go`

**Step 1: Add Sonarr endpoint tests**

```go
func TestSonarrLookup(t *testing.T) {
	srv, _, cleanup := setupCompatServer(t)
	defer cleanup()

	// Test lookup for non-existent series
	req := httptest.NewRequest("GET", "/api/v3/series/lookup?term=tvdb:12345", nil)
	req.Header.Set("X-Api-Key", "test")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &results)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, float64(12345), results[0]["tvdbId"])
}

func TestSonarrAddSeries(t *testing.T) {
	srv, _, cleanup := setupCompatServer(t)
	defer cleanup()

	body := `{
		"tvdbId": 12345,
		"title": "Test Show",
		"year": 2024,
		"qualityProfileId": 1,
		"languageProfileId": 1,
		"rootFolderPath": "/tv",
		"seriesType": "standard",
		"monitored": true,
		"seasons": [1],
		"addOptions": {"searchForMissingEpisodes": false}
	}`

	req := httptest.NewRequest("POST", "/api/v3/series", strings.NewReader(body))
	req.Header.Set("X-Api-Key", "test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var result map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "Test Show", result["title"])
	assert.Equal(t, float64(12345), result["tvdbId"])
	assert.NotNil(t, result["id"])
}

func TestLanguageProfiles(t *testing.T) {
	srv, _, cleanup := setupCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v3/languageprofile", nil)
	req.Header.Set("X-Api-Key", "test")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &results)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "English", results[0]["name"])
}
```

**Step 2: Run tests**

```bash
task test -- -run TestSonarr -v
task test -- -run TestLanguageProfiles -v
```

**Step 3: Commit**

```bash
git add internal/api/compat/compat_test.go
git commit -m "test(compat): add Sonarr endpoint integration tests"
```

---

## Task 8: Final Verification with Overseerr

**Step 1: Build and run**

```bash
task build
./arrgod
```

**Step 2: Configure Overseerr**

In Overseerr settings:
1. Add Sonarr server pointing to arrgo (e.g., `http://localhost:8484`)
2. Use API key from arrgo config
3. Test connection - should succeed

**Step 3: Test TV request flow**

1. Search for a TV show in Overseerr
2. Request it
3. Verify it appears in arrgo library: `./arrgo status`

**Step 4: Check logs for any errors**

```bash
# In arrgod terminal, look for Sonarr API calls
```

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix(compat): address Overseerr integration issues"
```

---

## Summary

After completing this plan, arrgo will support:

| Feature | Status |
|---------|--------|
| Sonarr connection test | ✅ |
| Language profiles | ✅ |
| Quality profiles | ✅ (existing) |
| Root folders | ✅ (existing) |
| Series lookup by TVDB ID | ✅ |
| Add series from Overseerr | ✅ |
| List existing series | ✅ |
| Update series monitoring | ✅ |
| Auto-search on add | ✅ |

This enables the full Overseerr → arrgo flow for TV series requests.
