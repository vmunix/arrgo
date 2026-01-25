package events

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			payload TEXT NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_events_type ON events(event_type);
		CREATE INDEX idx_events_entity ON events(entity_type, entity_id);
		CREATE INDEX idx_events_occurred ON events(occurred_at);
	`)
	require.NoError(t, err)
	return db
}

func TestEventLog_Append(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	e := &testEvent{
		BaseEvent: NewBaseEvent("test.created", "test", 1),
		Message:   "hello",
	}

	id, err := log.Append(e)
	require.NoError(t, err)
	assert.Positive(t, id)

	// Verify payload is stored correctly
	events, err := log.ForEntity("test", 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Payload, `"message":"hello"`)
	assert.Equal(t, "test.created", events[0].EventType)
	assert.Equal(t, "test", events[0].EntityType)
	assert.Equal(t, int64(1), events[0].EntityID)
}

func TestEventLog_Since(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	start := time.Now().Add(-time.Hour)

	// Add events
	e1 := &testEvent{BaseEvent: NewBaseEvent("test.first", "test", 1), Message: "first"}
	e2 := &testEvent{BaseEvent: NewBaseEvent("test.second", "test", 2), Message: "second"}

	_, err := log.Append(e1)
	require.NoError(t, err)
	_, err = log.Append(e2)
	require.NoError(t, err)

	// Query
	events, err := log.Since(start)
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// Verify order (by id ascending)
	assert.Equal(t, "test.first", events[0].EventType)
	assert.Equal(t, "test.second", events[1].EventType)
}

func TestEventLog_ForEntity(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	// Add events for different entities
	e1 := &testEvent{BaseEvent: NewBaseEvent("test.one", "download", 1), Message: "one"}
	e2 := &testEvent{BaseEvent: NewBaseEvent("test.two", "download", 2), Message: "two"}
	e3 := &testEvent{BaseEvent: NewBaseEvent("test.three", "download", 1), Message: "three"}

	_, err := log.Append(e1)
	require.NoError(t, err)
	_, err = log.Append(e2)
	require.NoError(t, err)
	_, err = log.Append(e3)
	require.NoError(t, err)

	// Query for entity 1
	events, err := log.ForEntity("download", 1)
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// Verify correct events returned (order by id)
	assert.Equal(t, "test.one", events[0].EventType)
	assert.Equal(t, "test.three", events[1].EventType)

	// Verify entity 2 only has one event
	events2, err := log.ForEntity("download", 2)
	require.NoError(t, err)
	assert.Len(t, events2, 1)
	assert.Equal(t, "test.two", events2[0].EventType)
}

func TestEventLog_Prune(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	// Insert an event with a manually backdated occurred_at
	_, err := db.Exec(`
		INSERT INTO events (event_type, entity_type, entity_id, payload, occurred_at)
		VALUES (?, ?, ?, ?, ?)`,
		"test.old", "test", 1, `{"message":"old"}`, time.Now().Add(-100*24*time.Hour),
	)
	require.NoError(t, err)

	// Insert a recent event
	e := &testEvent{BaseEvent: NewBaseEvent("test.new", "test", 2), Message: "new"}
	_, err = log.Append(e)
	require.NoError(t, err)

	// Prune events older than 90 days
	count, err := log.Prune(90 * 24 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify only the new event remains
	events, err := log.Since(time.Time{})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "test.new", events[0].EventType)
}

func TestEventLog_Recent(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	// Insert 5 events
	for i := 0; i < 5; i++ {
		evt := &ContentAdded{
			BaseEvent: NewBaseEvent(EventContentAdded, EntityContent, int64(i+1)),
			ContentID: int64(i + 1),
			Title:     fmt.Sprintf("Movie %d", i+1),
		}
		_, err := log.Append(evt)
		require.NoError(t, err)
	}

	// Get last 3
	events, err := log.Recent(3)
	require.NoError(t, err)
	assert.Len(t, events, 3)
	// Should be in reverse chronological order (newest first)
	assert.Equal(t, int64(5), events[0].EntityID)
	assert.Equal(t, int64(4), events[1].EntityID)
	assert.Equal(t, int64(3), events[2].EntityID)
}

// testEvent is a concrete event type for testing
type testEvent struct {
	BaseEvent
	Message string `json:"message"`
}
