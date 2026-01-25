// internal/importer/history.go
package importer

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Event types for history records.
const (
	EventGrabbed  = "grabbed"
	EventImported = "imported"
	EventDeleted  = "deleted"
	EventUpgraded = "upgraded"
	EventFailed   = "failed"
)

// HistoryEntry represents a history record.
type HistoryEntry struct {
	ID        int64
	ContentID int64
	EpisodeID *int64
	Event     string
	Data      string // JSON blob
	CreatedAt time.Time
}

// HistoryFilter specifies criteria for listing history.
type HistoryFilter struct {
	ContentID *int64
	EpisodeID *int64
	Event     *string
	Limit     int // Maximum number of results (0 = unlimited)
	Offset    int // Number of results to skip
}

// HistoryStore persists history records.
type HistoryStore struct {
	db *sql.DB
}

// NewHistoryStore creates a history store.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// Add inserts a new history entry.
func (s *HistoryStore) Add(h *HistoryEntry) error {
	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO history (content_id, episode_id, event, data, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		h.ContentID, h.EpisodeID, h.Event, h.Data, now,
	)
	if err != nil {
		return fmt.Errorf("insert history: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	h.ID = id
	h.CreatedAt = now
	return nil
}

// List returns history entries matching the filter.
// Results are ordered by most recent first.
// Returns the matching entries and total count (before pagination).
func (s *HistoryStore) List(f HistoryFilter) ([]*HistoryEntry, int, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Event != nil {
		conditions = append(conditions, "event = ?")
		args = append(args, *f.Event)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count first
	// G202: False positive - whereClause contains only "col = ?" conditions,
	// actual values are passed via args parameter (parameterized query).
	countQuery := "SELECT COUNT(*) FROM history " + whereClause //nolint:gosec
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count history: %w", err)
	}

	// G202: False positive - whereClause contains only "WHERE col = ?" style conditions,
	// actual values are passed via args parameter (parameterized query).
	query := `SELECT id, content_id, episode_id, event, data, created_at ` + //nolint:gosec
		`FROM history ` + whereClause + ` ORDER BY created_at DESC`

	// Add LIMIT/OFFSET if specified
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*HistoryEntry
	for rows.Next() {
		h := &HistoryEntry{}
		if err := rows.Scan(&h.ID, &h.ContentID, &h.EpisodeID, &h.Event, &h.Data, &h.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan history: %w", err)
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate history: %w", err)
	}

	return results, total, nil
}
