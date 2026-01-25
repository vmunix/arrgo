package main

import (
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientDownloads_WithItems(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/downloads").
		ExpectGET().
		RespondJSON(ListDownloadsResponse{
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
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Downloads(false)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "The.Matrix.1999.1080p.BluRay.x264", resp.Items[0].ReleaseName)
	assert.Equal(t, "downloading", resp.Items[0].Status)
	assert.Equal(t, "completed", resp.Items[1].Status)
}

func TestClientDownloads_EmptyQueue(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/downloads").
		ExpectGET().
		RespondJSON(ListDownloadsResponse{
			Items: []DownloadResponse{},
			Total: 0,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Downloads(false)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Items)
}

func TestClientDownloads_ActiveOnlyFilter(t *testing.T) {
	tests := []struct {
		name         string
		activeOnly   bool
		expectedPath string
	}{
		{
			name:         "with active filter",
			activeOnly:   true,
			expectedPath: "/api/v1/downloads?active=true",
		},
		{
			name:         "without active filter",
			activeOnly:   false,
			expectedPath: "/api/v1/downloads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedPath string
			srv := newMockServer(t).
				Handler(func(w http.ResponseWriter, r *http.Request) {
					receivedPath = r.URL.String()
					respondJSON(t, w, ListDownloadsResponse{Items: []DownloadResponse{}, Total: 0})
				}).
				Build()
			defer srv.Close()

			client := NewClient(srv.URL)
			_, err := client.Downloads(tt.activeOnly)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedPath, receivedPath)
		})
	}
}

func TestClientDownloads_ServerError(t *testing.T) {
	srv := newMockServer(t).
		RespondError(http.StatusInternalServerError, "database error").
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Downloads(false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "database error")
}

func TestClientDownloads_WithEpisodeID(t *testing.T) {
	episodeID := int64(42)
	completedAt := "2024-01-15T12:00:00Z"

	srv := newMockServer(t).
		ExpectPath("/api/v1/downloads").
		ExpectGET().
		RespondJSON(ListDownloadsResponse{
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
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Downloads(false)
	require.NoError(t, err)
	require.NotNil(t, resp.Items[0].EpisodeID, "expected EpisodeID to be set")
	assert.Equal(t, int64(42), *resp.Items[0].EpisodeID)
	require.NotNil(t, resp.Items[0].CompletedAt, "expected CompletedAt to be set")
	assert.Equal(t, completedAt, *resp.Items[0].CompletedAt)
}

func TestClientCancelDownload_Success(t *testing.T) {
	var receivedMethod, receivedPath string
	srv := newMockServer(t).
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.String()
			w.WriteHeader(http.StatusNoContent)
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.CancelDownload(42, false)
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, receivedMethod)
	assert.Equal(t, "/api/v1/downloads/42", receivedPath)
}

func TestClientCancelDownload_WithDeleteFiles(t *testing.T) {
	var receivedPath string
	srv := newMockServer(t).
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			w.WriteHeader(http.StatusNoContent)
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.CancelDownload(42, true)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/downloads/42?delete_files=true", receivedPath)
}

func TestClientCancelDownload_ServerError(t *testing.T) {
	srv := newMockServer(t).
		RespondError(http.StatusNotFound, `{"error":"download not found"}`).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.CancelDownload(999, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestQueueCancelCmd_Exists(t *testing.T) {
	// Verify the cancel subcommand exists on queueCmd
	found := false
	for _, cmd := range queueCmd.Commands() {
		if cmd.Use == "cancel <id>" {
			found = true
			// Check that --delete flag exists
			deleteFlag := cmd.Flags().Lookup("delete")
			assert.NotNil(t, deleteFlag, "cancel command should have --delete flag")
			break
		}
	}
	assert.True(t, found, "queueCmd should have 'cancel' subcommand")
}

func TestRunQueueCancel_Success(t *testing.T) {
	var receivedMethod, receivedPath string
	srv := newMockServer(t).
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.String()
			w.WriteHeader(http.StatusNoContent)
		}).
		Build()
	defer srv.Close()

	defer withServerURL(srv.URL)()

	err := runQueueCancel(nil, []string{"42"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, receivedMethod)
	assert.Equal(t, "/api/v1/downloads/42", receivedPath)
}

func TestRunQueueCancel_InvalidID(t *testing.T) {
	err := runQueueCancel(nil, []string{"notanumber"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ID")
}

func TestRunQueueCancel_WithDeleteFlag(t *testing.T) {
	var receivedPath string
	srv := newMockServer(t).
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			w.WriteHeader(http.StatusNoContent)
		}).
		Build()
	defer srv.Close()

	defer withServerURL(srv.URL)()

	// Create a command with the delete flag set
	cmd := &cobra.Command{}
	cmd.Flags().BoolP("delete", "d", false, "Delete files")
	_ = cmd.Flags().Set("delete", "true")

	err := runQueueCancel(cmd, []string{"42"})
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/downloads/42?delete_files=true", receivedPath)
}
