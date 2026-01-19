# Serve Command Design

**Date:** 2026-01-18
**Status:** ✅ Complete

## Overview

`arrgo serve` starts the HTTP server and background poller. Single command, no flags - all configuration from config.toml.

## Entry Point

```
arrgo serve [--config path]
```

1. Load config (default: `config.toml`)
2. Validate config - exit with clear error if invalid
3. Open SQLite database (create if missing)
4. Run migrations (ensure schema exists)
5. Wire up all dependencies
6. Start background poller
7. Start HTTP server
8. Wait for SIGINT/SIGTERM
9. Graceful shutdown (30s timeout)

**Exit codes:**
- 0: Clean shutdown
- 1: Config error
- 2: Database error
- 3: Server startup error

## Dependency Wiring

All dependencies created in `cmd/arrgo/serve.go`. Order: stores → clients → services → APIs.

```go
db := sql.Open(cfg.Database.Path)

// Stores (always created)
libraryStore := library.NewStore(db)
downloadStore := download.NewStore(db)
historyStore := importer.NewHistoryStore(db)

// Clients (optional - nil if not configured)
var sabClient *download.SABnzbdClient
if cfg.Downloaders.SABnzbd != nil {
    sabClient = download.NewSABnzbdClient(...)
}

var prowlarrClient *search.ProwlarrClient
if cfg.Indexers.Prowlarr != nil {
    prowlarrClient = search.NewProwlarrClient(...)
}

var plexClient *importer.PlexClient
if cfg.Notifications.Plex != nil {
    plexClient = importer.NewPlexClient(...)
}

// Services (depend on clients)
var downloadManager *download.Manager
if sabClient != nil {
    downloadManager = download.NewManager(sabClient, downloadStore)
}

var searcher *search.Searcher
if prowlarrClient != nil {
    scorer := search.NewScorer(qualityProfiles)
    searcher = search.NewSearcher(prowlarrClient, scorer)
}

imp := importer.New(db, importerConfig)
```

Optional clients being nil is handled by the API (returns 503 Service Unavailable).

## Background Poller

Single goroutine, 30-second ticker:

```go
func runPoller(ctx context.Context, manager *download.Manager,
               imp *importer.Importer, store *download.Store) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            poll(ctx, manager, imp, store)
        }
    }
}

func poll(ctx context.Context, ...) {
    // 1. Refresh download statuses from SABnzbd
    manager.Refresh(ctx)

    // 2. Find completed downloads not yet imported
    completed := store.List(DownloadFilter{Status: StatusCompleted})

    // 3. Import each one inline (sequential)
    for _, dl := range completed {
        status, _ := manager.client.Status(ctx, dl.ClientID)
        if status == nil || status.Path == "" {
            continue
        }

        if err := imp.Import(ctx, dl.ID, status.Path); err != nil {
            log.Printf("import %d failed: %v", dl.ID, err)
            continue
        }

        dl.Status = StatusImported
        store.Update(dl)
    }
}
```

Poller only runs if `downloadManager != nil`.

## Graceful Shutdown

```go
// Context for background jobs
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Start poller
if downloadManager != nil {
    go runPoller(ctx, downloadManager, imp, downloadStore)
}

// Start HTTP server
srv := &http.Server{Addr: addr, Handler: mux}
go func() {
    if err := srv.ListenAndServe(); err != http.ErrServerClosed {
        log.Printf("server error: %v", err)
    }
}()

// Wait for signal
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
<-sigCh

log.Println("shutting down...")

// Cancel background jobs
cancel()

// Graceful HTTP shutdown (30s timeout)
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer shutdownCancel()

return srv.Shutdown(shutdownCtx)
```

## HTTP Routing

```go
mux := http.NewServeMux()

// Native API v1
apiV1 := v1.New(db, v1Config)
apiV1.SetDependencies(libraryStore, downloadStore, downloadManager,
                       searcher, historyStore, plexClient)
apiV1.RegisterRoutes(mux)  // /api/v1/*

// Compat API (if enabled)
if cfg.Compat.Radarr || cfg.Compat.Sonarr {
    apiCompat := compat.New(cfg.Compat.APIKey)
    apiCompat.SetDependencies(libraryStore, downloadStore, ...)
    apiCompat.RegisterRoutes(mux)  // /api/v3/*
}
```

## Code Changes

**`cmd/arrgo/main.go`**
- Add `runServe()` function call for "serve" command

**`cmd/arrgo/serve.go`** (new file)
- `runServe(configPath string) error` - main orchestration
- `runPoller(ctx, manager, imp, store)` - background polling
- `poll(ctx, ...)` - single poll iteration

**`internal/api/v1/api.go`**
- Add `SetDependencies()` method to inject optional services
- Add `RegisterRoutes(mux)` method

**`internal/api/compat/compat.go`**
- Add `SetDependencies()` method
- Wire handlers to actual stores

## Out of Scope

- CLI flags for host/port override
- Multiple download clients
- Parallel imports
- Health check endpoint (can add later)
