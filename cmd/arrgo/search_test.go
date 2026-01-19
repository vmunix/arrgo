package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSearch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SearchResponse{
			Releases: []ReleaseResponse{
				{
					Title:       "The Matrix 1999 1080p BluRay x264",
					Indexer:     "NZBgeek",
					GUID:        "abc123",
					DownloadURL: "https://example.com/download/abc123",
					Size:        15000000000,
					PublishDate: "2024-01-15T10:30:00Z",
					Quality:     "1080p",
					Score:       850,
				},
				{
					Title:       "The Matrix 1999 2160p UHD BluRay x265",
					Indexer:     "DrunkenSlug",
					GUID:        "def456",
					DownloadURL: "https://example.com/download/def456",
					Size:        45000000000,
					PublishDate: "2024-01-14T08:00:00Z",
					Quality:     "2160p",
					Score:       950,
				},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Search("The Matrix 1999", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(resp.Releases))
	}
	if resp.Releases[0].Title != "The Matrix 1999 1080p BluRay x264" {
		t.Errorf("unexpected title: %s", resp.Releases[0].Title)
	}
	if resp.Releases[1].Score != 950 {
		t.Errorf("unexpected score: %d", resp.Releases[1].Score)
	}
}

func TestClientSearch_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SearchResponse{
			Releases: []ReleaseResponse{},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Search("Nonexistent Movie 2099", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Releases) != 0 {
		t.Errorf("expected empty releases, got %d", len(resp.Releases))
	}
}

func TestClientSearch_WithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SearchResponse{
			Releases: []ReleaseResponse{
				{
					Title:   "Some Result",
					Indexer: "NZBgeek",
				},
			},
			Errors: []string{
				"DrunkenSlug: connection timeout",
				"NZBfinder: rate limited",
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Search("query", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Releases) != 1 {
		t.Errorf("expected 1 release, got %d", len(resp.Releases))
	}
	if len(resp.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(resp.Errors))
	}
	if resp.Errors[0] != "DrunkenSlug: connection timeout" {
		t.Errorf("unexpected error message: %s", resp.Errors[0])
	}
}

func TestClientSearch_RequestBodyValidation(t *testing.T) {
	var receivedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SearchResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Search("The Matrix", "movie", "hd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["query"] != "The Matrix" {
		t.Errorf("query = %q, want %q", receivedBody["query"], "The Matrix")
	}
	if receivedBody["type"] != "movie" {
		t.Errorf("type = %q, want %q", receivedBody["type"], "movie")
	}
	if receivedBody["profile"] != "hd" {
		t.Errorf("profile = %q, want %q", receivedBody["profile"], "hd")
	}
}

func TestClientSearch_RequestBodyOmitsEmptyFields(t *testing.T) {
	var receivedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SearchResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Search("query only", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["query"] != "query only" {
		t.Errorf("query = %q, want %q", receivedBody["query"], "query only")
	}
	if _, exists := receivedBody["type"]; exists {
		t.Error("type should not be present when empty")
	}
	if _, exists := receivedBody["profile"]; exists {
		t.Error("profile should not be present when empty")
	}
}

func TestClientSearch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("search service unavailable"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Search("query", "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500: %v", err)
	}
}
