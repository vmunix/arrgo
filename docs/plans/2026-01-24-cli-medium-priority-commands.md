# CLI Medium Priority Commands Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add medium priority CLI commands from issue #58: `library add`, `indexers`, `profiles`, `files`

**Architecture:** CLI commands call REST API endpoints. New `GET /api/v1/indexers` endpoint needed; others already exist.

**Tech Stack:** Go, Cobra CLI, net/http

---

### Task 1: Add Name getter to Newznab client

**Files:**
- Modify: `pkg/newznab/client.go`

**Step 1: Add Name() method to Client**

```go
// Name returns the indexer name.
func (c *Client) Name() string {
	return c.name
}
```

**Step 2: Verify it compiles**

Run: `go build ./...`

**Step 3: Commit**

```bash
git add pkg/newznab/client.go
git commit -m "feat(newznab): add Name getter to Client (#58)"
```

---

### Task 2: Add IndexerInfo type and getter to API deps

**Files:**
- Modify: `internal/api/v1/deps.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add IndexerAPI interface to deps.go**

```go
// IndexerAPI represents an indexer that can be queried.
type IndexerAPI interface {
	Name() string
	URL() string
	Caps(ctx context.Context) error // Simple connectivity test
}
```

**Step 2: Add Indexers field to ServerDeps**

```go
Indexers []IndexerAPI // Optional: configured indexers
```

**Step 3: Add response types to types.go**

```go
type indexerResponse struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	ResponseMs int64  `json:"response_ms,omitempty"`
}

type listIndexersResponse struct {
	Indexers []indexerResponse `json:"indexers"`
}
```

**Step 4: Verify it compiles**

Run: `go build ./...`

**Step 5: Commit**

```bash
git add internal/api/v1/deps.go internal/api/v1/types.go
git commit -m "feat(api): add IndexerAPI interface and response types (#58)"
```

---

### Task 3: Add URL and Caps methods to Newznab client

**Files:**
- Modify: `pkg/newznab/client.go`

**Step 1: Add URL() method**

```go
// URL returns the indexer base URL.
func (c *Client) URL() string {
	return c.baseURL
}
```

**Step 2: Add Caps() method for connectivity test**

```go
// Caps performs a capabilities request to test connectivity.
func (c *Client) Caps(ctx context.Context) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("t", "caps")
	q.Set("apikey", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caps request failed: %d", resp.StatusCode)
	}
	return nil
}
```

**Step 3: Verify it compiles**

Run: `go build ./...`

**Step 4: Commit**

```bash
git add pkg/newznab/client.go
git commit -m "feat(newznab): add URL and Caps methods (#58)"
```

---

### Task 4: Add GET /api/v1/indexers endpoint

**Files:**
- Modify: `internal/api/v1/api.go`

**Step 1: Register route**

Add to RegisterRoutes:
```go
mux.HandleFunc("GET /api/v1/indexers", s.listIndexers)
```

**Step 2: Implement handler**

```go
func (s *Server) listIndexers(w http.ResponseWriter, r *http.Request) {
	testConn := r.URL.Query().Get("test") == "true"
	ctx := r.Context()

	resp := listIndexersResponse{
		Indexers: make([]indexerResponse, len(s.deps.Indexers)),
	}

	for i, idx := range s.deps.Indexers {
		resp.Indexers[i] = indexerResponse{
			Name: idx.Name(),
			URL:  idx.URL(),
		}

		if testConn {
			start := time.Now()
			if err := idx.Caps(ctx); err != nil {
				resp.Indexers[i].Status = "error"
				resp.Indexers[i].Error = err.Error()
			} else {
				resp.Indexers[i].Status = "ok"
				resp.Indexers[i].ResponseMs = time.Since(start).Milliseconds()
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 3: Verify it compiles**

Run: `go build ./...`

**Step 4: Commit**

```bash
git add internal/api/v1/api.go
git commit -m "feat(api): add GET /api/v1/indexers endpoint (#58)"
```

---

### Task 5: Wire indexers into API server

**Files:**
- Modify: `cmd/arrgod/server.go`

**Step 1: Build indexer slice for API**

After creating newznabClients, build a slice for the API:

```go
// Build indexer list for API
var apiIndexers []v1.IndexerAPI
for _, c := range newznabClients {
	apiIndexers = append(apiIndexers, c)
}
```

**Step 2: Pass to ServerDeps**

Add to the v1.ServerDeps struct initialization:
```go
Indexers: apiIndexers,
```

**Step 3: Verify it compiles**

Run: `go build ./...`

**Step 4: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "feat(server): wire indexers into API (#58)"
```

---

### Task 6: Add arrgo indexers CLI command

**Files:**
- Create: `cmd/arrgo/indexers.go`

**Step 1: Create command file**

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

type IndexerResponse struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	ResponseMs int64  `json:"response_ms,omitempty"`
}

type ListIndexersResponse struct {
	Indexers []IndexerResponse `json:"indexers"`
}

var indexersCmd = &cobra.Command{
	Use:   "indexers",
	Short: "List configured indexers",
	RunE:  runIndexers,
}

func init() {
	indexersCmd.Flags().Bool("test", false, "Test connectivity to each indexer")
	rootCmd.AddCommand(indexersCmd)
}

func runIndexers(cmd *cobra.Command, args []string) error {
	testConn, _ := cmd.Flags().GetBool("test")

	url := fmt.Sprintf("%s/api/v1/indexers", serverURL)
	if testConn {
		url += "?test=true"
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var data ListIndexersResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if jsonOutput {
		printJSON(data)
		return nil
	}

	if len(data.Indexers) == 0 {
		fmt.Println("No indexers configured")
		return nil
	}

	if testConn {
		printIndexersWithTest(data.Indexers)
	} else {
		printIndexers(data.Indexers)
	}
	return nil
}

func printIndexers(indexers []IndexerResponse) {
	fmt.Printf("Indexers (%d):\n\n", len(indexers))
	fmt.Printf("  %-15s %s\n", "NAME", "URL")
	fmt.Println("  " + strings.Repeat("-", 60))
	for _, idx := range indexers {
		fmt.Printf("  %-15s %s\n", idx.Name, idx.URL)
	}
}

func printIndexersWithTest(indexers []IndexerResponse) {
	fmt.Printf("Indexers (%d):\n\n", len(indexers))
	fmt.Printf("  %-15s %-8s %s\n", "NAME", "STATUS", "LATENCY/ERROR")
	fmt.Println("  " + strings.Repeat("-", 60))
	for _, idx := range indexers {
		if idx.Status == "ok" {
			fmt.Printf("  %-15s %-8s %dms\n", idx.Name, idx.Status, idx.ResponseMs)
		} else {
			fmt.Printf("  %-15s %-8s %s\n", idx.Name, idx.Status, idx.Error)
		}
	}
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/indexers.go
git commit -m "feat(cli): add arrgo indexers command (#58)"
```

---

### Task 7: Add arrgo profiles CLI command

**Files:**
- Create: `cmd/arrgo/profiles.go`

**Step 1: Create command file**

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

type ProfileResponse struct {
	Name   string   `json:"name"`
	Accept []string `json:"accept"`
}

type ListProfilesResponse struct {
	Profiles []ProfileResponse `json:"profiles"`
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List quality profiles",
	RunE:  runProfiles,
}

func init() {
	rootCmd.AddCommand(profilesCmd)
}

func runProfiles(cmd *cobra.Command, args []string) error {
	url := fmt.Sprintf("%s/api/v1/profiles", serverURL)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var data ListProfilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if jsonOutput {
		printJSON(data)
		return nil
	}

	if len(data.Profiles) == 0 {
		fmt.Println("No quality profiles configured")
		return nil
	}

	fmt.Printf("Quality Profiles (%d):\n\n", len(data.Profiles))
	fmt.Printf("  %-12s %s\n", "NAME", "RESOLUTIONS")
	fmt.Println("  " + strings.Repeat("-", 50))
	for _, p := range data.Profiles {
		fmt.Printf("  %-12s %s\n", p.Name, strings.Join(p.Accept, ", "))
	}
	return nil
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/profiles.go
git commit -m "feat(cli): add arrgo profiles command (#58)"
```

---

### Task 8: Add arrgo files CLI command

**Files:**
- Create: `cmd/arrgo/files.go`

**Step 1: Create command file**

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type FileResponse struct {
	ID        int64     `json:"id"`
	ContentID int64     `json:"content_id"`
	EpisodeID *int64    `json:"episode_id,omitempty"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Quality   string    `json:"quality"`
	Source    string    `json:"source"`
	AddedAt   time.Time `json:"added_at"`
}

type ListFilesResponse struct {
	Items []FileResponse `json:"items"`
	Total int            `json:"total"`
}

var filesCmd = &cobra.Command{
	Use:   "files [content-id]",
	Short: "List tracked media files",
	Long:  "Lists all tracked media files, optionally filtered by content ID.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runFiles,
}

func init() {
	rootCmd.AddCommand(filesCmd)
}

func runFiles(cmd *cobra.Command, args []string) error {
	url := fmt.Sprintf("%s/api/v1/files", serverURL)

	var contentID int64
	if len(args) > 0 {
		var err error
		contentID, err = strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid content ID: %s", args[0])
		}
		url += fmt.Sprintf("?content_id=%d", contentID)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var data ListFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if jsonOutput {
		printJSON(data)
		return nil
	}

	if len(data.Items) == 0 {
		if contentID > 0 {
			fmt.Printf("No files for content ID %d\n", contentID)
		} else {
			fmt.Println("No tracked files")
		}
		return nil
	}

	printFiles(data.Items, contentID)
	return nil
}

func printFiles(files []FileResponse, contentID int64) {
	if contentID > 0 {
		fmt.Printf("Files for content %d (%d):\n\n", contentID, len(files))
		fmt.Printf("  %-4s %-50s %-10s %s\n", "ID", "PATH", "SIZE", "QUALITY")
	} else {
		fmt.Printf("Files (%d):\n\n", len(files))
		fmt.Printf("  %-4s %-8s %-45s %-10s %s\n", "ID", "CONTENT", "PATH", "SIZE", "QUALITY")
	}
	fmt.Println("  " + strings.Repeat("-", 80))

	for _, f := range files {
		path := f.Path
		if contentID > 0 {
			if len(path) > 50 {
				path = "..." + path[len(path)-47:]
			}
			fmt.Printf("  %-4d %-50s %-10s %s\n", f.ID, path, formatBytes(f.SizeBytes), f.Quality)
		} else {
			if len(path) > 45 {
				path = "..." + path[len(path)-42:]
			}
			fmt.Printf("  %-4d %-8d %-45s %-10s %s\n", f.ID, f.ContentID, path, formatBytes(f.SizeBytes), f.Quality)
		}
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 3: Commit**

```bash
git add cmd/arrgo/files.go
git commit -m "feat(cli): add arrgo files command (#58)"
```

---

### Task 9: Add arrgo library add CLI command

**Files:**
- Modify: `cmd/arrgo/library.go`

**Step 1: Add addCmd to library.go**

```go
addCmd := &cobra.Command{
	Use:   "add",
	Short: "Add content to library",
	Long:  "Adds a movie or series to the wanted list for future grabbing.",
	RunE:  runLibraryAdd,
}

addCmd.Flags().StringP("title", "t", "", "Title (required)")
addCmd.Flags().IntP("year", "y", 0, "Year (required)")
addCmd.Flags().String("type", "", "Type: movie or series (required)")
addCmd.Flags().Int64("tmdb-id", 0, "TMDB ID (optional, for movies)")
addCmd.Flags().Int64("tvdb-id", 0, "TVDB ID (optional, for series)")
addCmd.Flags().StringP("quality", "q", "", "Quality profile")

_ = addCmd.MarkFlagRequired("title")
_ = addCmd.MarkFlagRequired("year")
_ = addCmd.MarkFlagRequired("type")

libraryCmd.AddCommand(addCmd)
```

**Step 2: Implement runLibraryAdd function**

```go
func runLibraryAdd(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	year, _ := cmd.Flags().GetInt("year")
	contentType, _ := cmd.Flags().GetString("type")
	tmdbID, _ := cmd.Flags().GetInt64("tmdb-id")
	tvdbID, _ := cmd.Flags().GetInt64("tvdb-id")
	quality, _ := cmd.Flags().GetString("quality")

	// Validate type
	if contentType != "movie" && contentType != "series" {
		return fmt.Errorf("type must be 'movie' or 'series'")
	}

	// Build request
	reqBody := map[string]any{
		"title": title,
		"year":  year,
		"type":  contentType,
	}
	if tmdbID > 0 {
		reqBody["tmdb_id"] = tmdbID
	}
	if tvdbID > 0 {
		reqBody["tvdb_id"] = tvdbID
	}
	if quality != "" {
		reqBody["quality_profile"] = quality
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/content", serverURL)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("content already exists: %s (%d)", title, year)
	}
	if resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var content LibraryContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if jsonOutput {
		printJSON(content)
		return nil
	}

	fmt.Printf("Added: %s (%d) [ID: %d, status: %s]\n", content.Title, content.Year, content.ID, content.Status)
	return nil
}
```

**Step 3: Verify it compiles**

Run: `go build ./cmd/arrgo`

**Step 4: Commit**

```bash
git add cmd/arrgo/library.go
git commit -m "feat(cli): add arrgo library add command (#58)"
```

---

### Task 10: Final verification

**Step 1: Run all tests**

Run: `go test ./...`

**Step 2: Build and test commands manually**

```bash
./arrgo indexers
./arrgo indexers --test
./arrgo profiles
./arrgo files
./arrgo library add --title "Test Movie" --year 2024 --type movie
```

**Step 3: Verify JSON output works**

```bash
./arrgo indexers --json
./arrgo profiles --json
./arrgo files --json
```
