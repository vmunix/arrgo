# Compat API Wiring Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire Radarr/Sonarr compat API handlers to actual stores for Overseerr integration.

**Architecture:** Inject library.Store, download.Store, search.Searcher, and download.Manager into the compat Server. Translate Radarr/Sonarr request formats to native arrgo operations.

**Tech Stack:** Go net/http, existing arrgo stores and services

---

### Task 1: Add Dependencies to Server

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Update Server struct and constructor**

Replace the Server struct and New function:

```go
// Config holds compat API configuration.
type Config struct {
	APIKey          string
	MovieRoot       string
	SeriesRoot      string
	QualityProfiles map[string]int // name -> id mapping
}

// Server provides Radarr/Sonarr API compatibility.
type Server struct {
	cfg      Config
	library  *library.Store
	downloads *download.Store
	searcher *search.Searcher
	manager  *download.Manager
}

// New creates a new compatibility server.
func New(cfg Config, lib *library.Store, dl *download.Store) *Server {
	return &Server{
		cfg:       cfg,
		library:   lib,
		downloads: dl,
	}
}

// SetSearcher configures the searcher (optional).
func (s *Server) SetSearcher(searcher *search.Searcher) {
	s.searcher = searcher
}

// SetManager configures the download manager (optional).
func (s *Server) SetManager(manager *download.Manager) {
	s.manager = manager
}
```

**Step 2: Add imports**

```go
import (
	"encoding/json"
	"net/http"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/search"
)
```

**Step 3: Verify it compiles**

Run: `go build ./internal/api/compat/...`
Expected: Compiles (handlers still use stubs)

**Step 4: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): add dependencies to server"
```

---

### Task 2: Implement Root Folders and Quality Profiles

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Update listRootFolders**

```go
func (s *Server) listRootFolders(w http.ResponseWriter, r *http.Request) {
	folders := []map[string]any{}

	if s.cfg.MovieRoot != "" {
		folders = append(folders, map[string]any{
			"id":        1,
			"path":      s.cfg.MovieRoot,
			"freeSpace": 0,
		})
	}
	if s.cfg.SeriesRoot != "" {
		folders = append(folders, map[string]any{
			"id":        2,
			"path":      s.cfg.SeriesRoot,
			"freeSpace": 0,
		})
	}

	writeJSON(w, http.StatusOK, folders)
}
```

**Step 2: Update listQualityProfiles**

```go
func (s *Server) listQualityProfiles(w http.ResponseWriter, r *http.Request) {
	profiles := make([]map[string]any, 0, len(s.cfg.QualityProfiles))
	for name, id := range s.cfg.QualityProfiles {
		profiles = append(profiles, map[string]any{
			"id":   id,
			"name": name,
		})
	}
	writeJSON(w, http.StatusOK, profiles)
}
```

**Step 3: Verify it compiles**

Run: `go build ./internal/api/compat/...`

**Step 4: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): implement root folders and quality profiles"
```

---

### Task 3: Implement Add Movie

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Add request/response types**

```go
// radarrAddRequest is the Radarr format for adding a movie.
type radarrAddRequest struct {
	TMDBID           int64  `json:"tmdbId"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        bool   `json:"monitored"`
	AddOptions       struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// radarrMovieResponse is the Radarr format for a movie.
type radarrMovieResponse struct {
	ID        int64  `json:"id"`
	TMDBID    int64  `json:"tmdbId"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Monitored bool   `json:"monitored"`
	Status    string `json:"status"`
}
```

**Step 2: Implement addMovie**

```go
func (s *Server) addMovie(w http.ResponseWriter, r *http.Request) {
	var req radarrAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Map quality profile ID to name
	profileName := "hd" // default
	for name, id := range s.cfg.QualityProfiles {
		if id == req.QualityProfileID {
			profileName = name
			break
		}
	}

	// Determine root path
	rootPath := req.RootFolderPath
	if rootPath == "" {
		rootPath = s.cfg.MovieRoot
	}

	// Add to library
	tmdbID := req.TMDBID
	content := &library.Content{
		Type:           library.ContentTypeMovie,
		TMDBID:         &tmdbID,
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

	// Auto-search if requested and searcher available
	if req.AddOptions.SearchForMovie && s.searcher != nil && s.manager != nil {
		go s.searchAndGrab(content.ID, req.Title, req.Year, profileName)
	}

	writeJSON(w, http.StatusCreated, radarrMovieResponse{
		ID:        content.ID,
		TMDBID:    req.TMDBID,
		Title:     req.Title,
		Year:      req.Year,
		Monitored: req.Monitored,
		Status:    "announced",
	})
}
```

**Step 3: Add searchAndGrab helper**

```go
func (s *Server) searchAndGrab(contentID int64, title string, year int, profile string) {
	ctx := context.Background()

	query := search.Query{
		Query: fmt.Sprintf("%s %d", title, year),
		Type:  "movie",
	}

	result, err := s.searcher.Search(ctx, query, profile)
	if err != nil || len(result.Releases) == 0 {
		return
	}

	// Grab the best match (first result after scoring/sorting)
	best := result.Releases[0]
	_, _ = s.manager.Grab(ctx, contentID, nil, best.DownloadURL, best.Title, best.Indexer)
}
```

**Step 4: Add context and fmt imports**

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	// ... existing imports
)
```

**Step 5: Verify it compiles**

Run: `go build ./internal/api/compat/...`

**Step 6: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): implement add movie with auto-search"
```

---

### Task 4: Implement Add Series

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Add Sonarr request/response types**

```go
// sonarrAddRequest is the Sonarr format for adding a series.
type sonarrAddRequest struct {
	TVDBID           int64  `json:"tvdbId"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        bool   `json:"monitored"`
	AddOptions       struct {
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// sonarrSeriesResponse is the Sonarr format for a series.
type sonarrSeriesResponse struct {
	ID        int64  `json:"id"`
	TVDBID    int64  `json:"tvdbId"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Monitored bool   `json:"monitored"`
	Status    string `json:"status"`
}
```

**Step 2: Implement addSeries**

```go
func (s *Server) addSeries(w http.ResponseWriter, r *http.Request) {
	var req sonarrAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
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

	// Note: Series auto-search is more complex (episodes), skip for v1

	writeJSON(w, http.StatusCreated, sonarrSeriesResponse{
		ID:        content.ID,
		TVDBID:    req.TVDBID,
		Title:     req.Title,
		Year:      req.Year,
		Monitored: req.Monitored,
		Status:    "continuing",
	})
}
```

**Step 3: Verify it compiles**

Run: `go build ./internal/api/compat/...`

**Step 4: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): implement add series"
```

---

### Task 5: Implement List Queue

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Implement listQueue**

```go
func (s *Server) listQueue(w http.ResponseWriter, r *http.Request) {
	downloads, err := s.downloads.List(download.DownloadFilter{Active: true})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	records := make([]map[string]any, 0, len(downloads))
	for _, dl := range downloads {
		records = append(records, map[string]any{
			"id":           dl.ID,
			"movieId":      dl.ContentID,
			"title":        dl.ReleaseName,
			"status":       string(dl.Status),
			"trackedDownloadStatus": "ok",
			"indexer":      dl.Indexer,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"page":         1,
		"pageSize":     len(records),
		"totalRecords": len(records),
		"records":      records,
	})
}
```

**Step 2: Verify it compiles**

Run: `go build ./internal/api/compat/...`

**Step 3: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): implement list queue"
```

---

### Task 6: Wire Compat Server in serve.go

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Update compat server creation**

Find the compat API section and replace:

```go
// Compat API (if enabled)
if cfg.Compat.Radarr || cfg.Compat.Sonarr {
	// Build quality profile ID map
	profileIDs := make(map[string]int)
	id := 1
	for name := range cfg.Quality.Profiles {
		profileIDs[name] = id
		id++
	}

	compatCfg := compat.Config{
		APIKey:          cfg.Compat.APIKey,
		MovieRoot:       cfg.Libraries.Movies.Root,
		SeriesRoot:      cfg.Libraries.Series.Root,
		QualityProfiles: profileIDs,
	}
	apiCompat := compat.New(compatCfg, libraryStore, downloadStore)
	apiCompat.SetSearcher(searcher)
	apiCompat.SetManager(downloadManager)
	apiCompat.RegisterRoutes(mux)
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(cli): wire compat server with dependencies"
```

---

### Task 7: Final Verification

**Step 1: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: No new issues

**Step 3: Build and quick smoke test**

Run: `go build ./cmd/arrgo && ./arrgo serve &`

Test endpoints:
```bash
# Get root folders
curl -H "X-Api-Key: test" http://localhost:8484/api/v3/rootfolder

# Get quality profiles
curl -H "X-Api-Key: test" http://localhost:8484/api/v3/qualityprofile

# Get queue
curl -H "X-Api-Key: test" http://localhost:8484/api/v3/queue
```

Kill server: `kill %1`

**Step 4: Commit any fixes**

---

## Summary

After completing all tasks, the compat API will:
1. Accept movie/series additions from Overseerr
2. Return configured root folders and quality profiles
3. Show active downloads in queue
4. Auto-search and grab when adding movies (if Prowlarr configured)
