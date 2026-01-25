package library

import (
	"fmt"
	"strings"
	"time"
)

func addFile(q querier, f *File) error {
	now := time.Now()
	result, err := q.Exec(`
		INSERT INTO files (content_id, episode_id, path, size_bytes, quality, source, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ContentID, f.EpisodeID, f.Path, f.SizeBytes, f.Quality, f.Source, now,
	)
	if err != nil {
		return fmt.Errorf("insert file: %w", mapSQLiteError(err))
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	f.ID = id
	f.AddedAt = now
	return nil
}

// AddFile inserts a new file into the database.
// Sets ID and AddedAt on the struct.
func (s *Store) AddFile(f *File) error { return addFile(s.db, f) }

// AddFile inserts a new file within a transaction.
func (t *Tx) AddFile(f *File) error { return addFile(t.tx, f) }

func getFile(q querier, id int64) (*File, error) {
	f := &File{}
	err := q.QueryRow(`
		SELECT id, content_id, episode_id, path, size_bytes, quality, source, added_at
		FROM files WHERE id = ?`, id,
	).Scan(&f.ID, &f.ContentID, &f.EpisodeID, &f.Path, &f.SizeBytes, &f.Quality, &f.Source, &f.AddedAt)
	if err != nil {
		return nil, fmt.Errorf("get file %d: %w", id, mapSQLiteError(err))
	}
	return f, nil
}

// GetFile retrieves a file by ID.
// Returns ErrNotFound if the file does not exist.
func (s *Store) GetFile(id int64) (*File, error) { return getFile(s.db, id) }

// GetFile retrieves a file by ID within a transaction.
func (t *Tx) GetFile(id int64) (*File, error) { return getFile(t.tx, id) }

func listFiles(q querier, f FileFilter) ([]*File, int, error) {
	var conditions []string
	var args []any

	// Determine if we need to join with episodes table
	needsJoin := f.Season != nil

	// Use table prefix when joining
	filePrefix := ""
	if needsJoin {
		filePrefix = "f."
	}

	if f.ContentID != nil {
		conditions = append(conditions, filePrefix+"content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, filePrefix+"episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Season != nil {
		conditions = append(conditions, "e.season = ?")
		args = append(args, *f.Season)
	}
	if f.Quality != nil {
		conditions = append(conditions, filePrefix+"quality = ?")
		args = append(args, *f.Quality)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Build FROM clause with optional join
	fromClause := "files"
	if needsJoin {
		fromClause = "files f JOIN episodes e ON f.episode_id = e.id"
	}

	var total int
	if err := q.QueryRow("SELECT COUNT(*) FROM "+fromClause+" "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count files: %w", err)
	}

	selectCols := "id, content_id, episode_id, path, size_bytes, quality, source, added_at"
	if needsJoin {
		selectCols = "f.id, f.content_id, f.episode_id, f.path, f.size_bytes, f.quality, f.source, f.added_at"
	}
	query := "SELECT " + selectCols + " FROM " + fromClause + " " + whereClause + " ORDER BY " + filePrefix + "id"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list files: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*File
	for rows.Next() {
		file := &File{}
		if err := rows.Scan(&file.ID, &file.ContentID, &file.EpisodeID, &file.Path, &file.SizeBytes, &file.Quality, &file.Source, &file.AddedAt); err != nil {
			return nil, 0, fmt.Errorf("scan file: %w", err)
		}
		results = append(results, file)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate files: %w", err)
	}

	return results, total, nil
}

// ListFiles returns files matching the filter with pagination.
// Returns (results, totalCount, error).
func (s *Store) ListFiles(f FileFilter) ([]*File, int, error) { return listFiles(s.db, f) }

// ListFiles returns files matching the filter within a transaction.
func (t *Tx) ListFiles(f FileFilter) ([]*File, int, error) { return listFiles(t.tx, f) }

func updateFile(q querier, f *File) error {
	result, err := q.Exec(`
		UPDATE files SET content_id = ?, episode_id = ?, path = ?, size_bytes = ?, quality = ?, source = ?
		WHERE id = ?`,
		f.ContentID, f.EpisodeID, f.Path, f.SizeBytes, f.Quality, f.Source, f.ID,
	)
	if err != nil {
		return fmt.Errorf("update file %d: %w", f.ID, mapSQLiteError(err))
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update file %d: %w", f.ID, ErrNotFound)
	}
	return nil
}

// UpdateFile updates an existing file.
// Returns ErrNotFound if the file does not exist.
func (s *Store) UpdateFile(f *File) error { return updateFile(s.db, f) }

// UpdateFile updates an existing file within a transaction.
func (t *Tx) UpdateFile(f *File) error { return updateFile(t.tx, f) }

func deleteFile(q querier, id int64) error {
	_, err := q.Exec("DELETE FROM files WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete file %d: %w", id, mapSQLiteError(err))
	}
	return nil
}

// DeleteFile removes a file by ID.
// This operation is idempotent - no error is returned if the file does not exist.
func (s *Store) DeleteFile(id int64) error { return deleteFile(s.db, id) }

// DeleteFile removes a file by ID within a transaction.
func (t *Tx) DeleteFile(id int64) error { return deleteFile(t.tx, id) }
