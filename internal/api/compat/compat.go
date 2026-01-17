// Package compat implements Radarr/Sonarr API compatibility for Overseerr.
package compat

import (
	"encoding/json"
	"net/http"
)

// Server provides Radarr/Sonarr API compatibility.
type Server struct {
	apiKey string
	// TODO: add dependencies
}

// New creates a new compatibility server.
func New(apiKey string) *Server {
	return &Server{apiKey: apiKey}
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
		apiKey := r.Header.Get("X-Api-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("apikey")
		}
		if apiKey != s.apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
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
	// TODO: translate request, call native API
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Not implemented"})
}

func (s *Server) listRootFolders(w http.ResponseWriter, r *http.Request) {
	// Return configured movie root
	writeJSON(w, http.StatusOK, []map[string]any{
		{"id": 1, "path": "/srv/data/media/movies", "freeSpace": 0},
	})
}

func (s *Server) listQualityProfiles(w http.ResponseWriter, r *http.Request) {
	// Return profiles in Radarr format
	writeJSON(w, http.StatusOK, []map[string]any{
		{"id": 1, "name": "hd"},
		{"id": 2, "name": "uhd"},
		{"id": 3, "name": "any"},
	})
}

func (s *Server) listQueue(w http.ResponseWriter, r *http.Request) {
	// TODO: translate from native downloads
	writeJSON(w, http.StatusOK, map[string]any{
		"page":          1,
		"pageSize":      20,
		"totalRecords":  0,
		"records":       []any{},
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
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Not implemented"})
}
