// Package compat implements Radarr/Sonarr API compatibility for Overseerr.
package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/metadata"
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
	IMDBID           string `json:"imdbId,omitempty"`
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
	TVDBID            int64          `json:"tvdbId"`
	Title             string         `json:"title"`
	Year              int            `json:"year"`
	QualityProfileID  int            `json:"qualityProfileId"`
	LanguageProfileID int            `json:"languageProfileId"`
	Seasons           []sonarrSeason `json:"seasons"` // Seasons with monitored status
	SeasonFolder      bool           `json:"seasonFolder"`
	RootFolderPath    string         `json:"rootFolderPath"`
	SeriesType        string         `json:"seriesType"` // standard, daily, anime
	Monitored         bool           `json:"monitored"`
	Tags              []int          `json:"tags"`
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
	tvdbSvc   *metadata.TVDBService
	bus       *events.Bus // Optional event bus for event-driven grabs
	log       *slog.Logger
}

// New creates a new compatibility server.
func New(cfg Config, lib *library.Store, dl *download.Store, log *slog.Logger) *Server {
	return &Server{
		cfg:       cfg,
		library:   lib,
		downloads: dl,
		log:       log,
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

// SetBus configures the event bus for event-driven grabs (optional).
func (s *Server) SetBus(bus *events.Bus) {
	s.bus = bus
}

// SetTVDB configures the TVDB service (optional).
func (s *Server) SetTVDB(svc *metadata.TVDBService) {
	s.tvdbSvc = svc
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
			if movie.IMDBID != "" {
				response["imdbId"] = movie.IMDBID
			}
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

	// Publish ContentAdded event
	if s.bus != nil {
		evt := &events.ContentAdded{
			BaseEvent:      events.NewBaseEvent(events.EventContentAdded, events.EntityContent, content.ID),
			ContentID:      content.ID,
			ContentType:    string(content.Type),
			Title:          content.Title,
			Year:           content.Year,
			QualityProfile: content.QualityProfile,
		}
		_ = s.bus.Publish(r.Context(), evt)
	}

	// Auto-search if requested and searcher available
	if req.AddOptions.SearchForMovie && s.searcher != nil && s.bus != nil {
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
	if req.AddOptions.SearchForMovie && req.Monitored && s.searcher != nil && s.bus != nil {
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
			"type":      "movie",
		})
	}
	if s.cfg.SeriesRoot != "" {
		folders = append(folders, map[string]any{
			"id":        2,
			"path":      s.cfg.SeriesRoot,
			"freeSpace": getFreeSpace(s.cfg.SeriesRoot),
			"type":      "series",
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
	// Note: No pagination for compat API - returns all active for Radarr/Sonarr compatibility
	downloads, _, err := s.downloads.List(download.Filter{Active: true})
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
	var req struct {
		Name     string  `json:"name"`
		MovieIDs []int64 `json:"movieIds"` // For MoviesSearch
		SeriesID int64   `json:"seriesId"` // For SeriesSearch
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Dispatch based on command name
	switch req.Name {
	case "MoviesSearch":
		if s.searcher != nil && s.bus != nil && len(req.MovieIDs) > 0 {
			for _, movieID := range req.MovieIDs {
				content, err := s.library.GetContent(movieID)
				if err != nil {
					continue
				}
				go s.searchAndGrab(content.ID, content.Title, content.Year, content.QualityProfile)
			}
		}
	case "SeriesSearch":
		if s.searcher != nil && s.bus != nil && req.SeriesID > 0 {
			content, err := s.library.GetContent(req.SeriesID)
			if err == nil {
				// Search for season 1 by default (full series search not supported yet)
				go s.searchAndGrabSeries(content.ID, content.Title, content.QualityProfile, []int{1})
			}
		}
	}

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
		resp := s.contentToSonarrSeries(contents[0])
		// For "wanted" items, return monitored=false so Overseerr sends PUT to trigger search
		// (Overseerr skips items with monitored=true, thinking they're already handled)
		if contents[0].Status == library.StatusWanted {
			resp.Monitored = false
		}

		// Enrich with TVDB metadata if year is missing and TVDB is configured
		if resp.Year == 0 && s.tvdbSvc != nil {
			series, err := s.tvdbSvc.GetSeries(r.Context(), int(tvdbID))
			if err == nil {
				resp.Year = series.Year
				if resp.Overview == "" {
					resp.Overview = series.Overview
				}
			}
		}

		writeJSON(w, http.StatusOK, []sonarrSeriesResponse{resp})
		return
	}

	// Not in library - return stub that Overseerr can use to add
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

	// Enrich with TVDB metadata if service is configured
	if s.tvdbSvc != nil {
		series, err := s.tvdbSvc.GetSeries(r.Context(), int(tvdbID))
		if err == nil {
			response.Title = series.Name
			response.SortTitle = strings.ToLower(series.Name)
			response.Year = series.Year
			response.Overview = series.Overview
			response.CleanTitle = strings.ToLower(strings.ReplaceAll(series.Name, " ", ""))

			// Map TVDB status to Sonarr status format
			if series.Status == "Ended" {
				response.Status = "ended"
			} else {
				response.Status = "continuing"
			}

			// Get episodes to determine accurate season count
			episodes, err := s.tvdbSvc.GetEpisodes(r.Context(), int(tvdbID))
			if err == nil && len(episodes) > 0 {
				// Find unique season numbers (excluding specials, season 0)
				seasonMap := make(map[int]bool)
				for _, ep := range episodes {
					if ep.Season > 0 {
						seasonMap[ep.Season] = true
					}
				}

				// Build seasons array
				response.Seasons = make([]sonarrSeason, 0, len(seasonMap))
				for seasonNum := range seasonMap {
					response.Seasons = append(response.Seasons, sonarrSeason{
						SeasonNumber: seasonNum,
						Monitored:    false,
					})
				}
				response.SeasonCount = len(seasonMap)
			}
		}
		// On error, continue with stub response - graceful degradation
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

	// Query episodes to get actual season numbers and availability
	seasons := []sonarrSeason{}
	episodes, _, err := s.library.ListEpisodes(library.EpisodeFilter{ContentID: &c.ID})
	if err == nil && len(episodes) > 0 {
		// Track season numbers and whether each has any available episodes
		seasonHasAvailable := make(map[int]bool)
		for _, ep := range episodes {
			if ep.Season > 0 {
				// Initialize season if not seen yet
				if _, exists := seasonHasAvailable[ep.Season]; !exists {
					seasonHasAvailable[ep.Season] = false
				}
				// Mark season as available if any episode is available
				if ep.Status == library.StatusAvailable {
					seasonHasAvailable[ep.Season] = true
				}
			}
		}
		// Build seasons array - only mark as monitored if season has available episodes
		// This prevents Overseerr from thinking all seasons are already requested
		for seasonNum, hasAvailable := range seasonHasAvailable {
			seasons = append(seasons, sonarrSeason{SeasonNumber: seasonNum, Monitored: hasAvailable})
		}
	}
	// Default to 1 season if we don't have episode data
	if len(seasons) == 0 {
		seasons = []sonarrSeason{{SeasonNumber: 1, Monitored: false}}
	}

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

	// Debug: Log what Overseerr is requesting
	var monitoredSeasons []int
	for _, season := range req.Seasons {
		if season.Monitored {
			monitoredSeasons = append(monitoredSeasons, season.SeasonNumber)
		}
	}
	s.log.Debug("addSeries request",
		"title", req.Title,
		"tvdbId", req.TVDBID,
		"monitored", req.Monitored,
		"searchForMissing", req.AddOptions.SearchForMissingEpisodes,
		"totalSeasons", len(req.Seasons),
		"monitoredSeasons", monitoredSeasons,
	)

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

	// Sync episodes from TVDB if available
	if s.tvdbSvc != nil && tvdbID > 0 {
		go s.syncEpisodesFromTVDB(content.ID, int(tvdbID))
	}

	// Publish ContentAdded event
	if s.bus != nil {
		evt := &events.ContentAdded{
			BaseEvent:      events.NewBaseEvent(events.EventContentAdded, events.EntityContent, content.ID),
			ContentID:      content.ID,
			ContentType:    string(content.Type),
			Title:          content.Title,
			Year:           content.Year,
			QualityProfile: content.QualityProfile,
		}
		_ = s.bus.Publish(r.Context(), evt)
	}

	// Auto-search if requested
	if req.AddOptions.SearchForMissingEpisodes && s.searcher != nil && s.bus != nil {
		// Extract monitored season numbers
		var monitoredSeasons []int
		for _, season := range req.Seasons {
			if season.Monitored && season.SeasonNumber > 0 {
				monitoredSeasons = append(monitoredSeasons, season.SeasonNumber)
			}
		}
		go s.searchAndGrabSeries(content.ID, req.Title, profileName, monitoredSeasons)
	}

	writeJSON(w, http.StatusCreated, s.contentToSonarrSeries(content))
}

func (s *Server) updateSeries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         int64          `json:"id"`
		Monitored  bool           `json:"monitored"`
		Seasons    []sonarrSeason `json:"seasons"`
		Tags       []int          `json:"tags"`
		AddOptions struct {
			SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
		} `json:"addOptions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// Debug: Log what Overseerr is requesting
	var monitoredSeasons []int
	for _, season := range req.Seasons {
		if season.Monitored {
			monitoredSeasons = append(monitoredSeasons, season.SeasonNumber)
		}
	}
	s.log.Debug("updateSeries request",
		"id", req.ID,
		"monitored", req.Monitored,
		"searchForMissing", req.AddOptions.SearchForMissingEpisodes,
		"totalSeasons", len(req.Seasons),
		"monitoredSeasons", monitoredSeasons,
	)

	content, err := s.library.GetContent(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
		return
	}

	// Track if we need to trigger search
	shouldSearch := req.Monitored && content.Status == library.StatusWanted && s.searcher != nil && s.bus != nil

	// Update monitoring status
	if req.Monitored {
		content.Status = library.StatusWanted
	}

	if err := s.library.UpdateContent(content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Trigger search for re-request flow (Overseerr sends PUT when re-requesting wanted series)
	if shouldSearch || req.AddOptions.SearchForMissingEpisodes {
		// Extract monitored season numbers
		var monitoredSeasons []int
		for _, season := range req.Seasons {
			if season.Monitored && season.SeasonNumber > 0 {
				monitoredSeasons = append(monitoredSeasons, season.SeasonNumber)
			}
		}
		go s.searchAndGrabSeries(content.ID, content.Title, content.QualityProfile, monitoredSeasons)
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
	if s.searcher == nil {
		s.log.Warn("no searcher configured, cannot search")
		return
	}
	if s.bus == nil {
		s.log.Warn("no event bus configured, cannot grab")
		return
	}
	ctx := context.Background()

	query := search.Query{
		Text: fmt.Sprintf("%s %d", title, year),
		Type: "movie",
	}

	result, err := s.searcher.Search(ctx, query, profile)
	if err != nil || len(result.Releases) == 0 {
		s.log.Warn("search failed or no results", "title", title, "year", year)
		return
	}

	// Grab the best match (first result after scoring/sorting)
	best := result.Releases[0]

	if err := s.bus.Publish(ctx, &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   contentID,
		DownloadURL: best.DownloadURL,
		ReleaseName: best.Title,
		Indexer:     best.Indexer,
	}); err != nil {
		s.log.Error("failed to publish GrabRequested", "error", err)
	}
}

// searchAndGrabSeries performs a background search for series seasons.
func (s *Server) searchAndGrabSeries(contentID int64, title string, profile string, seasons []int) {
	if s.searcher == nil {
		s.log.Warn("no searcher configured, cannot search")
		return
	}
	if s.bus == nil {
		s.log.Warn("no event bus configured, cannot grab")
		return
	}
	ctx := context.Background()

	// Default to season 1 if no specific seasons requested
	if len(seasons) == 0 {
		seasons = []int{1}
	}

	// Search for each monitored season
	for _, seasonNum := range seasons {
		season := seasonNum // Create a copy for the pointer
		query := search.Query{
			Text:   fmt.Sprintf("%s S%02d", title, season),
			Type:   "series",
			Season: &season, // Signal we want season packs, not individual episodes
		}

		result, err := s.searcher.Search(ctx, query, profile)
		if err != nil || len(result.Releases) == 0 {
			s.log.Warn("search failed or no results", "title", title, "season", season)
			continue
		}

		// Grab the best match for this season
		best := result.Releases[0]

		if err := s.bus.Publish(ctx, &events.GrabRequested{
			BaseEvent:        events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
			ContentID:        contentID,
			Season:           &season,
			IsCompleteSeason: true,
			DownloadURL:      best.DownloadURL,
			ReleaseName:      best.Title,
			Indexer:          best.Indexer,
		}); err != nil {
			s.log.Error("failed to publish GrabRequested", "error", err)
		}
	}
}

// syncEpisodesFromTVDB fetches episodes from TVDB and creates Episode records.
func (s *Server) syncEpisodesFromTVDB(contentID int64, tvdbID int) {
	ctx := context.Background()

	episodes, err := s.tvdbSvc.GetEpisodes(ctx, tvdbID)
	if err != nil {
		s.log.Warn("failed to fetch episodes from TVDB", "tvdb_id", tvdbID, "error", err)
		return
	}

	// Convert to library.Episode
	libEpisodes := make([]*library.Episode, 0, len(episodes))
	for _, ep := range episodes {
		// Skip specials (season 0) and episodes without numbers
		if ep.Season == 0 || ep.Episode == 0 {
			continue
		}

		var airDate *time.Time
		if !ep.AirDate.IsZero() {
			airDate = &ep.AirDate
		}

		libEpisodes = append(libEpisodes, &library.Episode{
			ContentID: contentID,
			Season:    ep.Season,
			Episode:   ep.Episode,
			Title:     ep.Name,
			Status:    library.StatusWanted,
			AirDate:   airDate,
		})
	}

	inserted, err := s.library.BulkAddEpisodes(libEpisodes)
	if err != nil {
		s.log.Warn("failed to bulk add episodes", "content_id", contentID, "error", err)
		return
	}

	s.log.Info("synced episodes from TVDB",
		"content_id", contentID,
		"tvdb_id", tvdbID,
		"total", len(episodes),
		"inserted", inserted,
	)
}
