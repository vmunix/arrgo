package library

import (
	"errors"
	"testing"
	"time"
)

func TestStore_AddContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	before := time.Now()
	if err := store.AddContent(c); err != nil {
		t.Fatalf("AddContent: %v", err)
	}
	after := time.Now()

	// ID should be set
	if c.ID == 0 {
		t.Error("ID should be set after AddContent")
	}

	// AddedAt and UpdatedAt should be set
	if c.AddedAt.Before(before) || c.AddedAt.After(after) {
		t.Errorf("AddedAt %v not in expected range [%v, %v]", c.AddedAt, before, after)
	}
	if c.UpdatedAt.Before(before) || c.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not in expected range [%v, %v]", c.UpdatedAt, before, after)
	}
}

func TestStore_AddContent_Series(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeSeries,
		TVDBID:         ptr(int64(81189)),
		Title:          "Breaking Bad",
		Year:           2008,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}

	if err := store.AddContent(c); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	if c.ID == 0 {
		t.Error("ID should be set after AddContent")
	}
}

func TestStore_GetContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	original := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(original); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	retrieved, err := store.GetContent(original.ID)
	if err != nil {
		t.Fatalf("GetContent: %v", err)
	}

	// Verify all fields
	if retrieved.ID != original.ID {
		t.Errorf("ID = %d, want %d", retrieved.ID, original.ID)
	}
	if retrieved.Type != original.Type {
		t.Errorf("Type = %q, want %q", retrieved.Type, original.Type)
	}
	if retrieved.TMDBID == nil || *retrieved.TMDBID != *original.TMDBID {
		t.Errorf("TMDBID = %v, want %v", retrieved.TMDBID, original.TMDBID)
	}
	if retrieved.Title != original.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, original.Title)
	}
	if retrieved.Year != original.Year {
		t.Errorf("Year = %d, want %d", retrieved.Year, original.Year)
	}
	if retrieved.Status != original.Status {
		t.Errorf("Status = %q, want %q", retrieved.Status, original.Status)
	}
	if retrieved.QualityProfile != original.QualityProfile {
		t.Errorf("QualityProfile = %q, want %q", retrieved.QualityProfile, original.QualityProfile)
	}
	if retrieved.RootPath != original.RootPath {
		t.Errorf("RootPath = %q, want %q", retrieved.RootPath, original.RootPath)
	}
}

func TestStore_GetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetContent(9999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetContent(9999) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListContent_All(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add multiple content items
	movies := []*Content{
		{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"},
		{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusAvailable, QualityProfile: "hd", RootPath: "/movies"},
	}
	series := &Content{Type: ContentTypeSeries, TVDBID: ptr(int64(81189)), Title: "Breaking Bad", Year: 2008, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}

	for _, c := range movies {
		if err := store.AddContent(c); err != nil {
			t.Fatalf("AddContent: %v", err)
		}
	}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	// List all
	results, total, err := store.ListContent(ContentFilter{})
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
}

func TestStore_ListContent_FilterByType(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add movies and series
	movie := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	series := &Content{Type: ContentTypeSeries, TVDBID: ptr(int64(81189)), Title: "Breaking Bad", Year: 2008, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}

	if err := store.AddContent(movie); err != nil {
		t.Fatalf("AddContent: %v", err)
	}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	// Filter by type
	movieType := ContentTypeMovie
	results, total, err := store.ListContent(ContentFilter{Type: &movieType})
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}

	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].Title != "Fight Club" {
		t.Errorf("Title = %q, want Fight Club", results[0].Title)
	}
}

func TestStore_ListContent_FilterByStatus(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	wanted := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	available := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusAvailable, QualityProfile: "hd", RootPath: "/movies"}

	if err := store.AddContent(wanted); err != nil {
		t.Fatalf("AddContent: %v", err)
	}
	if err := store.AddContent(available); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	status := StatusAvailable
	results, total, err := store.ListContent(ContentFilter{Status: &status})
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}

	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].Title != "Pulp Fiction" {
		t.Errorf("Title = %q, want Pulp Fiction", results[0].Title)
	}
}

func TestStore_ListContent_FilterByTMDBID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c1 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	c2 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}

	if err := store.AddContent(c1); err != nil {
		t.Fatalf("AddContent: %v", err)
	}
	if err := store.AddContent(c2); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	results, total, err := store.ListContent(ContentFilter{TMDBID: ptr(int64(550))})
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}

	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].Title != "Fight Club" {
		t.Errorf("results = %v, want [Fight Club]", results)
	}
}

func TestStore_ListContent_Pagination(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add 5 items
	for i := 1; i <= 5; i++ {
		c := &Content{
			Type:           ContentTypeMovie,
			TMDBID:         ptr(int64(i)),
			Title:          "Movie",
			Year:           2000 + i,
			Status:         StatusWanted,
			QualityProfile: "hd",
			RootPath:       "/movies",
		}
		if err := store.AddContent(c); err != nil {
			t.Fatalf("AddContent: %v", err)
		}
	}

	// Get page 1 (first 2)
	results, total, err := store.ListContent(ContentFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}

	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Get page 2 (next 2)
	results2, total2, err := store.ListContent(ContentFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListContent: %v", err)
	}

	if total2 != 5 {
		t.Errorf("total = %d, want 5", total2)
	}
	if len(results2) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results2))
	}

	// Results should be different
	if results[0].ID == results2[0].ID {
		t.Error("pagination should return different items")
	}
}

func TestStore_UpdateContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	originalUpdatedAt := c.UpdatedAt

	// Give a small delay to ensure UpdatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Update the content
	c.Status = StatusAvailable
	c.QualityProfile = "4k"

	if err := store.UpdateContent(c); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	// UpdatedAt should be changed
	if !c.UpdatedAt.After(originalUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}

	// Verify in database
	retrieved, err := store.GetContent(c.ID)
	if err != nil {
		t.Fatalf("GetContent: %v", err)
	}

	if retrieved.Status != StatusAvailable {
		t.Errorf("Status = %q, want available", retrieved.Status)
	}
	if retrieved.QualityProfile != "4k" {
		t.Errorf("QualityProfile = %q, want 4k", retrieved.QualityProfile)
	}
}

func TestStore_UpdateContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		ID:             9999,
		Type:           ContentTypeMovie,
		Title:          "Nonexistent",
		Year:           2000,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	err := store.UpdateContent(c)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateContent error = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	// Delete
	if err := store.DeleteContent(c.ID); err != nil {
		t.Fatalf("DeleteContent: %v", err)
	}

	// Verify deleted
	_, err := store.GetContent(c.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetContent after delete: error = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteContent_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	if err := store.DeleteContent(9999); err != nil {
		t.Errorf("DeleteContent(9999) = %v, want nil (idempotent)", err)
	}
}

func TestTx_AddContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	if err := tx.AddContent(c); err != nil {
		t.Fatalf("tx.AddContent: %v", err)
	}

	if c.ID == 0 {
		t.Error("ID should be set")
	}

	// Should be visible within transaction
	retrieved, err := tx.GetContent(c.ID)
	if err != nil {
		t.Fatalf("tx.GetContent: %v", err)
	}
	if retrieved.Title != c.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, c.Title)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Should be visible after commit
	retrieved, err = store.GetContent(c.ID)
	if err != nil {
		t.Fatalf("store.GetContent after commit: %v", err)
	}
	if retrieved.Title != c.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, c.Title)
	}
}

func TestTx_Rollback(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	if err := tx.AddContent(c); err != nil {
		t.Fatalf("tx.AddContent: %v", err)
	}

	id := c.ID

	// Rollback
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Should NOT be visible after rollback
	_, err = store.GetContent(id)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetContent after rollback: error = %v, want ErrNotFound", err)
	}
}

func TestTx_ListContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add initial content
	c1 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(c1); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Add more in transaction
	c2 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := tx.AddContent(c2); err != nil {
		t.Fatalf("tx.AddContent: %v", err)
	}

	// List within transaction should see both
	results, total, err := tx.ListContent(ContentFilter{})
	if err != nil {
		t.Fatalf("tx.ListContent: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

func TestTx_UpdateContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	c.Status = StatusAvailable
	if err := tx.UpdateContent(c); err != nil {
		t.Fatalf("tx.UpdateContent: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	retrieved, err := store.GetContent(c.ID)
	if err != nil {
		t.Fatalf("GetContent: %v", err)
	}

	if retrieved.Status != StatusAvailable {
		t.Errorf("Status = %q, want available", retrieved.Status)
	}
}

func TestTx_DeleteContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	if err := store.AddContent(c); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.DeleteContent(c.ID); err != nil {
		t.Fatalf("tx.DeleteContent: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	_, err = store.GetContent(c.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetContent after delete: error = %v, want ErrNotFound", err)
	}
}
