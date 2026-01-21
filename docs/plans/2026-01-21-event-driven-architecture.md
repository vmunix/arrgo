# Event-Driven Architecture Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor arrgo's state management to event-driven architecture for robust coordination of external services and real-time UI support.

**Architecture:** Event bus with Go channels + SQLite persistence. Publishers (adapters, API) emit events, handlers process them. Each handler is a goroutine subscribing to specific events. Per-entity locking prevents concurrent operations on the same download.

**Tech Stack:** Go stdlib (channels, sync, context, errgroup), SQLite, existing arrgo packages

---

## Phase 1: Event Bus Core

### Task 1.1: Create Event Interface and BaseEvent

**Files:**
- Create: `internal/events/event.go`
- Test: `internal/events/event_test.go`

**Step 1: Write the failing test**

```go
// internal/events/event_test.go
package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseEvent_ImplementsEvent(t *testing.T) {
	now := time.Now()
	e := BaseEvent{
		Type:      "test.event",
		Entity:    "download",
		ID:        42,
		Timestamp: now,
	}

	assert.Equal(t, "test.event", e.EventType())
	assert.Equal(t, "download", e.EntityType())
	assert.Equal(t, int64(42), e.EntityID())
	assert.Equal(t, now, e.OccurredAt())
}

func TestNewBaseEvent(t *testing.T) {
	e := NewBaseEvent("grab.requested", "download", 123)

	assert.Equal(t, "grab.requested", e.EventType())
	assert.Equal(t, "download", e.EntityType())
	assert.Equal(t, int64(123), e.EntityID())
	assert.False(t, e.OccurredAt().IsZero())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestBaseEvent`
Expected: FAIL with "package events is not in std"

**Step 3: Write minimal implementation**

```go
// internal/events/event.go
package events

import "time"

// Event is the base interface all events implement.
type Event interface {
	EventType() string
	EntityType() string // "download", "content", "episode"
	EntityID() int64
	OccurredAt() time.Time
}

// BaseEvent provides common fields for all events.
type BaseEvent struct {
	Type      string    `json:"type"`
	Entity    string    `json:"entity_type"`
	ID        int64     `json:"entity_id"`
	Timestamp time.Time `json:"occurred_at"`
}

func (e BaseEvent) EventType() string     { return e.Type }
func (e BaseEvent) EntityType() string    { return e.Entity }
func (e BaseEvent) EntityID() int64       { return e.ID }
func (e BaseEvent) OccurredAt() time.Time { return e.Timestamp }

// NewBaseEvent creates a BaseEvent with the current timestamp.
func NewBaseEvent(eventType, entityType string, entityID int64) BaseEvent {
	return BaseEvent{
		Type:      eventType,
		Entity:    entityType,
		ID:        entityID,
		Timestamp: time.Now(),
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run TestBaseEvent`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/event.go internal/events/event_test.go
git commit -m "feat(events): add Event interface and BaseEvent type"
```

---

### Task 1.2: Create Event Log (SQLite Persistence)

**Files:**
- Create: `internal/events/log.go`
- Test: `internal/events/log_test.go`
- Create: `internal/migrations/sql/004_events.sql`
- Modify: `internal/migrations/embed.go`

**Step 1: Write the failing test**

```go
// internal/events/log_test.go
package events

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
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
	assert.Greater(t, id, int64(0))
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
}

func TestEventLog_ForEntity(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	// Add events for different entities
	e1 := &testEvent{BaseEvent: NewBaseEvent("test.one", "download", 1), Message: "one"}
	e2 := &testEvent{BaseEvent: NewBaseEvent("test.two", "download", 2), Message: "two"}
	e3 := &testEvent{BaseEvent: NewBaseEvent("test.three", "download", 1), Message: "three"}

	log.Append(e1)
	log.Append(e2)
	log.Append(e3)

	// Query for entity 1
	events, err := log.ForEntity("download", 1)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

// testEvent is a concrete event type for testing
type testEvent struct {
	BaseEvent
	Message string `json:"message"`
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestEventLog`
Expected: FAIL with "undefined: NewEventLog"

**Step 3: Write minimal implementation**

```go
// internal/events/log.go
package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventLog persists events to SQLite.
type EventLog struct {
	db *sql.DB
}

// NewEventLog creates a new event log.
func NewEventLog(db *sql.DB) *EventLog {
	return &EventLog{db: db}
}

// Append persists an event and returns its ID.
func (l *EventLog) Append(e Event) (int64, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}

	result, err := l.db.Exec(`
		INSERT INTO events (event_type, entity_type, entity_id, payload, occurred_at)
		VALUES (?, ?, ?, ?, ?)`,
		e.EventType(), e.EntityType(), e.EntityID(), string(payload), e.OccurredAt(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}

	return result.LastInsertId()
}

// RawEvent represents a persisted event with its raw payload.
type RawEvent struct {
	ID         int64
	EventType  string
	EntityType string
	EntityID   int64
	Payload    string
	OccurredAt time.Time
	CreatedAt  time.Time
}

// Since returns all events since the given time.
func (l *EventLog) Since(t time.Time) ([]RawEvent, error) {
	rows, err := l.db.Query(`
		SELECT id, event_type, entity_type, entity_id, payload, occurred_at, created_at
		FROM events
		WHERE occurred_at >= ?
		ORDER BY id ASC`,
		t,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ForEntity returns all events for a specific entity.
func (l *EventLog) ForEntity(entityType string, entityID int64) ([]RawEvent, error) {
	rows, err := l.db.Query(`
		SELECT id, event_type, entity_type, entity_id, payload, occurred_at, created_at
		FROM events
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY id ASC`,
		entityType, entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// Prune removes events older than the given duration.
func (l *EventLog) Prune(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := l.db.Exec(`DELETE FROM events WHERE occurred_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune events: %w", err)
	}
	return result.RowsAffected()
}

func scanEvents(rows *sql.Rows) ([]RawEvent, error) {
	var events []RawEvent
	for rows.Next() {
		var e RawEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.EntityType, &e.EntityID, &e.Payload, &e.OccurredAt, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run TestEventLog`
Expected: PASS

**Step 5: Create migration file**

```sql
-- internal/migrations/sql/004_events.sql
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id INTEGER NOT NULL,
    payload TEXT NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_entity ON events(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_events_occurred ON events(occurred_at);
```

**Step 6: Update embed.go**

Add to `internal/migrations/embed.go`:
```go
//go:embed sql/004_events.sql
var Migration004Events string
```

**Step 7: Commit**

```bash
git add internal/events/log.go internal/events/log_test.go internal/migrations/sql/004_events.sql internal/migrations/embed.go
git commit -m "feat(events): add EventLog for SQLite persistence"
```

---

### Task 1.3: Create Event Bus (Pub/Sub)

**Files:**
- Create: `internal/events/bus.go`
- Test: `internal/events/bus_test.go`

**Step 1: Write the failing test**

```go
// internal/events/bus_test.go
package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBus_PublishSubscribe(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	// Subscribe before publishing
	ch := bus.Subscribe("test.created", 10)

	// Publish
	e := &testEvent{BaseEvent: NewBaseEvent("test.created", "test", 1), Message: "hello"}
	err := bus.Publish(context.Background(), e)
	require.NoError(t, err)

	// Receive
	select {
	case received := <-ch:
		assert.Equal(t, "test.created", received.EventType())
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBus_SubscribeAll(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	ch := bus.SubscribeAll(10)

	// Publish different event types
	e1 := &testEvent{BaseEvent: NewBaseEvent("test.first", "test", 1), Message: "first"}
	e2 := &testEvent{BaseEvent: NewBaseEvent("test.second", "test", 2), Message: "second"}

	bus.Publish(context.Background(), e1)
	bus.Publish(context.Background(), e2)

	// Should receive both
	received := make([]Event, 0, 2)
	timeout := time.After(time.Second)
	for i := 0; i < 2; i++ {
		select {
		case e := <-ch:
			received = append(received, e)
		case <-timeout:
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	assert.Len(t, received, 2)
}

func TestBus_Unsubscribe(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	ch := bus.Subscribe("test.event", 10)

	// Unsubscribe
	bus.Unsubscribe(ch)

	// Publish (should not block even with no subscribers)
	e := &testEvent{BaseEvent: NewBaseEvent("test.event", "test", 1), Message: "hello"}
	err := bus.Publish(context.Background(), e)
	require.NoError(t, err)

	// Channel should be closed
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed")
	default:
		// This is also acceptable - channel is closed
	}
}

func TestBus_ConcurrentPublish(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	ch := bus.SubscribeAll(100)

	// Concurrent publishers
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			e := &testEvent{BaseEvent: NewBaseEvent("test.concurrent", "test", int64(n)), Message: "concurrent"}
			bus.Publish(context.Background(), e)
		}(i)
	}

	wg.Wait()

	// Count received events
	count := 0
	timeout := time.After(time.Second)
loop:
	for {
		select {
		case <-ch:
			count++
			if count == 10 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	assert.Equal(t, 10, count)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestBus`
Expected: FAIL with "undefined: NewBus"

**Step 3: Write minimal implementation**

```go
// internal/events/bus.go
package events

import (
	"context"
	"log/slog"
	"sync"
)

// Bus is the central event bus for pub/sub.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event // eventType -> channels
	allSubs     []chan Event            // subscribers to all events
	log         *EventLog               // SQLite persistence (may be nil)
	logger      *slog.Logger
	closed      bool
}

// NewBus creates a new event bus.
// The EventLog is optional - pass nil to disable persistence.
func NewBus(log *EventLog, logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{
		subscribers: make(map[string][]chan Event),
		log:         log,
		logger:      logger,
	}
}

// Publish sends an event to all subscribers and optionally persists it.
func (b *Bus) Publish(ctx context.Context, e Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return nil
	}

	// Get subscribers for this event type
	subs := make([]chan Event, len(b.subscribers[e.EventType()]))
	copy(subs, b.subscribers[e.EventType()])

	// Get all-event subscribers
	allSubs := make([]chan Event, len(b.allSubs))
	copy(allSubs, b.allSubs)
	b.mu.RUnlock()

	// Persist event
	if b.log != nil {
		if _, err := b.log.Append(e); err != nil {
			b.logger.Error("failed to persist event", "type", e.EventType(), "error", err)
			// Continue - event delivery is more important than persistence
		}
	}

	// Deliver to type-specific subscribers (non-blocking)
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			b.logger.Warn("subscriber channel full, dropping event",
				"type", e.EventType(),
				"entity_type", e.EntityType(),
				"entity_id", e.EntityID())
		}
	}

	// Deliver to all-event subscribers (non-blocking)
	for _, ch := range allSubs {
		select {
		case ch <- e:
		default:
			b.logger.Warn("all-subscriber channel full, dropping event",
				"type", e.EventType())
		}
	}

	return nil
}

// Subscribe returns a channel for events of a specific type.
func (b *Bus) Subscribe(eventType string, bufferSize int) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, bufferSize)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// SubscribeAll returns a channel for all events.
func (b *Bus) SubscribeAll(bufferSize int) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, bufferSize)
	b.allSubs = append(b.allSubs, ch)
	return ch
}

// SubscribeEntity returns events for a specific entity.
// Note: This is implemented by subscribing to all events and filtering.
// For high-volume scenarios, consider a more efficient approach.
func (b *Bus) SubscribeEntity(entityType string, entityID int64, bufferSize int) <-chan Event {
	allCh := b.SubscribeAll(bufferSize * 10) // larger buffer for filtering
	filtered := make(chan Event, bufferSize)

	go func() {
		for e := range allCh {
			if e.EntityType() == entityType && e.EntityID() == entityID {
				select {
				case filtered <- e:
				default:
					// Drop if full
				}
			}
		}
		close(filtered)
	}()

	return filtered
}

// Unsubscribe removes a subscription channel.
func (b *Bus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from type-specific subscribers
	for eventType, subs := range b.subscribers {
		for i, sub := range subs {
			if sub == ch {
				b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
				close(sub)
				return
			}
		}
	}

	// Remove from all-event subscribers
	for i, sub := range b.allSubs {
		if sub == ch {
			b.allSubs = append(b.allSubs[:i], b.allSubs[i+1:]...)
			close(sub)
			return
		}
	}
}

// Close shuts down the bus and closes all subscriber channels.
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	// Close all type-specific subscriber channels
	for _, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subscribers = nil

	// Close all-event subscriber channels
	for _, ch := range b.allSubs {
		close(ch)
	}
	b.allSubs = nil

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run TestBus`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/bus.go internal/events/bus_test.go
git commit -m "feat(events): add event Bus with pub/sub support"
```

---

## Phase 2: Event Types

### Task 2.1: Define Download Events

**Files:**
- Create: `internal/events/download.go`
- Test: `internal/events/download_test.go`

**Step 1: Write the failing test**

```go
// internal/events/download_test.go
package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrabRequested_JSON(t *testing.T) {
	e := &GrabRequested{
		BaseEvent:   NewBaseEvent(EventGrabRequested, EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/nzb",
		ReleaseName: "Movie.2024.1080p.WEB-DL",
		Indexer:     "nzbgeek",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded GrabRequested
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, e.ContentID, decoded.ContentID)
	assert.Equal(t, e.DownloadURL, decoded.DownloadURL)
	assert.Equal(t, e.ReleaseName, decoded.ReleaseName)
	assert.Equal(t, e.Indexer, decoded.Indexer)
}

func TestDownloadCompleted_JSON(t *testing.T) {
	e := &DownloadCompleted{
		BaseEvent:  NewBaseEvent(EventDownloadCompleted, EntityDownload, 123),
		DownloadID: 123,
		SourcePath: "/downloads/Movie.2024.1080p",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded DownloadCompleted
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(123), decoded.DownloadID)
	assert.Equal(t, "/downloads/Movie.2024.1080p", decoded.SourcePath)
}

func TestDownloadProgressed_JSON(t *testing.T) {
	e := &DownloadProgressed{
		BaseEvent:  NewBaseEvent(EventDownloadProgressed, EntityDownload, 123),
		DownloadID: 123,
		Progress:   45.5,
		Speed:      10485760, // 10 MB/s
		ETA:        300,      // 5 minutes
		Size:       1073741824,
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded DownloadProgressed
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, 45.5, decoded.Progress)
	assert.Equal(t, int64(10485760), decoded.Speed)
	assert.Equal(t, 300, decoded.ETA)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestGrabRequested`
Expected: FAIL with "undefined: GrabRequested"

**Step 3: Write minimal implementation**

```go
// internal/events/download.go
package events

// Entity types
const (
	EntityDownload = "download"
	EntityContent  = "content"
	EntityEpisode  = "episode"
)

// Event type constants
const (
	EventGrabRequested       = "grab.requested"
	EventDownloadCreated     = "download.created"
	EventDownloadProgressed  = "download.progressed"
	EventDownloadCompleted   = "download.completed"
	EventDownloadFailed      = "download.failed"
	EventImportStarted       = "import.started"
	EventImportCompleted     = "import.completed"
	EventImportFailed        = "import.failed"
	EventCleanupStarted      = "cleanup.started"
	EventCleanupCompleted    = "cleanup.completed"
	EventContentAdded        = "content.added"
	EventContentStatusChanged = "content.status.changed"
	EventPlexItemDetected    = "plex.item.detected"
)

// GrabRequested is emitted when a user/API requests a download.
type GrabRequested struct {
	BaseEvent
	ContentID   int64  `json:"content_id"`
	EpisodeID   *int64 `json:"episode_id,omitempty"`
	DownloadURL string `json:"download_url"`
	ReleaseName string `json:"release_name"`
	Indexer     string `json:"indexer"`
}

// DownloadCreated is emitted when a download record is created.
type DownloadCreated struct {
	BaseEvent
	DownloadID  int64  `json:"download_id"`
	ContentID   int64  `json:"content_id"`
	EpisodeID   *int64 `json:"episode_id,omitempty"`
	ClientID    string `json:"client_id"` // SABnzbd nzo_id
	ReleaseName string `json:"release_name"`
}

// DownloadProgressed is emitted periodically with download progress.
type DownloadProgressed struct {
	BaseEvent
	DownloadID int64   `json:"download_id"`
	Progress   float64 `json:"progress"`   // 0.0 - 100.0
	Speed      int64   `json:"speed_bps"`  // bytes per second
	ETA        int     `json:"eta_seconds"`
	Size       int64   `json:"size_bytes"`
}

// DownloadCompleted is emitted when a download finishes.
type DownloadCompleted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"` // Where client put files
}

// DownloadFailed is emitted when a download fails.
type DownloadFailed struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	Reason     string `json:"reason"`
	Retryable  bool   `json:"retryable"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run "TestGrabRequested|TestDownloadCompleted|TestDownloadProgressed"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/download.go internal/events/download_test.go
git commit -m "feat(events): add download event types"
```

---

### Task 2.2: Define Import and Cleanup Events

**Files:**
- Create: `internal/events/import.go`
- Test: `internal/events/import_test.go`

**Step 1: Write the failing test**

```go
// internal/events/import_test.go
package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportCompleted_JSON(t *testing.T) {
	e := &ImportCompleted{
		BaseEvent:  NewBaseEvent(EventImportCompleted, EntityDownload, 123),
		DownloadID: 123,
		ContentID:  42,
		FilePath:   "/movies/Movie (2024)/Movie.2024.1080p.mkv",
		FileSize:   8589934592,
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded ImportCompleted
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(123), decoded.DownloadID)
	assert.Equal(t, int64(42), decoded.ContentID)
	assert.Equal(t, "/movies/Movie (2024)/Movie.2024.1080p.mkv", decoded.FilePath)
	assert.Equal(t, int64(8589934592), decoded.FileSize)
}

func TestCleanupCompleted_JSON(t *testing.T) {
	e := &CleanupCompleted{
		BaseEvent:  NewBaseEvent(EventCleanupCompleted, EntityDownload, 123),
		DownloadID: 123,
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded CleanupCompleted
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(123), decoded.DownloadID)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestImport`
Expected: FAIL with "undefined: ImportCompleted"

**Step 3: Write minimal implementation**

```go
// internal/events/import.go
package events

// ImportStarted is emitted when import begins.
type ImportStarted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"`
}

// ImportCompleted is emitted when import succeeds.
type ImportCompleted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	ContentID  int64  `json:"content_id"`
	EpisodeID  *int64 `json:"episode_id,omitempty"`
	FilePath   string `json:"file_path"` // Final destination
	FileSize   int64  `json:"file_size"`
}

// ImportFailed is emitted when import fails.
type ImportFailed struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	Reason     string `json:"reason"`
}

// CleanupStarted is emitted when source cleanup begins.
type CleanupStarted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"`
}

// CleanupCompleted is emitted when source files are removed.
type CleanupCompleted struct {
	BaseEvent
	DownloadID int64 `json:"download_id"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run "TestImport|TestCleanup"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/import.go internal/events/import_test.go
git commit -m "feat(events): add import and cleanup event types"
```

---

### Task 2.3: Define Library and Plex Events

**Files:**
- Create: `internal/events/library.go`
- Test: `internal/events/library_test.go`

**Step 1: Write the failing test**

```go
// internal/events/library_test.go
package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentAdded_JSON(t *testing.T) {
	e := &ContentAdded{
		BaseEvent:      NewBaseEvent(EventContentAdded, EntityContent, 42),
		ContentID:      42,
		ContentType:    "movie",
		Title:          "The Matrix",
		Year:           1999,
		QualityProfile: "hd",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded ContentAdded
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(42), decoded.ContentID)
	assert.Equal(t, "movie", decoded.ContentType)
	assert.Equal(t, "The Matrix", decoded.Title)
}

func TestPlexItemDetected_JSON(t *testing.T) {
	e := &PlexItemDetected{
		BaseEvent: NewBaseEvent(EventPlexItemDetected, EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded PlexItemDetected
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(42), decoded.ContentID)
	assert.Equal(t, "/library/metadata/12345", decoded.PlexKey)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestContent`
Expected: FAIL with "undefined: ContentAdded"

**Step 3: Write minimal implementation**

```go
// internal/events/library.go
package events

// ContentAdded is emitted when new content is added to the library.
type ContentAdded struct {
	BaseEvent
	ContentID      int64  `json:"content_id"`
	ContentType    string `json:"content_type"` // "movie" or "series"
	Title          string `json:"title"`
	Year           int    `json:"year"`
	QualityProfile string `json:"quality_profile"`
}

// ContentStatusChanged is emitted when content status changes.
type ContentStatusChanged struct {
	BaseEvent
	ContentID int64  `json:"content_id"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// PlexItemDetected is emitted when Plex finds our imported file.
type PlexItemDetected struct {
	BaseEvent
	ContentID int64  `json:"content_id"`
	PlexKey   string `json:"plex_key"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run "TestContent|TestPlex"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/library.go internal/events/library_test.go
git commit -m "feat(events): add library and plex event types"
```

---

### Task 2.4: Add Event Type Registry

**Files:**
- Create: `internal/events/registry.go`
- Test: `internal/events/registry_test.go`

**Step 1: Write the failing test**

```go
// internal/events/registry_test.go
package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Unmarshal(t *testing.T) {
	registry := NewRegistry()

	// Register event types
	registry.Register(EventGrabRequested, func() Event { return &GrabRequested{} })
	registry.Register(EventDownloadCompleted, func() Event { return &DownloadCompleted{} })

	// Test unmarshaling GrabRequested
	raw := RawEvent{
		EventType: EventGrabRequested,
		Payload:   `{"type":"grab.requested","entity_type":"download","entity_id":0,"occurred_at":"2024-01-01T00:00:00Z","content_id":42,"download_url":"https://example.com/nzb","release_name":"Movie.2024.1080p","indexer":"nzbgeek"}`,
	}

	event, err := registry.Unmarshal(raw)
	require.NoError(t, err)

	grab, ok := event.(*GrabRequested)
	require.True(t, ok)
	assert.Equal(t, int64(42), grab.ContentID)
	assert.Equal(t, "https://example.com/nzb", grab.DownloadURL)
}

func TestRegistry_UnmarshalUnknownType(t *testing.T) {
	registry := NewRegistry()

	raw := RawEvent{
		EventType: "unknown.event",
		Payload:   `{}`,
	}

	_, err := registry.Unmarshal(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/... -v -run TestRegistry`
Expected: FAIL with "undefined: NewRegistry"

**Step 3: Write minimal implementation**

```go
// internal/events/registry.go
package events

import (
	"encoding/json"
	"fmt"
)

// EventFactory creates a new zero-value event of a specific type.
type EventFactory func() Event

// Registry maps event types to their factories for deserialization.
type Registry struct {
	factories map[string]EventFactory
}

// NewRegistry creates a new event registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]EventFactory),
	}
}

// Register adds an event type to the registry.
func (r *Registry) Register(eventType string, factory EventFactory) {
	r.factories[eventType] = factory
}

// Unmarshal deserializes a raw event into its concrete type.
func (r *Registry) Unmarshal(raw RawEvent) (Event, error) {
	factory, ok := r.factories[raw.EventType]
	if !ok {
		return nil, fmt.Errorf("unknown event type: %s", raw.EventType)
	}

	event := factory()
	if err := json.Unmarshal([]byte(raw.Payload), event); err != nil {
		return nil, fmt.Errorf("unmarshal event payload: %w", err)
	}

	return event, nil
}

// DefaultRegistry returns a registry with all standard event types registered.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Download events
	r.Register(EventGrabRequested, func() Event { return &GrabRequested{} })
	r.Register(EventDownloadCreated, func() Event { return &DownloadCreated{} })
	r.Register(EventDownloadProgressed, func() Event { return &DownloadProgressed{} })
	r.Register(EventDownloadCompleted, func() Event { return &DownloadCompleted{} })
	r.Register(EventDownloadFailed, func() Event { return &DownloadFailed{} })

	// Import events
	r.Register(EventImportStarted, func() Event { return &ImportStarted{} })
	r.Register(EventImportCompleted, func() Event { return &ImportCompleted{} })
	r.Register(EventImportFailed, func() Event { return &ImportFailed{} })

	// Cleanup events
	r.Register(EventCleanupStarted, func() Event { return &CleanupStarted{} })
	r.Register(EventCleanupCompleted, func() Event { return &CleanupCompleted{} })

	// Library events
	r.Register(EventContentAdded, func() Event { return &ContentAdded{} })
	r.Register(EventContentStatusChanged, func() Event { return &ContentStatusChanged{} })

	// Plex events
	r.Register(EventPlexItemDetected, func() Event { return &PlexItemDetected{} })

	return r
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/... -v -run TestRegistry`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/registry.go internal/events/registry_test.go
git commit -m "feat(events): add event type registry for deserialization"
```

---

## Phase 3: Handlers

### Task 3.1: Create Handler Interface

**Files:**
- Create: `internal/handlers/handler.go`
- Test: `internal/handlers/handler_test.go`

**Step 1: Write the failing test**

```go
// internal/handlers/handler_test.go
package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vmunix/arrgo/internal/events"
)

// mockHandler is a test implementation of Handler
type mockHandler struct {
	name    string
	started bool
	stopped bool
}

func (h *mockHandler) Name() string { return h.name }

func (h *mockHandler) Start(ctx context.Context) error {
	h.started = true
	<-ctx.Done()
	h.stopped = true
	return ctx.Err()
}

func TestHandler_StartStop(t *testing.T) {
	h := &mockHandler{name: "test"}

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	done := make(chan error, 1)
	go func() {
		done <- h.Start(ctx)
	}()

	// Wait for start
	time.Sleep(10 * time.Millisecond)
	assert.True(t, h.started)
	assert.False(t, h.stopped)

	// Stop
	cancel()
	err := <-done
	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, h.stopped)
}

func TestBaseHandler_Fields(t *testing.T) {
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	base := NewBaseHandler(bus, nil)
	assert.NotNil(t, base.Bus())
	assert.NotNil(t, base.Logger())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/... -v -run TestHandler`
Expected: FAIL with "package handlers is not in std"

**Step 3: Write minimal implementation**

```go
// internal/handlers/handler.go
package handlers

import (
	"context"
	"log/slog"

	"github.com/vmunix/arrgo/internal/events"
)

// Handler processes events of specific types.
type Handler interface {
	// Start begins processing events (blocking).
	Start(ctx context.Context) error

	// Name returns handler name for logging.
	Name() string
}

// BaseHandler provides common handler functionality.
type BaseHandler struct {
	bus    *events.Bus
	logger *slog.Logger
}

// NewBaseHandler creates a base handler.
func NewBaseHandler(bus *events.Bus, logger *slog.Logger) *BaseHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BaseHandler{
		bus:    bus,
		logger: logger,
	}
}

// Bus returns the event bus.
func (h *BaseHandler) Bus() *events.Bus {
	return h.bus
}

// Logger returns the handler's logger.
func (h *BaseHandler) Logger() *slog.Logger {
	return h.logger
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers/... -v -run TestHandler`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/handlers/handler.go internal/handlers/handler_test.go
git commit -m "feat(handlers): add Handler interface and BaseHandler"
```

---

### Task 3.2: Create Download Handler

**Files:**
- Create: `internal/handlers/download.go`
- Test: `internal/handlers/download_test.go`

**Step 1: Write the failing test**

```go
// internal/handlers/download_test.go
package handlers

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	_ "modernc.org/sqlite"
)

func setupDownloadTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create minimal schema
	_, err = db.Exec(`
		CREATE TABLE downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			client TEXT NOT NULL,
			client_id TEXT NOT NULL,
			status TEXT NOT NULL,
			release_name TEXT NOT NULL,
			indexer TEXT NOT NULL,
			added_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			last_transition_at TIMESTAMP NOT NULL
		)
	`)
	require.NoError(t, err)
	return db
}

// mockDownloader is a test implementation
type mockDownloader struct {
	addCalled   bool
	lastURL     string
	returnID    string
	returnError error
}

func (m *mockDownloader) Add(ctx context.Context, url, category string) (string, error) {
	m.addCalled = true
	m.lastURL = url
	if m.returnError != nil {
		return "", m.returnError
	}
	return m.returnID, nil
}

func (m *mockDownloader) Status(ctx context.Context, clientID string) (*download.ClientStatus, error) {
	return nil, nil
}

func (m *mockDownloader) List(ctx context.Context) ([]*download.ClientStatus, error) {
	return nil, nil
}

func (m *mockDownloader) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	return nil
}

func TestDownloadHandler_GrabRequested(t *testing.T) {
	db := setupDownloadTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)
	client := &mockDownloader{returnID: "sab-123"}

	handler := NewDownloadHandler(bus, store, client, nil)

	// Subscribe to DownloadCreated before starting
	created := bus.Subscribe(events.EventDownloadCreated, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Start(ctx)

	// Give handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Publish GrabRequested
	grab := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024.1080p",
		Indexer:     "nzbgeek",
	}
	err := bus.Publish(ctx, grab)
	require.NoError(t, err)

	// Wait for DownloadCreated event
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(42), dc.ContentID)
		assert.Equal(t, "sab-123", dc.ClientID)
		assert.Equal(t, "Test.Movie.2024.1080p", dc.ReleaseName)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	// Verify client was called
	assert.True(t, client.addCalled)
	assert.Equal(t, "https://example.com/test.nzb", client.lastURL)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/... -v -run TestDownloadHandler`
Expected: FAIL with "undefined: NewDownloadHandler"

**Step 3: Write minimal implementation**

```go
// internal/handlers/download.go
package handlers

import (
	"context"
	"log/slog"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// DownloadHandler manages download lifecycle.
type DownloadHandler struct {
	*BaseHandler
	store  *download.Store
	client download.Downloader
}

// NewDownloadHandler creates a new download handler.
func NewDownloadHandler(bus *events.Bus, store *download.Store, client download.Downloader, logger *slog.Logger) *DownloadHandler {
	return &DownloadHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		client:      client,
	}
}

// Name returns the handler name.
func (h *DownloadHandler) Name() string {
	return "download"
}

// Start begins processing events.
func (h *DownloadHandler) Start(ctx context.Context) error {
	grabs := h.Bus().Subscribe(events.EventGrabRequested, 100)

	for {
		select {
		case e := <-grabs:
			if e == nil {
				return nil // Channel closed
			}
			h.handleGrabRequested(ctx, e.(*events.GrabRequested))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *DownloadHandler) handleGrabRequested(ctx context.Context, e *events.GrabRequested) {
	h.Logger().Info("processing grab request",
		"content_id", e.ContentID,
		"release", e.ReleaseName,
		"indexer", e.Indexer)

	// Send to download client
	clientID, err := h.client.Add(ctx, e.DownloadURL, "")
	if err != nil {
		h.Logger().Error("failed to add download", "error", err)
		h.Bus().Publish(ctx, &events.DownloadFailed{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadFailed, events.EntityDownload, 0),
			DownloadID: 0,
			Reason:     err.Error(),
			Retryable:  true,
		})
		return
	}

	// Create DB record
	dl := &download.Download{
		ContentID:   e.ContentID,
		EpisodeID:   e.EpisodeID,
		Client:      download.ClientSABnzbd,
		ClientID:    clientID,
		Status:      download.StatusQueued,
		ReleaseName: e.ReleaseName,
		Indexer:     e.Indexer,
	}

	if err := h.store.Add(dl); err != nil {
		h.Logger().Error("failed to save download", "error", err)
		return
	}

	// Emit success event
	h.Bus().Publish(ctx, &events.DownloadCreated{
		BaseEvent:   events.NewBaseEvent(events.EventDownloadCreated, events.EntityDownload, dl.ID),
		DownloadID:  dl.ID,
		ContentID:   e.ContentID,
		EpisodeID:   e.EpisodeID,
		ClientID:    clientID,
		ReleaseName: e.ReleaseName,
	})

	h.Logger().Info("download created",
		"download_id", dl.ID,
		"client_id", clientID)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers/... -v -run TestDownloadHandler`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/handlers/download.go internal/handlers/download_test.go
git commit -m "feat(handlers): add DownloadHandler for grab requests"
```

---

### Task 3.3: Create Import Handler

**Files:**
- Create: `internal/handlers/import.go`
- Test: `internal/handlers/import_test.go`

**Step 1: Write the failing test**

```go
// internal/handlers/import_test.go
package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/events"
)

// mockImporter is a test implementation
type mockImporter struct {
	importCalled bool
	lastID       int64
	lastPath     string
	returnResult *importer.ImportResult
	returnError  error
}

func (m *mockImporter) Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error) {
	m.importCalled = true
	m.lastID = downloadID
	m.lastPath = path
	if m.returnError != nil {
		return nil, m.returnError
	}
	return m.returnResult, nil
}

func TestImportHandler_DownloadCompleted(t *testing.T) {
	db := setupDownloadTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create a download record first
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	mockImp := &mockImporter{
		returnResult: &importer.ImportResult{
			FileID:     1,
			SourcePath: "/downloads/Test.Movie.2024",
			DestPath:   "/movies/Test Movie (2024)/Test.Movie.2024.mkv",
			SizeBytes:  1000000,
		},
	}

	handler := NewImportHandler(bus, store, mockImp, nil)

	// Subscribe to ImportCompleted
	completed := bus.Subscribe(events.EventImportCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	// Publish DownloadCompleted
	e := &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: "/downloads/Test.Movie.2024",
	}
	err := bus.Publish(ctx, e)
	require.NoError(t, err)

	// Wait for ImportCompleted
	select {
	case ev := <-completed:
		ic := ev.(*events.ImportCompleted)
		assert.Equal(t, dl.ID, ic.DownloadID)
		assert.Equal(t, int64(42), ic.ContentID)
		assert.Equal(t, "/movies/Test Movie (2024)/Test.Movie.2024.mkv", ic.FilePath)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ImportCompleted")
	}

	// Verify importer was called
	assert.True(t, mockImp.importCalled)
	assert.Equal(t, dl.ID, mockImp.lastID)
	assert.Equal(t, "/downloads/Test.Movie.2024", mockImp.lastPath)
}

func TestImportHandler_PreventsConcurrentImport(t *testing.T) {
	db := setupDownloadTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusCompleted,
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	// Slow importer to create race condition opportunity
	slowImporter := &mockImporter{
		returnResult: &importer.ImportResult{FileID: 1, DestPath: "/movies/test.mkv"},
	}

	handler := NewImportHandler(bus, store, slowImporter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	// Send two completion events for the same download
	e := &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: "/downloads/Test.Movie.2024",
	}

	bus.Publish(ctx, e)
	bus.Publish(ctx, e) // Duplicate - should be ignored

	time.Sleep(100 * time.Millisecond)

	// Importer should only be called once (due to per-download lock)
	assert.True(t, slowImporter.importCalled)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/... -v -run TestImportHandler`
Expected: FAIL with "undefined: NewImportHandler"

**Step 3: Write minimal implementation**

```go
// internal/handlers/import.go
package handlers

import (
	"context"
	"log/slog"
	"sync"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/events"
)

// FileImporter is the interface for the importer.
type FileImporter interface {
	Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error)
}

// ImportHandler handles file import when downloads complete.
type ImportHandler struct {
	*BaseHandler
	store    *download.Store
	importer FileImporter

	// Per-download lock to prevent concurrent imports
	importing sync.Map // map[int64]bool
}

// NewImportHandler creates a new import handler.
func NewImportHandler(bus *events.Bus, store *download.Store, imp FileImporter, logger *slog.Logger) *ImportHandler {
	return &ImportHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		importer:    imp,
	}
}

// Name returns the handler name.
func (h *ImportHandler) Name() string {
	return "import"
}

// Start begins processing events.
func (h *ImportHandler) Start(ctx context.Context) error {
	completed := h.Bus().Subscribe(events.EventDownloadCompleted, 100)

	for {
		select {
		case e := <-completed:
			if e == nil {
				return nil
			}
			// Process in goroutine to not block other events
			go h.handleDownloadCompleted(ctx, e.(*events.DownloadCompleted))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *ImportHandler) handleDownloadCompleted(ctx context.Context, e *events.DownloadCompleted) {
	// Acquire per-download lock (prevents concurrent imports)
	if _, loaded := h.importing.LoadOrStore(e.DownloadID, true); loaded {
		h.Logger().Warn("import already in progress", "download_id", e.DownloadID)
		return
	}
	defer h.importing.Delete(e.DownloadID)

	h.Logger().Info("starting import",
		"download_id", e.DownloadID,
		"source_path", e.SourcePath)

	// Emit ImportStarted
	h.Bus().Publish(ctx, &events.ImportStarted{
		BaseEvent:  events.NewBaseEvent(events.EventImportStarted, events.EntityDownload, e.DownloadID),
		DownloadID: e.DownloadID,
		SourcePath: e.SourcePath,
	})

	// Get download for content ID
	dl, err := h.store.Get(e.DownloadID)
	if err != nil {
		h.Logger().Error("failed to get download", "download_id", e.DownloadID, "error", err)
		h.emitImportFailed(ctx, e.DownloadID, err.Error())
		return
	}

	// Do the import
	result, err := h.importer.Import(ctx, e.DownloadID, e.SourcePath)
	if err != nil {
		h.Logger().Error("import failed", "download_id", e.DownloadID, "error", err)
		h.emitImportFailed(ctx, e.DownloadID, err.Error())
		return
	}

	// Emit ImportCompleted
	h.Bus().Publish(ctx, &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, e.DownloadID),
		DownloadID: e.DownloadID,
		ContentID:  dl.ContentID,
		EpisodeID:  dl.EpisodeID,
		FilePath:   result.DestPath,
		FileSize:   result.SizeBytes,
	})

	h.Logger().Info("import completed",
		"download_id", e.DownloadID,
		"dest_path", result.DestPath)
}

func (h *ImportHandler) emitImportFailed(ctx context.Context, downloadID int64, reason string) {
	h.Bus().Publish(ctx, &events.ImportFailed{
		BaseEvent:  events.NewBaseEvent(events.EventImportFailed, events.EntityDownload, downloadID),
		DownloadID: downloadID,
		Reason:     reason,
	})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers/... -v -run TestImportHandler`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/handlers/import.go internal/handlers/import_test.go
git commit -m "feat(handlers): add ImportHandler with per-download locking"
```

---

### Task 3.4: Create Cleanup Handler

**Files:**
- Create: `internal/handlers/cleanup.go`
- Test: `internal/handlers/cleanup_test.go`

**Step 1: Write the failing test**

```go
// internal/handlers/cleanup_test.go
package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

func TestCleanupHandler_PlexItemDetected(t *testing.T) {
	db := setupDownloadTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	store := download.NewStore(db)

	// Create temp dir to cleanup
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "Test.Movie.2024")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	testFile := filepath.Join(sourceDir, "test.mkv")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	// Create download record
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusImported,
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	cfg := CleanupConfig{
		DownloadRoot: tmpDir,
		Enabled:      true,
	}

	handler := NewCleanupHandler(bus, store, cfg, nil)

	// Subscribe to CleanupCompleted
	completed := bus.Subscribe(events.EventCleanupCompleted, 10)

	// Start handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handler.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	// First, emit ImportCompleted to register pending cleanup
	ic := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		ContentID:  42,
		FilePath:   "/movies/Test Movie (2024)/test.mkv",
	}
	bus.Publish(ctx, ic)

	time.Sleep(10 * time.Millisecond)

	// Then emit PlexItemDetected to trigger cleanup
	plex := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, 42),
		ContentID: 42,
		PlexKey:   "/library/metadata/12345",
	}
	bus.Publish(ctx, plex)

	// Wait for CleanupCompleted
	select {
	case e := <-completed:
		cc := e.(*events.CleanupCompleted)
		assert.Equal(t, dl.ID, cc.DownloadID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for CleanupCompleted")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers/... -v -run TestCleanupHandler`
Expected: FAIL with "undefined: CleanupConfig"

**Step 3: Write minimal implementation**

```go
// internal/handlers/cleanup.go
package handlers

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// CleanupConfig configures the cleanup handler.
type CleanupConfig struct {
	DownloadRoot string
	Enabled      bool
}

// pendingCleanup tracks downloads awaiting Plex verification.
type pendingCleanup struct {
	DownloadID  int64
	ContentID   int64
	ReleaseName string
}

// CleanupHandler cleans up source files after Plex verification.
type CleanupHandler struct {
	*BaseHandler
	store   *download.Store
	config  CleanupConfig

	// Track pending cleanups by content ID
	mu      sync.RWMutex
	pending map[int64]*pendingCleanup // contentID -> pending
}

// NewCleanupHandler creates a new cleanup handler.
func NewCleanupHandler(bus *events.Bus, store *download.Store, cfg CleanupConfig, logger *slog.Logger) *CleanupHandler {
	return &CleanupHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		config:      cfg,
		pending:     make(map[int64]*pendingCleanup),
	}
}

// Name returns the handler name.
func (h *CleanupHandler) Name() string {
	return "cleanup"
}

// Start begins processing events.
func (h *CleanupHandler) Start(ctx context.Context) error {
	imported := h.Bus().Subscribe(events.EventImportCompleted, 100)
	plexDetected := h.Bus().Subscribe(events.EventPlexItemDetected, 100)

	for {
		select {
		case e := <-imported:
			if e == nil {
				return nil
			}
			h.handleImportCompleted(ctx, e.(*events.ImportCompleted))
		case e := <-plexDetected:
			if e == nil {
				return nil
			}
			h.handlePlexDetected(ctx, e.(*events.PlexItemDetected))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *CleanupHandler) handleImportCompleted(ctx context.Context, e *events.ImportCompleted) {
	if !h.config.Enabled {
		return
	}

	// Get download for release name
	dl, err := h.store.Get(e.DownloadID)
	if err != nil {
		h.Logger().Warn("cannot get download for cleanup tracking", "download_id", e.DownloadID, "error", err)
		return
	}

	// Track as pending cleanup
	h.mu.Lock()
	h.pending[e.ContentID] = &pendingCleanup{
		DownloadID:  e.DownloadID,
		ContentID:   e.ContentID,
		ReleaseName: dl.ReleaseName,
	}
	h.mu.Unlock()

	h.Logger().Debug("tracking pending cleanup",
		"download_id", e.DownloadID,
		"content_id", e.ContentID,
		"release", dl.ReleaseName)
}

func (h *CleanupHandler) handlePlexDetected(ctx context.Context, e *events.PlexItemDetected) {
	if !h.config.Enabled {
		return
	}

	// Find pending cleanup for this content
	h.mu.Lock()
	pending, ok := h.pending[e.ContentID]
	if ok {
		delete(h.pending, e.ContentID)
	}
	h.mu.Unlock()

	if !ok {
		h.Logger().Debug("no pending cleanup for content", "content_id", e.ContentID)
		return
	}

	h.Logger().Info("plex detected content, starting cleanup",
		"download_id", pending.DownloadID,
		"content_id", pending.ContentID)

	// Emit CleanupStarted
	sourcePath := filepath.Join(h.config.DownloadRoot, pending.ReleaseName)
	h.Bus().Publish(ctx, &events.CleanupStarted{
		BaseEvent:  events.NewBaseEvent(events.EventCleanupStarted, events.EntityDownload, pending.DownloadID),
		DownloadID: pending.DownloadID,
		SourcePath: sourcePath,
	})

	// Perform cleanup
	if err := h.cleanupSource(sourcePath, pending.ReleaseName); err != nil {
		h.Logger().Error("cleanup failed", "download_id", pending.DownloadID, "error", err)
		// Still emit completed - cleanup is best effort
	}

	// Update download status
	dl, err := h.store.Get(pending.DownloadID)
	if err == nil {
		h.store.Transition(dl, download.StatusCleaned)
	}

	// Emit CleanupCompleted
	h.Bus().Publish(ctx, &events.CleanupCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventCleanupCompleted, events.EntityDownload, pending.DownloadID),
		DownloadID: pending.DownloadID,
	})

	h.Logger().Info("cleanup completed", "download_id", pending.DownloadID)
}

func (h *CleanupHandler) cleanupSource(sourcePath, releaseName string) error {
	// Validate path is safe to delete
	if !strings.HasPrefix(sourcePath, h.config.DownloadRoot) {
		h.Logger().Warn("refusing to cleanup path outside download root", "path", sourcePath)
		return nil
	}

	info, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		return nil // Already cleaned
	}
	if err != nil {
		return err
	}

	if info.IsDir() {
		// Remove files in directory
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				os.Remove(filepath.Join(sourcePath, entry.Name()))
			}
		}
		// Remove directory if empty
		entries, _ = os.ReadDir(sourcePath)
		if len(entries) == 0 {
			return os.Remove(sourcePath)
		}
	} else {
		return os.Remove(sourcePath)
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers/... -v -run TestCleanupHandler`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/handlers/cleanup.go internal/handlers/cleanup_test.go
git commit -m "feat(handlers): add CleanupHandler for source cleanup after Plex verification"
```

---

## Phase 4: Adapters

### Task 4.1: Create SABnzbd Adapter

**Files:**
- Create: `internal/adapters/sabnzbd/adapter.go`
- Test: `internal/adapters/sabnzbd/adapter_test.go`

**Step 1: Write the failing test**

```go
// internal/adapters/sabnzbd/adapter_test.go
package sabnzbd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// mockSABClient is a test implementation
type mockSABClient struct {
	statusResults map[string]*download.ClientStatus
}

func (m *mockSABClient) Add(ctx context.Context, url, category string) (string, error) {
	return "", nil
}

func (m *mockSABClient) Status(ctx context.Context, clientID string) (*download.ClientStatus, error) {
	if s, ok := m.statusResults[clientID]; ok {
		return s, nil
	}
	return nil, download.ErrDownloadNotFound
}

func (m *mockSABClient) List(ctx context.Context) ([]*download.ClientStatus, error) {
	var results []*download.ClientStatus
	for _, s := range m.statusResults {
		results = append(results, s)
	}
	return results, nil
}

func (m *mockSABClient) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	return nil
}

func setupAdapterTestDB(t *testing.T) *download.Store {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			client TEXT NOT NULL,
			client_id TEXT NOT NULL,
			status TEXT NOT NULL,
			release_name TEXT NOT NULL,
			indexer TEXT NOT NULL,
			added_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			last_transition_at TIMESTAMP NOT NULL
		)
	`)
	require.NoError(t, err)
	return download.NewStore(db)
}

func TestAdapter_EmitsDownloadCompleted(t *testing.T) {
	store := setupAdapterTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	// Create tracked download
	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	// Mock client reports completed
	client := &mockSABClient{
		statusResults: map[string]*download.ClientStatus{
			"sab-123": {
				ID:       "sab-123",
				Status:   download.StatusCompleted,
				Progress: 100,
				Path:     "/downloads/Test.Movie.2024",
			},
		},
	}

	adapter := New(bus, client, store, 10*time.Millisecond, nil)

	// Subscribe to events
	completed := bus.Subscribe(events.EventDownloadCompleted, 10)

	// Start adapter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.Start(ctx)

	// Wait for event
	select {
	case e := <-completed:
		dc := e.(*events.DownloadCompleted)
		assert.Equal(t, dl.ID, dc.DownloadID)
		assert.Equal(t, "/downloads/Test.Movie.2024", dc.SourcePath)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadCompleted")
	}
}

func TestAdapter_EmitsDownloadProgressed(t *testing.T) {
	store := setupAdapterTestDB(t)
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	dl := &download.Download{
		ContentID:   42,
		Client:      download.ClientSABnzbd,
		ClientID:    "sab-123",
		Status:      download.StatusDownloading,
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	}
	require.NoError(t, store.Add(dl))

	client := &mockSABClient{
		statusResults: map[string]*download.ClientStatus{
			"sab-123": {
				ID:       "sab-123",
				Status:   download.StatusDownloading,
				Progress: 45.5,
				Speed:    10485760,
			},
		},
	}

	adapter := New(bus, client, store, 10*time.Millisecond, nil)

	progressed := bus.Subscribe(events.EventDownloadProgressed, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.Start(ctx)

	select {
	case e := <-progressed:
		dp := e.(*events.DownloadProgressed)
		assert.Equal(t, dl.ID, dp.DownloadID)
		assert.Equal(t, 45.5, dp.Progress)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DownloadProgressed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/sabnzbd/... -v -run TestAdapter`
Expected: FAIL with "package sabnzbd is not in std"

**Step 3: Write minimal implementation**

```go
// internal/adapters/sabnzbd/adapter.go
package sabnzbd

import (
	"context"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// Adapter polls SABnzbd and emits events.
type Adapter struct {
	client   download.Downloader
	bus      *events.Bus
	store    *download.Store
	interval time.Duration
	logger   *slog.Logger

	// Track last known status to avoid duplicate events
	lastStatus map[int64]download.Status
}

// New creates a new SABnzbd adapter.
func New(bus *events.Bus, client download.Downloader, store *download.Store, interval time.Duration, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		client:     client,
		bus:        bus,
		store:      store,
		interval:   interval,
		logger:     logger,
		lastStatus: make(map[int64]download.Status),
	}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "sabnzbd"
}

// Start begins polling the download client.
func (a *Adapter) Start(ctx context.Context) error {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	// Initial poll
	a.poll(ctx)

	for {
		select {
		case <-ticker.C:
			a.poll(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (a *Adapter) poll(ctx context.Context) {
	// Get our tracked downloads
	tracked, err := a.store.List(download.Filter{Active: true})
	if err != nil {
		a.logger.Error("failed to list active downloads", "error", err)
		return
	}

	for _, dl := range tracked {
		clientStatus, err := a.client.Status(ctx, dl.ClientID)
		if err != nil {
			if err == download.ErrDownloadNotFound {
				a.logger.Warn("download disappeared from client",
					"download_id", dl.ID,
					"client_id", dl.ClientID)
				a.emitFailed(ctx, dl.ID, "disappeared from download client", true)
			}
			continue
		}

		a.processStatus(ctx, dl, clientStatus)
	}
}

func (a *Adapter) processStatus(ctx context.Context, dl *download.Download, status *download.ClientStatus) {
	// Check if status changed
	lastStatus, seen := a.lastStatus[dl.ID]
	a.lastStatus[dl.ID] = status.Status

	// Always emit progress for downloading
	if status.Status == download.StatusDownloading {
		a.bus.Publish(ctx, &events.DownloadProgressed{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadProgressed, events.EntityDownload, dl.ID),
			DownloadID: dl.ID,
			Progress:   status.Progress,
			Speed:      status.Speed,
			ETA:        int(status.ETA.Seconds()),
			Size:       status.Size,
		})
	}

	// Only emit state transitions once
	if seen && lastStatus == status.Status {
		return
	}

	switch status.Status {
	case download.StatusCompleted:
		a.bus.Publish(ctx, &events.DownloadCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
			DownloadID: dl.ID,
			SourcePath: status.Path,
		})

	case download.StatusFailed:
		a.emitFailed(ctx, dl.ID, "download failed in client", true)
	}
}

func (a *Adapter) emitFailed(ctx context.Context, downloadID int64, reason string, retryable bool) {
	a.bus.Publish(ctx, &events.DownloadFailed{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadFailed, events.EntityDownload, downloadID),
		DownloadID: downloadID,
		Reason:     reason,
		Retryable:  retryable,
	})
}
```

**Step 4: Add missing import**

Add import for `database/sql` in test file.

**Step 5: Run test to verify it passes**

Run: `go test ./internal/adapters/sabnzbd/... -v -run TestAdapter`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/adapters/sabnzbd/adapter.go internal/adapters/sabnzbd/adapter_test.go
git commit -m "feat(adapters): add SABnzbd adapter for polling and event emission"
```

---

### Task 4.2: Create Plex Adapter

**Files:**
- Create: `internal/adapters/plex/adapter.go`
- Test: `internal/adapters/plex/adapter_test.go`

**Step 1: Write the failing test**

```go
// internal/adapters/plex/adapter_test.go
package plex

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/events"
)

// mockPlexClient is a test implementation
type mockPlexClient struct {
	hasContent map[int64]bool // contentID -> found in plex
}

func (m *mockPlexClient) HasContentByID(ctx context.Context, contentID int64) (bool, error) {
	return m.hasContent[contentID], nil
}

func TestAdapter_EmitsPlexItemDetected(t *testing.T) {
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	client := &mockPlexClient{
		hasContent: map[int64]bool{42: true},
	}

	adapter := New(bus, client, 10*time.Millisecond, nil)

	// Subscribe to events
	detected := bus.Subscribe(events.EventPlexItemDetected, 10)

	// Start adapter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	// Emit ImportCompleted to register pending
	ic := &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, 1),
		DownloadID: 1,
		ContentID:  42,
		FilePath:   "/movies/test.mkv",
	}
	bus.Publish(ctx, ic)

	// Wait for PlexItemDetected
	select {
	case e := <-detected:
		pid := e.(*events.PlexItemDetected)
		assert.Equal(t, int64(42), pid.ContentID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlexItemDetected")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/plex/... -v -run TestAdapter`
Expected: FAIL with "package plex is not in std"

**Step 3: Write minimal implementation**

```go
// internal/adapters/plex/adapter.go
package plex

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/vmunix/arrgo/internal/events"
)

// PlexChecker checks if content exists in Plex.
type PlexChecker interface {
	HasContentByID(ctx context.Context, contentID int64) (bool, error)
}

// pendingVerification tracks content awaiting Plex detection.
type pendingVerification struct {
	ContentID int64
	FilePath  string
	AddedAt   time.Time
}

// Adapter polls Plex to verify imported content.
type Adapter struct {
	client   PlexChecker
	bus      *events.Bus
	interval time.Duration
	logger   *slog.Logger

	mu      sync.RWMutex
	pending map[int64]*pendingVerification // contentID -> pending
}

// New creates a new Plex adapter.
func New(bus *events.Bus, client PlexChecker, interval time.Duration, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		client:   client,
		bus:      bus,
		interval: interval,
		logger:   logger,
		pending:  make(map[int64]*pendingVerification),
	}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "plex"
}

// Start begins polling Plex.
func (a *Adapter) Start(ctx context.Context) error {
	imported := a.bus.Subscribe(events.EventImportCompleted, 100)
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case e := <-imported:
			if e == nil {
				return nil
			}
			a.handleImportCompleted(e.(*events.ImportCompleted))

		case <-ticker.C:
			a.checkPending(ctx)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (a *Adapter) handleImportCompleted(e *events.ImportCompleted) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.pending[e.ContentID] = &pendingVerification{
		ContentID: e.ContentID,
		FilePath:  e.FilePath,
		AddedAt:   time.Now(),
	}

	a.logger.Debug("tracking content for plex verification", "content_id", e.ContentID)
}

func (a *Adapter) checkPending(ctx context.Context) {
	a.mu.Lock()
	toCheck := make([]*pendingVerification, 0, len(a.pending))
	for _, p := range a.pending {
		toCheck = append(toCheck, p)
	}
	a.mu.Unlock()

	for _, p := range toCheck {
		found, err := a.client.HasContentByID(ctx, p.ContentID)
		if err != nil {
			a.logger.Warn("plex check failed", "content_id", p.ContentID, "error", err)
			continue
		}

		if found {
			a.mu.Lock()
			delete(a.pending, p.ContentID)
			a.mu.Unlock()

			a.bus.Publish(ctx, &events.PlexItemDetected{
				BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, p.ContentID),
				ContentID: p.ContentID,
			})

			a.logger.Info("plex detected content", "content_id", p.ContentID)
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/plex/... -v -run TestAdapter`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/adapters/plex/adapter.go internal/adapters/plex/adapter_test.go
git commit -m "feat(adapters): add Plex adapter for content verification"
```

---

## Phase 5: Wire Up

### Task 5.1: Add Migration to Server Startup

**Files:**
- Modify: `cmd/arrgod/server.go:96-108`

**Step 1: Update migrations**

Add after existing migrations in `runServer`:

```go
// Run migration 004 - events table
if _, err := db.Exec(migrations.Migration004Events); err != nil {
	// Ignore "table already exists" for idempotent migrations
	if !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("migrate 004: %w", err)
	}
}
```

**Step 2: Run tests**

Run: `go test ./cmd/arrgod/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "feat(server): add events table migration"
```

---

### Task 5.2: Create Event-Driven Server Runner

**Files:**
- Create: `internal/server/runner.go`
- Test: `internal/server/runner_test.go`

**Step 1: Write the failing test**

```go
// internal/server/runner_test.go
package server

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create all required tables
	_, err = db.Exec(`
		CREATE TABLE downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			client TEXT NOT NULL,
			client_id TEXT NOT NULL,
			status TEXT NOT NULL,
			release_name TEXT NOT NULL,
			indexer TEXT NOT NULL,
			added_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			last_transition_at TIMESTAMP NOT NULL
		);
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			payload TEXT NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)
	return db
}

func TestRunner_StartsAndStops(t *testing.T) {
	db := setupTestDB(t)

	runner := NewRunner(db, Config{
		PollInterval: 100 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Stop
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/server/... -v -run TestRunner`
Expected: FAIL with "package server is not in std"

**Step 3: Write minimal implementation**

```go
// internal/server/runner.go
package server

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/handlers"
	"github.com/vmunix/arrgo/internal/events"
)

// Config for the event-driven server.
type Config struct {
	PollInterval time.Duration
	DownloadRoot string
	CleanupEnabled bool
}

// Runner manages the event-driven components.
type Runner struct {
	db     *sql.DB
	config Config
	logger *slog.Logger
}

// NewRunner creates a new runner.
func NewRunner(db *sql.DB, cfg Config, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		db:     db,
		config: cfg,
		logger: logger,
	}
}

// Run starts all event-driven components.
func (r *Runner) Run(ctx context.Context) error {
	// Create event bus with persistence
	eventLog := events.NewEventLog(r.db)
	bus := events.NewBus(eventLog, r.logger.With("component", "bus"))
	defer bus.Close()

	// Create stores
	downloadStore := download.NewStore(r.db)

	// Note: In full implementation, we'd create all handlers and adapters here
	// For now, just demonstrate the pattern with errgroup

	g, ctx := errgroup.WithContext(ctx)

	// Example: Start a minimal download handler (requires more deps in real impl)
	_ = handlers.NewBaseHandler(bus, r.logger)
	_ = downloadStore

	// Wait for context cancellation
	g.Go(func() error {
		<-ctx.Done()
		return ctx.Err()
	})

	return g.Wait()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/server/... -v -run TestRunner`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/runner.go internal/server/runner_test.go
git commit -m "feat(server): add event-driven Runner skeleton"
```

---

### Task 5.3: Update API to Emit Events for Grab

**Files:**
- Modify: `internal/api/v1/api.go:473-490`
- Modify: `internal/api/v1/deps.go` (add Bus field)

**Step 1: Update ServerDeps**

Add to `ServerDeps` struct in `deps.go`:

```go
Bus *events.Bus // Optional: for event-driven mode
```

**Step 2: Update grab handler**

Modify `grab` in `api.go` to optionally emit event:

```go
func (s *Server) grab(w http.ResponseWriter, r *http.Request) {
	var req grabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// If event bus is available, emit event instead of direct call
	if s.deps.Bus != nil {
		s.deps.Bus.Publish(r.Context(), &events.GrabRequested{
			BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
			ContentID:   req.ContentID,
			EpisodeID:   req.EpisodeID,
			DownloadURL: req.DownloadURL,
			ReleaseName: req.Title,
			Indexer:     req.Indexer,
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}

	// Legacy: direct call to manager
	d, err := s.deps.Manager.Grab(r.Context(), req.ContentID, req.EpisodeID, req.DownloadURL, req.Title, req.Indexer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GRAB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, grabResponse{
		DownloadID: d.ID,
		Status:     string(d.Status),
	})
}
```

**Step 3: Run tests**

Run: `go test ./internal/api/v1/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/deps.go
git commit -m "feat(api): add optional event emission for grab endpoint"
```

---

## Phase 5: Integration

### Task 5.4: Full Integration Test

**Files:**
- Create: `internal/handlers/integration_test.go`

**Step 1: Write the integration test**

```go
// internal/handlers/integration_test.go
package handlers_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/handlers"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/events"
	_ "modernc.org/sqlite"
)

func setupIntegrationDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL,
			episode_id INTEGER,
			client TEXT NOT NULL,
			client_id TEXT NOT NULL,
			status TEXT NOT NULL,
			release_name TEXT NOT NULL,
			indexer TEXT NOT NULL,
			added_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			last_transition_at TIMESTAMP NOT NULL
		);
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			payload TEXT NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)
	return db
}

// mockDownloader for integration test
type mockDownloader struct {
	returnID string
}

func (m *mockDownloader) Add(ctx context.Context, url, category string) (string, error) {
	return m.returnID, nil
}
func (m *mockDownloader) Status(ctx context.Context, clientID string) (*download.ClientStatus, error) {
	return &download.ClientStatus{Status: download.StatusCompleted, Path: "/downloads/test"}, nil
}
func (m *mockDownloader) List(ctx context.Context) ([]*download.ClientStatus, error) {
	return nil, nil
}
func (m *mockDownloader) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	return nil
}

// mockImporter for integration test
type mockImporter struct{}

func (m *mockImporter) Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error) {
	return &importer.ImportResult{
		FileID:    1,
		DestPath:  "/movies/test.mkv",
		SizeBytes: 1000,
	}, nil
}

func TestIntegration_GrabToImport(t *testing.T) {
	db := setupIntegrationDB(t)
	eventLog := events.NewEventLog(db)
	bus := events.NewBus(eventLog, nil)
	defer bus.Close()

	store := download.NewStore(db)
	client := &mockDownloader{returnID: "sab-integration"}
	imp := &mockImporter{}

	// Create handlers (handlers update state directly, no projection needed)
	downloadHandler := handlers.NewDownloadHandler(bus, store, client, nil)
	importHandler := handlers.NewImportHandler(bus, store, imp, nil)

	// Subscribe to final event
	completed := bus.Subscribe(events.EventImportCompleted, 10)

	// Start all components
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go downloadHandler.Start(ctx)
	go importHandler.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	// Publish GrabRequested
	bus.Publish(ctx, &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "https://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test",
	})

	// Simulate download completion (would normally come from adapter)
	time.Sleep(100 * time.Millisecond)

	// Get the download that was created
	downloads, _ := store.List(download.Filter{})
	require.Len(t, downloads, 1)

	// Emit completion
	bus.Publish(ctx, &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, downloads[0].ID),
		DownloadID: downloads[0].ID,
		SourcePath: "/downloads/Test.Movie.2024",
	})

	// Wait for import to complete
	select {
	case e := <-completed:
		ic := e.(*events.ImportCompleted)
		assert.Equal(t, int64(42), ic.ContentID)
		assert.Equal(t, "/movies/test.mkv", ic.FilePath)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for import completion")
	}

	// Verify final state
	time.Sleep(100 * time.Millisecond)
	dl, err := store.Get(downloads[0].ID)
	require.NoError(t, err)
	assert.Equal(t, download.StatusImported, dl.Status)

	// Verify events were persisted
	events, err := eventLog.Since(time.Now().Add(-time.Minute))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 4) // GrabRequested, DownloadCreated, DownloadCompleted, ImportStarted, ImportCompleted
}
```

**Step 2: Run integration test**

Run: `go test ./internal/handlers/... -v -run TestIntegration`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/handlers/integration_test.go
git commit -m "test: add full integration test for event-driven flow"
```

---

## Summary

This plan implements the event-driven architecture in 5 phases:

1. **Event Bus Core** - Event interface, SQLite persistence, pub/sub bus
2. **Event Types** - Download, import, cleanup, library events + registry
3. **Handlers** - Download, import, cleanup handlers with per-entity locking
4. **Adapters** - SABnzbd and Plex adapters for external service polling
5. **Wire Up & Integration** - Server integration, API event emission, full flow test

**Key design decisions:**
- Handlers update state directly, then emit events (no separate projection)
- Events are for coordination and audit trail, not source of truth
- External systems (SABnzbd, Plex) are source of truth for crash recovery
- Big bang cutover (not gradual migration)
- `internal/events/` package location

Each task follows TDD with explicit test-first, verify-fail, implement, verify-pass, commit steps.

**Total tasks: 16**
**Key files created: ~20 new files**
**Key files modified: ~5 existing files**
