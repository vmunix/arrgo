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
	remotePath string // Path prefix as seen by Plex
	localPath  string // Corresponding local path
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

// NewPlexClientWithPathMapping creates a new Plex client with path translation.
// localPath is the path on this machine, remotePath is how Plex sees it.
func NewPlexClientWithPathMapping(baseURL, token, localPath, remotePath string) *PlexClient {
	return &PlexClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		localPath:  localPath,
		remotePath: remotePath,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// translateToRemote converts a local path to the path Plex expects.
func (c *PlexClient) translateToRemote(path string) string {
	if c.localPath == "" || c.remotePath == "" {
		return path
	}
	if strings.HasPrefix(path, c.localPath) {
		return c.remotePath + path[len(c.localPath):]
	}
	return path
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

// FindSectionByName finds a library section by name (case-insensitive).
// Returns nil if not found.
func (c *PlexClient) FindSectionByName(ctx context.Context, name string) (*Section, error) {
	sections, err := c.GetSections(ctx)
	if err != nil {
		return nil, err
	}

	for _, sec := range sections {
		if strings.EqualFold(sec.Title, name) {
			return &sec, nil
		}
	}

	return nil, nil
}

// ScanPath triggers a partial scan of the directory containing the given file path.
func (c *PlexClient) ScanPath(ctx context.Context, filePath string) error {
	// Translate local path to Plex's path (for Docker path mapping)
	remotePath := c.translateToRemote(filePath)
	remoteDir := filepath.Dir(remotePath)

	// Find the section that contains this path
	sections, err := c.GetSections(ctx)
	if err != nil {
		return fmt.Errorf("get sections: %w", err)
	}

	var sectionKey string
	for _, section := range sections {
		for _, loc := range section.Locations {
			if strings.HasPrefix(remoteDir, loc.Path) || strings.HasPrefix(remotePath, loc.Path) {
				sectionKey = section.Key
				break
			}
		}
		if sectionKey != "" {
			break
		}
	}

	if sectionKey == "" {
		return fmt.Errorf("no library section found for path: %s (translated: %s)", filePath, remotePath)
	}

	// Trigger partial scan using the remote path
	scanURL := fmt.Sprintf("%s/library/sections/%s/refresh?path=%s",
		c.baseURL, sectionKey, url.QueryEscape(remoteDir))

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

// PlexItem represents a media item in Plex.
type PlexItem struct {
	Title    string
	Year     int
	Type     string // movie, show
	AddedAt  int64
	FilePath string
}

// plexItemXML is the XML representation of a Plex item.
type plexItemXML struct {
	Title   string `xml:"title,attr"`
	Year    int    `xml:"year,attr"`
	Type    string `xml:"type,attr"`
	AddedAt int64  `xml:"addedAt,attr"`
	Media   []struct {
		Part []struct {
			File string `xml:"file,attr"`
		} `xml:"Part"`
	} `xml:"Media"`
}

// libraryItemsResponse is the XML response from /library/sections/{key}/all.
type libraryItemsResponse struct {
	XMLName xml.Name      `xml:"MediaContainer"`
	Items   []plexItemXML `xml:"Video"`
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

// ListLibraryItems returns all items in a library section.
func (c *PlexClient) ListLibraryItems(ctx context.Context, sectionKey string) ([]PlexItem, error) {
	reqURL := fmt.Sprintf("%s/library/sections/%s/all", c.baseURL, sectionKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
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

	var result libraryItemsResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	items := make([]PlexItem, len(result.Items))
	for i, item := range result.Items {
		filePath := ""
		if len(item.Media) > 0 && len(item.Media[0].Part) > 0 {
			filePath = item.Media[0].Part[0].File
		}
		items[i] = PlexItem{
			Title:    item.Title,
			Year:     item.Year,
			Type:     item.Type,
			AddedAt:  item.AddedAt,
			FilePath: filePath,
		}
	}

	return items, nil
}

// searchResponse is the XML response from /search.
type searchResponse struct {
	XMLName xml.Name      `xml:"MediaContainer"`
	Items   []plexItemXML `xml:"Video"`
}

// Search searches for items across all libraries.
func (c *PlexClient) Search(ctx context.Context, query string) ([]PlexItem, error) {
	reqURL := fmt.Sprintf("%s/search?query=%s", c.baseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
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

	var result searchResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	items := make([]PlexItem, len(result.Items))
	for i, item := range result.Items {
		filePath := ""
		if len(item.Media) > 0 && len(item.Media[0].Part) > 0 {
			filePath = item.Media[0].Part[0].File
		}
		items[i] = PlexItem{
			Title:    item.Title,
			Year:     item.Year,
			Type:     item.Type,
			AddedAt:  item.AddedAt,
			FilePath: filePath,
		}
	}

	return items, nil
}

// HasMovie checks if Plex has a movie with the given title and year.
func (c *PlexClient) HasMovie(ctx context.Context, title string, year int) (bool, error) {
	items, err := c.Search(ctx, title)
	if err != nil {
		return false, err
	}

	for _, item := range items {
		if item.Type == "movie" && item.Year == year && strings.EqualFold(item.Title, title) {
			return true, nil
		}
	}

	return false, nil
}
