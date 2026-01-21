package compat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOverseerrSeriesFlow simulates the exact API flow Overseerr uses
// when a user requests a TV series season.
//
// Flow:
//  1. GET /series/lookup?term=tvdb:{id} - Check if series exists
//  2. POST /series (new) or PUT /series (existing) - Add/update series
//  3. Verify search is triggered with correct season
//
// This test uses exact payloads captured from Overseerr source code.
func TestOverseerrSeriesFlow_NewSeries(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	// Step 1: Overseerr looks up series by TVDB ID
	// Expects: Empty array or array with no ID (triggers POST)
	t.Run("lookup_new_series", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v3/series/lookup?term=tvdb:71470", nil)
		req.Header.Set("X-Api-Key", testAPIKey)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var results []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))

		// For new series, should return stub without ID
		require.Len(t, results, 1, "should return one stub result")
		_, hasID := results[0]["id"]
		assert.False(t, hasID, "new series lookup should not have id field")
		assert.EqualValues(t, 71470, results[0]["tvdbId"])
	})

	// Step 2: Overseerr sends POST to add new series
	// This is the exact payload format from Overseerr source
	t.Run("add_series_season1", func(t *testing.T) {
		payload := `{
			"tvdbId": 71470,
			"title": "Star Trek: The Next Generation",
			"qualityProfileId": 1,
			"languageProfileId": 1,
			"seasons": [
				{"seasonNumber": 0, "monitored": false},
				{"seasonNumber": 1, "monitored": true},
				{"seasonNumber": 2, "monitored": false},
				{"seasonNumber": 3, "monitored": false},
				{"seasonNumber": 4, "monitored": false},
				{"seasonNumber": 5, "monitored": false},
				{"seasonNumber": 6, "monitored": false},
				{"seasonNumber": 7, "monitored": false}
			],
			"tags": [],
			"seasonFolder": true,
			"monitored": true,
			"rootFolderPath": "/tv",
			"seriesType": "standard",
			"addOptions": {
				"ignoreEpisodesWithFiles": true,
				"searchForMissingEpisodes": true
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/api/v3/series", strings.NewReader(payload))
		req.Header.Set("X-Api-Key", testAPIKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code, "response: %s", w.Body.String())

		var result map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

		// Verify response has required fields for Overseerr
		assert.Equal(t, "Star Trek: The Next Generation", result["title"])
		assert.EqualValues(t, 71470, result["tvdbId"])
		assert.NotNil(t, result["id"], "response must have id for Overseerr to track")
		assert.NotEmpty(t, result["titleSlug"], "response must have titleSlug")

		// Verify content was created in database
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM content WHERE title = ?", "Star Trek: The Next Generation").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "series should be added to database")

		// Verify it's marked as wanted (for search)
		var status string
		err = db.QueryRow("SELECT status FROM content WHERE title = ?", "Star Trek: The Next Generation").Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "wanted", status)
	})
}

// TestOverseerrSeriesFlow_ExistingSeries tests the re-request flow
// when a series already exists in the library (status: wanted).
func TestOverseerrSeriesFlow_ExistingSeries(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	// Pre-populate with existing series
	_, err := db.Exec(`
		INSERT INTO content (id, type, tvdb_id, title, year, status, quality_profile, root_path)
		VALUES (100, 'series', 71470, 'Star Trek: The Next Generation', 1987, 'wanted', 'hd', '/tv')
	`)
	require.NoError(t, err)

	// Step 1: Lookup existing series
	t.Run("lookup_existing_series", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v3/series/lookup?term=tvdb:71470", nil)
		req.Header.Set("X-Api-Key", testAPIKey)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var results []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))

		require.Len(t, results, 1)
		// Existing series should have ID
		assert.EqualValues(t, 100, results[0]["id"], "existing series should have id")
		assert.EqualValues(t, 71470, results[0]["tvdbId"])
		// For wanted items, monitored should be false to trigger PUT
		assert.Equal(t, false, results[0]["monitored"], "wanted series should return monitored=false")
	})

	// Step 2: Overseerr sends PUT to update series (re-request flow)
	t.Run("update_series_add_season", func(t *testing.T) {
		payload := `{
			"id": 100,
			"monitored": true,
			"seasons": [
				{"seasonNumber": 1, "monitored": true},
				{"seasonNumber": 2, "monitored": true}
			],
			"tags": [],
			"addOptions": {
				"searchForMissingEpisodes": true
			}
		}`

		req := httptest.NewRequest(http.MethodPut, "/api/v3/series", strings.NewReader(payload))
		req.Header.Set("X-Api-Key", testAPIKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "response: %s", w.Body.String())

		var result map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

		assert.EqualValues(t, 100, result["id"])
		assert.Equal(t, "Star Trek: The Next Generation", result["title"])
	})
}

// TestOverseerrMovieFlow simulates the Radarr API flow for movies.
func TestOverseerrMovieFlow_NewMovie(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	// Step 1: Lookup movie by TMDB ID
	t.Run("lookup_new_movie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v3/movie/lookup?term=tmdb:533535", nil)
		req.Header.Set("X-Api-Key", testAPIKey)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var results []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))

		require.Len(t, results, 1)
		_, hasID := results[0]["id"]
		assert.False(t, hasID, "new movie lookup should not have id")
	})

	// Step 2: Add new movie
	t.Run("add_movie", func(t *testing.T) {
		payload := `{
			"tmdbId": 533535,
			"title": "Deadpool & Wolverine",
			"year": 2024,
			"qualityProfileId": 1,
			"rootFolderPath": "/movies",
			"monitored": true,
			"tags": [],
			"addOptions": {
				"searchForMovie": true
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/api/v3/movie", strings.NewReader(payload))
		req.Header.Set("X-Api-Key", testAPIKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code, "response: %s", w.Body.String())

		var result map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

		assert.Equal(t, "Deadpool & Wolverine", result["title"])
		assert.EqualValues(t, 533535, result["tmdbId"])
		assert.NotNil(t, result["id"])

		// Verify in database
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM content WHERE tmdb_id = 533535").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

// TestOverseerrMovieFlow_ExistingWanted tests re-request of wanted movie.
func TestOverseerrMovieFlow_ExistingWanted(t *testing.T) {
	_, mux, db := setupServer(t, testAPIKey)

	// Pre-populate with wanted movie
	_, err := db.Exec(`
		INSERT INTO content (id, type, tmdb_id, title, year, status, quality_profile, root_path)
		VALUES (200, 'movie', 533535, 'Deadpool & Wolverine', 2024, 'wanted', 'hd', '/movies')
	`)
	require.NoError(t, err)

	// Lookup should return monitored=false for wanted items
	t.Run("lookup_wanted_movie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v3/movie/lookup?term=tmdb:533535", nil)
		req.Header.Set("X-Api-Key", testAPIKey)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		var results []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))

		require.Len(t, results, 1)
		assert.EqualValues(t, 200, results[0]["id"])
		assert.Equal(t, false, results[0]["monitored"], "wanted movie should return monitored=false")
	})

	// PUT should trigger search
	t.Run("update_wanted_movie", func(t *testing.T) {
		payload := `{
			"id": 200,
			"monitored": true,
			"tags": [],
			"addOptions": {
				"searchForMovie": true
			}
		}`

		req := httptest.NewRequest(http.MethodPut, "/api/v3/movie", strings.NewReader(payload))
		req.Header.Set("X-Api-Key", testAPIKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "response: %s", w.Body.String())
	})
}

// TestOverseerrRequiredEndpoints verifies all endpoints Overseerr needs are present.
func TestOverseerrRequiredEndpoints(t *testing.T) {
	_, mux, _ := setupServer(t, testAPIKey)

	endpoints := []struct {
		method string
		path   string
		status int
	}{
		// Radarr endpoints
		{"GET", "/api/v3/movie", 200},
		{"GET", "/api/v3/movie/lookup?term=tmdb:123", 200},
		{"GET", "/api/v3/qualityProfile", 200},
		{"GET", "/api/v3/rootfolder", 200},
		{"GET", "/api/v3/tag", 200},

		// Sonarr endpoints
		{"GET", "/api/v3/series", 200},
		{"GET", "/api/v3/series/lookup?term=tvdb:123", 200},
		{"GET", "/api/v3/languageprofile", 200},

		// Command endpoint
		{"POST", "/api/v3/command", 200},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.method == "POST" {
				req = httptest.NewRequest(ep.method, ep.path, strings.NewReader(`{"name":"test"}`))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			req.Header.Set("X-Api-Key", testAPIKey)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, ep.status, w.Code, "endpoint %s %s should return %d, got %d: %s",
				ep.method, ep.path, ep.status, w.Code, w.Body.String())
		})
	}
}
