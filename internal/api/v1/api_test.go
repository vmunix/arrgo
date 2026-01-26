// internal/api/v1/api_test.go
package v1

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/vmunix/arrgo/internal/api/v1/mocks"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
	"github.com/vmunix/arrgo/pkg/tvdb"
	"go.uber.org/mock/gomock"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	require.NoError(t, err, "open db")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(testSchema)
	require.NoError(t, err, "apply schema")
	return db
}

func TestNew(t *testing.T) {
	db := setupTestDB(t)
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
	}

	srv := New(db, cfg)
	require.NotNil(t, srv, "New returned nil")
	assert.NotNil(t, srv.deps.Library, "library store not initialized")
	assert.NotNil(t, srv.deps.Downloads, "download store not initialized")
	assert.NotNil(t, srv.deps.History, "history store not initialized")
}

func TestListContent_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listContentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
	assert.Zero(t, resp.Total)
}

func TestListContent_WithItems(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add test content
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(c))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listContentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, "Test Movie", resp.Items[0].Title)
}

func TestListContent_WithFilters(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add movie
	movie := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(movie))

	// Add series
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Series",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, srv.deps.Library.AddContent(series))

	// Filter by type
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content?type=movie", nil)
	w := httptest.NewRecorder()
	srv.listContent(w, req)

	var resp listContentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 1, "filter by type: items")

	// Filter by status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/content?status=available", nil)
	w = httptest.NewRecorder()
	srv.listContent(w, req)

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 1, "filter by status: items")
}

func TestGetContent_Found(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(c))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.getContent(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp contentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Test Movie", resp.Title)
}

func TestGetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.getContent(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAddContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{MovieRoot: "/movies", SeriesRoot: "/tv"})

	body := `{"type":"movie","title":"New Movie","year":2024,"quality_profile":"hd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.addContent(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "response body: %s", w.Body.String())

	var resp contentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotZero(t, resp.ID, "ID should be set")
	assert.Equal(t, "New Movie", resp.Title)
	assert.Equal(t, "wanted", resp.Status)
	assert.Equal(t, "/movies", resp.RootPath)
}

func TestAddContent_InvalidType(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"type":"invalid","title":"Test","year":2024,"quality_profile":"hd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.addContent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content first
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(c))

	body := `{"status":"available"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/content/1", strings.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.updateContent(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "response body: %s", w.Body.String())

	// Verify update
	updated, err := srv.deps.Library.GetContent(1)
	require.NoError(t, err)
	assert.Equal(t, library.StatusAvailable, updated.Status)
}

func TestDeleteContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content first
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(c))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/content/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteContent(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted
	_, err := srv.deps.Library.GetContent(1)
	assert.ErrorIs(t, err, library.ErrNotFound, "expected ErrNotFound")
}

func TestListEpisodes(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add series
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, srv.deps.Library.AddContent(series))

	// Add episodes
	for i := 1; i <= 3; i++ {
		ep := &library.Episode{
			ContentID: series.ID,
			Season:    1,
			Episode:   i,
			Title:     fmt.Sprintf("Episode %d", i),
			Status:    library.StatusWanted,
		}
		require.NoError(t, srv.deps.Library.AddEpisode(ep))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/1/episodes", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.listEpisodes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listEpisodesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 3)
}

func TestUpdateEpisode(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add series and episode
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, srv.deps.Library.AddContent(series))

	ep := &library.Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    library.StatusWanted,
	}
	require.NoError(t, srv.deps.Library.AddEpisode(ep))

	body := `{"status":"available"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/episodes/1", strings.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.updateEpisode(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "response body: %s", w.Body.String())

	// Verify update
	updated, err := srv.deps.Library.GetEpisode(1)
	require.NoError(t, err)
	assert.Equal(t, library.StatusAvailable, updated.Status)
}

func TestSearch_NoSearcher(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?query=test+movie", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSearch_MissingQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	// Need a searcher for the endpoint to be available
	mockSearcher := mocks.NewMockSearcher(ctrl)
	srv.deps.Searcher = mockSearcher

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// GET without query param
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "MISSING_QUERY", errResp.Code)
}

func TestGrab_NoManager(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"content_id":1,"download_url":"http://example.com/nzb","title":"Test","indexer":"TestIndexer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestListDownloads_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads", nil)
	w := httptest.NewRecorder()

	srv.listDownloads(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listDownloadsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
}

func TestGetDownload_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.getDownload(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteDownload_NoManager(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/downloads/1", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestListHistory_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history", nil)
	w := httptest.NewRecorder()

	srv.listHistory(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
}

func TestListFiles_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	w := httptest.NewRecorder()

	srv.listFiles(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listFilesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
}

func TestDeleteFile(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content and file
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(c))

	f := &library.File{
		ContentID: c.ID,
		Path:      "/movies/test.mkv",
		SizeBytes: 1000,
		Quality:   "1080p",
		Source:    "test",
	}
	require.NoError(t, srv.deps.Library.AddFile(f))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteFile(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestGetStatus(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()

	srv.getStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp statusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
}

func TestListProfiles(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{
		QualityProfiles: map[string][]string{
			"hd":  {"1080p bluray", "1080p webdl"},
			"uhd": {"2160p bluray"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	w := httptest.NewRecorder()

	srv.listProfiles(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listProfilesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Profiles, 2)
}

func TestSearch_WithMockSearcher(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	// Create mock searcher
	mockSearcher := mocks.NewMockSearcher(ctrl)
	srv.deps.Searcher = mockSearcher

	// Set up expectation
	mockSearcher.EXPECT().
		Search(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&search.Result{
			Releases: []*search.Release{
				{Title: "Test Movie", Indexer: "TestIndexer"},
			},
		}, nil)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?query=test+movie", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp searchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Releases, 1)
	assert.Equal(t, "Test Movie", resp.Releases[0].Title)
}

func TestListEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Create event log and attach to server
	eventLog := events.NewEventLog(db)
	srv.deps.EventLog = eventLog

	// Insert a test event using EventLog.Append
	testEvent := events.NewBaseEvent("test.event", "content", 1)
	_, err := eventLog.Append(testEvent)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	w := httptest.NewRecorder()

	srv.listEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listEventsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 50, resp.Limit) // default limit
	assert.Equal(t, 0, resp.Offset) // default offset
	assert.Equal(t, "test.event", resp.Items[0].EventType)
	assert.Equal(t, "content", resp.Items[0].EntityType)
	assert.Equal(t, int64(1), resp.Items[0].EntityID)
}

func TestListEvents_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Create event log with no events
	eventLog := events.NewEventLog(db)
	srv.deps.EventLog = eventLog

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	w := httptest.NewRecorder()

	srv.listEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listEventsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
	assert.Zero(t, resp.Total)
	assert.Equal(t, 50, resp.Limit) // default limit
	assert.Equal(t, 0, resp.Offset) // default offset
}

func TestListDownloadEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Create event log and attach to server
	eventLog := events.NewEventLog(db)
	srv.deps.EventLog = eventLog

	// Add content first (required for download foreign key)
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(c))

	// Add a download
	d := &download.Download{
		ContentID:   c.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "test-client-id",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "TestIndexer",
	}
	require.NoError(t, srv.deps.Downloads.Add(d))

	// Insert event for this download
	testEvent := events.NewBaseEvent("download.started", "download", d.ID)
	_, err := eventLog.Append(testEvent)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/1/events", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.listDownloadEvents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listEventsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, "download.started", resp.Items[0].EventType)
	assert.Equal(t, "download", resp.Items[0].EntityType)
}

func TestListDownloadEvents_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Create event log
	eventLog := events.NewEventLog(db)
	srv.deps.EventLog = eventLog

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/999/events", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.listDownloadEvents(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDashboard_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add movies and series
	movie1 := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie 1",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(movie1))

	movie2 := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie 2",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(movie2))

	series1 := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Test Series",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, srv.deps.Library.AddContent(series1))

	// Add downloads in various states
	dlQueued := &download.Download{
		ContentID:   movie1.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "queued-1",
		Status:      download.StatusQueued,
		ReleaseName: "Test.Movie.1.2024.1080p",
		Indexer:     "TestIndexer",
	}
	require.NoError(t, srv.deps.Downloads.Add(dlQueued))

	dlDownloading := &download.Download{
		ContentID:   movie2.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "downloading-1",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2.2024.1080p",
		Indexer:     "TestIndexer",
	}
	require.NoError(t, srv.deps.Downloads.Add(dlDownloading))

	dlFailed := &download.Download{
		ContentID:   series1.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "failed-1",
		Status:      download.StatusFailed,
		ReleaseName: "Test.Series.S01E01.1080p",
		Indexer:     "TestIndexer",
	}
	require.NoError(t, srv.deps.Downloads.Add(dlFailed))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()

	srv.getDashboard(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DashboardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Verify version and connections
	assert.Equal(t, "0.1.0", resp.Version)
	assert.True(t, resp.Connections.Server)
	assert.False(t, resp.Connections.Plex)    // No Plex configured
	assert.False(t, resp.Connections.SABnzbd) // No Manager configured

	// Verify download counts
	assert.Equal(t, 1, resp.Downloads.Queued)
	assert.Equal(t, 1, resp.Downloads.Downloading)
	assert.Equal(t, 1, resp.Downloads.Failed)
	assert.Equal(t, 0, resp.Downloads.Completed)
	assert.Equal(t, 0, resp.Downloads.Importing)
	assert.Equal(t, 0, resp.Downloads.Imported)
	assert.Equal(t, 0, resp.Downloads.Cleaned)

	// Verify library counts
	assert.Equal(t, 2, resp.Library.Movies)
	assert.Equal(t, 1, resp.Library.Series)
}

func TestVerify_NoProblems(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// No downloads, no problems expected
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verify", nil)
	w := httptest.NewRecorder()

	srv.verify(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp VerifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 0, resp.Checked)
	assert.Equal(t, 0, resp.Passed)
	assert.Empty(t, resp.Problems)
	// Connections should be false since no Plex or Manager configured
	assert.False(t, resp.Connections.Plex)
	assert.False(t, resp.Connections.SABnzbd)
}

func TestVerify_WithDownloadID(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content for downloads
	content := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(content))

	// Add multiple downloads (using Active states so verify finds them)
	dl1 := &download.Download{
		ContentID:   content.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "client-1",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.Release1",
		Indexer:     "TestIndexer",
	}
	require.NoError(t, srv.deps.Downloads.Add(dl1))

	dl2 := &download.Download{
		ContentID:   content.ID,
		Client:      download.ClientSABnzbd,
		ClientID:    "client-2",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024.1080p.Release2",
		Indexer:     "TestIndexer",
	}
	require.NoError(t, srv.deps.Downloads.Add(dl2))

	// Verify with specific ID filter (should only check dl2)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/verify?id=%d", dl2.ID), nil)
	w := httptest.NewRecorder()

	srv.verify(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp VerifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Should only have checked 1 download (the one we filtered for)
	assert.Equal(t, 1, resp.Checked)
	// Without manager, verify doesn't find problems for completed status
	assert.Equal(t, 1, resp.Passed)
	assert.Empty(t, resp.Problems)
}

func TestGetPlexStatus_Connected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	// Setup Plex mock expectations
	mockPlex.EXPECT().
		GetIdentity(gomock.Any()).
		Return(&importer.Identity{
			Name:    "Test Plex Server",
			Version: "1.32.0",
		}, nil)

	mockPlex.EXPECT().
		GetSections(gomock.Any()).
		Return([]importer.Section{
			{
				Key:       "1",
				Title:     "Movies",
				Type:      "movie",
				Locations: []importer.Location{{Path: "/media/movies"}},
				ScannedAt: 1700000000,
			},
			{
				Key:       "2",
				Title:     "TV Shows",
				Type:      "show",
				Locations: []importer.Location{{Path: "/media/tv"}},
				ScannedAt: 1700000001,
			},
		}, nil)

	// GetLibraryCount is called for each section
	mockPlex.EXPECT().
		GetLibraryCount(gomock.Any(), "1").
		Return(150, nil)
	mockPlex.EXPECT().
		GetLibraryCount(gomock.Any(), "2").
		Return(50, nil)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/status", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp plexStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Connected)
	assert.Equal(t, "Test Plex Server", resp.ServerName)
	assert.Equal(t, "1.32.0", resp.Version)
	assert.Empty(t, resp.Error)
	assert.Len(t, resp.Libraries, 2)

	// Verify first library
	assert.Equal(t, "1", resp.Libraries[0].Key)
	assert.Equal(t, "Movies", resp.Libraries[0].Title)
	assert.Equal(t, "movie", resp.Libraries[0].Type)
	assert.Equal(t, 150, resp.Libraries[0].ItemCount)
	assert.Equal(t, "/media/movies", resp.Libraries[0].Location)
	assert.Equal(t, int64(1700000000), resp.Libraries[0].ScannedAt)

	// Verify second library
	assert.Equal(t, "2", resp.Libraries[1].Key)
	assert.Equal(t, "TV Shows", resp.Libraries[1].Title)
	assert.Equal(t, "show", resp.Libraries[1].Type)
	assert.Equal(t, 50, resp.Libraries[1].ItemCount)
}

func TestGetPlexStatus_NotConfigured(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/status", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp plexStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.False(t, resp.Connected)
	assert.Empty(t, resp.ServerName)
	assert.Empty(t, resp.Version)
	assert.Empty(t, resp.Libraries)
	assert.Equal(t, "Plex not configured", resp.Error)
}

func TestScanPlexLibraries_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	// Setup mock expectations
	mockPlex.EXPECT().
		GetSections(gomock.Any()).
		Return([]importer.Section{
			{Key: "1", Title: "Movies", Type: "movie"},
			{Key: "2", Title: "TV Shows", Type: "show"},
		}, nil)

	// Scan both libraries
	mockPlex.EXPECT().
		RefreshLibrary(gomock.Any(), "1").
		Return(nil)
	mockPlex.EXPECT().
		RefreshLibrary(gomock.Any(), "2").
		Return(nil)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Request to scan all libraries (empty array)
	body := `{"libraries":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plex/scan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp plexScanResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Len(t, resp.Scanned, 2)
	assert.Contains(t, resp.Scanned, "Movies")
	assert.Contains(t, resp.Scanned, "TV Shows")
}

func TestScanPlexLibraries_NoPlex(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"libraries":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plex/scan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "SERVICE_UNAVAILABLE", resp.Code)
	assert.Equal(t, "Plex not configured", resp.Error)
}

func TestSearchPlex_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	// Setup mock expectations
	mockPlex.EXPECT().
		Search(gomock.Any(), "inception").
		Return([]importer.PlexItem{
			{
				RatingKey: "12345",
				Title:     "Inception",
				Year:      2010,
				Type:      "movie",
				AddedAt:   1700000000,
				FilePath:  "/media/movies/Inception (2010)/Inception.mkv",
			},
			{
				RatingKey: "12346",
				Title:     "Inception: The Cobol Job",
				Year:      2010,
				Type:      "movie",
				AddedAt:   1700000001,
				FilePath:  "/media/movies/Inception The Cobol Job (2010)/cobol.mkv",
			},
		}, nil)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/search?query=inception", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp plexSearchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "inception", resp.Query)
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Items, 2)

	// Verify first item
	assert.Equal(t, "Inception", resp.Items[0].Title)
	assert.Equal(t, 2010, resp.Items[0].Year)
	assert.Equal(t, "movie", resp.Items[0].Type)
	assert.Equal(t, int64(1700000000), resp.Items[0].AddedAt)
	assert.Equal(t, "/media/movies/Inception (2010)/Inception.mkv", resp.Items[0].FilePath)
	assert.False(t, resp.Items[0].Tracked)
	assert.Nil(t, resp.Items[0].ContentID)

	// Verify second item
	assert.Equal(t, "Inception: The Cobol Job", resp.Items[1].Title)
	assert.Equal(t, 2010, resp.Items[1].Year)
}

func TestSearchPlex_MissingQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	// No EXPECT calls - search should not be called when query is missing

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/search", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "MISSING_QUERY", resp.Code)
	assert.Equal(t, "query parameter is required", resp.Error)
}

func TestListPlexLibraryItems_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	// Setup mock expectations
	mockPlex.EXPECT().
		FindSectionByName(gomock.Any(), "Movies").
		Return(&importer.Section{
			Key:   "1",
			Title: "Movies",
			Type:  "movie",
		}, nil)

	mockPlex.EXPECT().
		ListLibraryItems(gomock.Any(), "1").
		Return([]importer.PlexItem{
			{
				RatingKey: "100",
				Title:     "The Matrix",
				Year:      1999,
				Type:      "movie",
				AddedAt:   1699000000,
				FilePath:  "/media/movies/The Matrix (1999)/matrix.mkv",
			},
			{
				RatingKey: "101",
				Title:     "The Matrix Reloaded",
				Year:      2003,
				Type:      "movie",
				AddedAt:   1699000001,
				FilePath:  "/media/movies/The Matrix Reloaded (2003)/reloaded.mkv",
			},
			{
				RatingKey: "102",
				Title:     "The Matrix Revolutions",
				Year:      2003,
				Type:      "movie",
				AddedAt:   1699000002,
				FilePath:  "/media/movies/The Matrix Revolutions (2003)/revolutions.mkv",
			},
		}, nil)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/libraries/Movies/items", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp plexListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "Movies", resp.Library)
	assert.Equal(t, 3, resp.Total)
	assert.Len(t, resp.Items, 3)

	// Verify first item
	assert.Equal(t, "The Matrix", resp.Items[0].Title)
	assert.Equal(t, 1999, resp.Items[0].Year)
	assert.Equal(t, "movie", resp.Items[0].Type)
	assert.Equal(t, int64(1699000000), resp.Items[0].AddedAt)
	assert.Equal(t, "/media/movies/The Matrix (1999)/matrix.mkv", resp.Items[0].FilePath)
	assert.False(t, resp.Items[0].Tracked)
	assert.Nil(t, resp.Items[0].ContentID)

	// Verify second and third items exist
	assert.Equal(t, "The Matrix Reloaded", resp.Items[1].Title)
	assert.Equal(t, "The Matrix Revolutions", resp.Items[2].Title)
}

func TestListPlexLibraryItems_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockPlex := mocks.NewMockPlexClient(ctrl)

	// Return nil section (not found)
	mockPlex.EXPECT().
		FindSectionByName(gomock.Any(), "NonExistent").
		Return(nil, nil)

	// GetSections called to list available libraries
	mockPlex.EXPECT().
		GetSections(gomock.Any()).
		Return([]importer.Section{
			{Key: "1", Title: "Movies", Type: "movie"},
			{Key: "2", Title: "TV Shows", Type: "show"},
		}, nil)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Plex:      mockPlex,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plex/libraries/NonExistent/items", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "LIBRARY_NOT_FOUND", resp.Code)
	assert.Contains(t, resp.Error, "NonExistent")
	assert.Contains(t, resp.Error, "Movies")
	assert.Contains(t, resp.Error, "TV Shows")
}

// mockIndexer implements IndexerAPI for testing
type mockIndexer struct {
	name string
	url  string
}

func (m *mockIndexer) Name() string                 { return m.name }
func (m *mockIndexer) URL() string                  { return m.url }
func (m *mockIndexer) Caps(_ context.Context) error { return nil }

func TestCheckLibrary_Success(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// Add content with different statuses
	available := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Available Movie",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(available))

	wanted := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Wanted Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, srv.deps.Library.AddContent(wanted))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/check", nil)
	w := httptest.NewRecorder()

	srv.checkLibrary(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Items, 2)

	// Find each item
	var foundAvailable, foundWanted bool
	for _, item := range resp.Items {
		if item.Title == "Available Movie" {
			foundAvailable = true
			assert.Equal(t, "available", item.Status)
			assert.Equal(t, int64(1), item.ID)
			// Status is 'available' but no files - should have issue
			assert.Contains(t, item.Issues, "Status is 'available' but no files in database")
		}
		if item.Title == "Wanted Movie" {
			foundWanted = true
			assert.Equal(t, "wanted", item.Status)
			assert.Equal(t, int64(2), item.ID)
			// Status is 'wanted' with no files - no issue
			assert.Empty(t, item.Issues)
		}
	}
	assert.True(t, foundAvailable, "should find Available Movie")
	assert.True(t, foundWanted, "should find Wanted Movie")

	// 'available' with no files = issue, 'wanted' with no files = healthy
	assert.Equal(t, 1, resp.Healthy)
	assert.Equal(t, 1, resp.WithIssues)
}

func TestCheckLibrary_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// No content added

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/check", nil)
	w := httptest.NewRecorder()

	srv.checkLibrary(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Items)
	assert.Equal(t, 0, resp.Healthy)
	assert.Equal(t, 0, resp.WithIssues)
}

func TestListIndexers_Success(t *testing.T) {
	db := setupTestDB(t)

	// Create mock indexers
	indexers := []IndexerAPI{
		&mockIndexer{name: "NZBgeek", url: "https://api.nzbgeek.info"},
		&mockIndexer{name: "DrunkenSlug", url: "https://api.drunkenslug.com"},
	}

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Indexers:  indexers,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indexers", nil)
	w := httptest.NewRecorder()

	srv.listIndexers(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listIndexersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Len(t, resp.Indexers, 2)
	assert.Equal(t, "NZBgeek", resp.Indexers[0].Name)
	assert.Equal(t, "https://api.nzbgeek.info", resp.Indexers[0].URL)
	assert.Equal(t, "DrunkenSlug", resp.Indexers[1].Name)
	assert.Equal(t, "https://api.drunkenslug.com", resp.Indexers[1].URL)
}

func TestListIndexers_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	// No indexers configured (srv.deps.Indexers is nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indexers", nil)
	w := httptest.NewRecorder()

	srv.listIndexers(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listIndexersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Should return empty array, not 503
	assert.Empty(t, resp.Indexers)
}

func TestRetryDownload_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockSearcher := mocks.NewMockSearcher(ctrl)
	mockManager := mocks.NewMockDownloadManager(ctrl)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Searcher:  mockSearcher,
		Manager:   mockManager,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Try to retry a non-existent download
	req := httptest.NewRequest(http.MethodPost, "/api/v1/downloads/999/retry", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "NOT_FOUND", resp.Code)
	assert.Equal(t, "Download not found", resp.Error)
}
func TestLibraryImport_ValidationErrors(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	tests := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing source",
			body:     `{"library": "Movies"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "source is required",
		},
		{
			name:     "invalid source",
			body:     `{"source": "invalid", "library": "Movies"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "unsupported source",
		},
		{
			name:     "missing library",
			body:     `{"source": "plex"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "library is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantErr)
		})
	}
}

func TestLibraryImport_PlexNotConfigured(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})
	srv.deps.Plex = nil // Ensure Plex is not configured

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"source": "plex", "library": "Movies"}` //nolint:goconst
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Plex not configured")
}

func TestLibraryImport_LibraryNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	mockPlex := mocks.NewMockPlexClient(ctrl)
	srv.deps.Plex = mockPlex

	// Mock FindSectionByName to return nil (not found)
	mockPlex.EXPECT().
		FindSectionByName(gomock.Any(), "NonExistent").
		Return(nil, nil)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"source": "plex", "library": "NonExistent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Plex library not found")
}

func TestLibraryImport_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	mockPlex := mocks.NewMockPlexClient(ctrl)
	srv.deps.Plex = mockPlex

	// Mock FindSectionByName to return a section
	mockPlex.EXPECT().
		FindSectionByName(gomock.Any(), "Movies").
		Return(&importer.Section{Key: "1", Title: "Movies", Type: "movie"}, nil)

	// Mock ListLibraryItems to return some items
	mockPlex.EXPECT().
		ListLibraryItems(gomock.Any(), "1").
		Return([]importer.PlexItem{
			{Title: "Test Movie", Year: 2024, Type: "movie", FilePath: "/movies/Test.Movie.2024.mkv"},
		}, nil)

	// Mock TranslateToLocal (identity transform)
	mockPlex.EXPECT().
		TranslateToLocal("/movies/Test.Movie.2024.mkv").
		Return("/movies/Test.Movie.2024.mkv")

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Use dry_run since we're just testing response structure (no real files)
	body := `{"source": "plex", "library": "Movies", "dry_run": true}` //nolint:goconst
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Verify response structure has expected fields
	assert.NotNil(t, resp.Imported)
	assert.NotNil(t, resp.Skipped)
	assert.NotNil(t, resp.Errors)
}

func TestLibraryImport_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	mockPlex := mocks.NewMockPlexClient(ctrl)
	srv.deps.Plex = mockPlex

	// Mock Plex to return section and items
	mockPlex.EXPECT().
		FindSectionByName(gomock.Any(), "Movies").
		Return(&importer.Section{Key: "1", Title: "Movies"}, nil)
	mockPlex.EXPECT().
		ListLibraryItems(gomock.Any(), "1").
		Return([]importer.PlexItem{
			{Title: "Test Movie", Year: 2024, Type: "movie", FilePath: "/data/media/movies/Test.Movie.2024.2160p.BluRay.mkv"},
		}, nil)
	mockPlex.EXPECT().
		TranslateToLocal("/data/media/movies/Test.Movie.2024.2160p.BluRay.mkv").
		Return("/srv/media/movies/Test.Movie.2024.2160p.BluRay.mkv")

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"source": "plex", "library": "Movies", "dry_run": true}` //nolint:goconst
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Dry run should show what would be imported
	assert.Len(t, resp.Imported, 1)
	assert.Equal(t, "Test Movie", resp.Imported[0].Title)
	assert.Equal(t, 2024, resp.Imported[0].Year)
	assert.Equal(t, "uhd", resp.Imported[0].Quality) // parsed from 2160p

	// ContentID should be 0 because dry_run doesn't create records
	assert.Zero(t, resp.Imported[0].ContentID)

	// Summary should be correct
	assert.Equal(t, 1, resp.Summary.Imported)
	assert.Equal(t, 0, resp.Summary.Skipped)
	assert.Equal(t, 0, resp.Summary.Errors)
}

func TestLibraryImport_SkipsAlreadyTracked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	mockPlex := mocks.NewMockPlexClient(ctrl)
	srv.deps.Plex = mockPlex

	// Pre-create content that should be skipped
	content := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Existing Movie",
		Year:           2020,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
	}
	err := srv.deps.Library.AddContent(content)
	require.NoError(t, err)

	mockPlex.EXPECT().
		FindSectionByName(gomock.Any(), "Movies").
		Return(&importer.Section{Key: "1", Title: "Movies"}, nil)
	mockPlex.EXPECT().
		ListLibraryItems(gomock.Any(), "1").
		Return([]importer.PlexItem{
			{Title: "Existing Movie", Year: 2020, Type: "movie", FilePath: "/data/media/movies/file.mkv"},
		}, nil)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"source": "plex", "library": "Movies", "dry_run": true}` //nolint:goconst
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Empty(t, resp.Imported)
	assert.Len(t, resp.Skipped, 1)
	assert.Equal(t, "Existing Movie", resp.Skipped[0].Title)
	assert.Equal(t, "already tracked", resp.Skipped[0].Reason)
	assert.Equal(t, content.ID, resp.Skipped[0].ContentID)
}

func TestLibraryImport_CreatesRecords(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	mockPlex := mocks.NewMockPlexClient(ctrl)
	srv.deps.Plex = mockPlex

	// Create a temp file for the test
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "New.Movie.2024.1080p.BluRay.mkv")
	err := os.WriteFile(testFile, []byte("test content for size"), 0644)
	require.NoError(t, err)

	mockPlex.EXPECT().FindSectionByName(gomock.Any(), "Movies").Return(&importer.Section{Key: "1", Title: "Movies"}, nil)
	mockPlex.EXPECT().ListLibraryItems(gomock.Any(), "1").Return([]importer.PlexItem{
		{Title: "New Movie", Year: 2024, Type: "movie", FilePath: "/data/media/movies/New.Movie.2024.1080p.BluRay.mkv"},
	}, nil)
	// TranslateToLocal is called twice: once in processPlexImport for quality parsing, once in createImportedContent
	mockPlex.EXPECT().TranslateToLocal("/data/media/movies/New.Movie.2024.1080p.BluRay.mkv").Return(testFile).Times(2)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"source": "plex", "library": "Movies"}` //nolint:goconst
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Imported, 1)
	assert.NotZero(t, resp.Imported[0].ContentID)

	// Verify content was created
	content, err := srv.deps.Library.GetContent(resp.Imported[0].ContentID)
	require.NoError(t, err)
	assert.Equal(t, "New Movie", content.Title)
	assert.Equal(t, 2024, content.Year)
	assert.Equal(t, library.StatusAvailable, content.Status)
	assert.Equal(t, "hd", content.QualityProfile) // from 1080p

	// Verify file was created
	files, _, err := srv.deps.Library.ListFiles(library.FileFilter{ContentID: &content.ID})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, testFile, files[0].Path)
	assert.Equal(t, "1080p", files[0].Quality)
	assert.Equal(t, "plex-import", files[0].Source)
	assert.Equal(t, int64(21), files[0].SizeBytes) // "test content for size" is 21 bytes
}

func TestLibraryImport_FileStatError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	mockPlex := mocks.NewMockPlexClient(ctrl)
	srv.deps.Plex = mockPlex

	mockPlex.EXPECT().FindSectionByName(gomock.Any(), "Movies").Return(&importer.Section{Key: "1", Title: "Movies"}, nil)
	mockPlex.EXPECT().ListLibraryItems(gomock.Any(), "1").Return([]importer.PlexItem{
		{Title: "Missing File", Year: 2024, Type: "movie", FilePath: "/data/media/movies/Missing.mkv"},
	}, nil)
	// Return a path that doesn't exist (called twice: once for quality parsing, once for content creation)
	mockPlex.EXPECT().TranslateToLocal("/data/media/movies/Missing.mkv").Return("/nonexistent/path/Missing.mkv").Times(2)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"source": "plex", "library": "Movies"}` //nolint:goconst
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Should be in errors, not imported
	assert.Empty(t, resp.Imported)
	assert.Len(t, resp.Errors, 1)
	assert.Equal(t, "Missing File", resp.Errors[0].Title)
	assert.Contains(t, resp.Errors[0].Error, "cannot access file")
}

func TestGrab_ContentNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(ctrl)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab with non-existent content_id
	body := `{"content_id":999,"download_url":"http://example.com/nzb","title":"Test","indexer":"TestIndexer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "NOT_FOUND", resp.Code)
}

func TestGrab_MissingRequiredFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(ctrl)

	// Add content so content_id validation passes
	store := library.NewStore(db)
	c := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Test Movie",
		Year:           2024,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, store.AddContent(c))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing content_id",
			body:    `{"download_url":"http://example.com/nzb","title":"Test","indexer":"TestIndexer"}`,
			wantErr: "content_id",
		},
		{
			name:    "missing download_url",
			body:    `{"content_id":1,"title":"Test","indexer":"TestIndexer"}`,
			wantErr: "download_url",
		},
		{
			name:    "missing title",
			body:    `{"content_id":1,"download_url":"http://example.com/nzb","indexer":"TestIndexer"}`,
			wantErr: "title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantErr)
		})
	}
}

func TestGrab_SeriesWithEpisodeDetection(t *testing.T) {
	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(gomock.NewController(t))

	// Create event bus to capture published events
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Subscribe to capture events
	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Add series content
	store := library.NewStore(db)
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Breaking Bad",
		Year:           2008,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab with release name that contains episode info
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Breaking.Bad.S05E12.1080p.BluRay.x264-DEMAND",
		"indexer": "NZBgeek"
	}`, series.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "response body: %s", w.Body.String())

	// Verify the event was published
	select {
	case evt := <-eventCh:
		grabEvt, ok := evt.(*events.GrabRequested)
		require.True(t, ok, "expected GrabRequested event")
		assert.Equal(t, series.ID, grabEvt.ContentID)
		assert.NotNil(t, grabEvt.Season)
		assert.Equal(t, 5, *grabEvt.Season)
		assert.Len(t, grabEvt.EpisodeIDs, 1)
		assert.NotNil(t, grabEvt.EpisodeID) // Backward compatibility
		assert.False(t, grabEvt.IsCompleteSeason)
	default:
		t.Fatal("expected event to be published")
	}

	// Verify episode was created in database
	episodes, _, err := store.ListEpisodes(library.EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err)
	assert.Len(t, episodes, 1)
	assert.Equal(t, 5, episodes[0].Season)
	assert.Equal(t, 12, episodes[0].Episode)
}

func TestGrab_SeasonPack(t *testing.T) {
	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(gomock.NewController(t))

	// Create event bus
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Add series content
	store := library.NewStore(db)
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Peaky Blinders",
		Year:           2013,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab a season pack
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Peaky.Blinders.S01.1080p.BluRay.x264-DEMAND",
		"indexer": "NZBgeek"
	}`, series.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "response body: %s", w.Body.String())

	// Verify the event
	select {
	case evt := <-eventCh:
		grabEvt, ok := evt.(*events.GrabRequested)
		require.True(t, ok, "expected GrabRequested event")
		assert.Equal(t, series.ID, grabEvt.ContentID)
		assert.NotNil(t, grabEvt.Season)
		assert.Equal(t, 1, *grabEvt.Season)
		assert.True(t, grabEvt.IsCompleteSeason)
		assert.Empty(t, grabEvt.EpisodeIDs, "season pack should not have episode IDs yet")
	default:
		t.Fatal("expected event to be published")
	}
}

func TestGrab_SeriesNoEpisodeInfo(t *testing.T) {
	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(gomock.NewController(t))

	// Create event bus
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Add series content
	store := library.NewStore(db)
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Game of Thrones",
		Year:           2011,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab without season/episode info (should fail)
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Game.of.Thrones.1080p.BluRay.x264-DEMAND",
		"indexer": "NZBgeek"
	}`, series.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "INVALID_RELEASE", resp.Code)
	assert.Contains(t, resp.Error, "cannot determine season")
}

func TestGrab_SeriesWithOverrides(t *testing.T) {
	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(gomock.NewController(t))

	// Create event bus
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Add series content
	store := library.NewStore(db)
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "The Mandalorian",
		Year:           2019,
		Status:         library.StatusWanted,
		QualityProfile: "uhd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab with overrides (ignoring the release name parsing)
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "The.Mandalorian.2160p.WEB-DL.DDP5.1.Atmos.HDR.x265",
		"indexer": "NZBgeek",
		"season": 2,
		"episodes": [1, 2, 3]
	}`, series.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "response body: %s", w.Body.String())

	// Verify the event uses overrides
	select {
	case evt := <-eventCh:
		grabEvt, ok := evt.(*events.GrabRequested)
		require.True(t, ok, "expected GrabRequested event")
		assert.NotNil(t, grabEvt.Season)
		assert.Equal(t, 2, *grabEvt.Season)
		assert.Len(t, grabEvt.EpisodeIDs, 3)
		assert.False(t, grabEvt.IsCompleteSeason)
	default:
		t.Fatal("expected event to be published")
	}

	// Verify episodes were created
	episodes, _, err := store.ListEpisodes(library.EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err)
	assert.Len(t, episodes, 3)
}

func TestGrab_MovieIgnoresEpisodeDetection(t *testing.T) {
	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(gomock.NewController(t))

	// Create event bus
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Add movie content
	store := library.NewStore(db)
	movie := &library.Content{
		Type:           library.ContentTypeMovie,
		Title:          "Inception",
		Year:           2010,
		Status:         library.StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	require.NoError(t, store.AddContent(movie))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab movie (should not trigger episode detection)
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "Inception.2010.1080p.BluRay.x264-DEMAND",
		"indexer": "NZBgeek"
	}`, movie.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "response body: %s", w.Body.String())

	// Verify the event has no episode info
	select {
	case evt := <-eventCh:
		grabEvt, ok := evt.(*events.GrabRequested)
		require.True(t, ok, "expected GrabRequested event")
		assert.Nil(t, grabEvt.Season)
		assert.Nil(t, grabEvt.EpisodeID)
		assert.Empty(t, grabEvt.EpisodeIDs)
		assert.False(t, grabEvt.IsCompleteSeason)
	default:
		t.Fatal("expected event to be published")
	}
}

func TestGrab_MultiEpisodeRelease(t *testing.T) {
	db := setupTestDB(t)
	mockManager := mocks.NewMockDownloadManager(gomock.NewController(t))

	// Create event bus
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	eventCh := bus.Subscribe(events.EventGrabRequested, 10)

	// Add series content
	store := library.NewStore(db)
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "House of the Dragon",
		Year:           2022,
		Status:         library.StatusWanted,
		QualityProfile: "uhd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	deps := ServerDeps{
		Library:   store,
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
		Manager:   mockManager,
		Bus:       bus,
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Grab multi-episode release (e.g., S01E01-E03)
	body := fmt.Sprintf(`{
		"content_id": %d,
		"download_url": "http://example.com/nzb",
		"title": "House.of.the.Dragon.S01E01-E03.2160p.HMAX.WEB-DL.x265",
		"indexer": "NZBgeek"
	}`, series.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code, "response body: %s", w.Body.String())

	// Verify the event
	select {
	case evt := <-eventCh:
		grabEvt, ok := evt.(*events.GrabRequested)
		require.True(t, ok, "expected GrabRequested event")
		assert.NotNil(t, grabEvt.Season)
		assert.Equal(t, 1, *grabEvt.Season)
		assert.Len(t, grabEvt.EpisodeIDs, 3, "should have 3 episode IDs for E01-E03")
		assert.Nil(t, grabEvt.EpisodeID, "EpisodeID should be nil for multi-episode")
		assert.False(t, grabEvt.IsCompleteSeason)
	default:
		t.Fatal("expected event to be published")
	}

	// Verify episodes were created
	episodes, _, err := store.ListEpisodes(library.EpisodeFilter{ContentID: &series.ID})
	require.NoError(t, err)
	assert.Len(t, episodes, 3)
}

func TestTVDBSearch_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockTVDB := mocks.NewMockTVDBService(ctrl)

	// Set up expectation for search
	mockTVDB.EXPECT().
		Search(gomock.Any(), "breaking bad").
		Return([]tvdb.SearchResult{
			{
				ID:       81189,
				Name:     "Breaking Bad",
				Year:     2008,
				Status:   "Ended",
				Overview: "A high school chemistry teacher diagnosed with inoperable lung cancer...",
				Network:  "AMC",
			},
		}, nil)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)
	srv.SetTVDB(mockTVDB)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tvdb/search?q=breaking+bad", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []tvdb.SearchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	assert.Len(t, results, 1)
	assert.Equal(t, 81189, results[0].ID)
	assert.Equal(t, "Breaking Bad", results[0].Name)
	assert.Equal(t, 2008, results[0].Year)
}

func TestTVDBSearch_MissingQuery(t *testing.T) {
	db := setupTestDB(t)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tvdb/search", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error, "query parameter is required")
}

func TestTVDBSearch_NotConfigured(t *testing.T) {
	db := setupTestDB(t)

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)
	// Note: Not calling SetTVDB, so tvdbSvc remains nil

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tvdb/search?q=test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error, "TVDB not configured")
}

func TestTVDBSearch_SearchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	mockTVDB := mocks.NewMockTVDBService(ctrl)

	// Set up expectation for search failure
	mockTVDB.EXPECT().
		Search(gomock.Any(), "test").
		Return(nil, fmt.Errorf("TVDB API error"))

	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
	}
	srv, err := NewWithDeps(deps, Config{})
	require.NoError(t, err)
	srv.SetTVDB(mockTVDB)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tvdb/search?q=test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp errorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error, "TVDB API error")
}
