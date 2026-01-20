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

// Manager orchestrates download operations.
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

// Grab sends a release to the download client and records it in the database.
func (m *Manager) Grab(ctx context.Context, contentID int64, episodeID *int64,
	downloadURL, releaseName, indexer string) (*Download, error) {

	// Send to download client first
	clientID, err := m.client.Add(ctx, downloadURL, "")
	if err != nil {
		m.log.Error("grab failed", "content_id", contentID, "error", err)
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

	m.log.Info("grab sent", "content_id", contentID, "release", releaseName, "client_id", clientID)
	return d, nil
}

// Refresh polls the download client for status updates and syncs to the database.
func (m *Manager) Refresh(ctx context.Context) error {
	downloads, err := m.store.List(Filter{Active: true})
	if err != nil {
		return fmt.Errorf("list active: %w", err)
	}

	m.log.Debug("refresh started", "active_downloads", len(downloads))

	var lastErr error
	for _, d := range downloads {
		status, err := m.client.Status(ctx, d.ClientID)
		if err != nil {
			m.log.Error("refresh error", "download_id", d.ID, "error", err)
			lastErr = err
			continue
		}

		// Only update if client reports a different status AND transition is valid
		// This prevents overwriting terminal states (imported, cleaned) with client status
		if status.Status != d.Status && d.Status.CanTransitionTo(status.Status) {
			m.log.Info("download status changed", "download_id", d.ID, "status", status.Status, "prev", d.Status)
			if err := m.store.Transition(d, status.Status); err != nil {
				m.log.Error("refresh update failed", "download_id", d.ID, "error", err)
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
