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

	_ "modernc.org/sqlite"

	"github.com/vmunix/arrgo/internal/api/compat"
	v1 "github.com/vmunix/arrgo/internal/api/v1"
	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/migrations"
	"github.com/vmunix/arrgo/internal/search"
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

	if downloadManager != nil {
		go runPoller(ctx, downloadManager, imp, downloadStore, cfg.Downloaders.SABnzbd, &cfg.Importer, plexClient, libraryStore, logger.With("component", "poller"))
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
	}, v1.Config{
		MovieRoot:       cfg.Libraries.Movies.Root,
		SeriesRoot:      cfg.Libraries.Series.Root,
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
		apiCompat := compat.New(compatCfg, libraryStore, downloadStore)
		apiCompat.SetSearcher(searcher)
		apiCompat.SetManager(downloadManager)

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
	completed, err := store.List(download.Filter{Status: &status})
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

		// Set status to importing before starting the copy
		if err := store.Transition(dl, download.StatusImporting); err != nil {
			log.Error("transition to importing failed", "download_id", dl.ID, "error", err)
			continue
		}

		log.Info("importing download", "download_id", dl.ID, "path", importPath)

		if _, err := imp.Import(ctx, dl.ID, importPath); err != nil {
			log.Error("import failed", "download_id", dl.ID, "error", err)
			// Transition to failed on import error
			if transErr := store.Transition(dl, download.StatusFailed); transErr != nil {
				log.Error("transition to failed error", "download_id", dl.ID, "error", transErr)
			}
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
	imported, err := store.List(download.Filter{Status: &status})
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

		sourcePath := translatePath(clientStatus.Path, sabCfg)

		// Validate the cleanup path against our internal knowledge
		downloadRoot := ""
		if sabCfg != nil {
			downloadRoot = sabCfg.LocalPath
		}
		validation := validateCleanupPath(sourcePath, downloadRoot, dl.ReleaseName, log)

		if !validation.Safe {
			log.Warn("cleanup path validation failed",
				"download_id", dl.ID,
				"path", sourcePath,
				"expected_release", dl.ReleaseName,
				"reason", validation.Reason)
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
			continue
		}

		log.Debug("cleanup path validated",
			"download_id", dl.ID,
			"source_dir", validation.SourceDir,
			"source_file", validation.SourceFile,
			"is_directory", validation.IsDirectory)

		sourceDir := validation.SourceDir
		sourceFile := validation.SourceFile

		// Cleanup: if SABnzbd gave us a directory, remove files inside then directory if empty
		// If it gave us a file, remove file then directory if empty
		if sourceFile == "" {
			// Directory mode: remove known media/download files, then directory if empty
			entries, err := os.ReadDir(sourceDir)
			if err != nil {
				log.Error("cannot read source dir", "download_id", dl.ID, "path", sourceDir, "error", err)
			} else {
				for _, entry := range entries {
					entryPath := filepath.Join(sourceDir, entry.Name())
					if entry.IsDir() {
						// Don't recursively delete subdirs - leave them
						log.Debug("leaving subdir", "download_id", dl.ID, "path", entryPath)
						continue
					}
					// Delete individual files
					if err := os.Remove(entryPath); err != nil && !os.IsNotExist(err) {
						log.Warn("cleanup file failed", "download_id", dl.ID, "path", entryPath, "error", err)
					} else {
						log.Debug("cleaned up file", "download_id", dl.ID, "path", entryPath)
					}
				}
				// Remove directory only if now empty
				entries, err = os.ReadDir(sourceDir)
				if err == nil && len(entries) == 0 {
					if err := os.Remove(sourceDir); err != nil {
						log.Warn("cleanup dir failed", "download_id", dl.ID, "path", sourceDir, "error", err)
					} else {
						log.Info("cleaned up source dir", "download_id", dl.ID, "path", sourceDir)
					}
				} else if err == nil && len(entries) > 0 {
					log.Info("source dir not empty after cleanup, leaving", "download_id", dl.ID, "path", sourceDir, "remaining", len(entries))
				}
			}
		} else {
			// File mode: two-stage cleanup
			if err := os.Remove(sourceFile); err != nil && !os.IsNotExist(err) {
				log.Error("cleanup file failed", "download_id", dl.ID, "path", sourceFile, "error", err)
			} else {
				log.Info("cleaned up source file", "download_id", dl.ID, "path", sourceFile)

				// Only remove directory if it's now empty
				entries, err := os.ReadDir(sourceDir)
				if err == nil && len(entries) == 0 {
					if err := os.Remove(sourceDir); err != nil {
						log.Warn("cleanup dir failed", "download_id", dl.ID, "path", sourceDir, "error", err)
					} else {
						log.Info("cleaned up empty source dir", "download_id", dl.ID, "path", sourceDir)
					}
				} else if err == nil && len(entries) > 0 {
					log.Info("source dir not empty, leaving", "download_id", dl.ID, "path", sourceDir, "remaining", len(entries))
				}
			}
		}

		if err := store.Transition(dl, download.StatusCleaned); err != nil {
			log.Error("transition failed", "download_id", dl.ID, "error", err)
		}
	}
}

// cleanupPathValidation contains the result of validating a cleanup path.
type cleanupPathValidation struct {
	Safe        bool
	SourceDir   string
	SourceFile  string // Empty if SABnzbd returned a directory
	Reason      string // Why it's not safe (if Safe is false)
	IsDirectory bool   // True if SABnzbd returned a directory path
}

// validateCleanupPath checks if a path from the download client is safe to clean up.
// It cross-references the path against our internal knowledge:
// - The download root from config
// - The expected release name from our database
// - Path structure expectations (depth, format)
func validateCleanupPath(clientPath, downloadRoot, expectedRelease string, log *slog.Logger) cleanupPathValidation {
	result := cleanupPathValidation{}

	// Check 1: Download root must be configured
	if downloadRoot == "" {
		result.Reason = "no download root configured"
		return result
	}

	// Normalize paths
	downloadRoot = filepath.Clean(downloadRoot)
	clientPath = filepath.Clean(clientPath)

	// Check 2: Path must be under download root
	if !strings.HasPrefix(clientPath, downloadRoot+string(filepath.Separator)) && clientPath != downloadRoot {
		result.Reason = fmt.Sprintf("path %q not under download root %q", clientPath, downloadRoot)
		return result
	}

	// Check 3: Path must not BE the download root
	if clientPath == downloadRoot {
		result.Reason = "path is the download root itself"
		return result
	}

	// Determine the release directory (should be one level under root)
	relPath, err := filepath.Rel(downloadRoot, clientPath)
	if err != nil {
		result.Reason = fmt.Sprintf("cannot compute relative path: %v", err)
		return result
	}

	// Check 4: Path should be at most one level deep (release dir or file within it)
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) == 0 {
		result.Reason = "invalid relative path"
		return result
	}
	if len(parts) > 2 {
		result.Reason = fmt.Sprintf("path too deep: %d levels under root (expected 1-2)", len(parts))
		return result
	}

	// The first part should be the release directory name
	releaseDirName := parts[0]

	// Check 5: Release directory name should match expected release
	// Use case-insensitive comparison and allow for minor variations
	if expectedRelease != "" {
		// Normalize for comparison: lowercase, remove common separators
		normalizeForCompare := func(s string) string {
			s = strings.ToLower(s)
			s = strings.ReplaceAll(s, ".", " ")
			s = strings.ReplaceAll(s, "-", " ")
			s = strings.ReplaceAll(s, "_", " ")
			return strings.Join(strings.Fields(s), " ")
		}

		normalizedDir := normalizeForCompare(releaseDirName)
		normalizedExpected := normalizeForCompare(expectedRelease)

		// Check if the directory name contains the expected release name
		// or vice versa (to handle slight naming differences)
		if !strings.Contains(normalizedDir, normalizedExpected) &&
			!strings.Contains(normalizedExpected, normalizedDir) &&
			normalizedDir != normalizedExpected {
			log.Warn("release name mismatch",
				"dir_name", releaseDirName,
				"expected", expectedRelease,
				"normalized_dir", normalizedDir,
				"normalized_expected", normalizedExpected)
			result.Reason = fmt.Sprintf("release name mismatch: dir=%q expected=%q", releaseDirName, expectedRelease)
			return result
		}
	}

	// Determine if it's a file or directory
	info, err := os.Stat(clientPath)
	if err != nil {
		// Path doesn't exist - might already be cleaned
		result.Reason = fmt.Sprintf("path does not exist: %v", err)
		return result
	}

	result.IsDirectory = info.IsDir()
	if result.IsDirectory {
		result.SourceDir = clientPath
		result.SourceFile = ""
	} else {
		result.SourceFile = clientPath
		result.SourceDir = filepath.Dir(clientPath)
	}

	// Final check: the source directory should be exactly one level under root
	sourceDirRel, _ := filepath.Rel(downloadRoot, result.SourceDir)
	if strings.Contains(sourceDirRel, string(filepath.Separator)) {
		result.Reason = fmt.Sprintf("source dir %q is nested too deep", result.SourceDir)
		return result
	}

	result.Safe = true
	return result
}
