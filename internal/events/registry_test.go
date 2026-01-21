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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}

func TestRegistry_UnmarshalInvalidJSON(t *testing.T) {
	registry := NewRegistry()
	registry.Register(EventGrabRequested, func() Event { return &GrabRequested{} })

	raw := RawEvent{
		EventType: EventGrabRequested,
		Payload:   `{invalid json`,
	}

	_, err := registry.Unmarshal(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal event payload")
}

func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()

	// Verify all event types are registered
	eventTypes := []string{
		EventGrabRequested,
		EventDownloadCreated,
		EventDownloadProgressed,
		EventDownloadCompleted,
		EventDownloadFailed,
		EventImportStarted,
		EventImportCompleted,
		EventImportFailed,
		EventCleanupStarted,
		EventCleanupCompleted,
		EventContentAdded,
		EventContentStatusChanged,
		EventPlexItemDetected,
	}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			raw := RawEvent{
				EventType: eventType,
				Payload:   `{"type":"` + eventType + `","entity_type":"download","entity_id":1,"occurred_at":"2024-01-01T00:00:00Z"}`,
			}
			event, err := registry.Unmarshal(raw)
			require.NoError(t, err, "Failed to unmarshal %s", eventType)
			assert.Equal(t, eventType, event.EventType())
		})
	}
}

func TestRegistry_UnmarshalDownloadCompleted(t *testing.T) {
	registry := DefaultRegistry()

	raw := RawEvent{
		EventType: EventDownloadCompleted,
		Payload:   `{"type":"download.completed","entity_type":"download","entity_id":99,"occurred_at":"2024-01-01T12:00:00Z","download_id":123,"source_path":"/downloads/complete/Movie.2024"}`,
	}

	event, err := registry.Unmarshal(raw)
	require.NoError(t, err)

	completed, ok := event.(*DownloadCompleted)
	require.True(t, ok)
	assert.Equal(t, int64(123), completed.DownloadID)
	assert.Equal(t, "/downloads/complete/Movie.2024", completed.SourcePath)
	assert.Equal(t, int64(99), completed.EntityID())
}

func TestRegistry_UnmarshalContentAdded(t *testing.T) {
	registry := DefaultRegistry()

	raw := RawEvent{
		EventType: EventContentAdded,
		Payload:   `{"type":"content.added","entity_type":"content","entity_id":50,"occurred_at":"2024-01-01T00:00:00Z","content_id":50,"content_type":"movie","title":"Test Movie","year":2024,"quality_profile":"hd"}`,
	}

	event, err := registry.Unmarshal(raw)
	require.NoError(t, err)

	content, ok := event.(*ContentAdded)
	require.True(t, ok)
	assert.Equal(t, int64(50), content.ContentID)
	assert.Equal(t, "movie", content.ContentType)
	assert.Equal(t, "Test Movie", content.Title)
	assert.Equal(t, 2024, content.Year)
	assert.Equal(t, "hd", content.QualityProfile)
}
