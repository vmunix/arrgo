// internal/handlers/download.go
package handlers

import (
	"context"
	"log/slog"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// DownloadHandler manages download lifecycle.
type DownloadHandler struct {
	*BaseHandler
	store  *download.Store
	client download.Downloader
}

// NewDownloadHandler creates a new download handler.
func NewDownloadHandler(bus *events.Bus, store *download.Store, client download.Downloader, logger *slog.Logger) *DownloadHandler {
	return &DownloadHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
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
