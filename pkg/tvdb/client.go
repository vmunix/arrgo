package tvdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const defaultBaseURL = "https://api4.thetvdb.com/v4"

// Sentinel errors for TVDB API responses.
var (
	ErrNotFound     = errors.New("series not found")
	ErrUnauthorized = errors.New("unauthorized: invalid or expired API key")
	ErrRateLimited  = errors.New("rate limited: too many requests")
)

// Client is a TVDB API v4 client with JWT authentication.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	log        *slog.Logger

	// JWT token management (thread-safe)
	mu    sync.RWMutex
	token string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets a custom base URL (for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
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
		c.log = log.With("component", "tvdb")
	}
}

// New creates a new TVDB API v4 client.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// login authenticates with TVDB and stores the JWT token.
func (c *Client) login(ctx context.Context) error {
	body := map[string]string{"apikey": c.apiKey}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal login body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}

	if loginResp.Data.Token == "" {
		return errors.New("login response missing token")
	}

	c.mu.Lock()
	c.token = loginResp.Data.Token
	c.mu.Unlock()

	if c.log != nil {
		c.log.Debug("authenticated with TVDB")
	}

	return nil
}

// ensureToken ensures we have a valid JWT token, logging in if necessary.
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	hasToken := c.token != ""
	c.mu.RUnlock()

	if !hasToken {
		return c.login(ctx)
	}
	return nil
}

// doRequest performs an authenticated request, handling token refresh on 401.
func (c *Client) doRequest(ctx context.Context, method, endpoint string) (*http.Response, error) {
	// Ensure we have a token
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	// Try the request
	resp, err := c.doAuthenticatedRequest(ctx, method, endpoint)
	if err != nil {
		return nil, err
	}

	// If unauthorized, refresh token and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		if c.log != nil {
			c.log.Debug("token expired, refreshing")
		}

		// Clear token and re-login
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()

		if err := c.login(ctx); err != nil {
			return nil, err
		}

		// Retry request
		return c.doAuthenticatedRequest(ctx, method, endpoint)
	}

	return resp, nil
}

// doAuthenticatedRequest performs a single authenticated request.
func (c *Client) doAuthenticatedRequest(ctx context.Context, method, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	return resp, nil
}

// Search searches for series by name.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	start := time.Now()

	endpoint := "/search?query=" + url.QueryEscape(query) + "&type=series"
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResp.Data))
	for _, item := range searchResp.Data {
		// Parse TVDB ID from string
		tvdbID, _ := strconv.Atoi(item.TVDBID)
		if tvdbID == 0 {
			// Try objectID as fallback (format: "series-12345")
			if len(item.ObjectID) > 7 && item.ObjectID[:7] == "series-" {
				tvdbID, _ = strconv.Atoi(item.ObjectID[7:])
			}
		}

		// Parse year from string
		year, _ := strconv.Atoi(item.Year)

		results = append(results, SearchResult{
			ID:       tvdbID,
			Name:     item.Name,
			Year:     year,
			Status:   item.Status,
			Overview: item.Overview,
			Network:  item.Network,
		})
	}

	if c.log != nil {
		c.log.Debug("search completed", "query", query, "results", len(results), "duration_ms", time.Since(start).Milliseconds())
	}

	return results, nil
}

// GetSeries fetches series metadata by TVDB ID.
func (c *Client) GetSeries(ctx context.Context, id int) (*Series, error) {
	start := time.Now()

	endpoint := fmt.Sprintf("/series/%d", id)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		if c.log != nil && errors.Is(err, ErrNotFound) {
			c.log.Debug("series not found", "id", id)
		}
		return nil, err
	}

	var seriesResp seriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&seriesResp); err != nil {
		return nil, fmt.Errorf("decode series response: %w", err)
	}

	// Extract year from firstAired (format: YYYY-MM-DD)
	var year int
	if len(seriesResp.Data.FirstAired) >= 4 {
		year, _ = strconv.Atoi(seriesResp.Data.FirstAired[:4])
	}

	series := &Series{
		ID:       seriesResp.Data.ID,
		Name:     seriesResp.Data.Name,
		Year:     year,
		Status:   seriesResp.Data.Status.Name,
		Overview: seriesResp.Data.Overview,
	}

	if c.log != nil {
		c.log.Debug("fetched series", "id", id, "name", series.Name, "duration_ms", time.Since(start).Milliseconds())
	}

	return series, nil
}

// GetEpisodes fetches all episodes for a series, handling pagination automatically.
func (c *Client) GetEpisodes(ctx context.Context, seriesID int) ([]Episode, error) {
	start := time.Now()

	var allEpisodes []Episode
	page := 0

	for {
		endpoint := fmt.Sprintf("/series/%d/episodes/default?page=%d", seriesID, page)
		resp, err := c.doRequest(ctx, http.MethodGet, endpoint)
		if err != nil {
			return nil, err
		}

		if err := c.checkResponse(resp); err != nil {
			resp.Body.Close()
			return nil, err
		}

		var episodesResp episodesResponse
		if err := json.NewDecoder(resp.Body).Decode(&episodesResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode episodes response: %w", err)
		}
		resp.Body.Close()

		// Convert episodes
		for _, ep := range episodesResp.Data.Episodes {
			// Parse air date (format: YYYY-MM-DD)
			var airDate time.Time
			if ep.Aired != "" {
				airDate, _ = time.Parse("2006-01-02", ep.Aired)
			}

			allEpisodes = append(allEpisodes, Episode{
				ID:       ep.ID,
				Season:   ep.SeasonNumber,
				Episode:  ep.Number,
				Name:     ep.Name,
				Overview: ep.Overview,
				AirDate:  airDate,
				Runtime:  ep.Runtime,
			})
		}

		// Check for more pages
		if episodesResp.Links.Next == "" {
			break
		}
		page++

		// Safety limit to prevent infinite loops
		if page > 100 {
			if c.log != nil {
				c.log.Warn("hit pagination limit", "series_id", seriesID, "pages", page)
			}
			break
		}
	}

	if c.log != nil {
		c.log.Debug("fetched episodes", "series_id", seriesID, "count", len(allEpisodes), "pages", page+1, "duration_ms", time.Since(start).Milliseconds())
	}

	return allEpisodes, nil
}

// checkResponse checks the HTTP response for errors and returns appropriate sentinel errors.
func (c *Client) checkResponse(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("TVDB API error: %s", resp.Status)
	}
}
