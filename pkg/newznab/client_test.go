package newznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testXMLResponse = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Test.Release.2024.1080p.BluRay.x264</title>
      <guid>abc123</guid>
      <link>http://example.com/download/abc123</link>
      <pubDate>Sat, 18 Jan 2026 12:00:00 +0000</pubDate>
      <enclosure url="http://example.com/download/abc123" length="1500000000" type="application/x-nzb" />
      <newznab:attr name="category" value="2040" />
    </item>
    <item>
      <title>Another.Movie.2023.720p.WEB-DL</title>
      <guid>def456</guid>
      <link>http://example.com/download/def456</link>
      <pubDate>Fri, 17 Jan 2026 10:30:00 +0000</pubDate>
      <enclosure url="http://example.com/download/def456" length="800000000" type="application/x-nzb" />
    </item>
  </channel>
</rss>`

func TestClient_Search(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/api", r.URL.Path, "unexpected path")
		assert.Equal(t, "test-key", r.URL.Query().Get("apikey"), "unexpected apikey")
		assert.Equal(t, "test query", r.URL.Query().Get("q"), "unexpected query")
		assert.Equal(t, "search", r.URL.Query().Get("t"), "unexpected type")

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	// Create client
	client := NewClient("TestIndexer", server.URL, "test-key", nil)

	// Search
	releases, err := client.Search(context.Background(), "test query", []int{2000})
	require.NoError(t, err, "Search failed")

	// Verify results
	require.Len(t, releases, 2, "expected 2 releases")

	// Check first release
	assert.Equal(t, "Test.Release.2024.1080p.BluRay.x264", releases[0].Title, "unexpected title")
	assert.Equal(t, int64(1500000000), releases[0].Size, "unexpected size")
	assert.Equal(t, "TestIndexer", releases[0].Indexer, "unexpected indexer")
	assert.Equal(t, "abc123", releases[0].GUID, "unexpected GUID")
	assert.Equal(t, "http://example.com/download/abc123", releases[0].DownloadURL, "unexpected download URL")
}

func TestClient_SearchWithCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cat := r.URL.Query().Get("cat")
		assert.Equal(t, "2000,2040,2045", cat, "unexpected categories")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	_, err := client.Search(context.Background(), "test", []int{2000, 2040, 2045})
	require.NoError(t, err, "Search failed")
}

func TestClient_SearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "bad-key", nil)
	_, err := client.Search(context.Background(), "test", nil)
	assert.Error(t, err, "expected error for 401 response")
}

func TestClient_Name(t *testing.T) {
	client := NewClient("MyIndexer", "http://example.com", "key", nil)
	assert.Equal(t, "MyIndexer", client.Name(), "unexpected name")
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	client := NewClient("Test", "http://example.com/", "key", nil)
	assert.Equal(t, "http://example.com", client.baseURL, "expected trailing slash trimmed")
}
