// Package download manages download clients and tracks download progress.
package download

import (
	"context"
	"database/sql"
	"time"
)

// Client is a download client (SABnzbd, qBittorrent, etc.)
type Client string

const (
	ClientSABnzbd     Client = "sabnzbd"
	ClientQBittorrent Client = "qbittorrent"
)

// Status tracks download state.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusImported    Status = "imported"
)

// Download represents an active or recent download.
type Download struct {
	ID          int64
	ContentID   int64
	EpisodeID   *int64
	Client      Client
	ClientID    string // ID in the download client
	Status      Status
	ReleaseName string
	Indexer     string
	AddedAt     time.Time
	CompletedAt *time.Time
}

// DownloadFilter specifies criteria for listing downloads.
type DownloadFilter struct {
	ContentID *int64
	EpisodeID *int64
	Status    *Status
	Client    *Client
	Active    bool // If true, exclude "imported" status
}

// ClientStatus is the status from a download client.
type ClientStatus struct {
	ID       string
	Name     string
	Status   Status
	Progress float64 // 0-100
	Size     int64
	Speed    int64 // bytes/sec
	ETA      time.Duration
	Path     string // Completed download path
}

// Downloader sends items to download clients.
type Downloader interface {
	// Add sends a release to the download client.
	Add(ctx context.Context, url, category string) (clientID string, err error)
	// Status returns the status of a download.
	Status(ctx context.Context, clientID string) (*ClientStatus, error)
	// List returns all downloads.
	List(ctx context.Context) ([]*ClientStatus, error)
	// Remove cancels/removes a download.
	Remove(ctx context.Context, clientID string, deleteFiles bool) error
}

// SABnzbdClient interacts with SABnzbd.
type SABnzbdClient struct {
	baseURL  string
	apiKey   string
	category string
}

// NewSABnzbdClient creates a new SABnzbd client.
func NewSABnzbdClient(baseURL, apiKey, category string) *SABnzbdClient {
	return &SABnzbdClient{
		baseURL:  baseURL,
		apiKey:   apiKey,
		category: category,
	}
}

// Add sends an NZB URL to SABnzbd.
func (c *SABnzbdClient) Add(ctx context.Context, url, category string) (string, error) {
	// TODO: implement SABnzbd API call
	return "", nil
}

// Status gets the status of a download.
func (c *SABnzbdClient) Status(ctx context.Context, clientID string) (*ClientStatus, error) {
	// TODO: implement
	return nil, nil
}

// List returns all SABnzbd downloads.
func (c *SABnzbdClient) List(ctx context.Context) ([]*ClientStatus, error) {
	// TODO: implement
	return nil, nil
}

// Remove cancels a download.
func (c *SABnzbdClient) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	// TODO: implement
	return nil
}

// Store persists download records.
type Store struct {
	db *sql.DB
}

// NewStore creates a download store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add records a new download.
func (s *Store) Add(d *Download) error {
	// TODO: implement
	return nil
}

// Get retrieves a download by ID.
func (s *Store) Get(id int64) (*Download, error) {
	// TODO: implement
	return nil, nil
}

// UpdateStatus updates download status.
func (s *Store) UpdateStatus(id int64, status Status, completedAt *time.Time) error {
	// TODO: implement
	return nil
}

// ListActive returns non-imported downloads.
func (s *Store) ListActive() ([]*Download, error) {
	// TODO: implement
	return nil, nil
}
