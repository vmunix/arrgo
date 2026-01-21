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
