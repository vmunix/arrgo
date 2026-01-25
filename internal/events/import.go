// internal/events/import.go
package events

// ImportStarted is emitted when import begins.
type ImportStarted struct {
	BaseEvent
	DownloadID int64  `json:"download_id"`
	SourcePath string `json:"source_path"`
}

// EpisodeImportResult tracks the outcome of importing a single episode.
type EpisodeImportResult struct {
	EpisodeID int64  `json:"episode_id"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Success   bool   `json:"success"`
	FilePath  string `json:"file_path,omitempty"` // Empty if failed
	Error     string `json:"error,omitempty"`     // Empty if success
}

// ImportCompleted is emitted when import succeeds.
type ImportCompleted struct {
	BaseEvent
	DownloadID     int64                 `json:"download_id"`
	ContentID      int64                 `json:"content_id"`
	EpisodeID      *int64                `json:"episode_id,omitempty"`      // Deprecated: use EpisodeResults
	EpisodeResults []EpisodeImportResult `json:"episode_results,omitempty"` // Per-episode outcomes
	FilePath       string                `json:"file_path,omitempty"`       // Deprecated: use EpisodeResults
	FileSize       int64                 `json:"file_size"`                 // Total size
}

// AllSucceeded returns true if all episode imports succeeded.
func (e *ImportCompleted) AllSucceeded() bool {
	if len(e.EpisodeResults) == 0 {
		return true // No episodes means single-file import, which succeeded if event was emitted
	}
	for _, r := range e.EpisodeResults {
		if !r.Success {
			return false
		}
	}
	return true
}

// SuccessCount returns the number of successfully imported episodes.
func (e *ImportCompleted) SuccessCount() int {
	count := 0
	for _, r := range e.EpisodeResults {
		if r.Success {
			count++
		}
	}
	return count
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
