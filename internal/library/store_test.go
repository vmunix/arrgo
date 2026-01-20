package library

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, store.AddContent(c), "AddContent should succeed")
	after := time.Now()

	// ID should be set
	assert.NotZero(t, c.ID, "ID should be set after AddContent")

	// AddedAt and UpdatedAt should be set
	assert.False(t, c.AddedAt.Before(before) || c.AddedAt.After(after),
		"AddedAt %v not in expected range [%v, %v]", c.AddedAt, before, after)
	assert.False(t, c.UpdatedAt.Before(before) || c.UpdatedAt.After(after),
		"UpdatedAt %v not in expected range [%v, %v]", c.UpdatedAt, before, after)
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

	require.NoError(t, store.AddContent(c), "AddContent should succeed")
	assert.NotZero(t, c.ID, "ID should be set after AddContent")
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
	require.NoError(t, store.AddContent(original), "AddContent should succeed")

	retrieved, err := store.GetContent(original.ID)
	require.NoError(t, err, "GetContent should succeed")

	// Verify all fields
	assert.Equal(t, original.ID, retrieved.ID)
	assert.Equal(t, original.Type, retrieved.Type)
	require.NotNil(t, retrieved.TMDBID)
	assert.Equal(t, *original.TMDBID, *retrieved.TMDBID)
	assert.Equal(t, original.Title, retrieved.Title)
	assert.Equal(t, original.Year, retrieved.Year)
	assert.Equal(t, original.Status, retrieved.Status)
	assert.Equal(t, original.QualityProfile, retrieved.QualityProfile)
	assert.Equal(t, original.RootPath, retrieved.RootPath)
}

func TestStore_GetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.GetContent(9999)
	assert.ErrorIs(t, err, ErrNotFound, "GetContent(9999) should return ErrNotFound")
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
		require.NoError(t, store.AddContent(c), "AddContent should succeed")
	}
	require.NoError(t, store.AddContent(series), "AddContent should succeed")

	// List all
	results, total, err := store.ListContent(ContentFilter{})
	require.NoError(t, err, "ListContent should succeed")

	assert.Equal(t, 3, total)
	assert.Len(t, results, 3)
}

func TestStore_ListContent_FilterByType(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add movies and series
	movie := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	series := &Content{Type: ContentTypeSeries, TVDBID: ptr(int64(81189)), Title: "Breaking Bad", Year: 2008, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}

	require.NoError(t, store.AddContent(movie), "AddContent should succeed")
	require.NoError(t, store.AddContent(series), "AddContent should succeed")

	// Filter by type
	movieType := ContentTypeMovie
	results, total, err := store.ListContent(ContentFilter{Type: &movieType})
	require.NoError(t, err, "ListContent should succeed")

	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)
	assert.Equal(t, "Fight Club", results[0].Title)
}

func TestStore_ListContent_FilterByStatus(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	wanted := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	available := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusAvailable, QualityProfile: "hd", RootPath: "/movies"}

	require.NoError(t, store.AddContent(wanted), "AddContent should succeed")
	require.NoError(t, store.AddContent(available), "AddContent should succeed")

	status := StatusAvailable
	results, total, err := store.ListContent(ContentFilter{Status: &status})
	require.NoError(t, err, "ListContent should succeed")

	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)
	assert.Equal(t, "Pulp Fiction", results[0].Title)
}

func TestStore_ListContent_FilterByTMDBID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	c1 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	c2 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}

	require.NoError(t, store.AddContent(c1), "AddContent should succeed")
	require.NoError(t, store.AddContent(c2), "AddContent should succeed")

	results, total, err := store.ListContent(ContentFilter{TMDBID: ptr(int64(550))})
	require.NoError(t, err, "ListContent should succeed")

	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "Fight Club", results[0].Title)
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
		require.NoError(t, store.AddContent(c), "AddContent should succeed")
	}

	// Get page 1 (first 2)
	results, total, err := store.ListContent(ContentFilter{Limit: 2, Offset: 0})
	require.NoError(t, err, "ListContent should succeed")

	assert.Equal(t, 5, total)
	assert.Len(t, results, 2)

	// Get page 2 (next 2)
	results2, total2, err := store.ListContent(ContentFilter{Limit: 2, Offset: 2})
	require.NoError(t, err, "ListContent should succeed")

	assert.Equal(t, 5, total2)
	assert.Len(t, results2, 2)

	// Results should be different
	assert.NotEqual(t, results[0].ID, results2[0].ID, "pagination should return different items")
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
	require.NoError(t, store.AddContent(c), "AddContent should succeed")

	originalUpdatedAt := c.UpdatedAt

	// Give a small delay to ensure UpdatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Update the content
	c.Status = StatusAvailable
	c.QualityProfile = "4k"

	require.NoError(t, store.UpdateContent(c), "UpdateContent should succeed")

	// UpdatedAt should be changed
	assert.True(t, c.UpdatedAt.After(originalUpdatedAt), "UpdatedAt should be updated")

	// Verify in database
	retrieved, err := store.GetContent(c.ID)
	require.NoError(t, err, "GetContent should succeed")

	assert.Equal(t, StatusAvailable, retrieved.Status)
	assert.Equal(t, "4k", retrieved.QualityProfile)
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
	assert.ErrorIs(t, err, ErrNotFound, "UpdateContent should return ErrNotFound")
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
	require.NoError(t, store.AddContent(c), "AddContent should succeed")

	// Delete
	require.NoError(t, store.DeleteContent(c.ID), "DeleteContent should succeed")

	// Verify deleted
	_, err := store.GetContent(c.ID)
	assert.ErrorIs(t, err, ErrNotFound, "GetContent after delete should return ErrNotFound")
}

func TestStore_DeleteContent_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Delete non-existent should not error
	assert.NoError(t, store.DeleteContent(9999), "DeleteContent(9999) should be idempotent")
}

func TestTx_AddContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")
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

	require.NoError(t, tx.AddContent(c), "tx.AddContent should succeed")
	assert.NotZero(t, c.ID, "ID should be set")

	// Should be visible within transaction
	retrieved, err := tx.GetContent(c.ID)
	require.NoError(t, err, "tx.GetContent should succeed")
	assert.Equal(t, c.Title, retrieved.Title)

	// Commit
	require.NoError(t, tx.Commit(), "Commit should succeed")

	// Should be visible after commit
	retrieved, err = store.GetContent(c.ID)
	require.NoError(t, err, "store.GetContent after commit should succeed")
	assert.Equal(t, c.Title, retrieved.Title)
}

func TestTx_Rollback(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")

	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(550)),
		Title:          "Fight Club",
		Year:           1999,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	require.NoError(t, tx.AddContent(c), "tx.AddContent should succeed")

	id := c.ID

	// Rollback
	require.NoError(t, tx.Rollback(), "Rollback should succeed")

	// Should NOT be visible after rollback
	_, err = store.GetContent(id)
	assert.ErrorIs(t, err, ErrNotFound, "GetContent after rollback should return ErrNotFound")
}

func TestTx_ListContent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add initial content
	c1 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(550)), Title: "Fight Club", Year: 1999, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	require.NoError(t, store.AddContent(c1), "AddContent should succeed")

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")
	defer func() { _ = tx.Rollback() }()

	// Add more in transaction
	c2 := &Content{Type: ContentTypeMovie, TMDBID: ptr(int64(680)), Title: "Pulp Fiction", Year: 1994, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	require.NoError(t, tx.AddContent(c2), "tx.AddContent should succeed")

	// List within transaction should see both
	results, total, err := tx.ListContent(ContentFilter{})
	require.NoError(t, err, "tx.ListContent should succeed")

	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
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
	require.NoError(t, store.AddContent(c), "AddContent should succeed")

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")
	defer func() { _ = tx.Rollback() }()

	c.Status = StatusAvailable
	require.NoError(t, tx.UpdateContent(c), "tx.UpdateContent should succeed")

	require.NoError(t, tx.Commit(), "Commit should succeed")

	retrieved, err := store.GetContent(c.ID)
	require.NoError(t, err, "GetContent should succeed")

	assert.Equal(t, StatusAvailable, retrieved.Status)
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
	require.NoError(t, store.AddContent(c), "AddContent should succeed")

	tx, err := store.Begin()
	require.NoError(t, err, "Begin should succeed")
	defer func() { _ = tx.Rollback() }()

	require.NoError(t, tx.DeleteContent(c.ID), "tx.DeleteContent should succeed")

	require.NoError(t, tx.Commit(), "Commit should succeed")

	_, err = store.GetContent(c.ID)
	assert.ErrorIs(t, err, ErrNotFound, "GetContent after delete should return ErrNotFound")
}

func TestStore_GetByTitleYear(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add a movie
	movie := &Content{Type: ContentTypeMovie, Title: "Back to the Future", Year: 1985, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	require.NoError(t, store.AddContent(movie), "AddContent should succeed")

	// Find it
	found, err := store.GetByTitleYear("Back to the Future", 1985)
	require.NoError(t, err, "GetByTitleYear should succeed")
	require.NotNil(t, found, "expected to find content")
	assert.Equal(t, movie.ID, found.ID)

	// Not found
	notFound, err := store.GetByTitleYear("Nonexistent", 2000)
	require.NoError(t, err, "GetByTitleYear should succeed")
	assert.Nil(t, notFound, "expected nil for nonexistent content")
}
