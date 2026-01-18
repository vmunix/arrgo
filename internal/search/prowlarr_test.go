package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProwlarrClient_Search(t *testing.T) {
	mockReleases := []prowlarrRelease{
		{
			Title:       "Movie.2024.1080p.BluRay.x264-GROUP",
			GUID:        "abc123",
			Indexer:     "TestIndexer",
			DownloadURL: "https://example.com/download/abc123",
			Size:        8589934592,
			PublishDate: "2024-01-15T10:30:00Z",
		},
		{
			Title:       "Movie.2024.720p.WEB.x264-OTHER",
			GUID:        "def456",
			Indexer:     "TestIndexer",
			DownloadURL: "https://example.com/download/def456",
			Size:        4294967296,
			PublishDate: "2024-01-14T08:00:00Z",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockReleases); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "test-key")
	releases, err := client.Search(context.Background(), Query{Text: "Movie", Type: "movie"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	// Verify first release
	if releases[0].Title != "Movie.2024.1080p.BluRay.x264-GROUP" {
		t.Errorf("expected title 'Movie.2024.1080p.BluRay.x264-GROUP', got '%s'", releases[0].Title)
	}
	if releases[0].GUID != "abc123" {
		t.Errorf("expected GUID 'abc123', got '%s'", releases[0].GUID)
	}
	if releases[0].Indexer != "TestIndexer" {
		t.Errorf("expected indexer 'TestIndexer', got '%s'", releases[0].Indexer)
	}
	if releases[0].DownloadURL != "https://example.com/download/abc123" {
		t.Errorf("expected download URL 'https://example.com/download/abc123', got '%s'", releases[0].DownloadURL)
	}
	if releases[0].Size != 8589934592 {
		t.Errorf("expected size 8589934592, got %d", releases[0].Size)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	if !releases[0].PublishDate.Equal(expectedTime) {
		t.Errorf("expected publish date %v, got %v", expectedTime, releases[0].PublishDate)
	}
}

func TestProwlarrClient_Search_QueryParams(t *testing.T) {
	tests := []struct {
		name           string
		query          Query
		expectedParams map[string]string
	}{
		{
			name:  "movie text search",
			query: Query{Text: "Inception", Type: "movie"},
			expectedParams: map[string]string{
				"query":      "Inception",
				"categories": "2000",
			},
		},
		{
			name:  "series text search",
			query: Query{Text: "Breaking Bad", Type: "series"},
			expectedParams: map[string]string{
				"query":      "Breaking Bad",
				"categories": "5000",
			},
		},
		{
			name: "movie with tmdbId",
			query: Query{
				Type:   "movie",
				TMDBID: ptrInt64(12345),
			},
			expectedParams: map[string]string{
				"tmdbId":     "12345",
				"categories": "2000",
			},
		},
		{
			name: "series with tvdbId",
			query: Query{
				Type:   "series",
				TVDBID: ptrInt64(67890),
			},
			expectedParams: map[string]string{
				"tvdbId":     "67890",
				"categories": "5000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Key") != "test-key" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				// Verify query parameters
				for key, expectedValue := range tt.expectedParams {
					actualValue := r.URL.Query().Get(key)
					if actualValue != expectedValue {
						t.Errorf("expected param %s=%s, got %s", key, expectedValue, actualValue)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode([]prowlarrRelease{}); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			client := NewProwlarrClient(server.URL, "test-key")
			_, err := client.Search(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestProwlarrClient_Search_InvalidAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "wrong-key")
	_, err := client.Search(context.Background(), Query{Text: "Movie", Type: "movie"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Errorf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestProwlarrClient_Search_Unavailable(t *testing.T) {
	// Use a closed server to simulate connection refused
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	serverURL := server.URL
	server.Close() // Close immediately to cause connection refused

	client := NewProwlarrClient(serverURL, "test-key")
	_, err := client.Search(context.Background(), Query{Text: "Movie", Type: "movie"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrProwlarrUnavailable) {
		t.Errorf("expected ErrProwlarrUnavailable, got %v", err)
	}
}

func TestProwlarrClient_Search_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Search(ctx, Query{Text: "Movie", Type: "movie"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProwlarrClient_Search_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "test-key")
	_, err := client.Search(context.Background(), Query{Text: "Movie", Type: "movie"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProwlarrClient_Search_InvalidDateFormat(t *testing.T) {
	mockReleases := []prowlarrRelease{
		{
			Title:       "Movie.2024.1080p.BluRay.x264-GROUP",
			GUID:        "abc123",
			Indexer:     "TestIndexer",
			DownloadURL: "https://example.com/download/abc123",
			Size:        8589934592,
			PublishDate: "not-a-date",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockReleases); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "test-key")
	releases, err := client.Search(context.Background(), Query{Text: "Movie", Type: "movie"})

	// Should not fail, but the date should be zero
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if !releases[0].PublishDate.IsZero() {
		t.Errorf("expected zero time for invalid date, got %v", releases[0].PublishDate)
	}
}

func TestProwlarrClient_Search_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]prowlarrRelease{}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "test-key")
	releases, err := client.Search(context.Background(), Query{Text: "Movie", Type: "movie"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 0 {
		t.Errorf("expected 0 releases, got %d", len(releases))
	}
}

// Helper function to create pointer to int64
func ptrInt64(i int64) *int64 {
	return &i
}
