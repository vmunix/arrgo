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
