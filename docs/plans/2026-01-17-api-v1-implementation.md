# API v1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire up the existing API v1 stub handlers to the working library, search, download, and importer modules.

**Architecture:** The `Server` struct receives dependencies via constructor injection. Each handler decodes request → calls store/module → encodes response. Request/response types are defined in a separate `types.go` file for API-specific DTOs.

**Tech Stack:** Go stdlib (net/http, encoding/json), existing internal modules (library, search, download, importer).

---

### Task 1: Server Dependencies & Request Helpers

**Files:**
- Modify: `internal/api/v1/api.go`
- Create: `internal/api/v1/api_test.go`

**Step 1: Write test for Server construction**

```go
// internal/api/v1/api_test.go
package v1

import (
	"database/sql"
	_ "embed"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db
}

func TestNew(t *testing.T) {
	db := setupTestDB(t)
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
	}

	srv := New(db, cfg)
	if srv == nil {
		t.Fatal("New returned nil")
	}
	if srv.library == nil {
		t.Error("library store not initialized")
	}
	if srv.downloads == nil {
		t.Error("download store not initialized")
	}
}
```

**Step 2: Create testdata directory and copy schema**

```bash
mkdir -p internal/api/v1/testdata
cp migrations/001_initial.sql internal/api/v1/testdata/schema.sql
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -v -run TestNew`
Expected: FAIL (Config not defined, New signature wrong)

**Step 4: Write implementation - update Server struct and New**

```go
// internal/api/v1/api.go - replace existing Server struct and New function

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/importer"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/search"
)

// Config holds API server configuration.
type Config struct {
	MovieRoot       string
	SeriesRoot      string
	QualityProfiles map[string][]string
}

// Server is the v1 API server.
type Server struct {
	library   *library.Store
	downloads *download.Store
	manager   *download.Manager
	searcher  *search.Searcher
	history   *importer.HistoryStore
	plex      *importer.PlexClient
	cfg       Config
}

// New creates a new v1 API server.
func New(db *sql.DB, cfg Config) *Server {
	return &Server{
		library:   library.NewStore(db),
		downloads: download.NewStore(db),
		history:   importer.NewHistoryStore(db),
		cfg:       cfg,
	}
}

// SetSearcher configures the searcher (requires external Prowlarr client).
func (s *Server) SetSearcher(searcher *search.Searcher) {
	s.searcher = searcher
}

// SetManager configures the download manager (requires external SABnzbd client).
func (s *Server) SetManager(manager *download.Manager) {
	s.manager = manager
}

// SetPlex configures the Plex client for library scans.
func (s *Server) SetPlex(plex *importer.PlexClient) {
	s.plex = plex
}
```

**Step 5: Add helper functions for request parsing**

```go
// Add to api.go after writeJSON

// pathID extracts an integer ID from the URL path.
func pathID(r *http.Request, name string) (int64, error) {
	idStr := r.PathValue(name)
	if idStr == "" {
		return 0, fmt.Errorf("missing path parameter: %s", name)
	}
	return strconv.ParseInt(idStr, 10, 64)
}

// queryInt extracts an optional integer from query string.
func queryInt(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// queryString extracts an optional string from query string.
func queryString(r *http.Request, name string) *string {
	val := r.URL.Query().Get(name)
	if val == "" {
		return nil
	}
	return &val
}
```

**Step 6: Add fmt import**

Add `"fmt"` to the imports.

**Step 7: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -v -run TestNew`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go internal/api/v1/testdata/
git commit -m "feat(api): add Server dependencies and request helpers"
```

---

### Task 2: Content List & Get Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`

**Step 1: Write tests for listContent and getContent**

```go
// Add to api_test.go

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/arrgo/arrgo/internal/library"
)

func TestListContent_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestListContent_WithItems(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add test content
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := srv.library.AddContent(c); err != nil {
		t.Fatalf("add content: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("items = %d, want 1", len(resp.Items))
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
	if resp.Items[0].Title != "Test Movie" {
		t.Errorf("title = %q, want %q", resp.Items[0].Title, "Test Movie")
	}
}

func TestListContent_WithFilters(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add movie
	movie := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	srv.library.AddContent(movie)

	// Add series
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Series",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	srv.library.AddContent(series)

	// Filter by type
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content?type=movie", nil)
	w := httptest.NewRecorder()
	srv.listContent(w, req)

	var resp listContentResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Errorf("filter by type: items = %d, want 1", len(resp.Items))
	}

	// Filter by status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/content?status=available", nil)
	w = httptest.NewRecorder()
	srv.listContent(w, req)

	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Errorf("filter by status: items = %d, want 1", len(resp.Items))
	}
}

func TestGetContent_Found(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	srv.library.AddContent(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.getContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp contentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Title != "Test Movie" {
		t.Errorf("title = %q, want %q", resp.Title, "Test Movie")
	}
}

func TestGetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.getContent(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestListContent|TestGetContent"`
Expected: FAIL (response types not defined)

**Step 3: Create types.go with response types**

```go
// internal/api/v1/types.go
package v1

import "time"

// contentResponse is the API representation of content.
type contentResponse struct {
	ID             int64     `json:"id"`
	Type           string    `json:"type"`
	TMDBID         *int64    `json:"tmdb_id,omitempty"`
	TVDBID         *int64    `json:"tvdb_id,omitempty"`
	Title          string    `json:"title"`
	Year           int       `json:"year"`
	Status         string    `json:"status"`
	QualityProfile string    `json:"quality_profile"`
	RootPath       string    `json:"root_path"`
	AddedAt        time.Time `json:"added_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// listContentResponse is the response for GET /content.
type listContentResponse struct {
	Items  []contentResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}
```

**Step 4: Implement listContent handler**

```go
// Replace listContent in api.go

func (s *Server) listContent(w http.ResponseWriter, r *http.Request) {
	// Parse filters
	filter := library.ContentFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}

	if typeStr := queryString(r, "type"); typeStr != nil {
		t := library.ContentType(*typeStr)
		filter.Type = &t
	}
	if statusStr := queryString(r, "status"); statusStr != nil {
		st := library.ContentStatus(*statusStr)
		filter.Status = &st
	}

	items, total, err := s.library.ListContent(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listContentResponse{
		Items:  make([]contentResponse, len(items)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, c := range items {
		resp.Items[i] = contentToResponse(c)
	}

	writeJSON(w, http.StatusOK, resp)
}

func contentToResponse(c *library.Content) contentResponse {
	return contentResponse{
		ID:             c.ID,
		Type:           string(c.Type),
		TMDBID:         c.TMDBID,
		TVDBID:         c.TVDBID,
		Title:          c.Title,
		Year:           c.Year,
		Status:         string(c.Status),
		QualityProfile: c.QualityProfile,
		RootPath:       c.RootPath,
		AddedAt:        c.AddedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}
```

**Step 5: Implement getContent handler**

```go
// Replace getContent in api.go

func (s *Server) getContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	c, err := s.library.GetContent(id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, contentToResponse(c))
}
```

**Step 6: Add errors import**

Add `"errors"` to the imports.

**Step 7: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestListContent|TestGetContent"`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go internal/api/v1/types.go
git commit -m "feat(api): implement content list and get endpoints"
```

---

### Task 3: Content Add, Update, Delete Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add request types to types.go**

```go
// Add to types.go

// addContentRequest is the request body for POST /content.
type addContentRequest struct {
	Type           string `json:"type"`
	TMDBID         *int64 `json:"tmdb_id,omitempty"`
	TVDBID         *int64 `json:"tvdb_id,omitempty"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	QualityProfile string `json:"quality_profile"`
	RootPath       string `json:"root_path,omitempty"`
}

// updateContentRequest is the request body for PUT /content/:id.
type updateContentRequest struct {
	Status         *string `json:"status,omitempty"`
	QualityProfile *string `json:"quality_profile,omitempty"`
}
```

**Step 2: Write tests for add, update, delete**

```go
// Add to api_test.go

import "strings"

func TestAddContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{MovieRoot: "/movies", SeriesRoot: "/tv"})

	body := `{"type":"movie","title":"New Movie","year":2024,"quality_profile":"hd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.addContent(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp contentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID == 0 {
		t.Error("ID should be set")
	}
	if resp.Title != "New Movie" {
		t.Errorf("title = %q, want %q", resp.Title, "New Movie")
	}
	if resp.Status != "wanted" {
		t.Errorf("status = %q, want wanted", resp.Status)
	}
	if resp.RootPath != "/movies" {
		t.Errorf("root_path = %q, want /movies", resp.RootPath)
	}
}

func TestAddContent_InvalidType(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"type":"invalid","title":"Test","year":2024,"quality_profile":"hd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.addContent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content first
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	srv.library.AddContent(c)

	body := `{"status":"available"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/content/1", strings.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.updateContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update
	updated, _ := srv.library.GetContent(1)
	if updated.Status != library.StatusAvailable {
		t.Errorf("status = %q, want available", updated.Status)
	}
}

func TestDeleteContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content first
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	srv.library.AddContent(c)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/content/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteContent(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deleted
	_, err := srv.library.GetContent(1)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestAddContent|TestUpdateContent|TestDeleteContent"`
Expected: FAIL

**Step 4: Implement addContent handler**

```go
// Replace addContent in api.go

func (s *Server) addContent(w http.ResponseWriter, r *http.Request) {
	var req addContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Validate type
	contentType := library.ContentType(req.Type)
	if contentType != library.ContentTypeMovie && contentType != library.ContentTypeSeries {
		writeError(w, http.StatusBadRequest, "INVALID_TYPE", "type must be 'movie' or 'series'")
		return
	}

	// Default root path based on type
	rootPath := req.RootPath
	if rootPath == "" {
		if contentType == library.ContentTypeMovie {
			rootPath = s.cfg.MovieRoot
		} else {
			rootPath = s.cfg.SeriesRoot
		}
	}

	c := &library.Content{
		Type:           contentType,
		TMDBID:         req.TMDBID,
		TVDBID:         req.TVDBID,
		Title:          req.Title,
		Year:           req.Year,
		Status:         library.StatusWanted,
		QualityProfile: req.QualityProfile,
		RootPath:       rootPath,
	}

	if err := s.library.AddContent(c); err != nil {
		if errors.Is(err, library.ErrDuplicate) {
			writeError(w, http.StatusConflict, "DUPLICATE", "Content already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, contentToResponse(c))
}
```

**Step 5: Implement updateContent handler**

```go
// Replace updateContent in api.go

func (s *Server) updateContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var req updateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	c, err := s.library.GetContent(id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Apply updates
	if req.Status != nil {
		c.Status = library.ContentStatus(*req.Status)
	}
	if req.QualityProfile != nil {
		c.QualityProfile = *req.QualityProfile
	}

	if err := s.library.UpdateContent(c); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, contentToResponse(c))
}
```

**Step 6: Implement deleteContent handler**

```go
// Replace deleteContent in api.go

func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if err := s.library.DeleteContent(id); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

**Step 7: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestAddContent|TestUpdateContent|TestDeleteContent"`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): implement content add, update, delete endpoints"
```

---

### Task 4: Episode Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add episode types to types.go**

```go
// Add to types.go

// episodeResponse is the API representation of an episode.
type episodeResponse struct {
	ID        int64      `json:"id"`
	ContentID int64      `json:"content_id"`
	Season    int        `json:"season"`
	Episode   int        `json:"episode"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	AirDate   *time.Time `json:"air_date,omitempty"`
}

// listEpisodesResponse is the response for GET /content/:id/episodes.
type listEpisodesResponse struct {
	Items []episodeResponse `json:"items"`
	Total int               `json:"total"`
}

// updateEpisodeRequest is the request body for PUT /episodes/:id.
type updateEpisodeRequest struct {
	Status *string `json:"status,omitempty"`
}
```

**Step 2: Write tests for episode endpoints**

```go
// Add to api_test.go

func TestListEpisodes(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add series
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	srv.library.AddContent(series)

	// Add episodes
	for i := 1; i <= 3; i++ {
		ep := &library.Episode{
			ContentID: series.ID,
			Season:    1,
			Episode:   i,
			Title:     fmt.Sprintf("Episode %d", i),
			Status:    library.StatusWanted,
		}
		srv.library.AddEpisode(ep)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/1/episodes", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.listEpisodes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listEpisodesResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Items) != 3 {
		t.Errorf("items = %d, want 3", len(resp.Items))
	}
}

func TestUpdateEpisode(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add series and episode
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	srv.library.AddContent(series)

	ep := &library.Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    library.StatusWanted,
	}
	srv.library.AddEpisode(ep)

	body := `{"status":"available"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/episodes/1", strings.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.updateEpisode(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update
	updated, _ := srv.library.GetEpisode(1)
	if updated.Status != library.StatusAvailable {
		t.Errorf("status = %q, want available", updated.Status)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestListEpisodes|TestUpdateEpisode"`
Expected: FAIL

**Step 4: Implement listEpisodes handler**

```go
// Replace listEpisodes in api.go

func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request) {
	contentID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	filter := library.EpisodeFilter{ContentID: &contentID}
	episodes, err := s.library.ListEpisodes(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listEpisodesResponse{
		Items: make([]episodeResponse, len(episodes)),
		Total: len(episodes),
	}

	for i, ep := range episodes {
		resp.Items[i] = episodeToResponse(ep)
	}

	writeJSON(w, http.StatusOK, resp)
}

func episodeToResponse(ep *library.Episode) episodeResponse {
	return episodeResponse{
		ID:        ep.ID,
		ContentID: ep.ContentID,
		Season:    ep.Season,
		Episode:   ep.Episode,
		Title:     ep.Title,
		Status:    string(ep.Status),
		AirDate:   ep.AirDate,
	}
}
```

**Step 5: Implement updateEpisode handler**

```go
// Replace updateEpisode in api.go

func (s *Server) updateEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var req updateEpisodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	ep, err := s.library.GetEpisode(id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Episode not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	if req.Status != nil {
		ep.Status = library.ContentStatus(*req.Status)
	}

	if err := s.library.UpdateEpisode(ep); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, episodeToResponse(ep))
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestListEpisodes|TestUpdateEpisode"`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): implement episode list and update endpoints"
```

---

### Task 5: Search & Grab Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add search/grab types to types.go**

```go
// Add to types.go

// searchRequest is the request body for POST /search.
type searchRequest struct {
	ContentID *int64  `json:"content_id,omitempty"`
	Query     string  `json:"query,omitempty"`
	Type      string  `json:"type,omitempty"`
	Season    *int    `json:"season,omitempty"`
	Episode   *int    `json:"episode,omitempty"`
	Profile   string  `json:"profile,omitempty"`
}

// releaseResponse is the API representation of a search result.
type releaseResponse struct {
	Title       string    `json:"title"`
	Indexer     string    `json:"indexer"`
	GUID        string    `json:"guid"`
	DownloadURL string    `json:"download_url"`
	Size        int64     `json:"size"`
	PublishDate time.Time `json:"publish_date"`
	Quality     string    `json:"quality,omitempty"`
	Score       int       `json:"score"`
}

// searchResponse is the response for POST /search.
type searchResponse struct {
	Releases []releaseResponse `json:"releases"`
	Errors   []string          `json:"errors,omitempty"`
}

// grabRequest is the request body for POST /grab.
type grabRequest struct {
	ContentID   int64  `json:"content_id"`
	EpisodeID   *int64 `json:"episode_id,omitempty"`
	DownloadURL string `json:"download_url"`
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
}

// grabResponse is the response for POST /grab.
type grabResponse struct {
	DownloadID int64  `json:"download_id"`
	Status     string `json:"status"`
}
```

**Step 2: Write tests**

```go
// Add to api_test.go

func TestSearch_NoSearcher(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"query":"test movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.search(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestGrab_NoManager(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"content_id":1,"download_url":"http://example.com/nzb","title":"Test","indexer":"TestIndexer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.grab(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestSearch|TestGrab"`
Expected: FAIL

**Step 4: Implement search handler**

```go
// Replace search in api.go

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	if s.searcher == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Searcher not configured")
		return
	}

	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	profile := req.Profile
	if profile == "" {
		profile = "hd"
	}

	q := search.Query{
		Text:    req.Query,
		Type:    req.Type,
		Season:  req.Season,
		Episode: req.Episode,
	}
	if req.ContentID != nil {
		q.ContentID = *req.ContentID
	}

	result, err := s.searcher.Search(r.Context(), q, profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SEARCH_ERROR", err.Error())
		return
	}

	resp := searchResponse{
		Releases: make([]releaseResponse, len(result.Releases)),
	}

	for i, rel := range result.Releases {
		quality := ""
		if rel.Quality != nil {
			quality = rel.Quality.Resolution.String()
		}
		resp.Releases[i] = releaseResponse{
			Title:       rel.Title,
			Indexer:     rel.Indexer,
			GUID:        rel.GUID,
			DownloadURL: rel.DownloadURL,
			Size:        rel.Size,
			PublishDate: rel.PublishDate,
			Quality:     quality,
			Score:       rel.Score,
		}
	}

	for _, e := range result.Errors {
		resp.Errors = append(resp.Errors, e.Error())
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 5: Implement grab handler**

```go
// Replace grab in api.go

func (s *Server) grab(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Download manager not configured")
		return
	}

	var req grabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	d, err := s.manager.Grab(r.Context(), req.ContentID, req.EpisodeID, req.DownloadURL, req.Title, req.Indexer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GRAB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, grabResponse{
		DownloadID: d.ID,
		Status:     string(d.Status),
	})
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestSearch|TestGrab"`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): implement search and grab endpoints"
```

---

### Task 6: Download Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add download types to types.go**

```go
// Add to types.go

// downloadResponse is the API representation of a download.
type downloadResponse struct {
	ID          int64      `json:"id"`
	ContentID   int64      `json:"content_id"`
	EpisodeID   *int64     `json:"episode_id,omitempty"`
	Client      string     `json:"client"`
	ClientID    string     `json:"client_id"`
	Status      string     `json:"status"`
	ReleaseName string     `json:"release_name"`
	Indexer     string     `json:"indexer"`
	AddedAt     time.Time  `json:"added_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// listDownloadsResponse is the response for GET /downloads.
type listDownloadsResponse struct {
	Items []downloadResponse `json:"items"`
	Total int                `json:"total"`
}
```

**Step 2: Write tests**

```go
// Add to api_test.go

func TestListDownloads_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads", nil)
	w := httptest.NewRecorder()

	srv.listDownloads(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listDownloadsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
}

func TestGetDownload_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.getDownload(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteDownload_NoManager(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/downloads/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteDownload(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestListDownloads|TestGetDownload|TestDeleteDownload"`
Expected: FAIL

**Step 4: Implement download handlers**

```go
// Replace listDownloads, getDownload, deleteDownload in api.go

func (s *Server) listDownloads(w http.ResponseWriter, r *http.Request) {
	filter := download.DownloadFilter{}
	if activeStr := r.URL.Query().Get("active"); activeStr == "true" {
		filter.Active = true
	}

	downloads, err := s.downloads.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listDownloadsResponse{
		Items: make([]downloadResponse, len(downloads)),
		Total: len(downloads),
	}

	for i, d := range downloads {
		resp.Items[i] = downloadToResponse(d)
	}

	writeJSON(w, http.StatusOK, resp)
}

func downloadToResponse(d *download.Download) downloadResponse {
	return downloadResponse{
		ID:          d.ID,
		ContentID:   d.ContentID,
		EpisodeID:   d.EpisodeID,
		Client:      string(d.Client),
		ClientID:    d.ClientID,
		Status:      string(d.Status),
		ReleaseName: d.ReleaseName,
		Indexer:     d.Indexer,
		AddedAt:     d.AddedAt,
		CompletedAt: d.CompletedAt,
	}
}

func (s *Server) getDownload(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	d, err := s.downloads.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, downloadToResponse(d))
}

func (s *Server) deleteDownload(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Download manager not configured")
		return
	}

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	deleteFiles := r.URL.Query().Get("delete_files") == "true"
	if err := s.manager.Cancel(r.Context(), id, deleteFiles); err != nil {
		writeError(w, http.StatusInternalServerError, "CANCEL_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestListDownloads|TestGetDownload|TestDeleteDownload"`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): implement download list, get, delete endpoints"
```

---

### Task 7: History & Files Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add history and file types to types.go**

```go
// Add to types.go

// historyResponse is the API representation of a history entry.
type historyResponse struct {
	ID        int64     `json:"id"`
	ContentID int64     `json:"content_id"`
	EpisodeID *int64    `json:"episode_id,omitempty"`
	Event     string    `json:"event"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

// listHistoryResponse is the response for GET /history.
type listHistoryResponse struct {
	Items []historyResponse `json:"items"`
	Total int               `json:"total"`
}

// fileResponse is the API representation of a file.
type fileResponse struct {
	ID        int64     `json:"id"`
	ContentID int64     `json:"content_id"`
	EpisodeID *int64    `json:"episode_id,omitempty"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Quality   string    `json:"quality"`
	Source    string    `json:"source"`
	AddedAt   time.Time `json:"added_at"`
}

// listFilesResponse is the response for GET /files.
type listFilesResponse struct {
	Items []fileResponse `json:"items"`
	Total int            `json:"total"`
}
```

**Step 2: Write tests**

```go
// Add to api_test.go

func TestListHistory_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history", nil)
	w := httptest.NewRecorder()

	srv.listHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listHistoryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
}

func TestListFiles_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	w := httptest.NewRecorder()

	srv.listFiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listFilesResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
}

func TestDeleteFile(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content and file
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	srv.library.AddContent(c)

	f := &library.File{
		ContentID: c.ID,
		Path:      "/movies/test.mkv",
		SizeBytes: 1000,
		Quality:   "1080p",
		Source:    "test",
	}
	srv.library.AddFile(f)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteFile(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestListHistory|TestListFiles|TestDeleteFile"`
Expected: FAIL

**Step 4: Implement history handler**

```go
// Replace listHistory in api.go

func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	filter := importer.HistoryFilter{
		Limit: queryInt(r, "limit", 50),
	}

	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	entries, err := s.history.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listHistoryResponse{
		Items: make([]historyResponse, len(entries)),
		Total: len(entries),
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

**Step 5: Implement file handlers**

```go
// Replace listFiles and deleteFile in api.go

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	filter := library.FileFilter{}
	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	files, err := s.library.ListFiles(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listFilesResponse{
		Items: make([]fileResponse, len(files)),
		Total: len(files),
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

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if err := s.library.DeleteFile(id); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestListHistory|TestListFiles|TestDeleteFile"`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): implement history and files endpoints"
```

---

### Task 8: System Endpoints

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/api_test.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add system types to types.go**

```go
// Add to types.go

// statusResponse is the response for GET /status.
type statusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// profileResponse is the API representation of a quality profile.
type profileResponse struct {
	Name   string   `json:"name"`
	Accept []string `json:"accept"`
}

// listProfilesResponse is the response for GET /profiles.
type listProfilesResponse struct {
	Profiles []profileResponse `json:"profiles"`
}

// scanRequest is the request body for POST /scan.
type scanRequest struct {
	Path string `json:"path,omitempty"`
}
```

**Step 2: Write tests**

```go
// Add to api_test.go

func TestGetStatus(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()

	srv.getStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp statusResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

func TestListProfiles(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{
		QualityProfiles: map[string][]string{
			"hd":  {"1080p bluray", "1080p webdl"},
			"uhd": {"2160p bluray"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	w := httptest.NewRecorder()

	srv.listProfiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listProfilesResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Profiles) != 2 {
		t.Errorf("profiles = %d, want 2", len(resp.Profiles))
	}
}

func TestTriggerScan_NoPlex(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scan", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	srv.triggerScan(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/v1/... -v -run "TestGetStatus|TestListProfiles|TestTriggerScan"`
Expected: FAIL

**Step 4: Implement system handlers**

```go
// Replace getStatus, listProfiles, triggerScan in api.go

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Status:  "ok",
		Version: "0.1.0",
	})
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles := make([]profileResponse, 0, len(s.cfg.QualityProfiles))
	for name, accept := range s.cfg.QualityProfiles {
		profiles = append(profiles, profileResponse{
			Name:   name,
			Accept: accept,
		})
	}

	writeJSON(w, http.StatusOK, listProfilesResponse{Profiles: profiles})
}

func (s *Server) triggerScan(w http.ResponseWriter, r *http.Request) {
	if s.plex == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Plex not configured")
		return
	}

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	if req.Path != "" {
		if err := s.plex.ScanPath(r.Context(), req.Path); err != nil {
			writeError(w, http.StatusInternalServerError, "SCAN_ERROR", err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "scan triggered"})
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/v1/... -v -run "TestGetStatus|TestListProfiles|TestTriggerScan"`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): implement system status, profiles, and scan endpoints"
```

---

### Task 9: Final Verification

**Step 1: Run all API tests**

Run: `go test ./internal/api/v1/... -v`
Expected: All tests PASS

**Step 2: Run linter**

Run: `golangci-lint run ./internal/api/v1/...`
Expected: No issues

**Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 4: Verify build**

Run: `go build ./...`
Expected: Success

---

## Summary

| Task | Description | Endpoints |
|------|-------------|-----------|
| 1 | Server dependencies & request helpers | - |
| 2 | Content list & get | GET /content, GET /content/:id |
| 3 | Content add, update, delete | POST /content, PUT /content/:id, DELETE /content/:id |
| 4 | Episode endpoints | GET /content/:id/episodes, PUT /episodes/:id |
| 5 | Search & grab | POST /search, POST /grab |
| 6 | Download endpoints | GET /downloads, GET /downloads/:id, DELETE /downloads/:id |
| 7 | History & files | GET /history, GET /files, DELETE /files/:id |
| 8 | System endpoints | GET /status, GET /profiles, POST /scan |
| 9 | Final verification | - |

**Total: 18 endpoints wired up**
