// internal/handlers/cleanup.go
package handlers

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
)

// CleanupConfig configures the cleanup handler.
type CleanupConfig struct {
	DownloadRoot string
	Enabled      bool
}

// pendingCleanup tracks downloads awaiting Plex verification.
type pendingCleanup struct {
	DownloadID  int64
	ContentID   int64
	ReleaseName string
}

// CleanupHandler cleans up source files after Plex verification.
type CleanupHandler struct {
	*BaseHandler
	store  *download.Store
	config CleanupConfig

	// Track pending cleanups by content ID
	mu      sync.RWMutex
	pending map[int64]*pendingCleanup // contentID -> pending
}

// NewCleanupHandler creates a new cleanup handler.
func NewCleanupHandler(bus *events.Bus, store *download.Store, config CleanupConfig, logger *slog.Logger) *CleanupHandler {
	return &CleanupHandler{
		BaseHandler: NewBaseHandler(bus, logger),
		store:       store,
		config:      config,
		pending:     make(map[int64]*pendingCleanup),
	}
}

// reconcileOnStartup restores pending cleanups from database.
// This handles the case where server restarted after import but before Plex detection.
func (h *CleanupHandler) reconcileOnStartup(_ context.Context) {
	// Query downloads in "imported" status (no pagination - reconcile all)
	status := download.StatusImported
	downloads, _, err := h.store.List(download.Filter{Status: &status})
	if err != nil {
		h.Logger().Error("failed to list imported downloads for reconciliation", "error", err)
		return
	}

	if len(downloads) == 0 {
		h.Logger().Debug("no imported downloads to reconcile for cleanup")
		return
	}

	h.Logger().Info("reconciling imported downloads for cleanup", "count", len(downloads))

	// Pre-populate pending map
	h.mu.Lock()
	for _, dl := range downloads {
		h.pending[dl.ContentID] = &pendingCleanup{
			DownloadID:  dl.ID,
			ContentID:   dl.ContentID,
			ReleaseName: dl.ReleaseName,
		}
	}
	h.mu.Unlock()
}

// Name returns the handler name.
func (h *CleanupHandler) Name() string {
	return "cleanup"
}

// Start begins processing events.
func (h *CleanupHandler) Start(ctx context.Context) error {
	// Reconcile on startup - restore pending cleanups from database
	h.reconcileOnStartup(ctx)

	importCompleted := h.Bus().Subscribe(events.EventImportCompleted, 100)
	importSkipped := h.Bus().Subscribe(events.EventImportSkipped, 100)
	plexDetected := h.Bus().Subscribe(events.EventPlexItemDetected, 100)

	for {
		select {
		case e := <-importCompleted:
			if e == nil {
				return nil // Channel closed
			}
			h.handleImportCompleted(ctx, e.(*events.ImportCompleted))
		case e := <-importSkipped:
			if e == nil {
				return nil // Channel closed
			}
			h.handleImportSkipped(ctx, e.(*events.ImportSkipped))
		case e := <-plexDetected:
			if e == nil {
				return nil // Channel closed
			}
			h.handlePlexDetected(ctx, e.(*events.PlexItemDetected))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// handleImportSkipped cleans up immediately since Plex already has the content.
func (h *CleanupHandler) handleImportSkipped(ctx context.Context, e *events.ImportSkipped) {
	if !h.config.Enabled {
		h.Logger().Debug("cleanup disabled, skipping", "download_id", e.DownloadID)
		return
	}

	// Get download to retrieve release name
	dl, err := h.store.Get(e.DownloadID)
	if err != nil {
		h.Logger().Error("failed to get download for cleanup",
			"download_id", e.DownloadID,
			"error", err)
		return
	}

	h.Logger().Info("import skipped due to existing quality, starting immediate cleanup",
		"download_id", e.DownloadID,
		"content_id", e.ContentID,
		"release_name", dl.ReleaseName,
		"reason", e.Reason)

	// Perform cleanup immediately (Plex already has content)
	sourcePath := filepath.Join(h.config.DownloadRoot, dl.ReleaseName)

	// Emit CleanupStarted event
	if err := h.Bus().Publish(ctx, &events.CleanupStarted{
		BaseEvent:  events.NewBaseEvent(events.EventCleanupStarted, events.EntityDownload, e.DownloadID),
		DownloadID: e.DownloadID,
		SourcePath: sourcePath,
	}); err != nil {
		h.Logger().Error("failed to publish CleanupStarted event", "error", err)
	}

	// Safely delete source files
	if err := h.cleanupSource(sourcePath); err != nil {
		h.Logger().Error("cleanup failed",
			"download_id", e.DownloadID,
			"source_path", sourcePath,
			"error", err)
		return
	}

	// Note: StatusSkipped is terminal - no transition needed.
	// The skipped state correctly reflects that the import was skipped due to duplicate.

	// Emit CleanupCompleted event
	if err := h.Bus().Publish(ctx, &events.CleanupCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventCleanupCompleted, events.EntityDownload, e.DownloadID),
		DownloadID: e.DownloadID,
	}); err != nil {
		h.Logger().Error("failed to publish CleanupCompleted event", "error", err)
	}

	h.Logger().Info("cleanup completed for skipped import",
		"download_id", e.DownloadID,
		"source_path", sourcePath)
}

// handleImportCompleted tracks pending cleanup for the imported content.
func (h *CleanupHandler) handleImportCompleted(_ context.Context, e *events.ImportCompleted) {
	// Get download to retrieve release name
	dl, err := h.store.Get(e.DownloadID)
	if err != nil {
		h.Logger().Error("failed to get download for cleanup tracking",
			"download_id", e.DownloadID,
			"error", err)
		return
	}

	// Track pending cleanup
	h.mu.Lock()
	h.pending[e.ContentID] = &pendingCleanup{
		DownloadID:  e.DownloadID,
		ContentID:   e.ContentID,
		ReleaseName: dl.ReleaseName,
	}
	h.mu.Unlock()

	h.Logger().Debug("tracking pending cleanup",
		"download_id", e.DownloadID,
		"content_id", e.ContentID,
		"release_name", dl.ReleaseName)
}

// handlePlexDetected performs cleanup if pending cleanup exists for the content.
func (h *CleanupHandler) handlePlexDetected(ctx context.Context, e *events.PlexItemDetected) {
	if !h.config.Enabled {
		h.Logger().Debug("cleanup disabled, skipping", "content_id", e.ContentID)
		return
	}

	// Check for pending cleanup
	h.mu.Lock()
	pending, ok := h.pending[e.ContentID]
	if ok {
		delete(h.pending, e.ContentID)
	}
	h.mu.Unlock()

	if !ok {
		h.Logger().Debug("no pending cleanup for content", "content_id", e.ContentID)
		return
	}

	h.Logger().Info("plex item detected, starting cleanup",
		"content_id", e.ContentID,
		"download_id", pending.DownloadID,
		"release_name", pending.ReleaseName)

	// Perform cleanup
	sourcePath := filepath.Join(h.config.DownloadRoot, pending.ReleaseName)

	// Emit CleanupStarted event
	if err := h.Bus().Publish(ctx, &events.CleanupStarted{
		BaseEvent:  events.NewBaseEvent(events.EventCleanupStarted, events.EntityDownload, pending.DownloadID),
		DownloadID: pending.DownloadID,
		SourcePath: sourcePath,
	}); err != nil {
		h.Logger().Error("failed to publish CleanupStarted event", "error", err)
	}

	// Safely delete source files
	if err := h.cleanupSource(sourcePath); err != nil {
		h.Logger().Error("cleanup failed",
			"download_id", pending.DownloadID,
			"source_path", sourcePath,
			"error", err)
		return
	}

	// Update download status to cleaned
	dl, err := h.store.Get(pending.DownloadID)
	if err != nil {
		h.Logger().Error("failed to get download for status update",
			"download_id", pending.DownloadID,
			"error", err)
		return
	}

	if err := h.store.Transition(dl, download.StatusCleaned); err != nil {
		h.Logger().Error("failed to transition download to cleaned",
			"download_id", pending.DownloadID,
			"error", err)
		return
	}

	// Emit CleanupCompleted event
	if err := h.Bus().Publish(ctx, &events.CleanupCompleted{
		BaseEvent:  events.NewBaseEvent(events.EventCleanupCompleted, events.EntityDownload, pending.DownloadID),
		DownloadID: pending.DownloadID,
	}); err != nil {
		h.Logger().Error("failed to publish CleanupCompleted event", "error", err)
	}

	h.Logger().Info("cleanup completed",
		"download_id", pending.DownloadID,
		"source_path", sourcePath)
}

// ErrPathOutsideRoot is returned when cleanup path is outside download root.
var ErrPathOutsideRoot = os.ErrPermission

// cleanupSource safely deletes files under DownloadRoot.
func (h *CleanupHandler) cleanupSource(sourcePath string) error {
	// Resolve absolute paths for safety check
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return err
	}

	absRoot, err := filepath.Abs(h.config.DownloadRoot)
	if err != nil {
		return err
	}

	// Ensure source path is under download root (prevent path traversal)
	// Use filepath.Clean to normalize paths before comparison
	cleanSource := filepath.Clean(absSource)
	cleanRoot := filepath.Clean(absRoot)

	// Source must be under root (add separator to prevent /downloads matching /downloads-other)
	if !strings.HasPrefix(cleanSource, cleanRoot+string(filepath.Separator)) && cleanSource != cleanRoot {
		h.Logger().Warn("refusing to delete path outside download root",
			"source_path", sourcePath,
			"download_root", h.config.DownloadRoot)
		return ErrPathOutsideRoot
	}

	// Check if path exists
	info, err := os.Stat(absSource)
	if os.IsNotExist(err) {
		h.Logger().Debug("source path already deleted", "source_path", sourcePath)
		return nil
	}
	if err != nil {
		return err
	}

	// Delete file or directory
	if info.IsDir() {
		return os.RemoveAll(absSource)
	}
	return os.Remove(absSource)
}
