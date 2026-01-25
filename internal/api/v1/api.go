// Package v1 implements the native REST API.
package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
	"github.com/vmunix/arrgo/pkg/release"
)

const queryTrue = "true"

// Config holds API server configuration.
type Config struct {
	MovieRoot       string
	SeriesRoot      string
	DownloadRoot    string // Root path for completed downloads (for tracked imports)
	QualityProfiles map[string][]string
}

// Server is the v1 API server.
type Server struct {
	deps ServerDeps
	cfg  Config
}

// NewWithDeps creates a new v1 API server with explicit dependencies.
// Required dependencies (Library, Downloads, History) must be non-nil.
// Optional dependencies (Searcher, Manager, Plex, Importer) may be nil.
func NewWithDeps(deps ServerDeps, cfg Config) (*Server, error) {
	if err := deps.Validate(); err != nil {
		return nil, err
	}
	return &Server{deps: deps, cfg: cfg}, nil
}

// New creates a new v1 API server with default stores from the database.
// This is a convenience constructor primarily for testing.
// For production use with optional dependencies, use NewWithDeps.
func New(db *sql.DB, cfg Config) *Server {
	deps := ServerDeps{
		Library:   library.NewStore(db),
		Downloads: download.NewStore(db),
		History:   importer.NewHistoryStore(db),
	}
	return &Server{deps: deps, cfg: cfg}
}

// RegisterRoutes registers API routes on the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Content
	mux.HandleFunc("GET /api/v1/content", s.listContent)
	mux.HandleFunc("GET /api/v1/content/{id}", s.getContent)
	mux.HandleFunc("POST /api/v1/content", s.addContent)
	mux.HandleFunc("PUT /api/v1/content/{id}", s.updateContent)
	mux.HandleFunc("DELETE /api/v1/content/{id}", s.deleteContent)

	// Episodes
	mux.HandleFunc("GET /api/v1/content/{id}/episodes", s.listEpisodes)
	mux.HandleFunc("PUT /api/v1/episodes/{id}", s.updateEpisode)

	// Search & grab (require optional dependencies)
	mux.HandleFunc("GET /api/v1/search", s.requireSearcher(s.search))
	mux.HandleFunc("POST /api/v1/grab", s.requireManager(s.grab))

	// Downloads
	mux.HandleFunc("GET /api/v1/downloads", s.listDownloads)
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.getDownload)
	mux.HandleFunc("GET /api/v1/downloads/{id}/events", s.listDownloadEvents)
	mux.HandleFunc("DELETE /api/v1/downloads/{id}", s.requireManager(s.deleteDownload))
	mux.HandleFunc("POST /api/v1/downloads/{id}/retry", s.requireManager(s.requireSearcher(s.retryDownload)))

	// History
	mux.HandleFunc("GET /api/v1/history", s.listHistory)

	// Events
	mux.HandleFunc("GET /api/v1/events", s.listEvents)

	// Files
	mux.HandleFunc("GET /api/v1/files", s.listFiles)
	mux.HandleFunc("DELETE /api/v1/files/{id}", s.deleteFile)

	// Library check - validates content records against actual files and Plex.
	// Note: There is no /library resource. "Library" represents the validated state
	// of content + files + Plex awareness, not a standalone entity. This endpoint
	// performs cross-system health checks rather than CRUD operations.
	mux.HandleFunc("GET /api/v1/library/check", s.checkLibrary)

	// System
	mux.HandleFunc("GET /api/v1/status", s.getStatus)
	mux.HandleFunc("GET /api/v1/dashboard", s.getDashboard)
	mux.HandleFunc("GET /api/v1/verify", s.verify)
	mux.HandleFunc("GET /api/v1/profiles", s.listProfiles)
	mux.HandleFunc("GET /api/v1/indexers", s.listIndexers)

	// Plex (getPlexStatus handles nil gracefully, others require Plex)
	mux.HandleFunc("GET /api/v1/plex/status", s.getPlexStatus)
	mux.HandleFunc("POST /api/v1/plex/scan", s.requirePlex(s.scanPlexLibraries))
	mux.HandleFunc("GET /api/v1/plex/libraries/{name}/items", s.requirePlex(s.listPlexLibraryItems))
	mux.HandleFunc("GET /api/v1/plex/search", s.requirePlex(s.searchPlex))

	// Import
	mux.HandleFunc("POST /api/v1/import", s.requireImporter(s.importContent))

	// Library import (from external sources like Plex)
	mux.HandleFunc("POST /api/v1/library/import", s.importLibrary)
}

// Error response
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeError(w http.ResponseWriter, code int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: message, Code: errCode})
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// pathID extracts an integer ID from the URL path.
func pathID(r *http.Request) (int64, error) {
	idStr := r.PathValue("id")
	if idStr == "" {
		return 0, fmt.Errorf("missing path parameter: id")
	}
	return strconv.ParseInt(idStr, 10, 64)
}

// queryInt extracts an optional integer from query string.
func queryInt(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// queryString extracts an optional string from query string.
func queryString(r *http.Request, name string) *string {
	val := r.URL.Query().Get(name)
	if val == "" {
		return nil
	}
	return &val
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Handlers (stubs)

func (s *Server) listContent(w http.ResponseWriter, r *http.Request) {
	// Parse filters
	filter := library.ContentFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}

	if typeStr := queryString(r, "type"); typeStr != nil {
		t := library.ContentType(*typeStr)
		filter.Type = &t
	}
	if statusStr := queryString(r, "status"); statusStr != nil {
		st := library.ContentStatus(*statusStr)
		filter.Status = &st
	}
	if title := queryString(r, "title"); title != nil {
		filter.Title = title
	}
	if yearStr := r.URL.Query().Get("year"); yearStr != "" {
		if year, err := strconv.Atoi(yearStr); err == nil {
			filter.Year = &year
		}
	}

	items, total, err := s.deps.Library.ListContent(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Collect series IDs to batch-fetch stats
	var seriesIDs []int64
	for _, c := range items {
		if c.Type == library.ContentTypeSeries {
			seriesIDs = append(seriesIDs, c.ID)
		}
	}

	// Fetch stats for all series in one query
	seriesStats, _ := s.deps.Library.GetSeriesStatsBatch(seriesIDs)

	resp := listContentResponse{
		Items:  make([]contentResponse, len(items)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, c := range items {
		resp.Items[i] = contentToResponse(c, seriesStats[c.ID])
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	c, err := s.deps.Library.GetContent(id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Fetch stats for series
	var stats *library.SeriesStats
	if c.Type == library.ContentTypeSeries {
		stats, _ = s.deps.Library.GetSeriesStats(c.ID)
	}

	writeJSON(w, http.StatusOK, contentToResponse(c, stats))
}

func contentToResponse(c *library.Content, stats *library.SeriesStats) contentResponse {
	resp := contentResponse{
		ID:             c.ID,
		Type:           string(c.Type),
		TMDBID:         c.TMDBID,
		TVDBID:         c.TVDBID,
		Title:          c.Title,
		Year:           c.Year,
		Status:         string(c.Status),
		QualityProfile: c.QualityProfile,
		RootPath:       c.RootPath,
		AddedAt:        c.AddedAt,
		UpdatedAt:      c.UpdatedAt,
	}

	// For series, compute status from episode stats and include stats in response
	if c.Type == library.ContentTypeSeries && stats != nil {
		resp.EpisodeStats = &episodeStatsResponse{
			TotalEpisodes:     stats.TotalEpisodes,
			AvailableEpisodes: stats.AvailableEpisodes,
			SeasonCount:       stats.SeasonCount,
		}

		// Compute display status based on episode availability
		switch {
		case stats.AvailableEpisodes == 0:
			resp.Status = "wanted"
		case stats.AvailableEpisodes < stats.TotalEpisodes:
			resp.Status = "partial"
		default:
			resp.Status = "available"
		}
	}

	return resp
}

func (s *Server) addContent(w http.ResponseWriter, r *http.Request) {
	var req addContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Validate type
	contentType := library.ContentType(req.Type)
	if contentType != library.ContentTypeMovie && contentType != library.ContentTypeSeries {
		writeError(w, http.StatusBadRequest, "INVALID_TYPE", "type must be 'movie' or 'series'")
		return
	}

	// Default root path based on type
	rootPath := req.RootPath
	if rootPath == "" {
		if contentType == library.ContentTypeMovie {
			rootPath = s.cfg.MovieRoot
		} else {
			rootPath = s.cfg.SeriesRoot
		}
	}

	c := &library.Content{
		Type:           contentType,
		TMDBID:         req.TMDBID,
		TVDBID:         req.TVDBID,
		Title:          req.Title,
		Year:           req.Year,
		Status:         library.StatusWanted,
		QualityProfile: req.QualityProfile,
		RootPath:       rootPath,
	}

	if err := s.deps.Library.AddContent(c); err != nil {
		if errors.Is(err, library.ErrDuplicate) {
			writeError(w, http.StatusConflict, "DUPLICATE", "Content already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Emit ContentAdded event
	if s.deps.Bus != nil {
		evt := &events.ContentAdded{
			BaseEvent:      events.NewBaseEvent(events.EventContentAdded, events.EntityContent, c.ID),
			ContentID:      c.ID,
			ContentType:    string(c.Type),
			Title:          c.Title,
			Year:           c.Year,
			QualityProfile: c.QualityProfile,
		}
		_ = s.deps.Bus.Publish(r.Context(), evt)
	}

	writeJSON(w, http.StatusCreated, contentToResponse(c, nil))
}

func (s *Server) updateContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var req updateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	c, err := s.deps.Library.GetContent(id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Capture old status for event
	oldStatus := c.Status

	// Apply updates
	if req.Status != nil {
		c.Status = library.ContentStatus(*req.Status)
	}
	if req.QualityProfile != nil {
		c.QualityProfile = *req.QualityProfile
	}

	if err := s.deps.Library.UpdateContent(c); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Emit ContentStatusChanged event if status changed
	if s.deps.Bus != nil && oldStatus != c.Status {
		evt := &events.ContentStatusChanged{
			BaseEvent: events.NewBaseEvent(events.EventContentStatusChanged, events.EntityContent, c.ID),
			ContentID: c.ID,
			OldStatus: string(oldStatus),
			NewStatus: string(c.Status),
		}
		_ = s.deps.Bus.Publish(r.Context(), evt)
	}

	// Fetch stats for series
	var stats *library.SeriesStats
	if c.Type == library.ContentTypeSeries {
		stats, _ = s.deps.Library.GetSeriesStats(c.ID)
	}

	writeJSON(w, http.StatusOK, contentToResponse(c, stats))
}

func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if err := s.deps.Library.DeleteContent(id); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request) {
	contentID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	filter := library.EpisodeFilter{ContentID: &contentID}
	episodes, total, err := s.deps.Library.ListEpisodes(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listEpisodesResponse{
		Items: make([]episodeResponse, len(episodes)),
		Total: total,
	}

	for i, ep := range episodes {
		resp.Items[i] = episodeToResponse(ep)
	}

	writeJSON(w, http.StatusOK, resp)
}

func episodeToResponse(ep *library.Episode) episodeResponse {
	return episodeResponse{
		ID:        ep.ID,
		ContentID: ep.ContentID,
		Season:    ep.Season,
		Episode:   ep.Episode,
		Title:     ep.Title,
		Status:    string(ep.Status),
		AirDate:   ep.AirDate,
	}
}

func (s *Server) updateEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var req updateEpisodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	ep, err := s.deps.Library.GetEpisode(id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Episode not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	if req.Status != nil {
		ep.Status = library.ContentStatus(*req.Status)
	}

	if err := s.deps.Library.UpdateEpisode(ep); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, episodeToResponse(ep))
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUERY", "query parameter is required")
		return
	}

	profile := r.URL.Query().Get("profile")
	if profile == "" {
		profile = "hd"
	}

	q := search.Query{
		Text: query,
		Type: r.URL.Query().Get("type"),
	}

	// Parse optional season/episode
	if seasonStr := r.URL.Query().Get("season"); seasonStr != "" {
		if season, err := strconv.Atoi(seasonStr); err == nil {
			q.Season = &season
		}
	}
	if episodeStr := r.URL.Query().Get("episode"); episodeStr != "" {
		if episode, err := strconv.Atoi(episodeStr); err == nil {
			q.Episode = &episode
		}
	}

	// Parse optional content_id
	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		if contentID, err := strconv.ParseInt(contentIDStr, 10, 64); err == nil {
			q.ContentID = contentID
		}
	}

	result, err := s.deps.Searcher.Search(r.Context(), q, profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SEARCH_ERROR", err.Error())
		return
	}

	resp := searchResponse{
		Releases: make([]releaseResponse, len(result.Releases)),
	}

	for i, rel := range result.Releases {
		quality := ""
		if rel.Quality != nil {
			quality = rel.Quality.Resolution.String()
		}
		resp.Releases[i] = releaseResponse{
			Title:       rel.Title,
			Indexer:     rel.Indexer,
			GUID:        rel.GUID,
			DownloadURL: rel.DownloadURL,
			Size:        rel.Size,
			PublishDate: rel.PublishDate,
			Quality:     quality,
			Score:       rel.Score,
		}
	}

	for _, e := range result.Errors {
		resp.Errors = append(resp.Errors, e.Error())
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) grab(w http.ResponseWriter, r *http.Request) {
	var req grabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Validate required fields
	if req.ContentID == 0 {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "content_id is required")
		return
	}
	if req.DownloadURL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "download_url is required")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "title is required")
		return
	}

	// Verify content exists and get type
	content, err := s.deps.Library.GetContent(req.ContentID)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Require event bus for grab operations
	if s.deps.Bus == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_DOWNLOAD_CLIENT", "download client not configured")
		return
	}

	// Build the event
	event := &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   req.ContentID,
		DownloadURL: req.DownloadURL,
		ReleaseName: req.Title,
		Indexer:     req.Indexer,
	}

	// For series, parse release name to detect episodes
	if content.Type == library.ContentTypeSeries {
		parsed := release.Parse(req.Title)

		// Use overrides if provided, otherwise use parsed values
		season := parsed.Season
		if req.Season != nil {
			season = *req.Season
		}

		episodes := parsed.Episodes
		if len(req.Episodes) > 0 {
			episodes = req.Episodes
		}

		// Season is required for series
		if season == 0 {
			writeError(w, http.StatusBadRequest, "INVALID_RELEASE", "cannot determine season from release title")
			return
		}

		event.Season = &season

		// Handle season packs vs specific episodes
		switch {
		case parsed.IsCompleteSeason && len(episodes) == 0:
			// Season pack: set IsCompleteSeason, no EpisodeIDs yet
			event.IsCompleteSeason = true
		case len(episodes) > 0:
			// Specific episodes: find or create episode records
			eps, err := s.deps.Library.FindOrCreateEpisodes(req.ContentID, season, episodes)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
				return
			}
			for _, ep := range eps {
				event.EpisodeIDs = append(event.EpisodeIDs, ep.ID)
			}
			// Backward compatibility: set EpisodeID for single-episode grabs
			if len(event.EpisodeIDs) == 1 {
				event.EpisodeID = &event.EpisodeIDs[0]
			}
		default:
			// No episode info and not a season pack
			writeError(w, http.StatusBadRequest, "INVALID_RELEASE", "cannot determine episodes from release title")
			return
		}
	} else {
		// For movies, preserve legacy EpisodeID if provided (shouldn't happen, but handle gracefully)
		event.EpisodeID = req.EpisodeID
	}

	if err := s.deps.Bus.Publish(r.Context(), event); err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) listDownloads(w http.ResponseWriter, r *http.Request) {
	filter := download.Filter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}

	// Validate pagination parameters
	if filter.Limit < 0 || filter.Offset < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "limit and offset must be non-negative")
		return
	}
	const maxLimit = 1000
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}

	if activeStr := r.URL.Query().Get("active"); activeStr == queryTrue {
		filter.Active = true
	}

	downloads, total, err := s.deps.Downloads.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Build a map of live status if manager is available
	liveStatus := make(map[int64]*download.ClientStatus)
	if s.deps.Manager != nil {
		active, err := s.deps.Manager.GetActive(r.Context())
		if err == nil {
			for _, a := range active {
				if a.Live != nil {
					liveStatus[a.Download.ID] = a.Live
				}
			}
		}
	}

	resp := listDownloadsResponse{
		Items:  make([]downloadResponse, len(downloads)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, d := range downloads {
		resp.Items[i] = downloadToResponse(d, liveStatus[d.ID])
	}

	writeJSON(w, http.StatusOK, resp)
}

func downloadToResponse(d *download.Download, live *download.ClientStatus) downloadResponse {
	resp := downloadResponse{
		ID:               d.ID,
		ContentID:        d.ContentID,
		EpisodeID:        d.EpisodeID,
		Season:           d.Season,
		IsCompleteSeason: d.IsCompleteSeason,
		Client:           string(d.Client),
		ClientID:         d.ClientID,
		Status:           string(d.Status),
		ReleaseName:      d.ReleaseName,
		Indexer:          d.Indexer,
		AddedAt:          d.AddedAt,
		CompletedAt:      d.CompletedAt,
	}
	if live != nil {
		resp.Progress = &live.Progress
		resp.Size = &live.Size
		resp.Speed = &live.Speed
		if live.ETA > 0 {
			eta := live.ETA.String()
			resp.ETA = &eta
		}
	}
	return resp
}

func (s *Server) getDownload(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	d, err := s.deps.Downloads.Get(id)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Get live status if manager is available
	var live *download.ClientStatus
	if s.deps.Manager != nil && d.Status == download.StatusDownloading {
		live, _ = s.deps.Manager.Client().Status(r.Context(), d.ClientID)
	}

	writeJSON(w, http.StatusOK, downloadToResponse(d, live))
}

func (s *Server) deleteDownload(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	deleteFiles := r.URL.Query().Get("delete_files") == queryTrue
	if err := s.deps.Manager.Cancel(r.Context(), id, deleteFiles); err != nil {
		writeError(w, http.StatusInternalServerError, "CANCEL_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) retryDownload(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	// Get the failed download
	dl, err := s.deps.Downloads.Get(id)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Only allow retry on failed downloads
	if dl.Status != download.StatusFailed {
		writeError(w, http.StatusBadRequest, "INVALID_STATE",
			fmt.Sprintf("Can only retry failed downloads, current status: %s", dl.Status))
		return
	}

	// Require event bus for retry operations (grabs go through event bus)
	if s.deps.Bus == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_EVENT_BUS", "event bus not configured")
		return
	}

	// Get content to search for
	content, err := s.deps.Library.GetContent(dl.ContentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONTENT_ERROR", err.Error())
		return
	}

	// Build search query
	query := content.Title
	if content.Year > 0 {
		query = fmt.Sprintf("%s %d", content.Title, content.Year)
	}

	// Search indexers
	q := search.Query{
		Text:      query,
		ContentID: dl.ContentID,
		Type:      string(content.Type),
	}
	profile := content.QualityProfile
	if profile == "" {
		profile = "hd"
	}

	result, err := s.deps.Searcher.Search(r.Context(), q, profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SEARCH_ERROR", err.Error())
		return
	}

	if len(result.Releases) == 0 {
		writeError(w, http.StatusNotFound, "NO_RESULTS", "No releases found")
		return
	}

	// Grab best result (first one, already sorted by score)
	best := result.Releases[0]

	// Publish grab request via event bus (same pattern as grab handler)
	if err := s.deps.Bus.Publish(r.Context(), &events.GrabRequested{
		BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
		ContentID:   dl.ContentID,
		EpisodeID:   dl.EpisodeID,
		DownloadURL: best.DownloadURL,
		ReleaseName: best.Title,
		Indexer:     best.Indexer,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, retryResponse{
		ReleaseName: best.Title,
		Message:     "Retry queued",
	})
}

func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	filter := importer.HistoryFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}

	// Validate pagination parameters
	if filter.Limit < 0 || filter.Offset < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "limit and offset must be non-negative")
		return
	}
	const maxLimit = 1000
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}

	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	entries, total, err := s.deps.History.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listHistoryResponse{
		Items:  make([]historyResponse, len(entries)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, h := range entries {
		resp.Items[i] = historyResponse{
			ID:        h.ID,
			ContentID: h.ContentID,
			EpisodeID: h.EpisodeID,
			Event:     h.Event,
			Data:      h.Data,
			CreatedAt: h.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	filter := library.FileFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}

	// Validate pagination parameters
	if filter.Limit < 0 || filter.Offset < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "limit and offset must be non-negative")
		return
	}
	const maxLimit = 1000
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}

	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	files, total, err := s.deps.Library.ListFiles(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listFilesResponse{
		Items:  make([]fileResponse, len(files)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, f := range files {
		resp.Items[i] = fileResponse{
			ID:        f.ID,
			ContentID: f.ContentID,
			EpisodeID: f.EpisodeID,
			Path:      f.Path,
			SizeBytes: f.SizeBytes,
			Quality:   f.Quality,
			Source:    f.Source,
			AddedAt:   f.AddedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if err := s.deps.Library.DeleteFile(id); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkLibrary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse filters
	filter := library.ContentFilter{
		Limit:  queryInt(r, "limit", 100),
		Offset: queryInt(r, "offset", 0),
	}

	if typeStr := queryString(r, "type"); typeStr != nil {
		t := library.ContentType(*typeStr)
		filter.Type = &t
	}
	if statusStr := queryString(r, "status"); statusStr != nil {
		st := library.ContentStatus(*statusStr)
		filter.Status = &st
	}

	// Get content
	contents, total, err := s.deps.Library.ListContent(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := libraryCheckResponse{
		Items: make([]libraryCheckItem, 0, len(contents)),
		Total: total,
	}

	for _, c := range contents {
		item := libraryCheckItem{
			ID:     c.ID,
			Type:   string(c.Type),
			Title:  c.Title,
			Year:   c.Year,
			Status: string(c.Status),
		}

		// Get files for this content
		files, _, err := s.deps.Library.ListFiles(library.FileFilter{ContentID: &c.ID})
		if err == nil {
			item.FileCount = len(files)
			allExist := true
			for _, f := range files {
				item.Files = append(item.Files, f.Path)
				if !fileExists(f.Path) {
					allExist = false
					item.FileMissing = append(item.FileMissing, f.Path)
					item.Issues = append(item.Issues, "File missing: "+f.Path)
				}
			}
			item.FileExists = allExist && len(files) > 0
		}

		// Check content status vs file presence
		if c.Status == library.StatusAvailable && len(files) == 0 {
			item.Issues = append(item.Issues, "Status is 'available' but no files in database")
		}
		if c.Status == library.StatusWanted && len(files) > 0 {
			item.Issues = append(item.Issues, "Status is 'wanted' but has files")
		}

		// Check Plex if available
		if s.deps.Plex != nil {
			results, err := s.deps.Plex.Search(ctx, c.Title)
			if err == nil {
				// Look for matching title + year
				for _, result := range results {
					if result.Year == c.Year && strings.EqualFold(result.Title, c.Title) {
						item.InPlex = true
						item.PlexTitle = result.Title
						break
					}
				}
				// Check for approximate match if exact not found
				if !item.InPlex {
					for _, result := range results {
						if result.Year == c.Year {
							item.InPlex = true
							item.PlexTitle = result.Title + " (year match)"
							break
						}
					}
				}
			}

			// Check Plex consistency
			if c.Status == library.StatusAvailable && !item.InPlex {
				item.Issues = append(item.Issues, "Status is 'available' but not found in Plex")
			}
		}

		if len(item.Issues) > 0 {
			resp.WithIssues++
		} else {
			resp.Healthy++
		}

		resp.Items = append(resp.Items, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Status:  "ok",
		Version: "0.1.0",
	})
}

func (s *Server) getDashboard(w http.ResponseWriter, _ *http.Request) {
	resp := DashboardResponse{
		Version: "0.1.0",
	}

	// Connection status
	resp.Connections.Server = true
	resp.Connections.Plex = s.deps.Plex != nil
	resp.Connections.SABnzbd = s.deps.Manager != nil

	// Download counts by status (single GROUP BY query)
	counts, _ := s.deps.Downloads.CountByStatus()
	resp.Downloads.Queued = counts[download.StatusQueued]
	resp.Downloads.Downloading = counts[download.StatusDownloading]
	resp.Downloads.Completed = counts[download.StatusCompleted]
	resp.Downloads.Importing = counts[download.StatusImporting]
	resp.Downloads.Imported = counts[download.StatusImported]
	resp.Downloads.Cleaned = counts[download.StatusCleaned]
	resp.Downloads.Failed = counts[download.StatusFailed]

	// Stuck count (>1hr in non-terminal state)
	resp.Stuck.Threshold = 60
	thresholds := map[download.Status]time.Duration{
		download.StatusQueued:      time.Hour,
		download.StatusDownloading: time.Hour,
		download.StatusCompleted:   time.Hour,
		download.StatusImporting:   time.Hour,
	}
	stuck, _ := s.deps.Downloads.ListStuck(thresholds)
	resp.Stuck.Count = len(stuck)

	// Library counts
	movieType := library.ContentTypeMovie
	seriesType := library.ContentTypeSeries
	movies, _, _ := s.deps.Library.ListContent(library.ContentFilter{Type: &movieType})
	series, _, _ := s.deps.Library.ListContent(library.ContentFilter{Type: &seriesType})
	resp.Library.Movies = len(movies)
	resp.Library.Series = len(series)

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles := make([]profileResponse, 0, len(s.cfg.QualityProfiles))
	for name, accept := range s.cfg.QualityProfiles {
		profiles = append(profiles, profileResponse{
			Name:   name,
			Accept: accept,
		})
	}

	writeJSON(w, http.StatusOK, listProfilesResponse{Profiles: profiles})
}

func (s *Server) listIndexers(w http.ResponseWriter, r *http.Request) {
	testConn := r.URL.Query().Get("test") == queryTrue
	ctx := r.Context()

	resp := listIndexersResponse{
		Indexers: make([]indexerResponse, len(s.deps.Indexers)),
	}

	for i, idx := range s.deps.Indexers {
		resp.Indexers[i] = indexerResponse{
			Name: idx.Name(),
			URL:  idx.URL(),
		}

		if testConn {
			start := time.Now()
			if err := idx.Caps(ctx); err != nil {
				resp.Indexers[i].Status = "error"
				resp.Indexers[i].Error = err.Error()
			} else {
				resp.Indexers[i].Status = "ok"
				resp.Indexers[i].ResponseMs = time.Since(start).Milliseconds()
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getPlexStatus(w http.ResponseWriter, r *http.Request) {
	resp := plexStatusResponse{}

	if s.deps.Plex == nil {
		resp.Error = "Plex not configured"
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}

	ctx := r.Context()

	// Get identity
	identity, err := s.deps.Plex.GetIdentity(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("connection failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}

	resp.Connected = true
	resp.ServerName = identity.Name
	resp.Version = identity.Version

	// Get sections
	sections, err := s.deps.Plex.GetSections(ctx)
	if err != nil {
		// Connected but partial failure - still return 200
		resp.Error = fmt.Sprintf("failed to get libraries: %v", err)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Libraries = make([]plexLibrary, len(sections))
	for i, sec := range sections {
		location := ""
		if len(sec.Locations) > 0 {
			location = sec.Locations[0].Path
		}

		count, _ := s.deps.Plex.GetLibraryCount(ctx, sec.Key)

		resp.Libraries[i] = plexLibrary{
			Key:        sec.Key,
			Title:      sec.Title,
			Type:       sec.Type,
			ItemCount:  count,
			Location:   location,
			ScannedAt:  sec.ScannedAt,
			Refreshing: sec.Refreshing(),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) scanPlexLibraries(w http.ResponseWriter, r *http.Request) {
	var req plexScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	ctx := r.Context()

	// Get all sections
	sections, err := s.deps.Plex.GetSections(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	// Determine which libraries to scan
	var toScan []struct {
		name string
		key  string
	}
	if len(req.Libraries) == 0 {
		// Scan all
		for _, sec := range sections {
			toScan = append(toScan, struct {
				name string
				key  string
			}{sec.Title, sec.Key})
		}
	} else {
		// Validate and find requested libraries (case-insensitive)
		for _, name := range req.Libraries {
			var found bool
			for _, sec := range sections {
				if strings.EqualFold(sec.Title, name) {
					toScan = append(toScan, struct {
						name string
						key  string
					}{sec.Title, sec.Key})
					found = true
					break
				}
			}
			if !found {
				var available []string
				for _, sec := range sections {
					available = append(available, sec.Title)
				}
				writeError(w, http.StatusBadRequest, "LIBRARY_NOT_FOUND",
					fmt.Sprintf("library %q not found, available: %v", name, available))
				return
			}
		}
	}

	// Trigger scans
	scanned := make([]string, 0, len(toScan))
	for _, lib := range toScan {
		if err := s.deps.Plex.RefreshLibrary(ctx, lib.key); err != nil {
			writeError(w, http.StatusInternalServerError, "SCAN_ERROR",
				fmt.Sprintf("failed to scan %q: %v", lib.name, err))
			return
		}
		scanned = append(scanned, lib.name)
	}

	writeJSON(w, http.StatusOK, plexScanResponse{Scanned: scanned})
}

func (s *Server) listPlexLibraryItems(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	// Find section (case-insensitive)
	section, err := s.deps.Plex.FindSectionByName(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}
	if section == nil {
		sections, _ := s.deps.Plex.GetSections(ctx)
		var available []string
		for _, sec := range sections {
			available = append(available, sec.Title)
		}
		writeError(w, http.StatusNotFound, "LIBRARY_NOT_FOUND",
			fmt.Sprintf("library %q not found, available: %v", name, available))
		return
	}

	// Get items
	items, err := s.deps.Plex.ListLibraryItems(ctx, section.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	// Build response with tracking status
	resp := plexListResponse{
		Library: section.Title,
		Items:   make([]plexItemResponse, len(items)),
		Total:   len(items),
	}

	for i, item := range items {
		resp.Items[i] = plexItemResponse{
			Title:    item.Title,
			Year:     item.Year,
			Type:     item.Type,
			AddedAt:  item.AddedAt,
			FilePath: item.FilePath,
		}

		// Check if tracked in arrgo
		content, _ := s.deps.Library.GetByTitleYear(item.Title, item.Year)
		if content != nil {
			resp.Items[i].Tracked = true
			resp.Items[i].ContentID = &content.ID
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) searchPlex(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUERY", "query parameter is required")
		return
	}

	ctx := r.Context()

	items, err := s.deps.Plex.Search(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	resp := plexSearchResponse{
		Query: query,
		Items: make([]plexItemResponse, len(items)),
		Total: len(items),
	}

	for i, item := range items {
		resp.Items[i] = plexItemResponse{
			Title:    item.Title,
			Year:     item.Year,
			Type:     item.Type,
			AddedAt:  item.AddedAt,
			FilePath: item.FilePath,
		}

		// Check if tracked in arrgo
		content, _ := s.deps.Library.GetByTitleYear(item.Title, item.Year)
		if content != nil {
			resp.Items[i].Tracked = true
			resp.Items[i].ContentID = &content.ID
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) importContent(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Route to appropriate handler based on mode
	if req.DownloadID != nil {
		s.importTracked(w, r, req)
	} else {
		s.importManual(w, r, req)
	}
}

// importTracked handles import of a tracked download by ID.
func (s *Server) importTracked(w http.ResponseWriter, r *http.Request, req importRequest) {
	ctx := r.Context()

	// Get download from store
	dl, err := s.deps.Downloads.Get(*req.DownloadID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "download not found")
		return
	}

	// Verify download is in importable state (completed)
	if dl.Status != download.StatusCompleted {
		writeError(w, http.StatusBadRequest, "INVALID_STATE",
			fmt.Sprintf("download must be in 'completed' status, currently '%s'", dl.Status))
		return
	}

	// Construct source path from download root + release name
	if s.cfg.DownloadRoot == "" {
		writeError(w, http.StatusInternalServerError, "CONFIG_ERROR", "download_root not configured")
		return
	}
	sourcePath := filepath.Join(s.cfg.DownloadRoot, dl.ReleaseName)

	// Verify path exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "PATH_NOT_FOUND",
			fmt.Sprintf("source path not found: %s", sourcePath))
		return
	}

	// Get content for response
	content, err := s.deps.Library.GetContent(dl.ContentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Transition to importing status
	if err := s.deps.Downloads.Transition(dl, download.StatusImporting); err != nil {
		writeError(w, http.StatusInternalServerError, "TRANSITION_ERROR", err.Error())
		return
	}

	// Call appropriate importer method based on download type
	if dl.IsCompleteSeason {
		// Season pack import
		packResult, err := s.deps.Importer.ImportSeasonPack(ctx, dl.ID, sourcePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "IMPORT_ERROR", err.Error())
			return
		}

		// Transition to imported status
		if err := s.deps.Downloads.Transition(dl, download.StatusImported); err != nil {
			writeError(w, http.StatusInternalServerError, "TRANSITION_ERROR", err.Error())
			return
		}

		// Publish ImportCompleted event for event-driven pipeline
		if s.deps.Bus != nil {
			episodeResults := make([]events.EpisodeImportResult, 0, len(packResult.Episodes))
			for _, ep := range packResult.Episodes {
				var errStr string
				if ep.Error != nil {
					errStr = ep.Error.Error()
				}
				episodeResults = append(episodeResults, events.EpisodeImportResult{
					EpisodeID: ep.EpisodeID,
					Season:    ep.Season,
					Episode:   ep.Episode,
					Success:   ep.Error == nil,
					FilePath:  ep.FilePath,
					Error:     errStr,
				})
			}
			evt := &events.ImportCompleted{
				BaseEvent:      events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
				DownloadID:     dl.ID,
				ContentID:      dl.ContentID,
				FileSize:       packResult.TotalSize,
				EpisodeResults: episodeResults,
			}
			_ = s.deps.Bus.Publish(ctx, evt)
		}

		writeJSON(w, http.StatusOK, importResponse{
			ContentID:    content.ID,
			SourcePath:   sourcePath,
			SizeBytes:    packResult.TotalSize,
			PlexNotified: packResult.PlexNotified,
			EpisodeCount: len(packResult.Episodes),
		})
		return
	}

	// Single file import
	result, err := s.deps.Importer.Import(ctx, dl.ID, sourcePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMPORT_ERROR", err.Error())
		return
	}

	// Transition to imported status
	if err := s.deps.Downloads.Transition(dl, download.StatusImported); err != nil {
		writeError(w, http.StatusInternalServerError, "TRANSITION_ERROR", err.Error())
		return
	}

	// Publish ImportCompleted event for event-driven pipeline
	if s.deps.Bus != nil {
		evt := &events.ImportCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
			DownloadID: dl.ID,
			ContentID:  dl.ContentID,
			EpisodeID:  dl.EpisodeID,
			FilePath:   result.DestPath,
			FileSize:   result.SizeBytes,
		}
		_ = s.deps.Bus.Publish(ctx, evt)
	}

	writeJSON(w, http.StatusOK, importResponse{
		FileID:       result.FileID,
		ContentID:    content.ID,
		SourcePath:   result.SourcePath,
		DestPath:     result.DestPath,
		SizeBytes:    result.SizeBytes,
		PlexNotified: result.PlexNotified,
	})
}

// importManual handles manual file import with metadata.
func (s *Server) importManual(w http.ResponseWriter, r *http.Request, req importRequest) {
	ctx := r.Context()

	// Validate required fields
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "path is required")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "title is required")
		return
	}
	if req.Year == 0 {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "year is required")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", "type is required")
		return
	}

	// Validate type
	contentType := library.ContentType(req.Type)
	if contentType != library.ContentTypeMovie && contentType != library.ContentTypeSeries {
		writeError(w, http.StatusBadRequest, "INVALID_TYPE", "type must be 'movie' or 'series'")
		return
	}

	// Validate path is absolute
	if !filepath.IsAbs(req.Path) {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "path must be absolute")
		return
	}

	// Find or create content record
	// Note: GetByTitleYear returns nil, nil when not found
	content, err := s.deps.Library.GetByTitleYear(req.Title, req.Year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if content == nil {
		// Create new content
		rootPath := s.cfg.MovieRoot
		if contentType == library.ContentTypeSeries {
			rootPath = s.cfg.SeriesRoot
		}

		content = &library.Content{
			Type:           contentType,
			Title:          req.Title,
			Year:           req.Year,
			Status:         library.StatusWanted,
			QualityProfile: "hd",
			RootPath:       rootPath,
		}
		if err := s.deps.Library.AddContent(content); err != nil {
			writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
			return
		}
	}

	// For series, we need season and episode
	var episodeID *int64
	if contentType == library.ContentTypeSeries {
		if req.Season == nil || req.Episode == nil {
			writeError(w, http.StatusBadRequest, "MISSING_FIELD", "season and episode are required for series")
			return
		}

		// Find or create episode
		ep, err := s.findOrCreateEpisode(content.ID, *req.Season, *req.Episode)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
			return
		}
		episodeID = &ep.ID
	}

	// Build release name for audit trail
	releaseName := req.Title
	if req.Quality != "" {
		releaseName = req.Title + " " + req.Quality
	}

	// Create download record for audit trail
	now := time.Now()
	dl := &download.Download{
		ContentID:   content.ID,
		EpisodeID:   episodeID,
		Client:      download.ClientManual,
		ClientID:    fmt.Sprintf("manual-%d", now.UnixNano()),
		Status:      download.StatusCompleted,
		ReleaseName: releaseName,
		Indexer:     "manual",
		CompletedAt: &now,
	}
	if err := s.deps.Downloads.Add(dl); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Call importer
	result, err := s.deps.Importer.Import(ctx, dl.ID, req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMPORT_ERROR", err.Error())
		return
	}

	// Publish ImportCompleted event for event-driven pipeline (cleanup, etc.)
	if s.deps.Bus != nil {
		evt := &events.ImportCompleted{
			BaseEvent:  events.NewBaseEvent(events.EventImportCompleted, events.EntityDownload, dl.ID),
			DownloadID: dl.ID,
			ContentID:  content.ID,
			EpisodeID:  episodeID,
			FilePath:   result.DestPath,
			FileSize:   result.SizeBytes,
		}
		// Best effort - don't fail the request if event publishing fails
		_ = s.deps.Bus.Publish(ctx, evt)
	}

	writeJSON(w, http.StatusOK, importResponse{
		FileID:       result.FileID,
		ContentID:    content.ID,
		SourcePath:   result.SourcePath,
		DestPath:     result.DestPath,
		SizeBytes:    result.SizeBytes,
		PlexNotified: result.PlexNotified,
	})
}

// findOrCreateEpisode finds an episode by content ID, season, and episode number,
// creating it if it doesn't exist.
func (s *Server) findOrCreateEpisode(contentID int64, season, episode int) (*library.Episode, error) {
	// List episodes for this content/season
	filter := library.EpisodeFilter{
		ContentID: &contentID,
		Season:    &season,
	}
	episodes, _, err := s.deps.Library.ListEpisodes(filter)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}

	// Find matching episode number
	for _, ep := range episodes {
		if ep.Episode == episode {
			return ep, nil
		}
	}

	// Not found, create it
	ep := &library.Episode{
		ContentID: contentID,
		Season:    season,
		Episode:   episode,
		Status:    library.StatusWanted,
	}
	if err := s.deps.Library.AddEpisode(ep); err != nil {
		return nil, fmt.Errorf("add episode: %w", err)
	}

	return ep, nil
}

// importLibrary handles POST /api/v1/library/import for importing content from external sources.
func (s *Server) importLibrary(w http.ResponseWriter, r *http.Request) {
	var req libraryImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// Validate source
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "MISSING_SOURCE", "source is required")
		return
	}
	if req.Source != "plex" {
		writeError(w, http.StatusBadRequest, "INVALID_SOURCE", "unsupported source: "+req.Source)
		return
	}

	// Validate library
	if req.Library == "" {
		writeError(w, http.StatusBadRequest, "MISSING_LIBRARY", "library is required")
		return
	}

	// Check Plex is configured
	if s.deps.Plex == nil {
		writeError(w, http.StatusServiceUnavailable, "PLEX_NOT_CONFIGURED", "Plex not configured")
		return
	}

	// Find the library section
	section, err := s.deps.Plex.FindSectionByName(r.Context(), req.Library)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}
	if section == nil {
		writeError(w, http.StatusNotFound, "LIBRARY_NOT_FOUND", "Plex library not found: "+req.Library)
		return
	}

	// Get all items from library
	items, err := s.deps.Plex.ListLibraryItems(r.Context(), section.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	// Process items (stub for now - will be implemented in Task 5)
	resp := s.processPlexImport(r.Context(), items, req.QualityOverride, req.DryRun)
	writeJSON(w, http.StatusOK, resp)
}

// processPlexImport processes Plex items for import.
func (s *Server) processPlexImport(ctx context.Context, items []importer.PlexItem, qualityOverride string, dryRun bool) libraryImportResponse {
	resp := libraryImportResponse{
		Imported: []libraryImportItem{},
		Skipped:  []libraryImportItem{},
		Errors:   []libraryImportItem{},
	}

	for _, item := range items {
		// Map Plex type to our type
		contentType := library.ContentTypeMovie
		if item.Type == "show" {
			contentType = library.ContentTypeSeries
		}

		// Check if already tracked
		title := item.Title
		year := item.Year
		existing, _, _ := s.deps.Library.ListContent(library.ContentFilter{
			Type:  &contentType,
			Title: &title,
			Year:  &year,
			Limit: 1,
		})
		if len(existing) > 0 {
			resp.Skipped = append(resp.Skipped, libraryImportItem{
				Title:     item.Title,
				Year:      item.Year,
				Type:      string(contentType),
				ContentID: existing[0].ID,
				Reason:    "already tracked",
			})
			continue
		}

		// Parse quality from filename
		quality := "hd" // default
		if item.FilePath != "" {
			// Translate Plex path to local path for parsing
			localPath := item.FilePath
			if s.deps.Plex != nil {
				localPath = s.deps.Plex.TranslateToLocal(item.FilePath)
			}
			parsed := release.Parse(filepath.Base(localPath))
			quality = mapResolutionToProfile(parsed.Resolution)
		}
		if qualityOverride != "" {
			quality = qualityOverride
		}

		importItem := libraryImportItem{
			Title:   item.Title,
			Year:    item.Year,
			Type:    string(contentType),
			Quality: quality,
		}

		if !dryRun {
			// Create content and file records (will be implemented in Task 6)
			contentID, err := s.createImportedContent(ctx, item, contentType, quality)
			if err != nil {
				importItem.Error = err.Error()
				resp.Errors = append(resp.Errors, importItem)
				continue
			}
			importItem.ContentID = contentID
		}

		resp.Imported = append(resp.Imported, importItem)
	}

	resp.Summary.Imported = len(resp.Imported)
	resp.Summary.Skipped = len(resp.Skipped)
	resp.Summary.Errors = len(resp.Errors)

	return resp
}

// mapResolutionToProfile maps a resolution string to a quality profile name.
func mapResolutionToProfile(resolution release.Resolution) string {
	switch resolution {
	case release.Resolution2160p:
		return "uhd"
	case release.Resolution1080p:
		return "hd"
	case release.Resolution720p:
		return "hd720"
	default:
		return "hd"
	}
}

// createImportedContent creates content and file records for an imported item.
func (s *Server) createImportedContent(_ context.Context, item importer.PlexItem, contentType library.ContentType, qualityProfile string) (int64, error) {
	// Translate Plex path to local path
	localPath := item.FilePath
	if s.deps.Plex != nil {
		localPath = s.deps.Plex.TranslateToLocal(item.FilePath)
	}

	// Stat file to get size (for movies only)
	var fileSize int64
	if contentType == library.ContentTypeMovie && localPath != "" {
		info, err := os.Stat(localPath)
		if err != nil {
			return 0, fmt.Errorf("cannot access file: %w", err)
		}
		fileSize = info.Size()
	}

	// Derive root path from file path (go up two directories: /movies/Title (Year)/file.mkv -> /movies)
	rootPath := ""
	if localPath != "" {
		rootPath = filepath.Dir(filepath.Dir(localPath))
	}

	// Create content record
	content := &library.Content{
		Type:           contentType,
		Title:          item.Title,
		Year:           item.Year,
		Status:         library.StatusAvailable,
		QualityProfile: qualityProfile,
		RootPath:       rootPath,
	}

	if err := s.deps.Library.AddContent(content); err != nil {
		return 0, fmt.Errorf("create content: %w", err)
	}

	// Create file record (for movies only - series don't have single file)
	if contentType == library.ContentTypeMovie && localPath != "" {
		parsed := release.Parse(filepath.Base(localPath))
		file := &library.File{
			ContentID: content.ID,
			Path:      localPath,
			SizeBytes: fileSize,
			Quality:   parsed.Resolution.String(),
			Source:    "plex-import",
		}
		if err := s.deps.Library.AddFile(file); err != nil {
			// Best effort - content was created successfully
			// In production, this could be logged but we don't fail the operation
			_ = err
		}
	}

	return content.ID, nil
}
