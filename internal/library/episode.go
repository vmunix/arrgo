package library

import (
	"fmt"
	"strings"
)

func addEpisode(q querier, e *Episode) error {
	result, err := q.Exec(`
		INSERT INTO episodes (content_id, season, episode, title, status, air_date)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.ContentID, e.Season, e.Episode, e.Title, e.Status, e.AirDate,
	)
	if err != nil {
		return fmt.Errorf("insert episode: %w", mapSQLiteError(err))
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	e.ID = id
	return nil
}

// AddEpisode inserts a new episode into the database.
// Sets ID on the struct.
func (s *Store) AddEpisode(e *Episode) error { return addEpisode(s.db, e) }

// AddEpisode inserts a new episode within a transaction.
func (t *Tx) AddEpisode(e *Episode) error { return addEpisode(t.tx, e) }

func getEpisode(q querier, id int64) (*Episode, error) {
	e := &Episode{}
	err := q.QueryRow(`
		SELECT id, content_id, season, episode, title, status, air_date
		FROM episodes WHERE id = ?`, id,
	).Scan(&e.ID, &e.ContentID, &e.Season, &e.Episode, &e.Title, &e.Status, &e.AirDate)
	if err != nil {
		return nil, fmt.Errorf("get episode %d: %w", id, mapSQLiteError(err))
	}
	return e, nil
}

// GetEpisode retrieves an episode by ID.
// Returns ErrNotFound if the episode does not exist.
func (s *Store) GetEpisode(id int64) (*Episode, error) { return getEpisode(s.db, id) }

// GetEpisode retrieves an episode by ID within a transaction.
func (t *Tx) GetEpisode(id int64) (*Episode, error) { return getEpisode(t.tx, id) }

func listEpisodes(q querier, f EpisodeFilter) ([]*Episode, int, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.Season != nil {
		conditions = append(conditions, "season = ?")
		args = append(args, *f.Season)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := q.QueryRow("SELECT COUNT(*) FROM episodes "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count episodes: %w", err)
	}

	query := "SELECT id, content_id, season, episode, title, status, air_date FROM episodes " + whereClause + " ORDER BY season, episode"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list episodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*Episode
	for rows.Next() {
		e := &Episode{}
		if err := rows.Scan(&e.ID, &e.ContentID, &e.Season, &e.Episode, &e.Title, &e.Status, &e.AirDate); err != nil {
			return nil, 0, fmt.Errorf("scan episode: %w", err)
		}
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate episodes: %w", err)
	}

	return results, total, nil
}

// ListEpisodes returns episodes matching the filter with pagination.
// Returns (results, totalCount, error).
func (s *Store) ListEpisodes(f EpisodeFilter) ([]*Episode, int, error) { return listEpisodes(s.db, f) }

// ListEpisodes returns episodes matching the filter within a transaction.
func (t *Tx) ListEpisodes(f EpisodeFilter) ([]*Episode, int, error) { return listEpisodes(t.tx, f) }

func updateEpisode(q querier, e *Episode) error {
	result, err := q.Exec(`
		UPDATE episodes SET content_id = ?, season = ?, episode = ?, title = ?, status = ?, air_date = ?
		WHERE id = ?`,
		e.ContentID, e.Season, e.Episode, e.Title, e.Status, e.AirDate, e.ID,
	)
	if err != nil {
		return fmt.Errorf("update episode %d: %w", e.ID, mapSQLiteError(err))
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update episode %d: %w", e.ID, ErrNotFound)
	}
	return nil
}

// UpdateEpisode updates an existing episode.
// Returns ErrNotFound if the episode does not exist.
func (s *Store) UpdateEpisode(e *Episode) error { return updateEpisode(s.db, e) }

// UpdateEpisode updates an existing episode within a transaction.
func (t *Tx) UpdateEpisode(e *Episode) error { return updateEpisode(t.tx, e) }

func deleteEpisode(q querier, id int64) error {
	_, err := q.Exec("DELETE FROM episodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete episode %d: %w", id, mapSQLiteError(err))
	}
	return nil
}

// DeleteEpisode removes an episode by ID.
// This operation is idempotent - no error is returned if the episode does not exist.
func (s *Store) DeleteEpisode(id int64) error { return deleteEpisode(s.db, id) }

// DeleteEpisode removes an episode by ID within a transaction.
func (t *Tx) DeleteEpisode(id int64) error { return deleteEpisode(t.tx, id) }

// FindOrCreateEpisode finds an existing episode or creates a new one.
// Returns (episode, created, error) where created is true if a new episode was created.
func (s *Store) FindOrCreateEpisode(contentID int64, season, episode int) (*Episode, bool, error) {
	// Try to find existing - query by contentID and season
	eps, _, err := s.ListEpisodes(EpisodeFilter{
		ContentID: &contentID,
		Season:    &season,
	})
	if err != nil {
		return nil, false, fmt.Errorf("list episodes: %w", err)
	}

	for _, ep := range eps {
		if ep.Episode == episode {
			return ep, false, nil
		}
	}

	// Not found, create new with StatusWanted
	ep := &Episode{
		ContentID: contentID,
		Season:    season,
		Episode:   episode,
		Status:    StatusWanted,
	}
	if err := s.AddEpisode(ep); err != nil {
		return nil, false, fmt.Errorf("add episode: %w", err)
	}

	return ep, true, nil
}

// FindOrCreateEpisodes finds or creates multiple episodes for a season.
// Returns the episodes in the same order as the input episode numbers.
func (s *Store) FindOrCreateEpisodes(contentID int64, season int, episodeNums []int) ([]*Episode, error) {
	result := make([]*Episode, 0, len(episodeNums))

	for _, epNum := range episodeNums {
		ep, _, err := s.FindOrCreateEpisode(contentID, season, epNum)
		if err != nil {
			return nil, err
		}
		result = append(result, ep)
	}

	return result, nil
}

// SeriesStats contains statistics about a series.
type SeriesStats struct {
	TotalEpisodes     int
	AvailableEpisodes int
	SeasonCount       int
}

// GetSeriesStats returns episode statistics for a series.
func (s *Store) GetSeriesStats(contentID int64) (*SeriesStats, error) {
	stats := &SeriesStats{}

	// Get total and available episode counts
	// Use COALESCE to handle NULL when no episodes exist
	err := s.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN status = 'available' THEN 1 ELSE 0 END), 0) as available,
			COUNT(DISTINCT season) as seasons
		FROM episodes
		WHERE content_id = ?`, contentID,
	).Scan(&stats.TotalEpisodes, &stats.AvailableEpisodes, &stats.SeasonCount)
	if err != nil {
		return nil, fmt.Errorf("get series stats: %w", err)
	}

	return stats, nil
}

// BulkAddEpisodes inserts multiple episodes efficiently.
// Skips episodes that already exist (by content_id, season, episode).
// Returns the count of newly inserted episodes.
func (s *Store) BulkAddEpisodes(episodes []*Episode) (int, error) {
	if len(episodes) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO episodes (content_id, season, episode, title, status, air_date)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	inserted := 0
	for _, e := range episodes {
		result, err := stmt.Exec(e.ContentID, e.Season, e.Episode, e.Title, e.Status, e.AirDate)
		if err != nil {
			return inserted, fmt.Errorf("insert episode S%02dE%02d: %w", e.Season, e.Episode, err)
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return inserted, nil
}

// GetSeriesStatsBatch returns episode statistics for multiple series.
// Returns a map from content ID to stats.
func (s *Store) GetSeriesStatsBatch(contentIDs []int64) (map[int64]*SeriesStats, error) {
	if len(contentIDs) == 0 {
		return map[int64]*SeriesStats{}, nil
	}

	// Build placeholders
	placeholders := make([]string, len(contentIDs))
	args := make([]any, len(contentIDs))
	for i, id := range contentIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT
			content_id,
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN status = 'available' THEN 1 ELSE 0 END), 0) as available,
			COUNT(DISTINCT season) as seasons
		FROM episodes
		WHERE content_id IN (%s)
		GROUP BY content_id`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("get series stats batch: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]*SeriesStats)
	for rows.Next() {
		var contentID int64
		stats := &SeriesStats{}
		if err := rows.Scan(&contentID, &stats.TotalEpisodes, &stats.AvailableEpisodes, &stats.SeasonCount); err != nil {
			return nil, fmt.Errorf("scan series stats: %w", err)
		}
		result[contentID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate series stats: %w", err)
	}

	return result, nil
}
