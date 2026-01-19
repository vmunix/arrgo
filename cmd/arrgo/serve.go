package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/arrgo/arrgo/internal/api/compat"
	v1 "github.com/arrgo/arrgo/internal/api/v1"
	"github.com/arrgo/arrgo/internal/config"
	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/importer"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/migrations"
	"github.com/arrgo/arrgo/internal/search"
)

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
	if _, err := db.Exec(migrations.InitialSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// === Stores (always created) ===
	libraryStore := library.NewStore(db)
	downloadStore := download.NewStore(db)
	historyStore := importer.NewHistoryStore(db)

	// === Clients (optional - nil if not configured) ===
	var sabClient *download.SABnzbdClient
	if cfg.Downloaders.SABnzbd != nil {
		sabClient = download.NewSABnzbdClient(
			cfg.Downloaders.SABnzbd.URL,
			cfg.Downloaders.SABnzbd.APIKey,
			cfg.Downloaders.SABnzbd.Category,
		)
	}

	// TODO: Replace with multi-indexer Newznab client when pkg/newznab is implemented
	// For now, use the first configured indexer with ProwlarrClient (compatible API)
	var prowlarrClient *search.ProwlarrClient
	for _, indexer := range cfg.Indexers {
		prowlarrClient = search.NewProwlarrClient(
			indexer.URL,
			indexer.APIKey,
		)
		break // Use first indexer for now
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
		profiles := make(map[string][]string)
		for name, p := range cfg.Quality.Profiles {
			profiles[name] = p.Accept
		}
		scorer := search.NewScorer(profiles)
		searcher = search.NewSearcher(prowlarrClient, scorer)
	}

	// Create importer
	imp := importer.New(db, importer.Config{
		MovieRoot:      cfg.Libraries.Movies.Root,
		SeriesRoot:     cfg.Libraries.Series.Root,
		MovieTemplate:  cfg.Libraries.Movies.Naming,
		SeriesTemplate: cfg.Libraries.Series.Naming,
		PlexURL:        plexURLFromConfig(cfg),
		PlexToken:      plexTokenFromConfig(cfg),
	})

	// === Background Jobs ===
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if downloadManager != nil {
		go runPoller(ctx, downloadManager, imp, downloadStore)
	}

	// === HTTP Setup ===
	mux := http.NewServeMux()

	// Build quality profiles map for API
	profiles := make(map[string][]string)
	for name, p := range cfg.Quality.Profiles {
		profiles[name] = p.Accept
	}

	// Native API v1
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
		// Build quality profile ID map (sorted for deterministic IDs across restarts)
		names := make([]string, 0, len(cfg.Quality.Profiles))
		for name := range cfg.Quality.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		profileIDs := make(map[string]int)
		for i, name := range names {
			profileIDs[name] = i + 1
		}

		compatCfg := compat.Config{
			APIKey:          cfg.Compat.APIKey,
			MovieRoot:       cfg.Libraries.Movies.Root,
			SeriesRoot:      cfg.Libraries.Series.Root,
			QualityProfiles: profileIDs,
		}
		apiCompat := compat.New(compatCfg, libraryStore, downloadStore)
		apiCompat.SetSearcher(searcher)
		apiCompat.SetManager(downloadManager)
		apiCompat.RegisterRoutes(mux)
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("arrgo listening on %s\n", addr)
	fmt.Printf("  database: %s\n", cfg.Database.Path)
	fmt.Printf("  sabnzbd: %v\n", sabClient != nil)
	fmt.Printf("  indexers: %d configured\n", len(cfg.Indexers))
	fmt.Printf("  plex: %v\n", plexClient != nil)

	// Silence unused variable warnings for stores not yet wired up
	_ = historyStore

	// === HTTP Server ===
	srv := &http.Server{Addr: addr, Handler: mux}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	fmt.Printf("\nreceived %v, shutting down...\n", sig)

	// Cancel background jobs (this stops the poller)
	cancel()

	// Graceful HTTP shutdown with 30s timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	fmt.Println("arrgo stopped")
	return nil
}

func plexURLFromConfig(cfg *config.Config) string {
	if cfg.Notifications.Plex != nil {
		return cfg.Notifications.Plex.URL
	}
	return ""
}

func plexTokenFromConfig(cfg *config.Config) string {
	if cfg.Notifications.Plex != nil {
		return cfg.Notifications.Plex.Token
	}
	return ""
}

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
	status := download.StatusCompleted
	completed, err := store.List(download.DownloadFilter{Status: &status})
	if err != nil {
		fmt.Printf("poller: list error: %v\n", err)
		return
	}

	// Import each one
	for _, dl := range completed {
		clientStatus, err := manager.Client().Status(ctx, dl.ClientID)
		if err != nil || clientStatus == nil || clientStatus.Path == "" {
			continue
		}

		fmt.Printf("poller: importing download %d from %s\n", dl.ID, clientStatus.Path)

		if _, err := imp.Import(ctx, dl.ID, clientStatus.Path); err != nil {
			fmt.Printf("poller: import %d failed: %v\n", dl.ID, err)
			continue
		}

		dl.Status = download.StatusImported
		if err := store.Update(dl); err != nil {
			fmt.Printf("poller: update %d failed: %v\n", dl.ID, err)
		}
	}
}
