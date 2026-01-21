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
