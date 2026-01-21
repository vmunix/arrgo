// internal/importer/plex_test.go
package importer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlexClient_GetSections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections", r.URL.Path)
		assert.Equal(t, "test-token", r.Header.Get("X-Plex-Token"))

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
  <Directory key="2" title="TV Shows" type="show">
    <Location path="/tv"/>
  </Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)
	sections, err := client.GetSections(context.Background())
	require.NoError(t, err, "GetSections")

	require.Len(t, sections, 2)
	assert.Equal(t, "1", sections[0].Key)
	assert.Equal(t, "Movies", sections[0].Title)
	assert.Equal(t, "/movies", sections[0].Locations[0].Path)
}

func TestPlexClient_ScanPath(t *testing.T) {
	scanCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/sections" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
</MediaContainer>`))
			return
		}

		if r.URL.Path == "/library/sections/1/refresh" {
			scanCalled = true
			assert.Equal(t, "/movies/Test Movie (2024)", r.URL.Query().Get("path"))
			w.WriteHeader(http.StatusOK)
			return
		}

		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)
	err := client.ScanPath(context.Background(), "/movies/Test Movie (2024)/movie.mkv")
	require.NoError(t, err, "ScanPath")

	assert.True(t, scanCalled, "scan endpoint was not called")
}

func TestPlexClient_ScanPath_NoMatchingSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)
	err := client.ScanPath(context.Background(), "/other/path/movie.mkv")
	assert.Error(t, err, "expected error for non-matching path")
}

func TestPlexClient_ConnectionError(t *testing.T) {
	client := NewPlexClient("http://localhost:99999", "token", nil)
	_, err := client.GetSections(context.Background())
	assert.Error(t, err, "expected connection error")
}

func TestPlexClient_GetIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/", r.URL.Path)
		assert.Equal(t, "test-token", r.Header.Get("X-Plex-Token"))
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer friendlyName="velcro" version="1.42.2.10156">
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)
	identity, err := client.GetIdentity(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "velcro", identity.Name)
	assert.Equal(t, "1.42.2.10156", identity.Version)
}

func TestPlexClient_GetIdentity_ConnectionError(t *testing.T) {
	client := NewPlexClient("http://localhost:1", "test-token", nil)
	_, err := client.GetIdentity(context.Background())

	assert.Error(t, err, "expected error")
}

func TestPlexClient_GetSections_WithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections", r.URL.Path)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2">
<Directory key="1" type="movie" title="Movies" scannedAt="1737200000" refreshing="0">
<Location path="/data/media/movies"/>
</Directory>
<Directory key="2" type="show" title="TV Shows" scannedAt="1737100000" refreshing="1">
<Location path="/data/media/tv"/>
</Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)
	sections, err := client.GetSections(context.Background())

	require.NoError(t, err)
	require.Len(t, sections, 2)
	assert.Equal(t, int64(1737200000), sections[0].ScannedAt)
	assert.False(t, sections[0].Refreshing())
	assert.Equal(t, int64(1737100000), sections[1].ScannedAt)
	assert.True(t, sections[1].Refreshing())
}

func TestPlexClient_GetLibraryCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections/1/all", r.URL.Path)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="42">
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)
	count, err := client.GetLibraryCount(context.Background(), "1")

	require.NoError(t, err)
	assert.Equal(t, 42, count)
}

func TestPlexClient_HasMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" && strings.Contains(r.URL.RawQuery, "Test+Movie") {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<MediaContainer>
  <Video title="Test Movie" year="2024" type="movie"/>
</MediaContainer>`)
			return
		}
		// Return empty result for non-matching queries
		if r.URL.Path == "/search" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<MediaContainer>
</MediaContainer>`)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token", nil)

	// Should find movie
	found, err := client.HasMovie(context.Background(), "Test Movie", 2024)
	require.NoError(t, err, "HasMovie")
	assert.True(t, found, "should find Test Movie (2024)")

	// Should not find with wrong year
	found, err = client.HasMovie(context.Background(), "Test Movie", 2023)
	require.NoError(t, err, "HasMovie")
	assert.False(t, found, "should not find Test Movie (2023)")

	// Should not find non-existent movie
	found, err = client.HasMovie(context.Background(), "Nonexistent", 2024)
	require.NoError(t, err, "HasMovie")
	assert.False(t, found, "should not find Nonexistent")
}
