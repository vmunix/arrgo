// internal/handlers/import.go
package handlers

import (
	"context"
	"log/slog"
	"sync"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/pkg/release"
)

// FileImporter is the interface for the importer.
type FileImporter interface {
	Import(ctx context.Context, downloadID int64, path string) (*importer.ImportResult, error)
	ImportSeasonPack(ctx context.Context, downloadID int64, path string) (*importer.SeasonPackResult, error)
}

// ImportHandler handles file import when downloads complete.
type ImportHandler struct {
	*BaseHandler
	store    *download.Store
	library  *library.Store
	importer FileImporter

	// Per-download lock to prevent concurrent imports
	importing sync.Map // map[int64]bool
}

// NewImportHandler creates a new import handler.
func NewImportHandler(bus *events.Bus, store *download.Store, lib *library.Store, imp FileImporter, logger *slog.Logger) *ImportHandler {
	return &ImportHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		library:     lib,
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

	// Check for existing files before importing (duplicate prevention)
	// Must happen before transitioning to importing, since completed→skipped is valid but importing→skipped is not
	if dl.ContentID > 0 && h.library != nil {
		// Build filter - for season packs, only compare against files from the same season
		filter := library.FileFilter{ContentID: &dl.ContentID}
		if dl.IsCompleteSeason && dl.Season != nil {
			filter.Season = dl.Season
		}
		files, _, err := h.library.ListFiles(filter)
		if err != nil {
			h.Logger().Warn("failed to check existing files", "error", err)
			// Continue with import on error - better to import than skip
		} else if len(files) > 0 {
			// Parse release name to get quality
			parsed := release.Parse(dl.ReleaseName)
			newQuality := parsed.Resolution.String()
			bestExisting := getBestQuality(files)

			// Skip if not an upgrade
			if !isBetterQuality(newQuality, bestExisting) {
				h.Logger().Warn("skipping import, existing quality equal or better",
					"download_id", e.DownloadID,
					"content_id", dl.ContentID,
					"new_quality", newQuality,
					"existing_quality", bestExisting,
					"release", dl.ReleaseName)

				// Transition to skipped status (from completed state)
				if err := h.store.Transition(dl, download.StatusSkipped); err != nil {
					h.Logger().Error("failed to transition to skipped", "download_id", e.DownloadID, "error", err)
				}

				// Emit ImportSkipped event
				if err := h.Bus().Publish(ctx, &events.ImportSkipped{
					BaseEvent:       events.NewBaseEvent(events.EventImportSkipped, events.EntityDownload, e.DownloadID),
					DownloadID:      e.DownloadID,
					ContentID:       dl.ContentID,
					SourcePath:      e.SourcePath,
					ReleaseQuality:  newQuality,
					ExistingQuality: bestExisting,
					Reason:          "existing_quality_equal_or_better",
				}); err != nil {
					h.Logger().Error("failed to publish ImportSkipped event", "error", err)
				}
				return
			}

			h.Logger().Info("proceeding with import upgrade",
				"download_id", e.DownloadID,
				"content_id", dl.ContentID,
				"new_quality", newQuality,
				"existing_quality", bestExisting)
		}
	}

	// Transition to importing status
	if err := h.store.Transition(dl, download.StatusImporting); err != nil {
		h.Logger().Error("failed to transition to importing", "download_id", e.DownloadID, "error", err)
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
		"path", e.SourcePath,
		"is_complete_season", dl.IsCompleteSeason)

	// Route to appropriate import handler based on download type
	if dl.IsCompleteSeason {
		h.handleSeasonPackImport(ctx, dl, e.SourcePath)
	} else {
		h.handleSingleFileImport(ctx, dl, e.SourcePath)
	}
}

// handleSingleFileImport handles import of a single-file download (movie or single episode).
func (h *ImportHandler) handleSingleFileImport(ctx context.Context, dl *download.Download, sourcePath string) {
	// Call importer
	result, err := h.importer.Import(ctx, dl.ID, sourcePath)
	if err != nil {
		h.Logger().Error("import failed", "download_id", dl.ID, "error", err)
		h.publishImportFailed(ctx, dl.ID, err.Error())
		return
	}

	// Transition to imported status
	if err := h.store.Transition(dl, download.StatusImported); err != nil {
		h.Logger().Error("failed to transition to imported", "download_id", dl.ID, "error", err)
		// Don't return - the import succeeded, just log the transition failure
	}

	// Emit ImportCompleted event
	if err := h.Bus().Publish(ctx, &events.ImportCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID: dl.ID,
		ContentID:  dl.ContentID,
		EpisodeID:  dl.EpisodeID,
		FilePath:   result.DestPath,
		FileSize:   result.SizeBytes,
	}); err != nil {
		h.Logger().Error("failed to publish ImportCompleted event", "error", err)
	}

	h.Logger().Info("import completed",
		"download_id", dl.ID,
		"content_id", dl.ContentID,
		"dest", result.DestPath,
		"size_bytes", result.SizeBytes)
}

// handleSeasonPackImport handles import of a season pack download with multiple episodes.
func (h *ImportHandler) handleSeasonPackImport(ctx context.Context, dl *download.Download, sourcePath string) {
	// Call season pack importer
	result, err := h.importer.ImportSeasonPack(ctx, dl.ID, sourcePath)
	if err != nil {
		h.Logger().Error("season pack import failed", "download_id", dl.ID, "error", err)
		h.publishImportFailed(ctx, dl.ID, err.Error())
		return
	}

	// Transition to imported status
	if err := h.store.Transition(dl, download.StatusImported); err != nil {
		h.Logger().Error("failed to transition to imported", "download_id", dl.ID, "error", err)
		// Don't return - the import succeeded, just log the transition failure
	}

	// Convert importer results to event results
	episodeResults := make([]events.EpisodeImportResult, 0, len(result.Episodes))
	for _, ep := range result.Episodes {
		epResult := events.EpisodeImportResult{
			EpisodeID: ep.EpisodeID,
			Season:    ep.Season,
			Episode:   ep.Episode,
			Success:   ep.Success,
			FilePath:  ep.FilePath,
		}
		if ep.Error != nil {
			epResult.Error = ep.Error.Error()
		}
		episodeResults = append(episodeResults, epResult)
	}

	// Emit ImportCompleted event with episode results
	if err := h.Bus().Publish(ctx, &events.ImportCompleted{
		BaseEvent:      events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
		DownloadID:     dl.ID,
		ContentID:      dl.ContentID,
		EpisodeResults: episodeResults,
		FileSize:       result.TotalSize,
	}); err != nil {
		h.Logger().Error("failed to publish ImportCompleted event", "error", err)
	}

	h.Logger().Info("season pack import completed",
		"download_id", dl.ID,
		"content_id", dl.ContentID,
		"episodes_total", len(result.Episodes),
		"episodes_success", result.SuccessCount(),
		"total_size", result.TotalSize)
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
