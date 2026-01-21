// Package download manages download clients and tracks download progress.
package download

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Client is a download client (SABnzbd, qBittorrent, etc.)
type Client string

const (
	ClientSABnzbd     Client = "sabnzbd"
	ClientQBittorrent Client = "qbittorrent"
	ClientManual      Client = "manual"
)

// Status tracks download state.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusCompleted   Status = "completed"
	StatusImporting   Status = "importing"
	StatusFailed      Status = "failed"
	StatusImported    Status = "imported"
	StatusCleaned     Status = "cleaned"
	StatusSkipped     Status = "skipped" // Duplicate detected, import skipped
)

// Download represents an active or recent download.
type Download struct {
	ID               int64
	ContentID        int64
	EpisodeID        *int64
	Client           Client
	ClientID         string // ID in the download client
	Status           Status
	ReleaseName      string
	Indexer          string
	AddedAt          time.Time
	CompletedAt      *time.Time
	LastTransitionAt time.Time
}

// Filter specifies criteria for listing downloads.
type Filter struct {
	ContentID *int64
	EpisodeID *int64
	Status    *Status
	Client    *Client
	Active    bool // If true, exclude terminal states (cleaned, failed)
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

// Store persists download records.
type Store struct {
	db       *sql.DB
	handlers []TransitionHandler
}

// OnTransition registers a handler to be called on state transitions.
func (s *Store) OnTransition(h TransitionHandler) {
	s.handlers = append(s.handlers, h)
}

// NewStore creates a download store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add records a new download.
// This method is idempotent: if a download with the same content_id and release_name
// already exists, it returns the existing record's ID instead of creating a duplicate.
func (s *Store) Add(d *Download) error {
	// Check for existing download with same content_id and release_name
	var existingID int64
	var existingAddedAt time.Time
	err := s.db.QueryRow(`
		SELECT id, added_at FROM downloads
		WHERE content_id = ? AND release_name = ?`,
		d.ContentID, d.ReleaseName,
	).Scan(&existingID, &existingAddedAt)

	if err == nil {
		// Found existing record, return it
		d.ID = existingID
		d.AddedAt = existingAddedAt
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing download: %w", err)
	}

	// No existing record, insert new one
	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO downloads (content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ContentID, d.EpisodeID, d.Client, d.ClientID, d.Status, d.ReleaseName, d.Indexer, now, d.CompletedAt, now,
	)
	if err != nil {
		return fmt.Errorf("insert download: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	d.ID = id
	d.AddedAt = now
	d.LastTransitionAt = now
	return nil
}

// Get retrieves a download by ID.
// Returns ErrNotFound if the download does not exist.
func (s *Store) Get(id int64) (*Download, error) {
	d := &Download{}
	err := s.db.QueryRow(`
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at
		FROM downloads WHERE id = ?`, id,
	).Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("get download %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get download %d: %w", id, err)
	}
	return d, nil
}

// GetByClientID retrieves a download by its client type and client-specific ID.
// Returns ErrNotFound if no matching download exists.
func (s *Store) GetByClientID(client Client, clientID string) (*Download, error) {
	d := &Download{}
	err := s.db.QueryRow(`
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at
		FROM downloads WHERE client = ? AND client_id = ?`, client, clientID,
	).Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("get download by client %s/%s: %w", client, clientID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get download by client %s/%s: %w", client, clientID, err)
	}
	return d, nil
}

// Update updates a download's status and completed_at fields.
// Returns ErrNotFound if the download does not exist.
func (s *Store) Update(d *Download) error {
	result, err := s.db.Exec(`
		UPDATE downloads SET status = ?, completed_at = ?
		WHERE id = ?`,
		d.Status, d.CompletedAt, d.ID,
	)
	if err != nil {
		return fmt.Errorf("update download %d: %w", d.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update download %d: %w", d.ID, ErrNotFound)
	}
	return nil
}

// Transition changes a download's status with validation and event emission.
func (s *Store) Transition(d *Download, to Status) error {
	if !d.Status.CanTransitionTo(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, d.Status, to)
	}

	from := d.Status
	now := time.Now()

	// Set completed_at for terminal and completion states
	var completedAt *time.Time
	if to == StatusCompleted || to == StatusFailed {
		completedAt = &now
	}

	result, err := s.db.Exec(`
		UPDATE downloads SET status = ?, last_transition_at = ?, completed_at = COALESCE(?, completed_at)
		WHERE id = ?`,
		to, now, completedAt, d.ID,
	)
	if err != nil {
		return fmt.Errorf("update download %d: %w", d.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("transition download %d: %w", d.ID, ErrNotFound)
	}

	d.Status = to
	d.LastTransitionAt = now
	if completedAt != nil {
		d.CompletedAt = completedAt
	}

	// Emit event
	event := TransitionEvent{
		DownloadID: d.ID,
		From:       from,
		To:         to,
		At:         now,
	}
	for _, h := range s.handlers {
		h(event)
	}

	return nil
}

// List returns downloads matching the specified filter.
// If Active is true, downloads in terminal states (cleaned, failed) are excluded.
func (s *Store) List(f Filter) ([]*Download, error) {
	// Pre-allocate with capacity for potential filter conditions
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 6)

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}
	if f.Client != nil {
		conditions = append(conditions, "client = ?")
		args = append(args, *f.Client)
	}
	if f.Active {
		conditions = append(conditions, "status NOT IN (?, ?)")
		args = append(args, StatusCleaned, StatusFailed)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// G202: False positive - whereClause contains only "col = ?" conditions,
	// actual values are passed via args parameter (parameterized query).
	query := "SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at FROM downloads " + //nolint:gosec
		whereClause + " ORDER BY id"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list downloads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*Download
	for rows.Next() {
		d := &Download{}
		if err := rows.Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt); err != nil {
			return nil, fmt.Errorf("scan download: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate downloads: %w", err)
	}

	return results, nil
}

// Delete removes a download by ID.
// This operation is idempotent - no error is returned if the download does not exist.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM downloads WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete download %d: %w", id, err)
	}
	return nil
}

// ListStuck returns downloads that haven't transitioned within their expected threshold.
func (s *Store) ListStuck(thresholds map[Status]time.Duration) ([]*Download, error) {
	// Pre-allocate with capacity based on threshold count
	conditions := make([]string, 0, len(thresholds))
	args := make([]any, 0, len(thresholds)*2)

	now := time.Now()
	for status, threshold := range thresholds {
		cutoff := now.Add(-threshold)
		conditions = append(conditions, "(status = ? AND last_transition_at < ?)")
		args = append(args, status, cutoff)
	}

	if len(conditions) == 0 {
		return nil, nil
	}

	// False positive - conditions contain only "(status = ? AND last_transition_at < ?)" patterns,
	// actual values are passed via args parameter (parameterized query).
	whereClause := strings.Join(conditions, " OR ")
	//nolint:gosec // G201: whereClause is built from hardcoded conditions, not user input
	query := fmt.Sprintf(`SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at
		FROM downloads WHERE %s ORDER BY last_transition_at`, whereClause)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stuck downloads: %w", err)
	}
	defer rows.Close()

	var results []*Download
	for rows.Next() {
		d := &Download{}
		if err := rows.Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt); err != nil {
			return nil, fmt.Errorf("scan download: %w", err)
		}
		results = append(results, d)
	}

	return results, rows.Err()
}
