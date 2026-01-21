// internal/handlers/quality.go
package handlers

import (
	"strings"

	"github.com/vmunix/arrgo/internal/library"
)

// resolutionRank returns a numeric rank for resolution comparison.
// Higher is better: 2160p=4, 1080p=3, 720p=2, 480p=1, unknown=0
func resolutionRank(quality string) int {
	switch strings.ToLower(quality) {
	case "2160p", "4k", "uhd":
		return 4
	case "1080p", "fhd":
		return 3
	case "720p", "hd":
		return 2
	case "480p", "sd":
		return 1
	default:
		return 0
	}
}

// isBetterQuality returns true if newQuality is strictly better than existing.
func isBetterQuality(newQuality, existingQuality string) bool {
	return resolutionRank(newQuality) > resolutionRank(existingQuality)
}

// getBestQuality returns the highest resolution quality from a list of files.
func getBestQuality(files []*library.File) string {
	if len(files) == 0 {
		return ""
	}

	best := files[0].Quality
	for _, f := range files[1:] {
		if resolutionRank(f.Quality) > resolutionRank(best) {
			best = f.Quality
		}
	}
	return best
}
