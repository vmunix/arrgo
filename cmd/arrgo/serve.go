package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

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
		profiles := make(map[string][]string)
		for name, p := range cfg.Quality.Profiles {
			profiles[name] = p.Accept
		}
		scorer := search.NewScorer(profiles)
		searcher = search.NewSearcher(prowlarrClient, scorer)
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
		apiCompat := compat.New(cfg.Compat.APIKey)
		apiCompat.RegisterRoutes(mux)
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("arrgo listening on %s\n", addr)
	fmt.Printf("  database: %s\n", cfg.Database.Path)
	fmt.Printf("  sabnzbd: %v\n", sabClient != nil)
	fmt.Printf("  prowlarr: %v\n", prowlarrClient != nil)
	fmt.Printf("  plex: %v\n", plexClient != nil)

	// Silence unused variable warnings for stores not yet wired up
	_ = libraryStore
	_ = historyStore

	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}
