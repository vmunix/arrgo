package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vmunix/arrgo/pkg/tvdb"
)

// mockTVDBServer creates a test server that simulates the TVDB API.
func mockTVDBServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for handler by path
		if handler, ok := handlers[r.URL.Path]; ok {
			handler(w, r)
			return
		}
		// Default: 404
		w.WriteHeader(http.StatusNotFound)
	}))
}

// writeJSONResponse is a test helper that writes JSON response.
func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("test: failed to encode JSON: " + err.Error())
	}
}

// tvdbLoginHandler returns a handler that validates API key and returns a token.
func tvdbLoginHandler(validAPIKey, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			APIKey string `json:"apikey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if body.APIKey != validAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		writeJSONResponse(w, map[string]any{
			"status": "success",
			"data":   map[string]string{"token": token},
		})
	}
}

// tvdbRequireAuth wraps a handler with token validation.
func tvdbRequireAuth(validToken string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}

func TestTVDBService_Search_CacheMiss(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/search": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			assert.Equal(t, "Breaking Bad", r.URL.Query().Get("query"))

			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data": []map[string]any{
					{
						"objectID": "series-81189",
						"name":     "Breaking Bad",
						"year":     "2008",
						"status":   "Ended",
						"network":  "AMC",
						"tvdb_id":  "81189",
					},
				},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// First call - should call API
	results, err := svc.Search(ctx, "Breaking Bad")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 81189, results[0].ID)
	assert.Equal(t, "Breaking Bad", results[0].Name)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should have been called once")

	// Second call - should use cache
	results2, err := svc.Search(ctx, "Breaking Bad")
	require.NoError(t, err)
	require.Len(t, results2, 1)
	assert.Equal(t, 81189, results2[0].ID)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should NOT have been called again")
}

func TestTVDBService_Search_CacheHit(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/search": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data":   []map[string]any{},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Pre-populate the cache
	cachedResults := []tvdb.SearchResult{
		{ID: 12345, Name: "Cached Show", Year: 2020},
	}
	data, _ := json.Marshal(cachedResults)
	err := cache.Set(ctx, "tvdb:search:test query", data, time.Hour)
	require.NoError(t, err)

	// Call should hit cache
	results, err := svc.Search(ctx, "test query")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 12345, results[0].ID)
	assert.Equal(t, "Cached Show", results[0].Name)
	assert.Equal(t, int32(0), apiCallCount.Load(), "API should NOT have been called")
}

func TestTVDBService_GetSeries_CacheMiss(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/81189": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":         81189,
					"name":       "Breaking Bad",
					"status":     map[string]string{"name": "Ended"},
					"overview":   "A high school chemistry teacher...",
					"firstAired": "2008-01-20",
				},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// First call - should call API
	series, err := svc.GetSeries(ctx, 81189)
	require.NoError(t, err)
	assert.Equal(t, 81189, series.ID)
	assert.Equal(t, "Breaking Bad", series.Name)
	assert.Equal(t, 2008, series.Year)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should have been called once")

	// Second call - should use cache
	series2, err := svc.GetSeries(ctx, 81189)
	require.NoError(t, err)
	assert.Equal(t, 81189, series2.ID)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should NOT have been called again")
}

func TestTVDBService_GetSeries_CacheHit(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/12345": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":   12345,
					"name": "Should Not See This",
				},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Pre-populate the cache
	cachedSeries := tvdb.Series{ID: 12345, Name: "Cached Series", Year: 2021}
	data, _ := json.Marshal(cachedSeries)
	err := cache.Set(ctx, "tvdb:series:12345", data, 7*24*time.Hour)
	require.NoError(t, err)

	// Call should hit cache
	series, err := svc.GetSeries(ctx, 12345)
	require.NoError(t, err)
	assert.Equal(t, 12345, series.ID)
	assert.Equal(t, "Cached Series", series.Name)
	assert.Equal(t, 2021, series.Year)
	assert.Equal(t, int32(0), apiCallCount.Load(), "API should NOT have been called")
}

func TestTVDBService_GetEpisodes_CacheMiss(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/81189/episodes/default": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"episodes": []map[string]any{
						{
							"id":           349232,
							"seasonNumber": 1,
							"number":       1,
							"name":         "Pilot",
							"aired":        "2008-01-20",
							"runtime":      58,
						},
						{
							"id":           349233,
							"seasonNumber": 1,
							"number":       2,
							"name":         "Cat's in the Bag...",
							"aired":        "2008-01-27",
							"runtime":      48,
						},
					},
				},
				"links": map[string]string{"next": ""},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// First call - should call API
	episodes, err := svc.GetEpisodes(ctx, 81189)
	require.NoError(t, err)
	require.Len(t, episodes, 2)
	assert.Equal(t, "Pilot", episodes[0].Name)
	assert.Equal(t, 1, episodes[0].Season)
	assert.Equal(t, 1, episodes[0].Episode)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should have been called once")

	// Second call - should use cache
	episodes2, err := svc.GetEpisodes(ctx, 81189)
	require.NoError(t, err)
	require.Len(t, episodes2, 2)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should NOT have been called again")
}

func TestTVDBService_GetEpisodes_CacheHit(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/12345/episodes/default": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data":   map[string]any{"episodes": []map[string]any{}},
				"links":  map[string]string{"next": ""},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Pre-populate the cache
	cachedEpisodes := []tvdb.Episode{
		{ID: 1, Season: 1, Episode: 1, Name: "Cached Pilot"},
		{ID: 2, Season: 1, Episode: 2, Name: "Cached Episode 2"},
	}
	data, _ := json.Marshal(cachedEpisodes)
	err := cache.Set(ctx, "tvdb:episodes:12345", data, 24*time.Hour)
	require.NoError(t, err)

	// Call should hit cache
	episodes, err := svc.GetEpisodes(ctx, 12345)
	require.NoError(t, err)
	require.Len(t, episodes, 2)
	assert.Equal(t, "Cached Pilot", episodes[0].Name)
	assert.Equal(t, "Cached Episode 2", episodes[1].Name)
	assert.Equal(t, int32(0), apiCallCount.Load(), "API should NOT have been called")
}

func TestTVDBService_InvalidateSeries(t *testing.T) {
	const token = "test-token"

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/12345": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":         12345,
					"name":       "Fresh Data",
					"firstAired": "2020-01-01",
				},
			})
		}),
		"/series/12345/episodes/default": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data":   map[string]any{"episodes": []map[string]any{{
					"id":           1,
					"seasonNumber": 1,
					"number":       1,
					"name":         "Fresh Episode",
				}}},
				"links": map[string]string{"next": ""},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Pre-populate cache with old data
	oldSeries := tvdb.Series{ID: 12345, Name: "Old Cached Data"}
	oldEpisodes := []tvdb.Episode{{ID: 1, Name: "Old Episode"}}
	seriesData, _ := json.Marshal(oldSeries)
	episodesData, _ := json.Marshal(oldEpisodes)
	require.NoError(t, cache.Set(ctx, "tvdb:series:12345", seriesData, 7*24*time.Hour))
	require.NoError(t, cache.Set(ctx, "tvdb:episodes:12345", episodesData, 24*time.Hour))

	// Verify cache has old data
	series, err := svc.GetSeries(ctx, 12345)
	require.NoError(t, err)
	assert.Equal(t, "Old Cached Data", series.Name)

	episodes, err := svc.GetEpisodes(ctx, 12345)
	require.NoError(t, err)
	assert.Equal(t, "Old Episode", episodes[0].Name)

	// Invalidate
	err = svc.InvalidateSeries(ctx, 12345)
	require.NoError(t, err)

	// Now calls should fetch fresh data from API
	series, err = svc.GetSeries(ctx, 12345)
	require.NoError(t, err)
	assert.Equal(t, "Fresh Data", series.Name)

	episodes, err = svc.GetEpisodes(ctx, 12345)
	require.NoError(t, err)
	assert.Equal(t, "Fresh Episode", episodes[0].Name)
}

func TestTVDBService_Search_APIError(t *testing.T) {
	const token = "test-token"

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/search": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	_, err := svc.Search(ctx, "test")
	require.Error(t, err)
	assert.ErrorIs(t, err, tvdb.ErrRateLimited)
}

func TestTVDBService_GetSeries_NotFound(t *testing.T) {
	const token = "test-token"

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/99999": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	_, err := svc.GetSeries(ctx, 99999)
	require.Error(t, err)
	assert.ErrorIs(t, err, tvdb.ErrNotFound)
}

func TestTVDBService_GetEpisodes_NotFound(t *testing.T) {
	const token = "test-token"

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/99999/episodes/default": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	_, err := svc.GetEpisodes(ctx, 99999)
	require.Error(t, err)
	assert.ErrorIs(t, err, tvdb.ErrNotFound)
}

func TestTVDBService_InvalidateSeries_NonExistent(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	// Use a dummy client - won't be called since we're just testing invalidation
	client := tvdb.New("api-key")
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Invalidating non-existent cache entries should not error
	err := svc.InvalidateSeries(ctx, 99999)
	assert.NoError(t, err)
}

func TestTVDBService_CacheCorruptedData(t *testing.T) {
	const token = "test-token"
	var apiCallCount atomic.Int32

	server := mockTVDBServer(t, map[string]http.HandlerFunc{
		"/login": tvdbLoginHandler("api-key", token),
		"/series/12345": tvdbRequireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			writeJSONResponse(w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":         12345,
					"name":       "Fresh Series",
					"firstAired": "2020-01-01",
				},
			})
		}),
	})
	defer server.Close()

	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("api-key", tvdb.WithBaseURL(server.URL))
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Store corrupted JSON in cache
	err := cache.Set(ctx, "tvdb:series:12345", []byte("not valid json{{{"), 7*24*time.Hour)
	require.NoError(t, err)

	// Should detect corruption and fetch fresh data
	series, err := svc.GetSeries(ctx, 12345)
	require.NoError(t, err)
	assert.Equal(t, "Fresh Series", series.Name)
	assert.Equal(t, int32(1), apiCallCount.Load(), "API should have been called due to corrupted cache")
}

func TestNewTVDBService(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	client := tvdb.New("test-key")

	svc := NewTVDBService(client, cache, nil)

	assert.NotNil(t, svc)
	assert.NotNil(t, svc.client)
	assert.NotNil(t, svc.cache)
	assert.Nil(t, svc.log)
}
