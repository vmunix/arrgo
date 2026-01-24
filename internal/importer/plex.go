// internal/importer/plex.go
package importer

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
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
	log        *slog.Logger
}

// NewPlexClient creates a new Plex client.
func NewPlexClient(baseURL, token string, log *slog.Logger) *PlexClient {
	return &PlexClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		log:     plexLogger(log),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewPlexClientWithPathMapping creates a new Plex client with path translation.
// localPath is the path on this machine, remotePath is how Plex sees it.
func NewPlexClientWithPathMapping(baseURL, token, localPath, remotePath string, log *slog.Logger) *PlexClient {
	return &PlexClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		localPath:  localPath,
		remotePath: remotePath,
		log:        plexLogger(log),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func plexLogger(log *slog.Logger) *slog.Logger {
	if log == nil {
		return nil
	}
	return log.With("component", "plex")
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

	if c.log != nil {
		c.log.Debug("scanning path", "local", filePath, "remote", remotePath)
	}

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

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scan failed with status: %d", resp.StatusCode)
	}

	if c.log != nil {
		c.log.Debug("scan triggered", "section", sectionKey, "path", remoteDir, "duration_ms", time.Since(start).Milliseconds())
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
	RatingKey string // Plex's unique identifier for the item
	Title     string
	Year      int
	Type      string // movie, show
	AddedAt   int64
	FilePath  string
}

// plexItemXML is the XML representation of a Plex item.
type plexItemXML struct {
	RatingKey string `xml:"ratingKey,attr"`
	Title     string `xml:"title,attr"`
	Year      int    `xml:"year,attr"`
	Type      string `xml:"type,attr"`
	AddedAt   int64  `xml:"addedAt,attr"`
	Media     []struct {
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
			RatingKey: item.RatingKey,
			Title:     item.Title,
			Year:      item.Year,
			Type:      item.Type,
			AddedAt:   item.AddedAt,
			FilePath:  filePath,
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
			RatingKey: item.RatingKey,
			Title:     item.Title,
			Year:      item.Year,
			Type:      item.Type,
			AddedAt:   item.AddedAt,
			FilePath:  filePath,
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

// HasContent implements MediaServer.HasContent.
// It checks if Plex has content with the given title and year.
func (c *PlexClient) HasContent(ctx context.Context, title string, year int) (bool, error) {
	return c.HasMovie(ctx, title, year)
}

// FindMovie searches for a movie in Plex with fuzzy title matching and year tolerance.
// Returns (found, ratingKey, error). The ratingKey is Plex's unique identifier.
//
// Matching strategy:
//  1. Exact title match (case-insensitive) with exact year
//  2. Exact title match with ±1 year tolerance
//  3. Fuzzy title match (Jaro-Winkler ≥ 0.85) with year in title variations
//
// This handles common mismatches:
//   - Year off by one (release year vs theatrical year)
//   - Title includes year ("Blade Runner 2049" vs "Blade Runner" + year=2049)
func (c *PlexClient) FindMovie(ctx context.Context, title string, year int) (bool, string, error) {
	items, err := c.Search(ctx, title)
	if err != nil {
		return false, "", err
	}

	// Filter to movies only
	var movies []PlexItem
	for _, item := range items {
		if item.Type == "movie" {
			movies = append(movies, item)
		}
	}

	if len(movies) == 0 {
		return false, "", nil
	}

	// Strategy 1: Exact title match with exact year
	for _, item := range movies {
		if item.Year == year && strings.EqualFold(item.Title, title) {
			return true, item.RatingKey, nil
		}
	}

	// Strategy 2: Exact title match with ±1 year tolerance
	for _, item := range movies {
		yearDiff := item.Year - year
		if yearDiff >= -1 && yearDiff <= 1 && strings.EqualFold(item.Title, title) {
			return true, item.RatingKey, nil
		}
	}

	// Strategy 3: Fuzzy title matching for year-in-title variations
	// e.g., searching for "Blade Runner" year=2049 should match "Blade Runner 2049"
	// Only applies when the Plex title contains the search year.
	normalizedSearch := normalizeForMatch(title)

	for _, item := range movies {
		// Only consider items where the Plex title contains the year we're looking for
		// This handles "Blade Runner 2049" matching search for "Blade Runner" year=2049
		if containsYear(item.Title, year) {
			// Compare the title portion (without the year) against our search title
			plexTitleWithoutYear := removeYear(item.Title, year)
			similarity := jaroWinkler(normalizedSearch, normalizeForMatch(plexTitleWithoutYear))
			if similarity >= 0.85 {
				return true, item.RatingKey, nil
			}
		}
	}

	return false, "", nil
}

// normalizeForMatch normalizes a string for fuzzy comparison.
// Lowercases, removes punctuation, collapses whitespace.
func normalizeForMatch(s string) string {
	s = strings.ToLower(s)
	// Replace common punctuation with space
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ":", " ")
	s = strings.ReplaceAll(s, ".", " ")

	// Remove other punctuation
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			b.WriteRune(r)
		}
	}

	// Collapse whitespace
	fields := strings.Fields(b.String())
	return strings.Join(fields, " ")
}

// containsYear checks if the title contains the given year.
func containsYear(title string, year int) bool {
	return strings.Contains(title, fmt.Sprintf("%d", year))
}

// removeYear removes a year from a title string.
func removeYear(title string, year int) string {
	yearStr := fmt.Sprintf("%d", year)
	result := strings.ReplaceAll(title, yearStr, "")
	// Clean up extra spaces
	fields := strings.Fields(result)
	return strings.Join(fields, " ")
}

// jaroWinkler calculates the Jaro-Winkler similarity between two strings.
// Returns a value between 0.0 (no similarity) and 1.0 (identical).
func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Calculate match window
	matchWindow := max(len1, len2)/2 - 1
	if matchWindow < 0 {
		matchWindow = 0
	}

	s1Matches := make([]bool, len1)
	s2Matches := make([]bool, len2)

	matches := 0
	transpositions := 0

	// Find matches
	for i := 0; i < len1; i++ {
		start := max(0, i-matchWindow)
		end := min(len2, i+matchWindow+1)

		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	// Count transpositions
	k := 0
	for i := 0; i < len1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	// Jaro similarity
	m := float64(matches)
	jaro := (m/float64(len1) + m/float64(len2) + (m-float64(transpositions)/2)/m) / 3

	// Winkler modification: boost for common prefix
	prefixLen := 0
	for i := 0; i < min(4, min(len1, len2)); i++ {
		if s1[i] == s2[i] {
			prefixLen++
		} else {
			break
		}
	}

	return jaro + float64(prefixLen)*0.1*(1-jaro)
}
