package server

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/importer"
	_ "modernc.org/sqlite"
)

// Mock implementations

type mockDownloader struct{}

func (m *mockDownloader) Add(_ context.Context, _, _ string) (string, error) {
	return "mock-id", nil
}

func (m *mockDownloader) Status(_ context.Context, _ string) (*download.ClientStatus, error) {
	return nil, nil
}

func (m *mockDownloader) List(_ context.Context) ([]*download.ClientStatus, error) {
	return nil, nil
}

func (m *mockDownloader) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}

type mockImporter struct{}

func (m *mockImporter) Import(_ context.Context, _ int64, _ string) (*importer.ImportResult, error) {
	return &importer.ImportResult{DestPath: "/movies/test.mkv", SizeBytes: 1000}, nil
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Create the events table for EventLog
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

	// Create the downloads table for download store
	_, err = db.Exec(`
		CREATE TABLE content (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			year INTEGER,
			status TEXT NOT NULL DEFAULT 'wanted',
			quality_profile TEXT NOT NULL DEFAULT 'hd',
			root_path TEXT NOT NULL
		);

		CREATE TABLE downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_id INTEGER NOT NULL REFERENCES content(id),
			episode_id INTEGER,
			client TEXT NOT NULL,
			client_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			release_name TEXT,
			indexer TEXT,
			added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP,
			last_transition_at TIMESTAMP
		);
	`)
	require.NoError(t, err)

	return db
}

func TestRunner_StartsAndStops(t *testing.T) {
	db := setupTestDB(t)

	// Create mock dependencies
	mockDownloader := &mockDownloader{}
	mockImporter := &mockImporter{}

	runner := NewRunner(db, Config{
		PollInterval:   100 * time.Millisecond,
		DownloadRoot:   "/tmp/downloads",
		CleanupEnabled: false,
	}, nil, mockDownloader, mockImporter, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	// Give handlers time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel and wait for clean shutdown
	cancel()

	select {
	case err := <-done:
		// context.Canceled is expected
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runner to stop")
	}
}

func TestNewRunner_DefaultLogger(t *testing.T) {
	db := setupTestDB(t)

	// Should not panic with nil logger
	runner := NewRunner(db, Config{}, nil, &mockDownloader{}, &mockImporter{}, nil)
	require.NotNil(t, runner)
	require.NotNil(t, runner.logger)
}

func TestRunner_ConfigFields(t *testing.T) {
	db := setupTestDB(t)

	cfg := Config{
		PollInterval:   5 * time.Second,
		DownloadRoot:   "/downloads",
		CleanupEnabled: true,
	}

	runner := NewRunner(db, cfg, nil, &mockDownloader{}, &mockImporter{}, nil)

	require.Equal(t, cfg.PollInterval, runner.config.PollInterval)
	require.Equal(t, cfg.DownloadRoot, runner.config.DownloadRoot)
	require.True(t, runner.config.CleanupEnabled)
}
