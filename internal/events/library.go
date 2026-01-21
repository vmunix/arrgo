// internal/events/library.go
package events

// ContentAdded is emitted when new content is added to the library.
type ContentAdded struct {
	BaseEvent
	ContentID      int64  `json:"content_id"`
	ContentType    string `json:"content_type"` // "movie" or "series"
	Title          string `json:"title"`
	Year           int    `json:"year"`
	QualityProfile string `json:"quality_profile"`
}

// ContentStatusChanged is emitted when content status changes.
type ContentStatusChanged struct {
	BaseEvent
	ContentID int64  `json:"content_id"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// PlexItemDetected is emitted when Plex finds our imported file.
type PlexItemDetected struct {
	BaseEvent
	ContentID int64  `json:"content_id"`
	PlexKey   string `json:"plex_key"`
}
