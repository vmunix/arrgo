# TMDB Integration Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add TMDB API client to enrich Overseerr compat responses with real movie metadata.

**Architecture:** New `internal/tmdb` package with HTTP client, response types, and TTL cache. Wired into compat API's `lookupMovie` handler. Graceful degradation if TMDB unavailable.

**Tech Stack:** Go stdlib `net/http`, `encoding/json`, `sync` for cache mutex, `httptest` for testing.

---

## Task 1: Add TMDB Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.toml`

**Step 1: Add TMDBConfig struct to config.go**

Add after `ImporterConfig` (around line 136):

```go
type TMDBConfig struct {
	APIKey string `toml:"api_key"`
}
```

Add field to main `Config` struct (after `Importer`):

```go
TMDB *TMDBConfig `toml:"tmdb"`
```

**Step 2: Add example config to config.example.toml**

Add after `[importer]` section:

```toml
# TMDB metadata (enriches Overseerr responses)
# Get free API key at https://www.themoviedb.org/settings/api
[tmdb]
api_key = "${TMDB_API_KEY}"
```

**Step 3: Verify config loads**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add internal/config/config.go config.example.toml
git commit -m "config: add TMDB API key configuration"
```

---

## Task 2: Create TMDB Types

**Files:**
- Create: `internal/tmdb/types.go`

**Step 1: Create types file**

```go
// Package tmdb provides a client for The Movie Database API.
package tmdb

// Movie represents TMDB movie metadata.
type Movie struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Overview    string  `json:"overview"`
	ReleaseDate string  `json:"release_date"` // "2024-03-01"
	PosterPath  string  `json:"poster_path"`  // "/abc123.jpg"
	BackdropPath string `json:"backdrop_path"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
	Runtime     int     `json:"runtime"` // minutes
	Genres      []Genre `json:"genres"`
}

// Genre represents a movie genre.
type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Year extracts the year from ReleaseDate.
func (m *Movie) Year() int {
	if len(m.ReleaseDate) < 4 {
		return 0
	}
	var year int
	fmt.Sscanf(m.ReleaseDate[:4], "%d", &year)
	return year
}

// PosterURL returns the full poster image URL.
// Size can be: w92, w154, w185, w342, w500, w780, original
func (m *Movie) PosterURL(size string) string {
	if m.PosterPath == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/" + size + m.PosterPath
}
```

**Step 2: Add fmt import**

Add to imports: `"fmt"`

**Step 3: Verify it compiles**

Run: `go build ./internal/tmdb/...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add internal/tmdb/types.go
git commit -m "tmdb: add response types"
```

---

## Task 3: Create TMDB Cache

**Files:**
- Create: `internal/tmdb/cache.go`
- Create: `internal/tmdb/cache_test.go`

**Step 1: Write the failing test**

```go
package tmdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetSet(t *testing.T) {
	c := newCache(time.Hour)

	// Miss
	_, ok := c.get(12345)
	assert.False(t, ok, "empty cache should miss")

	// Set and hit
	movie := &Movie{ID: 12345, Title: "Test Movie"}
	c.set(12345, movie)

	got, ok := c.get(12345)
	require.True(t, ok, "should hit after set")
	assert.Equal(t, "Test Movie", got.Title)
}

func TestCache_Expiry(t *testing.T) {
	c := newCache(10 * time.Millisecond)

	c.set(12345, &Movie{ID: 12345, Title: "Test"})

	// Should hit immediately
	_, ok := c.get(12345)
	require.True(t, ok)

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	// Should miss after expiry
	_, ok = c.get(12345)
	assert.False(t, ok, "should miss after TTL")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmdb/... -v -run TestCache`
Expected: FAIL (newCache not defined)

**Step 3: Write the implementation**

```go
package tmdb

import (
	"sync"
	"time"
)

type cacheEntry struct {
	movie   *Movie
	expires time.Time
}

type cache struct {
	mu      sync.RWMutex
	entries map[int64]cacheEntry
	ttl     time.Duration
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		entries: make(map[int64]cacheEntry),
		ttl:     ttl,
	}
}

func (c *cache) get(tmdbID int64) (*Movie, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[tmdbID]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.movie, true
}

func (c *cache) set(tmdbID int64, movie *Movie) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[tmdbID] = cacheEntry{
		movie:   movie,
		expires: time.Now().Add(c.ttl),
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tmdb/... -v -run TestCache`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmdb/cache.go internal/tmdb/cache_test.go
git commit -m "tmdb: add TTL cache"
```

---

## Task 4: Create TMDB Client

**Files:**
- Create: `internal/tmdb/client.go`
- Create: `internal/tmdb/client_test.go`

**Step 1: Write the failing test**

```go
package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetMovie(t *testing.T) {
	// Mock TMDB API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/3/movie/550", r.URL.Path)
		assert.Equal(t, "test-key", r.URL.Query().Get("api_key"))

		resp := Movie{
			ID:          550,
			Title:       "Fight Club",
			Overview:    "A ticking-Loss insomnia cult favorite...",
			ReleaseDate: "1999-10-15",
			PosterPath:  "/pB8BM7pdSp6B6Ih7QZ4DrQ3PmJK.jpg",
			VoteAverage: 8.4,
			Runtime:     139,
			Genres:      []Genre{{ID: 18, Name: "Drama"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	movie, err := client.GetMovie(context.Background(), 550)
	require.NoError(t, err)
	assert.Equal(t, int64(550), movie.ID)
	assert.Equal(t, "Fight Club", movie.Title)
	assert.Equal(t, 1999, movie.Year())
	assert.Equal(t, 139, movie.Runtime)
}

func TestClient_GetMovie_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	movie, err := client.GetMovie(context.Background(), 99999999)
	assert.Nil(t, movie)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestClient_GetMovie_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := Movie{ID: 550, Title: "Fight Club"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithCacheTTL(time.Hour))

	// First call hits API
	_, err := client.GetMovie(context.Background(), 550)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call uses cache
	_, err = client.GetMovie(context.Background(), 550)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "should use cache, not call API again")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmdb/... -v -run TestClient`
Expected: FAIL (NewClient not defined)

**Step 3: Write the implementation**

```go
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.themoviedb.org"
const defaultCacheTTL = 24 * time.Hour

// ErrNotFound is returned when a movie doesn't exist in TMDB.
var ErrNotFound = errors.New("movie not found")

// Client is a TMDB API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	cache      *cache
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom base URL (for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithCacheTTL sets the cache TTL.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.cache = newCache(ttl)
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// NewClient creates a new TMDB client.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: newCache(defaultCacheTTL),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetMovie fetches movie metadata by TMDB ID.
func (c *Client) GetMovie(ctx context.Context, tmdbID int64) (*Movie, error) {
	// Check cache first
	if movie, ok := c.cache.get(tmdbID); ok {
		return movie, nil
	}

	// Build request
	url := fmt.Sprintf("%s/3/movie/%d?api_key=%s", c.baseURL, tmdbID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Execute
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Handle errors
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: %s", resp.Status)
	}

	// Decode
	var movie Movie
	if err := json.NewDecoder(resp.Body).Decode(&movie); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Cache and return
	c.cache.set(tmdbID, &movie)
	return &movie, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tmdb/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmdb/client.go internal/tmdb/client_test.go
git commit -m "tmdb: add API client with caching"
```

---

## Task 5: Wire TMDB into Compat API

**Files:**
- Modify: `internal/api/compat/compat.go`
- Modify: `cmd/arrgod/server.go`

**Step 1: Add TMDB client to compat.Server**

In `internal/api/compat/compat.go`, add import:

```go
"github.com/vmunix/arrgo/internal/tmdb"
```

Add field to Server struct (after `manager`):

```go
tmdb *tmdb.Client
```

Add setter method (after `SetManager`):

```go
// SetTMDB configures the TMDB client (optional).
func (s *Server) SetTMDB(client *tmdb.Client) {
	s.tmdb = client
}
```

**Step 2: Update lookupMovie to use TMDB**

Replace the stub response section in `lookupMovie` (the map starting with "Not in library" comment) with:

```go
	// Not in library - fetch metadata from TMDB if available
	response := map[string]any{
		"tmdbId":      tmdbID,
		"title":       "",
		"year":        0,
		"monitored":   false,
		"hasFile":     false,
		"isAvailable": false,
	}

	// Enrich with TMDB metadata if client configured
	if s.tmdb != nil {
		movie, err := s.tmdb.GetMovie(r.Context(), tmdbID)
		if err == nil {
			response["title"] = movie.Title
			response["year"] = movie.Year()
			response["overview"] = movie.Overview
			response["runtime"] = movie.Runtime
			if movie.PosterPath != "" {
				response["images"] = []map[string]any{{
					"coverType": "poster",
					"url":       movie.PosterURL("w500"),
				}}
			}
			if movie.VoteAverage > 0 {
				response["ratings"] = map[string]any{
					"tmdb": map[string]any{
						"value": movie.VoteAverage,
						"votes": movie.VoteCount,
					},
				}
			}
			if len(movie.Genres) > 0 {
				response["genres"] = movie.Genres
			}
		}
		// On error, continue with stub - graceful degradation
	}

	writeJSON(w, http.StatusOK, []map[string]any{response})
```

**Step 3: Wire TMDB client in server.go**

In `cmd/arrgod/server.go`, add import:

```go
"github.com/vmunix/arrgo/internal/tmdb"
```

After creating `apiCompat` (around line 230), add:

```go
		// Wire TMDB client if configured
		if cfg.TMDB != nil && cfg.TMDB.APIKey != "" {
			tmdbClient := tmdb.NewClient(cfg.TMDB.APIKey)
			apiCompat.SetTMDB(tmdbClient)
			logger.Info("TMDB client configured")
		}
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add internal/api/compat/compat.go cmd/arrgod/server.go
git commit -m "compat: wire TMDB client for enriched lookupMovie responses"
```

---

## Task 6: Add Integration Test

**Files:**
- Modify: `internal/api/compat/radarr_test.go`

**Step 1: Write test for enriched lookup**

Add to `radarr_test.go`:

```go
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
		json.NewEncoder(w).Encode(resp)
	}))
	defer tmdbServer.Close()

	// Create server with TMDB client
	db := setupTestDB(t)
	store := library.NewStore(db)
	dlStore := download.NewStore(db)
	srv := compat.New(compat.Config{APIKey: "test-key"}, store, dlStore)

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
	assert.Equal(t, float64(12345), result["tmdbId"])
	assert.Equal(t, "Test Movie", result["title"])
	assert.Equal(t, float64(2024), result["year"])
	assert.Equal(t, "A great movie about testing.", result["overview"])
	assert.Equal(t, float64(120), result["runtime"])

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
	assert.Equal(t, 8.5, tmdbRating["value"])
}
```

**Step 2: Add import**

Add to imports in `radarr_test.go`:

```go
"github.com/vmunix/arrgo/internal/tmdb"
```

**Step 3: Run the test**

Run: `go test ./internal/api/compat/... -v -run TestLookupMovie_WithTMDB`
Expected: PASS

**Step 4: Run all tests**

Run: `task test`
Expected: All tests pass

**Step 5: Commit**

```bash
git add internal/api/compat/radarr_test.go
git commit -m "test: add integration test for TMDB-enriched lookupMovie"
```

---

## Task 7: Final Verification

**Step 1: Run linter**

Run: `task lint`
Expected: No errors

**Step 2: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 3: Build binaries**

Run: `task build`
Expected: Build succeeds

**Step 4: Manual test (optional)**

If you have a TMDB API key:

```bash
export TMDB_API_KEY="your-key"
# Add [tmdb] section to config.toml
./arrgod &
curl -H "X-Api-Key: your-arrgo-key" "http://localhost:8484/api/v3/movie/lookup?term=tmdb:550"
```

Expected: Response includes title "Fight Club", year 1999, overview, poster URL, ratings.

**Step 5: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "tmdb: final polish and fixes"
```

---

## Summary

After completing all tasks:
- `internal/tmdb/` package with Client, types, and cache
- Config support for `[tmdb]` section
- Compat API enriches `/api/v3/movie/lookup` with real metadata
- Graceful degradation if TMDB unavailable
- Full test coverage
