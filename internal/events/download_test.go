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

	assert.InDelta(t, 45.5, decoded.Progress, 0.001)
	assert.Equal(t, int64(10485760), decoded.Speed)
	assert.Equal(t, 300, decoded.ETA)
}
