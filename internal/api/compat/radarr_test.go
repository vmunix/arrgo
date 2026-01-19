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

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/library"
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

func setupServer(t *testing.T, apiKey string) (*Server, *http.ServeMux, *sql.DB) {
	t.Helper()
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dl := download.NewStore(db)

	cfg := Config{
		APIKey:          apiKey,
		MovieRoot:       "/movies",
		SeriesRoot:      "/series",
		QualityProfiles: map[string]int{"hd": 1, "uhd": 2},
	}

	srv := New(cfg, lib, dl)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	return srv, mux, db
}

// Auth Middleware Tests

func TestAuthMiddleware_NoAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "Invalid API key" {
		t.Errorf("error = %q, want %q", resp["error"], "Invalid API key")
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

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "Invalid API key" {
		t.Errorf("error = %q, want %q", resp["error"], "Invalid API key")
	}
}

func TestAuthMiddleware_CorrectAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_APIKeyQueryParam(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie?apikey=test-api-key", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// Quality Profiles Tests

func TestListQualityProfiles_ReturnsConfiguredProfiles(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/qualityprofile", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var profiles []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &profiles); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(profiles) != 2 {
		t.Errorf("profiles count = %d, want 2", len(profiles))
	}
}

func TestListQualityProfiles_IncludesIDAndName(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/qualityprofile", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var profiles []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &profiles); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check that each profile has id and name
	for _, profile := range profiles {
		if _, ok := profile["id"]; !ok {
			t.Error("profile missing 'id' field")
		}
		if _, ok := profile["name"]; !ok {
			t.Error("profile missing 'name' field")
		}
	}

	// Verify specific mappings exist
	foundHD := false
	foundUHD := false
	for _, profile := range profiles {
		name, _ := profile["name"].(string)
		id, _ := profile["id"].(float64)
		if name == "hd" && id == 1 {
			foundHD = true
		}
		if name == "uhd" && id == 2 {
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
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/rootfolder", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var folders []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(folders) != 2 {
		t.Errorf("folders count = %d, want 2", len(folders))
	}

	// Verify paths
	paths := make(map[string]bool)
	for _, folder := range folders {
		if path, ok := folder["path"].(string); ok {
			paths[path] = true
		}
	}
	if !paths["/movies"] {
		t.Error("/movies root folder not found")
	}
	if !paths["/series"] {
		t.Error("/series root folder not found")
	}
}

func TestListRootFolders_EmptyWhenNoRootsConfigured(t *testing.T) {
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dl := download.NewStore(db)

	cfg := Config{
		APIKey:          "test-api-key",
		MovieRoot:       "", // Empty roots
		SeriesRoot:      "",
		QualityProfiles: map[string]int{"hd": 1},
	}

	srv := New(cfg, lib, dl)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/rootfolder", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var folders []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(folders) != 0 {
		t.Errorf("folders count = %d, want 0", len(folders))
	}
}

// Add Movie Tests

func TestAddMovie_CreatesContentInLibrary(t *testing.T) {
	srv, mux, db := setupServer(t, "test-api-key")
	_ = srv // silence unused variable

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
	req.Header.Set("X-Api-Key", "test-api-key")
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
	_, mux, _ := setupServer(t, "test-api-key")

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
	req.Header.Set("X-Api-Key", "test-api-key")
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
	if resp.Monitored != true {
		t.Error("monitored = false, want true")
	}
	if resp.Status != "announced" {
		t.Errorf("status = %q, want %q", resp.Status, "announced")
	}
}

func TestAddMovie_InvalidJSON(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	body := `{invalid json}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(body))
	req.Header.Set("X-Api-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "Invalid request" {
		t.Errorf("error = %q, want %q", resp["error"], "Invalid request")
	}
}

func TestAddMovie_QualityProfileMappedCorrectly(t *testing.T) {
	_, mux, db := setupServer(t, "test-api-key")

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
	req.Header.Set("X-Api-Key", "test-api-key")
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
	srv, mux, db := setupServer(t, "test-api-key")
	lib := library.NewStore(db)

	// Create content first
	content := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
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
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify queue response structure
	if resp["page"] != float64(1) {
		t.Errorf("page = %v, want 1", resp["page"])
	}
	if resp["totalRecords"] != float64(1) {
		t.Errorf("totalRecords = %v, want 1", resp["totalRecords"])
	}

	records, ok := resp["records"].([]any)
	if !ok {
		t.Fatal("records is not an array")
	}
	if len(records) != 1 {
		t.Errorf("records count = %d, want 1", len(records))
	}

	record := records[0].(map[string]any)
	if record["title"] != "Test.Movie.2024.1080p.BluRay.x264" {
		t.Errorf("title = %v, want Test.Movie.2024.1080p.BluRay.x264", record["title"])
	}
	if record["indexer"] != "NZBgeek" {
		t.Errorf("indexer = %v, want NZBgeek", record["indexer"])
	}
}

func TestListQueue_EmptyQueueReturnsEmptyRecords(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/queue", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	records, ok := resp["records"].([]any)
	if !ok {
		t.Fatal("records is not an array")
	}
	if len(records) != 0 {
		t.Errorf("records count = %d, want 0", len(records))
	}
	if resp["totalRecords"] != float64(0) {
		t.Errorf("totalRecords = %v, want 0", resp["totalRecords"])
	}
}

// List Movies Tests

func TestListMovies_ReturnsEmptyArray(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp []any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("response length = %d, want 0", len(resp))
	}
}

// Get Movie Tests

func TestGetMovie_NotFound(t *testing.T) {
	_, mux, _ := setupServer(t, "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie/999", nil)
	req.Header.Set("X-Api-Key", "test-api-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// Auth middleware: API key not configured

func TestAuthMiddleware_APIKeyNotConfigured(t *testing.T) {
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dl := download.NewStore(db)

	cfg := Config{
		APIKey:          "", // Empty API key
		MovieRoot:       "/movies",
		SeriesRoot:      "/series",
		QualityProfiles: map[string]int{"hd": 1},
	}

	srv := New(cfg, lib, dl)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", "any-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "API key not configured" {
		t.Errorf("error = %q, want %q", resp["error"], "API key not configured")
	}
}
