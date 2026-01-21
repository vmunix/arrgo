// Package v1 implements the native REST API.
package v1

import (
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
)

// Config holds API server configuration.
type Config struct {
	MovieRoot       string
	SeriesRoot      string
	DownloadRoot    string            // Root path for completed downloads (for tracked imports)
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
	mux.HandleFunc("POST /api/v1/search", s.requireSearcher(s.search))
	mux.HandleFunc("POST /api/v1/grab", s.requireManager(s.grab))

	// Downloads
	mux.HandleFunc("GET /api/v1/downloads", s.listDownloads)
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.getDownload)
	mux.HandleFunc("DELETE /api/v1/downloads/{id}", s.requireManager(s.deleteDownload))

	// History
	mux.HandleFunc("GET /api/v1/history", s.listHistory)

	// Files
	mux.HandleFunc("GET /api/v1/files", s.listFiles)
	mux.HandleFunc("DELETE /api/v1/files/{id}", s.deleteFile)

	// Library check
	mux.HandleFunc("GET /api/v1/library/check", s.checkLibrary)

	// System
	mux.HandleFunc("GET /api/v1/status", s.getStatus)
	mux.HandleFunc("GET /api/v1/dashboard", s.getDashboard)
	mux.HandleFunc("GET /api/v1/verify", s.verify)
	mux.HandleFunc("GET /api/v1/profiles", s.listProfiles)
	mux.HandleFunc("POST /api/v1/scan", s.requirePlex(s.triggerScan))

	// Plex (getPlexStatus handles nil gracefully, others require Plex)
	mux.HandleFunc("GET /api/v1/plex/status", s.getPlexStatus)
	mux.HandleFunc("POST /api/v1/plex/scan", s.requirePlex(s.scanPlexLibraries))
	mux.HandleFunc("GET /api/v1/plex/libraries/{name}/items", s.requirePlex(s.listPlexLibraryItems))
	mux.HandleFunc("GET /api/v1/plex/search", s.requirePlex(s.searchPlex))

	// Import
	mux.HandleFunc("POST /api/v1/import", s.requireImporter(s.importContent))
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

	resp := listContentResponse{
		Items:  make([]contentResponse, len(items)),
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	for i, c := range items {
		resp.Items[i] = contentToResponse(c)
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

	writeJSON(w, http.StatusOK, contentToResponse(c))
}

func contentToResponse(c *library.Content) contentResponse {
	return contentResponse{
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

	writeJSON(w, http.StatusCreated, contentToResponse(c))
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

	writeJSON(w, http.StatusOK, contentToResponse(c))
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
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	profile := req.Profile
	if profile == "" {
		profile = "hd"
	}

	q := search.Query{
		Text:    req.Query,
		Type:    req.Type,
		Season:  req.Season,
		Episode: req.Episode,
	}
	if req.ContentID != nil {
		q.ContentID = *req.ContentID
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

	// If event bus is available, emit event instead of direct call
	if s.deps.Bus != nil {
		if err := s.deps.Bus.Publish(r.Context(), &events.GrabRequested{
			BaseEvent:   events.NewBaseEvent(events.EventGrabRequested, events.EntityDownload, 0),
			ContentID:   req.ContentID,
			EpisodeID:   req.EpisodeID,
			DownloadURL: req.DownloadURL,
			ReleaseName: req.Title,
			Indexer:     req.Indexer,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}

	// Legacy: direct call to manager
	d, err := s.deps.Manager.Grab(r.Context(), req.ContentID, req.EpisodeID, req.DownloadURL, req.Title, req.Indexer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GRAB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, grabResponse{
		DownloadID: d.ID,
		Status:     string(d.Status),
	})
}

func (s *Server) listDownloads(w http.ResponseWriter, r *http.Request) {
	filter := download.Filter{}
	if activeStr := r.URL.Query().Get("active"); activeStr == "true" {
		filter.Active = true
	}

	downloads, err := s.deps.Downloads.List(filter)
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
		Items: make([]downloadResponse, len(downloads)),
		Total: len(downloads),
	}

	for i, d := range downloads {
		resp.Items[i] = downloadToResponse(d, liveStatus[d.ID])
	}

	writeJSON(w, http.StatusOK, resp)
}

func downloadToResponse(d *download.Download, live *download.ClientStatus) downloadResponse {
	resp := downloadResponse{
		ID:          d.ID,
		ContentID:   d.ContentID,
		EpisodeID:   d.EpisodeID,
		Client:      string(d.Client),
		ClientID:    d.ClientID,
		Status:      string(d.Status),
		ReleaseName: d.ReleaseName,
		Indexer:     d.Indexer,
		AddedAt:     d.AddedAt,
		CompletedAt: d.CompletedAt,
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

	deleteFiles := r.URL.Query().Get("delete_files") == "true"
	if err := s.deps.Manager.Cancel(r.Context(), id, deleteFiles); err != nil {
		writeError(w, http.StatusInternalServerError, "CANCEL_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	filter := importer.HistoryFilter{
		Limit: queryInt(r, "limit", 50),
	}

	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	entries, err := s.deps.History.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listHistoryResponse{
		Items: make([]historyResponse, len(entries)),
		Total: len(entries),
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
	filter := library.FileFilter{}
	if contentIDStr := r.URL.Query().Get("content_id"); contentIDStr != "" {
		id, _ := strconv.ParseInt(contentIDStr, 10, 64)
		filter.ContentID = &id
	}

	files, _, err := s.deps.Library.ListFiles(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listFilesResponse{
		Items: make([]fileResponse, len(files)),
		Total: len(files),
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

	// Download counts by status
	for _, status := range []download.Status{
		download.StatusQueued,
		download.StatusDownloading,
		download.StatusCompleted,
		download.StatusImporting,
		download.StatusImported,
		download.StatusCleaned,
		download.StatusFailed,
	} {
		st := status
		downloads, _ := s.deps.Downloads.List(download.Filter{Status: &st})
		switch status {
		case download.StatusQueued:
			resp.Downloads.Queued = len(downloads)
		case download.StatusDownloading:
			resp.Downloads.Downloading = len(downloads)
		case download.StatusCompleted:
			resp.Downloads.Completed = len(downloads)
		case download.StatusImporting:
			resp.Downloads.Importing = len(downloads)
		case download.StatusImported:
			resp.Downloads.Imported = len(downloads)
		case download.StatusCleaned:
			resp.Downloads.Cleaned = len(downloads)
		case download.StatusFailed:
			resp.Downloads.Failed = len(downloads)
		}
	}

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

func (s *Server) triggerScan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	if req.Path != "" {
		if err := s.deps.Plex.ScanPath(r.Context(), req.Path); err != nil {
			writeError(w, http.StatusInternalServerError, "SCAN_ERROR", err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "scan triggered"})
}

func (s *Server) getPlexStatus(w http.ResponseWriter, r *http.Request) {
	resp := plexStatusResponse{}

	if s.deps.Plex == nil {
		resp.Error = "Plex not configured"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	ctx := r.Context()

	// Get identity
	identity, err := s.deps.Plex.GetIdentity(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("connection failed: %v", err)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Connected = true
	resp.ServerName = identity.Name
	resp.Version = identity.Version

	// Get sections
	sections, err := s.deps.Plex.GetSections(ctx)
	if err != nil {
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

	// Call importer
	result, err := s.deps.Importer.Import(ctx, dl.ID, sourcePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "IMPORT_ERROR", err.Error())
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
