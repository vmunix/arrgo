//go:build integration

package v1

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	_ "modernc.org/sqlite"

	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
	searchmocks "github.com/vmunix/arrgo/internal/search/mocks"
)

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
	ctrl        *gomock.Controller
	mockIndexer *searchmocks.MockIndexerAPI

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
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err, "open db")
	env.db = db

	// Apply schema
	_, err = db.Exec(testSchema)
	require.NoError(t, err, "apply schema")

	// Create mock external services
	env.ctrl = gomock.NewController(t)
	env.mockIndexer = searchmocks.NewMockIndexerAPI(env.ctrl)
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
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: downloadStore,
		History:   importer.NewHistoryStore(db),
		Searcher:  searcher,
		Manager:   manager,
	}
	srv, err := NewWithDeps(deps, cfg)
	require.NoError(t, err, "create server")

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
	require.NoError(t, err, "httpPost")
	return resp
}

func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err, "httpGet")
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, json.Unmarshal(body, v), "decode JSON, body: %s", string(body))
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
	require.NoError(t, err, "insert content")
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
	require.NoError(t, err, "insert download")
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
	require.NoError(t, err, "query download")
	return d
}

func TestIntegration_SearchAndGrab(t *testing.T) {
	env := setupIntegrationTest(t)

	// 1. Configure mock indexer to return releases
	releases := []search.Release{
		mockRelease("The.Matrix.1999.1080p.BluRay.x264", 12_000_000_000, "nzbgeek"),
		mockRelease("The.Matrix.1999.720p.BluRay", 8_000_000_000, "drunken"),
	}
	env.mockIndexer.EXPECT().Search(gomock.Any(), gomock.Any()).Return(releases, nil)
	env.sabnzbdClientID = "SABnzbd_nzo_abc123"

	// 2. POST /api/v1/search - verify results returned
	searchResp := httpPost(t, env.api.URL+"/api/v1/search", map[string]any{
		"query":   "the matrix",
		"type":    "movie",
		"profile": "hd",
	})
	require.Equal(t, http.StatusOK, searchResp.StatusCode, "search failed: %s", readBody(t, searchResp))

	var searchResult searchResponse
	decodeJSON(t, searchResp, &searchResult)

	assert.Len(t, searchResult.Releases, 2)
	assert.Equal(t, "The.Matrix.1999.1080p.BluRay.x264", searchResult.Releases[0].Title, "first release should be 1080p version")

	// 3. POST /api/v1/content - create content entry
	contentResp := httpPost(t, env.api.URL+"/api/v1/content", map[string]any{
		"type":            "movie",
		"title":           "The Matrix",
		"year":            1999,
		"quality_profile": "hd",
	})
	require.Equal(t, http.StatusCreated, contentResp.StatusCode, "add content failed: %s", readBody(t, contentResp))

	var content contentResponse
	decodeJSON(t, contentResp, &content)

	require.NotZero(t, content.ID, "content ID should be set")

	// 4. POST /api/v1/grab - verify SABnzbd called, download record created
	grabResp := httpPost(t, env.api.URL+"/api/v1/grab", map[string]any{
		"content_id":   content.ID,
		"download_url": searchResult.Releases[0].DownloadURL,
		"title":        searchResult.Releases[0].Title,
		"indexer":      searchResult.Releases[0].Indexer,
	})
	require.Equal(t, http.StatusOK, grabResp.StatusCode, "grab failed: %s", readBody(t, grabResp))

	var grab grabResponse
	decodeJSON(t, grabResp, &grab)

	assert.NotZero(t, grab.DownloadID, "download ID should be set")
	assert.Equal(t, "queued", grab.Status)

	// 5. Verify DB state
	dl := queryDownload(t, env.db, content.ID)
	assert.Equal(t, "SABnzbd_nzo_abc123", dl.ClientID)
	assert.Equal(t, download.StatusQueued, dl.Status)
	assert.Equal(t, "The.Matrix.1999.1080p.BluRay.x264", dl.ReleaseName)
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
	require.NoError(t, env.manager.Refresh(ctx))

	// 4. Verify DB: download status updated to completed
	dl := queryDownload(t, env.db, contentID)
	assert.Equal(t, download.StatusCompleted, dl.Status)
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
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err, "open db")
	env.db = db

	// Apply schema
	_, err = db.Exec(testSchema)
	require.NoError(t, err, "apply schema")

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

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	assert.Equal(t, false, resp["connected"])
	assert.Equal(t, "Plex not configured", resp["error"])
}

func TestIntegration_FullHappyPath(t *testing.T) {
	env := setupIntegrationTest(t)

	// Configure all mocks upfront
	releases := []search.Release{
		mockRelease("Inception.2010.1080p.BluRay.x264", 15_000_000_000, "nzbgeek"),
	}
	env.mockIndexer.EXPECT().Search(gomock.Any(), gomock.Any()).Return(releases, nil)
	env.sabnzbdClientID = "SABnzbd_nzo_inception"

	// Phase 1: Search
	searchResp := httpPost(t, env.api.URL+"/api/v1/search", map[string]any{
		"query": "inception 2010",
		"type":  "movie",
	})
	require.Equal(t, http.StatusOK, searchResp.StatusCode, "search failed: %s", readBody(t, searchResp))

	var searchResult searchResponse
	decodeJSON(t, searchResp, &searchResult)

	require.NotEmpty(t, searchResult.Releases, "expected at least one release")

	// Phase 2: Add content
	contentResp := httpPost(t, env.api.URL+"/api/v1/content", map[string]any{
		"type":            "movie",
		"title":           "Inception",
		"year":            2010,
		"quality_profile": "hd",
	})
	require.Equal(t, http.StatusCreated, contentResp.StatusCode, "add content failed: %s", readBody(t, contentResp))

	var content contentResponse
	decodeJSON(t, contentResp, &content)

	// Phase 3: Grab
	grabResp := httpPost(t, env.api.URL+"/api/v1/grab", map[string]any{
		"content_id":   content.ID,
		"download_url": searchResult.Releases[0].DownloadURL,
		"title":        searchResult.Releases[0].Title,
		"indexer":      searchResult.Releases[0].Indexer,
	})
	require.Equal(t, http.StatusOK, grabResp.StatusCode, "grab failed: %s", readBody(t, grabResp))

	// Verify download created with queued status
	dl := queryDownload(t, env.db, content.ID)
	assert.Equal(t, download.StatusQueued, dl.Status, "initial status")

	// Phase 4: Simulate download completion
	env.sabnzbdStatus = &download.ClientStatus{
		ID:     "SABnzbd_nzo_inception",
		Name:   "Inception.2010.1080p.BluRay.x264",
		Status: download.StatusCompleted,
		Path:   "/downloads/complete/Inception.2010.1080p.BluRay.x264",
	}

	// Trigger refresh
	ctx := t.Context()
	require.NoError(t, env.manager.Refresh(ctx))

	// Verify final state: download completed
	dl = queryDownload(t, env.db, content.ID)
	assert.Equal(t, download.StatusCompleted, dl.Status, "final status")

	// Verify content still exists and can be retrieved via API
	getResp := httpGet(t, env.api.URL+"/api/v1/content/"+fmt.Sprintf("%d", content.ID))
	require.Equal(t, http.StatusOK, getResp.StatusCode, "get content failed: %s", readBody(t, getResp))
}

func TestIntegration_ManualImport(t *testing.T) {
	// Create temp directories for source and destination
	sourceDir := t.TempDir()
	movieRoot := t.TempDir()
	seriesRoot := t.TempDir()

	// Create a fake video file in a directory with a release name
	releaseName := "Back.to.the.Future.1985.1080p.BluRay.x264"
	releaseDir := filepath.Join(sourceDir, releaseName)
	require.NoError(t, os.MkdirAll(releaseDir, 0755), "create release dir")

	videoPath := filepath.Join(releaseDir, "back.to.the.future.mkv")
	videoContent := make([]byte, 5000) // 5KB fake video
	require.NoError(t, os.WriteFile(videoPath, videoContent, 0644), "create video file")

	// Set up in-memory database
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err, "open db")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(testSchema)
	require.NoError(t, err, "apply schema")

	// Create importer with test configuration
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importerCfg := importer.Config{
		MovieRoot:      movieRoot,
		SeriesRoot:     seriesRoot,
		MovieTemplate:  "", // Use default template
		SeriesTemplate: "", // Use default template
	}
	imp := importer.New(db, importerCfg, logger)

	// Create API server with importer configured
	cfg := Config{
		MovieRoot:  movieRoot,
		SeriesRoot: seriesRoot,
	}
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Importer:  imp,
	}
	srv, err := NewWithDeps(deps, cfg)
	require.NoError(t, err, "create server")

	// Create HTTP test server
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	// Make a manual import request
	importReq := map[string]any{
		"path":    releaseDir,
		"title":   "Back to the Future",
		"year":    1985,
		"type":    "movie",
		"quality": "1080p",
	}

	resp := httpPost(t, ts.URL+"/api/v1/import", importReq)
	require.Equal(t, http.StatusOK, resp.StatusCode, "import failed: %s", readBody(t, resp))

	var importResp importResponse
	decodeJSON(t, resp, &importResp)

	// Verify the response has correct fields
	assert.NotZero(t, importResp.FileID, "file_id should be set")
	assert.NotZero(t, importResp.ContentID, "content_id should be set")
	assert.Equal(t, videoPath, importResp.SourcePath)
	assert.Equal(t, int64(len(videoContent)), importResp.SizeBytes)

	// Verify dest_path is within movie root
	assert.True(t, strings.HasPrefix(importResp.DestPath, movieRoot), "dest_path = %q, should be under %q", importResp.DestPath, movieRoot)

	// Verify dest_path contains expected elements
	assert.Contains(t, importResp.DestPath, "Back to the Future", "dest_path should contain title")
	assert.Contains(t, importResp.DestPath, "1985", "dest_path should contain year")

	// Verify the file was copied to the destination
	_, err = os.Stat(importResp.DestPath)
	assert.False(t, os.IsNotExist(err), "destination file should exist")

	// Verify file content is correct
	destContent, err := os.ReadFile(importResp.DestPath)
	require.NoError(t, err, "read dest file")
	assert.Len(t, destContent, len(videoContent))

	// Verify content record was created with correct status
	var contentStatus string
	err = db.QueryRow("SELECT status FROM content WHERE id = ?", importResp.ContentID).Scan(&contentStatus)
	require.NoError(t, err, "query content status")
	assert.Equal(t, "available", contentStatus)

	// Verify file record was created in database
	var filePath string
	var fileQuality string
	err = db.QueryRow("SELECT path, quality FROM files WHERE id = ?", importResp.FileID).Scan(&filePath, &fileQuality)
	require.NoError(t, err, "query file record")
	assert.Equal(t, importResp.DestPath, filePath)
	assert.Equal(t, "1080p", fileQuality)

	// Verify history entry was created
	var historyCount int
	err = db.QueryRow("SELECT COUNT(*) FROM history WHERE content_id = ?", importResp.ContentID).Scan(&historyCount)
	require.NoError(t, err, "query history")
	assert.Equal(t, 1, historyCount)
}

// readBody is a helper to read response body for error messages
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(body)
}
