package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// writeJSON is a helper that writes a JSON response, failing the test on error.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("failed to encode JSON response: %v", err)
	}
}

func TestSABnzbdClient_Add(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Query().Get("mode") != "addurl" {
			t.Errorf("expected mode=addurl, got %s", r.URL.Query().Get("mode"))
		}
		if r.URL.Query().Get("apikey") != "test-key" {
			t.Errorf("expected apikey=test-key, got %s", r.URL.Query().Get("apikey"))
		}
		if r.URL.Query().Get("output") != "json" {
			t.Errorf("expected output=json, got %s", r.URL.Query().Get("output"))
		}
		if r.URL.Query().Get("name") != "http://example.com/test.nzb" {
			t.Errorf("expected name=http://example.com/test.nzb, got %s", r.URL.Query().Get("name"))
		}
		if r.URL.Query().Get("cat") != "movies" {
			t.Errorf("expected cat=movies, got %s", r.URL.Query().Get("cat"))
		}

		resp := map[string]any{
			"status":  true,
			"nzo_ids": []string{"nzo_abc123"},
		}
		writeJSON(t, w, resp)
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	id, err := client.Add(context.Background(), "http://example.com/test.nzb", "movies")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id != "nzo_abc123" {
		t.Errorf("expected id=nzo_abc123, got %s", id)
	}
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
	if err != ErrInvalidAPIKey {
		t.Errorf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestSABnzbdClient_Add_Unavailable(t *testing.T) {
	// Use a closed server to simulate unavailability
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	_, err := client.Add(context.Background(), "http://example.com/test.nzb", "movies")
	if err != ErrClientUnavailable {
		t.Errorf("expected ErrClientUnavailable, got %v", err)
	}
}

func TestSABnzbdClient_Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == "queue" {
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
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status.ID != "nzo_abc123" {
		t.Errorf("expected ID=nzo_abc123, got %s", status.ID)
	}
	if status.Name != "Test.Movie.2024.1080p" {
		t.Errorf("expected Name=Test.Movie.2024.1080p, got %s", status.Name)
	}
	if status.Status != StatusDownloading {
		t.Errorf("expected Status=downloading, got %s", status.Status)
	}
	if status.Progress != 45 {
		t.Errorf("expected Progress=45, got %f", status.Progress)
	}
}

func TestSABnzbdClient_Status_Completed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == "queue" {
			// Empty queue
			resp := map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{},
				},
			}
			writeJSON(t, w, resp)
			return
		}

		if mode == "history" {
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
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status.ID != "nzo_abc123" {
		t.Errorf("expected ID=nzo_abc123, got %s", status.ID)
	}
	if status.Status != StatusCompleted {
		t.Errorf("expected Status=completed, got %s", status.Status)
	}
	if status.Path != "/downloads/complete/Test.Movie.2024.1080p" {
		t.Errorf("expected Path=/downloads/complete/Test.Movie.2024.1080p, got %s", status.Path)
	}
}

func TestSABnzbdClient_Status_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == "queue" {
			resp := map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{},
				},
			}
			writeJSON(t, w, resp)
			return
		}

		if mode == "history" {
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
	if err != ErrDownloadNotFound {
		t.Errorf("expected ErrDownloadNotFound, got %v", err)
	}
}

func TestSABnzbdClient_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == "queue" {
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

		if mode == "history" {
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
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 items, got %d", len(list))
	}

	// Check queue items come first
	if list[0].ID != "nzo_queue1" || list[0].Status != StatusDownloading {
		t.Errorf("first item should be nzo_queue1 with StatusDownloading, got %s/%s", list[0].ID, list[0].Status)
	}
	if list[1].ID != "nzo_queue2" || list[1].Status != StatusDownloading {
		t.Errorf("second item should be nzo_queue2 with StatusDownloading, got %s/%s", list[1].ID, list[1].Status)
	}

	// Check history items
	if list[2].ID != "nzo_done1" || list[2].Status != StatusCompleted {
		t.Errorf("third item should be nzo_done1 with StatusCompleted, got %s/%s", list[2].ID, list[2].Status)
	}
	if list[3].ID != "nzo_fail1" || list[3].Status != StatusFailed {
		t.Errorf("fourth item should be nzo_fail1 with StatusFailed, got %s/%s", list[3].ID, list[3].Status)
	}
}

func TestSABnzbdClient_Remove(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "queue" {
			t.Errorf("expected mode=queue, got %s", r.URL.Query().Get("mode"))
		}
		if r.URL.Query().Get("name") != "delete" {
			t.Errorf("expected name=delete, got %s", r.URL.Query().Get("name"))
		}
		if r.URL.Query().Get("value") != "nzo_abc123" {
			t.Errorf("expected value=nzo_abc123, got %s", r.URL.Query().Get("value"))
		}

		resp := map[string]any{
			"status": true,
		}
		writeJSON(t, w, resp)
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "")
	err := client.Remove(context.Background(), "nzo_abc123", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
