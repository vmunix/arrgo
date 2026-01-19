# Plex List and Search Commands Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add CLI commands to list and search Plex library contents with arrgo tracking status.

**Architecture:** Add PlexClient methods for listing/searching, API endpoints to expose them, CLI commands for user interaction. Match Plex items to arrgo content by title+year. Case-insensitive library name matching across all plex commands.

**Tech Stack:** Go, Plex XML API, existing PlexClient pattern

---

### Task 1: Add PlexItem type and ListLibraryItems to PlexClient

**Files:**
- Modify: `internal/importer/plex.go`

**Step 1: Add the PlexItem type and XML response types**

Add after the `countResponse` type (around line 184):

```go
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
```

**Step 2: Add ListLibraryItems method**

Add after `GetLibraryCount`:

```go
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
```

**Step 3: Run tests**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add internal/importer/plex.go
git commit -m "feat(plex): add ListLibraryItems method to PlexClient"
```

---

### Task 2: Add Search method to PlexClient

**Files:**
- Modify: `internal/importer/plex.go`

**Step 1: Add search response type and Search method**

Add after `ListLibraryItems`:

```go
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
```

**Step 2: Run tests**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/importer/plex.go
git commit -m "feat(plex): add Search method to PlexClient"
```

---

### Task 3: Add case-insensitive library lookup helper

**Files:**
- Modify: `internal/importer/plex.go`

**Step 1: Add FindSectionByName method**

Add after `GetSections`:

```go
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
```

**Step 2: Run tests**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/importer/plex.go
git commit -m "feat(plex): add case-insensitive library lookup"
```

---

### Task 4: Add API endpoints for plex list and search

**Files:**
- Modify: `internal/api/v1/types.go`
- Modify: `internal/api/v1/api.go`

**Step 1: Add request/response types to types.go**

Add at end of file:

```go
// plexItemResponse is a Plex library item with tracking status.
type plexItemResponse struct {
	Title    string `json:"title"`
	Year     int    `json:"year"`
	Type     string `json:"type"`
	AddedAt  int64  `json:"added_at"`
	FilePath string `json:"file_path,omitempty"`
	Tracked  bool   `json:"tracked"`
	ContentID *int64 `json:"content_id,omitempty"`
}

// plexListResponse is the response for GET /plex/libraries/{name}/items.
type plexListResponse struct {
	Library string             `json:"library"`
	Items   []plexItemResponse `json:"items"`
	Total   int                `json:"total"`
}

// plexSearchResponse is the response for GET /plex/search.
type plexSearchResponse struct {
	Query string             `json:"query"`
	Items []plexItemResponse `json:"items"`
	Total int                `json:"total"`
}
```

**Step 2: Register routes in api.go RegisterRoutes**

Add after `POST /api/v1/plex/scan`:

```go
	mux.HandleFunc("GET /api/v1/plex/libraries/{name}/items", s.listPlexLibraryItems)
	mux.HandleFunc("GET /api/v1/plex/search", s.searchPlex)
```

**Step 3: Add listPlexLibraryItems handler**

Add after `scanPlexLibraries`:

```go
func (s *Server) listPlexLibraryItems(w http.ResponseWriter, r *http.Request) {
	if s.plex == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Plex not configured")
		return
	}

	name := r.PathValue("name")
	ctx := r.Context()

	// Find section (case-insensitive)
	section, err := s.plex.FindSectionByName(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}
	if section == nil {
		sections, _ := s.plex.GetSections(ctx)
		var available []string
		for _, sec := range sections {
			available = append(available, sec.Title)
		}
		writeError(w, http.StatusNotFound, "LIBRARY_NOT_FOUND",
			fmt.Sprintf("library %q not found, available: %v", name, available))
		return
	}

	// Get items
	items, err := s.plex.ListLibraryItems(ctx, section.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	// Build response with tracking status
	resp := plexListResponse{
		Library: section.Title,
		Items:   make([]plexItemResponse, len(items)),
		Total:   len(items),
	}

	for i, item := range items {
		resp.Items[i] = plexItemResponse{
			Title:    item.Title,
			Year:     item.Year,
			Type:     item.Type,
			AddedAt:  item.AddedAt,
			FilePath: item.FilePath,
		}

		// Check if tracked in arrgo
		content, _ := s.library.GetByTitleYear(item.Title, item.Year)
		if content != nil {
			resp.Items[i].Tracked = true
			resp.Items[i].ContentID = &content.ID
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Add searchPlex handler**

Add after `listPlexLibraryItems`:

```go
func (s *Server) searchPlex(w http.ResponseWriter, r *http.Request) {
	if s.plex == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Plex not configured")
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUERY", "query parameter is required")
		return
	}

	ctx := r.Context()

	items, err := s.plex.Search(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	resp := plexSearchResponse{
		Query: query,
		Items: make([]plexItemResponse, len(items)),
		Total: len(items),
	}

	for i, item := range items {
		resp.Items[i] = plexItemResponse{
			Title:    item.Title,
			Year:     item.Year,
			Type:     item.Type,
			AddedAt:  item.AddedAt,
			FilePath: item.FilePath,
		}

		// Check if tracked in arrgo
		content, _ := s.library.GetByTitleYear(item.Title, item.Year)
		if content != nil {
			resp.Items[i].Tracked = true
			resp.Items[i].ContentID = &content.ID
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 5: Run tests**

Run: `go build ./...`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add internal/api/v1/types.go internal/api/v1/api.go
git commit -m "feat(api): add plex list and search endpoints"
```

---

### Task 5: Add CLI client methods

**Files:**
- Modify: `cmd/arrgo/client.go`

**Step 1: Add response types and methods**

Add at end of file:

```go
type PlexItemResponse struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	AddedAt   int64  `json:"added_at"`
	FilePath  string `json:"file_path,omitempty"`
	Tracked   bool   `json:"tracked"`
	ContentID *int64 `json:"content_id,omitempty"`
}

type PlexListResponse struct {
	Library string             `json:"library"`
	Items   []PlexItemResponse `json:"items"`
	Total   int                `json:"total"`
}

type PlexSearchResponse struct {
	Query string             `json:"query"`
	Items []PlexItemResponse `json:"items"`
	Total int                `json:"total"`
}

func (c *Client) PlexListLibrary(library string) (*PlexListResponse, error) {
	var resp PlexListResponse
	if err := c.get("/api/v1/plex/libraries/"+url.PathEscape(library)+"/items", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) PlexSearch(query string) (*PlexSearchResponse, error) {
	var resp PlexSearchResponse
	if err := c.get("/api/v1/plex/search?query="+url.QueryEscape(query), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

**Step 2: Run tests**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add cmd/arrgo/client.go
git commit -m "feat(cli): add PlexListLibrary and PlexSearch client methods"
```

---

### Task 6: Add plex list CLI command

**Files:**
- Modify: `cmd/arrgo/plex.go`

**Step 1: Add list command and flags**

Add after `plexScanCmd` variable:

```go
var plexListCmd = &cobra.Command{
	Use:   "list [library]",
	Short: "List Plex library contents",
	Long:  "List items in a Plex library with arrgo tracking status. If no library specified, lists all libraries.",
	RunE:  runPlexListCmd,
}

var plexListVerbose bool
```

**Step 2: Register command in init()**

Update `init()` to add:

```go
	plexCmd.AddCommand(plexListCmd)
	plexListCmd.Flags().BoolVarP(&plexListVerbose, "verbose", "v", false, "Show detailed output")
```

**Step 3: Add runPlexListCmd function**

Add after `runPlexScanCmd`:

```go
func runPlexListCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	// If no library specified, show all libraries with counts
	if len(args) == 0 {
		status, err := client.PlexStatus()
		if err != nil {
			return fmt.Errorf("plex status failed: %w", err)
		}

		if jsonOutput {
			printJSON(status.Libraries)
			return nil
		}

		fmt.Println("Libraries:")
		for _, lib := range status.Libraries {
			fmt.Printf("  %-12s %4d items\n", lib.Title, lib.ItemCount)
		}
		fmt.Println("\nUse 'arrgo plex list <library>' to see contents")
		return nil
	}

	// List specific library
	library := args[0]
	resp, err := client.PlexListLibrary(library)
	if err != nil {
		return fmt.Errorf("plex list failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	fmt.Printf("%s (%d items):\n", resp.Library, resp.Total)
	for _, item := range resp.Items {
		status := "✗ not tracked"
		if item.Tracked {
			status = "✓ tracked"
		}

		if plexListVerbose {
			fmt.Printf("\n  %s (%d)\n", item.Title, item.Year)
			fmt.Printf("    Path: %s\n", item.FilePath)
			fmt.Printf("    Added: %s | %s\n", formatTimeAgo(item.AddedAt), status)
			if item.ContentID != nil {
				fmt.Printf("    Content ID: %d\n", *item.ContentID)
			}
		} else {
			fmt.Printf("  %-45s %s\n", fmt.Sprintf("%s (%d)", item.Title, item.Year), status)
		}
	}

	return nil
}
```

**Step 4: Run tests**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add cmd/arrgo/plex.go
git commit -m "feat(cli): add plex list command"
```

---

### Task 7: Add plex search CLI command

**Files:**
- Modify: `cmd/arrgo/plex.go`

**Step 1: Add search command**

Add after `plexListCmd`:

```go
var plexSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search Plex libraries",
	Long:  "Search for items across all Plex libraries.",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlexSearchCmd,
}
```

**Step 2: Register command in init()**

Add to `init()`:

```go
	plexCmd.AddCommand(plexSearchCmd)
```

**Step 3: Add runPlexSearchCmd function**

Add after `runPlexListCmd`:

```go
func runPlexSearchCmd(cmd *cobra.Command, args []string) error {
	query := args[0]
	client := NewClient(serverURL)

	resp, err := client.PlexSearch(query)
	if err != nil {
		return fmt.Errorf("plex search failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	if resp.Total == 0 {
		fmt.Printf("No results for %q\n", query)
		return nil
	}

	fmt.Printf("Results for %q (%d items):\n", query, resp.Total)
	for _, item := range resp.Items {
		status := "✗ not tracked"
		if item.Tracked {
			status = "✓ tracked"
		}

		fmt.Printf("\n  %s (%d)\n", item.Title, item.Year)
		if item.FilePath != "" {
			fmt.Printf("    Path: %s\n", item.FilePath)
		}
		fmt.Printf("    Added: %s | %s\n", formatTimeAgo(item.AddedAt), status)
		if item.ContentID != nil {
			fmt.Printf("    Content ID: %d\n", *item.ContentID)
		}
	}

	return nil
}
```

**Step 4: Run tests**

Run: `go build ./... && task test`
Expected: Build and tests succeed

**Step 5: Commit**

```bash
git add cmd/arrgo/plex.go
git commit -m "feat(cli): add plex search command"
```

---

### Task 8: Update plex scan to use case-insensitive matching

**Files:**
- Modify: `internal/api/v1/api.go`

**Step 1: Update scanPlexLibraries to use FindSectionByName**

Replace the library validation loop in `scanPlexLibraries` (the section that validates requested libraries) with:

```go
	// Determine which libraries to scan
	var toScan []struct {
		name string
		key  string
	}
	if len(req.Libraries) == 0 {
		// Scan all
		for _, sec := range sections {
			toScan = append(toScan, struct {
				name string
				key  string
			}{sec.Title, sec.Key})
		}
	} else {
		// Validate and find requested libraries (case-insensitive)
		for _, name := range req.Libraries {
			var found *importer.Section
			for i := range sections {
				if strings.EqualFold(sections[i].Title, name) {
					found = &sections[i]
					break
				}
			}
			if found == nil {
				var available []string
				for _, sec := range sections {
					available = append(available, sec.Title)
				}
				writeError(w, http.StatusBadRequest, "LIBRARY_NOT_FOUND",
					fmt.Sprintf("library %q not found, available: %v", name, available))
				return
			}
			toScan = append(toScan, struct {
				name string
				key  string
			}{found.Title, found.Key})
		}
	}

	// Trigger scans
	var scanned []string
	for _, lib := range toScan {
		if err := s.plex.RefreshLibrary(ctx, lib.key); err != nil {
			writeError(w, http.StatusInternalServerError, "SCAN_ERROR",
				fmt.Sprintf("failed to scan %q: %v", lib.name, err))
			return
		}
		scanned = append(scanned, lib.name)
	}
```

**Step 2: Add strings import if needed**

Ensure `"strings"` is in the imports.

**Step 3: Run tests**

Run: `go build ./... && task test`
Expected: Build and tests succeed

**Step 4: Commit**

```bash
git add internal/api/v1/api.go
git commit -m "fix(api): make plex scan library names case-insensitive"
```

---

### Task 9: Manual verification

**Step 1: Build and restart server**

```bash
task build
pkill -f arrgod; sleep 1; ./arrgod serve > /tmp/arrgod.log 2>&1 &
sleep 2
```

**Step 2: Test plex list**

```bash
./arrgo plex list
./arrgo plex list movies
./arrgo plex list Movies --verbose
```

Expected: Shows library contents with tracking status

**Step 3: Test plex search**

```bash
./arrgo plex search "Back to the Future"
```

Expected: Shows BTTF movies with tracking status

**Step 4: Test case-insensitive scan**

```bash
./arrgo plex scan movies
```

Expected: Scans Movies library successfully
