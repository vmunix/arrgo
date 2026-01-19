# Integration Testing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** End-to-end integration tests verifying API → external service calls → DB state changes.

**Architecture:** In-process testing using httptest servers for Prowlarr/SABnzbd mocks, in-memory SQLite, and direct API handler invocation. Three tests: search+grab flow, download completion flow, and full happy path.

**Tech Stack:** Go testing, httptest, in-memory SQLite, existing testdata/schema.sql

---

### Task 1: Create Integration Test File with Test Environment

**Files:**
- Create: `internal/api/v1/integration_test.go`

**Step 1: Create the integration test file with build tag and testEnv struct**

```go
//go:build integration

package v1

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/search"
	_ "github.com/mattn/go-sqlite3"
)

// testEnv holds all components needed for integration tests.
type testEnv struct {
	t *testing.T

	// Servers
	api      *httptest.Server // arrgo API under test
	prowlarr *httptest.Server // Mock Prowlarr
	sabnzbd  *httptest.Server // Mock SABnzbd

	// Database
	db *sql.DB

	// Mock response configuration
	prowlarrReleases []search.ProwlarrRelease
	sabnzbdClientID  string
	sabnzbdStatus    *download.ClientStatus
	sabnzbdErr       error
}

func (e *testEnv) cleanup() {
	if e.api != nil {
		e.api.Close()
	}
	if e.prowlarr != nil {
		e.prowlarr.Close()
	}
	if e.sabnzbd != nil {
		e.sabnzbd.Close()
	}
	if e.db != nil {
		_ = e.db.Close()
	}
}
```

**Step 2: Run to verify it compiles**

Run: `go build -tags=integration ./internal/api/v1/`
Expected: Build succeeds (no tests yet)

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add integration test file with testEnv struct"
```

---

### Task 2: Add Mock Prowlarr Server

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add mockProwlarrServer function**

Add after the testEnv struct:

```go
func (e *testEnv) mockProwlarrServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Return configured releases
		w.Header().Set("Content-Type", "application/json")
		resp := make([]map[string]any, len(e.prowlarrReleases))
		for i, rel := range e.prowlarrReleases {
			resp[i] = map[string]any{
				"title":       rel.Title,
				"guid":        rel.GUID,
				"indexer":     rel.Indexer,
				"downloadUrl": rel.DownloadURL,
				"size":        rel.Size,
				"publishDate": rel.PublishDate.Format(time.RFC3339),
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}
```

**Step 2: Run to verify it compiles**

Run: `go build -tags=integration ./internal/api/v1/`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add mock Prowlarr server"
```

---

### Task 3: Add Mock SABnzbd Server

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add mockSABnzbdServer function**

Add after mockProwlarrServer:

```go
func (e *testEnv) mockSABnzbdServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		w.Header().Set("Content-Type", "application/json")

		switch mode {
		case "addurl":
			// Return configured client ID
			resp := map[string]any{
				"status": true,
				"nzo_ids": []string{e.sabnzbdClientID},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "queue":
			// Return queue status if configured
			slots := []map[string]any{}
			if e.sabnzbdStatus != nil && e.sabnzbdStatus.Status != download.StatusCompleted {
				slots = append(slots, map[string]any{
					"nzo_id":     e.sabnzbdStatus.ID,
					"filename":   e.sabnzbdStatus.Name,
					"status":     "Downloading",
					"percentage": e.sabnzbdStatus.Progress,
					"mb":         float64(e.sabnzbdStatus.Size) / 1024 / 1024,
				})
			}
			resp := map[string]any{
				"queue": map[string]any{"slots": slots},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "history":
			// Return history status if configured and completed
			slots := []map[string]any{}
			if e.sabnzbdStatus != nil && e.sabnzbdStatus.Status == download.StatusCompleted {
				slots = append(slots, map[string]any{
					"nzo_id":       e.sabnzbdStatus.ID,
					"name":         e.sabnzbdStatus.Name,
					"status":       "Completed",
					"storage":      e.sabnzbdStatus.Path,
					"bytes":        e.sabnzbdStatus.Size,
				})
			}
			resp := map[string]any{
				"history": map[string]any{"slots": slots},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "unknown mode", http.StatusBadRequest)
		}
	}))
}
```

**Step 2: Run to verify it compiles**

Run: `go build -tags=integration ./internal/api/v1/`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add mock SABnzbd server"
```

---

### Task 4: Add setupIntegrationTest Function

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add setupIntegrationTest that wires everything together**

Add after mock server functions:

```go
func setupIntegrationTest(t *testing.T) *testEnv {
	t.Helper()

	env := &testEnv{t: t}

	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	env.db = db

	// Apply schema
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	// Create mock external services
	env.prowlarr = env.mockProwlarrServer()
	env.sabnzbd = env.mockSABnzbdServer()

	// Create Prowlarr client pointing to mock
	prowlarrClient := search.NewProwlarrClient(env.prowlarr.URL, "test-api-key")

	// Create scorer with default profiles
	profiles := map[string][]string{
		"hd":  {"1080p bluray", "1080p webdl", "720p bluray", "720p webdl"},
		"any": {"2160p", "1080p", "720p", "480p"},
	}
	scorer := search.NewScorer(profiles)

	// Create searcher
	searcher := search.NewSearcher(prowlarrClient, scorer)

	// Create SABnzbd client pointing to mock
	sabnzbdClient := download.NewSABnzbdClient(env.sabnzbd.URL, "test-api-key", "arrgo")

	// Create download store and manager
	downloadStore := download.NewStore(db)
	manager := download.NewManager(sabnzbdClient, downloadStore)

	// Create API server
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
		QualityProfiles: profiles,
	}
	srv := New(db, cfg)
	srv.SetSearcher(searcher)
	srv.SetManager(manager)

	// Create HTTP test server
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	env.api = httptest.NewServer(mux)

	return env
}
```

**Step 2: Run to verify it compiles**

Run: `go build -tags=integration ./internal/api/v1/`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add setupIntegrationTest function"
```

---

### Task 5: Add HTTP and Builder Helpers

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add HTTP helper functions**

Add after setupIntegrationTest:

```go
// HTTP helpers

func httpPost(url string, body any) *http.Response {
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		panic(err)
	}
	return resp
}

func httpGet(url string) *http.Response {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode JSON: %v\nbody: %s", err, string(body))
	}
}

// Builder helpers

func mockRelease(title string, size int64, indexer string) search.ProwlarrRelease {
	return search.ProwlarrRelease{
		Title:       title,
		GUID:        "guid-" + title,
		Indexer:     indexer,
		DownloadURL: "http://example.com/nzb/" + title,
		Size:        size,
		PublishDate: time.Now(),
	}
}

// DB helpers

func insertTestContent(t *testing.T, db *sql.DB, contentType, title string, year int) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES (?, ?, ?, 'wanted', 'hd', '/movies')`,
		contentType, title, year,
	)
	if err != nil {
		t.Fatalf("insert content: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func insertTestDownload(t *testing.T, db *sql.DB, contentID int64, clientID, status string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer, added_at)
		VALUES (?, 'sabnzbd', ?, ?, 'Test.Release', 'TestIndexer', datetime('now'))`,
		contentID, clientID, status,
	)
	if err != nil {
		t.Fatalf("insert download: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func queryDownload(t *testing.T, db *sql.DB, contentID int64) *download.Download {
	t.Helper()
	d := &download.Download{}
	err := db.QueryRow(`
		SELECT id, content_id, client, client_id, status, release_name, indexer
		FROM downloads WHERE content_id = ?`, contentID,
	).Scan(&d.ID, &d.ContentID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer)
	if err != nil {
		t.Fatalf("query download: %v", err)
	}
	return d
}
```

**Step 2: Run to verify it compiles**

Run: `go build -tags=integration ./internal/api/v1/`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add HTTP and builder helpers for integration tests"
```

---

### Task 6: Implement TestIntegration_SearchAndGrab

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add the search and grab integration test**

Add at the end of the file:

```go
func TestIntegration_SearchAndGrab(t *testing.T) {
	env := setupIntegrationTest(t)
	defer env.cleanup()

	// 1. Configure mock Prowlarr to return releases
	env.prowlarrReleases = []search.ProwlarrRelease{
		mockRelease("The.Matrix.1999.1080p.BluRay.x264", 12_000_000_000, "nzbgeek"),
		mockRelease("The.Matrix.1999.720p.BluRay", 8_000_000_000, "drunken"),
	}
	env.sabnzbdClientID = "SABnzbd_nzo_abc123"

	// 2. POST /api/v1/search - verify results returned
	searchResp := httpPost(env.api.URL+"/api/v1/search", map[string]any{
		"query":   "the matrix",
		"type":    "movie",
		"profile": "hd",
	})
	if searchResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(searchResp.Body)
		t.Fatalf("search status = %d, want 200: %s", searchResp.StatusCode, body)
	}

	var searchResult searchResponse
	decodeJSON(t, searchResp, &searchResult)

	if len(searchResult.Releases) != 2 {
		t.Errorf("releases = %d, want 2", len(searchResult.Releases))
	}
	if searchResult.Releases[0].Title != "The.Matrix.1999.1080p.BluRay.x264" {
		t.Errorf("first release = %q, want 1080p version", searchResult.Releases[0].Title)
	}

	// 3. POST /api/v1/content - create content entry
	contentResp := httpPost(env.api.URL+"/api/v1/content", map[string]any{
		"type":            "movie",
		"title":           "The Matrix",
		"year":            1999,
		"quality_profile": "hd",
	})
	if contentResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(contentResp.Body)
		t.Fatalf("add content status = %d, want 201: %s", contentResp.StatusCode, body)
	}

	var content contentResponse
	decodeJSON(t, contentResp, &content)

	if content.ID == 0 {
		t.Fatal("content ID should be set")
	}

	// 4. POST /api/v1/grab - verify SABnzbd called, download record created
	grabResp := httpPost(env.api.URL+"/api/v1/grab", map[string]any{
		"content_id":   content.ID,
		"download_url": searchResult.Releases[0].DownloadURL,
		"title":        searchResult.Releases[0].Title,
		"indexer":      searchResult.Releases[0].Indexer,
	})
	if grabResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(grabResp.Body)
		t.Fatalf("grab status = %d, want 200: %s", grabResp.StatusCode, body)
	}

	var grab grabResponse
	decodeJSON(t, grabResp, &grab)

	if grab.DownloadID == 0 {
		t.Error("download ID should be set")
	}
	if grab.Status != "queued" {
		t.Errorf("status = %q, want queued", grab.Status)
	}

	// 5. Verify DB state
	dl := queryDownload(t, env.db, content.ID)
	if dl.ClientID != "SABnzbd_nzo_abc123" {
		t.Errorf("client_id = %q, want SABnzbd_nzo_abc123", dl.ClientID)
	}
	if dl.Status != download.StatusQueued {
		t.Errorf("status = %q, want queued", dl.Status)
	}
	if dl.ReleaseName != "The.Matrix.1999.1080p.BluRay.x264" {
		t.Errorf("release_name = %q, want The.Matrix.1999.1080p.BluRay.x264", dl.ReleaseName)
	}
}
```

**Step 2: Run the test**

Run: `go test -tags=integration -v ./internal/api/v1/ -run TestIntegration_SearchAndGrab`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add TestIntegration_SearchAndGrab"
```

---

### Task 7: Implement TestIntegration_DownloadComplete

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add the download complete integration test**

This test requires access to the manager to call Refresh(). We need to store it in testEnv. First update the testEnv struct to include the manager:

Update testEnv struct (add field):

```go
type testEnv struct {
	t *testing.T

	// Servers
	api      *httptest.Server
	prowlarr *httptest.Server
	sabnzbd  *httptest.Server

	// Database
	db *sql.DB

	// Components for direct access in tests
	manager *download.Manager

	// Mock response configuration
	prowlarrReleases []search.ProwlarrRelease
	sabnzbdClientID  string
	sabnzbdStatus    *download.ClientStatus
	sabnzbdErr       error
}
```

Update setupIntegrationTest to store manager (add line after creating manager):

```go
	env.manager = manager
```

Then add the test:

```go
func TestIntegration_DownloadComplete(t *testing.T) {
	env := setupIntegrationTest(t)
	defer env.cleanup()

	// 1. Seed DB with content + download record
	contentID := insertTestContent(t, env.db, "movie", "The Matrix", 1999)
	_ = insertTestDownload(t, env.db, contentID, "SABnzbd_nzo_xyz789", "queued")

	// 2. Configure SABnzbd mock to report "completed"
	env.sabnzbdStatus = &download.ClientStatus{
		ID:     "SABnzbd_nzo_xyz789",
		Name:   "The.Matrix.1999.1080p.BluRay",
		Status: download.StatusCompleted,
		Path:   "/downloads/complete/The.Matrix.1999.1080p.BluRay",
	}

	// 3. Trigger status refresh
	ctx := t.Context()
	if err := env.manager.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// 4. Verify DB: download status updated to completed
	dl := queryDownload(t, env.db, contentID)
	if dl.Status != download.StatusCompleted {
		t.Errorf("status = %q, want completed", dl.Status)
	}
}
```

**Step 2: Run the test**

Run: `go test -tags=integration -v ./internal/api/v1/ -run TestIntegration_DownloadComplete`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add TestIntegration_DownloadComplete"
```

---

### Task 8: Implement TestIntegration_FullHappyPath

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add the full happy path test**

```go
func TestIntegration_FullHappyPath(t *testing.T) {
	env := setupIntegrationTest(t)
	defer env.cleanup()

	// Configure all mocks upfront
	env.prowlarrReleases = []search.ProwlarrRelease{
		mockRelease("Inception.2010.1080p.BluRay.x264", 15_000_000_000, "nzbgeek"),
	}
	env.sabnzbdClientID = "SABnzbd_nzo_inception"

	// Phase 1: Search
	searchResp := httpPost(env.api.URL+"/api/v1/search", map[string]any{
		"query": "inception 2010",
		"type":  "movie",
	})
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("search failed: %d", searchResp.StatusCode)
	}

	var searchResult searchResponse
	decodeJSON(t, searchResp, &searchResult)

	if len(searchResult.Releases) == 0 {
		t.Fatal("expected at least one release")
	}

	// Phase 2: Add content
	contentResp := httpPost(env.api.URL+"/api/v1/content", map[string]any{
		"type":            "movie",
		"title":           "Inception",
		"year":            2010,
		"quality_profile": "hd",
	})
	if contentResp.StatusCode != http.StatusCreated {
		t.Fatalf("add content failed: %d", contentResp.StatusCode)
	}

	var content contentResponse
	decodeJSON(t, contentResp, &content)

	// Phase 3: Grab
	grabResp := httpPost(env.api.URL+"/api/v1/grab", map[string]any{
		"content_id":   content.ID,
		"download_url": searchResult.Releases[0].DownloadURL,
		"title":        searchResult.Releases[0].Title,
		"indexer":      searchResult.Releases[0].Indexer,
	})
	if grabResp.StatusCode != http.StatusOK {
		t.Fatalf("grab failed: %d", grabResp.StatusCode)
	}

	// Verify download created with queued status
	dl := queryDownload(t, env.db, content.ID)
	if dl.Status != download.StatusQueued {
		t.Errorf("initial status = %q, want queued", dl.Status)
	}

	// Phase 4: Simulate download completion
	env.sabnzbdStatus = &download.ClientStatus{
		ID:     "SABnzbd_nzo_inception",
		Name:   "Inception.2010.1080p.BluRay.x264",
		Status: download.StatusCompleted,
		Path:   "/downloads/complete/Inception.2010.1080p.BluRay.x264",
	}

	// Trigger refresh
	ctx := t.Context()
	if err := env.manager.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Verify final state: download completed
	dl = queryDownload(t, env.db, content.ID)
	if dl.Status != download.StatusCompleted {
		t.Errorf("final status = %q, want completed", dl.Status)
	}

	// Verify content still exists and can be retrieved via API
	getResp := httpGet(env.api.URL + "/api/v1/content/" + fmt.Sprintf("%d", content.ID))
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get content failed: %d", getResp.StatusCode)
	}
}
```

**Step 2: Add missing import**

Add `"fmt"` to the imports at the top of the file if not already present.

**Step 3: Run all integration tests**

Run: `go test -tags=integration -v ./internal/api/v1/`
Expected: All 3 tests PASS

**Step 4: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: add TestIntegration_FullHappyPath"
```

---

### Task 9: Final Verification and Update Taskfile

**Files:**
- Modify: `Taskfile.yml` (if exists)

**Step 1: Run all tests (unit + integration)**

Run: `go test ./... && go test -tags=integration ./internal/api/v1/`
Expected: All tests PASS

**Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors

**Step 3: Add integration test task to Taskfile (if exists)**

Check if Taskfile.yml exists:
```bash
ls Taskfile.yml
```

If it exists, add an integration test task:

```yaml
  test:integration:
    desc: Run integration tests
    cmds:
      - go test -tags=integration -v ./internal/api/v1/
```

**Step 4: Commit**

```bash
git add Taskfile.yml
git commit -m "chore: add integration test task"
```

If no Taskfile, skip this step.

**Step 5: Final commit summary**

Run: `git log --oneline -10`

Expected commits:
- test: add TestIntegration_FullHappyPath
- test: add TestIntegration_DownloadComplete
- test: add TestIntegration_SearchAndGrab
- test: add HTTP and builder helpers for integration tests
- test: add setupIntegrationTest function
- test: add mock SABnzbd server
- test: add mock Prowlarr server
- test: add integration test file with testEnv struct
