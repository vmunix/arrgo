// internal/events/import_test.go
package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportStarted_JSON(t *testing.T) {
	e := &ImportStarted{
		BaseEvent:  NewBaseEvent(EventImportStarted, EntityDownload, 123),
		DownloadID: 123,
		SourcePath: "/downloads/complete/Movie.2024.1080p",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded ImportStarted
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, EventImportStarted, decoded.EventType())
	assert.Equal(t, EntityDownload, decoded.EntityType())
	assert.Equal(t, int64(123), decoded.DownloadID)
	assert.Equal(t, "/downloads/complete/Movie.2024.1080p", decoded.SourcePath)
}

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

func TestImportCompleted_WithEpisode(t *testing.T) {
	episodeID := int64(99)
	e := &ImportCompleted{
		BaseEvent:  NewBaseEvent(EventImportCompleted, EntityDownload, 123),
		DownloadID: 123,
		ContentID:  42,
		EpisodeID:  &episodeID,
		FilePath:   "/tv/Show/Season 1/Show.S01E01.1080p.mkv",
		FileSize:   4294967296,
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded ImportCompleted
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.NotNil(t, decoded.EpisodeID)
	assert.Equal(t, int64(99), *decoded.EpisodeID)
}

func TestImportFailed_JSON(t *testing.T) {
	e := &ImportFailed{
		BaseEvent:  NewBaseEvent(EventImportFailed, EntityDownload, 123),
		DownloadID: 123,
		Reason:     "destination disk full",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded ImportFailed
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, EventImportFailed, decoded.EventType())
	assert.Equal(t, int64(123), decoded.DownloadID)
	assert.Equal(t, "destination disk full", decoded.Reason)
}

func TestCleanupStarted_JSON(t *testing.T) {
	e := &CleanupStarted{
		BaseEvent:  NewBaseEvent(EventCleanupStarted, EntityDownload, 123),
		DownloadID: 123,
		SourcePath: "/downloads/complete/Movie.2024.1080p",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)

	var decoded CleanupStarted
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, EventCleanupStarted, decoded.EventType())
	assert.Equal(t, int64(123), decoded.DownloadID)
	assert.Equal(t, "/downloads/complete/Movie.2024.1080p", decoded.SourcePath)
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
