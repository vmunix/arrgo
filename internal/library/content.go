package library

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// mapSQLiteError converts SQLite errors to custom error types.
func mapSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	// modernc.org/sqlite wraps errors; check error message for constraint violations
	errStr := err.Error()
	if strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "PRIMARY KEY constraint failed") {
		return ErrDuplicate
	}
	if strings.Contains(errStr, "FOREIGN KEY constraint failed") ||
		strings.Contains(errStr, "CHECK constraint failed") {
		return ErrConstraint
	}
	return err
}

func addContent(q querier, c *Content) error {
	now := time.Now()
	result, err := q.Exec(`
		INSERT INTO content (type, tmdb_id, tvdb_id, title, year, status, quality_profile, root_path, added_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Type, c.TMDBID, c.TVDBID, c.Title, c.Year, c.Status, c.QualityProfile, c.RootPath, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert content: %w", mapSQLiteError(err))
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	c.ID = id
	c.AddedAt = now
	c.UpdatedAt = now
	return nil
}

// AddContent inserts a new content item into the database.
// Sets ID, AddedAt, and UpdatedAt on the struct.
func (s *Store) AddContent(c *Content) error { return addContent(s.db, c) }

// AddContent inserts a new content item within a transaction.
func (t *Tx) AddContent(c *Content) error { return addContent(t.tx, c) }

func getContent(q querier, id int64) (*Content, error) {
	c := &Content{}
	err := q.QueryRow(`
		SELECT id, type, tmdb_id, tvdb_id, title, year, status, quality_profile, root_path, added_at, updated_at
		FROM content WHERE id = ?`, id,
	).Scan(&c.ID, &c.Type, &c.TMDBID, &c.TVDBID, &c.Title, &c.Year, &c.Status, &c.QualityProfile, &c.RootPath, &c.AddedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get content %d: %w", id, mapSQLiteError(err))
	}
	return c, nil
}

// GetContent retrieves a content item by ID.
// Returns ErrNotFound if the content does not exist.
func (s *Store) GetContent(id int64) (*Content, error) { return getContent(s.db, id) }

// GetContent retrieves a content item by ID within a transaction.
func (t *Tx) GetContent(id int64) (*Content, error) { return getContent(t.tx, id) }

// GetByTitleYear finds content by title and year.
// Returns nil, nil if not found.
func (s *Store) GetByTitleYear(title string, year int) (*Content, error) {
	contents, _, err := s.ListContent(ContentFilter{Title: &title, Year: &year, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 {
		return nil, nil
	}
	return contents[0], nil
}

func listContent(q querier, f ContentFilter) ([]*Content, int, error) {
	var conditions []string
	var args []any

	if f.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *f.Type)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}
	if f.QualityProfile != nil {
		conditions = append(conditions, "quality_profile = ?")
		args = append(args, *f.QualityProfile)
	}
	if f.TMDBID != nil {
		conditions = append(conditions, "tmdb_id = ?")
		args = append(args, *f.TMDBID)
	}
	if f.TVDBID != nil {
		conditions = append(conditions, "tvdb_id = ?")
		args = append(args, *f.TVDBID)
	}
	if f.Title != nil {
		conditions = append(conditions, "title = ?")
		args = append(args, *f.Title)
	}
	if f.Year != nil {
		conditions = append(conditions, "year = ?")
		args = append(args, *f.Year)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := q.QueryRow("SELECT COUNT(*) FROM content "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count content: %w", err)
	}

	query := "SELECT id, type, tmdb_id, tvdb_id, title, year, status, quality_profile, root_path, added_at, updated_at FROM content " + whereClause + " ORDER BY id"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list content: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*Content
	for rows.Next() {
		c := &Content{}
		if err := rows.Scan(&c.ID, &c.Type, &c.TMDBID, &c.TVDBID, &c.Title, &c.Year, &c.Status, &c.QualityProfile, &c.RootPath, &c.AddedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan content: %w", err)
		}
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate content: %w", err)
	}

	return results, total, nil
}

// ListContent returns content items matching the filter with pagination.
// Returns (results, totalCount, error).
func (s *Store) ListContent(f ContentFilter) ([]*Content, int, error) { return listContent(s.db, f) }

// ListContent returns content items matching the filter within a transaction.
func (t *Tx) ListContent(f ContentFilter) ([]*Content, int, error) { return listContent(t.tx, f) }

func updateContent(q querier, c *Content) error {
	now := time.Now()
	result, err := q.Exec(`
		UPDATE content SET type = ?, tmdb_id = ?, tvdb_id = ?, title = ?, year = ?, status = ?, quality_profile = ?, root_path = ?, updated_at = ?
		WHERE id = ?`,
		c.Type, c.TMDBID, c.TVDBID, c.Title, c.Year, c.Status, c.QualityProfile, c.RootPath, now, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update content %d: %w", c.ID, mapSQLiteError(err))
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update content %d: %w", c.ID, ErrNotFound)
	}
	c.UpdatedAt = now
	return nil
}

// UpdateContent updates an existing content item.
// Sets UpdatedAt on the struct.
// Returns ErrNotFound if the content does not exist.
func (s *Store) UpdateContent(c *Content) error { return updateContent(s.db, c) }

// UpdateContent updates an existing content item within a transaction.
func (t *Tx) UpdateContent(c *Content) error { return updateContent(t.tx, c) }

func deleteContent(q querier, id int64) error {
	_, err := q.Exec("DELETE FROM content WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete content %d: %w", id, mapSQLiteError(err))
	}
	return nil
}

// DeleteContent removes a content item by ID.
// This operation is idempotent - no error is returned if the content does not exist.
func (s *Store) DeleteContent(id int64) error { return deleteContent(s.db, id) }

// DeleteContent removes a content item by ID within a transaction.
func (t *Tx) DeleteContent(id int64) error { return deleteContent(t.tx, id) }
