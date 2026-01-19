# Serve Command Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `arrgo serve` to start the HTTP server and background download poller.

**Architecture:** Single `serve.go` file handles config loading, dependency wiring, HTTP server startup, background poller, and graceful shutdown. All components are already implemented - this is pure orchestration.

**Tech Stack:** Go stdlib (net/http, database/sql, os/signal), existing arrgo modules

---

### Task 1: Serve Command Entry Point

**Files:**
- Modify: `cmd/arrgo/main.go`
- Create: `cmd/arrgo/serve.go`

**Step 1: Update main.go to call runServe**

In `cmd/arrgo/main.go`, replace the "serve" case stub:

```go
case "serve":
    configPath := "config.toml"
    if len(os.Args) > 2 && os.Args[2] == "--config" && len(os.Args) > 3 {
        configPath = os.Args[3]
    }
    if err := runServe(configPath); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
```

**Step 2: Create serve.go skeleton**

Create `cmd/arrgo/serve.go`:

```go
package main

import (
	"fmt"

	"github.com/arrgo/arrgo/internal/config"
)

func runServe(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	fmt.Printf("arrgo starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	return nil
}
```

**Step 3: Verify it compiles and runs**

Run: `go build ./cmd/arrgo && ./arrgo serve`
Expected: "config: ..." error (no config.toml) or "arrgo starting on ..." message

**Step 4: Commit**

```bash
git add cmd/arrgo/main.go cmd/arrgo/serve.go
git commit -m "feat(cli): add serve command skeleton"
```

---

### Task 2: Database Setup

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Add database opening and migration**

Update `runServe` in `cmd/arrgo/serve.go`:

```go
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/arrgo/arrgo/internal/config"
)

//go:embed ../../migrations/001_initial.sql
var migrationSQL string

func runServe(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Run migrations
	if _, err := db.Exec(migrationSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	fmt.Printf("arrgo starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	return nil
}
```

**Step 2: Verify database is created**

Create a minimal config.toml:
```bash
cat > config.toml << 'EOF'
[server]
port = 8484

[database]
path = "./data/test.db"

[libraries.movies]
root = "/tmp/movies"
naming = "{title}.{ext}"

[libraries.series]
root = "/tmp/series"
naming = "{title}.{ext}"
EOF
```

Run: `go build ./cmd/arrgo && ./arrgo serve`
Expected: "arrgo starting on 0.0.0.0:8484" and `./data/test.db` exists

**Step 3: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(cli): add database setup with migrations"
```

---

### Task 3: Dependency Wiring

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Add all dependency wiring**

Update `runServe` to create stores, clients, and services:

```go
package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/arrgo/arrgo/internal/config"
	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/importer"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/search"
)

//go:embed ../../migrations/001_initial.sql
var migrationSQL string

func runServe(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Run migrations
	if _, err := db.Exec(migrationSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// === Stores (always created) ===
	libraryStore := library.NewStore(db)
	downloadStore := download.NewStore(db)
	historyStore := importer.NewHistoryStore(db)

	// === Clients (optional) ===
	var sabClient *download.SABnzbdClient
	if cfg.Downloaders.SABnzbd != nil {
		sabClient = download.NewSABnzbdClient(
			cfg.Downloaders.SABnzbd.URL,
			cfg.Downloaders.SABnzbd.APIKey,
			cfg.Downloaders.SABnzbd.Category,
		)
	}

	var prowlarrClient *search.ProwlarrClient
	if cfg.Indexers.Prowlarr != nil {
		prowlarrClient = search.NewProwlarrClient(
			cfg.Indexers.Prowlarr.URL,
			cfg.Indexers.Prowlarr.APIKey,
		)
	}

	var plexClient *importer.PlexClient
	if cfg.Notifications.Plex != nil {
		plexClient = importer.NewPlexClient(
			cfg.Notifications.Plex.URL,
			cfg.Notifications.Plex.Token,
		)
	}

	// === Services ===
	var downloadManager *download.Manager
	if sabClient != nil {
		downloadManager = download.NewManager(sabClient, downloadStore)
	}

	var searcher *search.Searcher
	if prowlarrClient != nil {
		// Build quality profiles map
		profiles := make(map[string][]string)
		for name, p := range cfg.Quality.Profiles {
			profiles[name] = p.Accept
		}
		scorer := search.NewScorer(profiles)
		searcher = search.NewSearcher(prowlarrClient, scorer)
	}

	imp := importer.New(db, importer.Config{
		MovieRoot:      cfg.Libraries.Movies.Root,
		SeriesRoot:     cfg.Libraries.Series.Root,
		MovieTemplate:  cfg.Libraries.Movies.Naming,
		SeriesTemplate: cfg.Libraries.Series.Naming,
		PlexURL:        plexURL(cfg),
		PlexToken:      plexToken(cfg),
	})

	// Log what's configured
	fmt.Printf("arrgo starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("  database: %s\n", cfg.Database.Path)
	fmt.Printf("  sabnzbd: %v\n", sabClient != nil)
	fmt.Printf("  prowlarr: %v\n", prowlarrClient != nil)
	fmt.Printf("  plex: %v\n", plexClient != nil)

	// Silence unused variable warnings (will be used in next task)
	_ = libraryStore
	_ = historyStore
	_ = downloadManager
	_ = searcher
	_ = imp

	return nil
}

func plexURL(cfg *config.Config) string {
	if cfg.Notifications.Plex != nil {
		return cfg.Notifications.Plex.URL
	}
	return ""
}

func plexToken(cfg *config.Config) string {
	if cfg.Notifications.Plex != nil {
		return cfg.Notifications.Plex.Token
	}
	return ""
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo && ./arrgo serve`
Expected: Shows what's configured (sabnzbd: false, etc.)

**Step 3: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(cli): add dependency wiring"
```

---

### Task 4: HTTP Server Setup

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Add HTTP server with API routes**

Add imports and update runServe to set up HTTP:

```go
import (
	// ... existing imports ...
	"net/http"

	"github.com/arrgo/arrgo/internal/api/compat"
	v1 "github.com/arrgo/arrgo/internal/api/v1"
)

// In runServe, after dependency wiring, replace the _ = lines with:

	// === HTTP Setup ===
	mux := http.NewServeMux()

	// Native API v1
	profiles := make(map[string][]string)
	for name, p := range cfg.Quality.Profiles {
		profiles[name] = p.Accept
	}
	apiV1 := v1.New(db, v1.Config{
		MovieRoot:       cfg.Libraries.Movies.Root,
		SeriesRoot:      cfg.Libraries.Series.Root,
		QualityProfiles: profiles,
	})
	apiV1.SetSearcher(searcher)
	apiV1.SetManager(downloadManager)
	apiV1.SetPlex(plexClient)
	apiV1.RegisterRoutes(mux)

	// Compat API (if enabled)
	if cfg.Compat.Radarr || cfg.Compat.Sonarr {
		apiCompat := compat.New(cfg.Compat.APIKey)
		apiCompat.RegisterRoutes(mux)
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("arrgo listening on %s\n", addr)

	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
```

**Step 2: Test the server starts**

Run: `go build ./cmd/arrgo && ./arrgo serve &`
Then: `curl http://localhost:8484/api/v1/status`
Expected: `{"status":"ok","version":"0.1.0"}`

Kill the server: `kill %1`

**Step 3: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(cli): add HTTP server with API routes"
```

---

### Task 5: Background Poller

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Add poller functions**

Add at the end of serve.go:

```go
func runPoller(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	fmt.Println("poller: started (30s interval)")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("poller: stopped")
			return
		case <-ticker.C:
			poll(ctx, manager, imp, store)
		}
	}
}

func poll(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store) {
	// Refresh download statuses from client
	if err := manager.Refresh(ctx); err != nil {
		fmt.Printf("poller: refresh error: %v\n", err)
	}

	// Find completed downloads
	completed, err := store.List(download.DownloadFilter{Status: statusPtr(download.StatusCompleted)})
	if err != nil {
		fmt.Printf("poller: list error: %v\n", err)
		return
	}

	// Import each one
	for _, dl := range completed {
		status, err := manager.Client().Status(ctx, dl.ClientID)
		if err != nil || status == nil || status.Path == "" {
			continue
		}

		fmt.Printf("poller: importing download %d from %s\n", dl.ID, status.Path)

		if err := imp.Import(ctx, dl.ID, status.Path); err != nil {
			fmt.Printf("poller: import %d failed: %v\n", dl.ID, err)
			continue
		}

		dl.Status = download.StatusImported
		if err := store.Update(dl); err != nil {
			fmt.Printf("poller: update %d failed: %v\n", dl.ID, err)
		}
	}
}

func statusPtr(s download.Status) *download.Status {
	return &s
}
```

**Step 2: Add context import and start poller in runServe**

Add "context" and "time" to imports.

In runServe, before the HTTP server setup:

```go
	// === Background Jobs ===
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if downloadManager != nil {
		go runPoller(ctx, downloadManager, imp, downloadStore)
	}
```

**Step 3: Add Client() method to Manager**

The poller needs to access the download client. Check if Manager exposes it. If not, add to `internal/download/manager.go`:

```go
// Client returns the underlying download client.
func (m *Manager) Client() Downloader {
	return m.client
}
```

**Step 4: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: Compiles successfully

**Step 5: Commit**

```bash
git add cmd/arrgo/serve.go internal/download/manager.go
git commit -m "feat(cli): add background download poller"
```

---

### Task 6: Graceful Shutdown

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Add signal handling and graceful shutdown**

Add "os/signal" and "syscall" to imports.

Replace the HTTP server start section in runServe:

```go
	// === HTTP Server ===
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	// Start server in goroutine
	go func() {
		fmt.Printf("arrgo listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	fmt.Printf("\nreceived %v, shutting down...\n", sig)

	// Cancel background jobs
	cancel()

	// Graceful HTTP shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	fmt.Println("arrgo stopped")
	return nil
```

**Step 2: Test graceful shutdown**

Run: `go build ./cmd/arrgo && ./arrgo serve &`
Wait 2 seconds, then: `kill -SIGTERM %1`
Expected: "received terminated, shutting down..." then "arrgo stopped"

**Step 3: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(cli): add graceful shutdown"
```

---

### Task 7: Final Verification

**Step 1: Run the full test suite**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Run the linter**

Run: `golangci-lint run ./...`
Expected: No issues (or only minor ones)

**Step 3: Manual integration test**

Create test config if not exists, then:
```bash
./arrgo serve
# In another terminal:
curl http://localhost:8484/api/v1/status
curl http://localhost:8484/api/v1/content
# Ctrl+C to stop
```

Expected: Server responds correctly, shuts down cleanly

**Step 4: Commit any fixes**

If linter or tests found issues, fix and commit.

---

## Summary

After completing all tasks, the `arrgo serve` command will:

1. Load and validate config from config.toml
2. Create/open SQLite database with migrations
3. Wire up all dependencies (stores, clients, services)
4. Start background poller (if download client configured)
5. Start HTTP server with v1 and compat APIs
6. Handle SIGINT/SIGTERM with graceful shutdown
