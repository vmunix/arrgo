// Package library manages content tracking (movies, series, episodes, files).
package library

import (
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
