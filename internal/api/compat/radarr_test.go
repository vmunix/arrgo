// internal/api/compat/radarr_test.go
package compat

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/library"
)

// Test constants
const (
	testAPIKey     = "test-api-key"
	testMovieRoot  = "/movies"
	testSeriesRoot = "/series"
)

// Test response structs for type-safe assertions
type testQualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type testRootFolder struct {
	ID        int    `json:"id"`
	Path      string `json:"path"`
	FreeSpace int64  `json:"freeSpace"`
}

type testQueueRecord struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Indexer  string `json:"indexer"`
	MovieID  int    `json:"movieId"`
	Protocol string `json:"protocol"`
	Status   string `json:"status"`
}

type testQueueResponse struct {
	Page         int               `json:"page"`
	PageSize     int               `json:"pageSize"`
	TotalRecords int               `json:"totalRecords"`
	Records      []testQueueRecord `json:"records"`
}

type testErrorResponse struct {
	Error string `json:"error"`
}

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

func setupServer(t *testing.T, apiKey string) (*Server, *http.ServeMux, *sql.DB) {
	t.Helper()
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dl := download.NewStore(db)

	cfg := Config{
		APIKey:          apiKey,
		MovieRoot:       testMovieRoot,
		SeriesRoot:      testSeriesRoot,
		QualityProfiles: map[string]int{"hd": 1, "uhd": 2},
	}

	srv := New(cfg, lib, dl)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	return srv, mux, db
}

// Auth Middleware Tests

func TestAuthMiddleware_NoAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	var resp testErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "Invalid API key" {
		t.Errorf("error = %q, want %q", resp.Error, "Invalid API key")
	}
}

func TestAuthMiddleware_WrongAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, "correct-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", "wrong-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	var resp testErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "Invalid API key" {
		t.Errorf("error = %q, want %q", resp.Error, "Invalid API key")
	}
}

func TestAuthMiddleware_CorrectAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_APIKeyQueryParam(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie?apikey="+testAPIKey, nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// Quality Profiles Tests

func TestListQualityProfiles_ReturnsConfiguredProfiles(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/qualityprofile", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var profiles []testQualityProfile
	if err := json.Unmarshal(w.Body.Bytes(), &profiles); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(profiles) != 2 {
		t.Errorf("profiles count = %d, want 2", len(profiles))
	}
}

func TestListQualityProfiles_IncludesIDAndName(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/qualityprofile", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var profiles []testQualityProfile
	if err := json.Unmarshal(w.Body.Bytes(), &profiles); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify specific mappings exist
	foundHD := false
	foundUHD := false
	for _, profile := range profiles {
		if profile.Name == "hd" && profile.ID == 1 {
			foundHD = true
		}
		if profile.Name == "uhd" && profile.ID == 2 {
			foundUHD = true
		}
	}
	if !foundHD {
		t.Error("hd profile with id=1 not found")
	}
	if !foundUHD {
		t.Error("uhd profile with id=2 not found")
	}
}

// Root Folders Tests

func TestListRootFolders_ReturnsConfiguredRoots(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/rootfolder", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var folders []testRootFolder
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(folders) != 2 {
		t.Errorf("folders count = %d, want 2", len(folders))
	}

	// Verify paths
	paths := make(map[string]bool)
	for _, folder := range folders {
		paths[folder.Path] = true
	}
	if !paths[testMovieRoot] {
		t.Errorf("%s root folder not found", testMovieRoot)
	}
	if !paths[testSeriesRoot] {
		t.Errorf("%s root folder not found", testSeriesRoot)
	}
}

func TestListRootFolders_EmptyWhenNoRootsConfigured(t *testing.T) {
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dlStore := download.NewStore(db)

	cfg := Config{
		APIKey:          testAPIKey,
		MovieRoot:       "", // Empty roots
		SeriesRoot:      "",
		QualityProfiles: map[string]int{"hd": 1},
	}

	srv := New(cfg, lib, dlStore)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/rootfolder", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var folders []testRootFolder
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(folders) != 0 {
		t.Errorf("folders count = %d, want 0", len(folders))
	}
}

// Add Movie Tests

func TestAddMovie_CreatesContentInLibrary(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	body := `{
		"tmdbId": 12345,
		"title": "Test Movie",
		"year": 2024,
		"qualityProfileId": 1,
		"rootFolderPath": "/movies",
		"monitored": true,
		"addOptions": {"searchForMovie": false}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify content was created in database
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM content WHERE title = ?", "Test Movie").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("content count = %d, want 1", count)
	}

	// Verify TMDB ID was stored
	var tmdbID int64
	err = db.QueryRow("SELECT tmdb_id FROM content WHERE title = ?", "Test Movie").Scan(&tmdbID)
	if err != nil {
		t.Fatalf("query tmdb_id: %v", err)
	}
	if tmdbID != 12345 {
		t.Errorf("tmdb_id = %d, want 12345", tmdbID)
	}
}

func TestAddMovie_ReturnsRadarrFormatResponse(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	body := `{
		"tmdbId": 67890,
		"title": "Another Movie",
		"year": 2023,
		"qualityProfileId": 2,
		"rootFolderPath": "/movies",
		"monitored": true,
		"addOptions": {"searchForMovie": false}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp radarrMovieResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.ID == 0 {
		t.Error("response ID should not be 0")
	}
	if resp.TMDBID != 67890 {
		t.Errorf("tmdbId = %d, want 67890", resp.TMDBID)
	}
	if resp.Title != "Another Movie" {
		t.Errorf("title = %q, want %q", resp.Title, "Another Movie")
	}
	if resp.Year != 2023 {
		t.Errorf("year = %d, want 2023", resp.Year)
	}
	if !resp.Monitored {
		t.Error("monitored = false, want true")
	}
	if resp.Status != "released" {
		t.Errorf("status = %q, want %q", resp.Status, "released")
	}
}

func TestAddMovie_InvalidJSON(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	body := `{invalid json}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp testErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "Invalid request" {
		t.Errorf("error = %q, want %q", resp.Error, "Invalid request")
	}
}

func TestAddMovie_QualityProfileMappedCorrectly(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	// Add movie with qualityProfileId=2 which maps to "uhd"
	body := `{
		"tmdbId": 11111,
		"title": "UHD Movie",
		"year": 2024,
		"qualityProfileId": 2,
		"rootFolderPath": "/movies",
		"monitored": true,
		"addOptions": {"searchForMovie": false}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	// Verify quality profile was mapped correctly
	var qualityProfile string
	err := db.QueryRow("SELECT quality_profile FROM content WHERE title = ?", "UHD Movie").Scan(&qualityProfile)
	if err != nil {
		t.Fatalf("query quality_profile: %v", err)
	}
	if qualityProfile != "uhd" {
		t.Errorf("quality_profile = %q, want %q", qualityProfile, "uhd")
	}
}

// List Queue Tests

func TestListQueue_ReturnsDownloadsInRadarrFormat(t *testing.T) {
	srv, mux, db := setupServer(t, testAPIKey)
	lib := library.NewStore(db)

	// Create content first
	content := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       testMovieRoot,
	}
	if err := lib.AddContent(content); err != nil {
		t.Fatalf("add content: %v", err)
	}

	// Add a download
	dl := &download.Download{
		ContentID:   content.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.BluRay.x264",
		Indexer:     "NZBgeek",
	}
	if err := srv.downloads.Add(dl); err != nil {
		t.Fatalf("add download: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v3/queue", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp testQueueResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify queue response structure
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
	if resp.TotalRecords != 1 {
		t.Errorf("totalRecords = %d, want 1", resp.TotalRecords)
	}

	if len(resp.Records) != 1 {
		t.Errorf("records count = %d, want 1", len(resp.Records))
	}

	record := resp.Records[0]
	if record.Title != "Test.Movie.2024.1080p.BluRay.x264" {
		t.Errorf("title = %q, want %q", record.Title, "Test.Movie.2024.1080p.BluRay.x264")
	}
	if record.Indexer != "NZBgeek" {
		t.Errorf("indexer = %q, want %q", record.Indexer, "NZBgeek")
	}
}

func TestListQueue_EmptyQueueReturnsEmptyRecords(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/queue", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp testQueueResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Records) != 0 {
		t.Errorf("records count = %d, want 0", len(resp.Records))
	}
	if resp.TotalRecords != 0 {
		t.Errorf("totalRecords = %d, want 0", resp.TotalRecords)
	}
}

// List Movies Tests

func TestListMovies_ReturnsEmptyArray(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp []radarrMovieResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("response length = %d, want 0", len(resp))
	}
}

// Get Movie Tests

func TestGetMovie_NotFound(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie/999", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// Auth middleware: API key not configured (testing mode - auth skipped)

func TestAuthMiddleware_APIKeyNotConfigured_SkipsAuth(t *testing.T) {
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dlStore := download.NewStore(db)

	cfg := Config{
		APIKey:          "", // Empty API key = testing mode, auth skipped
		MovieRoot:       testMovieRoot,
		SeriesRoot:      testSeriesRoot,
		QualityProfiles: map[string]int{"hd": 1},
	}

	srv := New(cfg, lib, dlStore)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	// No API key header - should still work in testing mode
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should succeed (200 OK) when no API key configured
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
