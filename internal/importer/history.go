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
	Limit     int
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
func (s *HistoryStore) List(f HistoryFilter) ([]*HistoryEntry, error) {
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

	query := `SELECT id, content_id, episode_id, event, data, created_at
		FROM history ` + whereClause + ` ORDER BY created_at DESC`

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*HistoryEntry
	for rows.Next() {
		h := &HistoryEntry{}
		if err := rows.Scan(&h.ID, &h.ContentID, &h.EpisodeID, &h.Event, &h.Data, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}

	return results, nil
}
