// Package v1 implements the native REST API.
package v1

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/importer"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/search"
)

// Config holds API server configuration.
type Config struct {
	MovieRoot       string
	SeriesRoot      string
	QualityProfiles map[string][]string
}

// Server is the v1 API server.
type Server struct {
	library   *library.Store
	downloads *download.Store
	manager   *download.Manager
	searcher  *search.Searcher
	history   *importer.HistoryStore
	plex      *importer.PlexClient
	cfg       Config
}

// New creates a new v1 API server.
func New(db *sql.DB, cfg Config) *Server {
	return &Server{
		library:   library.NewStore(db),
		downloads: download.NewStore(db),
		history:   importer.NewHistoryStore(db),
		cfg:       cfg,
	}
}

// SetSearcher configures the searcher (requires external Prowlarr client).
func (s *Server) SetSearcher(searcher *search.Searcher) {
	s.searcher = searcher
}

// SetManager configures the download manager (requires external SABnzbd client).
func (s *Server) SetManager(manager *download.Manager) {
	s.manager = manager
}

// SetPlex configures the Plex client for library scans.
func (s *Server) SetPlex(plex *importer.PlexClient) {
	s.plex = plex
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

	// Search & grab
	mux.HandleFunc("POST /api/v1/search", s.search)
	mux.HandleFunc("POST /api/v1/grab", s.grab)

	// Downloads
	mux.HandleFunc("GET /api/v1/downloads", s.listDownloads)
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.getDownload)
	mux.HandleFunc("DELETE /api/v1/downloads/{id}", s.deleteDownload)

	// History
	mux.HandleFunc("GET /api/v1/history", s.listHistory)

	// Files
	mux.HandleFunc("GET /api/v1/files", s.listFiles)
	mux.HandleFunc("DELETE /api/v1/files/{id}", s.deleteFile)

	// System
	mux.HandleFunc("GET /api/v1/status", s.getStatus)
	mux.HandleFunc("GET /api/v1/profiles", s.listProfiles)
	mux.HandleFunc("POST /api/v1/scan", s.triggerScan)
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
func pathID(r *http.Request, name string) (int64, error) {
	idStr := r.PathValue(name)
	if idStr == "" {
		return 0, fmt.Errorf("missing path parameter: %s", name)
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

	items, total, err := s.library.ListContent(filter)
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
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	c, err := s.library.GetContent(id)
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

	if err := s.library.AddContent(c); err != nil {
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
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var req updateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	c, err := s.library.GetContent(id)
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

	if err := s.library.UpdateContent(c); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, contentToResponse(c))
}

func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if err := s.library.DeleteContent(id); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request) {
	contentID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	filter := library.EpisodeFilter{ContentID: &contentID}
	episodes, total, err := s.library.ListEpisodes(filter)
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
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	var req updateEpisodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	ep, err := s.library.GetEpisode(id)
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

	if err := s.library.UpdateEpisode(ep); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, episodeToResponse(ep))
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	if s.searcher == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Searcher not configured")
		return
	}

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

	result, err := s.searcher.Search(r.Context(), q, profile)
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
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Download manager not configured")
		return
	}

	var req grabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	d, err := s.manager.Grab(r.Context(), req.ContentID, req.EpisodeID, req.DownloadURL, req.Title, req.Indexer)
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
	filter := download.DownloadFilter{}
	if activeStr := r.URL.Query().Get("active"); activeStr == "true" {
		filter.Active = true
	}

	downloads, err := s.downloads.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	resp := listDownloadsResponse{
		Items: make([]downloadResponse, len(downloads)),
		Total: len(downloads),
	}

	for i, d := range downloads {
		resp.Items[i] = downloadToResponse(d)
	}

	writeJSON(w, http.StatusOK, resp)
}

func downloadToResponse(d *download.Download) downloadResponse {
	return downloadResponse{
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
}

func (s *Server) getDownload(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	d, err := s.downloads.Get(id)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, downloadToResponse(d))
}

func (s *Server) deleteDownload(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Download manager not configured")
		return
	}

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	deleteFiles := r.URL.Query().Get("delete_files") == "true"
	if err := s.manager.Cancel(r.Context(), id, deleteFiles); err != nil {
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

	entries, err := s.history.List(filter)
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

	files, _, err := s.library.ListFiles(filter)
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
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if err := s.library.DeleteFile(id); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) triggerScan(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}
