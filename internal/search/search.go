// Package search handles indexer queries and release matching.
package search

import (
	"context"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vmunix/arrgo/pkg/release"
)

// sequelPattern matches sequel indicators in titles.
// Roman numerals (II, III, IV, V) can stand alone since they're unambiguous.
// Arabic numerals (2, 3, 4, 5) require "Part" prefix to avoid matching "5.1" audio specs.
var sequelPattern = regexp.MustCompile(`(?i)\b(part\s+(II|III|IV|V|2|3|4|5)|(II|III|IV|V))\b`)

// seasonEpisodePattern matches season/episode indicators to extract the title portion.
var seasonEpisodePattern = regexp.MustCompile(`(?i)\s*S\d{1,2}(E\d{1,2})?.*$`)

// yearPattern matches year suffix to extract the title portion.
var yearPattern = regexp.MustCompile(`\s+\d{4}\s*$`)

// normalizeSequelNumber converts Roman numerals to Arabic for comparison.
func normalizeSequelNumber(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "part ")
	s = strings.TrimPrefix(s, "part")
	s = strings.TrimSpace(s)
	switch s {
	case "ii", "2":
		return 2
	case "iii", "3":
		return 3
	case "iv", "4":
		return 4
	case "v", "5":
		return 5
	default:
		return 0
	}
}

// extractQueryTitle extracts the content title from a search query.
// "The Walking Dead S03" -> "the walking dead"
// "Dune 2024" -> "dune"
func extractQueryTitle(query string) string {
	title := query
	// Remove season/episode info
	title = seasonEpisodePattern.ReplaceAllString(title, "")
	// Remove year suffix
	title = yearPattern.ReplaceAllString(title, "")
	// Normalize: lowercase, trim
	return strings.ToLower(strings.TrimSpace(title))
}

// normalizeTitle normalizes a title for comparison.
// Removes common articles, punctuation, and extra spaces.
func normalizeTitle(title string) string {
	s := strings.ToLower(title)
	// Remove punctuation except spaces
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			return r
		}
		return -1
	}, s)
	// Collapse multiple spaces
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

// titleMatches checks if a release title matches the query title.
// Returns true if they match, false if they're different content.
// This prevents "Fear the Walking Dead" from matching "The Walking Dead".
func titleMatches(queryTitle, releaseTitle string) bool {
	// Normalize both titles
	query := normalizeTitle(queryTitle)
	rel := normalizeTitle(releaseTitle)

	// Exact match
	if query == rel {
		return true
	}

	// Check if one is a prefix of the other with word boundary
	// "The Walking Dead" should NOT match "The Walking Dead Fear" or "Fear The Walking Dead"
	queryWords := strings.Fields(query)
	relWords := strings.Fields(rel)

	// If release has more words at the beginning, it's a different show
	// e.g., "Fear the Walking Dead" vs "The Walking Dead"
	if len(relWords) > len(queryWords) {
		// Check if query words appear at the end of release words (prefix mismatch)
		// "Fear the Walking Dead" has "the walking dead" at end but "fear" at start
		offset := len(relWords) - len(queryWords)
		suffixMatch := true
		for i, w := range queryWords {
			if relWords[offset+i] != w {
				suffixMatch = false
				break
			}
		}
		if suffixMatch {
			// Release has extra words at the beginning - different show
			return false
		}
	}

	// Check if query is a subset of release (release has extra words at end)
	// This is less common but handle it for completeness
	if len(queryWords) > 0 && len(relWords) >= len(queryWords) {
		prefixMatch := true
		for i, w := range queryWords {
			if i >= len(relWords) || relWords[i] != w {
				prefixMatch = false
				break
			}
		}
		if prefixMatch && len(relWords) > len(queryWords) {
			// Release has extra words at the end - might be different show
			// e.g., "The Walking Dead: World Beyond" vs "The Walking Dead"
			// Be conservative and allow this, but could be stricter
			return true
		}
	}

	// If query words all appear in release words in order, it's likely a match
	// This handles cases like "The Walking Dead" matching "The Walking Dead 2010"
	if len(queryWords) <= len(relWords) {
		allMatch := true
		for i, w := range queryWords {
			if i >= len(relWords) || relWords[i] != w {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}

	return false
}

// hasSequelMismatch returns true if the release title has sequel indicators
// that don't match the query. This helps rank:
// - Original films higher when no sequel specified
// - Correct sequel higher when sequel is specified (Part 2 matches Part II)
func hasSequelMismatch(query, releaseTitle string) bool {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(releaseTitle)

	// Find sequel indicators in release title
	titleMatches := sequelPattern.FindAllString(titleLower, -1)
	if len(titleMatches) == 0 {
		return false // No sequel indicators in release, no mismatch
	}

	// Find sequel indicators in query
	queryMatches := sequelPattern.FindAllString(queryLower, -1)
	if len(queryMatches) == 0 {
		return true // Query has no sequel, but release does = mismatch
	}

	// Both have sequel indicators - check if they match
	queryNum := normalizeSequelNumber(queryMatches[0])
	titleNum := normalizeSequelNumber(titleMatches[0])

	// If numbers don't match, it's a mismatch (e.g., query "Part 2" vs release "Part III")
	return queryNum != titleNum
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

// Result contains the results of a search operation.
type Result struct {
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
func (s *Searcher) Search(ctx context.Context, q Query, profile string) (*Result, error) {
	s.log.Info("search started", "query", q.Text, "type", q.Type, "profile", profile)

	result := &Result{
		Releases: make([]*Release, 0),
		Errors:   make([]error, 0),
	}

	// Query the indexers
	releases, errs := s.indexers.Search(ctx, q)
	result.Errors = append(result.Errors, errs...)

	// Extract the query title for matching
	queryTitle := extractQueryTitle(q.Text)

	// Process each release: parse, score, and filter
	for _, rel := range releases {
		// Parse quality info from release name
		info := release.Parse(rel.Title)

		// Filter out releases with mismatched titles
		// This prevents "Fear the Walking Dead" from matching "The Walking Dead"
		if queryTitle != "" && info.Title != "" && !titleMatches(queryTitle, info.Title) {
			continue
		}

		// Score against the quality profile
		score := s.scorer.Score(*info, profile)

		// Filter out releases with score 0
		if score == 0 {
			continue
		}

		// For series season requests: filter out individual episodes, prefer season packs
		// When searching for a season (Season set, Episode not set), we want season packs
		if q.Type == "series" && q.Season != nil && q.Episode == nil {
			// If release has an episode number (not a season pack), skip it
			if info.Episode > 0 && !info.IsCompleteSeason {
				continue
			}
			// Verify the release is for the right season
			if info.Season > 0 && info.Season != *q.Season {
				continue
			}
		}

		// Penalize sequels when query doesn't specify one
		// This ranks "Back to the Future" (1985) above "Part II" and "Part III"
		// Use negative score to rank below non-sequels with same quality
		// Note: Use rel.Title (raw) not info.Title (parsed) since parser strips sequel info
		if hasSequelMismatch(q.Text, rel.Title) {
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
