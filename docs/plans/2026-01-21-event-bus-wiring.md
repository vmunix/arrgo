# Event Bus Wiring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire the event-driven architecture components into the main server, replacing the legacy polling approach with event-based coordination.

**Architecture:** The event bus, handlers, and adapters are already implemented but sitting idle. This plan wires them into `cmd/arrgod/server.go`, passes the bus to the API layer, updates the compat layer to emit events instead of calling Manager directly, and adds event log pruning.

**Tech Stack:** Go, SQLite, errgroup, existing events/handlers/adapters packages

---

## Gap Analysis

| Design Spec Requires | Current Status | This Plan |
|---------------------|----------------|-----------|
| Runner starts handlers with errgroup | Runner is skeleton | Task 1: Wire handlers |
| Runner starts adapters with errgroup | Runner is skeleton | Task 2: Wire adapters |
| API passes Bus to ServerDeps | Bus field exists but unused | Task 3: Pass bus |
| Compat publishes GrabRequested | Uses Manager directly | Task 4: Add bus to compat |
| Event pruning scheduled | Not done | Task 5: Add pruning |
| Old poller removed | Still running | Task 6: Remove poller |

---

## Task 1: Wire Handlers into Runner

**Files:**
- Modify: `internal/server/runner.go`
- Test: `internal/server/runner_test.go`

**Step 1: Update Runner struct to hold dependencies**

Add fields to `internal/server/runner.go`:

```go
// Runner manages the event-driven components.
type Runner struct {
	db     *sql.DB
	config Config
	logger *slog.Logger

	// Dependencies
	downloader download.Downloader
	importer   handlers.FileImporter
}

// NewRunner creates a new runner.
func NewRunner(db *sql.DB, cfg Config, logger *slog.Logger, downloader download.Downloader, importer handlers.FileImporter) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		db:         db,
		config:     cfg,
		logger:     logger,
		downloader: downloader,
		importer:   importer,
	}
}
```

**Step 2: Wire handlers in Run method**

Replace skeleton `Run` method:

```go
// Run starts all event-driven components.
// It blocks until the context is canceled or an error occurs.
func (r *Runner) Run(ctx context.Context) error {
	// Create event bus with persistence
	eventLog := events.NewEventLog(r.db)
	bus := events.NewBus(eventLog, r.logger.With("component", "bus"))
	defer bus.Close()

	// Create stores
	downloadStore := download.NewStore(r.db)

	// Create handlers
	downloadHandler := handlers.NewDownloadHandler(bus, downloadStore, r.downloader, r.logger.With("handler", "download"))
	importHandler := handlers.NewImportHandler(bus, downloadStore, r.importer, r.logger.With("handler", "import"))
	cleanupHandler := handlers.NewCleanupHandler(bus, downloadStore, handlers.CleanupConfig{
		DownloadRoot: r.config.DownloadRoot,
		Enabled:      r.config.CleanupEnabled,
	}, r.logger.With("handler", "cleanup"))

	// Use errgroup to manage component lifecycle
	g, ctx := errgroup.WithContext(ctx)

	// Start handlers
	g.Go(func() error {
		r.logger.Info("starting download handler")
		return downloadHandler.Start(ctx)
	})
	g.Go(func() error {
		r.logger.Info("starting import handler")
		return importHandler.Start(ctx)
	})
	g.Go(func() error {
		r.logger.Info("starting cleanup handler")
		return cleanupHandler.Start(ctx)
	})

	return g.Wait()
}
```

**Step 3: Update imports**

Add to imports in `runner.go`:

```go
import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/handlers"
	"golang.org/x/sync/errgroup"
)
```

**Step 4: Update tests**

Update `internal/server/runner_test.go` to pass new dependencies:

```go
func TestRunner_StartsAndStops(t *testing.T) {
	db := setupTestDB(t)

	// Create mock dependencies
	mockDownloader := &mockDownloader{}
	mockImporter := &mockImporter{}

	runner := NewRunner(db, Config{
		PollInterval:   100 * time.Millisecond,
		DownloadRoot:   "/tmp/downloads",
		CleanupEnabled: false,
	}, nil, mockDownloader, mockImporter)

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
		if err != nil && err != context.Canceled {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runner to stop")
	}
}

// Mock implementations
type mockDownloader struct{}

func (m *mockDownloader) Add(ctx context.Context, url, category string) (string, error) {
	return "mock-id", nil
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

type mockImporter struct{}

func (m *mockImporter) Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error) {
	return &importer.ImportResult{DestPath: "/movies/test.mkv", SizeBytes: 1000}, nil
}
```

**Step 5: Run tests**

Run: `go test ./internal/server/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/server/runner.go internal/server/runner_test.go
git commit -m "feat(server): wire handlers into Runner"
```

---

## Task 2: Wire Adapters into Runner

**Files:**
- Modify: `internal/server/runner.go`
- Test: `internal/server/runner_test.go`

**Step 1: Add Plex checker to Runner dependencies**

Update struct and constructor:

```go
// Runner manages the event-driven components.
type Runner struct {
	db     *sql.DB
	config Config
	logger *slog.Logger

	// Dependencies
	downloader  download.Downloader
	importer    handlers.FileImporter
	plexChecker plex.Checker // Can be nil if Plex not configured
}

// NewRunner creates a new runner.
func NewRunner(db *sql.DB, cfg Config, logger *slog.Logger, downloader download.Downloader, importer handlers.FileImporter, plexChecker plex.Checker) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		db:          db,
		config:      cfg,
		logger:      logger,
		downloader:  downloader,
		importer:    importer,
		plexChecker: plexChecker,
	}
}
```

**Step 2: Wire adapters in Run method**

Add after handler creation:

```go
	// Create adapters
	sabnzbdAdapter := sabnzbd.New(bus, r.downloader, downloadStore, r.config.PollInterval, r.logger.With("adapter", "sabnzbd"))

	// Start adapters
	g.Go(func() error {
		r.logger.Info("starting sabnzbd adapter", "interval", r.config.PollInterval)
		return sabnzbdAdapter.Start(ctx)
	})

	// Only start Plex adapter if configured
	if r.plexChecker != nil {
		plexAdapter := plex.New(bus, r.plexChecker, r.config.PollInterval, r.logger.With("adapter", "plex"))
		g.Go(func() error {
			r.logger.Info("starting plex adapter", "interval", r.config.PollInterval)
			return plexAdapter.Start(ctx)
		})
	}
```

**Step 3: Update imports**

Add to imports:

```go
	"github.com/vmunix/arrgo/internal/adapters/plex"
	"github.com/vmunix/arrgo/internal/adapters/sabnzbd"
```

**Step 4: Update tests**

Add nil for plexChecker in test:

```go
runner := NewRunner(db, Config{...}, nil, mockDownloader, mockImporter, nil)
```

**Step 5: Run tests**

Run: `go test ./internal/server/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/server/runner.go internal/server/runner_test.go
git commit -m "feat(server): wire adapters into Runner"
```

---

## Task 3: Expose Bus from Runner for API

**Files:**
- Modify: `internal/server/runner.go`
- Test: `internal/server/runner_test.go`

**Step 1: Add Bus() method to Runner**

The API needs access to the bus. Add a method that returns it after Run creates it. However, the bus is created inside Run(). We need to restructure slightly - create the bus in a Start() method and expose it.

Update `runner.go`:

```go
// Runner manages the event-driven components.
type Runner struct {
	db     *sql.DB
	config Config
	logger *slog.Logger

	// Dependencies
	downloader  download.Downloader
	importer    handlers.FileImporter
	plexChecker plex.Checker

	// Runtime state
	bus      *events.Bus
	eventLog *events.EventLog
}

// Start initializes the runner and returns the event bus.
// Call Run() after Start() to begin processing.
func (r *Runner) Start() *events.Bus {
	r.eventLog = events.NewEventLog(r.db)
	r.bus = events.NewBus(r.eventLog, r.logger.With("component", "bus"))
	return r.bus
}

// Run starts all event-driven components.
// Must call Start() before Run().
func (r *Runner) Run(ctx context.Context) error {
	if r.bus == nil {
		return errors.New("must call Start() before Run()")
	}
	defer r.bus.Close()

	// Create stores
	downloadStore := download.NewStore(r.db)

	// Create handlers
	downloadHandler := handlers.NewDownloadHandler(r.bus, downloadStore, r.downloader, r.logger.With("handler", "download"))
	importHandler := handlers.NewImportHandler(r.bus, downloadStore, r.importer, r.logger.With("handler", "import"))
	cleanupHandler := handlers.NewCleanupHandler(r.bus, downloadStore, handlers.CleanupConfig{
		DownloadRoot: r.config.DownloadRoot,
		Enabled:      r.config.CleanupEnabled,
	}, r.logger.With("handler", "cleanup"))

	// Create adapters
	sabnzbdAdapter := sabnzbd.New(r.bus, r.downloader, downloadStore, r.config.PollInterval, r.logger.With("adapter", "sabnzbd"))

	// Use errgroup to manage component lifecycle
	g, ctx := errgroup.WithContext(ctx)

	// Start handlers
	g.Go(func() error {
		r.logger.Info("starting download handler")
		return downloadHandler.Start(ctx)
	})
	g.Go(func() error {
		r.logger.Info("starting import handler")
		return importHandler.Start(ctx)
	})
	g.Go(func() error {
		r.logger.Info("starting cleanup handler")
		return cleanupHandler.Start(ctx)
	})

	// Start adapters
	g.Go(func() error {
		r.logger.Info("starting sabnzbd adapter", "interval", r.config.PollInterval)
		return sabnzbdAdapter.Start(ctx)
	})

	if r.plexChecker != nil {
		plexAdapter := plex.New(r.bus, r.plexChecker, r.config.PollInterval, r.logger.With("adapter", "plex"))
		g.Go(func() error {
			r.logger.Info("starting plex adapter", "interval", r.config.PollInterval)
			return plexAdapter.Start(ctx)
		})
	}

	return g.Wait()
}
```

**Step 2: Add errors import**

```go
import (
	"context"
	"database/sql"
	"errors"
	// ...
)
```

**Step 3: Update tests**

```go
func TestRunner_StartsAndStops(t *testing.T) {
	db := setupTestDB(t)

	mockDownloader := &mockDownloader{}
	mockImporter := &mockImporter{}

	runner := NewRunner(db, Config{
		PollInterval:   100 * time.Millisecond,
		DownloadRoot:   "/tmp/downloads",
		CleanupEnabled: false,
	}, nil, mockDownloader, mockImporter, nil)

	// Start returns the bus
	bus := runner.Start()
	require.NotNil(t, bus)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runner to stop")
	}
}

func TestRunner_RunWithoutStart(t *testing.T) {
	db := setupTestDB(t)
	runner := NewRunner(db, Config{}, nil, &mockDownloader{}, &mockImporter{}, nil)

	err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must call Start()")
}
```

**Step 4: Run tests**

Run: `go test ./internal/server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/runner.go internal/server/runner_test.go
git commit -m "feat(server): add Start() method to expose Bus for API"
```

---

## Task 4: Integrate Runner into Main Server

**Files:**
- Modify: `cmd/arrgod/server.go`

**Step 1: Import server package**

Add to imports:

```go
import (
	// ... existing imports ...
	"github.com/vmunix/arrgo/internal/server"
)
```

**Step 2: Create PlexChecker adapter**

The Plex adapter needs a `plex.Checker` interface, but we have `*importer.PlexClient`. Add an adapter near `runServer`:

```go
// plexCheckerAdapter adapts PlexClient to the plex.Checker interface.
type plexCheckerAdapter struct {
	client *importer.PlexClient
	lib    *library.Store
}

func (a *plexCheckerAdapter) HasContentByID(ctx context.Context, contentID int64) (bool, string, error) {
	content, err := a.lib.GetContent(contentID)
	if err != nil {
		return false, "", err
	}
	found, err := a.client.HasMovie(ctx, content.Title, content.Year)
	if err != nil {
		return false, "", err
	}
	// PlexClient.HasMovie doesn't return the plex key, return empty for now
	return found, "", nil
}
```

**Step 3: Create and start Runner**

Replace the old poller startup (around line 189-191) with Runner startup:

```go
	// === Event-Driven Runner ===
	var runner *server.Runner
	var eventBus *events.Bus

	if sabClient != nil {
		// Create plex checker adapter if plex is configured
		var plexChecker plex.Checker
		if plexClient != nil {
			plexChecker = &plexCheckerAdapter{client: plexClient, lib: libraryStore}
		}

		runner = server.NewRunner(db, server.Config{
			PollInterval:   30 * time.Second,
			DownloadRoot:   sabDownloadRoot(cfg),
			CleanupEnabled: cfg.Importer.ShouldCleanupSource(),
		}, logger, sabClient, imp, plexChecker)

		eventBus = runner.Start()
		go func() {
			if err := runner.Run(ctx); err != nil && err != context.Canceled {
				logger.Error("runner error", "error", err)
			}
		}()
	}
```

**Step 4: Add helper function for download root**

```go
func sabDownloadRoot(cfg *config.Config) string {
	if cfg.Downloaders.SABnzbd != nil && cfg.Downloaders.SABnzbd.LocalPath != "" {
		return cfg.Downloaders.SABnzbd.LocalPath
	}
	return ""
}
```

**Step 5: Pass Bus to API**

Update API creation (around line 203-218):

```go
	// Native API v1
	apiV1, err := v1.NewWithDeps(v1.ServerDeps{
		Library:   libraryStore,
		Downloads: downloadStore,
		History:   historyStore,
		Searcher:  searcher,
		Manager:   downloadManager,
		Plex:      plexClient,
		Importer:  imp,
		Bus:       eventBus, // Add this line
	}, v1.Config{
		MovieRoot:       cfg.Libraries.Movies.Root,
		SeriesRoot:      cfg.Libraries.Series.Root,
		QualityProfiles: profiles,
	})
```

**Step 6: Add imports**

```go
import (
	// ... existing ...
	"github.com/vmunix/arrgo/internal/adapters/plex"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/server"
)
```

**Step 7: Run the server and verify**

Run: `go build ./cmd/arrgod && ./arrgod`
Expected: Server starts with log messages for handlers and adapters

**Step 8: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "feat(server): integrate event-driven Runner into main server"
```

---

## Task 5: Add Event Bus to Compat Layer

**Files:**
- Modify: `internal/api/compat/compat.go`
- Modify: `cmd/arrgod/server.go`

**Step 1: Add Bus field to compat Server**

In `internal/api/compat/compat.go`, add to Server struct (around line 60):

```go
type Server struct {
	config   Config
	library  *library.Store
	download *download.Store
	logger   *slog.Logger

	// Optional dependencies
	searcher *search.Searcher
	manager  *download.Manager
	tmdb     *tmdb.Client
	bus      *events.Bus // Add this
}
```

**Step 2: Add SetBus method**

Add after SetTMDB (around line 172):

```go
// SetBus configures the event bus for event-driven grabs (optional).
func (s *Server) SetBus(bus *events.Bus) {
	s.bus = bus
}
```

**Step 3: Update searchAndGrab to use bus when available**

Modify `searchAndGrab` (around line 971):

```go
// searchAndGrab performs a background search and grabs the best result.
func (s *Server) searchAndGrab(contentID int64, title string, year int, profile string) {
	if s.searcher == nil {
		s.logger.Warn("no searcher configured, cannot search")
		return
	}

	result, err := s.searcher.Search(context.Background(), search.Query{Title: title, Year: year, Type: "movie"}, profile)
	if err != nil || len(result.Releases) == 0 {
		s.logger.Warn("search failed or no results", "title", title, "year", year)
		return
	}

	// Grab the best match (first result after scoring/sorting)
	best := result.Releases[0]

	// Use event bus if available, otherwise fall back to manager
	if s.bus != nil {
		if err := s.bus.Publish(context.Background(), &events.GrabRequested{
			BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
			ContentID:   contentID,
			DownloadURL: best.DownloadURL,
			ReleaseName: best.Title,
			Indexer:     best.Indexer,
		}); err != nil {
			s.logger.Error("failed to publish GrabRequested", "error", err)
		}
		return
	}

	// Legacy: direct manager call
	if s.manager != nil {
		_, _ = s.manager.Grab(context.Background(), contentID, nil, best.DownloadURL, best.Title, best.Indexer)
	}
}
```

**Step 4: Update searchAndGrabSeries similarly**

Modify `searchAndGrabSeries` (around line 993):

```go
// searchAndGrabSeries performs a background search for series seasons.
func (s *Server) searchAndGrabSeries(contentID int64, title string, profile string, seasons []int) {
	if s.searcher == nil {
		s.logger.Warn("no searcher configured, cannot search")
		return
	}

	for _, season := range seasons {
		result, err := s.searcher.Search(context.Background(), search.Query{
			Title:  title,
			Type:   "series",
			Season: season,
		}, profile)
		if err != nil || len(result.Releases) == 0 {
			s.logger.Warn("search failed or no results", "title", title, "season", season)
			continue
		}

		// Grab the best match for this season
		best := result.Releases[0]

		// Use event bus if available, otherwise fall back to manager
		if s.bus != nil {
			if err := s.bus.Publish(context.Background(), &events.GrabRequested{
				BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
				ContentID:   contentID,
				DownloadURL: best.DownloadURL,
				ReleaseName: best.Title,
				Indexer:     best.Indexer,
			}); err != nil {
				s.logger.Error("failed to publish GrabRequested", "error", err)
			}
			continue
		}

		// Legacy: direct manager call
		if s.manager != nil {
			_, _ = s.manager.Grab(context.Background(), contentID, nil, best.DownloadURL, best.Title, best.Indexer)
		}
	}
}
```

**Step 5: Add events import**

Add to imports in `compat.go`:

```go
import (
	// ... existing ...
	"github.com/vmunix/arrgo/internal/events"
)
```

**Step 6: Wire bus in server.go**

Update compat setup in `cmd/arrgod/server.go` (around line 240):

```go
		apiCompat.SetSearcher(searcher)
		apiCompat.SetManager(downloadManager)
		if eventBus != nil {
			apiCompat.SetBus(eventBus)
		}
```

**Step 7: Run tests**

Run: `go test ./internal/api/compat/... -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/api/compat/compat.go cmd/arrgod/server.go
git commit -m "feat(compat): add event bus support for Overseerr grabs"
```

---

## Task 6: Add Event Log Pruning

**Files:**
- Modify: `internal/server/runner.go`

**Step 1: Add pruning goroutine to Run**

Add after adapter startup in `Run()`:

```go
	// Event log pruning (every 24 hours, keep 90 days)
	g.Go(func() error {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				pruned, err := r.eventLog.Prune(90 * 24 * time.Hour)
				if err != nil {
					r.logger.Error("failed to prune event log", "error", err)
				} else if pruned > 0 {
					r.logger.Info("pruned old events", "count", pruned)
				}
			}
		}
	})
```

**Step 2: Run tests**

Run: `go test ./internal/server/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/server/runner.go
git commit -m "feat(server): add 90-day event log pruning"
```

---

## Task 7: Remove Legacy Poller

**Files:**
- Modify: `cmd/arrgod/server.go`

**Step 1: Remove runPoller call**

Delete the old poller startup (the line that looks like):

```go
go runPoller(ctx, downloadManager, imp, downloadStore, cfg.Downloaders.SABnzbd, &cfg.Importer, plexClient, libraryStore, logger.With("component", "poller"))
```

**Step 2: Remove runPoller and poll functions**

Delete the following functions from `server.go`:
- `runPoller` (lines ~328-343)
- `poll` (lines ~345-395)
- `translatePath` (lines ~397-406)
- `processImportedDownloads` (lines ~408-547)
- `cleanupPathValidation` struct (lines ~549-556)
- `validateCleanupPath` (lines ~558-665)

**Step 3: Clean up unused imports**

Remove any imports that are no longer needed (the compiler will tell you).

**Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Manual verification**

1. Start server: `./arrgod`
2. Verify handlers start: Check logs for "starting download handler", etc.
3. Test a grab via API or CLI
4. Verify download appears in queue
5. When download completes, verify import happens via events

**Step 6: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "refactor(server): remove legacy poller in favor of event-driven architecture"
```

---

## Task 8: Integration Test

**Files:**
- Create: `cmd/arrgod/server_integration_test.go`

**Step 1: Write integration test**

```go
//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/server"
	_ "modernc.org/sqlite"
)

func TestServer_EventDrivenGrab(t *testing.T) {
	// Setup in-memory database
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	// Create tables
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
		);
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			payload TEXT NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	require.NoError(t, err)

	// Create mocks
	mockDL := &mockIntegrationDownloader{returnID: "test-123"}
	mockImp := &mockIntegrationImporter{db: db}

	// Create runner
	runner := server.NewRunner(db, server.Config{
		PollInterval:   100 * time.Millisecond,
		DownloadRoot:   "/tmp/test",
		CleanupEnabled: false,
	}, nil, mockDL, mockImp, nil)

	bus := runner.Start()
	require.NotNil(t, bus)

	// Subscribe to track events
	created := bus.Subscribe(events.EventDownloadCreated, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runner.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	// Publish GrabRequested
	err = bus.Publish(ctx, &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   42,
		DownloadURL: "http://example.com/test.nzb",
		ReleaseName: "Test.Movie.2024",
		Indexer:     "test-indexer",
	})
	require.NoError(t, err)

	// Wait for DownloadCreated
	select {
	case e := <-created:
		dc := e.(*events.DownloadCreated)
		assert.Equal(t, int64(42), dc.ContentID)
		assert.Equal(t, "test-123", dc.ClientID)
		assert.Equal(t, "Test.Movie.2024", dc.ReleaseName)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for DownloadCreated event")
	}

	// Verify download in database
	store := download.NewStore(db)
	downloads, err := store.List(download.Filter{})
	require.NoError(t, err)
	require.Len(t, downloads, 1)
	assert.Equal(t, "Test.Movie.2024", downloads[0].ReleaseName)
}

type mockIntegrationDownloader struct {
	returnID string
}

func (m *mockIntegrationDownloader) Add(ctx context.Context, url, category string) (string, error) {
	return m.returnID, nil
}
func (m *mockIntegrationDownloader) Status(ctx context.Context, clientID string) (*download.ClientStatus, error) {
	return nil, nil
}
func (m *mockIntegrationDownloader) List(ctx context.Context) ([]*download.ClientStatus, error) {
	return nil, nil
}
func (m *mockIntegrationDownloader) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	return nil
}

type mockIntegrationImporter struct {
	db *sql.DB
}

func (m *mockIntegrationImporter) Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error) {
	return &importer.ImportResult{DestPath: "/movies/test.mkv", SizeBytes: 1000}, nil
}
```

**Step 2: Run integration test**

Run: `go test ./cmd/arrgod/... -v -tags=integration -run TestServer_EventDrivenGrab`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/arrgod/server_integration_test.go
git commit -m "test: add server integration test for event-driven grab"
```

---

## Summary

This plan wires the existing event-driven components into the main server:

1. **Task 1-2**: Wire handlers and adapters into Runner
2. **Task 3**: Expose Bus from Runner for API layer
3. **Task 4**: Integrate Runner into main server, pass Bus to API
4. **Task 5**: Add event bus to compat layer for Overseerr
5. **Task 6**: Add 90-day event log pruning
6. **Task 7**: Remove legacy poller
7. **Task 8**: Integration test

After completion:
- Grabs via API return 202 Accepted and emit GrabRequested
- Grabs via Overseerr/compat emit GrabRequested
- SABnzbd adapter polls and emits completion events
- Import handler processes DownloadCompleted events
- Cleanup handler processes PlexItemDetected events
- Events are persisted and pruned after 90 days
- Old polling code is removed

---

Plan complete and saved to `docs/plans/2026-01-21-event-bus-wiring.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?
