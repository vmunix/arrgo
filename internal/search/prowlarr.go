package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// prowlarrRelease is the JSON response struct from Prowlarr API.
type prowlarrRelease struct {
	Title       string `json:"title"`
	GUID        string `json:"guid"`
	Indexer     string `json:"indexer"`
	DownloadURL string `json:"downloadUrl"`
	Size        int64  `json:"size"`
	PublishDate string `json:"publishDate"`
}

// ProwlarrRelease is our internal representation with parsed time.
type ProwlarrRelease struct {
	Title       string
	GUID        string
	Indexer     string
	DownloadURL string
	Size        int64
	PublishDate time.Time
}

// ProwlarrClient is an HTTP client for the Prowlarr API.
type ProwlarrClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewProwlarrClient creates a new Prowlarr API client.
func NewProwlarrClient(baseURL, apiKey string) *ProwlarrClient {
	return &ProwlarrClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Search queries Prowlarr for releases matching the given query.
func (c *ProwlarrClient) Search(ctx context.Context, q Query) ([]ProwlarrRelease, error) {
	// Build the request URL
	reqURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	reqURL.Path = "/api/v1/search"

	// Build query parameters
	params := url.Values{}

	// Set categories based on content type
	switch q.Type {
	case "movie":
		params.Set("categories", "2000")
	case "series":
		params.Set("categories", "5000")
	}

	// Add search parameters
	if q.Text != "" {
		params.Set("query", q.Text)
	}
	if q.TMDBID != nil {
		params.Set("tmdbId", strconv.FormatInt(*q.TMDBID, 10))
	}
	if q.TVDBID != nil {
		params.Set("tvdbId", strconv.FormatInt(*q.TVDBID, 10))
	}

	reqURL.RawQuery = params.Encode()

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the API key header
	req.Header.Set("X-Api-Key", c.apiKey)

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProwlarrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle error responses
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidAPIKey
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response
	var releases []prowlarrRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to our internal type
	result := make([]ProwlarrRelease, len(releases))
	for i, r := range releases {
		result[i] = ProwlarrRelease{
			Title:       r.Title,
			GUID:        r.GUID,
			Indexer:     r.Indexer,
			DownloadURL: r.DownloadURL,
			Size:        r.Size,
			PublishDate: parseTime(r.PublishDate),
		}
	}

	return result, nil
}

// parseTime parses a time string, returning zero time on failure.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
