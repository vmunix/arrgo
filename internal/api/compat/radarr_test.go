// internal/api/compat/radarr_test.go
package compat

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	_ "modernc.org/sqlite"

	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
	"github.com/vmunix/arrgo/internal/search/mocks"
	"github.com/vmunix/arrgo/internal/tmdb"
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
	Type      string `json:"type"`
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

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, lib, dl, testLogger)
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

	// Verify specific mappings exist (display names are used)
	foundHD := false
	foundUHD := false
	for _, profile := range profiles {
		if profile.Name == "HD-1080p" && profile.ID == 1 {
			foundHD = true
		}
		if profile.Name == "Ultra-HD" && profile.ID == 2 {
			foundUHD = true
		}
	}
	assert.True(t, foundHD, "HD-1080p profile with id=1 not found")
	assert.True(t, foundUHD, "Ultra-HD profile with id=2 not found")
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

	// Verify paths and types
	for _, folder := range folders {
		switch folder.Path {
		case testMovieRoot:
			assert.Equal(t, "movie", folder.Type, "movie root should have type=movie")
		case testSeriesRoot:
			assert.Equal(t, "series", folder.Type, "series root should have type=series")
		default:
			t.Errorf("unexpected root folder path: %s", folder.Path)
		}
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

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, lib, dlStore, testLogger)
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

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, lib, dlStore, testLogger)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/movie", nil)
	// No API key header - should still work in testing mode
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should succeed (200 OK) when no API key configured
	assert.Equal(t, http.StatusOK, w.Code)
}

// TMDB Lookup Tests

func TestLookupMovie_WithTMDB(t *testing.T) {
	// Mock TMDB server
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":           12345,
			"title":        "Test Movie",
			"overview":     "A great movie about testing.",
			"release_date": "2024-06-15",
			"poster_path":  "/test.jpg",
			"vote_average": 8.5,
			"vote_count":   1000,
			"runtime":      120,
			"genres":       []map[string]any{{"id": 28, "name": "Action"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tmdbServer.Close()

	// Create server with TMDB client
	db := setupTestDB(t)
	store := library.NewStore(db)
	dlStore := download.NewStore(db)
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(Config{APIKey: "test-key"}, store, dlStore, testLogger)

	tmdbClient := tmdb.NewClient("fake-key", tmdb.WithBaseURL(tmdbServer.URL))
	srv.SetTMDB(tmdbClient)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Make request
	req := httptest.NewRequest("GET", "/api/v3/movie/lookup?term=tmdb:12345", nil)
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var results []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	require.Len(t, results, 1)

	result := results[0]
	assert.EqualValues(t, 12345, result["tmdbId"])
	assert.Equal(t, "Test Movie", result["title"])
	assert.EqualValues(t, 2024, result["year"])
	assert.Equal(t, "A great movie about testing.", result["overview"])
	assert.EqualValues(t, 120, result["runtime"])

	// Check images
	images, ok := result["images"].([]any)
	require.True(t, ok)
	require.Len(t, images, 1)
	img := images[0].(map[string]any)
	assert.Equal(t, "poster", img["coverType"])
	assert.Contains(t, img["url"], "image.tmdb.org")

	// Check ratings
	ratings, ok := result["ratings"].(map[string]any)
	require.True(t, ok)
	tmdbRating := ratings["tmdb"].(map[string]any)
	assert.InDelta(t, 8.5, tmdbRating["value"], 0.001)
}

// Sonarr endpoint tests

func TestSonarrLookup(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	// Test lookup for non-existent series
	req := httptest.NewRequest(http.MethodGet, "/api/v3/series/lookup?term=tvdb:12345", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	require.Len(t, results, 1)
	assert.EqualValues(t, 12345, results[0]["tvdbId"])
	assert.Equal(t, "continuing", results[0]["status"])
}

func TestSonarrAddSeries(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	body := `{
		"tvdbId": 12345,
		"title": "Test Show",
		"year": 2024,
		"qualityProfileId": 1,
		"languageProfileId": 1,
		"rootFolderPath": "/series",
		"seriesType": "standard",
		"monitored": true,
		"seasons": [{"seasonNumber": 1, "monitored": true}],
		"addOptions": {"searchForMissingEpisodes": false}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/series", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "response body: %s", w.Body.String())

	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, "Test Show", result["title"])
	assert.EqualValues(t, 12345, result["tvdbId"])
	assert.NotNil(t, result["id"])

	// Verify content was created in database
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM content WHERE title = ?", "Test Show").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestLanguageProfiles(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/languageprofile", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "English", results[0]["name"])
	assert.EqualValues(t, 1, results[0]["id"])
}

func TestListSeries_Empty(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/series", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []sonarrSeriesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	assert.Empty(t, results)
}

func TestGetSeries_NotFound(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/series/999", nil)
	req.Header.Set("X-Api-Key", testAPIKey)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSonarrAddSeries_WithAutoSearch(t *testing.T) {
	// Set up database and stores
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dlStore := download.NewStore(db)

	// Set up event bus
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus(nil, testLogger)
	t.Cleanup(func() { bus.Close() })

	// Subscribe to GrabRequested events BEFORE setting up the server
	grabChan := bus.Subscribe(events.EventGrabRequested, 10)

	// Set up mock indexer
	ctrl := gomock.NewController(t)
	mockIndexer := mocks.NewMockIndexerAPI(ctrl)

	// Mock returns a season pack release
	mockIndexer.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, q search.Query) ([]search.Release, []error) {
			return []search.Release{{
				Title:       "Test.Show.S01.1080p.BluRay.x264",
				Indexer:     "TestIndexer",
				GUID:        "test-guid-123",
				DownloadURL: "https://indexer.test/download/123",
				Size:        5000000000,
				PublishDate: time.Now(),
			}}, nil
		}).
		AnyTimes()

	// Create scorer with minimal profile
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p", "720p"},
			Sources:    []string{"bluray", "web"},
			Codecs:     []string{"x264", "x265"},
		},
	}
	scorer := search.NewScorer(profiles)
	searcher := search.NewSearcher(mockIndexer, scorer, testLogger)

	// Create compat server with all dependencies
	cfg := Config{
		APIKey:          testAPIKey,
		MovieRoot:       testMovieRoot,
		SeriesRoot:      testSeriesRoot,
		QualityProfiles: map[string]int{"hd": 1, "uhd": 2},
	}
	srv := New(cfg, lib, dlStore, testLogger)
	srv.SetSearcher(searcher)
	srv.SetBus(bus)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Make addSeries request with auto-search enabled
	body := `{
		"tvdbId": 99999,
		"title": "Test Show",
		"year": 2024,
		"qualityProfileId": 1,
		"languageProfileId": 1,
		"rootFolderPath": "/series",
		"seriesType": "standard",
		"monitored": true,
		"seasons": [
			{"seasonNumber": 1, "monitored": true},
			{"seasonNumber": 2, "monitored": false}
		],
		"addOptions": {"searchForMissingEpisodes": true}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/series", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "response body: %s", w.Body.String())

	// Wait for GrabRequested event (with timeout)
	select {
	case evt := <-grabChan:
		grabEvt, ok := evt.(*events.GrabRequested)
		require.True(t, ok, "event should be GrabRequested")

		// Verify season info is set correctly
		require.NotNil(t, grabEvt.Season, "Season should be set for season pack grab")
		assert.Equal(t, 1, *grabEvt.Season, "Should grab season 1 (the monitored season)")
		assert.True(t, grabEvt.IsCompleteSeason, "IsCompleteSeason should be true for season pack")
		assert.Equal(t, "Test.Show.S01.1080p.BluRay.x264", grabEvt.ReleaseName)
		assert.Equal(t, "TestIndexer", grabEvt.Indexer)

	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for GrabRequested event")
	}
}

func TestSonarrAddSeries_WithAutoSearch_MultipleSeasons(t *testing.T) {
	// Set up database and stores
	db := setupTestDB(t)
	lib := library.NewStore(db)
	dlStore := download.NewStore(db)

	// Set up event bus
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus(nil, testLogger)
	t.Cleanup(func() { bus.Close() })

	// Subscribe to GrabRequested events
	grabChan := bus.Subscribe(events.EventGrabRequested, 10)

	// Set up mock indexer
	ctrl := gomock.NewController(t)
	mockIndexer := mocks.NewMockIndexerAPI(ctrl)

	// Track which seasons are searched for
	var searchedSeasons []int
	mockIndexer.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, q search.Query) ([]search.Release, []error) {
			if q.Season != nil {
				searchedSeasons = append(searchedSeasons, *q.Season)
			}
			seasonNum := 1
			if q.Season != nil {
				seasonNum = *q.Season
			}
			// Use matching title - the Searcher filters releases by title match
			return []search.Release{{
				Title:       "Multi.Season.Show.S0" + string(rune('0'+seasonNum)) + ".COMPLETE.1080p.BluRay.x264",
				Indexer:     "TestIndexer",
				GUID:        "test-guid-" + string(rune('0'+seasonNum)),
				DownloadURL: "https://indexer.test/download/" + string(rune('0'+seasonNum)),
				Size:        5000000000,
				PublishDate: time.Now(),
			}}, nil
		}).
		AnyTimes()

	// Create scorer and searcher
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}, Sources: []string{"bluray"}},
	}
	scorer := search.NewScorer(profiles)
	searcher := search.NewSearcher(mockIndexer, scorer, testLogger)

	// Create server with WaitGroup for test synchronization
	var pendingTasks sync.WaitGroup
	cfg := Config{
		APIKey:          testAPIKey,
		SeriesRoot:      testSeriesRoot,
		QualityProfiles: map[string]int{"hd": 1},
	}
	srv := New(cfg, lib, dlStore, testLogger)
	srv.SetSearcher(searcher)
	srv.SetBus(bus)
	srv.SetPendingWaitGroup(&pendingTasks)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Request with multiple monitored seasons
	body := `{
		"tvdbId": 88888,
		"title": "Multi Season Show",
		"year": 2020,
		"qualityProfileId": 1,
		"languageProfileId": 1,
		"rootFolderPath": "/series",
		"seriesType": "standard",
		"monitored": true,
		"seasons": [
			{"seasonNumber": 1, "monitored": true},
			{"seasonNumber": 2, "monitored": true},
			{"seasonNumber": 3, "monitored": false}
		],
		"addOptions": {"searchForMissingEpisodes": true}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v3/series", strings.NewReader(body))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Wait for the background goroutine to complete
	pendingTasks.Wait()

	// Now read the events - they should all be in the channel
	receivedSeasons := make(map[int]bool)
	timeout := time.After(1 * time.Second)

	for len(receivedSeasons) < 2 {
		select {
		case evt := <-grabChan:
			grabEvt, ok := evt.(*events.GrabRequested)
			require.True(t, ok)
			require.NotNil(t, grabEvt.Season)
			receivedSeasons[*grabEvt.Season] = true
			assert.True(t, grabEvt.IsCompleteSeason)
		case <-timeout:
			t.Fatalf("timed out waiting for events, got seasons: %v", receivedSeasons)
		}
	}

	// Should have received grabs for seasons 1 and 2, not 3
	assert.True(t, receivedSeasons[1], "should grab season 1")
	assert.True(t, receivedSeasons[2], "should grab season 2")
	assert.False(t, receivedSeasons[3], "should NOT grab season 3 (not monitored)")
}
