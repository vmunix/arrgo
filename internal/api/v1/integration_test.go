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
		MovieRoot:       "/movies",
		SeriesRoot:      "/tv",
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
