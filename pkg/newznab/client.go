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

// Newznab RSS response structures
type rssResponse struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string        `xml:"title"`
	GUID      string        `xml:"guid"`
	Link      string        `xml:"link"`
	Size      int64         `xml:"size"`
	PubDate   string        `xml:"pubDate"`
	Enclosure rssEnclosure  `xml:"enclosure"`
	Attrs     []newznabAttr `xml:"http://www.newznab.com/DTD/2010/feeds/attributes/ attr"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
}

type newznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Search queries the indexer for releases.
func (c *Client) Search(ctx context.Context, query string, categories []int) ([]Release, error) {
	return c.SearchWithOffset(ctx, query, categories, 100, 0)
}

// SearchWithOffset queries the indexer with pagination support.
func (c *Client) SearchWithOffset(ctx context.Context, query string, categories []int, limit, offset int) ([]Release, error) {
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
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
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

		// Parse publish date (try multiple formats)
		if item.PubDate != "" {
			for _, format := range []string{
				time.RFC1123Z,
				"Mon, 02 Jan 2006 15:04:05 -0700",
				"Mon, 02 Jan 2006 15:04:05 MST",
				time.RFC1123,
			} {
				if t, err := time.Parse(format, item.PubDate); err == nil {
					rel.PublishDate = t
					break
				}
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
