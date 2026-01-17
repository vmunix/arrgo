// Package search handles indexer queries and release matching.
package search

import (
	"context"
	"time"
)

// Release represents a search result from an indexer.
type Release struct {
	Title       string
	Indexer     string
	GUID        string
	DownloadURL string
	Size        int64
	Quality     string
	Source      string
	Codec       string
	PublishDate time.Time
	Score       int // Calculated match score
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

// Searcher queries indexers for releases.
type Searcher interface {
	Search(ctx context.Context, q Query) ([]*Release, error)
}

// ProwlarrSearcher searches via Prowlarr API.
type ProwlarrSearcher struct {
	baseURL string
	apiKey  string
}

// NewProwlarrSearcher creates a new Prowlarr client.
func NewProwlarrSearcher(baseURL, apiKey string) *ProwlarrSearcher {
	return &ProwlarrSearcher{
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// Search queries Prowlarr for releases.
func (p *ProwlarrSearcher) Search(ctx context.Context, q Query) ([]*Release, error) {
	// TODO: implement Prowlarr API call
	// TODO: parse release names for quality/source
	// TODO: score releases against quality profile
	return nil, nil
}
