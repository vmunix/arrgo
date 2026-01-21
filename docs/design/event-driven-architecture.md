# Event-Driven Architecture Design Spec

> **Purpose**: Refactor arrgo's state management to an event-driven architecture for robust coordination of external services, real-time UI updates, and future extensibility.

## Problem Statement

### Current Issues

1. **Race Conditions**: Poller and API can both trigger imports concurrently on the same download
2. **Contract Mismatches**: Poller transitions to `importing` before calling importer, but importer expects `completed`
3. **Non-Atomic Updates**: Library and download state updated in separate transactions
4. **No Real-Time Updates**: TUI would require polling; no subscription mechanism exists
5. **Tight Coupling**: Adding new download clients (BitTorrent) requires touching multiple files
6. **Debugging Difficulty**: No audit trail of state changes

### Future Requirements

- Live-updating TUI (needs event subscriptions)
- BitTorrent client support (qBittorrent adapter)
- Potential native usenet downloader
- Quality upgrade workflows
- Retry/fallback logic

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Event Bus                                  │
│                   (channels + SQLite persistence)                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Publishers:              Subscribers:                               │
│  - Adapters (SABnzbd)     - Handlers (Import, Cleanup, Library)     │
│  - API endpoints          - Projections (State derivation)          │
│  - Handlers (emit next)   - TUI/CLI (real-time updates)             │
│                           - Webhooks (future)                        │
└─────────────────────────────────────────────────────────────────────┘
         │                           │                      │
         ▼                           ▼                      ▼
┌─────────────────┐        ┌─────────────────┐    ┌─────────────────┐
│    Adapters     │        │    Handlers     │    │   Subscribers   │
│                 │        │                 │    │                 │
│ SABnzbd Adapter │        │ Download Hdlr   │    │ TUI (future)    │
│ Plex Adapter    │        │ Import Handler  │    │ SSE endpoint    │
│ qBit (future)   │        │ Cleanup Handler │    │ Webhooks        │
└─────────────────┘        │ Library Handler │    └─────────────────┘
                           └─────────────────┘
```

## Scope: What Changes vs What Stays

Not everything becomes event-driven. The key distinction:

- **Event-Driven**: Long-running workflows with state changes (download lifecycle, import, cleanup)
- **Request-Response**: Synchronous queries that return immediate results (search, parse, lookup)

### Components That Stay Request-Response (Unchanged)

| Component | Location | Reason |
|-----------|----------|--------|
| **Release Parser** | `pkg/release/` | Pure function: string → parsed info. No state. |
| **Searcher** | `internal/search/` | Query → immediate results. No state changes. |
| **Scorer** | `internal/search/scorer.go` | Pure scoring logic. No side effects. |
| **IndexerPool** | `internal/search/indexer.go` | Aggregates indexer responses. Stateless. |
| **Newznab Client** | `pkg/newznab/` | HTTP client for indexers. Stateless. |
| **CLI Commands** | `cmd/arrgo/` | Thin client calling API. No changes needed. |
| **Compat Layer** | `internal/api/compat/` | Translates Radarr/Sonarr API. Emits events at grab boundary. |

### The Boundary: Search → Grab

```
┌─────────────────────────────────────────────────────────────────┐
│                 REQUEST-RESPONSE (synchronous)                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  User ──→ POST /search ──→ Searcher ──→ IndexerPool ──→ Results │
│                               │                                  │
│                               ↓                                  │
│                        release.Parse()                           │
│                        scorer.Score()                            │
│                               │                                  │
│                               ↓                                  │
│                        Return JSON results                       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                               │
                               │ User selects result, clicks "grab"
                               ↓
┌─────────────────────────────────────────────────────────────────┐
│                   EVENT-DRIVEN (asynchronous)                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  POST /grab ──→ Publish(GrabRequested) ──→ Return 202 Accepted  │
│                         │                                        │
│                         ↓                                        │
│               DownloadHandler processes event                    │
│                         │                                        │
│                         ↓                                        │
│               DownloadCreated → DownloadProgressed → ...         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Components That Become Event-Driven

| Component | Current Location | New Location | Change |
|-----------|-----------------|--------------|--------|
| **Download Manager** | `internal/download/manager.go` | `internal/handlers/download.go` | Becomes event handler |
| **Poller (completed)** | `cmd/arrgod/server.go` | `internal/adapters/sabnzbd/` | Becomes adapter emitting events |
| **Poller (import)** | `cmd/arrgod/server.go` | `internal/handlers/import.go` | Becomes event handler |
| **Poller (cleanup)** | `cmd/arrgod/server.go` | `internal/handlers/cleanup.go` | Becomes event handler |
| **Plex verification** | `cmd/arrgod/server.go` | `internal/adapters/plex/` | Becomes adapter emitting events |
| **Importer** | `internal/importer/` | `internal/importer/` | Called by ImportHandler, no longer checks state |

### Optional: Audit Events

For logging/metrics (not flow control), we can optionally emit:

```go
// Emitted after search completes - useful for analytics
type SearchPerformed struct {
    BaseEvent
    Query       string   `json:"query"`
    Type        string   `json:"type"`         // "movie" or "series"
    Profile     string   `json:"profile"`
    ResultCount int      `json:"result_count"`
    Indexers    []string `json:"indexers"`
    DurationMs  int      `json:"duration_ms"`
}
```

These are fire-and-forget, don't affect flow, and can be added later.

## Core Components

### 1. Event Bus (`pkg/events/bus.go`)

Central pub/sub mechanism with persistence.

```go
package events

import (
    "context"
    "sync"
    "time"
)

// Event is the base interface all events implement
type Event interface {
    EventType() string
    EntityType() string  // "download", "content", "episode"
    EntityID() int64
    OccurredAt() time.Time
}

// BaseEvent provides common fields
type BaseEvent struct {
    Type      string    `json:"type"`
    Entity    string    `json:"entity_type"`
    ID        int64     `json:"entity_id"`
    Timestamp time.Time `json:"occurred_at"`
}

func (e BaseEvent) EventType() string    { return e.Type }
func (e BaseEvent) EntityType() string   { return e.Entity }
func (e BaseEvent) EntityID() int64      { return e.ID }
func (e BaseEvent) OccurredAt() time.Time { return e.Timestamp }

// Bus is the central event bus
type Bus struct {
    mu          sync.RWMutex
    subscribers map[string][]chan Event  // eventType -> channels
    allSubs     []chan Event             // subscribers to all events
    log         *EventLog                // SQLite persistence
    logger      *slog.Logger
}

// Publish sends an event to all subscribers and persists it
func (b *Bus) Publish(ctx context.Context, e Event) error

// Subscribe returns a channel for events of a specific type
func (b *Bus) Subscribe(eventType string, bufferSize int) <-chan Event

// SubscribeAll returns a channel for all events (for TUI)
func (b *Bus) SubscribeAll(bufferSize int) <-chan Event

// SubscribeEntity returns events for a specific entity (e.g., download ID)
func (b *Bus) SubscribeEntity(entityType string, entityID int64, bufferSize int) <-chan Event

// Replay replays events from the log (for crash recovery)
func (b *Bus) Replay(ctx context.Context, since time.Time, handler func(Event) error) error

// Close shuts down all subscribers
func (b *Bus) Close() error
```

### 2. Event Log (`pkg/events/log.go`)

SQLite-backed event persistence for durability and replay.

```go
// EventLog persists events to SQLite
type EventLog struct {
    db *sql.DB
}

// Schema:
// CREATE TABLE events (
//     id INTEGER PRIMARY KEY AUTOINCREMENT,
//     event_type TEXT NOT NULL,
//     entity_type TEXT NOT NULL,
//     entity_id INTEGER NOT NULL,
//     payload JSON NOT NULL,
//     occurred_at TIMESTAMP NOT NULL,
//     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
//
//     -- Indexes for common queries
//     INDEX idx_events_type (event_type),
//     INDEX idx_events_entity (entity_type, entity_id),
//     INDEX idx_events_occurred (occurred_at)
// );

func (l *EventLog) Append(e Event) (int64, error)
func (l *EventLog) Since(t time.Time) ([]Event, error)
func (l *EventLog) ForEntity(entityType string, entityID int64) ([]Event, error)
func (l *EventLog) Prune(olderThan time.Duration) (int64, error)  // Cleanup old events
```

### 3. Event Types (`pkg/events/types.go`)

All domain events with their payloads.

```go
// ==================== Download Events ====================

// GrabRequested - User/API/Overseerr requested a download
type GrabRequested struct {
    BaseEvent
    ContentID   int64  `json:"content_id"`
    EpisodeID   *int64 `json:"episode_id,omitempty"`
    DownloadURL string `json:"download_url"`
    ReleaseName string `json:"release_name"`
    Indexer     string `json:"indexer"`
}

// DownloadCreated - Download record created, sent to client
type DownloadCreated struct {
    BaseEvent
    DownloadID  int64  `json:"download_id"`
    ContentID   int64  `json:"content_id"`
    ClientID    string `json:"client_id"`  // SABnzbd nzo_id
    ReleaseName string `json:"release_name"`
}

// DownloadProgressed - Progress update from client
type DownloadProgressed struct {
    BaseEvent
    DownloadID int64   `json:"download_id"`
    Progress   float64 `json:"progress"`      // 0.0 - 100.0
    Speed      int64   `json:"speed_bps"`     // bytes per second
    ETA        int     `json:"eta_seconds"`
    Size       int64   `json:"size_bytes"`
}

// DownloadCompleted - Client reports download finished
type DownloadCompleted struct {
    BaseEvent
    DownloadID int64  `json:"download_id"`
    SourcePath string `json:"source_path"`  // Where client put files
}

// DownloadFailed - Download failed
type DownloadFailed struct {
    BaseEvent
    DownloadID int64  `json:"download_id"`
    Reason     string `json:"reason"`
    Retryable  bool   `json:"retryable"`
}

// ==================== Import Events ====================

// ImportStarted - Import process beginning
type ImportStarted struct {
    BaseEvent
    DownloadID int64  `json:"download_id"`
    SourcePath string `json:"source_path"`
}

// ImportCompleted - File successfully imported
type ImportCompleted struct {
    BaseEvent
    DownloadID int64  `json:"download_id"`
    ContentID  int64  `json:"content_id"`
    FilePath   string `json:"file_path"`   // Final destination
    FileSize   int64  `json:"file_size"`
}

// ImportFailed - Import failed
type ImportFailed struct {
    BaseEvent
    DownloadID int64  `json:"download_id"`
    Reason     string `json:"reason"`
}

// ==================== Cleanup Events ====================

// CleanupStarted - Source cleanup beginning
type CleanupStarted struct {
    BaseEvent
    DownloadID int64  `json:"download_id"`
    SourcePath string `json:"source_path"`
}

// CleanupCompleted - Source files removed
type CleanupCompleted struct {
    BaseEvent
    DownloadID int64 `json:"download_id"`
}

// ==================== Library Events ====================

// ContentAdded - New content added to library
type ContentAdded struct {
    BaseEvent
    ContentID      int64  `json:"content_id"`
    ContentType    string `json:"content_type"`  // "movie" or "series"
    Title          string `json:"title"`
    Year           int    `json:"year"`
    QualityProfile string `json:"quality_profile"`
}

// ContentStatusChanged - Content status updated
type ContentStatusChanged struct {
    BaseEvent
    ContentID  int64  `json:"content_id"`
    OldStatus  string `json:"old_status"`
    NewStatus  string `json:"new_status"`
}

// ==================== External Service Events ====================

// PlexItemDetected - Plex scan found our imported file
type PlexItemDetected struct {
    BaseEvent
    ContentID int64  `json:"content_id"`
    PlexKey   string `json:"plex_key"`
}

// ClientStatusPolled - Raw status from download client (internal)
type ClientStatusPolled struct {
    BaseEvent
    ClientType string                 `json:"client_type"`  // "sabnzbd", "qbittorrent"
    Downloads  []ClientDownloadStatus `json:"downloads"`
}

type ClientDownloadStatus struct {
    ClientID   string  `json:"client_id"`
    Status     string  `json:"status"`
    Progress   float64 `json:"progress"`
    Speed      int64   `json:"speed"`
    ETA        int     `json:"eta"`
    Path       string  `json:"path,omitempty"`
}
```

### 4. Handlers (`internal/handlers/`)

Each handler is a goroutine that subscribes to events and processes them.

```go
// Handler processes events of specific types
type Handler interface {
    // Start begins processing events (blocking)
    Start(ctx context.Context) error

    // Name returns handler name for logging
    Name() string
}

// BaseHandler provides common handler functionality
type BaseHandler struct {
    bus    *events.Bus
    logger *slog.Logger
}
```

#### Download Handler (`internal/handlers/download.go`)

Manages download lifecycle.

```go
type DownloadHandler struct {
    BaseHandler
    store    *download.Store
    clients  map[string]DownloadClient  // "sabnzbd" -> client
}

func (h *DownloadHandler) Start(ctx context.Context) error {
    grabs := h.bus.Subscribe("grab.requested", 100)

    for {
        select {
        case e := <-grabs:
            h.handleGrabRequested(ctx, e.(*events.GrabRequested))
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (h *DownloadHandler) handleGrabRequested(ctx context.Context, e *events.GrabRequested) {
    // 1. Send to download client
    clientID, err := h.clients["sabnzbd"].Add(ctx, e.DownloadURL)
    if err != nil {
        h.bus.Publish(ctx, &events.DownloadFailed{...})
        return
    }

    // 2. Create DB record
    dl := &download.Download{...}
    h.store.Add(dl)

    // 3. Emit success event
    h.bus.Publish(ctx, &events.DownloadCreated{
        DownloadID: dl.ID,
        ClientID:   clientID,
        ...
    })
}
```

#### Import Handler (`internal/handlers/import.go`)

Handles file import when downloads complete.

```go
type ImportHandler struct {
    BaseHandler
    downloads *download.Store
    library   *library.Store
    importer  *importer.FileImporter

    // Per-download lock to prevent concurrent imports
    importing sync.Map  // map[int64]bool
}

func (h *ImportHandler) Start(ctx context.Context) error {
    completed := h.bus.Subscribe("download.completed", 100)

    for {
        select {
        case e := <-completed:
            go h.handleDownloadCompleted(ctx, e.(*events.DownloadCompleted))
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (h *ImportHandler) handleDownloadCompleted(ctx context.Context, e *events.DownloadCompleted) {
    // Acquire per-download lock (prevents races)
    if _, loaded := h.importing.LoadOrStore(e.DownloadID, true); loaded {
        h.logger.Warn("import already in progress", "download_id", e.DownloadID)
        return
    }
    defer h.importing.Delete(e.DownloadID)

    // Emit ImportStarted
    h.bus.Publish(ctx, &events.ImportStarted{
        DownloadID: e.DownloadID,
        SourcePath: e.SourcePath,
    })

    // Do the import
    result, err := h.importer.Import(ctx, e.DownloadID, e.SourcePath)
    if err != nil {
        h.bus.Publish(ctx, &events.ImportFailed{...})
        return
    }

    // Emit ImportCompleted
    h.bus.Publish(ctx, &events.ImportCompleted{
        DownloadID: e.DownloadID,
        FilePath:   result.DestPath,
        ...
    })
}
```

#### Cleanup Handler (`internal/handlers/cleanup.go`)

Cleans up source files after Plex verification.

```go
type CleanupHandler struct {
    BaseHandler
    downloads *download.Store
    plex      *plex.Client
    config    CleanupConfig
}

func (h *CleanupHandler) Start(ctx context.Context) error {
    imported := h.bus.Subscribe("import.completed", 100)
    plexDetected := h.bus.Subscribe("plex.item.detected", 100)

    for {
        select {
        case e := <-imported:
            // Start watching for Plex detection
            h.watchForPlex(ctx, e.(*events.ImportCompleted))
        case e := <-plexDetected:
            // Plex found it, safe to cleanup
            h.handlePlexDetected(ctx, e.(*events.PlexItemDetected))
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 5. Adapters (`internal/adapters/`)

Adapters poll external services and emit events.

#### SABnzbd Adapter (`internal/adapters/sabnzbd/adapter.go`)

```go
type Adapter struct {
    client   *sabnzbd.Client
    bus      *events.Bus
    store    *download.Store
    interval time.Duration
    logger   *slog.Logger
}

func (a *Adapter) Start(ctx context.Context) error {
    ticker := time.NewTicker(a.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            a.poll(ctx)
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (a *Adapter) poll(ctx context.Context) {
    // Get current state from SABnzbd
    queue, _ := a.client.Queue(ctx)
    history, _ := a.client.History(ctx)

    // Get our tracked downloads
    tracked, _ := a.store.ListActive()

    // Reconcile and emit events
    for _, dl := range tracked {
        clientStatus := a.findInClient(dl.ClientID, queue, history)

        if clientStatus == nil {
            // Download disappeared from client
            a.bus.Publish(ctx, &events.DownloadFailed{
                DownloadID: dl.ID,
                Reason:     "disappeared from download client",
            })
            continue
        }

        // Emit progress or completion events
        switch clientStatus.Status {
        case "downloading":
            a.bus.Publish(ctx, &events.DownloadProgressed{
                DownloadID: dl.ID,
                Progress:   clientStatus.Progress,
                ...
            })
        case "completed":
            if dl.Status != download.StatusCompleted {
                a.bus.Publish(ctx, &events.DownloadCompleted{
                    DownloadID: dl.ID,
                    SourcePath: clientStatus.Path,
                })
            }
        case "failed":
            a.bus.Publish(ctx, &events.DownloadFailed{
                DownloadID: dl.ID,
                Reason:     clientStatus.FailReason,
            })
        }
    }
}
```

#### Plex Adapter (`internal/adapters/plex/adapter.go`)

```go
type Adapter struct {
    client   *plex.Client
    bus      *events.Bus
    library  *library.Store
    interval time.Duration
}

func (a *Adapter) Start(ctx context.Context) error {
    // Subscribe to import completions to know what to watch for
    imported := a.bus.Subscribe("import.completed", 100)

    // Track pending verifications
    pending := make(map[int64]*events.ImportCompleted)
    ticker := time.NewTicker(a.interval)

    for {
        select {
        case e := <-imported:
            ic := e.(*events.ImportCompleted)
            pending[ic.ContentID] = ic

        case <-ticker.C:
            for contentID, ic := range pending {
                content, _ := a.library.GetContent(contentID)
                if a.client.HasContent(ctx, content) {
                    a.bus.Publish(ctx, &events.PlexItemDetected{
                        ContentID: contentID,
                    })
                    delete(pending, contentID)
                }
            }

        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 6. State Projection (`internal/projection/`)

Derives current state from events (optional optimization).

```go
// DownloadState projects current download state from events
type DownloadState struct {
    store *download.Store
    bus   *events.Bus
}

func (p *DownloadState) Start(ctx context.Context) error {
    // Subscribe to all download-related events
    events := p.bus.Subscribe("download.*", 1000)

    for {
        select {
        case e := <-events:
            p.apply(e)
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (p *DownloadState) apply(e events.Event) {
    switch e := e.(type) {
    case *events.DownloadCreated:
        // Already in DB from handler
    case *events.DownloadProgressed:
        p.store.UpdateProgress(e.DownloadID, e.Progress, e.ETA)
    case *events.DownloadCompleted:
        p.store.Transition(e.DownloadID, download.StatusCompleted)
    case *events.ImportStarted:
        p.store.Transition(e.DownloadID, download.StatusImporting)
    case *events.ImportCompleted:
        p.store.Transition(e.DownloadID, download.StatusImported)
    case *events.CleanupCompleted:
        p.store.Transition(e.DownloadID, download.StatusCleaned)
    case *events.DownloadFailed, *events.ImportFailed:
        p.store.Transition(e.EntityID(), download.StatusFailed)
    }
}
```

## Server Startup

```go
func (s *Server) Run(ctx context.Context) error {
    // Create event bus
    eventLog := events.NewEventLog(s.db)
    bus := events.NewBus(eventLog, s.logger)

    // Create handlers
    downloadHandler := handlers.NewDownloadHandler(bus, s.downloadStore, s.clients)
    importHandler := handlers.NewImportHandler(bus, s.downloadStore, s.library, s.importer)
    cleanupHandler := handlers.NewCleanupHandler(bus, s.downloadStore, s.plex, s.config.Cleanup)

    // Create adapters
    sabnzbdAdapter := adapters.NewSABnzbdAdapter(bus, s.sabnzbd, s.downloadStore, 30*time.Second)
    plexAdapter := adapters.NewPlexAdapter(bus, s.plex, s.library, 60*time.Second)

    // Create state projection
    downloadProjection := projection.NewDownloadState(s.downloadStore, bus)

    // Replay any unprocessed events from crash recovery
    bus.Replay(ctx, s.lastShutdown, func(e events.Event) error {
        // Re-process events that weren't fully handled
        return nil
    })

    // Start all components
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error { return downloadHandler.Start(ctx) })
    g.Go(func() error { return importHandler.Start(ctx) })
    g.Go(func() error { return cleanupHandler.Start(ctx) })
    g.Go(func() error { return sabnzbdAdapter.Start(ctx) })
    g.Go(func() error { return plexAdapter.Start(ctx) })
    g.Go(func() error { return downloadProjection.Start(ctx) })
    g.Go(func() error { return s.httpServer.Serve(ctx) })

    return g.Wait()
}
```

## API Integration

APIs emit events instead of direct state manipulation.

```go
// POST /api/v1/grab
func (h *GrabHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req GrabRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Emit event - handler will process
    h.bus.Publish(r.Context(), &events.GrabRequested{
        BaseEvent: events.BaseEvent{
            Type:      "grab.requested",
            Timestamp: time.Now(),
        },
        ContentID:   req.ContentID,
        DownloadURL: req.DownloadURL,
        ReleaseName: req.ReleaseName,
        Indexer:     req.Indexer,
    })

    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
```

## TUI Integration (Future)

```go
func (t *TUI) Start(ctx context.Context) error {
    // Subscribe to all events for real-time updates
    events := t.bus.SubscribeAll(1000)

    for {
        select {
        case e := <-events:
            t.handleEvent(e)
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (t *TUI) handleEvent(e events.Event) {
    switch e := e.(type) {
    case *events.DownloadProgressed:
        t.updateDownloadProgress(e.DownloadID, e.Progress)
    case *events.DownloadCompleted:
        t.markDownloadComplete(e.DownloadID)
    case *events.ImportCompleted:
        t.showImportSuccess(e.ContentID)
    // etc.
    }
}
```

## Migration Strategy

### Phase 1: Event Bus Core
1. Create `pkg/events/` package
2. Implement Bus, EventLog, BaseEvent
3. Add events table migration
4. Unit tests for bus and log

### Phase 2: Event Types
1. Define all event types in `pkg/events/types.go`
2. Implement JSON marshaling/unmarshaling
3. Event type registry for deserialization

### Phase 3: Handlers
1. Create `internal/handlers/` package
2. Implement DownloadHandler (extract from current manager)
3. Implement ImportHandler (extract from current poller)
4. Implement CleanupHandler (extract from current poller)
5. Per-handler unit tests with mock bus

### Phase 4: Adapters
1. Create `internal/adapters/` package
2. Implement SABnzbdAdapter (extract polling from current manager)
3. Implement PlexAdapter (extract from current poller)
4. Per-adapter integration tests

### Phase 5: Wire Up
1. Update server.go to use event-driven components
2. Update API handlers to emit events
3. Remove old poller code
4. Integration tests for full flow

### Phase 6: State Projection
1. Implement download state projection
2. Migrate status updates to projection
3. Remove direct status updates from handlers

## Testing Strategy

### Unit Tests
- Bus: publish/subscribe, buffering, close
- EventLog: append, query, prune
- Handlers: mock bus, verify events emitted
- Adapters: mock clients, verify events emitted

### Integration Tests
- Full flow: GrabRequested → DownloadCreated → ... → CleanupCompleted
- Crash recovery: events replayed correctly
- Concurrent operations: no races

### Property Tests
- Event ordering preserved
- No duplicate processing
- State consistency after any event sequence

## File Structure

```
pkg/
└── events/
    ├── bus.go           # Event bus implementation
    ├── bus_test.go
    ├── log.go           # SQLite event log
    ├── log_test.go
    ├── types.go         # All event type definitions
    └── types_test.go

internal/
├── handlers/
│   ├── handler.go       # Handler interface
│   ├── download.go      # Download lifecycle handler
│   ├── download_test.go
│   ├── import.go        # Import handler
│   ├── import_test.go
│   ├── cleanup.go       # Cleanup handler
│   └── cleanup_test.go
├── adapters/
│   ├── adapter.go       # Adapter interface
│   ├── sabnzbd/
│   │   ├── adapter.go
│   │   └── adapter_test.go
│   └── plex/
│       ├── adapter.go
│       └── adapter_test.go
└── projection/
    ├── download.go      # Download state projection
    └── download_test.go
```

## Success Criteria

1. **No race conditions**: Concurrent imports impossible due to per-entity locking
2. **Clear audit trail**: All state changes recorded as events
3. **Crash recovery**: Server restart replays unprocessed events
4. **Real-time ready**: TUI can subscribe to event stream
5. **Extensible**: Adding BitTorrent = new adapter only
6. **Testable**: Each component testable in isolation
7. **All existing functionality preserved**: CLI, API, compat layer unchanged

## Open Questions

1. **Event retention**: How long to keep events? 7 days? 30 days? Configurable?
2. **Backpressure**: What if subscriber falls behind? Drop events? Block publisher?
3. **Event versioning**: How to handle event schema changes over time?
4. **Distributed future**: If arrgo ever needs multiple instances, do we need a real message broker?

## References

- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Share Memory By Communicating](https://go.dev/blog/codelab-share)
- [Kubernetes Controller Pattern](https://kubernetes.io/docs/concepts/architecture/controller/)
- [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
