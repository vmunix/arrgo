// internal/handlers/import.go
package handlers

import (
	"context"
	"log/slog"
	"sync"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
)

// FileImporter is the interface for the importer.
type FileImporter interface {
	Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error)
}

// ImportHandler handles file import when downloads complete.
type ImportHandler struct {
	*BaseHandler
	store    *download.Store
	importer FileImporter

	// Per-download lock to prevent concurrent imports
	importing sync.Map // map[int64]bool
}

// NewImportHandler creates a new import handler.
func NewImportHandler(bus *events.Bus, store *download.Store, imp FileImporter, logger *slog.Logger) *ImportHandler {
	return &ImportHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		importer:    imp,
	}
}

// Name returns the handler name.
func (h *ImportHandler) Name() string {
	return "import"
}

// Start begins processing events.
func (h *ImportHandler) Start(ctx context.Context) error {
	completed := h.Bus().Subscribe(events.EventDownloadCompleted, 100)

	for {
		select {
		case e := <-completed:
			if e == nil {
				return nil // Channel closed
			}
			// Process in goroutine to not block other events
			go h.handleDownloadCompleted(ctx, e.(*events.DownloadCompleted))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *ImportHandler) handleDownloadCompleted(ctx context.Context, e *events.DownloadCompleted) {
	// Acquire per-download lock (prevents concurrent imports)
	if _, loaded := h.importing.LoadOrStore(e.DownloadID, true); loaded {
		h.Logger().Warn("import already in progress", "download_id", e.DownloadID)
		return
	}
	defer h.importing.Delete(e.DownloadID)

	// Get download from store to retrieve ContentID for events
	dl, err := h.store.Get(e.DownloadID)
	if err != nil {
		h.Logger().Error("failed to get download", "download_id", e.DownloadID, "error", err)
		h.publishImportFailed(ctx, e.DownloadID, err.Error())
		return
	}

	// Emit ImportStarted event
	if err := h.Bus().Publish(ctx, &events.ImportStarted{
		BaseEvent:  events.NewBaseEvent(events.EventImportStarted, events.EntityDownload, e.DownloadID),
		DownloadID: e.DownloadID,
		SourcePath: e.SourcePath,
	}); err != nil {
		h.Logger().Error("failed to publish ImportStarted event", "error", err)
	}

	h.Logger().Info("starting import",
		"download_id", e.DownloadID,
		"content_id", dl.ContentID,
		"path", e.SourcePath)

	// Call importer
	result, err := h.importer.Import(ctx, e.DownloadID, e.SourcePath)
	if err != nil {
		h.Logger().Error("import failed", "download_id", e.DownloadID, "error", err)
		h.publishImportFailed(ctx, e.DownloadID, err.Error())
		return
	}

	// Emit ImportCompleted event
	if err := h.Bus().Publish(ctx, &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, e.DownloadID),
		DownloadID: e.DownloadID,
		ContentID:  dl.ContentID,
		EpisodeID:  dl.EpisodeID,
		FilePath:   result.DestPath,
		FileSize:   result.SizeBytes,
	}); err != nil {
		h.Logger().Error("failed to publish ImportCompleted event", "error", err)
	}

	h.Logger().Info("import completed",
		"download_id", e.DownloadID,
		"content_id", dl.ContentID,
		"dest", result.DestPath,
		"size_bytes", result.SizeBytes)
}

func (h *ImportHandler) publishImportFailed(ctx context.Context, downloadID int64, reason string) {
	if err := h.Bus().Publish(ctx, &events.ImportFailed{
		BaseEvent:  events.NewBaseEvent(events.EventImportFailed, events.EntityDownload, downloadID),
		DownloadID: downloadID,
		Reason:     reason,
	}); err != nil {
		h.Logger().Error("failed to publish ImportFailed event", "error", err)
	}
}
