package download

import (
	"context"
	"fmt"
	"log/slog"
)

// ActiveDownload combines database record with live client status.
type ActiveDownload struct {
	Download *Download
	Live     *ClientStatus
}

// Manager provides download client operations for API endpoints.
// Note: Grab and status polling are handled by the event-driven architecture
// (DownloadHandler and SABnzbd adapter). Manager is retained for:
// - Cancel: removing downloads from client and database
// - Client: accessing the download client for live status queries
// - GetActive: listing active downloads with live status
type Manager struct {
	client Downloader
	store  *Store
	log    *slog.Logger
}

// NewManager creates a new download manager.
func NewManager(client Downloader, store *Store, log *slog.Logger) *Manager {
	return &Manager{
		client: client,
		store:  store,
		log:    log,
	}
}

// Cancel removes a download from the client and database.
func (m *Manager) Cancel(ctx context.Context, downloadID int64, deleteFiles bool) error {
	d, err := m.store.Get(downloadID)
	if err != nil {
		return fmt.Errorf("get download: %w", err)
	}

	// Remove from client (best effort - may already be gone)
	_ = m.client.Remove(ctx, d.ClientID, deleteFiles)

	// Remove from database
	if err := m.store.Delete(downloadID); err != nil {
		return fmt.Errorf("delete download: %w", err)
	}

	return nil
}

// Client returns the underlying download client.
func (m *Manager) Client() Downloader {
	return m.client
}

// GetActive returns active downloads with live status from the client.
func (m *Manager) GetActive(ctx context.Context) ([]*ActiveDownload, error) {
	downloads, err := m.store.List(Filter{Active: true})
	if err != nil {
		return nil, fmt.Errorf("list active: %w", err)
	}

	results := make([]*ActiveDownload, 0, len(downloads))
	for _, d := range downloads {
		live, err := m.client.Status(ctx, d.ClientID)
		if err != nil {
			// Include download without live status
			results = append(results, &ActiveDownload{Download: d})
			continue
		}
		results = append(results, &ActiveDownload{Download: d, Live: live})
	}

	return results, nil
}
