// internal/events/import.go
package events

// ImportStarted is emitted when import begins.
type ImportStarted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"`
}

// ImportCompleted is emitted when import succeeds.
type ImportCompleted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	ContentID  int64  `json:"content_id"`
	EpisodeID  *int64 `json:"episode_id,omitempty"`
	FilePath   string `json:"file_path"` // Final destination
	FileSize   int64  `json:"file_size"`
}

// ImportFailed is emitted when import fails.
type ImportFailed struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	Reason     string `json:"reason"`
}

// CleanupStarted is emitted when source cleanup begins.
type CleanupStarted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"`
}

// CleanupCompleted is emitted when source files are removed.
type CleanupCompleted struct {
	BaseEvent
	DownloadID int64 `json:"download_id"`
}
