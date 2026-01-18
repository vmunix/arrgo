package download

import (
	"context"
	"fmt"
	"time"
)

// ActiveDownload combines database record with live client status.
type ActiveDownload struct {
	Download *Download
	Live     *ClientStatus
}

// Manager orchestrates download operations.
type Manager struct {
	client Downloader
	store  *Store
}

// NewManager creates a new download manager.
func NewManager(client Downloader, store *Store) *Manager {
	return &Manager{
		client: client,
		store:  store,
	}
}

// Grab sends a release to the download client and records it in the database.
func (m *Manager) Grab(ctx context.Context, contentID int64, episodeID *int64,
	downloadURL, releaseName, indexer string) (*Download, error) {

	// Send to download client first
	clientID, err := m.client.Add(ctx, downloadURL, "")
	if err != nil {
		return nil, fmt.Errorf("add to client: %w", err)
	}

	// Record in database (idempotent)
	d := &Download{
		ContentID:   contentID,
		EpisodeID:   episodeID,
		Client:      ClientSABnzbd, // TODO: make configurable when adding other clients
		ClientID:    clientID,
		Status:      StatusQueued,
		ReleaseName: releaseName,
		Indexer:     indexer,
	}

	if err := m.store.Add(d); err != nil {
		// Orphan in client is acceptable - Refresh will find it
		return nil, fmt.Errorf("save download: %w", err)
	}

	return d, nil
}

// Refresh polls the download client for status updates and syncs to the database.
func (m *Manager) Refresh(ctx context.Context) error {
	downloads, err := m.store.List(DownloadFilter{Active: true})
	if err != nil {
		return fmt.Errorf("list active: %w", err)
	}

	var lastErr error
	for _, d := range downloads {
		status, err := m.client.Status(ctx, d.ClientID)
		if err != nil {
			lastErr = err
			continue
		}

		if status.Status != d.Status {
			d.Status = status.Status
			if status.Status == StatusCompleted || status.Status == StatusFailed {
				now := time.Now()
				d.CompletedAt = &now
			}
			if err := m.store.Update(d); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
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
	downloads, err := m.store.List(DownloadFilter{Active: true})
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
