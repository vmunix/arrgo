# Test Coverage Improvement Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Raise test coverage from 48.5% to 65%+ by adding tests for internal/api/v1, cmd/arrgo, and pkg/newznab.

**Architecture:** Use existing test patterns (httptest for API handlers, mock server builder for CLI client). Table-driven tests with testify assertions. Focus on untested endpoints and client methods.

**Tech Stack:** Go testing, testify (assert/require), httptest, gomock, existing mockServer builder

---

## Priority Overview

| Priority | Package | Current | Target | Tasks |
|----------|---------|---------|--------|-------|
| 1 | internal/api/v1 | 24.9% | 70% | Tasks 1-5 |
| 2 | cmd/arrgo (client) | 20.4% | 60% | Tasks 6-9 |
| 3 | pkg/newznab | 59.5% | 80% | Task 10 |

---

## Task 1: API Tests - Events and Download Events Endpoints

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Write failing test for listEvents endpoint**

Add to `api_test.go`:

```go
func TestListEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Insert test event
	_, err := db.Exec(`
		INSERT INTO events (event_type, entity_type, entity_id, occurred_at)
		VALUES ('download.created', 'download', 1, datetime('now'))
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=10", nil)
	w := httptest.NewRecorder()
	srv.listEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []struct {
			EventType  string `json:"event_type"`
			EntityType string `json:"entity_type"`
			EntityID   int64  `json:"entity_id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, "download.created", resp.Items[0].EventType)
}

func TestListEvents_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	w := httptest.NewRecorder()
	srv.listEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []any `json:"items"`
		Total int   `json:"total"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Items)
}
```

**Step 2: Run test to verify it passes**

```bash
go test -v ./internal/api/v1/... -run "TestListEvents"
```

Expected: PASS (handlers already implemented, just adding test coverage)

**Step 3: Write test for listDownloadEvents endpoint**

```go
func TestListDownloadEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Insert test content and download
	_, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd')
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer)
		VALUES (1, 'sabnzbd', 'test-id', 'downloading', 'Test.Movie.2024.1080p', 'nzbgeek')
	`)
	require.NoError(t, err)

	// Insert events for this download
	_, err = db.Exec(`
		INSERT INTO events (event_type, entity_type, entity_id, occurred_at)
		VALUES ('download.created', 'download', 1, datetime('now'))
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/1/events", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	srv.listDownloadEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []struct {
			EventType string `json:"event_type"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "download.created", resp.Items[0].EventType)
}

func TestListDownloadEvents_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/999/events", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	srv.listDownloadEvents(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./internal/api/v1/... -run "TestListDownloadEvents"
```

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test(api): add tests for events and download events endpoints"
```

---

## Task 2: API Tests - Dashboard and Verify Endpoints

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Write failing test for getDashboard endpoint**

```go
func TestGetDashboard_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Insert test content
	_, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile)
		VALUES ('movie', 'Test Movie', 2024, 'available', 'hd'),
		       ('series', 'Test Series', 2024, 'wanted', 'hd')
	`)
	require.NoError(t, err)

	// Insert downloads in various states
	_, err = db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer)
		VALUES (1, 'sabnzbd', 'id1', 'queued', 'Release1', 'idx'),
		       (1, 'sabnzbd', 'id2', 'downloading', 'Release2', 'idx'),
		       (1, 'sabnzbd', 'id3', 'completed', 'Release3', 'idx')
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()
	srv.getDashboard(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Downloads struct {
			Queued      int `json:"queued"`
			Downloading int `json:"downloading"`
			Completed   int `json:"completed"`
		} `json:"downloads"`
		Library struct {
			Movies int `json:"movies"`
			Series int `json:"series"`
		} `json:"library"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Downloads.Queued)
	assert.Equal(t, 1, resp.Downloads.Downloading)
	assert.Equal(t, 1, resp.Downloads.Completed)
	assert.Equal(t, 1, resp.Library.Movies)
	assert.Equal(t, 1, resp.Library.Series)
}
```

**Step 2: Run test**

```bash
go test -v ./internal/api/v1/... -run "TestGetDashboard"
```

**Step 3: Write test for verify endpoint**

```go
func TestVerify_NoProblems(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/verify", nil)
	w := httptest.NewRecorder()
	srv.verify(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Checked  int   `json:"checked"`
		Passed   int   `json:"passed"`
		Problems []any `json:"problems"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Empty(t, resp.Problems)
}

func TestVerify_WithDownloadID(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Insert test content and download
	_, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd')
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer)
		VALUES (1, 'sabnzbd', 'test-id', 'completed', 'Test.Movie.2024', 'idx')
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/verify?id=1", nil)
	w := httptest.NewRecorder()
	srv.verify(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Checked int `json:"checked"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Checked)
}
```

**Step 4: Run test**

```bash
go test -v ./internal/api/v1/... -run "TestVerify"
```

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test(api): add tests for dashboard and verify endpoints"
```

---

## Task 3: API Tests - Plex Endpoints

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Write test for getPlexStatus with mock Plex client**

```go
func TestGetPlexStatus_Connected(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	mockPlex.EXPECT().GetIdentity().Return(&plex.Identity{
		Name:    "Test Server",
		Version: "1.32.0",
	}, nil)
	mockPlex.EXPECT().GetLibraries().Return([]plex.Library{
		{Key: "1", Title: "Movies", Type: "movie"},
		{Key: "2", Title: "TV Shows", Type: "show"},
	}, nil)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/status", nil)
	w := httptest.NewRecorder()
	srv.getPlexStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Connected  bool   `json:"connected"`
		ServerName string `json:"server_name"`
		Version    string `json:"version"`
		Libraries  []struct {
			Title string `json:"title"`
		} `json:"libraries"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Connected)
	assert.Equal(t, "Test Server", resp.ServerName)
	assert.Len(t, resp.Libraries, 2)
}

func TestGetPlexStatus_NotConfigured(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{}) // No Plex client

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/status", nil)
	w := httptest.NewRecorder()
	srv.getPlexStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Connected bool   `json:"connected"`
		Error     string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Connected)
}
```

**Step 2: Run test**

```bash
go test -v ./internal/api/v1/... -run "TestGetPlexStatus"
```

**Step 3: Write test for scanPlexLibraries**

```go
func TestScanPlexLibraries_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	mockPlex.EXPECT().FindSectionByName("Movies").Return(&plex.Library{
		Key:   "1",
		Title: "Movies",
	}, nil)
	mockPlex.EXPECT().ScanSection("1").Return(nil)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	body := `{"libraries": ["Movies"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plex/scan", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.scanPlexLibraries(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Scanned []string `json:"scanned"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp.Scanned, "Movies")
}

func TestScanPlexLibraries_NoPlex(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"libraries": ["Movies"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plex/scan", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.scanPlexLibraries(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
```

**Step 4: Run test**

```bash
go test -v ./internal/api/v1/... -run "TestScanPlex"
```

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test(api): add tests for Plex status and scan endpoints"
```

---

## Task 4: API Tests - Plex Search and Library List

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Write test for searchPlex**

```go
func TestSearchPlex_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	mockPlex.EXPECT().Search("Matrix").Return([]plex.SearchResult{
		{
			Title:   "The Matrix",
			Year:    1999,
			Type:    "movie",
			AddedAt: time.Now().Unix(),
		},
	}, nil)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/search?query=Matrix", nil)
	w := httptest.NewRecorder()
	srv.searchPlex(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Query string `json:"query"`
		Items []struct {
			Title string `json:"title"`
			Year  int    `json:"year"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "Matrix", resp.Query)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "The Matrix", resp.Items[0].Title)
}

func TestSearchPlex_MissingQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/search", nil)
	w := httptest.NewRecorder()
	srv.searchPlex(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

**Step 2: Run test**

```bash
go test -v ./internal/api/v1/... -run "TestSearchPlex"
```

**Step 3: Write test for listPlexLibraryItems**

```go
func TestListPlexLibraryItems_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	mockPlex.EXPECT().FindSectionByName("Movies").Return(&plex.Library{
		Key:   "1",
		Title: "Movies",
	}, nil)
	mockPlex.EXPECT().GetSectionItems("1").Return([]plex.LibraryItem{
		{
			Title:    "The Matrix",
			Year:     1999,
			Type:     "movie",
			AddedAt:  time.Now().Unix(),
			FilePath: "/movies/The Matrix (1999)/The Matrix.mkv",
		},
	}, nil)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/libraries/Movies/items", nil)
	req.SetPathValue("name", "Movies")
	w := httptest.NewRecorder()
	srv.listPlexLibraryItems(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Library string `json:"library"`
		Items   []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "Movies", resp.Library)
	assert.Len(t, resp.Items, 1)
}

func TestListPlexLibraryItems_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	mockPlex.EXPECT().FindSectionByName("NonExistent").Return(nil, nil)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/libraries/NonExistent/items", nil)
	req.SetPathValue("name", "NonExistent")
	w := httptest.NewRecorder()
	srv.listPlexLibraryItems(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

**Step 4: Run test**

```bash
go test -v ./internal/api/v1/... -run "TestListPlexLibrary"
```

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test(api): add tests for Plex search and library list endpoints"
```

---

## Task 5: API Tests - Library Check, Indexers, Retry Download

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Write test for checkLibrary**

```go
func TestCheckLibrary_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Insert test content
	_, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile)
		VALUES ('movie', 'Test Movie', 2024, 'available', 'hd')
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/check", nil)
	w := httptest.NewRecorder()
	srv.checkLibrary(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.GreaterOrEqual(t, resp.Total, 1)
}

func TestCheckLibrary_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/check", nil)
	w := httptest.NewRecorder()
	srv.checkLibrary(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Items []any `json:"items"`
		Total int   `json:"total"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 0, resp.Total)
}
```

**Step 2: Write test for listIndexers**

```go
func TestListIndexers_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSearcher := mocks.NewMockSearcher(ctrl)

	// Mock indexer list
	mockSearcher.EXPECT().Indexers().Return([]string{"NZBgeek", "DrunkenSlug"})

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Searcher:  mockSearcher,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indexers", nil)
	w := httptest.NewRecorder()
	srv.listIndexers(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Indexers []struct {
			Name string `json:"name"`
		} `json:"indexers"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Indexers, 2)
}

func TestListIndexers_NoSearcher(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indexers", nil)
	w := httptest.NewRecorder()
	srv.listIndexers(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
```

**Step 3: Write test for retryDownload**

```go
func TestRetryDownload_DownloadNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockManager := mocks.NewMockDownloadManager(ctrl)
	mockSearcher := mocks.NewMockSearcher(ctrl)

	db := setupTestDB(t)
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Searcher:  mockSearcher,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/downloads/999/retry", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	srv.retryDownload(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

**Step 4: Run tests**

```bash
go test -v ./internal/api/v1/... -run "TestCheckLibrary|TestListIndexers|TestRetryDownload"
```

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test(api): add tests for library check, indexers, and retry download"
```

---

## Task 6: CLI Client Tests - Dashboard and Verify

**Files:**
- Create: `cmd/arrgo/client_test.go`

**Step 1: Write test for Dashboard client method**

Create `cmd/arrgo/client_test.go`:

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Dashboard_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/dashboard").
		ExpectGET().
		RespondJSON(map[string]any{
			"version": "1.0.0",
			"connections": map[string]bool{
				"server":  true,
				"plex":    true,
				"sabnzbd": true,
			},
			"downloads": map[string]int{
				"queued":      1,
				"downloading": 2,
				"completed":   3,
			},
			"library": map[string]int{
				"movies": 10,
				"series": 5,
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Dashboard()

	require.NoError(t, err)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.True(t, resp.Connections.Plex)
	assert.Equal(t, 1, resp.Downloads.Queued)
	assert.Equal(t, 10, resp.Library.Movies)
}

func TestClient_Dashboard_ServerError(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/dashboard").
		RespondError(http.StatusInternalServerError, "internal error").
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Dashboard()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
```

**Step 2: Run test**

```bash
go test -v ./cmd/arrgo/... -run "TestClient_Dashboard"
```

**Step 3: Write test for Verify client method**

```go
func TestClient_Verify_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/verify").
		ExpectGET().
		RespondJSON(map[string]any{
			"connections": map[string]any{
				"plex":    true,
				"sabnzbd": true,
			},
			"checked":  5,
			"passed":   5,
			"problems": []any{},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Verify(nil)

	require.NoError(t, err)
	assert.Equal(t, 5, resp.Checked)
	assert.Equal(t, 5, resp.Passed)
	assert.Empty(t, resp.Problems)
}

func TestClient_Verify_WithID(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "42", r.URL.Query().Get("id"))
			respondJSON(t, w, map[string]any{
				"checked":  1,
				"passed":   1,
				"problems": []any{},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	id := int64(42)
	resp, err := client.Verify(&id)

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Checked)
}
```

**Step 4: Run test**

```bash
go test -v ./cmd/arrgo/... -run "TestClient_Verify"
```

**Step 5: Commit**

```bash
git add cmd/arrgo/client_test.go
git commit -m "test(cli): add client tests for Dashboard and Verify methods"
```

---

## Task 7: CLI Client Tests - Plex Methods

**Files:**
- Modify: `cmd/arrgo/client_test.go`

**Step 1: Write test for PlexStatus**

```go
func TestClient_PlexStatus_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/status").
		ExpectGET().
		RespondJSON(map[string]any{
			"connected":   true,
			"server_name": "Test Plex",
			"version":     "1.32.0",
			"libraries": []map[string]any{
				{"key": "1", "title": "Movies", "type": "movie"},
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexStatus()

	require.NoError(t, err)
	assert.True(t, resp.Connected)
	assert.Equal(t, "Test Plex", resp.ServerName)
	assert.Len(t, resp.Libraries, 1)
}

func TestClient_PlexStatus_NotConnected(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/status").
		RespondJSON(map[string]any{
			"connected": false,
			"error":     "connection refused",
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexStatus()

	require.NoError(t, err)
	assert.False(t, resp.Connected)
	assert.Equal(t, "connection refused", resp.Error)
}
```

**Step 2: Write test for PlexScan**

```go
func TestClient_PlexScan_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/scan").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			var req PlexScanRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, []string{"Movies", "TV Shows"}, req.Libraries)
			respondJSON(t, w, map[string]any{
				"scanned": []string{"Movies", "TV Shows"},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexScan([]string{"Movies", "TV Shows"})

	require.NoError(t, err)
	assert.Equal(t, []string{"Movies", "TV Shows"}, resp.Scanned)
}
```

**Step 3: Write test for PlexListLibrary**

```go
func TestClient_PlexListLibrary_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/libraries/Movies/items").
		ExpectGET().
		RespondJSON(map[string]any{
			"library": "Movies",
			"items": []map[string]any{
				{"title": "The Matrix", "year": 1999, "tracked": true},
			},
			"total": 1,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexListLibrary("Movies")

	require.NoError(t, err)
	assert.Equal(t, "Movies", resp.Library)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "The Matrix", resp.Items[0].Title)
}

func TestClient_PlexSearch_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Matrix", r.URL.Query().Get("query"))
			respondJSON(t, w, map[string]any{
				"query": "Matrix",
				"items": []map[string]any{
					{"title": "The Matrix", "year": 1999},
				},
				"total": 1,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexSearch("Matrix")

	require.NoError(t, err)
	assert.Equal(t, "Matrix", resp.Query)
	assert.Len(t, resp.Items, 1)
}
```

**Step 4: Run tests**

```bash
go test -v ./cmd/arrgo/... -run "TestClient_Plex"
```

**Step 5: Commit**

```bash
git add cmd/arrgo/client_test.go
git commit -m "test(cli): add client tests for Plex methods"
```

---

## Task 8: CLI Client Tests - Import, Events, Downloads

**Files:**
- Modify: `cmd/arrgo/client_test.go`

**Step 1: Write test for Import**

```go
func TestClient_Import_TrackedDownload(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/import").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			var req ImportRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.NotNil(t, req.DownloadID)
			assert.Equal(t, int64(42), *req.DownloadID)
			respondJSON(t, w, map[string]any{
				"file_id":       1,
				"content_id":    1,
				"source_path":   "/downloads/movie.mkv",
				"dest_path":     "/movies/Movie/movie.mkv",
				"size_bytes":    1000000,
				"plex_notified": true,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	downloadID := int64(42)
	resp, err := client.Import(&ImportRequest{DownloadID: &downloadID})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.FileID)
	assert.True(t, resp.PlexNotified)
}

func TestClient_Import_ManualPath(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/import").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			var req ImportRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "/path/to/file.mkv", req.Path)
			assert.Equal(t, "Test Movie", req.Title)
			assert.Equal(t, 2024, req.Year)
			respondJSON(t, w, map[string]any{
				"file_id":    1,
				"content_id": 1,
				"dest_path":  "/movies/Test Movie (2024)/file.mkv",
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Import(&ImportRequest{
		Path:  "/path/to/file.mkv",
		Title: "Test Movie",
		Year:  2024,
		Type:  "movie",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.FileID)
}
```

**Step 2: Write test for Events**

```go
func TestClient_Events_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "20", r.URL.Query().Get("limit"))
			respondJSON(t, w, map[string]any{
				"items": []map[string]any{
					{
						"id":          1,
						"event_type":  "download.created",
						"entity_type": "download",
						"entity_id":   1,
						"occurred_at": "2024-01-01T00:00:00Z",
					},
				},
				"total": 1,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Events(20)

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, "download.created", resp.Items[0].EventType)
}
```

**Step 3: Write test for DownloadEvents**

```go
func TestClient_DownloadEvents_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/downloads/42/events").
		ExpectGET().
		RespondJSON(map[string]any{
			"items": []map[string]any{
				{"event_type": "download.created"},
				{"event_type": "download.completed"},
			},
			"total": 2,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.DownloadEvents(42)

	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)
}

func TestClient_Download_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/downloads/42").
		ExpectGET().
		RespondJSON(map[string]any{
			"id":           42,
			"content_id":   1,
			"client":       "sabnzbd",
			"status":       "downloading",
			"release_name": "Test.Movie.2024.1080p",
			"progress":     0.5,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Download(42)

	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.ID)
	assert.Equal(t, "downloading", resp.Status)
	require.NotNil(t, resp.Progress)
	assert.Equal(t, 0.5, *resp.Progress)
}
```

**Step 4: Run tests**

```bash
go test -v ./cmd/arrgo/... -run "TestClient_Import|TestClient_Events|TestClient_Download"
```

**Step 5: Commit**

```bash
git add cmd/arrgo/client_test.go
git commit -m "test(cli): add client tests for Import, Events, and Download methods"
```

---

## Task 9: CLI Client Tests - Remaining Methods (Indexers, Profiles, Files, Retry, Content)

**Files:**
- Modify: `cmd/arrgo/client_test.go`

**Step 1: Write test for Indexers**

```go
func TestClient_Indexers_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/indexers").
		ExpectGET().
		RespondJSON(map[string]any{
			"indexers": []map[string]any{
				{"name": "NZBgeek", "url": "https://api.nzbgeek.info"},
				{"name": "DrunkenSlug", "url": "https://api.drunkenslug.com"},
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Indexers(false)

	require.NoError(t, err)
	assert.Len(t, resp.Indexers, 2)
	assert.Equal(t, "NZBgeek", resp.Indexers[0].Name)
}

func TestClient_Indexers_WithTest(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "true", r.URL.Query().Get("test"))
			respondJSON(t, w, map[string]any{
				"indexers": []map[string]any{
					{"name": "NZBgeek", "status": "ok", "response_ms": 150},
				},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Indexers(true)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Indexers[0].Status)
}
```

**Step 2: Write test for Profiles**

```go
func TestClient_Profiles_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/profiles").
		ExpectGET().
		RespondJSON(map[string]any{
			"profiles": []map[string]any{
				{"name": "hd", "accept": []string{"1080p", "720p"}},
				{"name": "uhd", "accept": []string{"2160p", "1080p"}},
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Profiles()

	require.NoError(t, err)
	assert.Len(t, resp.Profiles, 2)
	assert.Equal(t, "hd", resp.Profiles[0].Name)
}
```

**Step 3: Write test for Files**

```go
func TestClient_Files_All(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/files").
		ExpectGET().
		RespondJSON(map[string]any{
			"items": []map[string]any{
				{"id": 1, "path": "/movies/Test.mkv", "size_bytes": 1000000},
			},
			"total": 1,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Files(nil)

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Total)
}

func TestClient_Files_ByContentID(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "42", r.URL.Query().Get("content_id"))
			respondJSON(t, w, map[string]any{
				"items": []map[string]any{},
				"total": 0,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	contentID := int64(42)
	resp, err := client.Files(&contentID)

	require.NoError(t, err)
	assert.Equal(t, 0, resp.Total)
}
```

**Step 4: Write test for RetryDownload**

```go
func TestClient_RetryDownload_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/downloads/42/retry").
		ExpectPOST().
		RespondJSON(map[string]any{
			"new_download_id": 43,
			"release_name":    "Test.Movie.2024.1080p.Remux",
			"message":         "Found and grabbed new release",
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.RetryDownload(42)

	require.NoError(t, err)
	assert.Equal(t, int64(43), resp.NewDownloadID)
	assert.Contains(t, resp.Message, "Found")
}
```

**Step 5: Write tests for AddContent and FindContent**

```go
func TestClient_AddContent_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/content").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "movie", req["type"])
			assert.Equal(t, "Test Movie", req["title"])
			assert.Equal(t, float64(2024), req["year"])
			w.WriteHeader(http.StatusCreated)
			respondJSON(t, w, map[string]any{
				"id":              1,
				"type":            "movie",
				"title":           "Test Movie",
				"year":            2024,
				"status":          "wanted",
				"quality_profile": "hd",
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.AddContent("movie", "Test Movie", 2024, "hd")

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.ID)
	assert.Equal(t, "wanted", resp.Status)
}

func TestClient_FindContent_Found(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "movie", r.URL.Query().Get("type"))
			assert.Equal(t, "Test Movie", r.URL.Query().Get("title"))
			respondJSON(t, w, map[string]any{
				"items": []map[string]any{
					{"id": 1, "title": "Test Movie", "year": 2024},
				},
				"total": 1,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.FindContent("movie", "Test Movie", 2024)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(1), resp.ID)
}

func TestClient_FindContent_NotFound(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		RespondJSON(map[string]any{
			"items": []map[string]any{},
			"total": 0,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.FindContent("movie", "Nonexistent", 2024)

	require.NoError(t, err)
	assert.Nil(t, resp)
}

func TestClient_Grab_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/grab").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, float64(1), req["content_id"])
			assert.Equal(t, "http://example.com/nzb", req["download_url"])
			respondJSON(t, w, map[string]any{
				"download_id": 42,
				"status":      "queued",
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Grab(1, "http://example.com/nzb", "Test.Release", "NZBgeek")

	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.DownloadID)
}
```

**Step 6: Run all client tests**

```bash
go test -v ./cmd/arrgo/... -run "TestClient_"
```

**Step 7: Commit**

```bash
git add cmd/arrgo/client_test.go
git commit -m "test(cli): add client tests for Indexers, Profiles, Files, Retry, and Content methods"
```

---

## Task 10: Newznab Package Tests - Edge Cases and Error Handling

**Files:**
- Modify: `pkg/newznab/client_test.go`

**Step 1: Review existing tests and identify gaps**

```bash
go test -v -cover ./pkg/newznab/...
```

**Step 2: Write tests for malformed XML responses**

Add to `pkg/newznab/client_test.go`:

```go
func TestSearch_MalformedXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<invalid xml without closing`))
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	_, err := client.Search(context.Background(), "test query", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestSearch_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	results, err := client.Search(context.Background(), "test", nil)

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
			<error code="100" description="Incorrect user credentials"/>
		`))
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	_, err := client.Search(context.Background(), "test", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "100")
}
```

**Step 3: Write tests for HTTP errors**

```go
func TestSearch_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	_, err := client.Search(context.Background(), "test", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestSearch_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	client.HTTPClient.Timeout = 10 * time.Millisecond

	ctx := context.Background()
	_, err := client.Search(ctx, "test", nil)

	require.Error(t, err)
}

func TestSearch_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Search(ctx, "test", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
```

**Step 4: Write tests for edge cases in parsing**

```go
func TestSearch_MissingFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
			<rss version="2.0">
				<channel>
					<item>
						<title>Test Release</title>
						<!-- Missing other fields -->
					</item>
				</channel>
			</rss>
		`))
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	results, err := client.Search(context.Background(), "test", nil)

	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Test Release", results[0].Title)
	// Other fields should be zero values
	assert.Equal(t, int64(0), results[0].Size)
}

func TestSearch_InvalidDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
			<rss version="2.0">
				<channel>
					<item>
						<title>Test Release</title>
						<pubDate>not a valid date</pubDate>
					</item>
				</channel>
			</rss>
		`))
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	results, err := client.Search(context.Background(), "test", nil)

	// Should not fail, but date should be zero
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
```

**Step 5: Run tests**

```bash
go test -v -cover ./pkg/newznab/...
```

**Step 6: Commit**

```bash
git add pkg/newznab/client_test.go
git commit -m "test(newznab): add tests for error handling and edge cases"
```

---

## Final Step: Verify Coverage Improvement

**Run full coverage report:**

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -E "^total:|internal/api/v1|cmd/arrgo|pkg/newznab"
```

**Expected results:**
- `internal/api/v1`: ~70% (up from 24.9%)
- `cmd/arrgo`: ~60% (up from 20.4%)
- `pkg/newznab`: ~80% (up from 59.5%)
- Total: ~65% (up from 48.5%)

**Final commit:**

```bash
git add -A
git commit -m "test: improve test coverage to 65%+ (closes #63)"
```
