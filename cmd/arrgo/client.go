package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client wraps HTTP calls to the arrgo server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new arrgo API client.
func NewClient(serverURL string) *Client {
	return &Client{
		baseURL: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) get(path string, result any) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) post(path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// API response types (mirror server types)

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type DownloadResponse struct {
	ID          int64   `json:"id"`
	ContentID   int64   `json:"content_id"`
	EpisodeID   *int64  `json:"episode_id,omitempty"`
	Client      string  `json:"client"`
	ClientID    string  `json:"client_id"`
	Status      string  `json:"status"`
	ReleaseName string  `json:"release_name"`
	Indexer     string  `json:"indexer"`
	AddedAt     string  `json:"added_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

type ListDownloadsResponse struct {
	Items []DownloadResponse `json:"items"`
	Total int                `json:"total"`
}

type ReleaseResponse struct {
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
	GUID        string `json:"guid"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	PublishDate string `json:"publish_date"`
	Quality     string `json:"quality"`
	Score       int    `json:"score"`
}

type SearchResponse struct {
	Releases []ReleaseResponse `json:"releases"`
	Errors   []string          `json:"errors,omitempty"`
}

type ContentResponse struct {
	ID             int64  `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	Status         string `json:"status"`
	QualityProfile string `json:"quality_profile"`
	RootPath       string `json:"root_path"`
}

type ListContentResponse struct {
	Items  []ContentResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

type GrabResponse struct {
	DownloadID int64  `json:"download_id"`
	Status     string `json:"status"`
}

// API methods

func (c *Client) Status() (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.get("/api/v1/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Downloads(activeOnly bool) (*ListDownloadsResponse, error) {
	path := "/api/v1/downloads"
	if activeOnly {
		path += "?active=true"
	}
	var resp ListDownloadsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Search(query, contentType, profile string) (*SearchResponse, error) {
	req := map[string]any{
		"query": query,
	}
	if contentType != "" {
		req["type"] = contentType
	}
	if profile != "" {
		req["profile"] = profile
	}

	var resp SearchResponse
	if err := c.post("/api/v1/search", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) AddContent(contentType, title string, year int, profile string) (*ContentResponse, error) {
	req := map[string]any{
		"type":            contentType,
		"title":           title,
		"year":            year,
		"quality_profile": profile,
	}

	var resp ContentResponse
	if err := c.post("/api/v1/content", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) FindContent(contentType, title string, year int) (*ContentResponse, error) {
	path := fmt.Sprintf("/api/v1/content?type=%s&title=%s&year=%d&limit=1",
		contentType, url.QueryEscape(title), year)

	var resp ListContentResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, nil // Not found
	}
	return &resp.Items[0], nil
}

func (c *Client) Grab(contentID int64, downloadURL, title, indexer string) (*GrabResponse, error) {
	req := map[string]any{
		"content_id":   contentID,
		"download_url": downloadURL,
		"title":        title,
		"indexer":      indexer,
	}

	var resp GrabResponse
	if err := c.post("/api/v1/grab", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
