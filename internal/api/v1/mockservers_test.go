//go:build integration

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vmunix/arrgo/internal/download"
)

// SABnzbdMock provides a configurable mock SABnzbd server with realistic behavior.
// It validates API keys, checks HTTP methods, and returns appropriate error responses.
type SABnzbdMock struct {
	t *testing.T

	// Configuration
	APIKey   string                 // Expected API key (empty = no validation)
	ClientID string                 // Client ID to return for addurl
	Status   *download.ClientStatus // Current download status

	// Tracking
	Requests []SABnzbdRequest // All requests received
}

// SABnzbdRequest captures details of a request to the mock server.
type SABnzbdRequest struct {
	Method string
	Mode   string
	APIKey string
}

// NewSABnzbdMock creates a new mock SABnzbd server.
func NewSABnzbdMock(t *testing.T) *SABnzbdMock {
	t.Helper()
	return &SABnzbdMock{
		t:        t,
		APIKey:   "test-api-key", // Default expected API key
		ClientID: "SABnzbd_nzo_test123",
	}
}

// WithAPIKey sets the expected API key.
func (m *SABnzbdMock) WithAPIKey(key string) *SABnzbdMock {
	m.APIKey = key
	return m
}

// WithClientID sets the client ID to return for addurl requests.
func (m *SABnzbdMock) WithClientID(id string) *SABnzbdMock {
	m.ClientID = id
	return m
}

// WithStatus sets the download status to return.
func (m *SABnzbdMock) WithStatus(status *download.ClientStatus) *SABnzbdMock {
	m.Status = status
	return m
}

// Build creates the httptest.Server.
func (m *SABnzbdMock) Build() *httptest.Server {
	m.t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		apiKey := r.URL.Query().Get("apikey")

		// Track request
		m.Requests = append(m.Requests, SABnzbdRequest{
			Method: r.Method,
			Mode:   mode,
			APIKey: apiKey,
		})

		// Check HTTP method - SABnzbd API uses GET
		if r.Method != http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": false,
				"error":  fmt.Sprintf("Method %s not allowed, use GET", r.Method),
			})
			return
		}

		// Validate API key
		if m.APIKey != "" && apiKey != m.APIKey {
			w.Header().Set("Content-Type", "application/json")
			// SABnzbd returns 200 with error in body, not 401
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": false,
				"error":  "API Key Incorrect",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch mode {
		case "addurl":
			m.handleAddURL(w)
		case "queue":
			m.handleQueue(w)
		case "history":
			m.handleHistory(w)
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": false,
				"error":  "Missing 'mode' parameter",
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": false,
				"error":  fmt.Sprintf("Unknown mode: %s", mode),
			})
		}
	}))
}

func (m *SABnzbdMock) handleAddURL(w http.ResponseWriter) {
	if m.ClientID == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": false,
			"error":  "Failed to add NZB",
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  true,
		"nzo_ids": []string{m.ClientID},
	})
}

func (m *SABnzbdMock) handleQueue(w http.ResponseWriter) {
	slots := []map[string]any{}
	if m.Status != nil && (m.Status.Status == download.StatusQueued || m.Status.Status == download.StatusDownloading) {
		slots = append(slots, map[string]any{
			"nzo_id":     m.Status.ID,
			"filename":   m.Status.Name,
			"status":     m.statusToSABString(m.Status.Status),
			"percentage": fmt.Sprintf("%.0f", m.Status.Progress),
			"mb":         fmt.Sprintf("%.0f", float64(m.Status.Size)/1024/1024),
			"mbleft":     fmt.Sprintf("%.0f", float64(m.Status.Size)*(100-m.Status.Progress)/100/1024/1024),
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"queue": map[string]any{"slots": slots},
	})
}

func (m *SABnzbdMock) handleHistory(w http.ResponseWriter) {
	slots := []map[string]any{}
	if m.Status != nil && (m.Status.Status == download.StatusCompleted || m.Status.Status == download.StatusFailed) {
		slots = append(slots, map[string]any{
			"nzo_id":  m.Status.ID,
			"name":    m.Status.Name,
			"status":  m.statusToSABString(m.Status.Status),
			"storage": m.Status.Path,
			"bytes":   m.Status.Size,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"history": map[string]any{"slots": slots},
	})
}

func (m *SABnzbdMock) statusToSABString(status download.Status) string {
	switch status {
	case download.StatusQueued:
		return "Queued"
	case download.StatusDownloading:
		return "Downloading"
	case download.StatusCompleted:
		return "Completed"
	case download.StatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// AssertAPIKeyChecked verifies that all requests had the correct API key.
func (m *SABnzbdMock) AssertAPIKeyChecked(t *testing.T) {
	t.Helper()
	for i, req := range m.Requests {
		if req.APIKey != m.APIKey {
			t.Errorf("request %d: expected API key %q, got %q", i, m.APIKey, req.APIKey)
		}
	}
}

// AssertAllGET verifies that all requests used GET method.
func (m *SABnzbdMock) AssertAllGET(t *testing.T) {
	t.Helper()
	for i, req := range m.Requests {
		if req.Method != http.MethodGet {
			t.Errorf("request %d: expected GET, got %s", i, req.Method)
		}
	}
}

// NewznabMock provides a configurable mock Newznab/indexer server.
type NewznabMock struct {
	t *testing.T

	// Configuration
	APIKey    string            // Expected API key
	Responses map[string][]byte // Query -> XML response
	Error     *NewznabError
}

// NewznabError represents an error response from the indexer.
type NewznabError struct {
	Code    int
	Message string
}

// NewNewznabMock creates a new mock Newznab server.
func NewNewznabMock(t *testing.T) *NewznabMock {
	t.Helper()
	return &NewznabMock{
		t:         t,
		APIKey:    "test-api-key",
		Responses: make(map[string][]byte),
	}
}

// WithAPIKey sets the expected API key.
func (m *NewznabMock) WithAPIKey(key string) *NewznabMock {
	m.APIKey = key
	return m
}

// WithError configures the mock to return an error.
func (m *NewznabMock) WithError(code int, message string) *NewznabMock {
	m.Error = &NewznabError{Code: code, Message: message}
	return m
}

// Build creates the httptest.Server.
func (m *NewznabMock) Build() *httptest.Server {
	m.t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check HTTP method - Newznab API uses GET
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Validate API key
		apiKey := r.URL.Query().Get("apikey")
		if m.APIKey != "" && apiKey != m.APIKey {
			w.WriteHeader(http.StatusUnauthorized)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<error code="100" description="Incorrect user credentials"/>`))
			return
		}

		// Return configured error
		if m.Error != nil {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<error code="%d" description="%s"/>`, m.Error.Code, m.Error.Message)))
			return
		}

		// Return empty success response by default
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel></channel></rss>`))
	}))
}
