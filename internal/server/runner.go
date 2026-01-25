// Package server provides the event-driven server components.
package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/vmunix/arrgo/internal/adapters/plex"
	"github.com/vmunix/arrgo/internal/adapters/sabnzbd"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/handlers"
	"github.com/vmunix/arrgo/internal/library"
	"golang.org/x/sync/errgroup"
)

// Config for the event-driven server.
type Config struct {
	SABnzbdPollInterval time.Duration // How often to poll SABnzbd (default: 5s)
	PlexPollInterval    time.Duration // How often to poll Plex (default: 60s)
	DownloadRoot        string
	DownloadRemotePath  string // Path prefix as seen by SABnzbd
	DownloadLocalPath   string // Local path prefix
	CleanupEnabled      bool
}

// Runner manages the event-driven components.
type Runner struct {
	db     *sql.DB
	config Config
	logger *slog.Logger

	// Dependencies
	downloader  download.Downloader
	importer    handlers.FileImporter
	plexChecker plex.Checker // Can be nil if Plex not configured

	// Runtime state
	startOnce sync.Once
	bus       *events.Bus
	eventLog  *events.EventLog
}

// NewRunner creates a new runner.
func NewRunner(db *sql.DB, cfg Config, logger *slog.Logger, downloader download.Downloader, importer handlers.FileImporter, plexChecker plex.Checker) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		db:          db,
		config:      cfg,
		logger:      logger,
		downloader:  downloader,
		importer:    importer,
		plexChecker: plexChecker,
	}
}

// Start initializes the runner and returns the event bus.
// Call Run() after Start() to begin processing.
// Safe to call from multiple goroutines; initialization happens only once.
func (r *Runner) Start() *events.Bus {
	r.startOnce.Do(func() {
		r.eventLog = events.NewEventLog(r.db)
		r.bus = events.NewBus(r.eventLog, r.logger.With("component", "bus"))
	})
	return r.bus
}

// EventLog returns the event log. Must call Start() first.
func (r *Runner) EventLog() *events.EventLog {
	return r.eventLog
}

// Run starts all event-driven components.
// Must call Start() before Run().
func (r *Runner) Run(ctx context.Context) error {
	if r.bus == nil {
		return errors.New("must call Start() before Run()")
	}
	defer r.bus.Close()

	// Create stores
	downloadStore := download.NewStore(r.db)
	libraryStore := library.NewStore(r.db)

	// Create handlers
	downloadHandler := handlers.NewDownloadHandler(r.bus, downloadStore, libraryStore, r.downloader, r.logger.With("handler", "download"))
	importHandler := handlers.NewImportHandler(r.bus, downloadStore, libraryStore, r.importer, r.logger.With("handler", "import"))
	cleanupHandler := handlers.NewCleanupHandler(r.bus, downloadStore, handlers.CleanupConfig{
		DownloadRoot: r.config.DownloadRoot,
		Enabled:      r.config.CleanupEnabled,
	}, r.logger.With("handler", "cleanup"))

	// Use errgroup to manage component lifecycle
	g, ctx := errgroup.WithContext(ctx)

	// Start handlers
	g.Go(func() error {
		r.logger.Info("starting download handler")
		return downloadHandler.Start(ctx)
	})
	g.Go(func() error {
		r.logger.Info("starting import handler")
		return importHandler.Start(ctx)
	})
	g.Go(func() error {
		r.logger.Info("starting cleanup handler")
		return cleanupHandler.Start(ctx)
	})

	// Create adapters
	sabnzbdAdapter := sabnzbd.New(r.bus, r.downloader, downloadStore, sabnzbd.Config{
		Interval:   r.config.SABnzbdPollInterval,
		RemotePath: r.config.DownloadRemotePath,
		LocalPath:  r.config.DownloadLocalPath,
	}, r.logger.With("adapter", "sabnzbd"))

	// Start adapters
	g.Go(func() error {
		r.logger.Info("starting sabnzbd adapter", "interval", r.config.SABnzbdPollInterval)
		return sabnzbdAdapter.Start(ctx)
	})

	// Only start Plex adapter if configured
	if r.plexChecker != nil {
		plexAdapter := plex.New(r.bus, r.plexChecker, downloadStore, r.config.PlexPollInterval, r.logger.With("adapter", "plex"))
		g.Go(func() error {
			r.logger.Info("starting plex adapter", "interval", r.config.PlexPollInterval)
			return plexAdapter.Start(ctx)
		})
	}

	// Event log pruning (every 24 hours, keep 90 days)
	g.Go(func() error {
		// Prune on startup
		if pruned, err := r.eventLog.Prune(90 * 24 * time.Hour); err != nil {
			r.logger.Error("failed to prune event log on startup", "error", err)
		} else if pruned > 0 {
			r.logger.Info("pruned old events on startup", "count", pruned)
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				pruned, err := r.eventLog.Prune(90 * 24 * time.Hour)
				if err != nil {
					r.logger.Error("failed to prune event log", "error", err)
				} else if pruned > 0 {
					r.logger.Info("pruned old events", "count", pruned)
				}
			}
		}
	})

	return g.Wait()
}
