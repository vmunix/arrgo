package newznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestSearch_MalformedXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel><item><title>broken`))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	_, err := client.Search(context.Background(), "test", nil)
	require.Error(t, err, "expected error for malformed XML")
	assert.Contains(t, err.Error(), "parse response", "error should mention parsing")
}

func TestSearch_EmptyResponse(t *testing.T) {
	const emptyXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(emptyXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "no results", nil)
	require.NoError(t, err, "empty response should not error")
	assert.Empty(t, releases, "expected empty results")
}

func TestSearch_ErrorResponse(t *testing.T) {
	// Newznab API error responses have a different root element (<error> instead of <rss>).
	// The XML parser returns an error because it expects <rss>.
	const errorXML = `<?xml version="1.0" encoding="UTF-8"?>
<error code="100" description="Incorrect user credentials"/>
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(errorXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	_, err := client.Search(context.Background(), "test", nil)
	// Current implementation: XML parser fails because root element is <error> not <rss>
	require.Error(t, err, "error response should cause parsing error")
	assert.Contains(t, err.Error(), "parse response", "error should indicate parsing issue")
}

func TestSearch_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	_, err := client.Search(context.Background(), "test", nil)
	require.Error(t, err, "expected error for 500 response")
	assert.Contains(t, err.Error(), "500", "error should contain status code")
}

func TestSearch_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than client timeout
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	// Create client with short timeout
	client := NewClient("Test", server.URL, "key", nil)
	client.httpClient.Timeout = 50 * time.Millisecond

	_, err := client.Search(context.Background(), "test", nil)
	require.Error(t, err, "expected timeout error")
	assert.Contains(t, err.Error(), "request failed", "error should indicate request failure")
}

func TestSearch_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay to allow context cancellation
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	_, err := client.Search(ctx, "test", nil)
	require.Error(t, err, "expected error for canceled context")
	assert.Contains(t, err.Error(), "request failed", "error should indicate request failure")
}

func TestSearch_MissingFields(t *testing.T) {
	// XML with minimal fields - missing size, pubDate, enclosure
	const minimalXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Minimal.Release.2024</title>
      <guid>minimal123</guid>
      <link>http://example.com/download/minimal123</link>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(minimalXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "test", nil)
	require.NoError(t, err, "missing fields should not cause error")
	require.Len(t, releases, 1, "expected 1 release")

	rel := releases[0]
	assert.Equal(t, "Minimal.Release.2024", rel.Title, "title should be set")
	assert.Equal(t, "minimal123", rel.GUID, "GUID should be set")
	assert.Equal(t, "http://example.com/download/minimal123", rel.DownloadURL, "download URL should be set")
	assert.Equal(t, int64(0), rel.Size, "size should default to 0")
	assert.True(t, rel.PublishDate.IsZero(), "publish date should be zero value")
}

func TestSearch_InvalidDate(t *testing.T) {
	// XML with unparseable date format
	const invalidDateXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Release.With.Bad.Date.2024</title>
      <guid>baddate123</guid>
      <link>http://example.com/download/baddate123</link>
      <pubDate>not-a-valid-date-format</pubDate>
      <enclosure url="http://example.com/download/baddate123" length="500000000" type="application/x-nzb" />
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(invalidDateXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "test", nil)
	require.NoError(t, err, "invalid date should not cause error")
	require.Len(t, releases, 1, "expected 1 release")

	rel := releases[0]
	assert.Equal(t, "Release.With.Bad.Date.2024", rel.Title, "title should be set")
	assert.True(t, rel.PublishDate.IsZero(), "invalid date should result in zero time")
	assert.Equal(t, int64(500000000), rel.Size, "size should still be parsed")
}

func TestCaps_Success(t *testing.T) {
	const capsXML = `<?xml version="1.0" encoding="UTF-8"?>
<caps>
  <server version="0.1" title="Test Indexer"/>
  <limits max="100" default="100"/>
  <searching>
    <search available="yes"/>
  </searching>
  <categories>
    <category id="2000" name="Movies"/>
  </categories>
</caps>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "caps", r.URL.Query().Get("t"), "expected caps request type")
		assert.Equal(t, "test-key", r.URL.Query().Get("apikey"), "expected apikey")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(capsXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "test-key", nil)
	err := client.Caps(context.Background())
	require.NoError(t, err, "Caps should succeed")
}

func TestCaps_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Service Unavailable"))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	err := client.Caps(context.Background())
	require.Error(t, err, "expected error for 503 response")
	assert.Contains(t, err.Error(), "503", "error should contain status code")
}

func TestClient_URL(t *testing.T) {
	client := NewClient("TestIndexer", "http://example.com/api", "key", nil)
	assert.Equal(t, "http://example.com/api", client.URL(), "URL should return base URL")
}

func TestSearchWithOffset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "50", r.URL.Query().Get("limit"), "expected limit param")
		assert.Equal(t, "25", r.URL.Query().Get("offset"), "expected offset param")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.SearchWithOffset(context.Background(), "test", nil, 50, 25)
	require.NoError(t, err, "SearchWithOffset should succeed")
	require.Len(t, releases, 2, "expected 2 releases")
}

func TestSearch_SizeFromNewznabAttr(t *testing.T) {
	// XML where size comes from newznab:attr instead of enclosure
	const sizeAttrXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Release.With.Attr.Size.2024</title>
      <guid>attrsize123</guid>
      <link>http://example.com/download/attrsize123</link>
      <newznab:attr name="size" value="2000000000" />
      <newznab:attr name="category" value="2040" />
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(sizeAttrXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "test", nil)
	require.NoError(t, err, "should handle size from newznab attr")
	require.Len(t, releases, 1, "expected 1 release")
	assert.Equal(t, int64(2000000000), releases[0].Size, "size should be parsed from newznab:attr")
}

func TestSearch_DownloadURLFromEnclosure(t *testing.T) {
	// XML where download URL comes from enclosure when link is empty
	const enclosureURLXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Release.Enclosure.URL.2024</title>
      <guid>encurl123</guid>
      <enclosure url="http://example.com/nzb/encurl123.nzb" length="1000000000" type="application/x-nzb" />
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(enclosureURLXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "test", nil)
	require.NoError(t, err, "should handle URL from enclosure")
	require.Len(t, releases, 1, "expected 1 release")
	assert.Equal(t, "http://example.com/nzb/encurl123.nzb", releases[0].DownloadURL, "URL should come from enclosure")
}

func TestSearch_ItemSize(t *testing.T) {
	// XML where size comes from item element directly
	const itemSizeXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Release.Item.Size.2024</title>
      <guid>itemsize123</guid>
      <link>http://example.com/download/itemsize123</link>
      <size>3000000000</size>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(itemSizeXML))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "test", nil)
	require.NoError(t, err, "should handle size from item element")
	require.Len(t, releases, 1, "expected 1 release")
	assert.Equal(t, int64(3000000000), releases[0].Size, "size should be parsed from item element")
}

func TestSearch_AlternateDateFormats(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		wantZero bool
	}{
		{"RFC1123Z", "Mon, 18 Jan 2026 12:00:00 -0700", false},
		{"RFC1123", "Mon, 18 Jan 2026 12:00:00 UTC", false},
		{"RFC1123 with MST", "Mon, 18 Jan 2026 12:00:00 EST", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Date.Test.Release</title>
      <guid>date123</guid>
      <link>http://example.com/download/date123</link>
      <pubDate>` + tt.dateStr + `</pubDate>
    </item>
  </channel>
</rss>`

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(xmlResp))
			}))
			defer server.Close()

			client := NewClient("Test", server.URL, "key", nil)
			releases, err := client.Search(context.Background(), "test", nil)
			require.NoError(t, err, "date parsing should not error")
			require.Len(t, releases, 1, "expected 1 release")

			if tt.wantZero {
				assert.True(t, releases[0].PublishDate.IsZero(), "expected zero time for %s", tt.name)
			} else {
				assert.False(t, releases[0].PublishDate.IsZero(), "expected non-zero time for %s", tt.name)
			}
		})
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that empty query doesn't add q param
		assert.Empty(t, r.URL.Query().Get("q"), "empty query should not add q param")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), "", []int{2000})
	require.NoError(t, err, "empty query search should succeed")
	assert.Len(t, releases, 2, "expected 2 releases")
}
