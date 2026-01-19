//go:build integration

package v1

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arrgo/arrgo/internal/config"
	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/search"
	_ "github.com/mattn/go-sqlite3"
)

// mockIndexerAPI implements search.IndexerAPI for testing.
type mockIndexerAPI struct {
	releases []search.Release
}

func (m *mockIndexerAPI) Search(ctx context.Context, q search.Query) ([]search.Release, []error) {
	return m.releases, nil
}

// testEnv holds all components needed for integration tests.
type testEnv struct {
	t *testing.T

	// Servers
	api     *httptest.Server // arrgo API under test
	sabnzbd *httptest.Server // Mock SABnzbd

	// Database
	db *sql.DB

	// Components for direct access in tests
	manager     *download.Manager
	mockIndexer *mockIndexerAPI

	// Mock response configuration
	sabnzbdClientID string
	sabnzbdStatus   *download.ClientStatus
}

func (e *testEnv) cleanup() {
	if e.api != nil {
		e.api.Close()
	}
	if e.sabnzbd != nil {
		e.sabnzbd.Close()
	}
	if e.db != nil {
		_ = e.db.Close()
	}
}

func (e *testEnv) mockSABnzbdServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		w.Header().Set("Content-Type", "application/json")

		switch mode {
		case "addurl":
			// Return configured client ID
			resp := map[string]any{
				"status":  true,
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
					"nzo_id":  e.sabnzbdStatus.ID,
					"name":    e.sabnzbdStatus.Name,
					"status":  "Completed",
					"storage": e.sabnzbdStatus.Path,
					"bytes":   e.sabnzbdStatus.Size,
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

func setupIntegrationTest(t *testing.T) *testEnv {
	t.Helper()

	env := &testEnv{t: t}
	t.Cleanup(env.cleanup)

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
	env.mockIndexer = &mockIndexerAPI{}
	env.sabnzbd = env.mockSABnzbdServer()

	// Create scorer with default profiles
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p", "720p"},
			Sources:    []string{"bluray", "webdl"},
		},
		"any": {
			Resolution: []string{"2160p", "1080p", "720p", "480p"},
		},
	}
	scorer := search.NewScorer(profiles)

	// Create searcher with mock indexer
	searcher := search.NewSearcher(env.mockIndexer, scorer, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Create SABnzbd client pointing to mock
	sabnzbdClient := download.NewSABnzbdClient(env.sabnzbd.URL, "test-api-key", "arrgo")

	// Create download store and manager
	downloadStore := download.NewStore(db)
	manager := download.NewManager(sabnzbdClient, downloadStore, slog.New(slog.NewTextHandler(io.Discard, nil)))
	env.manager = manager

	// Build quality profiles map for API (resolution names for display)
	qualityProfileNames := make(map[string][]string)
	for name, p := range profiles {
		qualityProfileNames[name] = p.Resolution
	}

	// Create API server
	cfg := Config{
		MovieRoot:       "/movies",
		SeriesRoot:      "/tv",
		QualityProfiles: qualityProfileNames,
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

// HTTP helpers

func httpPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("httpPost: %v", err)
	}
	return resp
}

func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("httpGet: %v", err)
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

func mockRelease(title string, size int64, indexer string) search.Release {
	return search.Release{
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

func TestIntegration_SearchAndGrab(t *testing.T) {
	env := setupIntegrationTest(t)

	// 1. Configure mock indexer to return releases
	env.mockIndexer.releases = []search.Release{
		mockRelease("The.Matrix.1999.1080p.BluRay.x264", 12_000_000_000, "nzbgeek"),
		mockRelease("The.Matrix.1999.720p.BluRay", 8_000_000_000, "drunken"),
	}
	env.sabnzbdClientID = "SABnzbd_nzo_abc123"

	// 2. POST /api/v1/search - verify results returned
	searchResp := httpPost(t, env.api.URL+"/api/v1/search", map[string]any{
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
	contentResp := httpPost(t, env.api.URL+"/api/v1/content", map[string]any{
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
	grabResp := httpPost(t, env.api.URL+"/api/v1/grab", map[string]any{
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

func TestIntegration_DownloadComplete(t *testing.T) {
	env := setupIntegrationTest(t)

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

// simpleTestEnv holds minimal components for tests that don't need full integration setup.
type simpleTestEnv struct {
	t      *testing.T
	db     *sql.DB
	server *Server
	mux    *http.ServeMux
}

func (e *simpleTestEnv) cleanup() {
	if e.db != nil {
		_ = e.db.Close()
	}
}

func newTestEnv(t *testing.T) *simpleTestEnv {
	t.Helper()

	env := &simpleTestEnv{t: t}
	t.Cleanup(env.cleanup)

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

	// Create API server with minimal config
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
	}
	env.server = New(db, cfg)
	env.mux = http.NewServeMux()

	return env
}

func TestIntegration_PlexStatus_NotConfigured(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Don't configure Plex client - leave it nil
	req := httptest.NewRequest("GET", "/api/v1/plex/status", nil)
	rr := httptest.NewRecorder()

	env.server.RegisterRoutes(env.mux)
	env.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["connected"] != false {
		t.Errorf("connected: got %v, want false", resp["connected"])
	}
	if resp["error"] != "Plex not configured" {
		t.Errorf("error: got %v, want 'Plex not configured'", resp["error"])
	}
}

func TestIntegration_FullHappyPath(t *testing.T) {
	env := setupIntegrationTest(t)

	// Configure all mocks upfront
	env.mockIndexer.releases = []search.Release{
		mockRelease("Inception.2010.1080p.BluRay.x264", 15_000_000_000, "nzbgeek"),
	}
	env.sabnzbdClientID = "SABnzbd_nzo_inception"

	// Phase 1: Search
	searchResp := httpPost(t, env.api.URL+"/api/v1/search", map[string]any{
		"query": "inception 2010",
		"type":  "movie",
	})
	if searchResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(searchResp.Body)
		t.Fatalf("search status = %d, want 200: %s", searchResp.StatusCode, body)
	}

	var searchResult searchResponse
	decodeJSON(t, searchResp, &searchResult)

	if len(searchResult.Releases) == 0 {
		t.Fatal("expected at least one release")
	}

	// Phase 2: Add content
	contentResp := httpPost(t, env.api.URL+"/api/v1/content", map[string]any{
		"type":            "movie",
		"title":           "Inception",
		"year":            2010,
		"quality_profile": "hd",
	})
	if contentResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(contentResp.Body)
		t.Fatalf("add content status = %d, want 201: %s", contentResp.StatusCode, body)
	}

	var content contentResponse
	decodeJSON(t, contentResp, &content)

	// Phase 3: Grab
	grabResp := httpPost(t, env.api.URL+"/api/v1/grab", map[string]any{
		"content_id":   content.ID,
		"download_url": searchResult.Releases[0].DownloadURL,
		"title":        searchResult.Releases[0].Title,
		"indexer":      searchResult.Releases[0].Indexer,
	})
	if grabResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(grabResp.Body)
		t.Fatalf("grab status = %d, want 200: %s", grabResp.StatusCode, body)
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
	getResp := httpGet(t, env.api.URL+"/api/v1/content/"+fmt.Sprintf("%d", content.ID))
	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("get content status = %d, want 200: %s", getResp.StatusCode, body)
	}
}
