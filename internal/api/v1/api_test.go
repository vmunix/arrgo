// internal/api/v1/api_test.go
package v1

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/vmunix/arrgo/internal/api/v1/mocks"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
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

	body := `{"query":"test movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
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

func TestTriggerScan_NoPlex(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scan", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
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

	body := `{"query":"test movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
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
