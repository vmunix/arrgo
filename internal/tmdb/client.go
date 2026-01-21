package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.themoviedb.org"
const defaultCacheTTL = 24 * time.Hour

// ErrNotFound is returned when a movie doesn't exist in TMDB.
var ErrNotFound = errors.New("movie not found")

// Client is a TMDB API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	cache      *cache
	log        *slog.Logger
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom base URL (for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithCacheTTL sets the cache TTL.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.cache = newCache(ttl)
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithLogger sets a logger for debug output.
func WithLogger(log *slog.Logger) Option {
	return func(c *Client) {
		c.log = log.With("component", "tmdb")
	}
}

// NewClient creates a new TMDB client.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: newCache(defaultCacheTTL),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetMovie fetches movie metadata by TMDB ID.
func (c *Client) GetMovie(ctx context.Context, tmdbID int64) (*Movie, error) {
	// Check cache first
	if movie, ok := c.cache.get(tmdbID); ok {
		if c.log != nil {
			c.log.Debug("cache hit", "tmdb_id", tmdbID, "title", movie.Title)
		}
		return movie, nil
	}

	start := time.Now()

	// Build request
	url := fmt.Sprintf("%s/3/movie/%d?api_key=%s", c.baseURL, tmdbID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Execute
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Debug("request failed", "tmdb_id", tmdbID, "error", err)
		}
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Handle errors
	if resp.StatusCode == http.StatusNotFound {
		if c.log != nil {
			c.log.Debug("not found", "tmdb_id", tmdbID)
		}
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		if c.log != nil {
			c.log.Debug("api error", "tmdb_id", tmdbID, "status", resp.StatusCode)
		}
		return nil, fmt.Errorf("TMDB API error: %s", resp.Status)
	}

	// Decode
	var movie Movie
	if err := json.NewDecoder(resp.Body).Decode(&movie); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if c.log != nil {
		c.log.Debug("fetched movie", "tmdb_id", tmdbID, "title", movie.Title, "duration_ms", time.Since(start).Milliseconds())
	}

	// Cache and return
	c.cache.set(tmdbID, &movie)
	return &movie, nil
}
