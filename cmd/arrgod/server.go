package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
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
	"github.com/arrgo/arrgo/pkg/newznab"
)

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
	if r.status == 200 { // Only capture first WriteHeader call
		r.status = code
	}
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

func runServer(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	}))

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
	defer func() { _ = db.Close() }()

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

	// Create Newznab clients for all configured indexers
	var newznabClients []*newznab.Client
	for name, indexer := range cfg.Indexers {
		newznabClients = append(newznabClients, newznab.NewClient(name, indexer.URL, indexer.APIKey))
	}
	var indexerPool *search.IndexerPool
	if len(newznabClients) > 0 {
		indexerPool = search.NewIndexerPool(newznabClients, logger.With("component", "indexerpool"))
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
		downloadManager = download.NewManager(sabClient, downloadStore, logger.With("component", "download"))
	}

	var searcher *search.Searcher
	if indexerPool != nil {
		profiles := make(map[string][]string)
		for name, p := range cfg.Quality.Profiles {
			profiles[name] = p.Resolution
		}
		scorer := search.NewScorer(profiles)
		searcher = search.NewSearcher(indexerPool, scorer, logger.With("component", "search"))
	}

	// Create importer
	imp := importer.New(db, importer.Config{
		MovieRoot:      cfg.Libraries.Movies.Root,
		SeriesRoot:     cfg.Libraries.Series.Root,
		MovieTemplate:  cfg.Libraries.Movies.Naming,
		SeriesTemplate: cfg.Libraries.Series.Naming,
		PlexURL:        plexURLFromConfig(cfg),
		PlexToken:      plexTokenFromConfig(cfg),
	}, logger.With("component", "importer"))

	// === Background Jobs ===
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if downloadManager != nil {
		go runPoller(ctx, downloadManager, imp, downloadStore, logger.With("component", "poller"))
	}

	// === HTTP Setup ===
	mux := http.NewServeMux()

	// Build quality profiles map for API
	profiles := make(map[string][]string)
	for name, p := range cfg.Quality.Profiles {
		profiles[name] = p.Resolution
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
	logger.Info("server starting",
		"addr", addr,
		"database", cfg.Database.Path,
		"sabnzbd", sabClient != nil,
		"indexers", len(cfg.Indexers),
		"plex", plexClient != nil,
		"log_level", cfg.Server.LogLevel,
	)

	// Silence unused variable warnings for stores not yet wired up
	_ = historyStore

	// === HTTP Server ===
	srv := &http.Server{Addr: addr, Handler: logRequests(mux, logger)}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down", "signal", sig.String())

	// Cancel background jobs (this stops the poller)
	cancel()

	// Graceful HTTP shutdown with 30s timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	logger.Info("server stopped")
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

func runPoller(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Info("poller started", "interval", "30s")

	for {
		select {
		case <-ctx.Done():
			log.Info("poller stopped")
			return
		case <-ticker.C:
			poll(ctx, manager, imp, store, log)
		}
	}
}

func poll(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, log *slog.Logger) {
	// Refresh download statuses from client
	if err := manager.Refresh(ctx); err != nil {
		log.Error("refresh failed", "error", err)
	}

	// Find completed downloads
	status := download.StatusCompleted
	completed, err := store.List(download.DownloadFilter{Status: &status})
	if err != nil {
		log.Error("list failed", "error", err)
		return
	}

	// Import each one
	for _, dl := range completed {
		clientStatus, err := manager.Client().Status(ctx, dl.ClientID)
		if err != nil || clientStatus == nil || clientStatus.Path == "" {
			continue
		}

		log.Info("importing download", "download_id", dl.ID, "path", clientStatus.Path)

		if _, err := imp.Import(ctx, dl.ID, clientStatus.Path); err != nil {
			log.Error("import failed", "download_id", dl.ID, "error", err)
			continue
		}

		dl.Status = download.StatusImported
		if err := store.Update(dl); err != nil {
			log.Error("update failed", "download_id", dl.ID, "error", err)
		}
	}
}
