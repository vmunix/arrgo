// Package server provides the event-driven server components.
package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/internal/adapters/plex"
	"github.com/vmunix/arrgo/internal/adapters/sabnzbd"
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/handlers"
	"golang.org/x/sync/errgroup"
)

// Config for the event-driven server.
type Config struct {
	PollInterval   time.Duration
	DownloadRoot   string
	CleanupEnabled bool
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
	bus      *events.Bus
	eventLog *events.EventLog
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
func (r *Runner) Start() *events.Bus {
	r.eventLog = events.NewEventLog(r.db)
	r.bus = events.NewBus(r.eventLog, r.logger.With("component", "bus"))
	return r.bus
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

	// Create handlers
	downloadHandler := handlers.NewDownloadHandler(r.bus, downloadStore, r.downloader, r.logger.With("handler", "download"))
	importHandler := handlers.NewImportHandler(r.bus, downloadStore, r.importer, r.logger.With("handler", "import"))
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
	sabnzbdAdapter := sabnzbd.New(r.bus, r.downloader, downloadStore, r.config.PollInterval, r.logger.With("adapter", "sabnzbd"))

	// Start adapters
	g.Go(func() error {
		r.logger.Info("starting sabnzbd adapter", "interval", r.config.PollInterval)
		return sabnzbdAdapter.Start(ctx)
	})

	// Only start Plex adapter if configured
	if r.plexChecker != nil {
		plexAdapter := plex.New(r.bus, r.plexChecker, r.config.PollInterval, r.logger.With("adapter", "plex"))
		g.Go(func() error {
			r.logger.Info("starting plex adapter", "interval", r.config.PollInterval)
			return plexAdapter.Start(ctx)
		})
	}

	return g.Wait()
}
