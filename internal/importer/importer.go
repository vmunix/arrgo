// Package importer handles file import, renaming, and media server notification.
package importer

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
)

// Importer processes completed downloads.
type Importer struct {
	movieRoot    string
	seriesRoot   string
	movieNaming  string
	seriesNaming string
	plexURL      string
	plexToken    string
	plexLibs     []string
}

// Config for the importer.
type Config struct {
	MovieRoot    string
	SeriesRoot   string
	MovieNaming  string
	SeriesNaming string
	PlexURL      string
	PlexToken    string
	PlexLibs     []string
}

// New creates a new importer.
func New(cfg Config) *Importer {
	return &Importer{
		movieRoot:    cfg.MovieRoot,
		seriesRoot:   cfg.SeriesRoot,
		movieNaming:  cfg.MovieNaming,
		seriesNaming: cfg.SeriesNaming,
		plexURL:      cfg.PlexURL,
		plexToken:    cfg.PlexToken,
		plexLibs:     cfg.PlexLibs,
	}
}

// ImportResult is the result of an import operation.
type ImportResult struct {
	SourcePath string
	DestPath   string
	Quality    string
	Source     string
	SizeBytes  int64
}

// ImportMovie imports a completed movie download.
func (i *Importer) ImportMovie(ctx context.Context, downloadPath, title string, year int, quality string) (*ImportResult, error) {
	// TODO: find video file in downloadPath
	// TODO: apply naming template
	// TODO: create hardlink or move file
	// TODO: notify Plex
	return nil, nil
}

// ImportEpisode imports a completed episode download.
func (i *Importer) ImportEpisode(ctx context.Context, downloadPath, title string, season, episode int, quality string) (*ImportResult, error) {
	// TODO: implement
	return nil, nil
}

// NotifyPlex triggers a library scan.
func (i *Importer) NotifyPlex(ctx context.Context, libraryName string) error {
	// TODO: implement Plex API call
	return nil
}

// FindVideoFiles finds video files in a directory.
func FindVideoFiles(dir string) ([]string, error) {
	// TODO: implement
	return nil, nil
}

// ApplyNamingTemplate applies a naming template to generate a filename.
func ApplyNamingTemplate(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		placeholder := "{" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	// Handle format specifiers like {season:02d}
	re := regexp.MustCompile(`\{(\w+):(\d+)d\}`)
	result = re.ReplaceAllStringFunc(result, func(match string) string {
		// TODO: parse and apply format
		return match
	})
	return result
}

// VideoExtensions is the list of recognized video file extensions.
var VideoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".mov":  true,
	".wmv":  true,
	".ts":   true,
	".webm": true,
}

// IsVideoFile checks if a path is a video file.
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return VideoExtensions[ext]
}
