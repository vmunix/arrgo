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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

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
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err, "open db")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(testSchema)
	require.NoError(t, err, "apply schema")
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

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp testErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Invalid API key", resp.Error)
}

func TestAuthMiddleware_WrongAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, "correct-key")

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", "wrong-key")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp testErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Invalid API key", resp.Error)
}

func TestAuthMiddleware_CorrectAPIKey(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_APIKeyQueryParam(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie?apikey="+testAPIKey, nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Quality Profiles Tests

func TestListQualityProfiles_ReturnsConfiguredProfiles(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/qualityprofile", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var profiles []testQualityProfile
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profiles))
	assert.Len(t, profiles, 2)
}

func TestListQualityProfiles_IncludesIDAndName(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/qualityprofile", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var profiles []testQualityProfile
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profiles))

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
	assert.True(t, foundHD, "hd profile with id=1 not found")
	assert.True(t, foundUHD, "uhd profile with id=2 not found")
}

// Root Folders Tests

func TestListRootFolders_ReturnsConfiguredRoots(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/rootfolder", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var folders []testRootFolder
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &folders))
	assert.Len(t, folders, 2)

	// Verify paths
	paths := make(map[string]bool)
	for _, folder := range folders {
		paths[folder.Path] = true
	}
	assert.True(t, paths[testMovieRoot], "%s root folder not found", testMovieRoot)
	assert.True(t, paths[testSeriesRoot], "%s root folder not found", testSeriesRoot)
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

	assert.Equal(t, http.StatusOK, w.Code)

	var folders []testRootFolder
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &folders))
	assert.Empty(t, folders)
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

	assert.Equal(t, http.StatusCreated, w.Code, "response body: %s", w.Body.String())

	// Verify content was created in database
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM content WHERE title = ?", "Test Movie").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify TMDB ID was stored
	var tmdbID int64
	err = db.QueryRow("SELECT tmdb_id FROM content WHERE title = ?", "Test Movie").Scan(&tmdbID)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), tmdbID)
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

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp radarrMovieResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.NotZero(t, resp.ID, "response ID should not be 0")
	assert.Equal(t, int64(67890), resp.TMDBID)
	assert.Equal(t, "Another Movie", resp.Title)
	assert.Equal(t, 2023, resp.Year)
	assert.True(t, resp.Monitored)
	assert.Equal(t, "released", resp.Status)
}

func TestAddMovie_InvalidJSON(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	body := `{invalid json}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp testErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Invalid request", resp.Error)
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

	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify quality profile was mapped correctly
	var qualityProfile string
	err := db.QueryRow("SELECT quality_profile FROM content WHERE title = ?", "UHD Movie").Scan(&qualityProfile)
	require.NoError(t, err)
	assert.Equal(t, "uhd", qualityProfile)
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
	require.NoError(t, lib.AddContent(content))

	// Add a download
	dl := &download.Download{
		ContentID:   content.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.BluRay.x264",
		Indexer:     "NZBgeek",
	}
	require.NoError(t, srv.downloads.Add(dl))

	req := httptest.NewRequest(http.MethodGet, "/api/v3/queue", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp testQueueResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Verify queue response structure
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 1, resp.TotalRecords)
	assert.Len(t, resp.Records, 1)

	record := resp.Records[0]
	assert.Equal(t, "Test.Movie.2024.1080p.BluRay.x264", record.Title)
	assert.Equal(t, "NZBgeek", record.Indexer)
}

func TestListQueue_EmptyQueueReturnsEmptyRecords(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/queue", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp testQueueResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Empty(t, resp.Records)
	assert.Zero(t, resp.TotalRecords)
}

// List Movies Tests

func TestListMovies_ReturnsEmptyArray(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []radarrMovieResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp)
}

// Get Movie Tests

func TestGetMovie_NotFound(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie/999", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
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
	assert.Equal(t, http.StatusOK, w.Code)
}
