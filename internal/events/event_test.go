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
