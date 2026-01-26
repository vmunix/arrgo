package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/vmunix/arrgo/pkg/tvdb"
)

const (
	// Cache TTLs
	seriesTTL  = 7 * 24 * time.Hour // 7 days
	episodeTTL = 24 * time.Hour     // 24 hours
	searchTTL  = time.Hour          // 1 hour
)

// Cache key prefixes
const (
	keyPrefixSearch   = "tvdb:search:"
	keyPrefixSeries   = "tvdb:series:"
	keyPrefixEpisodes = "tvdb:episodes:"
)

// TVDBService provides cached access to TVDB metadata.
type TVDBService struct {
	client *tvdb.Client
	cache  *Cache
	log    *slog.Logger
}

// NewTVDBService creates a new TVDB service.
func NewTVDBService(client *tvdb.Client, cache *Cache, log *slog.Logger) *TVDBService {
	return &TVDBService{
		client: client,
		cache:  cache,
		log:    log,
	}
}

// Search searches for series by name (cached).
func (s *TVDBService) Search(ctx context.Context, query string) ([]tvdb.SearchResult, error) {
	key := keyPrefixSearch + query

	// Check cache first
	if data, ok := s.cache.Get(ctx, key); ok {
		var results []tvdb.SearchResult
		if err := json.Unmarshal(data, &results); err == nil {
			if s.log != nil {
				s.log.Debug("cache hit for search", "query", query, "results", len(results))
			}
			return results, nil
		}
		// If unmarshal fails, treat as cache miss and fetch fresh data
		if s.log != nil {
			s.log.Warn("failed to unmarshal cached search results", "query", query)
		}
	}

	// Cache miss - call API
	if s.log != nil {
		s.log.Debug("cache miss for search, calling API", "query", query)
	}

	results, err := s.client.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// Cache the results
	data, err := json.Marshal(results)
	if err != nil {
		// Log but don't fail the operation
		if s.log != nil {
			s.log.Warn("failed to marshal search results for cache", "query", query, "error", err)
		}
		return results, nil
	}

	if err := s.cache.Set(ctx, key, data, searchTTL); err != nil {
		if s.log != nil {
			s.log.Warn("failed to cache search results", "query", query, "error", err)
		}
	}

	return results, nil
}

// GetSeries fetches series metadata by TVDB ID (cached).
func (s *TVDBService) GetSeries(ctx context.Context, tvdbID int) (*tvdb.Series, error) {
	key := fmt.Sprintf("%s%d", keyPrefixSeries, tvdbID)

	// Check cache first
	if data, ok := s.cache.Get(ctx, key); ok {
		var series tvdb.Series
		if err := json.Unmarshal(data, &series); err == nil {
			if s.log != nil {
				s.log.Debug("cache hit for series", "tvdb_id", tvdbID, "name", series.Name)
			}
			return &series, nil
		}
		// If unmarshal fails, treat as cache miss and fetch fresh data
		if s.log != nil {
			s.log.Warn("failed to unmarshal cached series", "tvdb_id", tvdbID)
		}
	}

	// Cache miss - call API
	if s.log != nil {
		s.log.Debug("cache miss for series, calling API", "tvdb_id", tvdbID)
	}

	series, err := s.client.GetSeries(ctx, tvdbID)
	if err != nil {
		return nil, fmt.Errorf("get series: %w", err)
	}

	// Cache the result
	data, err := json.Marshal(series)
	if err != nil {
		// Log but don't fail the operation
		if s.log != nil {
			s.log.Warn("failed to marshal series for cache", "tvdb_id", tvdbID, "error", err)
		}
		return series, nil
	}

	if err := s.cache.Set(ctx, key, data, seriesTTL); err != nil {
		if s.log != nil {
			s.log.Warn("failed to cache series", "tvdb_id", tvdbID, "error", err)
		}
	}

	return series, nil
}

// GetEpisodes fetches all episodes for a series (cached).
func (s *TVDBService) GetEpisodes(ctx context.Context, tvdbID int) ([]tvdb.Episode, error) {
	key := fmt.Sprintf("%s%d", keyPrefixEpisodes, tvdbID)

	// Check cache first
	if data, ok := s.cache.Get(ctx, key); ok {
		var episodes []tvdb.Episode
		if err := json.Unmarshal(data, &episodes); err == nil {
			if s.log != nil {
				s.log.Debug("cache hit for episodes", "tvdb_id", tvdbID, "count", len(episodes))
			}
			return episodes, nil
		}
		// If unmarshal fails, treat as cache miss and fetch fresh data
		if s.log != nil {
			s.log.Warn("failed to unmarshal cached episodes", "tvdb_id", tvdbID)
		}
	}

	// Cache miss - call API
	if s.log != nil {
		s.log.Debug("cache miss for episodes, calling API", "tvdb_id", tvdbID)
	}

	episodes, err := s.client.GetEpisodes(ctx, tvdbID)
	if err != nil {
		return nil, fmt.Errorf("get episodes: %w", err)
	}

	// Cache the results
	data, err := json.Marshal(episodes)
	if err != nil {
		// Log but don't fail the operation
		if s.log != nil {
			s.log.Warn("failed to marshal episodes for cache", "tvdb_id", tvdbID, "error", err)
		}
		return episodes, nil
	}

	if err := s.cache.Set(ctx, key, data, episodeTTL); err != nil {
		if s.log != nil {
			s.log.Warn("failed to cache episodes", "tvdb_id", tvdbID, "error", err)
		}
	}

	return episodes, nil
}

// InvalidateSeries removes cached data for a series.
// This clears the series metadata and episodes cache entries.
func (s *TVDBService) InvalidateSeries(ctx context.Context, tvdbID int) error {
	seriesKey := fmt.Sprintf("%s%d", keyPrefixSeries, tvdbID)
	episodesKey := fmt.Sprintf("%s%d", keyPrefixEpisodes, tvdbID)

	var errs []error

	if err := s.cache.Delete(ctx, seriesKey); err != nil {
		errs = append(errs, fmt.Errorf("delete series cache: %w", err))
	}

	if err := s.cache.Delete(ctx, episodesKey); err != nil {
		errs = append(errs, fmt.Errorf("delete episodes cache: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalidate series %d: %v", tvdbID, errs)
	}

	if s.log != nil {
		s.log.Debug("invalidated series cache", "tvdb_id", tvdbID)
	}

	return nil
}
