// Package v1 implements the native REST API.
package v1

import (
	"encoding/json"
	"net/http"
)

// Server is the v1 API server.
type Server struct {
	// TODO: add dependencies (library store, search, download, etc.)
}

// New creates a new v1 API server.
func New() *Server {
	return &Server{}
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
	json.NewEncoder(w).Encode(errorResponse{Error: message, Code: errCode})
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// Handlers (stubs)

func (s *Server) listContent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) getContent(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "Content not found")
}

func (s *Server) addContent(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) updateContent(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) updateEpisode(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) grab(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) listDownloads(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) getDownload(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
}

func (s *Server) deleteDownload(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
}

func (s *Server) listHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not yet implemented")
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
