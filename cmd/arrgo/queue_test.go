package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientDownloads_WithItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/downloads" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListDownloadsResponse{
			Items: []DownloadResponse{
				{
					ID:          1,
					ContentID:   100,
					Client:      "sabnzbd",
					ClientID:    "SABnzbd_nzo_abc123",
					Status:      "downloading",
					ReleaseName: "The.Matrix.1999.1080p.BluRay.x264",
					Indexer:     "NZBgeek",
					AddedAt:     "2024-01-15T10:30:00Z",
				},
				{
					ID:          2,
					ContentID:   101,
					Client:      "sabnzbd",
					ClientID:    "SABnzbd_nzo_def456",
					Status:      "completed",
					ReleaseName: "Inception.2010.2160p.UHD.BluRay.x265",
					Indexer:     "DrunkenSlug",
					AddedAt:     "2024-01-14T08:00:00Z",
				},
			},
			Total: 2,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Downloads(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].ReleaseName != "The.Matrix.1999.1080p.BluRay.x264" {
		t.Errorf("unexpected release name: %s", resp.Items[0].ReleaseName)
	}
	if resp.Items[0].Status != "downloading" {
		t.Errorf("unexpected status: %s", resp.Items[0].Status)
	}
	if resp.Items[1].Status != "completed" {
		t.Errorf("unexpected status: %s", resp.Items[1].Status)
	}
}

func TestClientDownloads_EmptyQueue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListDownloadsResponse{
			Items: []DownloadResponse{},
			Total: 0,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Downloads(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(resp.Items))
	}
}

func TestClientDownloads_ActiveOnlyFilter(t *testing.T) {
	var receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.String()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListDownloadsResponse{
			Items: []DownloadResponse{
				{
					ID:          1,
					Status:      "downloading",
					ReleaseName: "Active.Download",
				},
			},
			Total: 1,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Downloads(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPath != "/api/v1/downloads?active=true" {
		t.Errorf("path = %q, want %q", receivedPath, "/api/v1/downloads?active=true")
	}
}

func TestClientDownloads_NoActiveFilter(t *testing.T) {
	var receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.String()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListDownloadsResponse{
			Items: []DownloadResponse{},
			Total: 0,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Downloads(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPath != "/api/v1/downloads" {
		t.Errorf("path = %q, want %q", receivedPath, "/api/v1/downloads")
	}
}

func TestClientDownloads_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("database error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Downloads(false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500: %v", err)
	}
	if !strings.Contains(err.Error(), "database error") {
		t.Errorf("error should contain response body: %v", err)
	}
}

func TestClientDownloads_WithEpisodeID(t *testing.T) {
	episodeID := int64(42)
	completedAt := "2024-01-15T12:00:00Z"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListDownloadsResponse{
			Items: []DownloadResponse{
				{
					ID:          1,
					ContentID:   100,
					EpisodeID:   &episodeID,
					Client:      "sabnzbd",
					ClientID:    "SABnzbd_nzo_abc123",
					Status:      "completed",
					ReleaseName: "Breaking.Bad.S01E01.1080p.BluRay",
					Indexer:     "NZBgeek",
					AddedAt:     "2024-01-15T10:30:00Z",
					CompletedAt: &completedAt,
				},
			},
			Total: 1,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Downloads(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Items[0].EpisodeID == nil {
		t.Fatal("expected EpisodeID to be set")
	}
	if *resp.Items[0].EpisodeID != 42 {
		t.Errorf("EpisodeID = %d, want 42", *resp.Items[0].EpisodeID)
	}
	if resp.Items[0].CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}
	if *resp.Items[0].CompletedAt != completedAt {
		t.Errorf("CompletedAt = %q, want %q", *resp.Items[0].CompletedAt, completedAt)
	}
}
