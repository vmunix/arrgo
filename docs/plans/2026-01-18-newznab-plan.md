# Direct Newznab Support Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace Prowlarr with direct Newznab protocol support for usenet indexers.

**Architecture:** New `pkg/newznab` client, update search module to use IndexerPool, update config format.

**Tech Stack:** Go stdlib (encoding/xml, net/http, context, sync)

---

### Task 1: Create pkg/newznab/client.go

**Files:**
- Create: `pkg/newznab/client.go`

**Step 1: Create the package and types**

```go
// Package newznab implements the Newznab usenet indexer API protocol.
package newznab

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a Newznab API client for a single indexer.
type Client struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Release represents a search result from a Newznab indexer.
type Release struct {
	Title       string
	GUID        string
	DownloadURL string
	Size        int64
	PublishDate time.Time
	Indexer     string
}

// NewClient creates a new Newznab client.
func NewClient(name, baseURL, apiKey string) *Client {
	return &Client{
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the indexer name.
func (c *Client) Name() string {
	return c.name
}
```

**Step 2: Add XML response structs**

```go
// Newznab RSS response structures
type rssResponse struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string         `xml:"title"`
	GUID      string         `xml:"guid"`
	Link      string         `xml:"link"`
	Size      int64          `xml:"size"`
	PubDate   string         `xml:"pubDate"`
	Enclosure rssEnclosure   `xml:"enclosure"`
	Attrs     []newznabAttr  `xml:"attr"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
}

type newznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}
```

**Step 3: Add Search method**

```go
// Search queries the indexer for releases.
func (c *Client) Search(ctx context.Context, query string, categories []int) ([]Release, error) {
	// Build URL
	reqURL, err := url.Parse(c.baseURL + "/api")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Build query params
	params := url.Values{}
	params.Set("apikey", c.apiKey)
	params.Set("t", "search")
	if query != "" {
		params.Set("q", query)
	}
	if len(categories) > 0 {
		cats := make([]string, len(categories))
		for i, cat := range categories {
			cats[i] = strconv.Itoa(cat)
		}
		params.Set("cat", strings.Join(cats, ","))
	}
	reqURL.RawQuery = params.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Execute
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Parse XML
	var rss rssResponse
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Convert to releases
	releases := make([]Release, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		rel := Release{
			Title:       item.Title,
			GUID:        item.GUID,
			DownloadURL: item.Link,
			Indexer:     c.name,
		}

		// Size from enclosure or item
		if item.Enclosure.Length > 0 {
			rel.Size = item.Enclosure.Length
		} else if item.Size > 0 {
			rel.Size = item.Size
		}

		// Download URL from enclosure if link is empty
		if rel.DownloadURL == "" && item.Enclosure.URL != "" {
			rel.DownloadURL = item.Enclosure.URL
		}

		// Parse publish date
		if item.PubDate != "" {
			if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
				rel.PublishDate = t
			} else if t, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", item.PubDate); err == nil {
				rel.PublishDate = t
			}
		}

		// Get size from newznab:attr if not set
		if rel.Size == 0 {
			for _, attr := range item.Attrs {
				if attr.Name == "size" {
					rel.Size, _ = strconv.ParseInt(attr.Value, 10, 64)
					break
				}
			}
		}

		releases = append(releases, rel)
	}

	return releases, nil
}
```

**Step 4: Verify**

```bash
go build ./pkg/newznab/...
```

**Step 5: Commit**

```bash
git add pkg/newznab/client.go
git commit -m "feat(newznab): add Newznab protocol client"
```

---

### Task 2: Add Newznab client tests

**Files:**
- Create: `pkg/newznab/client_test.go`

**Step 1: Create test with mock server**

```go
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
```

**Step 2: Run tests**

```bash
go test ./pkg/newznab/... -v
```

**Step 3: Commit**

```bash
git add pkg/newznab/client_test.go
git commit -m "test(newznab): add client tests"
```

---

### Task 3: Update config for Newznab indexers

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Change IndexersConfig**

Replace:
```go
type IndexersConfig struct {
	Prowlarr *ProwlarrConfig `toml:"prowlarr"`
}

type ProwlarrConfig struct {
	URL    string `toml:"url"`
	APIKey string `toml:"api_key"`
}
```

With:
```go
type IndexersConfig map[string]*NewznabConfig

type NewznabConfig struct {
	URL    string `toml:"url"`
	APIKey string `toml:"api_key"`
}
```

**Step 2: Update Validate method if needed**

Check `internal/config/validate.go` - update any Prowlarr-specific validation to work with the new indexer map.

**Step 3: Verify**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/config/config.go internal/config/validate.go
git commit -m "feat(config): change indexers to named Newznab map"
```

---

### Task 4: Create IndexerPool in search module

**Files:**
- Create: `internal/search/indexer.go`

**Step 1: Create IndexerPool**

```go
package search

import (
	"context"
	"sync"

	"github.com/arrgo/arrgo/pkg/newznab"
)

// IndexerPool manages multiple Newznab indexers and searches them in parallel.
type IndexerPool struct {
	clients []*newznab.Client
}

// NewIndexerPool creates a pool from the given clients.
func NewIndexerPool(clients []*newznab.Client) *IndexerPool {
	return &IndexerPool{clients: clients}
}

// Search queries all indexers in parallel and merges results.
func (p *IndexerPool) Search(ctx context.Context, q Query) ([]Release, []error) {
	if len(p.clients) == 0 {
		return nil, []error{ErrNoIndexers}
	}

	// Determine categories based on content type
	var categories []int
	switch q.Type {
	case "movie":
		categories = []int{2000, 2010, 2020, 2030, 2040, 2045, 2050}
	case "series":
		categories = []int{5000, 5010, 5020, 5030, 5040, 5045, 5050, 5070}
	}

	type result struct {
		releases []newznab.Release
		err      error
	}

	results := make(chan result, len(p.clients))
	var wg sync.WaitGroup

	// Query all indexers in parallel
	for _, client := range p.clients {
		wg.Add(1)
		go func(c *newznab.Client) {
			defer wg.Done()
			releases, err := c.Search(ctx, q.Text, categories)
			results <- result{releases: releases, err: err}
		}(client)
	}

	// Close results channel when all done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allReleases []Release
	var errors []error

	for r := range results {
		if r.err != nil {
			errors = append(errors, r.err)
			continue
		}
		for _, nr := range r.releases {
			allReleases = append(allReleases, Release{
				Title:       nr.Title,
				GUID:        nr.GUID,
				DownloadURL: nr.DownloadURL,
				Size:        nr.Size,
				PublishDate: nr.PublishDate,
				Indexer:     nr.Indexer,
			})
		}
	}

	return allReleases, errors
}
```

**Step 2: Add error variable**

In `internal/search/errors.go` (or create it):
```go
var ErrNoIndexers = errors.New("no indexers configured")
```

**Step 3: Verify**

```bash
go build ./internal/search/...
```

**Step 4: Commit**

```bash
git add internal/search/indexer.go internal/search/errors.go
git commit -m "feat(search): add IndexerPool for parallel indexer queries"
```

---

### Task 5: Update search.go to use IndexerPool

**Files:**
- Modify: `internal/search/search.go`

**Step 1: Update interface and Searcher**

Change:
```go
type ProwlarrAPI interface {
	Search(ctx context.Context, q Query) ([]ProwlarrRelease, error)
}

type Searcher struct {
	client ProwlarrAPI
	scorer *Scorer
}
```

To:
```go
type IndexerAPI interface {
	Search(ctx context.Context, q Query) ([]Release, []error)
}

type Searcher struct {
	indexers IndexerAPI
	scorer   *Scorer
}

func NewSearcher(indexers IndexerAPI, scorer *Scorer) *Searcher {
	return &Searcher{
		indexers: indexers,
		scorer:   scorer,
	}
}
```

**Step 2: Update Search method**

Change the Search method to use indexers instead of client, and handle the []error return.

**Step 3: Verify**

```bash
go build ./internal/search/...
```

**Step 4: Commit**

```bash
git add internal/search/search.go
git commit -m "feat(search): update Searcher to use IndexerAPI interface"
```

---

### Task 6: Delete Prowlarr client

**Files:**
- Delete: `internal/search/prowlarr.go`
- Delete: `internal/search/prowlarr_test.go`

**Step 1: Remove files**

```bash
rm internal/search/prowlarr.go internal/search/prowlarr_test.go
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add -u internal/search/
git commit -m "refactor(search): remove Prowlarr client"
```

---

### Task 7: Update serve.go to wire up IndexerPool

**Files:**
- Modify: `cmd/arrgo/serve.go`

**Step 1: Update indexer initialization**

Find where ProwlarrClient is created and replace with IndexerPool:

```go
// Before
prowlarrClient := search.NewProwlarrClient(cfg.Indexers.Prowlarr.URL, cfg.Indexers.Prowlarr.APIKey)
searcher := search.NewSearcher(prowlarrClient, scorer)

// After
var clients []*newznab.Client
for name, indexerCfg := range cfg.Indexers {
	clients = append(clients, newznab.NewClient(name, indexerCfg.URL, indexerCfg.APIKey))
}
indexerPool := search.NewIndexerPool(clients)
searcher := search.NewSearcher(indexerPool, scorer)
```

Add import: `"github.com/arrgo/arrgo/pkg/newznab"`

**Step 2: Verify**

```bash
go build ./cmd/arrgo/...
```

**Step 3: Commit**

```bash
git add cmd/arrgo/serve.go
git commit -m "feat(serve): wire up IndexerPool instead of Prowlarr"
```

---

### Task 8: Update init wizard for Newznab

**Files:**
- Modify: `cmd/arrgo/init.go`

**Step 1: Update prompts**

Change Prowlarr prompts to Newznab:
```go
cfg.IndexerName = promptWithDefault(reader, "Indexer Name", "nzbgeek")
cfg.IndexerURL = promptWithDefault(reader, "Indexer URL", "https://api.nzbgeek.info")
cfg.IndexerAPIKey = promptRequired(reader, "Indexer API Key")
```

**Step 2: Update initConfig struct**

```go
type initConfig struct {
	IndexerName   string
	IndexerURL    string
	IndexerAPIKey string
	// ... rest unchanged
}
```

**Step 3: Update config template**

Change `[indexers.prowlarr]` to use the indexer name:
```go
[indexers.{{.IndexerName}}]
url = "{{.IndexerURL}}"
api_key = "{{.IndexerAPIKey}}"
```

**Step 4: Verify**

```bash
go build ./cmd/arrgo/...
```

**Step 5: Commit**

```bash
git add cmd/arrgo/init.go
git commit -m "feat(init): update wizard for direct Newznab indexers"
```

---

### Task 9: Update tests and verify full flow

**Step 1: Update any broken tests**

Check and fix:
- `internal/search/search_test.go`
- `internal/api/v1/api_test.go`
- `internal/api/v1/integration_test.go`

**Step 2: Run all tests**

```bash
go test ./...
```

**Step 3: Run linter**

```bash
golangci-lint run ./...
```

**Step 4: Manual test**

```bash
rm config.toml
./arrgo init  # Enter NZBgeek details
./arrgo serve &
./arrgo search "ninja scroll"
```

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: update tests for Newznab refactor"
```
