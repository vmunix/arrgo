package download

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SABnzbdClient interacts with SABnzbd.
type SABnzbdClient struct {
	baseURL    string
	apiKey     string
	category   string
	httpClient *http.Client
	log        *slog.Logger
}

// NewSABnzbdClient creates a new SABnzbd client.
func NewSABnzbdClient(baseURL, apiKey, category string, log *slog.Logger) *SABnzbdClient {
	if log == nil {
		log = slog.Default()
	}
	return &SABnzbdClient{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		apiKey:   apiKey,
		category: category,
		log:      log.With("component", "sabnzbd"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Add sends an NZB URL to SABnzbd.
func (c *SABnzbdClient) Add(ctx context.Context, nzbURL, category string) (string, error) {
	c.log.Debug("adding nzb", "category", category)

	params := url.Values{
		"apikey": {c.apiKey},
		"output": {"json"},
		"mode":   {"addurl"},
		"name":   {nzbURL},
		"cat":    {category},
	}

	var resp addResponse
	if err := c.doRequest(ctx, "addurl", params, &resp); err != nil {
		return "", err
	}

	if !resp.Status {
		if isAPIKeyError(resp.Error) {
			return "", ErrInvalidAPIKey
		}
		return "", fmt.Errorf("sabnzbd add failed: %s", resp.Error)
	}

	if len(resp.NzoIDs) == 0 {
		return "", fmt.Errorf("sabnzbd returned no nzo_id")
	}

	c.log.Debug("nzb added", "nzo_id", resp.NzoIDs[0])
	return resp.NzoIDs[0], nil
}

// Status gets the status of a download.
func (c *SABnzbdClient) Status(ctx context.Context, clientID string) (*ClientStatus, error) {
	// Check queue first
	queueItems, err := c.getQueue(ctx)
	if err != nil {
		return nil, err
	}

	for _, item := range queueItems {
		if item.ID == clientID {
			return item, nil
		}
	}

	// Check history
	historyItems, err := c.getHistory(ctx)
	if err != nil {
		return nil, err
	}

	for _, item := range historyItems {
		if item.ID == clientID {
			return item, nil
		}
	}

	return nil, ErrDownloadNotFound
}

// List returns all SABnzbd downloads.
func (c *SABnzbdClient) List(ctx context.Context) ([]*ClientStatus, error) {
	queueItems, err := c.getQueue(ctx)
	if err != nil {
		return nil, err
	}

	historyItems, err := c.getHistory(ctx)
	if err != nil {
		return nil, err
	}

	// Combine queue and history, queue items first
	result := make([]*ClientStatus, 0, len(queueItems)+len(historyItems))
	result = append(result, queueItems...)
	result = append(result, historyItems...)

	return result, nil
}

// Remove cancels a download from the queue.
func (c *SABnzbdClient) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	c.log.Debug("removing download", "client_id", clientID, "delete_files", deleteFiles)

	params := url.Values{
		"apikey": {c.apiKey},
		"output": {"json"},
		"mode":   {"queue"},
		"name":   {"delete"},
		"value":  {clientID},
	}

	var resp statusResponse
	if err := c.doRequest(ctx, "queue/delete", params, &resp); err != nil {
		return err
	}

	if !resp.Status {
		return fmt.Errorf("sabnzbd remove failed")
	}

	c.log.Debug("download removed", "client_id", clientID)
	return nil
}

// getQueue fetches the current download queue.
func (c *SABnzbdClient) getQueue(ctx context.Context) ([]*ClientStatus, error) {
	params := url.Values{
		"apikey": {c.apiKey},
		"output": {"json"},
		"mode":   {"queue"},
	}

	var resp queueResponse
	if err := c.doRequest(ctx, "queue", params, &resp); err != nil {
		return nil, err
	}

	// Parse queue-level speed (applies to active download)
	queueSpeed := parseSpeed(resp.Queue.Speed)

	items := make([]*ClientStatus, 0, len(resp.Queue.Slots))
	for i := range resp.Queue.Slots {
		slot := &resp.Queue.Slots[i]
		// Only the first (active) slot gets the speed; others are queued
		speed := int64(0)
		if i == 0 {
			speed = queueSpeed
		}
		items = append(items, &ClientStatus{
			ID:       slot.NzoID,
			Name:     slot.Filename,
			Status:   mapQueueStatus(slot.Status),
			Progress: parseFloat(slot.Percentage),
			Size:     int64(parseFloat(slot.MB) * 1024 * 1024),
			Speed:    speed,
			ETA:      parseTimeLeft(slot.TimeLeft),
		})
	}

	return items, nil
}

// getHistory fetches the download history.
func (c *SABnzbdClient) getHistory(ctx context.Context) ([]*ClientStatus, error) {
	params := url.Values{
		"apikey": {c.apiKey},
		"output": {"json"},
		"mode":   {"history"},
	}

	var resp historyResponse
	if err := c.doRequest(ctx, "history", params, &resp); err != nil {
		return nil, err
	}

	items := make([]*ClientStatus, 0, len(resp.History.Slots))
	for _, slot := range resp.History.Slots {
		items = append(items, &ClientStatus{
			ID:       slot.NzoID,
			Name:     slot.Name,
			Status:   mapHistoryStatus(slot.Status),
			Progress: 100,
			Size:     slot.Bytes,
			Path:     slot.Storage,
		})
	}

	return items, nil
}

// doRequest performs an HTTP request to the SABnzbd API.
func (c *SABnzbdClient) doRequest(ctx context.Context, mode string, params url.Values, result any) error {
	start := time.Now()
	reqURL := c.baseURL + "/api?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Debug("api request failed", "mode", mode, "error", err)
		return ErrClientUnavailable
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.log.Debug("api unexpected status", "mode", mode, "status", resp.StatusCode)
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	c.log.Debug("api request complete", "mode", mode, "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// Response types for SABnzbd API

type addResponse struct {
	Status bool     `json:"status"`
	NzoIDs []string `json:"nzo_ids"`
	Error  string   `json:"error"`
}

type statusResponse struct {
	Status bool `json:"status"`
}

type queueResponse struct {
	Queue struct {
		Speed string      `json:"speed"` // Queue-level speed (e.g., "5.2 M")
		Slots []queueSlot `json:"slots"`
	} `json:"queue"`
}

type queueSlot struct {
	NzoID      string `json:"nzo_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Percentage string `json:"percentage"`
	MB         string `json:"mb"`
	MBLeft     string `json:"mbleft"`
	TimeLeft   string `json:"timeleft"`
}

type historyResponse struct {
	History struct {
		Slots []historySlot `json:"slots"`
	} `json:"history"`
}

type historySlot struct {
	NzoID   string `json:"nzo_id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Bytes   int64  `json:"bytes"`
	Storage string `json:"storage"`
}

// mapQueueStatus maps SABnzbd queue status to our Status type.
func mapQueueStatus(sabStatus string) Status {
	switch sabStatus {
	case "Downloading", "Fetching", "Grabbing", "Checking":
		return StatusDownloading
	case "Queued", "Paused", "Propagating":
		return StatusQueued
	default:
		return StatusDownloading // fallback for unknown statuses
	}
}

// mapHistoryStatus maps SABnzbd history status to our Status type.
func mapHistoryStatus(sabStatus string) Status {
	switch sabStatus {
	case "Completed":
		return StatusCompleted
	case "Failed":
		return StatusFailed
	default:
		return StatusDownloading
	}
}

// isAPIKeyError checks if the error message indicates an invalid API key.
func isAPIKeyError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "api key") || strings.Contains(lower, "apikey")
}

// parseFloat parses a string to float64, returning 0 on error.
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseSpeed parses SABnzbd speed string (e.g., "5.2 M") to bytes/sec.
func parseSpeed(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Handle formats like "5.2 M", "1.5 K", "500 B"
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0
	}

	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}

	if len(parts) > 1 {
		switch strings.ToUpper(parts[1]) {
		case "M":
			val *= 1024 * 1024
		case "K":
			val *= 1024
		}
	}

	return int64(val)
}

// parseTimeLeft parses SABnzbd time string (e.g., "0:05:30") to duration.
func parseTimeLeft(s string) time.Duration {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}
