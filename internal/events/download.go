// internal/events/download.go
package events

// Entity types
const (
	EntityDownload = "download"
	EntityContent  = "content"
	EntityEpisode  = "episode"
)

// Event type constants
const (
	EventGrabRequested        = "grab.requested"
	EventGrabSkipped          = "grab.skipped"
	EventDownloadCreated      = "download.created"
	EventDownloadProgressed   = "download.progressed"
	EventDownloadCompleted    = "download.completed"
	EventDownloadFailed       = "download.failed"
	EventImportStarted        = "import.started"
	EventImportCompleted      = "import.completed"
	EventImportFailed         = "import.failed"
	EventImportSkipped        = "import.skipped"
	EventCleanupStarted       = "cleanup.started"
	EventCleanupCompleted     = "cleanup.completed"
	EventContentAdded         = "content.added"
	EventContentStatusChanged = "content.status.changed"
	EventPlexItemDetected     = "plex.item.detected"
)

// GrabRequested is emitted when a user/API requests a download.
type GrabRequested struct {
	BaseEvent
	ContentID        int64   `json:"content_id"`
	EpisodeID        *int64  `json:"episode_id,omitempty"`         // Deprecated: use EpisodeIDs
	EpisodeIDs       []int64 `json:"episode_ids,omitempty"`        // Episode IDs for multi-episode grabs
	Season           *int    `json:"season,omitempty"`             // Season number (for season packs)
	IsCompleteSeason bool    `json:"is_complete_season,omitempty"` // True if grabbing complete season
	DownloadURL      string  `json:"download_url"`
	ReleaseName      string  `json:"release_name"`
	Indexer          string  `json:"indexer"`
}

// DownloadCreated is emitted when a download record is created.
type DownloadCreated struct {
	BaseEvent
	DownloadID       int64   `json:"download_id"`
	ContentID        int64   `json:"content_id"`
	EpisodeID        *int64  `json:"episode_id,omitempty"`         // Deprecated: use EpisodeIDs
	EpisodeIDs       []int64 `json:"episode_ids,omitempty"`        // Episode IDs for multi-episode grabs
	Season           *int    `json:"season,omitempty"`             // Season number (for season packs)
	IsCompleteSeason bool    `json:"is_complete_season,omitempty"` // True if grabbing complete season
	ClientID         string  `json:"client_id"`                    // SABnzbd nzo_id
	ReleaseName      string  `json:"release_name"`
}

// DownloadProgressed is emitted periodically with download progress.
type DownloadProgressed struct {
	BaseEvent
	DownloadID int64   `json:"download_id"`
	Progress   float64 `json:"progress"`  // 0.0 - 100.0
	Speed      int64   `json:"speed_bps"` // bytes per second
	ETA        int     `json:"eta_seconds"`
	Size       int64   `json:"size_bytes"`
}

// DownloadCompleted is emitted when a download finishes.
type DownloadCompleted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"` // Where client put files
}

// DownloadFailed is emitted when a download fails.
type DownloadFailed struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	Reason     string `json:"reason"`
	Retryable  bool   `json:"retryable"`
}

// GrabSkipped is emitted when a grab is skipped due to existing quality.
type GrabSkipped struct {
	BaseEvent
	ContentID       int64  `json:"content_id"`
	ReleaseName     string `json:"release_name"`
	ReleaseQuality  string `json:"release_quality"`  // e.g., "1080p"
	ExistingQuality string `json:"existing_quality"` // e.g., "2160p"
	Reason          string `json:"reason"`           // "existing_quality_equal_or_better"
}

// ImportSkipped is emitted when an import is skipped due to existing quality.
type ImportSkipped struct {
	BaseEvent
	DownloadID      int64  `json:"download_id"`
	ContentID       int64  `json:"content_id"`
	SourcePath      string `json:"source_path"`
	ReleaseQuality  string `json:"release_quality"`
	ExistingQuality string `json:"existing_quality"`
	Reason          string `json:"reason"`
}
