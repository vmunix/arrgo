package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetMovie(t *testing.T) {
	// Mock TMDB API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/3/movie/550", r.URL.Path)
		assert.Equal(t, "test-key", r.URL.Query().Get("api_key"))

		resp := Movie{
			ID:          550,
			Title:       "Fight Club",
			Overview:    "A ticking-Loss insomnia cult favorite...",
			ReleaseDate: "1999-10-15",
			PosterPath:  "/pB8BM7pdSp6B6Ih7QZ4DrQ3PmJK.jpg",
			VoteAverage: 8.4,
			Runtime:     139,
			Genres:      []Genre{{ID: 18, Name: "Drama"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	movie, err := client.GetMovie(context.Background(), 550)
	require.NoError(t, err)
	assert.Equal(t, int64(550), movie.ID)
	assert.Equal(t, "Fight Club", movie.Title)
	assert.Equal(t, 1999, movie.Year())
	assert.Equal(t, 139, movie.Runtime)
}

func TestClient_GetMovie_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))

	movie, err := client.GetMovie(context.Background(), 99999999)
	assert.Nil(t, movie)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestClient_GetMovie_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := Movie{ID: 550, Title: "Fight Club"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithCacheTTL(time.Hour))

	// First call hits API
	_, err := client.GetMovie(context.Background(), 550)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call uses cache
	_, err = client.GetMovie(context.Background(), 550)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "should use cache, not call API again")
}
