package v1

import "net/http"

// requireSearcher wraps a handler and returns 503 if searcher is not configured.
func (s *Server) requireSearcher(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Searcher == nil {
			writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Searcher not configured")
			return
		}
		next(w, r)
	}
}

// requireManager wraps a handler and returns 503 if download manager is not configured.
func (s *Server) requireManager(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Manager == nil {
			writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Download manager not configured")
			return
		}
		next(w, r)
	}
}

// requirePlex wraps a handler and returns 503 if Plex is not configured.
func (s *Server) requirePlex(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Plex == nil {
			writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Plex not configured")
			return
		}
		next(w, r)
	}
}

// requireImporter wraps a handler and returns 503 if importer is not configured.
func (s *Server) requireImporter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Importer == nil {
			writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Importer not configured")
			return
		}
		next(w, r)
	}
}
