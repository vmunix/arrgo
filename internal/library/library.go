// Package library manages content tracking (movies, series, episodes, files).
package library

import (
	"database/sql"
	"time"
)

// ContentType distinguishes movies from series.
type ContentType string

const (
	ContentTypeMovie  ContentType = "movie"
	ContentTypeSeries ContentType = "series"
)

// ContentStatus tracks the state of content.
type ContentStatus string

const (
	StatusWanted      ContentStatus = "wanted"
	StatusAvailable   ContentStatus = "available"
	StatusUnmonitored ContentStatus = "unmonitored"
)

// Content represents a movie or series.
type Content struct {
	ID             int64
	Type           ContentType
	TMDBID         *int64 // nil for series
	TVDBID         *int64 // nil for movies
	Title          string
	Year           int
	Status         ContentStatus
	QualityProfile string
	RootPath       string
	AddedAt        time.Time
	UpdatedAt      time.Time
}

// Episode represents a single episode of a series.
type Episode struct {
	ID        int64
	ContentID int64
	Season    int
	Episode   int
	Title     string
	Status    ContentStatus
	AirDate   *time.Time
}

// File represents a media file on disk.
type File struct {
	ID        int64
	ContentID int64
	EpisodeID *int64 // nil for movies
	Path      string
	SizeBytes int64
	Quality   string
	Source    string
	AddedAt   time.Time
}

// Store provides access to content data.
type Store struct {
	db *sql.DB
}

// NewStore creates a new library store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// AddContent adds a movie or series to the library.
func (s *Store) AddContent(c *Content) error {
	// TODO: implement
	return nil
}

// GetContent retrieves content by ID.
func (s *Store) GetContent(id int64) (*Content, error) {
	// TODO: implement
	return nil, nil
}

// ListContent returns all content, optionally filtered.
func (s *Store) ListContent(contentType *ContentType, status *ContentStatus) ([]*Content, error) {
	// TODO: implement
	return nil, nil
}

// UpdateContent updates content fields.
func (s *Store) UpdateContent(c *Content) error {
	// TODO: implement
	return nil
}

// DeleteContent removes content from the library.
func (s *Store) DeleteContent(id int64) error {
	// TODO: implement
	return nil
}

// GetEpisodes returns all episodes for a series.
func (s *Store) GetEpisodes(contentID int64) ([]*Episode, error) {
	// TODO: implement
	return nil, nil
}

// AddFile records a media file.
func (s *Store) AddFile(f *File) error {
	// TODO: implement
	return nil
}

// GetFiles returns files for content.
func (s *Store) GetFiles(contentID int64) ([]*File, error) {
	// TODO: implement
	return nil, nil
}
