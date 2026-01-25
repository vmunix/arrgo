package v1

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if s.deps.EventLog == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_EVENT_LOG", "Event log not configured")
		return
	}

	events, err := s.deps.EventLog.Recent(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	resp := listEventsResponse{
		Items: make([]EventResponse, len(events)),
		Total: len(events),
	}
	for i, e := range events {
		resp.Items[i] = EventResponse{
			ID:         e.ID,
			EventType:  e.EventType,
			EntityType: e.EntityType,
			EntityID:   e.EntityID,
			OccurredAt: e.OccurredAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) listDownloadEvents(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if s.deps.EventLog == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_EVENT_LOG", "Event log not configured")
		return
	}

	// Verify download exists
	if _, err := s.deps.Downloads.Get(id); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
		return
	}

	events, err := s.deps.EventLog.ForEntity("download", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	resp := listEventsResponse{
		Items: make([]EventResponse, len(events)),
		Total: len(events),
	}
	for i, e := range events {
		resp.Items[i] = EventResponse{
			ID:         e.ID,
			EventType:  e.EventType,
			EntityType: e.EntityType,
			EntityID:   e.EntityID,
			OccurredAt: e.OccurredAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
