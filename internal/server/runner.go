// Package server provides the event-driven server components.
package server

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
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
}

// NewRunner creates a new runner.
func NewRunner(db *sql.DB, cfg Config, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		db:     db,
		config: cfg,
		logger: logger,
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
	_ = download.NewStore(r.db)

	// Use errgroup to manage component lifecycle
	g, ctx := errgroup.WithContext(ctx)

	// Skeleton: wait for context cancellation
	// Full wiring requires more dependencies (download client, importer, plex client)
	// that will be added when integrating with the existing server.
	g.Go(func() error {
		<-ctx.Done()
		return ctx.Err()
	})

	return g.Wait()
}
