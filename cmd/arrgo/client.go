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

func (c *Client) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("request creation failed: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// API response types (mirror server types)

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type DashboardResponse struct {
	Version     string `json:"version"`
	Connections struct {
		Server  bool `json:"server"`
		Plex    bool `json:"plex"`
		SABnzbd bool `json:"sabnzbd"`
	} `json:"connections"`
	Downloads struct {
		Queued      int `json:"queued"`
		Downloading int `json:"downloading"`
		Completed   int `json:"completed"`
		Importing   int `json:"importing"`
		Imported    int `json:"imported"`
		Cleaned     int `json:"cleaned"`
		Failed      int `json:"failed"`
	} `json:"downloads"`
	Stuck struct {
		Count     int   `json:"count"`
		Threshold int64 `json:"threshold_minutes"`
	} `json:"stuck"`
	Library struct {
		Movies int `json:"movies"`
		Series int `json:"series"`
	} `json:"library"`
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
	// Live status fields
	Progress *float64 `json:"progress,omitempty"`
	Size     *int64   `json:"size,omitempty"`
	Speed    *int64   `json:"speed,omitempty"`
	ETA      *string  `json:"eta,omitempty"`
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

type ImportRequest struct {
	DownloadID *int64 `json:"download_id,omitempty"`
	Path       string `json:"path,omitempty"`
	Title      string `json:"title,omitempty"`
	Year       int    `json:"year,omitempty"`
	Type       string `json:"type,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Season     *int   `json:"season,omitempty"`
	Episode    *int   `json:"episode,omitempty"`
}

type ImportResponse struct {
	FileID       int64  `json:"file_id"`
	ContentID    int64  `json:"content_id"`
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	SizeBytes    int64  `json:"size_bytes"`
	PlexNotified bool   `json:"plex_notified"`
}

type PlexLibrary struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	ItemCount  int    `json:"item_count"`
	Location   string `json:"location"`
	ScannedAt  int64  `json:"scanned_at"`
	Refreshing bool   `json:"refreshing"`
}

type PlexStatusResponse struct {
	Connected  bool          `json:"connected"`
	ServerName string        `json:"server_name,omitempty"`
	Version    string        `json:"version,omitempty"`
	Libraries  []PlexLibrary `json:"libraries,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// API methods

func (c *Client) Status() (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.get("/api/v1/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Dashboard() (*DashboardResponse, error) {
	var resp DashboardResponse
	if err := c.get("/api/v1/dashboard", &resp); err != nil {
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
	params := url.Values{}
	params.Set("query", query)
	if contentType != "" {
		params.Set("type", contentType)
	}
	if profile != "" {
		params.Set("profile", profile)
	}

	var resp SearchResponse
	if err := c.get("/api/v1/search?"+params.Encode(), &resp); err != nil {
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

func (c *Client) PlexStatus() (*PlexStatusResponse, error) {
	var resp PlexStatusResponse
	if err := c.get("/api/v1/plex/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Import(req *ImportRequest) (*ImportResponse, error) {
	var resp ImportResponse
	if err := c.post("/api/v1/import", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type PlexScanRequest struct {
	Libraries []string `json:"libraries"`
}

type PlexScanResponse struct {
	Scanned []string `json:"scanned"`
}

func (c *Client) PlexScan(libraries []string) (*PlexScanResponse, error) {
	req := PlexScanRequest{Libraries: libraries}
	var resp PlexScanResponse
	if err := c.post("/api/v1/plex/scan", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type PlexItemResponse struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	AddedAt   int64  `json:"added_at"`
	FilePath  string `json:"file_path,omitempty"`
	Tracked   bool   `json:"tracked"`
	ContentID *int64 `json:"content_id,omitempty"`
}

type PlexListResponse struct {
	Library string             `json:"library"`
	Items   []PlexItemResponse `json:"items"`
	Total   int                `json:"total"`
}

type PlexSearchResponse struct {
	Query string             `json:"query"`
	Items []PlexItemResponse `json:"items"`
	Total int                `json:"total"`
}

func (c *Client) PlexListLibrary(library string) (*PlexListResponse, error) {
	var resp PlexListResponse
	if err := c.get("/api/v1/plex/libraries/"+url.PathEscape(library)+"/items", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) PlexSearch(query string) (*PlexSearchResponse, error) {
	var resp PlexSearchResponse
	if err := c.get("/api/v1/plex/search?query="+url.QueryEscape(query), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type VerifyProblem struct {
	DownloadID int64    `json:"download_id"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Since      string   `json:"since"`
	Issue      string   `json:"issue"`
	Checks     []string `json:"checks"`
	Likely     string   `json:"likely_cause"`
	Fixes      []string `json:"suggested_fixes"`
}

type VerifyResponse struct {
	Connections struct {
		Plex    bool   `json:"plex"`
		PlexErr string `json:"plex_error,omitempty"`
		SABnzbd bool   `json:"sabnzbd"`
		SABErr  string `json:"sabnzbd_error,omitempty"`
	} `json:"connections"`
	Checked  int             `json:"checked"`
	Passed   int             `json:"passed"`
	Problems []VerifyProblem `json:"problems"`
}

func (c *Client) Verify(id *int64) (*VerifyResponse, error) {
	path := "/api/v1/verify"
	if id != nil {
		path += fmt.Sprintf("?id=%d", *id)
	}
	var resp VerifyResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CancelDownload cancels a download and optionally deletes its files.
func (c *Client) CancelDownload(id int64, deleteFiles bool) error {
	path := fmt.Sprintf("/api/v1/downloads/%d", id)
	if deleteFiles {
		path += "?delete_files=true"
	}
	return c.delete(path)
}

// Event types

type EventResponse struct {
	ID         int64  `json:"id"`
	EventType  string `json:"event_type"`
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	OccurredAt string `json:"occurred_at"`
}

type ListEventsResponse struct {
	Items []EventResponse `json:"items"`
	Total int             `json:"total"`
}

func (c *Client) Events(limit int) (*ListEventsResponse, error) {
	path := fmt.Sprintf("/api/v1/events?limit=%d", limit)
	var resp ListEventsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Download(id int64) (*DownloadResponse, error) {
	path := fmt.Sprintf("/api/v1/downloads/%d", id)
	var resp DownloadResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DownloadEvents(id int64) (*ListEventsResponse, error) {
	path := fmt.Sprintf("/api/v1/downloads/%d/events", id)
	var resp ListEventsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RetryResponse is the response from retrying a failed download.
type RetryResponse struct {
	NewDownloadID int64  `json:"new_download_id,omitempty"`
	ReleaseName   string `json:"release_name"`
	Message       string `json:"message"`
}

// RetryDownload re-searches indexers for the content and grabs the best matching release.
func (c *Client) RetryDownload(id int64) (*RetryResponse, error) {
	path := fmt.Sprintf("/api/v1/downloads/%d/retry", id)
	var resp RetryResponse
	if err := c.post(path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Indexer types

type IndexerResponse struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	ResponseMs int64  `json:"response_ms,omitempty"`
}

type ListIndexersResponse struct {
	Indexers []IndexerResponse `json:"indexers"`
}

func (c *Client) Indexers(test bool) (*ListIndexersResponse, error) {
	path := "/api/v1/indexers"
	if test {
		path += "?test=true"
	}
	var resp ListIndexersResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Profile types

type ProfileResponse struct {
	Name   string   `json:"name"`
	Accept []string `json:"accept"`
}

type ListProfilesResponse struct {
	Profiles []ProfileResponse `json:"profiles"`
}

func (c *Client) Profiles() (*ListProfilesResponse, error) {
	var resp ListProfilesResponse
	if err := c.get("/api/v1/profiles", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Files(contentID *int64) (*ListFilesResponse, error) {
	path := "/api/v1/files"
	if contentID != nil {
		path += fmt.Sprintf("?content_id=%d", *contentID)
	}
	var resp ListFilesResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
