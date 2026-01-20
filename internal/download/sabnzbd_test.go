package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SABnzbd API mode constants for tests.
const (
	modeQueue   = "queue"
	modeHistory = "history"
)

// writeJSON is a helper that writes a JSON response, failing the test on error.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

func TestSABnzbdClient_Add(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "addurl", r.URL.Query().Get("mode"))
		assert.Equal(t, "test-key", r.URL.Query().Get("apikey"))
		assert.Equal(t, "json", r.URL.Query().Get("output"))
		assert.Equal(t, "http://example.com/test.nzb", r.URL.Query().Get("name"))
		assert.Equal(t, "movies", r.URL.Query().Get("cat"))

		resp := map[string]any{
			"status":  true,
			"nzo_ids": []string{"nzo_abc123"},
		}
		writeJSON(t, w, resp)
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	id, err := client.Add(context.Background(), "http://example.com/test.nzb", "movies")
	require.NoError(t, err)
	assert.Equal(t, "nzo_abc123", id)
}

func TestSABnzbdClient_Add_InvalidKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"status": false,
			"error":  "API Key Incorrect",
		}
		writeJSON(t, w, resp)
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "bad-key", "")
	_, err := client.Add(context.Background(), "http://example.com/test.nzb", "movies")
	require.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestSABnzbdClient_Add_Unavailable(t *testing.T) {
	// Use a closed server to simulate unavailability
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	_, err := client.Add(context.Background(), "http://example.com/test.nzb", "movies")
	require.ErrorIs(t, err, ErrClientUnavailable)
}

func TestSABnzbdClient_Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == modeQueue {
			resp := map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{
						{
							"nzo_id":     "nzo_abc123",
							"filename":   "Test.Movie.2024.1080p",
							"status":     "Downloading",
							"percentage": "45",
							"mb":         "1500",
							"mbleft":     "825",
							"timeleft":   "0:05:30",
							"speed":      "5.2 M",
						},
					},
				},
			}
			writeJSON(t, w, resp)
			return
		}

		// Should not reach history for this test
		t.Error("unexpected call to history")
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	status, err := client.Status(context.Background(), "nzo_abc123")
	require.NoError(t, err)
	assert.Equal(t, "nzo_abc123", status.ID)
	assert.Equal(t, "Test.Movie.2024.1080p", status.Name)
	assert.Equal(t, StatusDownloading, status.Status)
	assert.InDelta(t, 45, status.Progress, 0.001)
}

func TestSABnzbdClient_Status_Completed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == modeQueue {
			// Empty queue
			resp := map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{},
				},
			}
			writeJSON(t, w, resp)
			return
		}

		if mode == modeHistory {
			resp := map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{
						{
							"nzo_id":  "nzo_abc123",
							"name":    "Test.Movie.2024.1080p",
							"status":  "Completed",
							"bytes":   1572864000,
							"storage": "/downloads/complete/Test.Movie.2024.1080p",
						},
					},
				},
			}
			writeJSON(t, w, resp)
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	status, err := client.Status(context.Background(), "nzo_abc123")
	require.NoError(t, err)
	assert.Equal(t, "nzo_abc123", status.ID)
	assert.Equal(t, StatusCompleted, status.Status)
	assert.Equal(t, "/downloads/complete/Test.Movie.2024.1080p", status.Path)
}

func TestSABnzbdClient_Status_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == modeQueue {
			resp := map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{},
				},
			}
			writeJSON(t, w, resp)
			return
		}

		if mode == modeHistory {
			resp := map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{},
				},
			}
			writeJSON(t, w, resp)
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	_, err := client.Status(context.Background(), "nzo_notfound")
	require.ErrorIs(t, err, ErrDownloadNotFound)
}

func TestSABnzbdClient_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == modeQueue {
			resp := map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{
						{
							"nzo_id":     "nzo_queue1",
							"filename":   "Downloading.Movie.2024",
							"status":     "Downloading",
							"percentage": "50",
							"mb":         "2000",
							"mbleft":     "1000",
						},
						{
							"nzo_id":     "nzo_queue2",
							"filename":   "Queued.Movie.2024",
							"status":     "Queued",
							"percentage": "0",
							"mb":         "1500",
							"mbleft":     "1500",
						},
					},
				},
			}
			writeJSON(t, w, resp)
			return
		}

		if mode == modeHistory {
			resp := map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{
						{
							"nzo_id":  "nzo_done1",
							"name":    "Completed.Movie.2024",
							"status":  "Completed",
							"bytes":   1572864000,
							"storage": "/downloads/complete/Completed.Movie.2024",
						},
						{
							"nzo_id": "nzo_fail1",
							"name":   "Failed.Movie.2024",
							"status": "Failed",
							"bytes":  0,
						},
					},
				},
			}
			writeJSON(t, w, resp)
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	list, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 4)

	// Check queue items come first
	assert.Equal(t, "nzo_queue1", list[0].ID)
	assert.Equal(t, StatusDownloading, list[0].Status)
	assert.Equal(t, "nzo_queue2", list[1].ID)
	assert.Equal(t, StatusDownloading, list[1].Status)

	// Check history items
	assert.Equal(t, "nzo_done1", list[2].ID)
	assert.Equal(t, StatusCompleted, list[2].Status)
	assert.Equal(t, "nzo_fail1", list[3].ID)
	assert.Equal(t, StatusFailed, list[3].Status)
}

func TestSABnzbdClient_Remove(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "queue", r.URL.Query().Get("mode"))
		assert.Equal(t, "delete", r.URL.Query().Get("name"))
		assert.Equal(t, "nzo_abc123", r.URL.Query().Get("value"))

		resp := map[string]any{
			"status": true,
		}
		writeJSON(t, w, resp)
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	err := client.Remove(context.Background(), "nzo_abc123", false)
	require.NoError(t, err)
}
