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
