// internal/api/v1/types.go
package v1

import "time"

// contentResponse is the API representation of content.
type contentResponse struct {
	ID             int64     `json:"id"`
	Type           string    `json:"type"`
	TMDBID         *int64    `json:"tmdb_id,omitempty"`
	TVDBID         *int64    `json:"tvdb_id,omitempty"`
	Title          string    `json:"title"`
	Year           int       `json:"year"`
	Status         string    `json:"status"`
	QualityProfile string    `json:"quality_profile"`
	RootPath       string    `json:"root_path"`
	AddedAt        time.Time `json:"added_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// listContentResponse is the response for GET /content.
type listContentResponse struct {
	Items  []contentResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// addContentRequest is the request body for POST /content.
type addContentRequest struct {
	Type           string `json:"type"`
	TMDBID         *int64 `json:"tmdb_id,omitempty"`
	TVDBID         *int64 `json:"tvdb_id,omitempty"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	QualityProfile string `json:"quality_profile"`
	RootPath       string `json:"root_path,omitempty"`
}

// updateContentRequest is the request body for PUT /content/:id.
type updateContentRequest struct {
	Status         *string `json:"status,omitempty"`
	QualityProfile *string `json:"quality_profile,omitempty"`
}

// episodeResponse is the API representation of an episode.
type episodeResponse struct {
	ID        int64      `json:"id"`
	ContentID int64      `json:"content_id"`
	Season    int        `json:"season"`
	Episode   int        `json:"episode"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	AirDate   *time.Time `json:"air_date,omitempty"`
}

// listEpisodesResponse is the response for GET /content/:id/episodes.
type listEpisodesResponse struct {
	Items []episodeResponse `json:"items"`
	Total int               `json:"total"`
}

// updateEpisodeRequest is the request body for PUT /episodes/:id.
type updateEpisodeRequest struct {
	Status *string `json:"status,omitempty"`
}

// searchRequest is the request body for POST /search.
type searchRequest struct {
	ContentID *int64 `json:"content_id,omitempty"`
	Query     string `json:"query,omitempty"`
	Type      string `json:"type,omitempty"`
	Season    *int   `json:"season,omitempty"`
	Episode   *int   `json:"episode,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

// releaseResponse is the API representation of a search result.
type releaseResponse struct {
	Title       string    `json:"title"`
	Indexer     string    `json:"indexer"`
	GUID        string    `json:"guid"`
	DownloadURL string    `json:"download_url"`
	Size        int64     `json:"size"`
	PublishDate time.Time `json:"publish_date"`
	Quality     string    `json:"quality,omitempty"`
	Score       int       `json:"score"`
}

// searchResponse is the response for POST /search.
type searchResponse struct {
	Releases []releaseResponse `json:"releases"`
	Errors   []string          `json:"errors,omitempty"`
}

// grabRequest is the request body for POST /grab.
type grabRequest struct {
	ContentID   int64  `json:"content_id"`
	EpisodeID   *int64 `json:"episode_id,omitempty"`
	DownloadURL string `json:"download_url"`
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
}

// grabResponse is the response for POST /grab.
type grabResponse struct {
	DownloadID int64  `json:"download_id"`
	Status     string `json:"status"`
}

// downloadResponse is the API representation of a download.
type downloadResponse struct {
	ID          int64      `json:"id"`
	ContentID   int64      `json:"content_id"`
	EpisodeID   *int64     `json:"episode_id,omitempty"`
	Client      string     `json:"client"`
	ClientID    string     `json:"client_id"`
	Status      string     `json:"status"`
	ReleaseName string     `json:"release_name"`
	Indexer     string     `json:"indexer"`
	AddedAt     time.Time  `json:"added_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Live status from download client (only present for active downloads)
	Progress *float64 `json:"progress,omitempty"` // 0-100
	Size     *int64   `json:"size,omitempty"`     // bytes
	Speed    *int64   `json:"speed,omitempty"`    // bytes/sec
	ETA      *string  `json:"eta,omitempty"`      // human readable
}

// listDownloadsResponse is the response for GET /downloads.
type listDownloadsResponse struct {
	Items []downloadResponse `json:"items"`
	Total int                `json:"total"`
}

// historyResponse is the API representation of a history entry.
type historyResponse struct {
	ID        int64     `json:"id"`
	ContentID int64     `json:"content_id"`
	EpisodeID *int64    `json:"episode_id,omitempty"`
	Event     string    `json:"event"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

// listHistoryResponse is the response for GET /history.
type listHistoryResponse struct {
	Items []historyResponse `json:"items"`
	Total int               `json:"total"`
}

// fileResponse is the API representation of a file.
type fileResponse struct {
	ID        int64     `json:"id"`
	ContentID int64     `json:"content_id"`
	EpisodeID *int64    `json:"episode_id,omitempty"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Quality   string    `json:"quality"`
	Source    string    `json:"source"`
	AddedAt   time.Time `json:"added_at"`
}

// listFilesResponse is the response for GET /files.
type listFilesResponse struct {
	Items []fileResponse `json:"items"`
	Total int            `json:"total"`
}

// statusResponse is the response for GET /status.
type statusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// profileResponse is the API representation of a quality profile.
type profileResponse struct {
	Name   string   `json:"name"`
	Accept []string `json:"accept"`
}

// listProfilesResponse is the response for GET /profiles.
type listProfilesResponse struct {
	Profiles []profileResponse `json:"profiles"`
}

// scanRequest is the request body for POST /scan.
type scanRequest struct {
	Path string `json:"path,omitempty"`
}

// plexStatusResponse is the response for GET /plex/status.
type plexStatusResponse struct {
	Connected  bool          `json:"connected"`
	ServerName string        `json:"server_name,omitempty"`
	Version    string        `json:"version,omitempty"`
	Libraries  []plexLibrary `json:"libraries,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// plexLibrary represents a Plex library section.
type plexLibrary struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	ItemCount  int    `json:"item_count"`
	Location   string `json:"location"`
	ScannedAt  int64  `json:"scanned_at"`
	Refreshing bool   `json:"refreshing"`
}

// importRequest is the request body for POST /import.
type importRequest struct {
	// For tracked imports
	DownloadID *int64 `json:"download_id,omitempty"`
	// For manual imports
	Path    string `json:"path,omitempty"`
	Title   string `json:"title,omitempty"`
	Year    int    `json:"year,omitempty"`
	Type    string `json:"type,omitempty"`    // "movie" or "series"
	Quality string `json:"quality,omitempty"` // "1080p", "2160p", etc.
	Season  *int   `json:"season,omitempty"`  // For series
	Episode *int   `json:"episode,omitempty"` // For series
}

// importResponse is the response for POST /import.
type importResponse struct {
	FileID       int64  `json:"file_id"`
	ContentID    int64  `json:"content_id"`
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	SizeBytes    int64  `json:"size_bytes"`
	PlexNotified bool   `json:"plex_notified"`
}

// plexScanRequest is the request body for POST /plex/scan.
type plexScanRequest struct {
	Libraries []string `json:"libraries"` // Empty = all libraries
}

// plexScanResponse is the response for POST /plex/scan.
type plexScanResponse struct {
	Scanned []string `json:"scanned"`
}

// plexItemResponse is a Plex library item with tracking status.
type plexItemResponse struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	AddedAt   int64  `json:"added_at"`
	FilePath  string `json:"file_path,omitempty"`
	Tracked   bool   `json:"tracked"`
	ContentID *int64 `json:"content_id,omitempty"`
}

// plexListResponse is the response for GET /plex/libraries/{name}/items.
type plexListResponse struct {
	Library string             `json:"library"`
	Items   []plexItemResponse `json:"items"`
	Total   int                `json:"total"`
}

// plexSearchResponse is the response for GET /plex/search.
type plexSearchResponse struct {
	Query string             `json:"query"`
	Items []plexItemResponse `json:"items"`
	Total int                `json:"total"`
}

// DashboardResponse is the response for GET /dashboard with aggregated stats.
type DashboardResponse struct {
	Version     string `json:"version"`
	Connections struct {
		Server  bool `json:"server"`
		Plex    bool `json:"plex"`
		SABnzbd bool `json:"sabnzbd"`
	} `json:"connections"`
	Downloads struct {
		Queued      int `json:"queued"`
		Downloading int `json:"downloading"`
		Completed   int `json:"completed"`
		Imported    int `json:"imported"`
		Cleaned     int `json:"cleaned"`
		Failed      int `json:"failed"`
	} `json:"downloads"`
	Stuck struct {
		Count     int   `json:"count"`
		Threshold int64 `json:"threshold_minutes"`
	} `json:"stuck"`
	Library struct {
		Movies int `json:"movies"`
		Series int `json:"series"`
	} `json:"library"`
}
