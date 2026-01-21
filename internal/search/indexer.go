package search

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/vmunix/arrgo/pkg/newznab"
	"github.com/vmunix/arrgo/pkg/release"
)

// ErrNoIndexers is returned when no indexers are configured.
var ErrNoIndexers = errors.New("no indexers configured")

// IndexerPool manages multiple Newznab indexers and searches them in parallel.
type IndexerPool struct {
	clients []*newznab.Client
	log     *slog.Logger
}

// NewIndexerPool creates a pool from the given clients.
func NewIndexerPool(clients []*newznab.Client, log *slog.Logger) *IndexerPool {
	return &IndexerPool{clients: clients, log: log}
}

// Search queries all indexers in parallel and merges results.
// Returns releases from all indexers and any errors encountered.
func (p *IndexerPool) Search(ctx context.Context, q Query) ([]Release, []error) {
	// Normalize query for better indexer matching (e.g., & → and)
	searchText := release.NormalizeSearchQuery(q.Text)
	p.log.Debug("search started", "query", searchText, "original", q.Text, "type", q.Type, "indexers", len(p.clients))
	start := time.Now()

	if len(p.clients) == 0 {
		return nil, []error{ErrNoIndexers}
	}

	// Determine categories based on content type
	var categories []int
	switch q.Type {
	case "movie":
		categories = []int{2000, 2010, 2020, 2030, 2040, 2045, 2050}
	case "series":
		categories = []int{5000, 5010, 5020, 5030, 5040, 5045, 5050, 5070}
	}

	type result struct {
		releases []newznab.Release
		err      error
	}

	results := make(chan result, len(p.clients))
	var wg sync.WaitGroup

	// Query all indexers in parallel
	for _, client := range p.clients {
		wg.Add(1)
		go func(c *newznab.Client) {
			defer wg.Done()
			indexerStart := time.Now()
			releases, err := c.Search(ctx, searchText, categories)
			if err != nil {
				p.log.Warn("indexer failed", "indexer", c.Name(), "error", err, "duration_ms", time.Since(indexerStart).Milliseconds())
			} else {
				p.log.Debug("indexer returned", "indexer", c.Name(), "results", len(releases), "duration_ms", time.Since(indexerStart).Milliseconds())
			}
			results <- result{releases: releases, err: err}
		}(client)
	}

	// Close results channel when all done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allReleases []Release
	var errs []error

	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		for _, nr := range r.releases {
			allReleases = append(allReleases, Release{
				Title:       nr.Title,
				GUID:        nr.GUID,
				DownloadURL: nr.DownloadURL,
				Size:        nr.Size,
				PublishDate: nr.PublishDate,
				Indexer:     nr.Indexer,
			})
		}
	}

	p.log.Info("search complete", "query", searchText, "results", len(allReleases), "errors", len(errs), "duration_ms", time.Since(start).Milliseconds())
	return allReleases, errs
}
