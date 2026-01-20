// internal/importer/history_test.go
package importer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryStore_Add(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db)

	h := &HistoryEntry{
		ContentID: contentID,
		Event:     EventImported,
		Data:      `{"source_path": "/downloads/movie.mkv"}`,
	}

	require.NoError(t, store.Add(h))

	assert.NotZero(t, h.ID, "ID should be set after Add")
	assert.False(t, h.CreatedAt.IsZero(), "CreatedAt should be set")
}

func TestHistoryStore_List(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db)

	// Add multiple entries
	events := []string{EventGrabbed, EventImported, EventGrabbed}
	for _, event := range events {
		h := &HistoryEntry{ContentID: contentID, Event: event, Data: "{}"}
		require.NoError(t, store.Add(h))
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	// List all
	entries, err := store.List(HistoryFilter{})
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// List by content
	entries, err = store.List(HistoryFilter{ContentID: &contentID})
	require.NoError(t, err, "List by content")
	assert.Len(t, entries, 3)

	// List by event
	event := EventGrabbed
	entries, err = store.List(HistoryFilter{Event: &event})
	require.NoError(t, err, "List by event")
	assert.Len(t, entries, 2, "expected 2 grabbed entries")

	// List with limit
	entries, err = store.List(HistoryFilter{Limit: 2})
	require.NoError(t, err, "List with limit")
	assert.Len(t, entries, 2, "expected 2 entries with limit")
}

func TestHistoryStore_List_OrderByRecent(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db)

	// Add entries
	for i := 0; i < 3; i++ {
		h := &HistoryEntry{ContentID: contentID, Event: EventImported, Data: "{}"}
		_ = store.Add(h)
		time.Sleep(time.Millisecond)
	}

	entries, _ := store.List(HistoryFilter{})

	// Should be ordered by most recent first
	for i := 1; i < len(entries); i++ {
		assert.False(t, entries[i].CreatedAt.After(entries[i-1].CreatedAt),
			"entries should be ordered by most recent first")
	}
}
