// Package compat implements Radarr/Sonarr API compatibility for Overseerr.
package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
	"github.com/vmunix/arrgo/internal/tmdb"
)

// Config holds compat API configuration.
type Config struct {
	APIKey          string
	MovieRoot       string
	SeriesRoot      string
	QualityProfiles map[string]int // name -> id mapping
}

// radarrAddRequest is the Radarr format for adding a movie.
type radarrAddRequest struct {
	TMDBID              int64  `json:"tmdbId"`
	Title               string `json:"title"`
	Year                int    `json:"year"`
	QualityProfileID    int    `json:"qualityProfileId"`
	ProfileID           int    `json:"profileId"` // Overseerr sends both
	RootFolderPath      string `json:"rootFolderPath"`
	Monitored           bool   `json:"monitored"`
	TitleSlug           string `json:"titleSlug"`           // Overseerr sets to tmdbId
	MinimumAvailability string `json:"minimumAvailability"` // e.g., "released"
	Tags                []int  `json:"tags"`
	AddOptions          struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// radarrMovieResponse is the Radarr format for a movie.
type radarrMovieResponse struct {
	ID               int64  `json:"id"`
	TMDBID           int64  `json:"tmdbId"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	Monitored        bool   `json:"monitored"`
	Status           string `json:"status"`
	HasFile          bool   `json:"hasFile"`
	IsAvailable      bool   `json:"isAvailable"`
	Path             string `json:"path"`
	FolderName       string `json:"folderName"`
	TitleSlug        string `json:"titleSlug"`
	QualityProfileID int    `json:"qualityProfileId"`
	Tags             []int  `json:"tags"`
	Added            string `json:"added"`
}

// sonarrSeason represents a season in Sonarr format.
type sonarrSeason struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
	Statistics   *struct {
		EpisodeFileCount  int `json:"episodeFileCount"`
		EpisodeCount      int `json:"episodeCount"`
		TotalEpisodeCount int `json:"totalEpisodeCount"`
		SizeOnDisk        int `json:"sizeOnDisk"`
		PercentOfEpisodes int `json:"percentOfEpisodes"`
	} `json:"statistics,omitempty"`
}

// sonarrSeriesResponse is the full Sonarr format for a series.
type sonarrSeriesResponse struct {
	ID          int64          `json:"id,omitempty"`
	TVDBID      int64          `json:"tvdbId"`
	Title       string         `json:"title"`
	SortTitle   string         `json:"sortTitle"`
	Year        int            `json:"year"`
	SeasonCount int            `json:"seasonCount"`
	Seasons     []sonarrSeason `json:"seasons"`
	Status      string         `json:"status"`
	Overview    string         `json:"overview,omitempty"`
	Network     string         `json:"network,omitempty"`
	Runtime     int            `json:"runtime,omitempty"`
	Images      []struct {
		CoverType string `json:"coverType"`
		URL       string `json:"url"`
	} `json:"images,omitempty"`
	SeriesType        string   `json:"seriesType"`
	Monitored         bool     `json:"monitored"`
	QualityProfileID  int      `json:"qualityProfileId"`
	LanguageProfileID int      `json:"languageProfileId"`
	SeasonFolder      bool     `json:"seasonFolder"`
	Path              string   `json:"path,omitempty"`
	RootFolderPath    string   `json:"rootFolderPath,omitempty"`
	TitleSlug         string   `json:"titleSlug"`
	Certification     string   `json:"certification,omitempty"`
	Genres            []string `json:"genres,omitempty"`
	Tags              []int    `json:"tags"`
	Added             string   `json:"added,omitempty"`
	FirstAired        string   `json:"firstAired,omitempty"`
	CleanTitle        string   `json:"cleanTitle"`
	ImdbID            string   `json:"imdbId,omitempty"`
	Statistics        *struct {
		SeasonCount       int `json:"seasonCount"`
		EpisodeFileCount  int `json:"episodeFileCount"`
		EpisodeCount      int `json:"episodeCount"`
		TotalEpisodeCount int `json:"totalEpisodeCount"`
		SizeOnDisk        int `json:"sizeOnDisk"`
		PercentOfEpisodes int `json:"percentOfEpisodes"`
	} `json:"statistics,omitempty"`
}

// sonarrAddRequest is the Sonarr format for adding a series (full Overseerr format).
type sonarrAddRequest struct {
	TVDBID            int64  `json:"tvdbId"`
	Title             string `json:"title"`
	Year              int    `json:"year"`
	QualityProfileID  int    `json:"qualityProfileId"`
	LanguageProfileID int    `json:"languageProfileId"`
	Seasons           []int  `json:"seasons"` // Season numbers to monitor
	SeasonFolder      bool   `json:"seasonFolder"`
	RootFolderPath    string `json:"rootFolderPath"`
	SeriesType        string `json:"seriesType"` // standard, daily, anime
	Monitored         bool   `json:"monitored"`
	Tags              []int  `json:"tags"`
	AddOptions        struct {
		IgnoreEpisodesWithFiles  bool `json:"ignoreEpisodesWithFiles"`
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// Server provides Radarr/Sonarr API compatibility.
type Server struct {
	cfg       Config
	library   *library.Store
	downloads *download.Store
	searcher  *search.Searcher
	manager   *download.Manager
	tmdb      *tmdb.Client
}

// New creates a new compatibility server.
func New(cfg Config, lib *library.Store, dl *download.Store) *Server {
	return &Server{
		cfg:       cfg,
		library:   lib,
		downloads: dl,
	}
}

// SetSearcher configures the searcher (optional).
func (s *Server) SetSearcher(searcher *search.Searcher) {
	s.searcher = searcher
}

// SetManager configures the download manager (optional).
func (s *Server) SetManager(manager *download.Manager) {
	s.manager = manager
}

// SetTMDB configures the TMDB client (optional).
func (s *Server) SetTMDB(client *tmdb.Client) {
	s.tmdb = client
}

// RegisterRoutes registers compatibility API routes.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// System endpoints (used by Overseerr to test connection)
	mux.HandleFunc("GET /api/v3/system/status", s.authMiddleware(s.systemStatus))

	// Radarr compatibility
	mux.HandleFunc("GET /api/v3/movie", s.authMiddleware(s.listMovies))
	mux.HandleFunc("GET /api/v3/movie/lookup", s.authMiddleware(s.lookupMovie))
	mux.HandleFunc("GET /api/v3/movie/{id}", s.authMiddleware(s.getMovie))
	mux.HandleFunc("POST /api/v3/movie", s.authMiddleware(s.addMovie))
	mux.HandleFunc("PUT /api/v3/movie", s.authMiddleware(s.updateMovie))
	mux.HandleFunc("GET /api/v3/rootfolder", s.authMiddleware(s.listRootFolders))
	mux.HandleFunc("GET /api/v3/qualityprofile", s.authMiddleware(s.listQualityProfiles))
	mux.HandleFunc("GET /api/v3/qualityProfile", s.authMiddleware(s.listQualityProfiles)) // Radarr uses camelCase
	mux.HandleFunc("GET /api/v3/queue", s.authMiddleware(s.listQueue))
	mux.HandleFunc("POST /api/v3/command", s.authMiddleware(s.executeCommand))
	mux.HandleFunc("GET /api/v3/tag", s.authMiddleware(s.listTags))
	mux.HandleFunc("POST /api/v3/tag", s.authMiddleware(s.createTag))

	// Sonarr compatibility
	mux.HandleFunc("GET /api/v3/series", s.authMiddleware(s.listSeries))
	mux.HandleFunc("GET /api/v3/series/lookup", s.authMiddleware(s.lookupSeries))
	mux.HandleFunc("GET /api/v3/series/{id}", s.authMiddleware(s.getSeries))
	mux.HandleFunc("POST /api/v3/series", s.authMiddleware(s.addSeries))
	mux.HandleFunc("PUT /api/v3/series", s.authMiddleware(s.updateSeries))
	mux.HandleFunc("GET /api/v3/languageprofile", s.authMiddleware(s.listLanguageProfiles))
}

// authMiddleware validates the X-Api-Key header.
// If no API key is configured on server, any non-empty key is accepted (testing mode).
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Api-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("apikey")
		}

		// If server has no API key configured, accept any non-empty key (testing mode)
		if s.cfg.APIKey == "" {
			if apiKey != "" {
				next(w, r)
				return
			}
			// No key from client either - still allow for direct testing
			next(w, r)
			return
		}

		// Server has API key configured - must match
		if apiKey != s.cfg.APIKey {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// System handlers

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   "1.0.0",
		"appName":   "arrgo",
		"startTime": "2026-01-19T00:00:00Z",
	})
}

// Radarr handlers

func (s *Server) listMovies(w http.ResponseWriter, r *http.Request) {
	contentType := library.ContentTypeMovie
	contents, _, err := s.library.ListContent(library.ContentFilter{Type: &contentType})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	movies := make([]radarrMovieResponse, 0, len(contents))
	for _, c := range contents {
		movies = append(movies, s.contentToRadarrMovie(c))
	}
	writeJSON(w, http.StatusOK, movies)
}

func (s *Server) lookupMovie(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Parse tmdb:12345 format
	var tmdbID int64
	if _, err := fmt.Sscanf(term, "tmdb:%d", &tmdbID); err != nil {
		// Not a TMDB lookup, return empty
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Check if we have this movie
	contents, _, err := s.library.ListContent(library.ContentFilter{TMDBID: &tmdbID, Limit: 1})
	if err == nil && len(contents) > 0 {
		// Found in library - return with ID
		// For "wanted" items (no file yet), return monitored=false so Overseerr
		// sends a PUT to re-enable monitoring, which triggers a search.
		resp := s.contentToRadarrMovie(contents[0])
		if contents[0].Status == library.StatusWanted {
			resp.Monitored = false
		}
		writeJSON(w, http.StatusOK, []radarrMovieResponse{resp})
		return
	}

	// Not in library - fetch metadata from TMDB if available
	response := map[string]any{
		"tmdbId":      tmdbID,
		"title":       "",
		"year":        0,
		"monitored":   false,
		"hasFile":     false,
		"isAvailable": false,
	}

	// Enrich with TMDB metadata if client configured
	if s.tmdb != nil {
		movie, err := s.tmdb.GetMovie(r.Context(), tmdbID)
		if err == nil {
			response["title"] = movie.Title
			response["year"] = movie.Year()
			response["overview"] = movie.Overview
			response["runtime"] = movie.Runtime
			if movie.PosterPath != "" {
				response["images"] = []map[string]any{{
					"coverType": "poster",
					"url":       movie.PosterURL("w500"),
				}}
			}
			if movie.VoteAverage > 0 {
				response["ratings"] = map[string]any{
					"tmdb": map[string]any{
						"value": movie.VoteAverage,
						"votes": movie.VoteCount,
					},
				}
			}
			if len(movie.Genres) > 0 {
				response["genres"] = movie.Genres
			}
		}
		// On error, continue with stub - graceful degradation
	}

	writeJSON(w, http.StatusOK, []map[string]any{response})
}

func (s *Server) getMovie(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	content, err := s.library.GetContent(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Movie not found"})
		return
	}

	writeJSON(w, http.StatusOK, s.contentToRadarrMovie(content))
}

// contentToRadarrMovie converts library content to Radarr response format.
func (s *Server) contentToRadarrMovie(c *library.Content) radarrMovieResponse {
	var tmdbID int64
	if c.TMDBID != nil {
		tmdbID = *c.TMDBID
	}

	// Determine profile ID from name
	profileID := 1
	for name, id := range s.cfg.QualityProfiles {
		if name == c.QualityProfile {
			profileID = id
			break
		}
	}

	folderName := fmt.Sprintf("%s (%d)", c.Title, c.Year)
	path := fmt.Sprintf("%s/%s", c.RootPath, folderName)

	return radarrMovieResponse{
		ID:               c.ID,
		TMDBID:           tmdbID,
		Title:            c.Title,
		Year:             c.Year,
		Monitored:        c.Status == library.StatusWanted || c.Status == library.StatusAvailable,
		Status:           "released",
		HasFile:          c.Status == library.StatusAvailable,
		IsAvailable:      true,
		Path:             path,
		FolderName:       folderName,
		TitleSlug:        fmt.Sprintf("%d", tmdbID),
		QualityProfileID: profileID,
		Tags:             []int{},
		Added:            c.AddedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *Server) addMovie(w http.ResponseWriter, r *http.Request) {
	var req radarrAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Map quality profile ID to name
	profileName := "hd" // default
	for name, id := range s.cfg.QualityProfiles {
		if id == req.QualityProfileID {
			profileName = name
			break
		}
	}

	// Determine root path
	rootPath := req.RootFolderPath
	if rootPath == "" {
		rootPath = s.cfg.MovieRoot
	}

	// Add to library
	tmdbID := req.TMDBID
	content := &library.Content{
		Type:           library.ContentTypeMovie,
		TMDBID:         &tmdbID,
		Title:          req.Title,
		Year:           req.Year,
		Status:         library.StatusWanted,
		QualityProfile: profileName,
		RootPath:       rootPath,
	}

	if err := s.library.AddContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Auto-search if requested and searcher available
	if req.AddOptions.SearchForMovie && s.searcher != nil && s.manager != nil {
		go s.searchAndGrab(content.ID, req.Title, req.Year, profileName)
	}

	writeJSON(w, http.StatusCreated, s.contentToRadarrMovie(content))
}

func (s *Server) updateMovie(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		Year       int    `json:"year"`
		Monitored  bool   `json:"monitored"`
		Tags       []int  `json:"tags"`
		AddOptions struct {
			SearchForMovie bool `json:"searchForMovie"`
		} `json:"addOptions"`
		QualityProfileID int `json:"qualityProfileId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	content, err := s.library.GetContent(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Movie not found"})
		return
	}

	// Update monitoring status
	if req.Monitored {
		content.Status = library.StatusWanted
	}

	// Update quality profile if provided
	if req.QualityProfileID > 0 {
		for name, id := range s.cfg.QualityProfiles {
			if id == req.QualityProfileID {
				content.QualityProfile = name
				break
			}
		}
	}

	if err := s.library.UpdateContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Trigger search if requested
	if req.AddOptions.SearchForMovie && req.Monitored && s.searcher != nil && s.manager != nil {
		title := req.Title
		if title == "" {
			title = content.Title
		}
		year := req.Year
		if year == 0 {
			year = content.Year
		}
		go s.searchAndGrab(content.ID, title, year, content.QualityProfile)
	}

	writeJSON(w, http.StatusOK, s.contentToRadarrMovie(content))
}

func (s *Server) listRootFolders(w http.ResponseWriter, r *http.Request) {
	folders := []map[string]any{}

	if s.cfg.MovieRoot != "" {
		folders = append(folders, map[string]any{
			"id":        1,
			"path":      s.cfg.MovieRoot,
			"freeSpace": getFreeSpace(s.cfg.MovieRoot),
		})
	}
	if s.cfg.SeriesRoot != "" {
		folders = append(folders, map[string]any{
			"id":        2,
			"path":      s.cfg.SeriesRoot,
			"freeSpace": getFreeSpace(s.cfg.SeriesRoot),
		})
	}

	writeJSON(w, http.StatusOK, folders)
}

// getFreeSpace returns the free space in bytes for a given path.
func getFreeSpace(path string) uint64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	// Bsize is always positive, safe to convert
	if stat.Bsize < 0 {
		return 0
	}
	return stat.Bavail * uint64(stat.Bsize) //nolint:gosec // Bsize checked above
}

func (s *Server) listQualityProfiles(w http.ResponseWriter, r *http.Request) {
	// Map internal profile names to Radarr-style display names
	displayNames := map[string]string{
		"any":    "Any",
		"sd":     "SD",
		"hd":     "HD-1080p",
		"hd720":  "HD-720p",
		"hd1080": "HD-1080p",
		"uhd":    "Ultra-HD",
	}

	profiles := make([]map[string]any, 0, len(s.cfg.QualityProfiles))
	for name, id := range s.cfg.QualityProfiles {
		displayName := name
		if dn, ok := displayNames[strings.ToLower(name)]; ok {
			displayName = dn
		}
		profiles = append(profiles, map[string]any{
			"id":             id,
			"name":           displayName,
			"upgradeAllowed": true,
		})
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) listQueue(w http.ResponseWriter, r *http.Request) {
	downloads, err := s.downloads.List(download.Filter{Active: true})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	records := make([]map[string]any, 0, len(downloads))
	for _, dl := range downloads {
		record := map[string]any{
			"id":                    dl.ID,
			"movieId":               dl.ContentID,
			"title":                 dl.ReleaseName,
			"status":                string(dl.Status),
			"trackedDownloadStatus": "ok",
			"indexer":               dl.Indexer,
		}

		// Fetch live progress from download client if available
		if s.manager != nil {
			if clientStatus, err := s.manager.Client().Status(r.Context(), dl.ClientID); err == nil && clientStatus != nil {
				record["size"] = clientStatus.Size
				record["sizeleft"] = int64(float64(clientStatus.Size) * (100 - clientStatus.Progress) / 100)
				record["status"] = string(clientStatus.Status)

				// Format timeleft as HH:MM:SS
				if clientStatus.ETA > 0 {
					eta := clientStatus.ETA
					hours := int(eta.Hours())
					minutes := int(eta.Minutes()) % 60
					seconds := int(eta.Seconds()) % 60
					record["timeleft"] = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
					record["estimatedCompletionTime"] = time.Now().Add(eta).UTC().Format(time.RFC3339)
				} else {
					record["timeleft"] = "00:00:00"
					record["estimatedCompletionTime"] = time.Now().UTC().Format(time.RFC3339)
				}
			}
		}

		records = append(records, record)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"page":         1,
		"pageSize":     len(records),
		"totalRecords": len(records),
		"records":      records,
	})
}

func (s *Server) executeCommand(w http.ResponseWriter, r *http.Request) {
	// Handle commands like MoviesSearch, RefreshMovie
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// TODO: dispatch to appropriate handler
	writeJSON(w, http.StatusOK, map[string]any{
		"id":     1,
		"name":   req.Name,
		"status": "queued",
	})
}

// Sonarr handlers

func (s *Server) listSeries(w http.ResponseWriter, r *http.Request) {
	contentType := library.ContentTypeSeries
	contents, _, err := s.library.ListContent(library.ContentFilter{Type: &contentType})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	series := make([]sonarrSeriesResponse, 0, len(contents))
	for _, c := range contents {
		series = append(series, s.contentToSonarrSeries(c))
	}
	writeJSON(w, http.StatusOK, series)
}

func (s *Server) getSeries(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	content, err := s.library.GetContent(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	if content.Type != library.ContentTypeSeries {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	writeJSON(w, http.StatusOK, s.contentToSonarrSeries(content))
}

func (s *Server) lookupSeries(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Parse tvdb:12345 format
	var tvdbID int64
	if _, err := fmt.Sscanf(term, "tvdb:%d", &tvdbID); err != nil {
		// Not a TVDB lookup - could be title search, return empty for now
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Check if we have this series in library
	contents, _, err := s.library.ListContent(library.ContentFilter{TVDBID: &tvdbID, Limit: 1})
	if err == nil && len(contents) > 0 {
		// Found in library - return with ID (signals "already exists")
		writeJSON(w, http.StatusOK, []sonarrSeriesResponse{s.contentToSonarrSeries(contents[0])})
		return
	}

	// Not in library - return stub that Overseerr can use to add
	// Overseerr will fill in metadata from TMDB/TVDB on its side
	response := sonarrSeriesResponse{
		TVDBID:            tvdbID,
		Title:             "",
		SortTitle:         "",
		Year:              0,
		SeasonCount:       1,
		Seasons:           []sonarrSeason{{SeasonNumber: 1, Monitored: false}},
		Status:            "continuing",
		SeriesType:        "standard",
		Monitored:         false,
		QualityProfileID:  1,
		LanguageProfileID: 1,
		SeasonFolder:      true,
		TitleSlug:         fmt.Sprintf("tvdb-%d", tvdbID),
		Tags:              []int{},
		CleanTitle:        "",
	}

	writeJSON(w, http.StatusOK, []sonarrSeriesResponse{response})
}

// contentToSonarrSeries converts library content to Sonarr response format.
func (s *Server) contentToSonarrSeries(c *library.Content) sonarrSeriesResponse {
	var tvdbID int64
	if c.TVDBID != nil {
		tvdbID = *c.TVDBID
	}

	// Determine profile ID from name
	profileID := 1
	for name, id := range s.cfg.QualityProfiles {
		if name == c.QualityProfile {
			profileID = id
			break
		}
	}

	// Build path
	path := fmt.Sprintf("%s/%s", c.RootPath, c.Title)

	// Default to 1 season if we don't have episode data
	seasons := []sonarrSeason{{SeasonNumber: 1, Monitored: true}}

	return sonarrSeriesResponse{
		ID:                c.ID,
		TVDBID:            tvdbID,
		Title:             c.Title,
		SortTitle:         strings.ToLower(c.Title),
		Year:              c.Year,
		SeasonCount:       len(seasons),
		Seasons:           seasons,
		Status:            "continuing",
		SeriesType:        "standard",
		Monitored:         c.Status == library.StatusWanted,
		QualityProfileID:  profileID,
		LanguageProfileID: 1,
		SeasonFolder:      true,
		Path:              path,
		RootFolderPath:    c.RootPath,
		TitleSlug:         fmt.Sprintf("tvdb-%d", tvdbID),
		Tags:              []int{},
		Added:             c.AddedAt.Format("2006-01-02T15:04:05Z"),
		CleanTitle:        strings.ToLower(strings.ReplaceAll(c.Title, " ", "")),
	}
}

func (s *Server) addSeries(w http.ResponseWriter, r *http.Request) {
	var req sonarrAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Check if series already exists
	contents, _, err := s.library.ListContent(library.ContentFilter{TVDBID: &req.TVDBID, Limit: 1})
	if err == nil && len(contents) > 0 {
		existing := contents[0]
		// Update monitoring status if needed
		if req.Monitored && existing.Status != library.StatusWanted {
			existing.Status = library.StatusWanted
			if err := s.library.UpdateContent(existing); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, s.contentToSonarrSeries(existing))
		return
	}

	// Map quality profile ID to name
	profileName := "hd"
	for name, id := range s.cfg.QualityProfiles {
		if id == req.QualityProfileID {
			profileName = name
			break
		}
	}

	// Determine root path
	rootPath := req.RootFolderPath
	if rootPath == "" {
		rootPath = s.cfg.SeriesRoot
	}

	// Add to library
	tvdbID := req.TVDBID
	content := &library.Content{
		Type:           library.ContentTypeSeries,
		TVDBID:         &tvdbID,
		Title:          req.Title,
		Year:           req.Year,
		Status:         library.StatusWanted,
		QualityProfile: profileName,
		RootPath:       rootPath,
	}

	if err := s.library.AddContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Auto-search if requested
	if req.AddOptions.SearchForMissingEpisodes && s.searcher != nil && s.manager != nil {
		go s.searchAndGrabSeries(content.ID, req.Title, req.Year, profileName)
	}

	writeJSON(w, http.StatusCreated, s.contentToSonarrSeries(content))
}

func (s *Server) updateSeries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        int64          `json:"id"`
		Monitored bool           `json:"monitored"`
		Seasons   []sonarrSeason `json:"seasons"`
		Tags      []int          `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	content, err := s.library.GetContent(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	// Update monitoring status
	if req.Monitored {
		content.Status = library.StatusWanted
	}

	if err := s.library.UpdateContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, s.contentToSonarrSeries(content))
}

// Tag handlers (Overseerr uses these for per-user tagging)

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	// Return empty tags list - we don't support tags yet
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Return a fake tag - we don't persist these yet
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    1,
		"label": req.Label,
	})
}

// Language profile handler (Sonarr v3+ requires this)

func (s *Server) listLanguageProfiles(w http.ResponseWriter, r *http.Request) {
	// Return a single "English" profile - arrgo doesn't filter by language
	writeJSON(w, http.StatusOK, []map[string]any{
		{
			"id":   1,
			"name": "English",
		},
	})
}

// searchAndGrab performs a background search and grabs the best result.
func (s *Server) searchAndGrab(contentID int64, title string, year int, profile string) {
	ctx := context.Background()

	query := search.Query{
		Text: fmt.Sprintf("%s %d", title, year),
		Type: "movie",
	}

	result, err := s.searcher.Search(ctx, query, profile)
	if err != nil || len(result.Releases) == 0 {
		return
	}

	// Grab the best match (first result after scoring/sorting)
	best := result.Releases[0]
	_, _ = s.manager.Grab(ctx, contentID, nil, best.DownloadURL, best.Title, best.Indexer)
}

// searchAndGrabSeries performs a background search for series (season pack or latest episode).
func (s *Server) searchAndGrabSeries(contentID int64, title string, _ int, profile string) {
	ctx := context.Background()

	// Search for season 1 pack first
	query := search.Query{
		Text: fmt.Sprintf("%s S01", title),
		Type: "series",
	}

	result, err := s.searcher.Search(ctx, query, profile)
	if err != nil || len(result.Releases) == 0 {
		return
	}

	// Grab the best match
	best := result.Releases[0]
	_, _ = s.manager.Grab(ctx, contentID, nil, best.DownloadURL, best.Title, best.Indexer)
}
