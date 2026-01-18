// Package importer handles file import, renaming, and media server notification.
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/library"
)

// Importer processes completed downloads.
type Importer struct {
	downloads  *download.Store
	library    *library.Store
	history    *HistoryStore
	renamer    *Renamer
	plex       *PlexClient // nil if not configured
	movieRoot  string
	seriesRoot string
}

// Config for the importer.
type Config struct {
	MovieRoot      string
	SeriesRoot     string
	MovieTemplate  string
	SeriesTemplate string
	PlexURL        string
	PlexToken      string
}

// New creates a new importer.
func New(db *sql.DB, cfg Config) *Importer {
	var plex *PlexClient
	if cfg.PlexURL != "" && cfg.PlexToken != "" {
		plex = NewPlexClient(cfg.PlexURL, cfg.PlexToken)
	}

	return &Importer{
		downloads:  download.NewStore(db),
		library:    library.NewStore(db),
		history:    NewHistoryStore(db),
		renamer:    NewRenamer(cfg.MovieTemplate, cfg.SeriesTemplate),
		plex:       plex,
		movieRoot:  cfg.MovieRoot,
		seriesRoot: cfg.SeriesRoot,
	}
}

// ImportResult is the result of an import operation.
type ImportResult struct {
	FileID       int64
	SourcePath   string
	DestPath     string
	SizeBytes    int64
	Quality      string
	PlexNotified bool
	PlexError    error
}

// Import processes a completed download.
func (i *Importer) Import(ctx context.Context, downloadID int64, downloadPath string) (*ImportResult, error) {
	// Get download record
	dl, err := i.downloads.Get(downloadID)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			return nil, fmt.Errorf("%w: %v", ErrDownloadNotFound, err)
		}
		return nil, fmt.Errorf("get download: %w", err)
	}

	// Verify download is completed
	if dl.Status != download.StatusCompleted {
		return nil, fmt.Errorf("%w: status is %s", ErrDownloadNotReady, dl.Status)
	}

	// Get content record
	content, err := i.library.GetContent(dl.ContentID)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}

	// Find largest video file
	srcPath, _, err := FindLargestVideo(downloadPath)
	if err != nil {
		return nil, err
	}

	// Extract quality from release name
	quality := extractQuality(dl.ReleaseName)

	// Build destination path
	ext := strings.TrimPrefix(filepath.Ext(srcPath), ".")
	var relPath string
	var root string

	if content.Type == library.ContentTypeMovie {
		relPath = i.renamer.MoviePath(content.Title, content.Year, quality, ext)
		root = i.movieRoot
	} else {
		// For series, we'd need episode info - simplified for now
		return nil, fmt.Errorf("series import not yet implemented")
	}

	destPath := filepath.Join(root, relPath)

	// Validate path is within root (security check)
	if err := ValidatePath(destPath, root); err != nil {
		return nil, err
	}

	// Copy file
	size, err := CopyFile(srcPath, destPath)
	if err != nil {
		return nil, err
	}

	// Update database in transaction
	tx, err := i.library.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert file record
	file := &library.File{
		ContentID: content.ID,
		Path:      destPath,
		SizeBytes: size,
		Quality:   quality,
		Source:    dl.Indexer,
	}
	if err := tx.AddFile(file); err != nil {
		return nil, fmt.Errorf("add file: %w", err)
	}

	// Update content status
	content.Status = library.StatusAvailable
	content.UpdatedAt = time.Now()
	if err := tx.UpdateContent(content); err != nil {
		return nil, fmt.Errorf("update content: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Update download status (separate from library transaction)
	dl.Status = download.StatusImported
	now := time.Now()
	dl.CompletedAt = &now
	_ = i.downloads.Update(dl) // Log but don't fail

	// Add history entry
	historyData, _ := json.Marshal(map[string]any{
		"source_path":  srcPath,
		"dest_path":    destPath,
		"size_bytes":   size,
		"quality":      quality,
		"indexer":      dl.Indexer,
		"release_name": dl.ReleaseName,
	})
	_ = i.history.Add(&HistoryEntry{
		ContentID: content.ID,
		Event:     EventImported,
		Data:      string(historyData),
	})

	result := &ImportResult{
		FileID:     file.ID,
		SourcePath: srcPath,
		DestPath:   destPath,
		SizeBytes:  size,
		Quality:    quality,
	}

	// Notify Plex (best effort)
	if i.plex != nil {
		if err := i.plex.ScanPath(ctx, destPath); err != nil {
			result.PlexError = err
		} else {
			result.PlexNotified = true
		}
	}

	return result, nil
}

// extractQuality extracts resolution from a release name.
func extractQuality(releaseName string) string {
	lower := strings.ToLower(releaseName)
	switch {
	case strings.Contains(lower, "2160p") || strings.Contains(lower, "4k"):
		return "2160p"
	case strings.Contains(lower, "1080p"):
		return "1080p"
	case strings.Contains(lower, "720p"):
		return "720p"
	case strings.Contains(lower, "480p"):
		return "480p"
	default:
		return "unknown"
	}
}
