// Package library manages content tracking (movies, series, episodes, files).
package library

// ContentFilter specifies criteria for listing content.
type ContentFilter struct {
	Type           *ContentType
	Status         *ContentStatus
	QualityProfile *string
	TMDBID         *int64
	TVDBID         *int64
	Title          *string
	Year           *int
	Limit          int // 0 = no limit
	Offset         int
}

// EpisodeFilter specifies criteria for listing episodes.
type EpisodeFilter struct {
	ContentID *int64
	Season    *int
	Status    *ContentStatus
	Limit     int
	Offset    int
}

// FileFilter specifies criteria for listing files.
type FileFilter struct {
	ContentID *int64
	EpisodeID *int64
	Quality   *string
	Limit     int
	Offset    int
}
