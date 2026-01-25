package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Dashboard_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/dashboard").
		ExpectGET().
		RespondJSON(DashboardResponse{
			Version: "1.2.3",
			Connections: struct {
				Server  bool `json:"server"`
				Plex    bool `json:"plex"`
				SABnzbd bool `json:"sabnzbd"`
			}{
				Server:  true,
				Plex:    true,
				SABnzbd: true,
			},
			Downloads: struct {
				Queued      int `json:"queued"`
				Downloading int `json:"downloading"`
				Completed   int `json:"completed"`
				Importing   int `json:"importing"`
				Imported    int `json:"imported"`
				Cleaned     int `json:"cleaned"`
				Failed      int `json:"failed"`
			}{
				Queued:      2,
				Downloading: 1,
				Completed:   3,
				Importing:   0,
				Imported:    10,
				Cleaned:     5,
				Failed:      1,
			},
			Stuck: struct {
				Count     int   `json:"count"`
				Threshold int64 `json:"threshold_minutes"`
			}{
				Count:     1,
				Threshold: 60,
			},
			Library: struct {
				Movies int `json:"movies"`
				Series int `json:"series"`
			}{
				Movies: 150,
				Series: 25,
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Dashboard()
	require.NoError(t, err)

	// Verify version
	assert.Equal(t, "1.2.3", resp.Version)

	// Verify connections
	assert.True(t, resp.Connections.Server)
	assert.True(t, resp.Connections.Plex)
	assert.True(t, resp.Connections.SABnzbd)

	// Verify downloads
	assert.Equal(t, 2, resp.Downloads.Queued)
	assert.Equal(t, 1, resp.Downloads.Downloading)
	assert.Equal(t, 3, resp.Downloads.Completed)
	assert.Equal(t, 0, resp.Downloads.Importing)
	assert.Equal(t, 10, resp.Downloads.Imported)
	assert.Equal(t, 5, resp.Downloads.Cleaned)
	assert.Equal(t, 1, resp.Downloads.Failed)

	// Verify stuck
	assert.Equal(t, 1, resp.Stuck.Count)
	assert.Equal(t, int64(60), resp.Stuck.Threshold)

	// Verify library
	assert.Equal(t, 150, resp.Library.Movies)
	assert.Equal(t, 25, resp.Library.Series)
}

func TestClient_Dashboard_ServerError(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/dashboard").
		ExpectGET().
		RespondError(http.StatusInternalServerError, "database connection failed").
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Dashboard()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestClient_Verify_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/verify").
		ExpectGET().
		RespondJSON(VerifyResponse{
			Connections: struct {
				Plex    bool   `json:"plex"`
				PlexErr string `json:"plex_error,omitempty"`
				SABnzbd bool   `json:"sabnzbd"`
				SABErr  string `json:"sabnzbd_error,omitempty"`
			}{
				Plex:    true,
				SABnzbd: true,
			},
			Checked: 5,
			Passed:  4,
			Problems: []VerifyProblem{
				{
					DownloadID: 42,
					Status:     "downloading",
					Title:      "Test.Movie.2024.1080p.WEB-DL",
					Since:      "2024-01-15T10:00:00Z",
					Issue:      "stuck",
					Checks:     []string{"progress_stalled", "no_eta"},
					Likely:     "Download stalled in SABnzbd",
					Fixes:      []string{"Check SABnzbd queue", "Verify disk space"},
				},
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Verify(nil)
	require.NoError(t, err)

	// Verify connections
	assert.True(t, resp.Connections.Plex)
	assert.True(t, resp.Connections.SABnzbd)
	assert.Empty(t, resp.Connections.PlexErr)
	assert.Empty(t, resp.Connections.SABErr)

	// Verify counts
	assert.Equal(t, 5, resp.Checked)
	assert.Equal(t, 4, resp.Passed)

	// Verify problems
	require.Len(t, resp.Problems, 1)
	prob := resp.Problems[0]
	assert.Equal(t, int64(42), prob.DownloadID)
	assert.Equal(t, "downloading", prob.Status)
	assert.Equal(t, "Test.Movie.2024.1080p.WEB-DL", prob.Title)
	assert.Equal(t, "2024-01-15T10:00:00Z", prob.Since)
	assert.Equal(t, "stuck", prob.Issue)
	assert.Equal(t, []string{"progress_stalled", "no_eta"}, prob.Checks)
	assert.Equal(t, "Download stalled in SABnzbd", prob.Likely)
	assert.Equal(t, []string{"Check SABnzbd queue", "Verify disk space"}, prob.Fixes)
}

func TestClient_Verify_WithID(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, VerifyResponse{
				Checked:  1,
				Passed:   1,
				Problems: []VerifyProblem{},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	id := int64(123)
	resp, err := client.Verify(&id)
	require.NoError(t, err)

	// Verify the ID was sent as query parameter
	assert.Equal(t, "/api/v1/verify?id=123", receivedPath)

	// Verify response parsing
	assert.Equal(t, 1, resp.Checked)
	assert.Equal(t, 1, resp.Passed)
	assert.Empty(t, resp.Problems)
}
