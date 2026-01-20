// internal/api/v1/api_test.go
package v1

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/vmunix/arrgo/internal/library"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db
}

func TestNew(t *testing.T) {
	db := setupTestDB(t)
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
	}

	srv := New(db, cfg)
	if srv == nil {
		t.Fatal("New returned nil")
	}
	if srv.library == nil {
		t.Error("library store not initialized")
	}
	if srv.downloads == nil {
		t.Error("download store not initialized")
	}
	if srv.history == nil {
		t.Error("history store not initialized")
	}
}

func TestListContent_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
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
	if err := srv.library.AddContent(c); err != nil {
		t.Fatalf("add content: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("items = %d, want 1", len(resp.Items))
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
	if resp.Items[0].Title != "Test Movie" {
		t.Errorf("title = %q, want %q", resp.Items[0].Title, "Test Movie")
	}
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
	if err := srv.library.AddContent(movie); err != nil {
		t.Fatalf("add movie: %v", err)
	}

	// Add series
	series := &library.Content{
		Type:           library.ContentTypeSeries,
		Title:          "Series",
		Year:           2024,
		Status:         library.StatusAvailable,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	if err := srv.library.AddContent(series); err != nil {
		t.Fatalf("add series: %v", err)
	}

	// Filter by type
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content?type=movie", nil)
	w := httptest.NewRecorder()
	srv.listContent(w, req)

	var resp listContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("filter by type: items = %d, want 1", len(resp.Items))
	}

	// Filter by status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/content?status=available", nil)
	w = httptest.NewRecorder()
	srv.listContent(w, req)

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("filter by status: items = %d, want 1", len(resp.Items))
	}
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
	if err := srv.library.AddContent(c); err != nil {
		t.Fatalf("add content: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.getContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp contentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Title != "Test Movie" {
		t.Errorf("title = %q, want %q", resp.Title, "Test Movie")
	}
}

func TestGetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.getContent(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAddContent(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{MovieRoot: "/movies", SeriesRoot: "/tv"})

	body := `{"type":"movie","title":"New Movie","year":2024,"quality_profile":"hd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.addContent(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp contentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID == 0 {
		t.Error("ID should be set")
	}
	if resp.Title != "New Movie" {
		t.Errorf("title = %q, want %q", resp.Title, "New Movie")
	}
	if resp.Status != "wanted" {
		t.Errorf("status = %q, want wanted", resp.Status)
	}
	if resp.RootPath != "/movies" {
		t.Errorf("root_path = %q, want /movies", resp.RootPath)
	}
}

func TestAddContent_InvalidType(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"type":"invalid","title":"Test","year":2024,"quality_profile":"hd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.addContent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
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
	if err := srv.library.AddContent(c); err != nil {
		t.Fatalf("add content: %v", err)
	}

	body := `{"status":"available"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/content/1", strings.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.updateContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update
	updated, err := srv.library.GetContent(1)
	if err != nil {
		t.Fatalf("get content: %v", err)
	}
	if updated.Status != library.StatusAvailable {
		t.Errorf("status = %q, want available", updated.Status)
	}
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
	if err := srv.library.AddContent(c); err != nil {
		t.Fatalf("add content: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/content/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteContent(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deleted
	_, err := srv.library.GetContent(1)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
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
	if err := srv.library.AddContent(series); err != nil {
		t.Fatalf("add series: %v", err)
	}

	// Add episodes
	for i := 1; i <= 3; i++ {
		ep := &library.Episode{
			ContentID: series.ID,
			Season:    1,
			Episode:   i,
			Title:     fmt.Sprintf("Episode %d", i),
			Status:    library.StatusWanted,
		}
		if err := srv.library.AddEpisode(ep); err != nil {
			t.Fatalf("add episode: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/1/episodes", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.listEpisodes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listEpisodesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 3 {
		t.Errorf("items = %d, want 3", len(resp.Items))
	}
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
	if err := srv.library.AddContent(series); err != nil {
		t.Fatalf("add series: %v", err)
	}

	ep := &library.Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    library.StatusWanted,
	}
	if err := srv.library.AddEpisode(ep); err != nil {
		t.Fatalf("add episode: %v", err)
	}

	body := `{"status":"available"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/episodes/1", strings.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.updateEpisode(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update
	updated, err := srv.library.GetEpisode(1)
	if err != nil {
		t.Fatalf("get episode: %v", err)
	}
	if updated.Status != library.StatusAvailable {
		t.Errorf("status = %q, want available", updated.Status)
	}
}

func TestSearch_NoSearcher(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"query":"test movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.search(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestGrab_NoManager(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	body := `{"content_id":1,"download_url":"http://example.com/nzb","title":"Test","indexer":"TestIndexer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/grab", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.grab(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestListDownloads_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads", nil)
	w := httptest.NewRecorder()

	srv.listDownloads(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listDownloadsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
}

func TestGetDownload_NotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	srv.getDownload(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteDownload_NoManager(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/downloads/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteDownload(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestListHistory_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history", nil)
	w := httptest.NewRecorder()

	srv.listHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listHistoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
}

func TestListFiles_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	w := httptest.NewRecorder()

	srv.listFiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listFilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
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
	if err := srv.library.AddContent(c); err != nil {
		t.Fatalf("add content: %v", err)
	}

	f := &library.File{
		ContentID: c.ID,
		Path:      "/movies/test.mkv",
		SizeBytes: 1000,
		Quality:   "1080p",
		Source:    "test",
	}
	if err := srv.library.AddFile(f); err != nil {
		t.Fatalf("add file: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	srv.deleteFile(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestGetStatus(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()

	srv.getStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
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

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listProfilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Profiles) != 2 {
		t.Errorf("profiles = %d, want 2", len(resp.Profiles))
	}
}

func TestTriggerScan_NoPlex(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scan", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	srv.triggerScan(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
