package main

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSearch_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/search").
		ExpectPOST().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"), "unexpected content-type")
			respondJSON(t, w, SearchResponse{
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
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Search("The Matrix 1999", "", "")
	require.NoError(t, err)
	require.Len(t, resp.Releases, 2)
	assert.Equal(t, "The Matrix 1999 1080p BluRay x264", resp.Releases[0].Title)
	assert.Equal(t, 950, resp.Releases[1].Score)
}

func TestClientSearch_EmptyResults(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/search").
		ExpectPOST().
		RespondJSON(SearchResponse{
			Releases: []ReleaseResponse{},
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Search("Nonexistent Movie 2099", "", "")
	require.NoError(t, err)
	assert.Empty(t, resp.Releases)
}

func TestClientSearch_WithErrors(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/search").
		ExpectPOST().
		RespondJSON(SearchResponse{
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
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	resp, err := client.Search("query", "", "")
	require.NoError(t, err)
	assert.Len(t, resp.Releases, 1)
	assert.Len(t, resp.Errors, 2)
	assert.Equal(t, "DrunkenSlug: connection timeout", resp.Errors[0])
}

func TestClientSearch_RequestBody(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		contentType   string
		profile       string
		expectType    bool
		expectProfile bool
	}{
		{
			name:          "with all fields",
			query:         "The Matrix",
			contentType:   "movie",
			profile:       "hd",
			expectType:    true,
			expectProfile: true,
		},
		{
			name:          "query only - omits empty fields",
			query:         "query only",
			contentType:   "",
			profile:       "",
			expectType:    false,
			expectProfile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody map[string]any

			srv := newMockServer(t).
				Handler(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("failed to read request body: %v", err)
						return
					}
					if err := json.Unmarshal(body, &receivedBody); err != nil {
						t.Errorf("failed to parse request body: %v", err)
						return
					}
					respondJSON(t, w, SearchResponse{})
				}).
				Build()
			defer srv.Close()

			client := NewClient(srv.URL)
			_, err := client.Search(tt.query, tt.contentType, tt.profile)
			require.NoError(t, err)

			assert.Equal(t, tt.query, receivedBody["query"])

			_, hasType := receivedBody["type"]
			assert.Equal(t, tt.expectType, hasType, "type field presence mismatch")

			_, hasProfile := receivedBody["profile"]
			assert.Equal(t, tt.expectProfile, hasProfile, "profile field presence mismatch")

			if tt.expectType {
				assert.Equal(t, tt.contentType, receivedBody["type"])
			}
			if tt.expectProfile {
				assert.Equal(t, tt.profile, receivedBody["profile"])
			}
		})
	}
}

func TestClientSearch_ServerError(t *testing.T) {
	srv := newMockServer(t).
		RespondError(http.StatusInternalServerError, "search service unavailable").
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Search("query", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
