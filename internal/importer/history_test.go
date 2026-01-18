// internal/importer/history_test.go
package importer

import (
	"testing"
	"time"
)

func TestHistoryStore_Add(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	h := &HistoryEntry{
		ContentID: contentID,
		Event:     EventImported,
		Data:      `{"source_path": "/downloads/movie.mkv"}`,
	}

	if err := store.Add(h); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if h.ID == 0 {
		t.Error("ID should be set after Add")
	}
	if h.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestHistoryStore_List(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	// Add multiple entries
	events := []string{EventGrabbed, EventImported, EventGrabbed}
	for _, event := range events {
		h := &HistoryEntry{ContentID: contentID, Event: event, Data: "{}"}
		if err := store.Add(h); err != nil {
			t.Fatalf("Add: %v", err)
		}
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	// List all
	entries, err := store.List(HistoryFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// List by content
	entries, err = store.List(HistoryFilter{ContentID: &contentID})
	if err != nil {
		t.Fatalf("List by content: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// List by event
	event := EventGrabbed
	entries, err = store.List(HistoryFilter{Event: &event})
	if err != nil {
		t.Fatalf("List by event: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 grabbed entries, got %d", len(entries))
	}

	// List with limit
	entries, err = store.List(HistoryFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List with limit: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(entries))
	}
}

func TestHistoryStore_List_OrderByRecent(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	// Add entries
	for i := 0; i < 3; i++ {
		h := &HistoryEntry{ContentID: contentID, Event: EventImported, Data: "{}"}
		_ = store.Add(h)
		time.Sleep(time.Millisecond)
	}

	entries, _ := store.List(HistoryFilter{})

	// Should be ordered by most recent first
	for i := 1; i < len(entries); i++ {
		if entries[i].CreatedAt.After(entries[i-1].CreatedAt) {
			t.Error("entries should be ordered by most recent first")
		}
	}
}
