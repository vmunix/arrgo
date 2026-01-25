package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSearch_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/search").
		ExpectGET().
		Handler(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "The Matrix 1999", r.URL.Query().Get("query"), "unexpected query param")
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
		ExpectGET().
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
		ExpectGET().
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

func TestClientSearch_QueryParams(t *testing.T) {
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
			srv := newMockServer(t).
				Handler(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)

					query := r.URL.Query()
					assert.Equal(t, tt.query, query.Get("query"))

					hasType := query.Has("type")
					assert.Equal(t, tt.expectType, hasType, "type param presence mismatch")

					hasProfile := query.Has("profile")
					assert.Equal(t, tt.expectProfile, hasProfile, "profile param presence mismatch")

					if tt.expectType {
						assert.Equal(t, tt.contentType, query.Get("type"))
					}
					if tt.expectProfile {
						assert.Equal(t, tt.profile, query.Get("profile"))
					}

					respondJSON(t, w, SearchResponse{})
				}).
				Build()
			defer srv.Close()

			client := NewClient(srv.URL)
			_, err := client.Search(tt.query, tt.contentType, tt.profile)
			require.NoError(t, err)
		})
	}
}

func TestClientSearch_ServerError(t *testing.T) {
	srv := newMockServer(t).
		ExpectGET().
		RespondError(http.StatusInternalServerError, "search service unavailable").
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Search("query", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
