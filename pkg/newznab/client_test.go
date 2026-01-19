package newznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
		if r.URL.Path != "/api" {
			t.Errorf("expected path /api, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("apikey") != "test-key" {
			t.Errorf("expected apikey test-key, got %s", r.URL.Query().Get("apikey"))
		}
		if r.URL.Query().Get("q") != "test query" {
			t.Errorf("expected q 'test query', got %s", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("t") != "search" {
			t.Errorf("expected t search, got %s", r.URL.Query().Get("t"))
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	// Create client
	client := NewClient("TestIndexer", server.URL, "test-key")

	// Search
	releases, err := client.Search(context.Background(), "test query", []int{2000})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify results
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	// Check first release
	if releases[0].Title != "Test.Release.2024.1080p.BluRay.x264" {
		t.Errorf("unexpected title: %s", releases[0].Title)
	}
	if releases[0].Size != 1500000000 {
		t.Errorf("unexpected size: %d", releases[0].Size)
	}
	if releases[0].Indexer != "TestIndexer" {
		t.Errorf("unexpected indexer: %s", releases[0].Indexer)
	}
	if releases[0].GUID != "abc123" {
		t.Errorf("unexpected GUID: %s", releases[0].GUID)
	}
	if releases[0].DownloadURL != "http://example.com/download/abc123" {
		t.Errorf("unexpected download URL: %s", releases[0].DownloadURL)
	}
}

func TestClient_SearchWithCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cat := r.URL.Query().Get("cat")
		if cat != "2000,2040,2045" {
			t.Errorf("expected categories '2000,2040,2045', got %s", cat)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(testXMLResponse))
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "key")
	_, err := client.Search(context.Background(), "test", []int{2000, 2040, 2045})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
}

func TestClient_SearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient("Test", server.URL, "bad-key")
	_, err := client.Search(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestClient_Name(t *testing.T) {
	client := NewClient("MyIndexer", "http://example.com", "key")
	if client.Name() != "MyIndexer" {
		t.Errorf("expected name MyIndexer, got %s", client.Name())
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	client := NewClient("Test", "http://example.com/", "key")
	if client.baseURL != "http://example.com" {
		t.Errorf("expected trailing slash trimmed, got %s", client.baseURL)
	}
}
