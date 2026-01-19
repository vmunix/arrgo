# Logging & Event Emission Design

**Date:** 2026-01-18
**Status:** Approved

## Overview

Add structured logging to arrgo serve for debugging and observability. Primary consumers are humans watching terminal output and LLMs analyzing pasted logs.

## Design Decisions

1. **Format: logfmt** - `level=INFO msg="query complete" query="The Matrix" results=44`
   - Human readable, LLM parseable, convertible to JSON if needed later

2. **Levels: debug/info/warn/error** - Standard four levels, `info` default
   - `debug` for verbose details (scoring, API responses)
   - `info` for normal operations (requests, searches, grabs)
   - `warn` for partial failures (indexer timeout but others worked)
   - `error` for failures

3. **Output: always stdout** - Let external tools handle redirection
   - Works with systemd, Docker, process managers
   - No file rotation complexity

4. **Implementation: stdlib log/slog** - Built into Go 1.21+, no dependencies

## Components

### HTTP Middleware

Wrap mux to log every request:

```go
func logRequests(next http.Handler, log *slog.Logger) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        wrapped := &statusRecorder{ResponseWriter: w, status: 200}
        next.ServeHTTP(wrapped, r)
        log.Info("http request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", wrapped.status,
            "duration_ms", time.Since(start).Milliseconds(),
        )
    })
}
```

### Component Loggers

Each module receives a child logger with component name:

```go
searcher := search.NewSearcher(indexerPool, scorer, logger.With("component", "search"))
downloadManager := download.NewManager(sabClient, store, logger.With("component", "download"))
imp := importer.New(db, cfg, logger.With("component", "importer"))
```

### Event Coverage

**error**
- HTTP 5xx responses
- Indexer query failures
- SABnzbd communication failures
- Import failures
- Database errors

**warn**
- Partial indexer failure (some worked, some didn't)
- Download stuck/stalled detection
- Config validation warnings

**info**
- Server start/stop
- HTTP requests (method, path, status, duration)
- Search initiated and completed
- Grab sent to downloader
- Download status changes
- Import completed

**debug**
- Full search results before filtering
- Quality scoring decisions
- Indexer response details
- SABnzbd API calls and responses
- File operations during import

## Wiring

```go
func runServe(configPath string) error {
    // Create logger from config
    level := parseLogLevel(cfg.Server.LogLevel)
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: level,
    }))

    // Pass to components
    indexerPool := search.NewIndexerPool(clients, logger.With("component", "indexer"))
    searcher := search.NewSearcher(indexerPool, scorer, logger.With("component", "search"))
    // ... etc

    srv := &http.Server{Addr: addr, Handler: logRequests(mux, logger)}
}

func parseLogLevel(s string) slog.Level {
    switch strings.ToLower(s) {
    case "debug":
        return slog.LevelDebug
    case "warn":
        return slog.LevelWarn
    case "error":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}
```

## Example Output

At `info` level:
```
level=INFO msg="server started" addr=0.0.0.0:8484 log_level=info
level=INFO msg="poller started" interval=30s
level=INFO msg="http request" method=POST path=/api/v1/search status=200 duration_ms=1247
level=INFO msg="search started" component=search query="The Matrix" profile=hd
level=INFO msg="search complete" component=search query="The Matrix" results=44 filtered=12 duration_ms=1243
level=INFO msg="http request" method=POST path=/api/v1/grab status=200 duration_ms=156
level=INFO msg="grab sent" component=download content_id=5 release="The.Matrix.1999.1080p.BluRay" client=sabnzbd
```

## Files Changed

| Action | File |
|--------|------|
| Modify | `cmd/arrgo/serve.go` - logger setup, HTTP middleware, wiring |
| Modify | `internal/search/search.go` - add logger param, log events |
| Modify | `internal/search/indexer.go` - add logger param, log events |
| Modify | `internal/download/manager.go` - add logger param, log events |
| Modify | `internal/importer/importer.go` - add logger param, log events |
