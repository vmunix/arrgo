// internal/importer/files.go
package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindAllVideos finds all video files in a directory (recursive).
// Skips files with "sample" in the name.
func FindAllVideos(root string) ([]string, error) {
	var videos []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !IsVideoFile(path) {
			return nil
		}

		// Skip sample files
		name := strings.ToLower(info.Name())
		if strings.Contains(name, "sample") {
			return nil
		}

		videos = append(videos, path)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return videos, nil
}
