# Logging Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add structured logging (slog/logfmt) to arrgo serve for debugging and observability.

**Architecture:** Use stdlib log/slog with TextHandler for logfmt output. Create logger in serve.go, pass child loggers to components. Add HTTP middleware for request logging.

**Tech Stack:** Go 1.21+ stdlib `log/slog`

---

### Task 1: Add HTTP logging middleware to serve.go

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Add imports and helper function**

Add to imports:
```go
"log/slog"
"strings"
```

Add after the imports, before `runServe`:
```go
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

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

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

**Step 2: Create logger and wire up in runServe**

At the start of `runServe`, after loading config, add:
```go
// Create logger
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	Level: parseLogLevel(cfg.Server.LogLevel),
}))
```

Replace the startup fmt.Printf statements with logger calls:
```go
logger.Info("server starting",
	"addr", addr,
	"database", cfg.Database.Path,
	"sabnzbd", sabClient != nil,
	"indexers", len(cfg.Indexers),
	"plex", plexClient != nil,
	"log_level", cfg.Server.LogLevel,
)
```

Replace the shutdown fmt.Printf with:
```go
logger.Info("received signal, shutting down", "signal", sig.String())
```

Replace the stopped message with:
```go
logger.Info("server stopped")
```

**Step 3: Wire HTTP middleware**

Change:
```go
srv := &http.Server{Addr: addr, Handler: mux}
```
To:
```go
srv := &http.Server{Addr: addr, Handler: logRequests(mux, logger)}
```

**Step 4: Run and verify**

Run: `go build ./cmd/arrgo && ./arrgo serve`
Expected: See logfmt output like `level=INFO msg="server starting" addr=0.0.0.0:8484 ...`

**Step 5: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(logging): add HTTP request logging middleware"
```

---

### Task 2: Add logging to IndexerPool

**Files:**
- Modify: `internal/search/indexer.go`

**Step 1: Add logger field and update constructor**

Add import:
```go
"log/slog"
"time"
```

Update struct:
```go
type IndexerPool struct {
	clients []*newznab.Client
	log     *slog.Logger
}
```

Update constructor:
```go
func NewIndexerPool(clients []*newznab.Client, log *slog.Logger) *IndexerPool {
	return &IndexerPool{clients: clients, log: log}
}
```

**Step 2: Add logging to Search method**

At the start of Search:
```go
p.log.Debug("search started", "query", q.Text, "type", q.Type, "indexers", len(p.clients))
start := time.Now()
```

Inside the goroutine, after getting results:
```go
go func(c *newznab.Client) {
	defer wg.Done()
	indexerStart := time.Now()
	releases, err := c.Search(ctx, q.Text, categories)
	if err != nil {
		p.log.Warn("indexer failed", "indexer", c.Name(), "error", err, "duration_ms", time.Since(indexerStart).Milliseconds())
	} else {
		p.log.Debug("indexer returned", "indexer", c.Name(), "results", len(releases), "duration_ms", time.Since(indexerStart).Milliseconds())
	}
	results <- result{releases: releases, err: err}
}(client)
```

At the end of Search, before return:
```go
p.log.Info("search complete", "query", q.Text, "results", len(allReleases), "errors", len(errs), "duration_ms", time.Since(start).Milliseconds())
```

**Step 3: Add Name() method to newznab.Client if missing**

Check `pkg/newznab/client.go` - if no Name() method exists, add:
```go
func (c *Client) Name() string {
	return c.name
}
```

**Step 4: Run tests**

Run: `go build ./...`
Expected: Build fails because NewIndexerPool signature changed

**Step 5: Commit**

```bash
git add internal/search/indexer.go pkg/newznab/client.go
git commit -m "feat(logging): add logging to IndexerPool"
```

---

### Task 3: Add logging to Searcher

**Files:**
- Modify: `internal/search/search.go`

**Step 1: Add logger field and update constructor**

Add import:
```go
"log/slog"
```

Update struct:
```go
type Searcher struct {
	indexers IndexerAPI
	scorer   *Scorer
	log      *slog.Logger
}
```

Update constructor:
```go
func NewSearcher(indexers IndexerAPI, scorer *Scorer, log *slog.Logger) *Searcher {
	return &Searcher{
		indexers: indexers,
		scorer:   scorer,
		log:      log,
	}
}
```

**Step 2: Add logging to Search method**

At the start:
```go
s.log.Info("search started", "query", q.Text, "type", q.Type, "profile", profile)
```

After scoring loop, before sort:
```go
s.log.Debug("scoring complete", "raw", len(releases), "filtered", len(result.Releases))
```

**Step 3: Commit**

```bash
git add internal/search/search.go
git commit -m "feat(logging): add logging to Searcher"
```

---

### Task 4: Add logging to download Manager

**Files:**
- Modify: `internal/download/manager.go`

**Step 1: Add logger field and update constructor**

Add import:
```go
"log/slog"
```

Update struct:
```go
type Manager struct {
	client Downloader
	store  *Store
	log    *slog.Logger
}
```

Update constructor:
```go
func NewManager(client Downloader, store *Store, log *slog.Logger) *Manager {
	return &Manager{
		client: client,
		store:  store,
		log:    log,
	}
}
```

**Step 2: Add logging to Grab method**

After successful client.Add:
```go
m.log.Info("grab sent", "content_id", contentID, "release", releaseName, "client_id", clientID)
```

On error:
```go
m.log.Error("grab failed", "content_id", contentID, "error", err)
```

**Step 3: Add logging to Refresh method**

At start:
```go
m.log.Debug("refresh started", "active_downloads", len(downloads))
```

On status change:
```go
m.log.Info("download status changed", "download_id", d.ID, "status", status.Status, "prev", d.Status)
```

On error:
```go
m.log.Error("refresh error", "download_id", d.ID, "error", err)
```

**Step 4: Commit**

```bash
git add internal/download/manager.go
git commit -m "feat(logging): add logging to download Manager"
```

---

### Task 5: Add logging to Importer

**Files:**
- Modify: `internal/importer/importer.go`

**Step 1: Add logger field and update constructor**

Add import:
```go
"log/slog"
```

Update struct (add log field):
```go
type Importer struct {
	downloads  *download.Store
	library    *library.Store
	history    *HistoryStore
	renamer    *Renamer
	plex       *PlexClient
	movieRoot  string
	seriesRoot string
	log        *slog.Logger
}
```

Update New function signature and initialization:
```go
func New(db *sql.DB, cfg Config, log *slog.Logger) *Importer {
	// ... existing code ...
	return &Importer{
		// ... existing fields ...
		log: log,
	}
}
```

**Step 2: Add logging to Import method**

At start:
```go
i.log.Info("import started", "download_id", downloadID, "path", downloadPath)
```

After finding video:
```go
i.log.Debug("found video", "path", srcPath)
```

After successful copy:
```go
i.log.Debug("file copied", "src", srcPath, "dest", destPath, "size_bytes", size)
```

At end on success:
```go
i.log.Info("import complete", "download_id", downloadID, "dest", destPath, "quality", quality)
```

On Plex notification:
```go
if result.PlexNotified {
	i.log.Debug("plex notified", "path", destPath)
} else if result.PlexError != nil {
	i.log.Warn("plex notification failed", "error", result.PlexError)
}
```

**Step 3: Commit**

```bash
git add internal/importer/importer.go
git commit -m "feat(logging): add logging to Importer"
```

---

### Task 6: Wire up loggers in serve.go

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Update component creation to pass loggers**

Update IndexerPool creation:
```go
indexerPool = search.NewIndexerPool(newznabClients, logger.With("component", "indexer"))
```

Update Searcher creation:
```go
searcher = search.NewSearcher(indexerPool, scorer, logger.With("component", "search"))
```

Update Manager creation:
```go
downloadManager = download.NewManager(sabClient, downloadStore, logger.With("component", "download"))
```

Update Importer creation:
```go
imp := importer.New(db, importer.Config{
	// ... existing config ...
}, logger.With("component", "importer"))
```

**Step 2: Update poller to use logger**

Update runPoller signature:
```go
func runPoller(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, log *slog.Logger)
```

Replace fmt.Println in poller:
```go
log.Info("poller started", "interval", "30s")
// ...
log.Info("poller stopped")
// ...
log.Info("importing download", "download_id", dl.ID, "path", clientStatus.Path)
// ...
log.Error("import failed", "download_id", dl.ID, "error", err)
```

Update call site:
```go
go runPoller(ctx, downloadManager, imp, downloadStore, logger.With("component", "poller"))
```

**Step 3: Build and test**

Run: `go build ./cmd/arrgo && ./arrgo serve`
Expected: Full logfmt output from all components

**Step 4: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(logging): wire up component loggers in serve"
```

---

### Task 7: Update tests

**Files:**
- Modify: `internal/search/search_test.go`
- Modify: Any other test files that call modified constructors

**Step 1: Fix search tests**

Update mock creation and NewSearcher calls to pass a logger:
```go
import "log/slog"

// In tests, use a discard logger
log := slog.New(slog.NewTextHandler(io.Discard, nil))
searcher := NewSearcher(mockClient, scorer, log)
```

**Step 2: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 3: Commit**

```bash
git add -A
git commit -m "test: update tests for new logger parameters"
```

---

### Task 8: Final verification

**Step 1: Full test**

```bash
go build ./cmd/arrgo
./arrgo serve
```

In another terminal:
```bash
./arrgo search "The Matrix"
```

**Step 2: Verify log output**

Expected output should include:
- `level=INFO msg="server starting"` with config details
- `level=INFO msg="http request"` for each API call
- `level=INFO msg="search started"` and `level=INFO msg="search complete"`
- Indexer-level logs at debug level

**Step 3: Test debug level**

Edit config.toml: `log_level = "debug"`
Restart server, run search again.
Expected: More verbose output including indexer timing, scoring details.

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat(logging): complete structured logging implementation"
```
