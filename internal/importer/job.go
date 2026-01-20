package importer

import (
	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/library"
)

// ImportJob represents a prepared import operation.
// It contains all the data needed to execute an import after validation.
type ImportJob struct {
	// Download is the download record being imported.
	Download *download.Download

	// Content is the library content associated with this download.
	Content *library.Content

	// Episode is the episode record (nil for movies).
	Episode *library.Episode

	// SourcePath is the path to the video file being imported.
	SourcePath string

	// DestPath is the full destination path for the imported file.
	DestPath string

	// Quality is the extracted quality string (e.g., "1080p").
	Quality string

	// RootPath is the library root directory.
	RootPath string
}
