// Package plex provides an adapter that polls Plex to verify imported content.
package plex

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/vmunix/arrgo/internal/events"
)

// Checker checks if content exists in Plex.
type Checker interface {
	// HasContentByID checks if content with the given ID exists in Plex.
	// Returns (found, plexKey, error).
	HasContentByID(ctx context.Context, contentID int64) (bool, string, error)
}

// pendingVerification tracks content waiting to appear in Plex.
type pendingVerification struct {
	contentID  int64
	downloadID int64
	filePath   string
	addedAt    time.Time
}

// Adapter polls Plex and emits events when imported content is detected.
type Adapter struct {
	client   Checker
	bus      *events.Bus
	interval time.Duration
	logger   *slog.Logger

	mu      sync.RWMutex
	pending map[int64]*pendingVerification // contentID -> pending
}

// New creates a new Plex adapter.
func New(bus *events.Bus, client Checker, interval time.Duration, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		client:   client,
		bus:      bus,
		interval: interval,
		logger:   logger.With("component", "plex-adapter"),
		pending:  make(map[int64]*pendingVerification),
	}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "plex"
}

// Start begins listening for ImportCompleted events and polling Plex.
// It runs until the context is canceled.
func (a *Adapter) Start(ctx context.Context) error {
	// Subscribe to ImportCompleted events
	importCh := a.bus.Subscribe(events.EventImportCompleted, 100)

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case evt, ok := <-importCh:
			if !ok {
				return nil
			}
			a.handleImportCompleted(evt)

		case <-ticker.C:
			a.checkPending(ctx)
		}
	}
}

// handleImportCompleted registers a pending verification.
func (a *Adapter) handleImportCompleted(evt events.Event) {
	ic, ok := evt.(*events.ImportCompleted)
	if !ok {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.pending[ic.ContentID] = &pendingVerification{
		contentID:  ic.ContentID,
		downloadID: ic.DownloadID,
		filePath:   ic.FilePath,
		addedAt:    time.Now(),
	}

	a.logger.Debug("tracking pending Plex verification",
		"content_id", ic.ContentID,
		"download_id", ic.DownloadID,
		"file_path", ic.FilePath)
}

// checkPending polls Plex for each pending verification.
func (a *Adapter) checkPending(ctx context.Context) {
	a.mu.RLock()
	// Make a copy of pending IDs to avoid holding lock during API calls
	pendingIDs := make([]int64, 0, len(a.pending))
	for id := range a.pending {
		pendingIDs = append(pendingIDs, id)
	}
	a.mu.RUnlock()

	a.logger.Debug("checking pending Plex verifications", "count", len(pendingIDs))

	for _, contentID := range pendingIDs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		found, plexKey, err := a.client.HasContentByID(ctx, contentID)
		if err != nil {
			a.logger.Error("failed to check content in Plex",
				"content_id", contentID,
				"error", err)
			continue
		}

		if found {
			a.emitPlexItemDetected(ctx, contentID, plexKey)
		}
	}
}

// emitPlexItemDetected publishes a PlexItemDetected event and removes from pending.
func (a *Adapter) emitPlexItemDetected(ctx context.Context, contentID int64, plexKey string) {
	a.mu.Lock()
	pv, exists := a.pending[contentID]
	if !exists {
		a.mu.Unlock()
		return
	}
	delete(a.pending, contentID)
	a.mu.Unlock()

	evt := &events.PlexItemDetected{
		BaseEvent: events.NewBaseEvent(events.EventPlexItemDetected, events.EntityContent, contentID),
		ContentID: contentID,
		PlexKey:   plexKey,
	}

	if err := a.bus.Publish(ctx, evt); err != nil {
		a.logger.Error("failed to publish PlexItemDetected event",
			"content_id", contentID,
			"error", err)
		return
	}

	a.logger.Info("Plex detected imported content",
		"content_id", contentID,
		"download_id", pv.downloadID,
		"plex_key", plexKey,
		"file_path", pv.filePath)
}
