// internal/library/sqlite_compat_test.go
package library

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteCompat_NullHandling verifies NULL handling works correctly.
func TestSQLiteCompat_NullHandling(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create content with nil optional fields
	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         nil, // NULL
		Title:          "Test Movie",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	err := store.AddContent(c)
	require.NoError(t, err)

	// Retrieve and verify NULL preserved
	retrieved, err := store.GetContent(c.ID)
	require.NoError(t, err)
	assert.Nil(t, retrieved.TMDBID)
}

// TestSQLiteCompat_TypeAffinity verifies type coercion works.
func TestSQLiteCompat_TypeAffinity(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Store with int64 ID
	c := &Content{
		Type:           ContentTypeMovie,
		TMDBID:         ptr(int64(12345)),
		Title:          "Type Test",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}

	err := store.AddContent(c)
	require.NoError(t, err)

	retrieved, err := store.GetContent(c.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), *retrieved.TMDBID)
}

// TestSQLiteCompat_ConcurrentWrites verifies concurrent operations work
// with SQLite's serialized access model. For in-memory databases,
// we limit to a single connection to ensure all goroutines share
// the same database state.
func TestSQLiteCompat_ConcurrentWrites(t *testing.T) {
	db := setupTestDB(t)
	// With in-memory SQLite, multiple connections create separate databases.
	// Limit to 1 connection so all goroutines share the same database.
	db.SetMaxOpenConns(1)
	store := NewStore(db)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	// Run writes from concurrent goroutines
	// SQLite handles serialization internally with SQLITE_BUSY retries
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := &Content{
				Type:           ContentTypeMovie,
				Title:          "Concurrent Test",
				Year:           2024 + idx,
				Status:         StatusWanted,
				QualityProfile: "hd",
				RootPath:       "/movies",
			}
			if err := store.AddContent(c); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	for _, err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}

	// Verify all 10 were inserted
	contents, total, err := store.ListContent(ContentFilter{Limit: 20})
	require.NoError(t, err)
	assert.Len(t, contents, 10)
	assert.Equal(t, 10, total)
}

// TestSQLiteCompat_ConstraintErrors verifies error mapping works.
func TestSQLiteCompat_ConstraintErrors(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add a series with an episode
	series := &Content{
		Type:           ContentTypeSeries,
		TVDBID:         ptr(int64(12345)),
		Title:          "Constraint Test Series",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/tv",
	}
	require.NoError(t, store.AddContent(series))

	// Add first episode
	ep1 := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1,
		Title:     "Pilot",
		Status:    StatusWanted,
	}
	require.NoError(t, store.AddEpisode(ep1))

	// Try to add duplicate episode (same content_id, season, episode) - should get ErrDuplicate
	// The schema has UNIQUE(content_id, season, episode) constraint on episodes table
	ep2 := &Episode{
		ContentID: series.ID,
		Season:    1,
		Episode:   1, // Same season/episode
		Title:     "Duplicate Pilot",
		Status:    StatusWanted,
	}
	err := store.AddEpisode(ep2)
	assert.ErrorIs(t, err, ErrDuplicate)
}

// TestSQLiteCompat_TransactionIsolation verifies transactions work correctly.
func TestSQLiteCompat_TransactionIsolation(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Start transaction
	tx, err := store.Begin()
	require.NoError(t, err)

	// Add content in transaction
	c := &Content{
		Type:           ContentTypeMovie,
		Title:          "Transaction Test",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	err = tx.AddContent(c)
	require.NoError(t, err)

	// Content should be visible within the transaction
	retrieved, err := tx.GetContent(c.ID)
	require.NoError(t, err)
	assert.Equal(t, "Transaction Test", retrieved.Title)

	// Commit
	require.NoError(t, tx.Commit())

	// After commit, content should be visible through store
	contents, total, err := store.ListContent(ContentFilter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, contents, 1)
	assert.Equal(t, 1, total)
}

// TestSQLiteCompat_TransactionRollback verifies rollback works correctly.
func TestSQLiteCompat_TransactionRollback(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Start transaction
	tx, err := store.Begin()
	require.NoError(t, err)

	// Add content in transaction
	c := &Content{
		Type:           ContentTypeMovie,
		Title:          "Rollback Test",
		Year:           2024,
		Status:         StatusWanted,
		QualityProfile: "hd",
		RootPath:       "/movies",
	}
	err = tx.AddContent(c)
	require.NoError(t, err)
	id := c.ID

	// Rollback
	require.NoError(t, tx.Rollback())

	// After rollback, content should NOT be visible
	_, err = store.GetContent(id)
	assert.ErrorIs(t, err, ErrNotFound)
}
