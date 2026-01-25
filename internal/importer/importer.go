// Package importer handles file import, renaming, and media server notification.
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/library"
)

// Importer processes completed downloads.
type Importer struct {
	downloads   *download.Store
	library     *library.Store
	history     *HistoryStore
	renamer     *Renamer
	mediaServer MediaServer // nil if not configured
	movieRoot   string
	seriesRoot  string
	log         *slog.Logger
}

// Config for the importer.
type Config struct {
	MovieRoot      string
	SeriesRoot     string
	MovieTemplate  string
	SeriesTemplate string
	PlexURL        string
	PlexToken      string
	PlexLocalPath  string // Local path prefix (e.g., /srv/data/media)
	PlexRemotePath string // Plex's path prefix (e.g., /data/media)
}

// New creates a new importer.
func New(db *sql.DB, cfg Config, log *slog.Logger) *Importer {
	var mediaServer MediaServer
	if cfg.PlexURL != "" && cfg.PlexToken != "" {
		if cfg.PlexLocalPath != "" && cfg.PlexRemotePath != "" {
			mediaServer = NewPlexClientWithPathMapping(cfg.PlexURL, cfg.PlexToken, cfg.PlexLocalPath, cfg.PlexRemotePath, log)
		} else {
			mediaServer = NewPlexClient(cfg.PlexURL, cfg.PlexToken, log)
		}
	}

	return &Importer{
		downloads:   download.NewStore(db),
		library:     library.NewStore(db),
		history:     NewHistoryStore(db),
		renamer:     NewRenamer(cfg.MovieTemplate, cfg.SeriesTemplate),
		mediaServer: mediaServer,
		movieRoot:   cfg.MovieRoot,
		seriesRoot:  cfg.SeriesRoot,
		log:         log,
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
// It orchestrates three phases: prepare, execute, and notify.
func (i *Importer) Import(ctx context.Context, downloadID int64, downloadPath string) (*ImportResult, error) {
	i.log.Info("import started", "download_id", downloadID, "path", downloadPath)

	// Phase 1: Prepare - validate download, find video, build paths
	job, err := i.prepareImport(downloadID, downloadPath)
	if err != nil {
		return nil, err
	}

	// Phase 2: Execute - copy file, update database, record history
	result, err := i.executeImport(job)
	if err != nil {
		return nil, err
	}

	// Phase 3: Notify - trigger media server scan (best effort)
	i.notifyMediaServer(ctx, job, result)

	i.log.Info("import complete", "download_id", downloadID, "dest", job.DestPath, "quality", job.Quality)
	return result, nil
}

// prepareImport validates the download and prepares an import job.
// It verifies the download is ready, finds the video file, and builds paths.
func (i *Importer) prepareImport(downloadID int64, downloadPath string) (*ImportJob, error) {
	// Get download record
	dl, err := i.downloads.Get(downloadID)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrDownloadNotFound, err)
		}
		return nil, fmt.Errorf("get download: %w", err)
	}

	// Verify download is ready for import (completed or already transitioning to importing)
	if dl.Status != download.StatusCompleted && dl.Status != download.StatusImporting {
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
	i.log.Debug("found video", "path", srcPath)

	// Extract quality from release name
	quality := extractQuality(dl.ReleaseName)

	// Build destination path
	ext := strings.TrimPrefix(filepath.Ext(srcPath), ".")
	var relPath string
	var root string
	var episode *library.Episode

	if content.Type == library.ContentTypeMovie {
		relPath = i.renamer.MoviePath(content.Title, content.Year, quality, ext)
		root = i.movieRoot
	} else {
		// Series: require episode to be specified
		if dl.EpisodeID == nil {
			return nil, ErrEpisodeNotSpecified
		}

		episode, err = i.library.GetEpisode(*dl.EpisodeID)
		if err != nil {
			return nil, fmt.Errorf("get episode: %w", err)
		}

		relPath = i.renamer.EpisodePath(content.Title, episode.Season, episode.Episode, quality, ext)
		root = i.seriesRoot
	}

	destPath := filepath.Join(root, relPath)

	// Validate path is within root (security check)
	if err := ValidatePath(destPath, root); err != nil {
		return nil, err
	}

	return &ImportJob{
		Download:   dl,
		Content:    content,
		Episode:    episode,
		SourcePath: srcPath,
		DestPath:   destPath,
		Quality:    quality,
		RootPath:   root,
	}, nil
}

// executeImport copies the file and updates the database.
// It handles the file copy, database transaction, and history recording.
func (i *Importer) executeImport(job *ImportJob) (*ImportResult, error) {
	// Copy file
	size, err := CopyFile(job.SourcePath, job.DestPath)
	if err != nil {
		return nil, err
	}
	i.log.Debug("file copied", "src", job.SourcePath, "dest", job.DestPath, "size_bytes", size)

	// Update database in transaction
	tx, err := i.library.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert file record
	file := &library.File{
		ContentID: job.Content.ID,
		EpisodeID: job.Download.EpisodeID,
		Path:      job.DestPath,
		SizeBytes: size,
		Quality:   job.Quality,
		Source:    job.Download.Indexer,
	}
	if err := tx.AddFile(file); err != nil {
		return nil, fmt.Errorf("add file: %w", err)
	}

	// Update status: content for movies, episode for series
	if job.Episode != nil {
		job.Episode.Status = library.StatusAvailable
		if err := tx.UpdateEpisode(job.Episode); err != nil {
			return nil, fmt.Errorf("update episode: %w", err)
		}
	} else {
		job.Content.Status = library.StatusAvailable
		job.Content.UpdatedAt = time.Now()
		if err := tx.UpdateContent(job.Content); err != nil {
			return nil, fmt.Errorf("update content: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Update download status (separate from library transaction)
	job.Download.Status = download.StatusImported
	now := time.Now()
	job.Download.CompletedAt = &now
	if err := i.downloads.Update(job.Download); err != nil {
		i.log.Warn("update download status failed", "download_id", job.Download.ID, "error", err)
	}

	// Add history entry
	historyMap := map[string]any{
		"source_path":  job.SourcePath,
		"dest_path":    job.DestPath,
		"size_bytes":   size,
		"quality":      job.Quality,
		"indexer":      job.Download.Indexer,
		"release_name": job.Download.ReleaseName,
	}
	if job.Episode != nil {
		historyMap["season"] = job.Episode.Season
		historyMap["episode"] = job.Episode.Episode
	}
	historyData, _ := json.Marshal(historyMap)
	_ = i.history.Add(&HistoryEntry{
		ContentID: job.Content.ID,
		EpisodeID: job.Download.EpisodeID,
		Event:     EventImported,
		Data:      string(historyData),
	})

	return &ImportResult{
		FileID:     file.ID,
		SourcePath: job.SourcePath,
		DestPath:   job.DestPath,
		SizeBytes:  size,
		Quality:    job.Quality,
	}, nil
}

// notifyMediaServer triggers a scan of the imported file path.
// This is best-effort and failures are logged but don't fail the import.
func (i *Importer) notifyMediaServer(ctx context.Context, job *ImportJob, result *ImportResult) {
	if i.mediaServer == nil {
		return
	}

	if err := i.mediaServer.ScanPath(ctx, job.DestPath); err != nil {
		result.PlexError = err
		i.log.Warn("plex notification failed", "error", err)
	} else {
		result.PlexNotified = true
		i.log.Debug("plex notified", "path", job.DestPath)
	}
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

// EpisodeResult represents the outcome of importing a single episode.
type EpisodeResult struct {
	EpisodeID int64
	Season    int
	Episode   int
	Success   bool
	FilePath  string // Destination path (empty if failed)
	SizeBytes int64
	Error     error // nil if success
}

// SeasonPackResult is the result of importing a season pack.
type SeasonPackResult struct {
	TotalSize    int64           // Total bytes imported
	Episodes     []EpisodeResult // Per-episode outcomes
	PlexNotified bool
	PlexError    error
}

// SuccessCount returns the number of successfully imported episodes.
func (r *SeasonPackResult) SuccessCount() int {
	count := 0
	for _, ep := range r.Episodes {
		if ep.Success {
			count++
		}
	}
	return count
}

// ImportSeasonPack processes a season pack download with multiple video files.
// It matches each video file to an episode and imports them.
func (i *Importer) ImportSeasonPack(ctx context.Context, downloadID int64, downloadPath string) (*SeasonPackResult, error) {
	i.log.Info("season pack import started", "download_id", downloadID, "path", downloadPath)

	// Get download record
	dl, err := i.downloads.Get(downloadID)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrDownloadNotFound, err)
		}
		return nil, fmt.Errorf("get download: %w", err)
	}

	// Verify download is ready for import
	if dl.Status != download.StatusCompleted && dl.Status != download.StatusImporting {
		return nil, fmt.Errorf("%w: status is %s", ErrDownloadNotReady, dl.Status)
	}

	// Get content record
	content, err := i.library.GetContent(dl.ContentID)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}

	// Must be a series
	if content.Type != library.ContentTypeSeries {
		return nil, fmt.Errorf("season pack import requires series content, got %s", content.Type)
	}

	// Find all video files
	videos, err := FindAllVideos(downloadPath)
	if err != nil {
		return nil, fmt.Errorf("find videos: %w", err)
	}
	if len(videos) == 0 {
		return nil, ErrNoVideoFile
	}

	i.log.Info("found video files", "download_id", downloadID, "count", len(videos))

	// Extract quality from release name
	quality := extractQuality(dl.ReleaseName)

	result := &SeasonPackResult{
		Episodes: make([]EpisodeResult, 0, len(videos)),
	}

	// Process each video file
	for _, srcPath := range videos {
		epResult := i.importEpisodeFile(ctx, dl, content, srcPath, quality)
		result.Episodes = append(result.Episodes, epResult)
		if epResult.Success {
			result.TotalSize += epResult.SizeBytes
		}
	}

	// Notify media server once for the series folder (best effort)
	if i.mediaServer != nil {
		// Scan the series root folder
		seriesPath := filepath.Join(i.seriesRoot, SanitizeFilename(content.Title))
		if err := i.mediaServer.ScanPath(ctx, seriesPath); err != nil {
			result.PlexError = err
			i.log.Warn("plex notification failed", "error", err)
		} else {
			result.PlexNotified = true
			i.log.Debug("plex notified", "path", seriesPath)
		}
	}

	i.log.Info("season pack import complete",
		"download_id", downloadID,
		"episodes", len(result.Episodes),
		"success", result.SuccessCount(),
		"total_size", result.TotalSize)

	return result, nil
}

// importEpisodeFile imports a single episode file from a season pack.
func (i *Importer) importEpisodeFile(_ context.Context, dl *download.Download, content *library.Content, srcPath, quality string) EpisodeResult {
	// Parse filename to get season/episode
	season, epNum, err := MatchFileToSeason(srcPath)
	if err != nil {
		i.log.Warn("failed to match file to season", "path", srcPath, "error", err)
		return EpisodeResult{
			Season:  0,
			Episode: 0,
			Success: false,
			Error:   fmt.Errorf("match file to season: %w", err),
		}
	}

	// Find or create episode record
	episode, created, err := i.library.FindOrCreateEpisode(content.ID, season, epNum)
	if err != nil {
		i.log.Warn("failed to find/create episode", "season", season, "episode", epNum, "error", err)
		return EpisodeResult{
			Season:  season,
			Episode: epNum,
			Success: false,
			Error:   fmt.Errorf("find/create episode: %w", err),
		}
	}
	if created {
		i.log.Debug("created episode record", "episode_id", episode.ID, "season", season, "episode", epNum)
	}

	// Build destination path
	ext := strings.TrimPrefix(filepath.Ext(srcPath), ".")
	relPath := i.renamer.EpisodePath(content.Title, season, epNum, quality, ext)
	destPath := filepath.Join(i.seriesRoot, relPath)

	// Validate path is within root (security check)
	if err := ValidatePath(destPath, i.seriesRoot); err != nil {
		i.log.Warn("path validation failed", "path", destPath, "error", err)
		return EpisodeResult{
			EpisodeID: episode.ID,
			Season:    season,
			Episode:   epNum,
			Success:   false,
			Error:     err,
		}
	}

	// Check if destination already exists (for resumable imports)
	var size int64
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		i.log.Warn("failed to stat source file", "src", srcPath, "error", err)
		return EpisodeResult{
			EpisodeID: episode.ID,
			Season:    season,
			Episode:   epNum,
			Success:   false,
			Error:     fmt.Errorf("stat source: %w", err),
		}
	}

	if destInfo, err := os.Stat(destPath); err == nil {
		// Destination exists - check if it matches source size
		if destInfo.Size() == srcInfo.Size() {
			// File already imported, skip copy
			size = destInfo.Size()
			i.log.Info("episode file already exists, skipping copy",
				"dest", destPath, "size", size, "season", season, "episode", epNum)
		} else {
			// Size mismatch - file is incomplete, remove and re-copy
			i.log.Warn("destination file exists with wrong size, removing",
				"dest", destPath, "expected", srcInfo.Size(), "actual", destInfo.Size())
			if err := os.Remove(destPath); err != nil {
				i.log.Warn("failed to remove incomplete file", "dest", destPath, "error", err)
				return EpisodeResult{
					EpisodeID: episode.ID,
					Season:    season,
					Episode:   epNum,
					Success:   false,
					Error:     fmt.Errorf("remove incomplete file: %w", err),
				}
			}
			// Now copy the file
			size, err = CopyFile(srcPath, destPath)
			if err != nil {
				i.log.Warn("failed to copy file", "src", srcPath, "dest", destPath, "error", err)
				return EpisodeResult{
					EpisodeID: episode.ID,
					Season:    season,
					Episode:   epNum,
					Success:   false,
					Error:     fmt.Errorf("copy file: %w", err),
				}
			}
			i.log.Debug("copied episode file", "src", srcPath, "dest", destPath, "size", size)
		}
	} else {
		// Destination doesn't exist - copy the file
		size, err = CopyFile(srcPath, destPath)
		if err != nil {
			i.log.Warn("failed to copy file", "src", srcPath, "dest", destPath, "error", err)
			return EpisodeResult{
				EpisodeID: episode.ID,
				Season:    season,
				Episode:   epNum,
				Success:   false,
				Error:     fmt.Errorf("copy file: %w", err),
			}
		}
		i.log.Debug("copied episode file", "src", srcPath, "dest", destPath, "size", size)
	}

	// Update database in transaction
	tx, err := i.library.Begin()
	if err != nil {
		i.log.Warn("failed to begin transaction", "error", err)
		return EpisodeResult{
			EpisodeID: episode.ID,
			Season:    season,
			Episode:   epNum,
			Success:   false,
			Error:     fmt.Errorf("begin transaction: %w", err),
		}
	}
	defer func() { _ = tx.Rollback() }()

	// Insert file record (skip if already exists from previous import attempt)
	file := &library.File{
		ContentID: content.ID,
		EpisodeID: &episode.ID,
		Path:      destPath,
		SizeBytes: size,
		Quality:   quality,
		Source:    dl.Indexer,
	}
	if err := tx.AddFile(file); err != nil {
		if errors.Is(err, library.ErrDuplicate) {
			// File record already exists - this is fine for resumable imports
			i.log.Debug("file record already exists, skipping insert", "path", destPath)
		} else {
			i.log.Warn("failed to add file record", "error", err)
			return EpisodeResult{
				EpisodeID: episode.ID,
				Season:    season,
				Episode:   epNum,
				Success:   false,
				Error:     fmt.Errorf("add file: %w", err),
			}
		}
	}

	// Update episode status to available
	episode.Status = library.StatusAvailable
	if err := tx.UpdateEpisode(episode); err != nil {
		i.log.Warn("failed to update episode status", "error", err)
		return EpisodeResult{
			EpisodeID: episode.ID,
			Season:    season,
			Episode:   epNum,
			Success:   false,
			Error:     fmt.Errorf("update episode: %w", err),
		}
	}

	if err := tx.Commit(); err != nil {
		i.log.Warn("failed to commit transaction", "error", err)
		return EpisodeResult{
			EpisodeID: episode.ID,
			Season:    season,
			Episode:   epNum,
			Success:   false,
			Error:     fmt.Errorf("commit: %w", err),
		}
	}

	// Add history entry
	historyMap := map[string]any{
		"source_path":  srcPath,
		"dest_path":    destPath,
		"size_bytes":   size,
		"quality":      quality,
		"indexer":      dl.Indexer,
		"release_name": dl.ReleaseName,
		"season":       season,
		"episode":      epNum,
	}
	historyData, _ := json.Marshal(historyMap)
	_ = i.history.Add(&HistoryEntry{
		ContentID: content.ID,
		EpisodeID: &episode.ID,
		Event:     EventImported,
		Data:      string(historyData),
	})

	return EpisodeResult{
		EpisodeID: episode.ID,
		Season:    season,
		Episode:   epNum,
		Success:   true,
		FilePath:  destPath,
		SizeBytes: size,
	}
}
