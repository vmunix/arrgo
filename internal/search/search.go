// Package search handles indexer queries and release matching.
package search

import (
	"context"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/arrgo/arrgo/pkg/release"
)

// sequelPattern matches sequel indicators in titles.
// Matches: Part II, Part III, Part 2, Part 3, II, III, 2, 3 (at word boundaries)
var sequelPattern = regexp.MustCompile(`(?i)\b(part\s+)?(II|III|IV|V|2|3|4|5)\b`)

// hasSequelMismatch returns true if the release title has sequel indicators
// that are not present in the query. This helps rank the original film higher
// when searching for e.g. "Back to the Future" without specifying a sequel.
func hasSequelMismatch(query, releaseTitle string) bool {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(releaseTitle)

	// Find sequel indicators in release title
	titleSequels := sequelPattern.FindAllString(titleLower, -1)
	if len(titleSequels) == 0 {
		return false // No sequel indicators in release
	}

	// Check if query also has sequel indicators
	querySequels := sequelPattern.FindAllString(queryLower, -1)
	if len(querySequels) > 0 {
		return false // Query specifies a sequel, no mismatch
	}

	// Release has sequel indicators but query doesn't
	return true
}

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

// IndexerAPI defines the interface for indexer operations.
// This allows for easy mocking in tests.
type IndexerAPI interface {
	Search(ctx context.Context, q Query) ([]Release, []error)
}

// Searcher orchestrates searches across indexers with quality scoring.
type Searcher struct {
	indexers IndexerAPI
	scorer   *Scorer
	log      *slog.Logger
}

// NewSearcher creates a new Searcher with the given indexer pool and scorer.
func NewSearcher(indexers IndexerAPI, scorer *Scorer, log *slog.Logger) *Searcher {
	return &Searcher{
		indexers: indexers,
		scorer:   scorer,
		log:      log,
	}
}

// Search queries the indexers for releases matching the query,
// parses quality information, scores against the profile,
// filters out zero-score releases, and sorts by score descending.
func (s *Searcher) Search(ctx context.Context, q Query, profile string) (*SearchResult, error) {
	s.log.Info("search started", "query", q.Text, "type", q.Type, "profile", profile)

	result := &SearchResult{
		Releases: make([]*Release, 0),
		Errors:   make([]error, 0),
	}

	// Query the indexers
	releases, errs := s.indexers.Search(ctx, q)
	result.Errors = append(result.Errors, errs...)

	// Process each release: parse, score, and filter
	for _, rel := range releases {
		// Parse quality info from release name
		info := release.Parse(rel.Title)

		// Score against the quality profile
		score := s.scorer.Score(*info, profile)

		// Filter out releases with score 0
		if score == 0 {
			continue
		}

		// Penalize sequels when query doesn't specify one
		// This ranks "Back to the Future" (1985) above "Part II" and "Part III"
		// Use negative score to rank below non-sequels with same quality
		if hasSequelMismatch(q.Text, info.Title) {
			score = -score // Negative score, still included but ranked last
		}

		// Create a copy with quality info and score
		r := &Release{
			Title:       rel.Title,
			Indexer:     rel.Indexer,
			GUID:        rel.GUID,
			DownloadURL: rel.DownloadURL,
			Size:        rel.Size,
			PublishDate: rel.PublishDate,
			Quality:     info,
			Score:       score,
		}

		result.Releases = append(result.Releases, r)
	}

	s.log.Debug("scoring complete", "raw", len(releases), "filtered", len(result.Releases))

	// Sort by score descending (stable sort to preserve order for equal scores)
	sort.SliceStable(result.Releases, func(i, j int) bool {
		return result.Releases[i].Score > result.Releases[j].Score
	})

	return result, nil
}
