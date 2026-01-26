# TVDB Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Status:** Complete

**Goal:** Add TVDB API integration for TV series metadata, enabling accurate episode counts and year data.

**Architecture:** Pure API client in `pkg/tvdb/`, SQLite-backed cache in `internal/metadata/`, integration via compat API and CLI.

**Tech Stack:** Go 1.24, SQLite, TVDB API v4 (JWT auth)

---

## Task 1: TVDB API Client Types

**Files:**
- Create: `pkg/tvdb/types.go`
- Test: `pkg/tvdb/types_test.go`

**Step 1: Write the types file**

```go
// Package tvdb provides a client for the TVDB API v4.
package tvdb

import "time"

// Series represents a TV series from TVDB.
type Series struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Year     int    `json:"year"`      // Extracted from firstAired
	Status   string `json:"status"`    // "Continuing" or "Ended"
	Overview string `json:"overview"`
}

// Episode represents a single episode from TVDB.
type Episode struct {
	ID        int       `json:"id"`
	Season    int       `json:"seasonNumber"`
	Episode   int       `json:"number"`
	Name      string    `json:"name"`
	Overview  string    `json:"overview"`
	AirDate   time.Time `json:"aired"` // Parsed from YYYY-MM-DD
	Runtime   int       `json:"runtime"`
}

// SearchResult represents a series search result.
type SearchResult struct {
	ID       int    `json:"tvdb_id"`
	Name     string `json:"name"`
	Year     int    `json:"year"`
	Status   string `json:"status"`
	Overview string `json:"overview"`
	Network  string `json:"network"`
}

// loginResponse is the TVDB login API response.
type loginResponse struct {
	Status string `json:"status"`
	Data   struct {
		Token string `json:"token"`
	} `json:"data"`
}

// searchResponse is the TVDB search API response.
type searchResponse struct {
	Status string `json:"status"`
	Data   []struct {
		ObjectID   string `json:"objectID"`
		Name       string `json:"name"`
		Year       string `json:"year"`
		Status     string `json:"status"`
		Overview   string `json:"overview"`
		Network    string `json:"network"`
		TVDBID     string `json:"tvdb_id"`
	} `json:"data"`
}

// seriesResponse is the TVDB get series API response.
type seriesResponse struct {
	Status string `json:"status"`
	Data   struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Status     struct {
			Name string `json:"name"`
		} `json:"status"`
		Overview   string `json:"overview"`
		FirstAired string `json:"firstAired"` // YYYY-MM-DD
	} `json:"data"`
}

// episodesResponse is the TVDB get episodes API response.
type episodesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Episodes []struct {
			ID           int    `json:"id"`
			SeasonNumber int    `json:"seasonNumber"`
			Number       int    `json:"number"`
			Name         string `json:"name"`
			Overview     string `json:"overview"`
			Aired        string `json:"aired"` // YYYY-MM-DD
			Runtime      int    `json:"runtime"`
		} `json:"episodes"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}
```

**Step 2: Write a simple test to verify types compile**

```go
package tvdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTypes(t *testing.T) {
	// Verify types are usable
	s := Series{
		ID:       12345,
		Name:     "Breaking Bad",
		Year:     2008,
		Status:   "Ended",
		Overview: "A chemistry teacher becomes a drug lord.",
	}
	assert.Equal(t, 12345, s.ID)
	assert.Equal(t, "Breaking Bad", s.Name)

	e := Episode{
		ID:      1,
		Season:  1,
		Episode: 1,
		Name:    "Pilot",
		AirDate: time.Date(2008, 1, 20, 0, 0, 0, 0, time.UTC),
	}
	assert.Equal(t, 1, e.Season)
	assert.Equal(t, "Pilot", e.Name)
}
```

**Step 3: Run test**

Run: `go test ./pkg/tvdb/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/tvdb/types.go pkg/tvdb/types_test.go
git commit -m "feat(tvdb): add API types for TVDB v4

Part of #69"
```

---

## Task 2: TVDB API Client Core

**Files:**
- Create: `pkg/tvdb/client.go`
- Test: `pkg/tvdb/client_test.go`

**Step 1: Write the client implementation**

```go
package tvdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://api4.thetvdb.com/v4"
	defaultTimeout = 10 * time.Second
)

var (
	ErrNotFound      = errors.New("series not found")
	ErrUnauthorized  = errors.New("invalid API key")
	ErrRateLimited   = errors.New("rate limited")
)

// Client is a TVDB API v4 client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	mu       sync.RWMutex
	token    string
	tokenExp time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom base URL (for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// New creates a new TVDB client.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ensureToken gets a valid token, refreshing if needed.
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return nil
	}

	return c.login(ctx)
}

// login authenticates with TVDB and stores the token.
func (c *Client) login(ctx context.Context) error {
	body := fmt.Sprintf(`{"apikey": "%s"}`, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login",
		strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}

	c.token = result.Data.Token
	c.tokenExp = time.Now().Add(29 * 24 * time.Hour) // Token valid ~1 month
	return nil
}

// doRequest performs an authenticated request.
func (c *Client) doRequest(ctx context.Context, method, path string) (*http.Response, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.mu.RUnlock()
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		// Token expired, clear and retry once
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		return c.doRequest(ctx, method, path)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return nil, ErrRateLimited
	}

	return resp, nil
}

// Search searches for series by name.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	path := "/search?query=" + url.QueryEscape(query) + "&type=series"
	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: %s", resp.Status)
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	results := make([]SearchResult, 0, len(result.Data))
	for _, r := range result.Data {
		id, _ := strconv.Atoi(r.TVDBID)
		year, _ := strconv.Atoi(r.Year)
		results = append(results, SearchResult{
			ID:       id,
			Name:     r.Name,
			Year:     year,
			Status:   r.Status,
			Overview: r.Overview,
			Network:  r.Network,
		})
	}
	return results, nil
}

// GetSeries fetches series metadata by TVDB ID.
func (c *Client) GetSeries(ctx context.Context, tvdbID int) (*Series, error) {
	path := fmt.Sprintf("/series/%d", tvdbID)
	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get series failed: %s", resp.Status)
	}

	var result seriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode series response: %w", err)
	}

	// Parse year from firstAired
	var year int
	if result.Data.FirstAired != "" {
		if t, err := time.Parse("2006-01-02", result.Data.FirstAired); err == nil {
			year = t.Year()
		}
	}

	return &Series{
		ID:       result.Data.ID,
		Name:     result.Data.Name,
		Year:     year,
		Status:   result.Data.Status.Name,
		Overview: result.Data.Overview,
	}, nil
}

// GetEpisodes fetches all episodes for a series.
func (c *Client) GetEpisodes(ctx context.Context, tvdbID int) ([]Episode, error) {
	var allEpisodes []Episode
	page := 0

	for {
		path := fmt.Sprintf("/series/%d/episodes/default?page=%d", tvdbID, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil, ErrNotFound
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("get episodes failed: %s", resp.Status)
		}

		var result episodesResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode episodes response: %w", err)
		}
		resp.Body.Close()

		for _, e := range result.Data.Episodes {
			var airDate time.Time
			if e.Aired != "" {
				airDate, _ = time.Parse("2006-01-02", e.Aired)
			}
			allEpisodes = append(allEpisodes, Episode{
				ID:      e.ID,
				Season:  e.SeasonNumber,
				Episode: e.Number,
				Name:    e.Name,
				Overview: e.Overview,
				AirDate: airDate,
				Runtime: e.Runtime,
			})
		}

		if result.Links.Next == "" {
			break
		}
		page++
	}

	return allEpisodes, nil
}
```

**Step 2: Add missing import**

Add `"strings"` to imports in client.go.

**Step 3: Write tests with mock server**

```go
package tvdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/login":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]string{"token": "test-token"},
			})
		case "/v4/search":
			assert.Equal(t, "Breaking Bad", r.URL.Query().Get("query"))
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": []map[string]any{
					{"tvdb_id": "81189", "name": "Breaking Bad", "year": "2008", "status": "Ended"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL+"/v4"))
	results, err := client.Search(context.Background(), "Breaking Bad")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 81189, results[0].ID)
	assert.Equal(t, "Breaking Bad", results[0].Name)
	assert.Equal(t, 2008, results[0].Year)
}

func TestClient_GetSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/login":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]string{"token": "test-token"},
			})
		case "/v4/series/81189":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":         81189,
					"name":       "Breaking Bad",
					"firstAired": "2008-01-20",
					"status":     map[string]string{"name": "Ended"},
					"overview":   "A high school chemistry teacher turned meth producer.",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL+"/v4"))
	series, err := client.GetSeries(context.Background(), 81189)

	require.NoError(t, err)
	assert.Equal(t, 81189, series.ID)
	assert.Equal(t, "Breaking Bad", series.Name)
	assert.Equal(t, 2008, series.Year)
	assert.Equal(t, "Ended", series.Status)
}

func TestClient_GetEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/login":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]string{"token": "test-token"},
			})
		case "/v4/series/81189/episodes/default":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"episodes": []map[string]any{
						{"id": 1, "seasonNumber": 1, "number": 1, "name": "Pilot", "aired": "2008-01-20"},
						{"id": 2, "seasonNumber": 1, "number": 2, "name": "Cat's in the Bag...", "aired": "2008-01-27"},
					},
				},
				"links": map[string]string{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL+"/v4"))
	episodes, err := client.GetEpisodes(context.Background(), 81189)

	require.NoError(t, err)
	require.Len(t, episodes, 2)
	assert.Equal(t, 1, episodes[0].Season)
	assert.Equal(t, 1, episodes[0].Episode)
	assert.Equal(t, "Pilot", episodes[0].Name)
}

func TestClient_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/login":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]string{"token": "test-token"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New("test-key", WithBaseURL(server.URL+"/v4"))
	_, err := client.GetSeries(context.Background(), 99999)

	assert.ErrorIs(t, err, ErrNotFound)
}

func TestClient_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New("bad-key", WithBaseURL(server.URL+"/v4"))
	_, err := client.Search(context.Background(), "test")

	assert.ErrorIs(t, err, ErrUnauthorized)
}
```

**Step 4: Run tests**

Run: `go test ./pkg/tvdb/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/tvdb/client.go pkg/tvdb/client_test.go
git commit -m "feat(tvdb): add TVDB API v4 client

Implements:
- JWT authentication with auto-refresh
- Search series by name
- Get series metadata by ID
- Get all episodes for a series
- Error handling (not found, unauthorized, rate limited)

Part of #69"
```

---

## Task 3: Metadata Cache Migration

**Files:**
- Create: `internal/migrations/sql/007_metadata_cache.sql`

**Step 1: Write the migration**

```sql
-- Metadata cache for external API responses (TVDB, TMDB)
CREATE TABLE IF NOT EXISTS metadata_cache (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metadata_cache_expires ON metadata_cache(expires_at);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (7);
```

**Step 2: Verify migration numbering**

Run: `ls internal/migrations/sql/`
Expected: Files 001-006 exist, 007 is next.

**Step 3: Commit**

```bash
git add internal/migrations/sql/007_metadata_cache.sql
git commit -m "feat(db): add metadata_cache table for TVDB/TMDB responses

Part of #69"
```

---

## Task 4: Metadata Cache Implementation

**Files:**
- Create: `internal/metadata/cache.go`
- Test: `internal/metadata/cache_test.go`

**Step 1: Write the cache implementation**

```go
// Package metadata provides caching and orchestration for external metadata APIs.
package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Cache provides SQLite-backed caching for metadata API responses.
type Cache struct {
	db *sql.DB
}

// NewCache creates a new metadata cache.
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// Get retrieves a cached value by key.
// Returns nil if not found or expired.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	var value string
	var expiresAt time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT value, expires_at FROM metadata_cache WHERE key = ?", key,
	).Scan(&value, &expiresAt)

	if err != nil || time.Now().After(expiresAt) {
		return nil, false
	}

	return []byte(value), true
}

// Set stores a value with the given TTL.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)

	_, err := c.db.ExecContext(ctx,
		`INSERT INTO metadata_cache (key, value, expires_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, string(value), expiresAt,
	)
	if err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	return nil
}

// Delete removes a cached value.
func (c *Cache) Delete(ctx context.Context, key string) error {
	_, err := c.db.ExecContext(ctx, "DELETE FROM metadata_cache WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// Prune removes all expired entries.
// Returns the number of entries removed.
func (c *Cache) Prune(ctx context.Context) (int64, error) {
	result, err := c.db.ExecContext(ctx,
		"DELETE FROM metadata_cache WHERE expires_at < ?", time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("cache prune: %w", err)
	}
	return result.RowsAffected()
}
```

**Step 2: Write tests**

```go
package metadata

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE metadata_cache (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at DATETIME NOT NULL
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })
	return db
}

func TestCache_SetGet(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Set a value
	err := cache.Set(ctx, "test:key", []byte(`{"name":"test"}`), time.Hour)
	require.NoError(t, err)

	// Get it back
	value, ok := cache.Get(ctx, "test:key")
	assert.True(t, ok)
	assert.Equal(t, `{"name":"test"}`, string(value))
}

func TestCache_Expiration(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Set with very short TTL
	err := cache.Set(ctx, "test:expiring", []byte(`{"data":1}`), time.Millisecond)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	// Should not be found
	_, ok := cache.Get(ctx, "test:expiring")
	assert.False(t, ok)
}

func TestCache_Overwrite(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Set initial value
	err := cache.Set(ctx, "test:key", []byte(`{"v":1}`), time.Hour)
	require.NoError(t, err)

	// Overwrite
	err = cache.Set(ctx, "test:key", []byte(`{"v":2}`), time.Hour)
	require.NoError(t, err)

	// Get updated value
	value, ok := cache.Get(ctx, "test:key")
	assert.True(t, ok)
	assert.Equal(t, `{"v":2}`, string(value))
}

func TestCache_Delete(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Set and delete
	err := cache.Set(ctx, "test:delete", []byte(`data`), time.Hour)
	require.NoError(t, err)

	err = cache.Delete(ctx, "test:delete")
	require.NoError(t, err)

	// Should not be found
	_, ok := cache.Get(ctx, "test:delete")
	assert.False(t, ok)
}

func TestCache_Prune(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Set expired entry directly
	_, err := db.Exec(
		"INSERT INTO metadata_cache (key, value, expires_at) VALUES (?, ?, ?)",
		"expired", "data", time.Now().Add(-time.Hour),
	)
	require.NoError(t, err)

	// Set valid entry
	err = cache.Set(ctx, "valid", []byte(`data`), time.Hour)
	require.NoError(t, err)

	// Prune
	count, err := cache.Prune(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Valid entry should still exist
	_, ok := cache.Get(ctx, "valid")
	assert.True(t, ok)
}
```

**Step 3: Run tests**

Run: `go test ./internal/metadata/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/metadata/cache.go internal/metadata/cache_test.go
git commit -m "feat(metadata): add SQLite-backed cache for API responses

Implements:
- Get/Set with TTL
- Automatic expiration check on Get
- Delete and Prune operations

Part of #69"
```

---

## Task 5: TVDB Service (Cache + Client Orchestration)

**Files:**
- Create: `internal/metadata/tvdb.go`
- Test: `internal/metadata/tvdb_test.go`

**Step 1: Write the TVDB service**

```go
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/pkg/tvdb"
)

const (
	// Cache TTLs
	seriesTTL  = 7 * 24 * time.Hour  // 7 days
	episodeTTL = 24 * time.Hour      // 24 hours
	searchTTL  = time.Hour           // 1 hour
)

// TVDBService provides cached access to TVDB metadata.
type TVDBService struct {
	client *tvdb.Client
	cache  *Cache
	log    *slog.Logger
}

// NewTVDBService creates a new TVDB service.
func NewTVDBService(client *tvdb.Client, cache *Cache, log *slog.Logger) *TVDBService {
	if log == nil {
		log = slog.Default()
	}
	return &TVDBService{
		client: client,
		cache:  cache,
		log:    log.With("component", "tvdb-service"),
	}
}

// Search searches for series by name (cached).
func (s *TVDBService) Search(ctx context.Context, query string) ([]tvdb.SearchResult, error) {
	key := fmt.Sprintf("tvdb:search:%s", query)

	// Check cache
	if data, ok := s.cache.Get(ctx, key); ok {
		var results []tvdb.SearchResult
		if err := json.Unmarshal(data, &results); err == nil {
			s.log.Debug("cache hit", "key", key)
			return results, nil
		}
	}

	// Fetch from API
	results, err := s.client.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("tvdb search: %w", err)
	}

	// Cache results
	if data, err := json.Marshal(results); err == nil {
		if err := s.cache.Set(ctx, key, data, searchTTL); err != nil {
			s.log.Warn("cache set failed", "key", key, "error", err)
		}
	}

	s.log.Debug("fetched from API", "key", key, "results", len(results))
	return results, nil
}

// GetSeries fetches series metadata by TVDB ID (cached).
func (s *TVDBService) GetSeries(ctx context.Context, tvdbID int) (*tvdb.Series, error) {
	key := fmt.Sprintf("tvdb:series:%d", tvdbID)

	// Check cache
	if data, ok := s.cache.Get(ctx, key); ok {
		var series tvdb.Series
		if err := json.Unmarshal(data, &series); err == nil {
			s.log.Debug("cache hit", "key", key)
			return &series, nil
		}
	}

	// Fetch from API
	series, err := s.client.GetSeries(ctx, tvdbID)
	if err != nil {
		return nil, fmt.Errorf("tvdb get series: %w", err)
	}

	// Cache result
	if data, err := json.Marshal(series); err == nil {
		if err := s.cache.Set(ctx, key, data, seriesTTL); err != nil {
			s.log.Warn("cache set failed", "key", key, "error", err)
		}
	}

	s.log.Debug("fetched from API", "key", key, "series", series.Name)
	return series, nil
}

// GetEpisodes fetches all episodes for a series (cached).
func (s *TVDBService) GetEpisodes(ctx context.Context, tvdbID int) ([]tvdb.Episode, error) {
	key := fmt.Sprintf("tvdb:episodes:%d", tvdbID)

	// Check cache
	if data, ok := s.cache.Get(ctx, key); ok {
		var episodes []tvdb.Episode
		if err := json.Unmarshal(data, &episodes); err == nil {
			s.log.Debug("cache hit", "key", key, "episodes", len(episodes))
			return episodes, nil
		}
	}

	// Fetch from API
	episodes, err := s.client.GetEpisodes(ctx, tvdbID)
	if err != nil {
		return nil, fmt.Errorf("tvdb get episodes: %w", err)
	}

	// Cache result
	if data, err := json.Marshal(episodes); err == nil {
		if err := s.cache.Set(ctx, key, data, episodeTTL); err != nil {
			s.log.Warn("cache set failed", "key", key, "error", err)
		}
	}

	s.log.Debug("fetched from API", "key", key, "episodes", len(episodes))
	return episodes, nil
}

// InvalidateSeries removes cached data for a series.
func (s *TVDBService) InvalidateSeries(ctx context.Context, tvdbID int) error {
	keys := []string{
		fmt.Sprintf("tvdb:series:%d", tvdbID),
		fmt.Sprintf("tvdb:episodes:%d", tvdbID),
	}
	for _, key := range keys {
		if err := s.cache.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
```

**Step 2: Write tests**

```go
package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/pkg/tvdb"
	_ "modernc.org/sqlite"
)

func setupTVDBTest(t *testing.T) (*sql.DB, *httptest.Server) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE metadata_cache (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at DATETIME NOT NULL
		)
	`)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/login":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]string{"token": "test-token"},
			})
		case "/v4/search":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": []map[string]any{
					{"tvdb_id": "81189", "name": "Breaking Bad", "year": "2008"},
				},
			})
		case "/v4/series/81189":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id": 81189, "name": "Breaking Bad", "firstAired": "2008-01-20",
					"status": map[string]string{"name": "Ended"},
				},
			})
		case "/v4/series/81189/episodes/default":
			json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"episodes": []map[string]any{
						{"id": 1, "seasonNumber": 1, "number": 1, "name": "Pilot"},
					},
				},
				"links": map[string]string{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(func() {
		db.Close()
		server.Close()
	})

	return db, server
}

func TestTVDBService_Search(t *testing.T) {
	db, server := setupTVDBTest(t)
	client := tvdb.New("test-key", tvdb.WithBaseURL(server.URL+"/v4"))
	cache := NewCache(db)
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// First call hits API
	results, err := svc.Search(ctx, "Breaking Bad")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Breaking Bad", results[0].Name)

	// Second call should hit cache (we can verify by checking cache)
	data, ok := cache.Get(ctx, "tvdb:search:Breaking Bad")
	assert.True(t, ok)
	assert.Contains(t, string(data), "Breaking Bad")
}

func TestTVDBService_GetSeries(t *testing.T) {
	db, server := setupTVDBTest(t)
	client := tvdb.New("test-key", tvdb.WithBaseURL(server.URL+"/v4"))
	cache := NewCache(db)
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	series, err := svc.GetSeries(ctx, 81189)
	require.NoError(t, err)
	assert.Equal(t, "Breaking Bad", series.Name)
	assert.Equal(t, 2008, series.Year)

	// Verify cached
	data, ok := cache.Get(ctx, "tvdb:series:81189")
	assert.True(t, ok)
	assert.Contains(t, string(data), "Breaking Bad")
}

func TestTVDBService_GetEpisodes(t *testing.T) {
	db, server := setupTVDBTest(t)
	client := tvdb.New("test-key", tvdb.WithBaseURL(server.URL+"/v4"))
	cache := NewCache(db)
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	episodes, err := svc.GetEpisodes(ctx, 81189)
	require.NoError(t, err)
	require.Len(t, episodes, 1)
	assert.Equal(t, "Pilot", episodes[0].Name)
}

func TestTVDBService_InvalidateSeries(t *testing.T) {
	db, server := setupTVDBTest(t)
	client := tvdb.New("test-key", tvdb.WithBaseURL(server.URL+"/v4"))
	cache := NewCache(db)
	svc := NewTVDBService(client, cache, nil)

	ctx := context.Background()

	// Populate cache
	_, _ = svc.GetSeries(ctx, 81189)
	_, _ = svc.GetEpisodes(ctx, 81189)

	// Invalidate
	err := svc.InvalidateSeries(ctx, 81189)
	require.NoError(t, err)

	// Verify cleared
	_, ok := cache.Get(ctx, "tvdb:series:81189")
	assert.False(t, ok)
	_, ok = cache.Get(ctx, "tvdb:episodes:81189")
	assert.False(t, ok)
}
```

**Step 3: Run tests**

Run: `go test ./internal/metadata/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/metadata/tvdb.go internal/metadata/tvdb_test.go
git commit -m "feat(metadata): add TVDB service with caching

Orchestrates TVDB API client with SQLite cache:
- Search with 1-hour TTL
- GetSeries with 7-day TTL
- GetEpisodes with 24-hour TTL
- Invalidation support

Part of #69"
```

---

## Task 6: Configuration

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Add TVDB config struct**

Add after TMDBConfig:

```go
type TVDBConfig struct {
	APIKey string `toml:"api_key"`
}
```

**Step 2: Add TVDB field to Config struct**

Add to Config struct:

```go
TVDB          *TVDBConfig         `toml:"tvdb"`
```

**Step 3: Run tests to verify config loads**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add TVDB configuration section

Adds [tvdb] section with api_key field.

Part of #69"
```

---

## Task 7: Bulk Episode Creation

**Files:**
- Modify: `internal/library/episode.go`

**Step 1: Add BulkAddEpisodes method**

Add after AddEpisode:

```go
// BulkAddEpisodes inserts multiple episodes efficiently.
// Skips episodes that already exist (by content_id, season, episode).
func (s *Store) BulkAddEpisodes(episodes []*Episode) (int, error) {
	if len(episodes) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO episodes (content_id, season, episode, title, status, air_date)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, e := range episodes {
		result, err := stmt.Exec(e.ContentID, e.Season, e.Episode, e.Title, e.Status, e.AirDate)
		if err != nil {
			return inserted, fmt.Errorf("insert episode S%02dE%02d: %w", e.Season, e.Episode, err)
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return inserted, nil
}
```

**Step 2: Write test**

Add to episode_test.go (create if needed):

```go
func TestStore_BulkAddEpisodes(t *testing.T) {
	store := setupTestStore(t)

	// Create a series first
	content := &Content{
		Type:           ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(content))

	// Bulk add episodes
	episodes := []*Episode{
		{ContentID: content.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted},
		{ContentID: content.ID, Season: 1, Episode: 2, Title: "Second", Status: StatusWanted},
		{ContentID: content.ID, Season: 1, Episode: 3, Title: "Third", Status: StatusWanted},
	}

	inserted, err := store.BulkAddEpisodes(episodes)
	require.NoError(t, err)
	assert.Equal(t, 3, inserted)

	// Verify episodes exist
	eps, _, err := store.ListEpisodes(EpisodeFilter{ContentID: &content.ID})
	require.NoError(t, err)
	assert.Len(t, eps, 3)

	// Try adding again - should skip duplicates
	inserted, err = store.BulkAddEpisodes(episodes)
	require.NoError(t, err)
	assert.Equal(t, 0, inserted)
}
```

**Step 3: Run tests**

Run: `go test ./internal/library/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/library/episode.go internal/library/episode_test.go
git commit -m "feat(library): add BulkAddEpisodes for efficient batch insert

Uses INSERT OR IGNORE for idempotent bulk operations.

Part of #69"
```

---

## Task 8: Compat API Integration - lookupSeries

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Add TVDB service field to Server struct**

Add to Server struct:

```go
tvdbSvc   *metadata.TVDBService
```

**Step 2: Add SetTVDB method**

Add after SetBus:

```go
// SetTVDB configures the TVDB service (optional).
func (s *Server) SetTVDB(svc *metadata.TVDBService) {
	s.tvdbSvc = svc
}
```

**Step 3: Update lookupSeries to use TVDB**

Replace the lookupSeries function with:

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
		resp := s.contentToSonarrSeries(contents[0])
		if contents[0].Status == library.StatusWanted {
			resp.Monitored = false
		}
		writeJSON(w, http.StatusOK, []sonarrSeriesResponse{resp})
		return
	}

	// Not in library - try to get metadata from TVDB
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

	// Enrich with TVDB metadata if available
	if s.tvdbSvc != nil {
		series, err := s.tvdbSvc.GetSeries(r.Context(), int(tvdbID))
		if err == nil {
			response.Title = series.Name
			response.SortTitle = strings.ToLower(series.Name)
			response.Year = series.Year
			response.Status = strings.ToLower(series.Status)
			response.Overview = series.Overview
			response.CleanTitle = strings.ToLower(strings.ReplaceAll(series.Name, " ", ""))

			// Get episode count for season info
			if episodes, err := s.tvdbSvc.GetEpisodes(r.Context(), int(tvdbID)); err == nil {
				seasonMap := make(map[int]int)
				for _, ep := range episodes {
					seasonMap[ep.Season]++
				}
				response.SeasonCount = len(seasonMap)
				response.Seasons = make([]sonarrSeason, 0, len(seasonMap))
				for seasonNum := range seasonMap {
					response.Seasons = append(response.Seasons, sonarrSeason{
						SeasonNumber: seasonNum,
						Monitored:    false,
					})
				}
			}
		}
		// On error, continue with stub - graceful degradation
	}

	writeJSON(w, http.StatusOK, []sonarrSeriesResponse{response})
}
```

**Step 4: Add import**

Add to imports:

```go
"github.com/vmunix/arrgo/internal/metadata"
```

**Step 5: Run tests**

Run: `go test ./internal/api/compat/... -v`
Expected: PASS (existing tests should still pass)

**Step 6: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): enrich lookupSeries with TVDB metadata

Returns real year and season count from TVDB when available.
Falls back to stub response on error.

Part of #69"
```

---

## Task 9: Compat API Integration - addSeries Episode Sync

**Files:**
- Modify: `internal/api/compat/compat.go`

**Step 1: Update addSeries to sync episodes**

After adding content successfully, add episode sync:

```go
// After: if err := s.library.AddContent(content); err != nil { ... }

// Sync episodes from TVDB if available
if s.tvdbSvc != nil && tvdbID > 0 {
	go s.syncEpisodesFromTVDB(content.ID, int(tvdbID))
}
```

**Step 2: Add syncEpisodesFromTVDB helper**

Add after searchAndGrabSeries:

```go
// syncEpisodesFromTVDB fetches episodes from TVDB and creates Episode records.
func (s *Server) syncEpisodesFromTVDB(contentID int64, tvdbID int) {
	ctx := context.Background()

	episodes, err := s.tvdbSvc.GetEpisodes(ctx, tvdbID)
	if err != nil {
		s.log.Warn("failed to fetch episodes from TVDB", "tvdb_id", tvdbID, "error", err)
		return
	}

	// Convert to library.Episode
	libEpisodes := make([]*library.Episode, 0, len(episodes))
	for _, ep := range episodes {
		// Skip specials (season 0) and episodes without numbers
		if ep.Season == 0 || ep.Episode == 0 {
			continue
		}

		var airDate *time.Time
		if !ep.AirDate.IsZero() {
			airDate = &ep.AirDate
		}

		libEpisodes = append(libEpisodes, &library.Episode{
			ContentID: contentID,
			Season:    ep.Season,
			Episode:   ep.Episode,
			Title:     ep.Name,
			Status:    library.StatusWanted,
			AirDate:   airDate,
		})
	}

	inserted, err := s.library.BulkAddEpisodes(libEpisodes)
	if err != nil {
		s.log.Warn("failed to bulk add episodes", "content_id", contentID, "error", err)
		return
	}

	s.log.Info("synced episodes from TVDB",
		"content_id", contentID,
		"tvdb_id", tvdbID,
		"total", len(episodes),
		"inserted", inserted,
	)
}
```

**Step 3: Add time import if needed**

Ensure `"time"` is in imports.

**Step 4: Run tests**

Run: `go test ./internal/api/compat/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/compat/compat.go
git commit -m "feat(compat): sync episodes from TVDB on series add

Creates Episode records for all episodes from TVDB.
Runs asynchronously to not block the API response.

Part of #69"
```

---

## Task 10: Server Wiring

**Files:**
- Modify: `cmd/arrgod/main.go` (or wherever server is initialized)

**Step 1: Find server initialization**

Run: `grep -r "compat.New" cmd/`

**Step 2: Add TVDB client and service initialization**

Add after TMDB client setup (or similar location):

```go
// Initialize TVDB if configured
var tvdbSvc *metadata.TVDBService
if cfg.TVDB != nil && cfg.TVDB.APIKey != "" {
	tvdbClient := tvdb.New(cfg.TVDB.APIKey)
	metadataCache := metadata.NewCache(db)
	tvdbSvc = metadata.NewTVDBService(tvdbClient, metadataCache, log)
	compatServer.SetTVDB(tvdbSvc)
	log.Info("TVDB integration enabled")
}
```

**Step 3: Add imports**

```go
"github.com/vmunix/arrgo/internal/metadata"
"github.com/vmunix/arrgo/pkg/tvdb"
```

**Step 4: Build and test**

Run: `task build`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add cmd/arrgod/
git commit -m "feat(server): wire TVDB service into server startup

Initializes TVDB client and service when configured.

Part of #69"
```

---

## Task 11: CLI TVDB Search Integration

**Files:**
- Modify: `cmd/arrgo/search.go`

**Step 1: Add TVDB lookup for series searches**

This requires adding a TVDB client to the CLI. Add a helper function:

```go
// tvdbLookup searches TVDB and returns selected series info.
// Returns tvdbID, title, year, or 0, "", 0 if cancelled.
func tvdbLookup(client *Client, query string) (int64, string, int) {
	// Call server API to search TVDB
	results, err := client.TVDBSearch(query)
	if err != nil || len(results) == 0 {
		fmt.Println("No series found on TVDB")
		return 0, "", 0
	}

	if len(results) == 1 {
		r := results[0]
		fmt.Printf("Found: %s (%d) [TVDB:%d]\n", r.Name, r.Year, r.ID)
		return int64(r.ID), r.Name, r.Year
	}

	// Multiple results - prompt user
	fmt.Println("Multiple series found:")
	for i, r := range results {
		fmt.Printf("  %d. %s (%d)\n", i+1, r.Name, r.Year)
	}

	input := prompt(fmt.Sprintf("Select series [1-%d, n to cancel]: ", len(results)))
	if input == "n" || input == "N" || input == "" {
		return 0, "", 0
	}

	idx, _ := strconv.Atoi(input)
	if idx < 1 || idx > len(results) {
		return 0, "", 0
	}

	r := results[idx-1]
	return int64(r.ID), r.Name, r.Year
}
```

**Step 2: Add client method for TVDB search**

Add to client.go:

```go
type TVDBSearchResult struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Year    int    `json:"year"`
	Status  string `json:"status"`
}

func (c *Client) TVDBSearch(query string) ([]TVDBSearchResult, error) {
	resp, err := c.get("/api/v1/tvdb/search?q=" + url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var results []TVDBSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}
```

**Step 3: This requires a new API endpoint**

Add to Task 12 instead - this task is getting complex.

**Step 4: Commit partial progress**

```bash
git add cmd/arrgo/search.go cmd/arrgo/client.go
git commit -m "wip(cli): add TVDB search integration scaffolding

Part of #69"
```

---

## Task 12: Native API - TVDB Search Endpoint

**Files:**
- Modify: `internal/api/v1/handlers.go` (or appropriate file)

**Step 1: Add TVDB search endpoint**

```go
// GET /api/v1/tvdb/search?q=query
func (s *Server) handleTVDBSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query required"})
		return
	}

	if s.tvdbSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TVDB not configured"})
		return
	}

	results, err := s.tvdbSvc.Search(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}
```

**Step 2: Register route**

```go
mux.HandleFunc("GET /api/v1/tvdb/search", s.handleTVDBSearch)
```

**Step 3: Run tests**

Run: `go test ./internal/api/v1/... -v`

**Step 4: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): add TVDB search endpoint

GET /api/v1/tvdb/search?q=query

Part of #69"
```

---

## Task 13: CLI Series Search with TVDB

**Files:**
- Modify: `cmd/arrgo/search.go`

**Step 1: Update runSearchCmd to use TVDB for series**

In runSearchCmd, before calling client.Search:

```go
// For series searches, do TVDB lookup first
if contentType == "series" {
	tvdbID, title, year := tvdbLookup(client, query)
	if tvdbID > 0 {
		// Use TVDB ID in search
		query = fmt.Sprintf("%s %d", title, year)
		// Store tvdbID for grab
	}
}
```

**Step 2: Pass TVDB ID to grab request**

Update grabRelease to accept optional tvdbID.

**Step 3: Run CLI tests**

Run: `go test ./cmd/arrgo/... -v`

**Step 4: Manual test**

Run: `./arrgo search "Breaking Bad" --type series`
Expected: TVDB disambiguation if multiple results

**Step 5: Commit**

```bash
git add cmd/arrgo/
git commit -m "feat(cli): integrate TVDB lookup into series search

Searches TVDB first, prompts for disambiguation on multiple matches.

Part of #69"
```

---

## Task 14: Update example config

**Files:**
- Modify: `config.example.toml`

**Step 1: Add TVDB section**

```toml
# TVDB API for TV series metadata
# Get your API key at https://thetvdb.com/api-information
[tvdb]
api_key = "${TVDB_API_KEY}"
```

**Step 2: Commit**

```bash
git add config.example.toml
git commit -m "docs: add TVDB configuration to example config

Part of #69"
```

---

## Task 15: Integration Test

**Files:**
- Create: `internal/metadata/integration_test.go`

**Step 1: Write integration test (skipped in CI)**

```go
//go:build integration

package metadata

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/pkg/tvdb"
)

func TestTVDB_Integration(t *testing.T) {
	apiKey := os.Getenv("TVDB_API_KEY")
	if apiKey == "" {
		t.Skip("TVDB_API_KEY not set")
	}

	client := tvdb.New(apiKey)
	ctx := context.Background()

	// Test search
	results, err := client.Search(ctx, "Breaking Bad")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Find Breaking Bad
	var bbID int
	for _, r := range results {
		if r.Name == "Breaking Bad" {
			bbID = r.ID
			break
		}
	}
	require.NotZero(t, bbID, "Breaking Bad not found in search results")

	// Test get series
	series, err := client.GetSeries(ctx, bbID)
	require.NoError(t, err)
	require.Equal(t, "Breaking Bad", series.Name)
	require.Equal(t, 2008, series.Year)

	// Test get episodes
	episodes, err := client.GetEpisodes(ctx, bbID)
	require.NoError(t, err)
	require.NotEmpty(t, episodes)
	t.Logf("Found %d episodes for Breaking Bad", len(episodes))
}
```

**Step 2: Run locally**

Run: `TVDB_API_KEY=your-key go test ./internal/metadata/... -tags=integration -v`

**Step 3: Commit**

```bash
git add internal/metadata/integration_test.go
git commit -m "test: add TVDB integration test

Run with: go test -tags=integration ./internal/metadata/...
Requires TVDB_API_KEY environment variable.

Part of #69"
```

---

## Task 16: Final Cleanup and Documentation

**Step 1: Run full test suite**

Run: `task check`
Expected: All tests pass, no lint errors

**Step 2: Update issue**

Run: `gh issue comment 69 --body "Implementation complete. See commits on feature/tvdb-integration branch."`

**Step 3: Final commit for any stragglers**

```bash
git status
# Add any missed files
git add .
git commit -m "chore: cleanup TVDB integration

Closes #69"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | TVDB API types | `pkg/tvdb/types.go` |
| 2 | TVDB API client | `pkg/tvdb/client.go` |
| 3 | Cache migration | `migrations/007_metadata_cache.sql` |
| 4 | Cache implementation | `internal/metadata/cache.go` |
| 5 | TVDB service | `internal/metadata/tvdb.go` |
| 6 | Configuration | `internal/config/config.go` |
| 7 | Bulk episode creation | `internal/library/episode.go` |
| 8 | Compat lookupSeries | `internal/api/compat/compat.go` |
| 9 | Compat addSeries sync | `internal/api/compat/compat.go` |
| 10 | Server wiring | `cmd/arrgod/main.go` |
| 11 | CLI scaffolding | `cmd/arrgo/search.go` |
| 12 | Native TVDB API | `internal/api/v1/` |
| 13 | CLI series search | `cmd/arrgo/search.go` |
| 14 | Example config | `config.example.toml` |
| 15 | Integration test | `internal/metadata/integration_test.go` |
| 16 | Cleanup | - |
