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
