// internal/importer/plex.go
package importer

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// PlexClient interacts with the Plex Media Server API.
type PlexClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewPlexClient creates a new Plex client.
func NewPlexClient(baseURL, token string) *PlexClient {
	return &PlexClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Identity holds Plex server identity information.
type Identity struct {
	Name    string
	Version string
}

// identityResponse is the XML response from root endpoint.
type identityResponse struct {
	XMLName      xml.Name `xml:"MediaContainer"`
	FriendlyName string   `xml:"friendlyName,attr"`
	Version      string   `xml:"version,attr"`
}

// Section represents a Plex library section.
type Section struct {
	Key           string     `xml:"key,attr"`
	Title         string     `xml:"title,attr"`
	Type          string     `xml:"type,attr"`
	Locations     []Location `xml:"Location"`
	ScannedAt     int64      `xml:"scannedAt,attr"`
	RefreshingRaw int        `xml:"refreshing,attr"`
}

// Refreshing returns true if the section is currently being scanned.
func (s Section) Refreshing() bool {
	return s.RefreshingRaw == 1
}

// Location represents a library section's filesystem location.
type Location struct {
	Path string `xml:"path,attr"`
}

// sectionsResponse is the XML response from /library/sections.
type sectionsResponse struct {
	XMLName  xml.Name  `xml:"MediaContainer"`
	Sections []Section `xml:"Directory"`
}

// GetSections returns all library sections.
func (c *PlexClient) GetSections(ctx context.Context) ([]Section, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/library/sections", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result sectionsResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Sections, nil
}

// ScanPath triggers a partial scan of the directory containing the given file path.
func (c *PlexClient) ScanPath(ctx context.Context, filePath string) error {
	// Get directory containing the file
	dir := filepath.Dir(filePath)

	// Find the section that contains this path
	sections, err := c.GetSections(ctx)
	if err != nil {
		return fmt.Errorf("get sections: %w", err)
	}

	var sectionKey string
	for _, section := range sections {
		for _, loc := range section.Locations {
			if strings.HasPrefix(dir, loc.Path) || strings.HasPrefix(filePath, loc.Path) {
				sectionKey = section.Key
				break
			}
		}
		if sectionKey != "" {
			break
		}
	}

	if sectionKey == "" {
		return fmt.Errorf("no library section found for path: %s", filePath)
	}

	// Trigger partial scan
	scanURL := fmt.Sprintf("%s/library/sections/%s/refresh?path=%s",
		c.baseURL, sectionKey, url.QueryEscape(dir))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scanURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scan failed with status: %d", resp.StatusCode)
	}

	return nil
}

// GetIdentity returns the Plex server name and version.
func (c *PlexClient) GetIdentity(ctx context.Context) (*Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result identityResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &Identity{
		Name:    result.FriendlyName,
		Version: result.Version,
	}, nil
}

// countResponse is the XML response for getting library item count.
type countResponse struct {
	XMLName xml.Name `xml:"MediaContainer"`
	Size    int      `xml:"size,attr"`
}

// RefreshLibrary triggers a full scan of a library section.
func (c *PlexClient) RefreshLibrary(ctx context.Context, sectionKey string) error {
	scanURL := fmt.Sprintf("%s/library/sections/%s/refresh", c.baseURL, sectionKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scanURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed with status: %d", resp.StatusCode)
	}

	return nil
}

// GetLibraryCount returns the number of items in a library section.
func (c *PlexClient) GetLibraryCount(ctx context.Context, sectionKey string) (int, error) {
	// Use X-Plex-Container-Size=0 to get just the count without items
	reqURL := fmt.Sprintf("%s/library/sections/%s/all?X-Plex-Container-Size=0", c.baseURL, sectionKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result countResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	return result.Size, nil
}
