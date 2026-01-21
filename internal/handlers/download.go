// internal/handlers/download.go
package handlers

import (
	"context"
	"log/slog"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/pkg/release"
)

// DownloadHandler manages download lifecycle.
type DownloadHandler struct {
	*BaseHandler
	store   *download.Store
	library *library.Store
	client  download.Downloader
}

// NewDownloadHandler creates a new download handler.
func NewDownloadHandler(bus *events.Bus, store *download.Store, lib *library.Store, client download.Downloader, logger *slog.Logger) *DownloadHandler {
	return &DownloadHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		library:     lib,
		client:      client,
	}
}

// Name returns the handler name.
func (h *DownloadHandler) Name() string {
	return "download"
}

// Start begins processing events.
func (h *DownloadHandler) Start(ctx context.Context) error {
	grabs := h.Bus().Subscribe(events.EventGrabRequested, 100)

	for {
		select {
		case e := <-grabs:
			if e == nil {
				return nil // Channel closed
			}
			h.handleGrabRequested(ctx, e.(*events.GrabRequested))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *DownloadHandler) handleGrabRequested(ctx context.Context, e *events.GrabRequested) {
	h.Logger().Info("processing grab request",
		"content_id", e.ContentID,
		"release", e.ReleaseName,
		"indexer", e.Indexer)

	// Check for existing files before grabbing (duplicate prevention)
	if e.ContentID > 0 && h.library != nil {
		files, _, err := h.library.ListFiles(library.FileFilter{ContentID: &e.ContentID})
		if err != nil {
			h.Logger().Warn("failed to check existing files", "error", err)
			// Continue with grab on error - better to grab than miss content
		} else if len(files) > 0 {
			// Parse release name to get quality
			parsed := release.Parse(e.ReleaseName)
			newQuality := parsed.Resolution.String()
			bestExisting := getBestQuality(files)

			// Skip if not an upgrade
			if !isBetterQuality(newQuality, bestExisting) {
				h.Logger().Warn("skipping grab, existing quality equal or better",
					"content_id", e.ContentID,
					"new_quality", newQuality,
					"existing_quality", bestExisting,
					"release", e.ReleaseName)

				// Emit GrabSkipped event
				if err := h.Bus().Publish(ctx, &events.GrabSkipped{
					BaseEvent:       events.NewBaseEvent(events.EventGrabSkipped, events.EntityContent, e.ContentID),
					ContentID:       e.ContentID,
					ReleaseName:     e.ReleaseName,
					ReleaseQuality:  newQuality,
					ExistingQuality: bestExisting,
					Reason:          "existing_quality_equal_or_better",
				}); err != nil {
					h.Logger().Error("failed to publish GrabSkipped event", "error", err)
				}
				return
			}

			h.Logger().Info("proceeding with upgrade",
				"content_id", e.ContentID,
				"new_quality", newQuality,
				"existing_quality", bestExisting)
		}
	}

	// Send to download client
	clientID, err := h.client.Add(ctx, e.DownloadURL, "")
	if err != nil {
		h.Logger().Error("failed to add download", "error", err)
		if pubErr := h.Bus().Publish(ctx, &events.DownloadFailed{
			BaseEvent:  events.NewBaseEvent(events.EventDownloadFailed, events.EntityDownload, 0),
			DownloadID: 0,
			Reason:     err.Error(),
			Retryable:  true,
		}); pubErr != nil {
			h.Logger().Error("failed to publish DownloadFailed event", "error", pubErr)
		}
		return
	}

	// Create DB record
	dl := &download.Download{
		ContentID:   e.ContentID,
		EpisodeID:   e.EpisodeID,
		Client:      download.ClientSABnzbd,
		ClientID:    clientID,
		Status:      download.StatusQueued,
		ReleaseName: e.ReleaseName,
		Indexer:     e.Indexer,
	}

	if err := h.store.Add(dl); err != nil {
		h.Logger().Error("failed to save download", "error", err)
		return
	}

	// Emit success event
	if err := h.Bus().Publish(ctx, &events.DownloadCreated{
		BaseEvent:   events.NewBaseEvent(events.EventDownloadCreated, events.EntityDownload, dl.ID),
		DownloadID:  dl.ID,
		ContentID:   e.ContentID,
		EpisodeID:   e.EpisodeID,
		ClientID:    clientID,
		ReleaseName: e.ReleaseName,
	}); err != nil {
		h.Logger().Error("failed to publish DownloadCreated event", "error", err)
	}

	h.Logger().Info("download created",
		"download_id", dl.ID,
		"client_id", clientID)
}
