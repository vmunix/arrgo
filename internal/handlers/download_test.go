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
	go func() {
		_ = handler.Start(ctx)
	}()

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
