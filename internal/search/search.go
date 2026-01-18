// Package search handles indexer queries and release matching.
package search

import (
	"context"
	"sort"
	"time"

	"github.com/arrgo/arrgo/pkg/release"
)

// Release represents a search result from an indexer.
type Release struct {
	Title       string
	Indexer     string
	GUID        string
	DownloadURL string
	Size        int64
	PublishDate time.Time
	Quality     *release.Info // Parsed quality info
	Score       int           // Match score (higher is better)
}

// Query specifies what to search for.
type Query struct {
	ContentID int64  // If searching for known content
	Text      string // Free text search
	Type      string // "movie" or "series"
	TMDBID    *int64
	TVDBID    *int64
	Season    *int
	Episode   *int
}

// SearchResult contains the results of a search operation.
type SearchResult struct {
	Releases []*Release
	Errors   []error
}

// ProwlarrAPI defines the interface for Prowlarr API operations.
// This allows for easy mocking in tests.
type ProwlarrAPI interface {
	Search(ctx context.Context, q Query) ([]ProwlarrRelease, error)
}

// Searcher orchestrates searches across indexers with quality scoring.
type Searcher struct {
	client ProwlarrAPI
	scorer *Scorer
}

// NewSearcher creates a new Searcher with the given Prowlarr client and scorer.
func NewSearcher(client ProwlarrAPI, scorer *Scorer) *Searcher {
	return &Searcher{
		client: client,
		scorer: scorer,
	}
}

// Search queries the indexer for releases matching the query,
// parses quality information, scores against the profile,
// filters out zero-score releases, and sorts by score descending.
func (s *Searcher) Search(ctx context.Context, q Query, profile string) (*SearchResult, error) {
	result := &SearchResult{
		Releases: make([]*Release, 0),
		Errors:   make([]error, 0),
	}

	// Query the indexer
	prowlarrReleases, err := s.client.Search(ctx, q)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result, nil
	}

	// Process each release: parse, score, and filter
	for _, pr := range prowlarrReleases {
		// Parse quality info from release name
		info := release.Parse(pr.Title)

		// Score against the quality profile
		score := s.scorer.Score(*info, profile)

		// Filter out releases with score 0
		if score == 0 {
			continue
		}

		// Convert to our Release type
		r := &Release{
			Title:       pr.Title,
			Indexer:     pr.Indexer,
			GUID:        pr.GUID,
			DownloadURL: pr.DownloadURL,
			Size:        pr.Size,
			PublishDate: pr.PublishDate,
			Quality:     info,
			Score:       score,
		}

		result.Releases = append(result.Releases, r)
	}

	// Sort by score descending (stable sort to preserve order for equal scores)
	sort.SliceStable(result.Releases, func(i, j int) bool {
		return result.Releases[i].Score > result.Releases[j].Score
	})

	return result, nil
}
