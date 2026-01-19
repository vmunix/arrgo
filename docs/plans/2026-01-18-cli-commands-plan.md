# CLI Commands Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement status, search, and queue CLI commands as HTTP clients to the running arrgo server.

**Architecture:** Thin HTTP client layer (`client.go`) with command implementations (`commands.go`). Commands call API endpoints and format output for terminal or JSON.

**Tech Stack:** Go stdlib (flag, net/http, encoding/json, fmt, text/tabwriter)

---

### Task 1: Create HTTP Client

**Files:**
- Create: `cmd/arrgo/client.go`

**Step 1: Create client.go with base client**

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP calls to the arrgo server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new arrgo API client.
func NewClient(serverURL string) *Client {
	return &Client{
		baseURL: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) get(path string, result any) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) post(path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: Compiles (client not used yet)

**Step 3: Commit**

```bash
git add cmd/arrgo/client.go
git commit -m "feat(cli): add HTTP client wrapper"
```

---

### Task 2: Add Client API Methods

**Files:**
- Modify: `cmd/arrgo/client.go`

**Step 1: Add response types and API methods**

Add after the `post` method:

```go
// API response types (mirror server types)

type StatusResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type DownloadResponse struct {
	ID          int64   `json:"id"`
	ContentID   int64   `json:"content_id"`
	EpisodeID   *int64  `json:"episode_id,omitempty"`
	Client      string  `json:"client"`
	ClientID    string  `json:"client_id"`
	Status      string  `json:"status"`
	ReleaseName string  `json:"release_name"`
	Indexer     string  `json:"indexer"`
	AddedAt     string  `json:"added_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

type ListDownloadsResponse struct {
	Items []DownloadResponse `json:"items"`
	Total int                `json:"total"`
}

type ReleaseResponse struct {
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
	GUID        string `json:"guid"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	PublishDate string `json:"publish_date"`
	Quality     string `json:"quality"`
	Score       int    `json:"score"`
}

type SearchResponse struct {
	Releases []ReleaseResponse `json:"releases"`
	Errors   []string          `json:"errors,omitempty"`
}

type ContentResponse struct {
	ID             int64  `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	Status         string `json:"status"`
	QualityProfile string `json:"quality_profile"`
	RootPath       string `json:"root_path"`
}

type GrabResponse struct {
	DownloadID int64  `json:"download_id"`
	Status     string `json:"status"`
}

// API methods

func (c *Client) Status() (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.get("/api/v1/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Downloads(activeOnly bool) (*ListDownloadsResponse, error) {
	path := "/api/v1/downloads"
	if activeOnly {
		path += "?active=true"
	}
	var resp ListDownloadsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Search(query, contentType, profile string) (*SearchResponse, error) {
	req := map[string]any{
		"query": query,
	}
	if contentType != "" {
		req["type"] = contentType
	}
	if profile != "" {
		req["profile"] = profile
	}

	var resp SearchResponse
	if err := c.post("/api/v1/search", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) AddContent(contentType, title string, year int, profile string) (*ContentResponse, error) {
	req := map[string]any{
		"type":            contentType,
		"title":           title,
		"year":            year,
		"quality_profile": profile,
	}

	var resp ContentResponse
	if err := c.post("/api/v1/content", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Grab(contentID int64, downloadURL, title, indexer string) (*GrabResponse, error) {
	req := map[string]any{
		"content_id":   contentID,
		"download_url": downloadURL,
		"title":        title,
		"indexer":      indexer,
	}

	var resp GrabResponse
	if err := c.post("/api/v1/grab", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/client.go
git commit -m "feat(cli): add client API methods"
```

---

### Task 3: Implement Status Command

**Files:**
- Create: `cmd/arrgo/commands.go`

**Step 1: Create commands.go with status command**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// Common flags
type commonFlags struct {
	server string
	json   bool
}

func parseCommonFlags(fs *flag.FlagSet, args []string) commonFlags {
	var f commonFlags
	fs.StringVar(&f.server, "server", "http://localhost:8484", "Server URL")
	fs.BoolVar(&f.json, "json", false, "Output as JSON")
	fs.Parse(args)
	return f
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	flags := parseCommonFlags(fs, args)

	client := NewClient(flags.server)
	status, err := client.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flags.json {
		printJSON(status)
		return
	}

	printStatusHuman(flags.server, status)
}

func printStatusHuman(server string, s *StatusResponse) {
	fmt.Printf("Server:     %s (%s)\n", server, s.Status)
	fmt.Printf("Version:    %s\n", s.Version)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/commands.go
git commit -m "feat(cli): implement status command"
```

---

### Task 4: Implement Queue Command

**Files:**
- Modify: `cmd/arrgo/commands.go`

**Step 1: Add queue command**

Add to commands.go:

```go
func runQueue(args []string) {
	fs := flag.NewFlagSet("queue", flag.ExitOnError)
	var showAll bool
	fs.BoolVar(&showAll, "all", false, "Include completed/imported downloads")
	flags := parseCommonFlags(fs, args)

	client := NewClient(flags.server)
	downloads, err := client.Downloads(!showAll)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flags.json {
		printJSON(downloads)
		return
	}

	printQueueHuman(downloads)
}

func printQueueHuman(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No downloads in queue")
		return
	}

	fmt.Printf("Downloads (%d):\n\n", d.Total)
	fmt.Printf("  # │ %-40s │ %-12s │ %s\n", "TITLE", "STATUS", "INDEXER")
	fmt.Println("────┼──────────────────────────────────────────┼──────────────┼─────────")

	for i, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf(" %2d │ %-40s │ %-12s │ %s\n", i+1, title, dl.Status, dl.Indexer)
	}
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/commands.go
git commit -m "feat(cli): implement queue command"
```

---

### Task 5: Implement Search Command

**Files:**
- Modify: `cmd/arrgo/commands.go`

**Step 1: Add imports and helper**

Add to imports at top of commands.go:

```go
import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/arrgo/arrgo/pkg/release"
)
```

Add helper function:

```go
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func prompt(message string) string {
	fmt.Print(message)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
```

**Step 2: Add search command**

```go
func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	var contentType, profile, grabFlag string
	fs.StringVar(&contentType, "type", "", "Content type (movie or series)")
	fs.StringVar(&profile, "profile", "", "Quality profile")
	fs.StringVar(&grabFlag, "grab", "", "Grab release: number or 'best'")
	flags := parseCommonFlags(fs, args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: arrgo search <query> [--type movie|series] [--grab N|best]")
		os.Exit(1)
	}
	query := strings.Join(remaining, " ")

	client := NewClient(flags.server)
	results, err := client.Search(query, contentType, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flags.json {
		printJSON(results)
		return
	}

	if len(results.Releases) == 0 {
		fmt.Println("No releases found")
		return
	}

	printSearchHuman(query, results)

	// Handle grab
	var grabIndex int
	if grabFlag == "best" {
		grabIndex = 1
	} else if grabFlag != "" {
		grabIndex, _ = strconv.Atoi(grabFlag)
	} else if !flags.json {
		// Interactive prompt
		input := prompt(fmt.Sprintf("\nGrab? [1-%d, n]: ", len(results.Releases)))
		if input == "" || input == "n" || input == "N" {
			return
		}
		grabIndex, _ = strconv.Atoi(input)
	}

	if grabIndex < 1 || grabIndex > len(results.Releases) {
		return
	}

	// Grab the selected release
	selected := results.Releases[grabIndex-1]
	grabRelease(client, selected, contentType, profile)
}

func printSearchHuman(query string, r *SearchResponse) {
	fmt.Printf("Found %d releases for %q:\n\n", len(r.Releases), query)
	fmt.Printf("  # │ %-42s │ %8s │ %-10s │ %5s\n", "TITLE", "SIZE", "INDEXER", "SCORE")
	fmt.Println("────┼────────────────────────────────────────────┼──────────┼────────────┼───────")

	for i, rel := range r.Releases {
		title := rel.Title
		if len(title) > 42 {
			title = title[:39] + "..."
		}
		fmt.Printf(" %2d │ %-42s │ %8s │ %-10s │ %5d\n",
			i+1, title, formatSize(rel.Size), rel.Indexer, rel.Score)
	}

	if len(r.Errors) > 0 {
		fmt.Printf("\nWarnings: %s\n", strings.Join(r.Errors, ", "))
	}
}

func grabRelease(client *Client, rel ReleaseResponse, contentType, profile string) {
	// Parse release name to get title/year
	info := release.Parse(rel.Title)
	if info.Title == "" {
		fmt.Fprintln(os.Stderr, "Error: Could not parse release title")
		return
	}

	// Determine content type if not specified
	if contentType == "" {
		if info.Season > 0 || info.Episode > 0 {
			contentType = "series"
		} else {
			contentType = "movie"
		}
	}

	// Show what we parsed
	fmt.Printf("\nParsed: %s (%d)\n", info.Title, info.Year)

	input := prompt("Confirm? [Y/n]: ")
	if input != "" && input != "y" && input != "Y" {
		fmt.Println("Cancelled")
		return
	}

	// Default profile
	if profile == "" {
		profile = "hd"
	}

	// Create content entry
	content, err := client.AddContent(contentType, info.Title, info.Year, profile)
	if err != nil {
		// Might already exist, try to proceed anyway
		fmt.Printf("Note: %v\n", err)
	} else {
		fmt.Printf("✓ Added to library (ID: %d)\n", content.ID)
	}

	// Grab the release
	if content != nil {
		grab, err := client.Grab(content.ID, rel.DownloadURL, rel.Title, rel.Indexer)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error grabbing: %v\n", err)
			return
		}
		fmt.Printf("✓ Download started (ID: %d)\n", grab.DownloadID)
	}
}
```

**Step 3: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 4: Commit**

```bash
git add cmd/arrgo/commands.go
git commit -m "feat(cli): implement search command with grab"
```

---

### Task 6: Wire Commands in main.go

**Files:**
- Modify: `cmd/arrgo/main.go`

**Step 1: Update command cases**

Replace the stub cases in main.go:

```go
	case "status":
		runStatus(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "queue":
		runQueue(os.Args[2:])
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Test commands (requires running server)**

```bash
# Start server in background (if not running)
./arrgo serve &

# Test status
./arrgo status
./arrgo status --json

# Test queue
./arrgo queue
./arrgo queue --all

# Test search (requires Prowlarr)
./arrgo search "the matrix 1999" --type movie
```

**Step 4: Commit**

```bash
git add cmd/arrgo/main.go
git commit -m "feat(cli): wire up status, search, queue commands"
```

---

### Task 7: Final Verification

**Step 1: Run tests**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: No new issues

**Step 3: Verify all commands work**

```bash
./arrgo --help
./arrgo status --help
./arrgo search --help
./arrgo queue --help
```

**Step 4: Commit any fixes**

---

## Summary

After completing all tasks:

1. `arrgo status` - Shows server status and version
2. `arrgo search <query>` - Searches indexers, prompts to grab
3. `arrgo queue` - Shows active downloads

All commands support:
- `--server URL` for remote servers
- `--json` for machine-readable output
