// Package compat implements Radarr/Sonarr API compatibility for Overseerr.
package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/search"
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
	TMDBID           int64  `json:"tmdbId"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        bool   `json:"monitored"`
	AddOptions       struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// radarrMovieResponse is the Radarr format for a movie.
type radarrMovieResponse struct {
	ID        int64  `json:"id"`
	TMDBID    int64  `json:"tmdbId"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Monitored bool   `json:"monitored"`
	Status    string `json:"status"`
}

// sonarrAddRequest is the Sonarr format for adding a series.
type sonarrAddRequest struct {
	TVDBID           int64  `json:"tvdbId"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        bool   `json:"monitored"`
	AddOptions       struct {
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// sonarrSeriesResponse is the Sonarr format for a series.
type sonarrSeriesResponse struct {
	ID        int64  `json:"id"`
	TVDBID    int64  `json:"tvdbId"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Monitored bool   `json:"monitored"`
	Status    string `json:"status"`
}

// Server provides Radarr/Sonarr API compatibility.
type Server struct {
	cfg       Config
	library   *library.Store
	downloads *download.Store
	searcher  *search.Searcher
	manager   *download.Manager
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

// RegisterRoutes registers compatibility API routes.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Radarr compatibility
	mux.HandleFunc("GET /api/v3/movie", s.authMiddleware(s.listMovies))
	mux.HandleFunc("GET /api/v3/movie/{id}", s.authMiddleware(s.getMovie))
	mux.HandleFunc("POST /api/v3/movie", s.authMiddleware(s.addMovie))
	mux.HandleFunc("GET /api/v3/rootfolder", s.authMiddleware(s.listRootFolders))
	mux.HandleFunc("GET /api/v3/qualityprofile", s.authMiddleware(s.listQualityProfiles))
	mux.HandleFunc("GET /api/v3/queue", s.authMiddleware(s.listQueue))
	mux.HandleFunc("POST /api/v3/command", s.authMiddleware(s.executeCommand))

	// Sonarr compatibility
	mux.HandleFunc("GET /api/v3/series", s.authMiddleware(s.listSeries))
	mux.HandleFunc("GET /api/v3/series/{id}", s.authMiddleware(s.getSeries))
	mux.HandleFunc("POST /api/v3/series", s.authMiddleware(s.addSeries))
}

// authMiddleware validates the X-Api-Key header.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Require API key to be configured
		if s.cfg.APIKey == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "API key not configured"})
			return
		}
		apiKey := r.Header.Get("X-Api-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("apikey")
		}
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

// Radarr handlers

func (s *Server) listMovies(w http.ResponseWriter, r *http.Request) {
	// TODO: translate from native API
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) getMovie(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "Movie not found"})
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

	writeJSON(w, http.StatusCreated, radarrMovieResponse{
		ID:        content.ID,
		TMDBID:    req.TMDBID,
		Title:     req.Title,
		Year:      req.Year,
		Monitored: req.Monitored,
		Status:    "announced",
	})
}

func (s *Server) listRootFolders(w http.ResponseWriter, r *http.Request) {
	folders := []map[string]any{}

	if s.cfg.MovieRoot != "" {
		folders = append(folders, map[string]any{
			"id":        1,
			"path":      s.cfg.MovieRoot,
			"freeSpace": 0,
		})
	}
	if s.cfg.SeriesRoot != "" {
		folders = append(folders, map[string]any{
			"id":        2,
			"path":      s.cfg.SeriesRoot,
			"freeSpace": 0,
		})
	}

	writeJSON(w, http.StatusOK, folders)
}

func (s *Server) listQualityProfiles(w http.ResponseWriter, r *http.Request) {
	profiles := make([]map[string]any, 0, len(s.cfg.QualityProfiles))
	for name, id := range s.cfg.QualityProfiles {
		profiles = append(profiles, map[string]any{
			"id":   id,
			"name": name,
		})
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) listQueue(w http.ResponseWriter, r *http.Request) {
	downloads, err := s.downloads.List(download.DownloadFilter{Active: true})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	records := make([]map[string]any, 0, len(downloads))
	for _, dl := range downloads {
		records = append(records, map[string]any{
			"id":                    dl.ID,
			"movieId":               dl.ContentID,
			"title":                 dl.ReleaseName,
			"status":                string(dl.Status),
			"trackedDownloadStatus": "ok",
			"indexer":               dl.Indexer,
		})
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
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) getSeries(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "Series not found"})
}

func (s *Server) addSeries(w http.ResponseWriter, r *http.Request) {
	var req sonarrAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
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

	// Note: Series auto-search is more complex (episodes), skip for v1

	writeJSON(w, http.StatusCreated, sonarrSeriesResponse{
		ID:        content.ID,
		TVDBID:    req.TVDBID,
		Title:     req.Title,
		Year:      req.Year,
		Monitored: req.Monitored,
		Status:    "continuing",
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
