package v1

import (
	"net/http"
	"time"
)

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	// Validate pagination parameters
	if limit < 0 || offset < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "limit and offset must be non-negative")
		return
	}
	const maxLimit = 1000
	if limit > maxLimit {
		limit = maxLimit
	}

	if s.deps.EventLog == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_EVENT_LOG", "Event log not configured")
		return
	}

	events, total, err := s.deps.EventLog.Recent(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	resp := listEventsResponse{
		Items:  make([]EventResponse, len(events)),
		Total:  total,
		Limit:  limit,
		Offset: offset,
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
		Items:  make([]EventResponse, len(events)),
		Total:  len(events),
		Limit:  len(events),
		Offset: 0,
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
