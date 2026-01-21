// Package sabnzbd provides an adapter that polls SABnzbd for download status
// and emits events when status changes.
package sabnzbd

import (
	"context"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// Adapter polls SABnzbd and emits events for status changes.
type Adapter struct {
	client   download.Downloader
	bus      *events.Bus
	store    *download.Store
	interval time.Duration
	logger   *slog.Logger

	// Track last known status to avoid duplicate state transition events
	lastStatus map[int64]download.Status
}

// New creates a new SABnzbd adapter.
func New(bus *events.Bus, client download.Downloader, store *download.Store, interval time.Duration, logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		client:     client,
		bus:        bus,
		store:      store,
		interval:   interval,
		logger:     logger,
		lastStatus: make(map[int64]download.Status),
	}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "sabnzbd"
}

// Start begins polling at the configured interval.
// It runs until the context is canceled.
func (a *Adapter) Start(ctx context.Context) error {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	// Poll immediately on start
	a.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.poll(ctx)
		}
	}
}

// poll retrieves tracked downloads and checks their status.
func (a *Adapter) poll(ctx context.Context) {
	// Get active SABnzbd downloads from store
	client := download.ClientSABnzbd
	downloads, err := a.store.List(download.Filter{
		Client: &client,
		Active: true,
	})
	if err != nil {
		a.logger.Error("failed to list downloads", "error", err)
		return
	}

	for _, dl := range downloads {
		// Skip downloads already in terminal states
		if isTerminalStatus(dl.Status) {
			continue
		}

		a.checkDownload(ctx, dl)
	}
}

// checkDownload queries the client for status and emits appropriate events.
func (a *Adapter) checkDownload(ctx context.Context, dl *download.Download) {
	status, err := a.client.Status(ctx, dl.ClientID)
	if err != nil {
		a.logger.Error("failed to get download status",
			"download_id", dl.ID,
			"client_id", dl.ClientID,
			"error", err)
		return
	}

	// Handle disappeared download
	if status == nil {
		a.processDisappeared(ctx, dl)
		return
	}

	a.processStatus(ctx, dl, status)
}

// processStatus compares status and emits appropriate events.
func (a *Adapter) processStatus(ctx context.Context, dl *download.Download, status *download.ClientStatus) {
	// Check if this is a state transition we've already emitted
	lastStatus, seen := a.lastStatus[dl.ID]
	if seen && lastStatus == status.Status {
		// No state change since last emission for terminal states
		// For progress events, we always emit
		if status.Status != download.StatusDownloading && status.Status != download.StatusQueued {
			return
		}
	}

	switch status.Status {
	case download.StatusCompleted:
		if !seen || lastStatus != download.StatusCompleted {
			a.emitCompleted(ctx, dl, status)
			a.lastStatus[dl.ID] = download.StatusCompleted
		}

	case download.StatusFailed:
		if !seen || lastStatus != download.StatusFailed {
			a.emitFailed(ctx, dl, "download reported failed by client", true)
			a.lastStatus[dl.ID] = download.StatusFailed
		}

	case download.StatusDownloading, download.StatusQueued:
		a.emitProgressed(ctx, dl, status)
		a.lastStatus[dl.ID] = status.Status
	}
}

// processDisappeared handles when a download is no longer in the client.
func (a *Adapter) processDisappeared(ctx context.Context, dl *download.Download) {
	// Only emit if we haven't already marked it as failed
	if lastStatus, seen := a.lastStatus[dl.ID]; seen && lastStatus == download.StatusFailed {
		return
	}

	a.emitFailed(ctx, dl, "download disappeared from client", false)
	a.lastStatus[dl.ID] = download.StatusFailed
}

// emitCompleted publishes a DownloadCompleted event.
func (a *Adapter) emitCompleted(ctx context.Context, dl *download.Download, status *download.ClientStatus) {
	evt := &events.DownloadCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		SourcePath: status.Path,
	}

	if err := a.bus.Publish(ctx, evt); err != nil {
		a.logger.Error("failed to publish DownloadCompleted event",
			"download_id", dl.ID,
			"error", err)
	}

	a.logger.Info("download completed",
		"download_id", dl.ID,
		"path", status.Path)
}

// emitFailed publishes a DownloadFailed event.
func (a *Adapter) emitFailed(ctx context.Context, dl *download.Download, reason string, retryable bool) {
	evt := &events.DownloadFailed{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadFailed, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		Reason:     reason,
		Retryable:  retryable,
	}

	if err := a.bus.Publish(ctx, evt); err != nil {
		a.logger.Error("failed to publish DownloadFailed event",
			"download_id", dl.ID,
			"error", err)
	}

	a.logger.Warn("download failed",
		"download_id", dl.ID,
		"reason", reason,
		"retryable", retryable)
}

// emitProgressed publishes a DownloadProgressed event.
func (a *Adapter) emitProgressed(ctx context.Context, dl *download.Download, status *download.ClientStatus) {
	evt := &events.DownloadProgressed{
		BaseEvent:  events.NewBaseEvent(events.EventDownloadProgressed, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		Progress:   status.Progress,
		Speed:      status.Speed,
		ETA:        int(status.ETA.Seconds()),
		Size:       status.Size,
	}

	if err := a.bus.Publish(ctx, evt); err != nil {
		a.logger.Error("failed to publish DownloadProgressed event",
			"download_id", dl.ID,
			"error", err)
	}

	a.logger.Debug("download progress",
		"download_id", dl.ID,
		"progress", status.Progress,
		"speed", status.Speed)
}

// isTerminalStatus returns true if the status is a terminal state.
func isTerminalStatus(s download.Status) bool {
	switch s {
	case download.StatusCompleted, download.StatusImported, download.StatusCleaned, download.StatusFailed:
		return true
	default:
		return false
	}
}
