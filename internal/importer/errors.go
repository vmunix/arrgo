// internal/importer/errors.go
package importer

import "errors"

var (
	// ErrDownloadNotFound indicates the download record doesn't exist.
	ErrDownloadNotFound = errors.New("download not found")

	// ErrDownloadNotReady indicates the download is not in completed status.
	ErrDownloadNotReady = errors.New("download not in completed status")

	// ErrNoVideoFile indicates no video file was found in the download.
	ErrNoVideoFile = errors.New("no video file found in download")

	// ErrCopyFailed indicates the file copy operation failed.
	ErrCopyFailed = errors.New("failed to copy file")

	// ErrDestinationExists indicates the destination file already exists.
	ErrDestinationExists = errors.New("destination file already exists")

	// ErrPathTraversal indicates a path traversal attack was detected.
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrEpisodeNotSpecified indicates a series download is missing the episode ID.
	ErrEpisodeNotSpecified = errors.New("episode not specified for series download")
)
