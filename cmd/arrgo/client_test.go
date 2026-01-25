package main

import (
	"encoding/json"
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

func TestClient_PlexStatus_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/status").
		ExpectGET().
		RespondJSON(PlexStatusResponse{
			Connected:  true,
			ServerName: "Plex Media Server",
			Version:    "1.40.0.8395",
			Libraries: []PlexLibrary{
				{
					Key:       "1",
					Title:     "Movies",
					Type:      "movie",
					ItemCount: 150,
					Location:  "/media/movies",
					ScannedAt: 1705329600,
				},
				{
					Key:       "2",
					Title:     "TV Shows",
					Type:      "show",
					ItemCount: 50,
					Location:  "/media/tv",
					ScannedAt: 1705329500,
				},
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexStatus()
	require.NoError(t, err)

	// Verify connection status
	assert.True(t, resp.Connected)
	assert.Equal(t, "Plex Media Server", resp.ServerName)
	assert.Equal(t, "1.40.0.8395", resp.Version)
	assert.Empty(t, resp.Error)

	// Verify libraries
	require.Len(t, resp.Libraries, 2)

	movies := resp.Libraries[0]
	assert.Equal(t, "1", movies.Key)
	assert.Equal(t, "Movies", movies.Title)
	assert.Equal(t, "movie", movies.Type)
	assert.Equal(t, 150, movies.ItemCount)
	assert.Equal(t, "/media/movies", movies.Location)
	assert.Equal(t, int64(1705329600), movies.ScannedAt)

	tv := resp.Libraries[1]
	assert.Equal(t, "2", tv.Key)
	assert.Equal(t, "TV Shows", tv.Title)
	assert.Equal(t, "show", tv.Type)
	assert.Equal(t, 50, tv.ItemCount)
}

func TestClient_PlexStatus_NotConnected(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/status").
		ExpectGET().
		RespondJSON(PlexStatusResponse{
			Connected: false,
			Error:     "connection refused: dial tcp 192.168.1.100:32400",
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexStatus()
	require.NoError(t, err)

	// Verify disconnected status
	assert.False(t, resp.Connected)
	assert.Empty(t, resp.ServerName)
	assert.Empty(t, resp.Version)
	assert.Empty(t, resp.Libraries)
	assert.Equal(t, "connection refused: dial tcp 192.168.1.100:32400", resp.Error)
}

func TestClient_PlexScan_Success(t *testing.T) {
	var receivedReq PlexScanRequest

	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/scan").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedReq)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			respondJSON(t, w, PlexScanResponse{
				Scanned: []string{"Movies", "TV Shows"},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexScan([]string{"Movies", "TV Shows"})
	require.NoError(t, err)

	// Verify request body was sent correctly
	assert.Equal(t, []string{"Movies", "TV Shows"}, receivedReq.Libraries)

	// Verify response
	assert.Equal(t, []string{"Movies", "TV Shows"}, resp.Scanned)
}

func TestClient_PlexListLibrary_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/plex/libraries/Movies/items").
		ExpectGET().
		RespondJSON(PlexListResponse{
			Library: "Movies",
			Items: []PlexItemResponse{
				{
					Title:     "The Matrix",
					Year:      1999,
					Type:      "movie",
					AddedAt:   1705329600,
					FilePath:  "/media/movies/The Matrix (1999)/The Matrix.mkv",
					Tracked:   true,
					ContentID: ptrInt64(42),
				},
				{
					Title:    "Inception",
					Year:     2010,
					Type:     "movie",
					AddedAt:  1705329700,
					FilePath: "/media/movies/Inception (2010)/Inception.mkv",
					Tracked:  false,
				},
			},
			Total: 2,
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexListLibrary("Movies")
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, "Movies", resp.Library)
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Items, 2)

	// Verify first item (tracked)
	matrix := resp.Items[0]
	assert.Equal(t, "The Matrix", matrix.Title)
	assert.Equal(t, 1999, matrix.Year)
	assert.Equal(t, "movie", matrix.Type)
	assert.Equal(t, int64(1705329600), matrix.AddedAt)
	assert.Equal(t, "/media/movies/The Matrix (1999)/The Matrix.mkv", matrix.FilePath)
	assert.True(t, matrix.Tracked)
	require.NotNil(t, matrix.ContentID)
	assert.Equal(t, int64(42), *matrix.ContentID)

	// Verify second item (not tracked)
	inception := resp.Items[1]
	assert.Equal(t, "Inception", inception.Title)
	assert.False(t, inception.Tracked)
	assert.Nil(t, inception.ContentID)
}

func TestClient_PlexSearch_Success(t *testing.T) {
	var receivedQuery string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			// Verify path and capture query param
			assert.Equal(t, "/api/v1/plex/search", r.URL.Path)
			receivedQuery = r.URL.Query().Get("query")
			respondJSON(t, w, PlexSearchResponse{
				Query: "Matrix",
				Items: []PlexItemResponse{
					{
						Title:     "The Matrix",
						Year:      1999,
						Type:      "movie",
						AddedAt:   1705329600,
						FilePath:  "/media/movies/The Matrix (1999)/The Matrix.mkv",
						Tracked:   true,
						ContentID: ptrInt64(42),
					},
					{
						Title:    "The Matrix Reloaded",
						Year:     2003,
						Type:     "movie",
						AddedAt:  1705329650,
						FilePath: "/media/movies/The Matrix Reloaded (2003)/The Matrix Reloaded.mkv",
						Tracked:  true,
						ContentID: ptrInt64(43),
					},
				},
				Total: 2,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.PlexSearch("Matrix")
	require.NoError(t, err)

	// Verify query was sent correctly
	assert.Equal(t, "Matrix", receivedQuery)

	// Verify response
	assert.Equal(t, "Matrix", resp.Query)
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Items, 2)

	// Verify items
	assert.Equal(t, "The Matrix", resp.Items[0].Title)
	assert.Equal(t, 1999, resp.Items[0].Year)
	assert.Equal(t, "The Matrix Reloaded", resp.Items[1].Title)
	assert.Equal(t, 2003, resp.Items[1].Year)
}

// ptrInt64 is a helper to create a pointer to an int64.
func ptrInt64(v int64) *int64 {
	return &v
}

func TestClient_Import_TrackedDownload(t *testing.T) {
	var receivedReq ImportRequest

	srv := newMockServer(t).
		ExpectPath("/api/v1/import").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedReq)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			respondJSON(t, w, ImportResponse{
				FileID:       101,
				ContentID:    42,
				SourcePath:   "/downloads/complete/Test.Movie.2024.1080p.WEB-DL.mkv",
				DestPath:     "/media/movies/Test Movie (2024)/Test Movie (2024).mkv",
				SizeBytes:    4294967296,
				PlexNotified: true,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	downloadID := int64(42)
	resp, err := client.Import(&ImportRequest{
		DownloadID: &downloadID,
	})
	require.NoError(t, err)

	// Verify request body was sent correctly
	require.NotNil(t, receivedReq.DownloadID)
	assert.Equal(t, int64(42), *receivedReq.DownloadID)
	assert.Empty(t, receivedReq.Path)
	assert.Empty(t, receivedReq.Title)
	assert.Zero(t, receivedReq.Year)

	// Verify response
	assert.Equal(t, int64(101), resp.FileID)
	assert.Equal(t, int64(42), resp.ContentID)
	assert.Equal(t, "/downloads/complete/Test.Movie.2024.1080p.WEB-DL.mkv", resp.SourcePath)
	assert.Equal(t, "/media/movies/Test Movie (2024)/Test Movie (2024).mkv", resp.DestPath)
	assert.Equal(t, int64(4294967296), resp.SizeBytes)
	assert.True(t, resp.PlexNotified)
}

func TestClient_Import_ManualPath(t *testing.T) {
	var receivedReq ImportRequest

	srv := newMockServer(t).
		ExpectPath("/api/v1/import").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedReq)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			respondJSON(t, w, ImportResponse{
				FileID:       102,
				ContentID:    55,
				SourcePath:   "/manual/Blade.Runner.1982.2160p.UHD.BluRay.mkv",
				DestPath:     "/media/movies/Blade Runner (1982)/Blade Runner (1982).mkv",
				SizeBytes:    8589934592,
				PlexNotified: true,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Import(&ImportRequest{
		Path:    "/manual/Blade.Runner.1982.2160p.UHD.BluRay.mkv",
		Title:   "Blade Runner",
		Year:    1982,
		Type:    "movie",
		Quality: "uhd",
	})
	require.NoError(t, err)

	// Verify request body was sent correctly (manual import fields)
	assert.Nil(t, receivedReq.DownloadID)
	assert.Equal(t, "/manual/Blade.Runner.1982.2160p.UHD.BluRay.mkv", receivedReq.Path)
	assert.Equal(t, "Blade Runner", receivedReq.Title)
	assert.Equal(t, 1982, receivedReq.Year)
	assert.Equal(t, "movie", receivedReq.Type)
	assert.Equal(t, "uhd", receivedReq.Quality)

	// Verify response
	assert.Equal(t, int64(102), resp.FileID)
	assert.Equal(t, int64(55), resp.ContentID)
	assert.Equal(t, "/manual/Blade.Runner.1982.2160p.UHD.BluRay.mkv", resp.SourcePath)
	assert.Equal(t, "/media/movies/Blade Runner (1982)/Blade Runner (1982).mkv", resp.DestPath)
	assert.Equal(t, int64(8589934592), resp.SizeBytes)
	assert.True(t, resp.PlexNotified)
}

func TestClient_Events_Success(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, ListEventsResponse{
				Items: []EventResponse{
					{
						ID:         1,
						EventType:  "download.grabbed",
						EntityType: "download",
						EntityID:   42,
						OccurredAt: "2024-01-15T10:00:00Z",
					},
					{
						ID:         2,
						EventType:  "download.completed",
						EntityType: "download",
						EntityID:   42,
						OccurredAt: "2024-01-15T11:30:00Z",
					},
					{
						ID:         3,
						EventType:  "import.completed",
						EntityType: "file",
						EntityID:   101,
						OccurredAt: "2024-01-15T11:35:00Z",
					},
				},
				Total: 3,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Events(50)
	require.NoError(t, err)

	// Verify the limit was sent as query parameter
	assert.Equal(t, "/api/v1/events?limit=50", receivedPath)

	// Verify response
	assert.Equal(t, 3, resp.Total)
	require.Len(t, resp.Items, 3)

	// Verify first event
	assert.Equal(t, int64(1), resp.Items[0].ID)
	assert.Equal(t, "download.grabbed", resp.Items[0].EventType)
	assert.Equal(t, "download", resp.Items[0].EntityType)
	assert.Equal(t, int64(42), resp.Items[0].EntityID)
	assert.Equal(t, "2024-01-15T10:00:00Z", resp.Items[0].OccurredAt)

	// Verify second event
	assert.Equal(t, int64(2), resp.Items[1].ID)
	assert.Equal(t, "download.completed", resp.Items[1].EventType)

	// Verify third event
	assert.Equal(t, int64(3), resp.Items[2].ID)
	assert.Equal(t, "import.completed", resp.Items[2].EventType)
	assert.Equal(t, "file", resp.Items[2].EntityType)
}

func TestClient_DownloadEvents_Success(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			respondJSON(t, w, ListEventsResponse{
				Items: []EventResponse{
					{
						ID:         10,
						EventType:  "download.grabbed",
						EntityType: "download",
						EntityID:   123,
						OccurredAt: "2024-01-20T08:00:00Z",
					},
					{
						ID:         15,
						EventType:  "download.progress",
						EntityType: "download",
						EntityID:   123,
						OccurredAt: "2024-01-20T08:30:00Z",
					},
					{
						ID:         20,
						EventType:  "download.completed",
						EntityType: "download",
						EntityID:   123,
						OccurredAt: "2024-01-20T09:00:00Z",
					},
				},
				Total: 3,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.DownloadEvents(123)
	require.NoError(t, err)

	// Verify the download ID was included in the path
	assert.Equal(t, "/api/v1/downloads/123/events", receivedPath)

	// Verify response
	assert.Equal(t, 3, resp.Total)
	require.Len(t, resp.Items, 3)

	// Verify all events are for the same download
	for _, event := range resp.Items {
		assert.Equal(t, "download", event.EntityType)
		assert.Equal(t, int64(123), event.EntityID)
	}

	// Verify event types
	assert.Equal(t, "download.grabbed", resp.Items[0].EventType)
	assert.Equal(t, "download.progress", resp.Items[1].EventType)
	assert.Equal(t, "download.completed", resp.Items[2].EventType)
}

func TestClient_Download_Success(t *testing.T) {
	var receivedPath string

	progress := 75.5
	size := int64(4294967296)
	speed := int64(10485760)
	eta := "15m"

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			respondJSON(t, w, DownloadResponse{
				ID:          42,
				ContentID:   100,
				EpisodeID:   nil,
				Client:      "sabnzbd",
				ClientID:    "SABnzbd_nzo_abc123",
				Status:      "downloading",
				ReleaseName: "Test.Movie.2024.1080p.WEB-DL.DDP5.1.H.264-GROUP",
				Indexer:     "nzbgeek",
				AddedAt:     "2024-01-15T10:00:00Z",
				CompletedAt: nil,
				Progress:    &progress,
				Size:        &size,
				Speed:       &speed,
				ETA:         &eta,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Download(42)
	require.NoError(t, err)

	// Verify the download ID was included in the path
	assert.Equal(t, "/api/v1/downloads/42", receivedPath)

	// Verify response fields
	assert.Equal(t, int64(42), resp.ID)
	assert.Equal(t, int64(100), resp.ContentID)
	assert.Nil(t, resp.EpisodeID)
	assert.Equal(t, "sabnzbd", resp.Client)
	assert.Equal(t, "SABnzbd_nzo_abc123", resp.ClientID)
	assert.Equal(t, "downloading", resp.Status)
	assert.Equal(t, "Test.Movie.2024.1080p.WEB-DL.DDP5.1.H.264-GROUP", resp.ReleaseName)
	assert.Equal(t, "nzbgeek", resp.Indexer)
	assert.Equal(t, "2024-01-15T10:00:00Z", resp.AddedAt)
	assert.Nil(t, resp.CompletedAt)

	// Verify live status fields
	require.NotNil(t, resp.Progress)
	assert.InDelta(t, 75.5, *resp.Progress, 0.001)
	require.NotNil(t, resp.Size)
	assert.Equal(t, int64(4294967296), *resp.Size)
	require.NotNil(t, resp.Speed)
	assert.Equal(t, int64(10485760), *resp.Speed)
	require.NotNil(t, resp.ETA)
	assert.Equal(t, "15m", *resp.ETA)
}

func TestClient_Indexers_Success(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, ListIndexersResponse{
				Indexers: []IndexerResponse{
					{
						Name: "nzbgeek",
						URL:  "https://api.nzbgeek.info",
					},
					{
						Name: "drunkenslug",
						URL:  "https://api.drunkenslug.com",
					},
				},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Indexers(false)
	require.NoError(t, err)

	// Verify no test param was sent
	assert.Equal(t, "/api/v1/indexers", receivedPath)

	// Verify response
	require.Len(t, resp.Indexers, 2)
	assert.Equal(t, "nzbgeek", resp.Indexers[0].Name)
	assert.Equal(t, "https://api.nzbgeek.info", resp.Indexers[0].URL)
	assert.Equal(t, "drunkenslug", resp.Indexers[1].Name)
	assert.Equal(t, "https://api.drunkenslug.com", resp.Indexers[1].URL)
}

func TestClient_Indexers_WithTest(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, ListIndexersResponse{
				Indexers: []IndexerResponse{
					{
						Name:       "nzbgeek",
						URL:        "https://api.nzbgeek.info",
						Status:     "ok",
						ResponseMs: 150,
					},
					{
						Name:       "drunkenslug",
						URL:        "https://api.drunkenslug.com",
						Status:     "error",
						Error:      "connection timeout",
						ResponseMs: 5000,
					},
				},
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Indexers(true)
	require.NoError(t, err)

	// Verify test param was sent
	assert.Equal(t, "/api/v1/indexers?test=true", receivedPath)

	// Verify response with test results
	require.Len(t, resp.Indexers, 2)

	// First indexer - success
	assert.Equal(t, "nzbgeek", resp.Indexers[0].Name)
	assert.Equal(t, "ok", resp.Indexers[0].Status)
	assert.Equal(t, int64(150), resp.Indexers[0].ResponseMs)
	assert.Empty(t, resp.Indexers[0].Error)

	// Second indexer - error
	assert.Equal(t, "drunkenslug", resp.Indexers[1].Name)
	assert.Equal(t, "error", resp.Indexers[1].Status)
	assert.Equal(t, "connection timeout", resp.Indexers[1].Error)
	assert.Equal(t, int64(5000), resp.Indexers[1].ResponseMs)
}

func TestClient_Profiles_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/profiles").
		ExpectGET().
		RespondJSON(ListProfilesResponse{
			Profiles: []ProfileResponse{
				{
					Name:   "hd",
					Accept: []string{"1080p", "720p"},
				},
				{
					Name:   "uhd",
					Accept: []string{"2160p", "1080p"},
				},
				{
					Name:   "any",
					Accept: []string{"2160p", "1080p", "720p", "480p"},
				},
			},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Profiles()
	require.NoError(t, err)

	// Verify response
	require.Len(t, resp.Profiles, 3)

	// Verify first profile
	assert.Equal(t, "hd", resp.Profiles[0].Name)
	assert.Equal(t, []string{"1080p", "720p"}, resp.Profiles[0].Accept)

	// Verify second profile
	assert.Equal(t, "uhd", resp.Profiles[1].Name)
	assert.Equal(t, []string{"2160p", "1080p"}, resp.Profiles[1].Accept)

	// Verify third profile
	assert.Equal(t, "any", resp.Profiles[2].Name)
	assert.Equal(t, []string{"2160p", "1080p", "720p", "480p"}, resp.Profiles[2].Accept)
}

func TestClient_Files_All(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, ListFilesResponse{
				Items: []FileResponse{
					{
						ID:        1,
						ContentID: 100,
						Path:      "/media/movies/The Matrix (1999)/The Matrix.mkv",
						SizeBytes: 4294967296,
						Quality:   "1080p",
						Source:    "bluray",
					},
					{
						ID:        2,
						ContentID: 101,
						Path:      "/media/movies/Inception (2010)/Inception.mkv",
						SizeBytes: 5368709120,
						Quality:   "2160p",
						Source:    "uhd.bluray",
					},
				},
				Total: 2,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Files(nil)
	require.NoError(t, err)

	// Verify no filter was sent
	assert.Equal(t, "/api/v1/files", receivedPath)

	// Verify response
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Items, 2)

	// Verify first file
	assert.Equal(t, int64(1), resp.Items[0].ID)
	assert.Equal(t, int64(100), resp.Items[0].ContentID)
	assert.Equal(t, "/media/movies/The Matrix (1999)/The Matrix.mkv", resp.Items[0].Path)
	assert.Equal(t, int64(4294967296), resp.Items[0].SizeBytes)
	assert.Equal(t, "1080p", resp.Items[0].Quality)
	assert.Equal(t, "bluray", resp.Items[0].Source)

	// Verify second file
	assert.Equal(t, int64(2), resp.Items[1].ID)
	assert.Equal(t, int64(101), resp.Items[1].ContentID)
	assert.Equal(t, "2160p", resp.Items[1].Quality)
}

func TestClient_Files_ByContentID(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, ListFilesResponse{
				Items: []FileResponse{
					{
						ID:        5,
						ContentID: 42,
						Path:      "/media/movies/Test Movie (2024)/Test Movie.mkv",
						SizeBytes: 3221225472,
						Quality:   "1080p",
						Source:    "web",
					},
				},
				Total: 1,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	contentID := int64(42)
	resp, err := client.Files(&contentID)
	require.NoError(t, err)

	// Verify content_id filter was sent
	assert.Equal(t, "/api/v1/files?content_id=42", receivedPath)

	// Verify response
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, int64(42), resp.Items[0].ContentID)
}

func TestClient_RetryDownload_Success(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			respondJSON(t, w, RetryResponse{
				NewDownloadID: 456,
				ReleaseName:   "Test.Movie.2024.1080p.BluRay.x264-GROUP",
				Message:       "Download re-grabbed successfully",
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.RetryDownload(123)
	require.NoError(t, err)

	// Verify the download ID was included in the path
	assert.Equal(t, "/api/v1/downloads/123/retry", receivedPath)

	// Verify response
	assert.Equal(t, int64(456), resp.NewDownloadID)
	assert.Equal(t, "Test.Movie.2024.1080p.BluRay.x264-GROUP", resp.ReleaseName)
	assert.Equal(t, "Download re-grabbed successfully", resp.Message)
}

func TestClient_AddContent_Success(t *testing.T) {
	var receivedReq map[string]any

	srv := newMockServer(t).
		ExpectPath("/api/v1/content").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedReq)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			respondJSON(t, w, ContentResponse{
				ID:             42,
				Type:           "movie",
				Title:          "The Matrix",
				Year:           1999,
				Status:         "wanted",
				QualityProfile: "hd",
				RootPath:       "/media/movies",
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.AddContent("movie", "The Matrix", 1999, "hd")
	require.NoError(t, err)

	// Verify request body was sent correctly
	assert.Equal(t, "movie", receivedReq["type"])
	assert.Equal(t, "The Matrix", receivedReq["title"])
	assert.InDelta(t, 1999, receivedReq["year"], 0.001)
	assert.Equal(t, "hd", receivedReq["quality_profile"])

	// Verify response
	assert.Equal(t, int64(42), resp.ID)
	assert.Equal(t, "movie", resp.Type)
	assert.Equal(t, "The Matrix", resp.Title)
	assert.Equal(t, 1999, resp.Year)
	assert.Equal(t, "wanted", resp.Status)
	assert.Equal(t, "hd", resp.QualityProfile)
	assert.Equal(t, "/media/movies", resp.RootPath)
}

func TestClient_FindContent_Found(t *testing.T) {
	var receivedPath string

	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.String()
			respondJSON(t, w, ListContentResponse{
				Items: []ContentResponse{
					{
						ID:             42,
						Type:           "movie",
						Title:          "The Matrix",
						Year:           1999,
						Status:         "available",
						QualityProfile: "hd",
						RootPath:       "/media/movies",
					},
				},
				Total:  1,
				Limit:  1,
				Offset: 0,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.FindContent("movie", "The Matrix", 1999)
	require.NoError(t, err)

	// Verify query parameters were sent correctly
	assert.Contains(t, receivedPath, "type=movie")
	assert.Contains(t, receivedPath, "title=The+Matrix")
	assert.Contains(t, receivedPath, "year=1999")
	assert.Contains(t, receivedPath, "limit=1")

	// Verify response (should return first item)
	require.NotNil(t, resp)
	assert.Equal(t, int64(42), resp.ID)
	assert.Equal(t, "movie", resp.Type)
	assert.Equal(t, "The Matrix", resp.Title)
	assert.Equal(t, 1999, resp.Year)
	assert.Equal(t, "available", resp.Status)
}

func TestClient_FindContent_NotFound(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			respondJSON(t, w, ListContentResponse{
				Items:  []ContentResponse{},
				Total:  0,
				Limit:  1,
				Offset: 0,
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.FindContent("movie", "Nonexistent Movie", 2099)

	// Should return nil, nil when not found (not an error)
	require.NoError(t, err)
	assert.Nil(t, resp)
}

func TestClient_Grab_Success(t *testing.T) {
	var receivedReq map[string]any

	srv := newMockServer(t).
		ExpectPath("/api/v1/grab").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedReq)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			respondJSON(t, w, GrabResponse{
				DownloadID: 789,
				Status:     "queued",
			})
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Grab(42, "https://api.nzbgeek.info/api?t=get&id=abc123", "Test.Movie.2024.1080p.WEB-DL", "nzbgeek")
	require.NoError(t, err)

	// Verify request body was sent correctly
	assert.InDelta(t, 42, receivedReq["content_id"], 0.001)
	assert.Equal(t, "https://api.nzbgeek.info/api?t=get&id=abc123", receivedReq["download_url"])
	assert.Equal(t, "Test.Movie.2024.1080p.WEB-DL", receivedReq["title"])
	assert.Equal(t, "nzbgeek", receivedReq["indexer"])

	// Verify response
	assert.Equal(t, int64(789), resp.DownloadID)
	assert.Equal(t, "queued", resp.Status)
}
