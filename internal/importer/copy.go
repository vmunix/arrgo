// internal/importer/copy.go
package importer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyFile copies a file from src to dst.
// Creates destination directory if it doesn't exist.
// Returns ErrDestinationExists if dst already exists.
func CopyFile(src, dst string) (int64, error) {
	// Check if destination exists
	if _, err := os.Stat(dst); err == nil {
		return 0, ErrDestinationExists
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, fmt.Errorf("%w: create directory: %v", ErrCopyFailed, err)
	}

	// Open source
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("%w: open source: %v", ErrCopyFailed, err)
	}
	defer func() { _ = srcFile.Close() }()

	// Create destination
	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("%w: create destination: %v", ErrCopyFailed, err)
	}
	defer func() { _ = dstFile.Close() }()

	// Copy content
	size, err := io.Copy(dstFile, srcFile)
	if err != nil {
		// Clean up partial file on error
		_ = os.Remove(dst)
		return 0, fmt.Errorf("%w: copy content: %v", ErrCopyFailed, err)
	}

	// Sync to disk
	if err := dstFile.Sync(); err != nil {
		return 0, fmt.Errorf("%w: sync: %v", ErrCopyFailed, err)
	}

	return size, nil
}

// FindLargestVideo finds the largest video file in a directory tree.
// Returns ErrNoVideoFile if no video files are found.
// Skips files with "sample" in the name.
func FindLargestVideo(dir string) (string, int64, error) {
	var largestPath string
	var largestSize int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if info.IsDir() {
			return nil
		}

		// Skip non-video files
		if !IsVideoFile(path) {
			return nil
		}

		// Skip sample files
		name := strings.ToLower(info.Name())
		if strings.Contains(name, "sample") {
			return nil
		}

		// Track largest
		if info.Size() > largestSize {
			largestSize = info.Size()
			largestPath = path
		}

		return nil
	})

	if err != nil {
		return "", 0, fmt.Errorf("walk directory: %w", err)
	}

	if largestPath == "" {
		return "", 0, ErrNoVideoFile
	}

	return largestPath, largestSize, nil
}
