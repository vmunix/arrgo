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
		return fmt.Errorf("migrate 001: %w", err)
	}
	// Run migration 002 (ignore "duplicate column" error for already-migrated DBs)
	if _, err := db.Exec(migrations.Migration002LastTransitionAt); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate 002: %w", err)
		}
	}
	// Run migration 003 - adds 'cleaned' to status CHECK constraint
	if _, err := db.Exec(migrations.Migration003DownloadsStatusCleaned); err != nil {
		return fmt.Errorf("migrate 003: %w", err)
	}

	// === Stores (always created) ===
	libraryStore := library.NewStore(db)
	downloadStore := download.NewStore(db)
	historyStore := importer.NewHistoryStore(db)

	// Log all state transitions
	downloadStore.OnTransition(func(e download.TransitionEvent) {
		logger.Info("download status changed",
			"download_id", e.DownloadID,
			"from", e.From,
			"to", e.To,
		)
	})

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
		scorer := search.NewScorer(cfg.Quality.Profiles)
		searcher = search.NewSearcher(indexerPool, scorer, logger.With("component", "search"))
	}

	// Create importer
	imp := importer.New(db, importer.Config{
		MovieRoot:       cfg.Libraries.Movies.Root,
		SeriesRoot:      cfg.Libraries.Series.Root,
		MovieTemplate:   cfg.Libraries.Movies.Naming,
		SeriesTemplate:  cfg.Libraries.Series.Naming,
		PlexURL:         plexURLFromConfig(cfg),
		PlexToken:       plexTokenFromConfig(cfg),
		PlexLocalPath:   plexLocalPathFromConfig(cfg),
		PlexRemotePath:  plexRemotePathFromConfig(cfg),
	}, logger.With("component", "importer"))

	// === Background Jobs ===
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if downloadManager != nil {
		go runPoller(ctx, downloadManager, imp, downloadStore, cfg.Downloaders.SABnzbd, &cfg.Importer, plexClient, libraryStore, logger.With("component", "poller"))
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
	apiV1.SetImporter(imp)
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

func plexLocalPathFromConfig(cfg *config.Config) string {
	if cfg.Notifications.Plex != nil {
		return cfg.Notifications.Plex.LocalPath
	}
	return ""
}

func plexRemotePathFromConfig(cfg *config.Config) string {
	if cfg.Notifications.Plex != nil {
		return cfg.Notifications.Plex.RemotePath
	}
	return ""
}

func runPoller(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, sabCfg *config.SABnzbdConfig, impCfg *config.ImporterConfig, plex *importer.PlexClient, lib *library.Store, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Info("poller started", "interval", "30s")

	for {
		select {
		case <-ctx.Done():
			log.Info("poller stopped")
			return
		case <-ticker.C:
			poll(ctx, manager, imp, store, sabCfg, impCfg, plex, lib, log)
		}
	}
}

func poll(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, sabCfg *config.SABnzbdConfig, impCfg *config.ImporterConfig, plex *importer.PlexClient, lib *library.Store, log *slog.Logger) {
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

		// Translate remote path to local path if configured
		importPath := translatePath(clientStatus.Path, sabCfg)

		log.Info("importing download", "download_id", dl.ID, "path", importPath)

		if _, err := imp.Import(ctx, dl.ID, importPath); err != nil {
			log.Error("import failed", "download_id", dl.ID, "error", err)
			continue
		}

		if err := store.Transition(dl, download.StatusImported); err != nil {
			log.Error("transition failed", "download_id", dl.ID, "error", err)
		}
	}

	// Process imported downloads -> verify in Plex and cleanup
	if impCfg.ShouldCleanupSource() {
		processImportedDownloads(ctx, store, manager, plex, lib, sabCfg, log)
	}
}

// translatePath converts a remote path (as seen by the download client) to a local path.
func translatePath(path string, sabCfg *config.SABnzbdConfig) string {
	if sabCfg == nil || sabCfg.RemotePath == "" || sabCfg.LocalPath == "" {
		return path
	}
	if strings.HasPrefix(path, sabCfg.RemotePath) {
		return sabCfg.LocalPath + path[len(sabCfg.RemotePath):]
	}
	return path
}

// processImportedDownloads verifies imported content in Plex and cleans up source files.
func processImportedDownloads(ctx context.Context, store *download.Store, manager *download.Manager, plex *importer.PlexClient, lib *library.Store, sabCfg *config.SABnzbdConfig, log *slog.Logger) {
	status := download.StatusImported
	imported, err := store.List(download.DownloadFilter{Status: &status})
	if err != nil {
		log.Error("list imported failed", "error", err)
		return
	}

	if plex == nil {
		// No Plex configured - transition directly to cleaned without verification
		for _, dl := range imported {
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
		}
		return
	}

	for _, dl := range imported {
		// Get content info
		content, err := lib.GetContent(dl.ContentID)
		if err != nil {
			log.Error("get content failed", "download_id", dl.ID, "error", err)
			continue
		}

		// Check if Plex has it
		found, err := plex.HasMovie(ctx, content.Title, content.Year)
		if err != nil {
			log.Warn("plex check failed", "download_id", dl.ID, "error", err)
			continue
		}

		if !found {
			log.Debug("waiting for Plex to index", "title", content.Title, "year", content.Year)
			continue
		}

		// Get source path for cleanup
		clientStatus, err := manager.Client().Status(ctx, dl.ClientID)
		if err != nil || clientStatus == nil || clientStatus.Path == "" {
			// Can't determine source path - just transition without cleanup
			log.Warn("cannot determine source path for cleanup", "download_id", dl.ID)
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
			continue
		}

		sourceFile := translatePath(clientStatus.Path, sabCfg)
		sourceDir := filepath.Dir(sourceFile)

		// Safety checks:
		// 1. Path must be under download root
		// 2. Directory must not BE the download root (don't delete everything!)
		if sabCfg == nil || sabCfg.LocalPath == "" {
			log.Warn("no download root configured, skipping cleanup", "download_id", dl.ID)
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
			continue
		}
		if !strings.HasPrefix(sourceDir, sabCfg.LocalPath) || sourceDir == sabCfg.LocalPath {
			log.Warn("path not safe for cleanup", "download_id", dl.ID, "path", sourceDir, "root", sabCfg.LocalPath)
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
			continue
		}

		// Delete source directory (contains the downloaded file)
		if err := os.RemoveAll(sourceDir); err != nil {
			log.Error("cleanup failed", "download_id", dl.ID, "path", sourceDir, "error", err)
			// Continue to mark as cleaned anyway - cleanup is best effort
		} else {
			log.Info("cleaned up source", "download_id", dl.ID, "path", sourceDir)
		}

		if err := store.Transition(dl, download.StatusCleaned); err != nil {
			log.Error("transition failed", "download_id", dl.ID, "error", err)
		}
	}
}
