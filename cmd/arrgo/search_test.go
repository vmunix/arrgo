package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSearch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/search", r.URL.Path, "unexpected path")
		assert.Equal(t, http.MethodPost, r.Method, "unexpected method")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"), "unexpected content-type")

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
	require.NoError(t, err)
	require.Len(t, resp.Releases, 2)
	assert.Equal(t, "The Matrix 1999 1080p BluRay x264", resp.Releases[0].Title)
	assert.Equal(t, 950, resp.Releases[1].Score)
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
	require.NoError(t, err)
	assert.Empty(t, resp.Releases)
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
	require.NoError(t, err)
	assert.Len(t, resp.Releases, 1)
	assert.Len(t, resp.Errors, 2)
	assert.Equal(t, "DrunkenSlug: connection timeout", resp.Errors[0])
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
	require.NoError(t, err)

	assert.Equal(t, "The Matrix", receivedBody["query"])
	assert.Equal(t, "movie", receivedBody["type"])
	assert.Equal(t, "hd", receivedBody["profile"])
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
	require.NoError(t, err)

	assert.Equal(t, "query only", receivedBody["query"])
	_, exists := receivedBody["type"]
	assert.False(t, exists, "type should not be present when empty")
	_, exists = receivedBody["profile"]
	assert.False(t, exists, "profile should not be present when empty")
}

func TestClientSearch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("search service unavailable"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Search("query", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
