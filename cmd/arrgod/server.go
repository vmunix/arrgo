package main

import (
	"context"
	"database/sql"
	"errors"
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

	_ "modernc.org/sqlite"

	"github.com/vmunix/arrgo/internal/adapters/plex"
	"github.com/vmunix/arrgo/internal/api/compat"
	v1 "github.com/vmunix/arrgo/internal/api/v1"
	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/migrations"
	"github.com/vmunix/arrgo/internal/search"
	"github.com/vmunix/arrgo/internal/server"
	"github.com/vmunix/arrgo/internal/tmdb"
	"github.com/vmunix/arrgo/pkg/newznab"
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
	db, err := sql.Open("sqlite", cfg.Database.Path)
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
	// Run migration 005 - events table
	if _, err := db.Exec(migrations.Migration005Events); err != nil {
		// Ignore "table already exists" for idempotent migrations
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("migrate 005: %w", err)
		}
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
			logger,
		)
	}

	// Create Newznab clients for all configured indexers
	newznabClients := make([]*newznab.Client, 0, len(cfg.Indexers))
	for name, indexer := range cfg.Indexers {
		newznabClients = append(newznabClients, newznab.NewClient(name, indexer.URL, indexer.APIKey, logger))
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
			logger,
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
		MovieRoot:      cfg.Libraries.Movies.Root,
		SeriesRoot:     cfg.Libraries.Series.Root,
		MovieTemplate:  cfg.Libraries.Movies.Naming,
		SeriesTemplate: cfg.Libraries.Series.Naming,
		PlexURL:        plexURLFromConfig(cfg),
		PlexToken:      plexTokenFromConfig(cfg),
		PlexLocalPath:  plexLocalPathFromConfig(cfg),
		PlexRemotePath: plexRemotePathFromConfig(cfg),
	}, logger.With("component", "importer"))

	// === Background Jobs ===
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// === Event-Driven Runner ===
	var eventBus *events.Bus

	if sabClient != nil {
		// Create plex checker adapter if plex is configured
		var plexChecker plex.Checker
		if plexClient != nil {
			plexChecker = &plexCheckerAdapter{client: plexClient, lib: libraryStore}
		}

		runner := server.NewRunner(db, server.Config{
			SABnzbdPollInterval: sabPollInterval(cfg),
			PlexPollInterval:    plexPollInterval(cfg),
			DownloadRoot:        sabDownloadRoot(cfg),
			DownloadRemotePath:  sabRemotePath(cfg),
			DownloadLocalPath:   sabLocalPath(cfg),
			CleanupEnabled:      cfg.Importer.ShouldCleanupSource(),
		}, logger, sabClient, imp, plexChecker)

		eventBus = runner.Start()
		go func() {
			if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("runner error", "error", err)
			}
		}()
	}

	// === HTTP Setup ===
	mux := http.NewServeMux()

	// Build quality profiles map for API
	profiles := make(map[string][]string)
	for name := range cfg.Quality.Profiles {
		profiles[name] = cfg.Quality.Profiles[name].Resolution
	}

	// Native API v1
	apiV1, err := v1.NewWithDeps(v1.ServerDeps{
		Library:   libraryStore,
		Downloads: downloadStore,
		History:   historyStore,
		Searcher:  searcher,
		Manager:   downloadManager,
		Plex:      plexClient,
		Importer:  imp,
		Bus:       eventBus,
	}, v1.Config{
		MovieRoot:       cfg.Libraries.Movies.Root,
		SeriesRoot:      cfg.Libraries.Series.Root,
		DownloadRoot:    sabDownloadRoot(cfg),
		QualityProfiles: profiles,
	})
	if err != nil {
		return fmt.Errorf("create api: %w", err)
	}
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
		apiCompat := compat.New(compatCfg, libraryStore, downloadStore, logger.With("component", "compat"))
		apiCompat.SetSearcher(searcher)
		apiCompat.SetManager(downloadManager)
		if eventBus != nil {
			apiCompat.SetBus(eventBus)
		}

		// Wire TMDB client if configured
		if cfg.TMDB != nil && cfg.TMDB.APIKey != "" {
			tmdbClient := tmdb.NewClient(cfg.TMDB.APIKey, tmdb.WithLogger(logger))
			apiCompat.SetTMDB(tmdbClient)
			logger.Info("TMDB client configured")
		}

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

	// === HTTP Server ===
	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux, logger),
		ReadHeaderTimeout: 10 * time.Second, // Prevent Slowloris attacks
	}

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

// sabDownloadRoot returns the local path for SABnzbd downloads.
func sabDownloadRoot(cfg *config.Config) string {
	if cfg.Downloaders.SABnzbd != nil && cfg.Downloaders.SABnzbd.LocalPath != "" {
		return cfg.Downloaders.SABnzbd.LocalPath
	}
	return ""
}

// sabRemotePath returns the remote path prefix as seen by SABnzbd.
func sabRemotePath(cfg *config.Config) string {
	if cfg.Downloaders.SABnzbd != nil {
		return cfg.Downloaders.SABnzbd.RemotePath
	}
	return ""
}

// sabLocalPath returns the local path prefix for SABnzbd downloads.
func sabLocalPath(cfg *config.Config) string {
	if cfg.Downloaders.SABnzbd != nil {
		return cfg.Downloaders.SABnzbd.LocalPath
	}
	return ""
}

// sabPollInterval returns the SABnzbd poll interval, defaulting to 5 seconds.
func sabPollInterval(cfg *config.Config) time.Duration {
	if cfg.Downloaders.SABnzbd != nil && cfg.Downloaders.SABnzbd.PollInterval > 0 {
		return cfg.Downloaders.SABnzbd.PollInterval
	}
	return 5 * time.Second
}

// plexPollInterval returns the Plex poll interval, defaulting to 60 seconds.
func plexPollInterval(cfg *config.Config) time.Duration {
	if cfg.Notifications.Plex != nil && cfg.Notifications.Plex.PollInterval > 0 {
		return cfg.Notifications.Plex.PollInterval
	}
	return 60 * time.Second
}

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
