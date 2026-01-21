// Package server provides the event-driven server components.
package server

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

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
	downloader download.Downloader
	importer   handlers.FileImporter
}

// NewRunner creates a new runner.
func NewRunner(db *sql.DB, cfg Config, logger *slog.Logger, downloader download.Downloader, importer handlers.FileImporter) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		db:         db,
		config:     cfg,
		logger:     logger,
		downloader: downloader,
		importer:   importer,
	}
}

// Run starts all event-driven components.
// It blocks until the context is canceled or an error occurs.
func (r *Runner) Run(ctx context.Context) error {
	// Create event bus with persistence
	eventLog := events.NewEventLog(r.db)
	bus := events.NewBus(eventLog, r.logger.With("component", "bus"))
	defer bus.Close()

	// Create stores
	downloadStore := download.NewStore(r.db)

	// Create handlers
	downloadHandler := handlers.NewDownloadHandler(bus, downloadStore, r.downloader, r.logger.With("handler", "download"))
	importHandler := handlers.NewImportHandler(bus, downloadStore, r.importer, r.logger.With("handler", "import"))
	cleanupHandler := handlers.NewCleanupHandler(bus, downloadStore, handlers.CleanupConfig{
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

	return g.Wait()
}
