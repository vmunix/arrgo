package importer

import (
	"fmt"
	"path/filepath"

	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/pkg/release"
)

// FileMatch represents a matched file-to-episode pairing.
type FileMatch struct {
	FilePath string
	Episode  *library.Episode
}

// MatchFileToEpisode finds the episode that matches a filename.
// Returns error if no match is found.
func MatchFileToEpisode(filename string, episodes []*library.Episode) (*library.Episode, error) {
	info := release.Parse(filepath.Base(filename))

	if info.Season == 0 || len(info.Episodes) == 0 {
		return nil, fmt.Errorf("cannot parse episode info from %s", filename)
	}

	// Match first episode in the file (for multi-episode files like S01E05E06)
	targetEp := info.Episodes[0]

	for _, ep := range episodes {
		if ep.Season == info.Season && ep.Episode == targetEp {
			return ep, nil
		}
	}

	return nil, fmt.Errorf("no matching episode for S%02dE%02d in %s", info.Season, targetEp, filename)
}

// MatchFilesToEpisodes matches multiple files to episodes.
// Returns matched pairs and a list of unmatched files.
func MatchFilesToEpisodes(files []string, episodes []*library.Episode) ([]FileMatch, []string) {
	matches := make([]FileMatch, 0, len(files))
	var unmatched []string

	for _, f := range files {
		ep, err := MatchFileToEpisode(f, episodes)
		if err != nil {
			unmatched = append(unmatched, f)
			continue
		}
		matches = append(matches, FileMatch{FilePath: f, Episode: ep})
	}

	return matches, unmatched
}

// MatchFileToSeason parses a file and returns episode info for season pack handling.
// Unlike MatchFileToEpisode, this doesn't require pre-existing episode records.
// Returns (season, episodeNumber, error).
func MatchFileToSeason(filename string) (int, int, error) {
	info := release.Parse(filepath.Base(filename))

	if info.Season == 0 {
		return 0, 0, fmt.Errorf("cannot parse season from %s", filename)
	}
	if len(info.Episodes) == 0 {
		return 0, 0, fmt.Errorf("cannot parse episode number from %s", filename)
	}

	return info.Season, info.Episodes[0], nil
}
