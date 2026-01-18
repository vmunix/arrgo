// internal/api/v1/api_test.go
package v1

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arrgo/arrgo/internal/library"
	_ "github.com/mattn/go-sqlite3"
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
