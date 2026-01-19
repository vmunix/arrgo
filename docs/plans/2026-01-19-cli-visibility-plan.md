# CLI Visibility Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve CLI visibility into download pipeline with dashboard, enhanced queue, imports view, and verify command.

**Architecture:** Add new API endpoints for aggregated stats and verification checks. CLI commands call these endpoints and format output. Server-side logic reuses existing store queries and client connections.

**Tech Stack:** Go, Cobra CLI, existing API v1 patterns

---

## Task 1: Add Dashboard Stats API Endpoint

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/status.go`

**Step 1: Add response type for dashboard stats**

In `internal/api/v1/status.go`, add after existing types:

```go
type DashboardResponse struct {
	Version   string `json:"version"`
	Connections struct {
		Server   bool   `json:"server"`
		Plex     bool   `json:"plex"`
		SABnzbd  bool   `json:"sabnzbd"`
	} `json:"connections"`
	Downloads struct {
		Queued      int `json:"queued"`
		Downloading int `json:"downloading"`
		Completed   int `json:"completed"`
		Imported    int `json:"imported"`
		Cleaned     int `json:"cleaned"`
		Failed      int `json:"failed"`
	} `json:"downloads"`
	Stuck struct {
		Count     int   `json:"count"`
		Threshold int64 `json:"threshold_minutes"`
	} `json:"stuck"`
	Library struct {
		Movies int `json:"movies"`
		Series int `json:"series"`
	} `json:"library"`
}
```

**Step 2: Add handler method**

In `internal/api/v1/status.go`, add:

```go
func (s *Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp := DashboardResponse{
		Version: "0.1.0",
	}

	// Connection status
	resp.Connections.Server = true
	resp.Connections.Plex = s.plex != nil
	resp.Connections.SABnzbd = s.manager != nil

	// Download counts by status
	for _, status := range []string{"queued", "downloading", "completed", "imported", "cleaned", "failed"} {
		st := download.Status(status)
		downloads, _ := s.downloads.List(download.DownloadFilter{Status: &st})
		switch status {
		case "queued":
			resp.Downloads.Queued = len(downloads)
		case "downloading":
			resp.Downloads.Downloading = len(downloads)
		case "completed":
			resp.Downloads.Completed = len(downloads)
		case "imported":
			resp.Downloads.Imported = len(downloads)
		case "cleaned":
			resp.Downloads.Cleaned = len(downloads)
		case "failed":
			resp.Downloads.Failed = len(downloads)
		}
	}

	// Stuck count (>1hr in non-terminal state)
	resp.Stuck.Threshold = 60
	stuck, _ := s.downloads.ListStuck(time.Hour, false)
	resp.Stuck.Count = len(stuck)

	// Library counts
	movies, _ := s.library.List(library.ContentFilter{Type: ptrTo("movie")})
	series, _ := s.library.List(library.ContentFilter{Type: ptrTo("series")})
	resp.Library.Movies = len(movies)
	resp.Library.Series = len(series)

	writeJSON(w, http.StatusOK, resp)
}

func ptrTo[T any](v T) *T {
	return &v
}
```

**Step 3: Register route**

In `internal/api/v1/api.go`, add in `RegisterRoutes`:

```go
mux.HandleFunc("GET /api/v1/dashboard", s.getDashboard)
```

**Step 4: Run tests**

```bash
go test ./internal/api/v1/... -v
```

**Step 5: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): add dashboard endpoint with aggregated stats"
```

---

## Task 2: Enhanced Status CLI Command

**Files:**
- Modify: `cmd/arrgo/client.go`
- Modify: `cmd/arrgo/status.go`

**Step 1: Add Dashboard client method**

In `cmd/arrgo/client.go`, add response type:

```go
type DashboardResponse struct {
	Version     string `json:"version"`
	Connections struct {
		Server  bool `json:"server"`
		Plex    bool `json:"plex"`
		SABnzbd bool `json:"sabnzbd"`
	} `json:"connections"`
	Downloads struct {
		Queued      int `json:"queued"`
		Downloading int `json:"downloading"`
		Completed   int `json:"completed"`
		Imported    int `json:"imported"`
		Cleaned     int `json:"cleaned"`
		Failed      int `json:"failed"`
	} `json:"downloads"`
	Stuck struct {
		Count     int   `json:"count"`
		Threshold int64 `json:"threshold_minutes"`
	} `json:"stuck"`
	Library struct {
		Movies int `json:"movies"`
		Series int `json:"series"`
	} `json:"library"`
}
```

Add method:

```go
func (c *Client) Dashboard() (*DashboardResponse, error) {
	var resp DashboardResponse
	if err := c.get("/api/v1/dashboard", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

**Step 2: Update status command**

Replace `cmd/arrgo/status.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "System status (health, disk, queue summary)",
	RunE:  runStatusCmd,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatusCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	dash, err := client.Dashboard()
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	if jsonOutput {
		printJSON(dash)
		return nil
	}

	printDashboard(serverURL, dash)
	return nil
}

func printDashboard(server string, d *DashboardResponse) {
	// Header
	plexStatus := "disconnected"
	if d.Connections.Plex {
		plexStatus = "connected"
	}
	sabStatus := "disconnected"
	if d.Connections.SABnzbd {
		sabStatus = "connected"
	}

	fmt.Printf("arrgo v%s | Server: %s | Plex: %s | SABnzbd: %s\n\n",
		d.Version, server, plexStatus, sabStatus)

	// Downloads
	fmt.Println("Downloads")
	fmt.Printf("  Queued:       %d\n", d.Downloads.Queued)
	fmt.Printf("  Downloading:  %d\n", d.Downloads.Downloading)
	fmt.Printf("  Completed:    %d\n", d.Downloads.Completed)
	fmt.Printf("  Imported:     %d  (awaiting Plex verification)\n", d.Downloads.Imported)
	fmt.Println()

	// Library
	fmt.Println("Library")
	fmt.Printf("  Movies:     %d tracked\n", d.Library.Movies)
	fmt.Printf("  Series:     %d tracked\n", d.Library.Series)
	fmt.Println()

	// Problems
	if d.Stuck.Count > 0 || d.Downloads.Failed > 0 {
		problems := d.Stuck.Count + d.Downloads.Failed
		fmt.Printf("Problems: %d detected\n", problems)
		fmt.Println("  → Run 'arrgo verify' for details")
	}
}
```

**Step 3: Build and test manually**

```bash
go build -o arrgo ./cmd/arrgo
./arrgo status
```

**Step 4: Commit**

```bash
git add cmd/arrgo/
git commit -m "feat(cli): enhance status command with dashboard view"
```

---

## Task 3: Enhanced Queue Command

**Files:**
- Modify: `cmd/arrgo/queue.go`

**Step 1: Add flags and update output**

Replace `cmd/arrgo/queue.go`:

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show active downloads",
	RunE:  runQueueCmd,
}

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.Flags().BoolP("all", "a", false, "Include terminal states (cleaned, failed)")
	queueCmd.Flags().StringP("state", "s", "", "Filter by state (queued, downloading, completed, imported, cleaned, failed)")
	queueCmd.Flags().Bool("stuck", false, "Only show stuck downloads")
}

func runQueueCmd(cmd *cobra.Command, args []string) error {
	showAll, _ := cmd.Flags().GetBool("all")
	stateFilter, _ := cmd.Flags().GetString("state")
	showStuck, _ := cmd.Flags().GetBool("stuck")

	client := NewClient(serverURL)
	downloads, err := client.Downloads(!showAll)
	if err != nil {
		return fmt.Errorf("queue fetch failed: %w", err)
	}

	// Filter by state if specified
	if stateFilter != "" {
		filtered := make([]DownloadResponse, 0)
		for _, dl := range downloads.Items {
			if strings.EqualFold(dl.Status, stateFilter) {
				filtered = append(filtered, dl)
			}
		}
		downloads.Items = filtered
		downloads.Total = len(filtered)
	}

	if jsonOutput {
		printJSON(downloads)
		return nil
	}

	if showAll {
		printQueueAll(downloads)
	} else {
		printQueueActive(downloads)
	}
	return nil
}

func printQueueActive(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No active downloads")
		return
	}

	fmt.Printf("Active Downloads (%d):\n\n", d.Total)
	fmt.Printf("  %-4s %-12s %-44s %s\n", "ID", "STATE", "RELEASE", "PROGRESS")
	fmt.Println("  " + strings.Repeat("-", 70))

	for _, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 44 {
			title = title[:41] + "..."
		}
		progress := "-"
		if dl.Status == "downloading" {
			progress = "..." // Would need live data from SABnzbd
		} else if dl.Status == "completed" {
			progress = "100%"
		}
		fmt.Printf("  %-4d %-12s %-44s %s\n", dl.ID, dl.Status, title, progress)
	}
}

func printQueueAll(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No downloads")
		return
	}

	fmt.Printf("All Downloads (%d):\n\n", d.Total)
	fmt.Printf("  %-4s %-12s %-40s %-12s\n", "ID", "STATE", "RELEASE", "COMPLETED")
	fmt.Println("  " + strings.Repeat("-", 72))

	for _, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		completed := "-"
		if dl.CompletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
				completed = formatTimeAgo(t)
			}
		}
		fmt.Printf("  %-4d %-12s %-40s %-12s\n", dl.ID, dl.Status, title, completed)
	}
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

**Step 2: Build and test**

```bash
go build -o arrgo ./cmd/arrgo
./arrgo queue
./arrgo queue --all
./arrgo queue --state=cleaned
```

**Step 3: Commit**

```bash
git add cmd/arrgo/queue.go
git commit -m "feat(cli): enhance queue with state filter and better formatting"
```

---

## Task 4: Add Imports Command

**Files:**
- Create: `cmd/arrgo/imports.go`

**Step 1: Create imports command**

Create `cmd/arrgo/imports.go`:

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var importsCmd = &cobra.Command{
	Use:   "imports",
	Short: "Show pending imports and recent completions",
	RunE:  runImportsCmd,
}

func init() {
	rootCmd.AddCommand(importsCmd)
	importsCmd.Flags().Bool("pending", false, "Only show pending imports")
	importsCmd.Flags().Bool("recent", false, "Only show recent completions")
}

func runImportsCmd(cmd *cobra.Command, args []string) error {
	pendingOnly, _ := cmd.Flags().GetBool("pending")
	recentOnly, _ := cmd.Flags().GetBool("recent")

	client := NewClient(serverURL)

	// Get all downloads to filter
	downloads, err := client.Downloads(false)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	// Split into pending (imported) and recent (cleaned)
	var pending, recent []DownloadResponse
	for _, dl := range downloads.Items {
		switch dl.Status {
		case "imported":
			pending = append(pending, dl)
		case "cleaned":
			// Only include if completed within last 24h
			if dl.CompletedAt != nil {
				if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
					if time.Since(t) < 24*time.Hour {
						recent = append(recent, dl)
					}
				}
			}
		}
	}

	if jsonOutput {
		printJSON(map[string]any{
			"pending": pending,
			"recent":  recent,
		})
		return nil
	}

	if !recentOnly {
		printPendingImports(pending)
	}
	if !pendingOnly && !recentOnly {
		fmt.Println()
	}
	if !pendingOnly {
		printRecentImports(recent)
	}
	return nil
}

func printPendingImports(items []DownloadResponse) {
	fmt.Printf("Pending (%d):\n\n", len(items))

	if len(items) == 0 {
		fmt.Println("  No pending imports")
		return
	}

	fmt.Printf("  %-4s %-28s %-12s %s\n", "ID", "TITLE", "IMPORTED", "PLEX")
	fmt.Println("  " + strings.Repeat("-", 56))

	for _, dl := range items {
		title := dl.ReleaseName
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		imported := "-"
		if dl.CompletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
				imported = formatTimeAgo(t)
				// Warn if waiting too long
				if time.Since(t) > time.Hour {
					fmt.Printf("  %-4d %-28s %-12s %s\n", dl.ID, title, imported, "waiting")
					fmt.Printf("    ⚠ Waiting >1hr - run 'arrgo verify %d' to check\n", dl.ID)
					continue
				}
			}
		}
		fmt.Printf("  %-4d %-28s %-12s %s\n", dl.ID, title, imported, "waiting")
	}
}

func printRecentImports(items []DownloadResponse) {
	fmt.Printf("Recent (last 24h):\n\n", len(items))

	if len(items) == 0 {
		fmt.Println("  No recent imports")
		return
	}

	fmt.Printf("  %-4s %-28s %-12s %s\n", "ID", "TITLE", "IMPORTED", "STATUS")
	fmt.Println("  " + strings.Repeat("-", 56))

	for _, dl := range items {
		title := dl.ReleaseName
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		imported := "-"
		if dl.CompletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
				imported = formatTimeAgo(t)
			}
		}
		fmt.Printf("  %-4d %-28s %-12s %s\n", dl.ID, title, imported, "✓")
	}
}
```

**Step 2: Build and test**

```bash
go build -o arrgo ./cmd/arrgo
./arrgo imports
./arrgo imports --pending
./arrgo imports --recent
```

**Step 3: Commit**

```bash
git add cmd/arrgo/imports.go
git commit -m "feat(cli): add imports command showing pending and recent"
```

---

## Task 5: Add Verify API Endpoint

**Files:**
- Create: `internal/api/v1/verify.go`
- Modify: `internal/api/v1/api.go`

**Step 1: Create verify handler**

Create `internal/api/v1/verify.go`:

```go
package v1

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/arrgo/arrgo/internal/download"
)

type VerifyProblem struct {
	DownloadID int64    `json:"download_id"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Since      string   `json:"since"`
	Issue      string   `json:"issue"`
	Checks     []string `json:"checks"`
	Likely     string   `json:"likely_cause"`
	Fixes      []string `json:"suggested_fixes"`
}

type VerifyResponse struct {
	Connections struct {
		Plex    bool   `json:"plex"`
		PlexErr string `json:"plex_error,omitempty"`
		SABnzbd bool   `json:"sabnzbd"`
		SABErr  string `json:"sabnzbd_error,omitempty"`
	} `json:"connections"`
	Checked  int             `json:"checked"`
	Passed   int             `json:"passed"`
	Problems []VerifyProblem `json:"problems"`
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check for specific download ID
	idStr := r.URL.Query().Get("id")
	var filterID *int64
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id", "INVALID_ID")
			return
		}
		filterID = &id
	}

	resp := VerifyResponse{}

	// Test connections
	if s.plex != nil {
		_, err := s.plex.GetIdentity(ctx)
		resp.Connections.Plex = err == nil
		if err != nil {
			resp.Connections.PlexErr = err.Error()
		}
	}
	if s.manager != nil {
		_, err := s.manager.Client().List(ctx)
		resp.Connections.SABnzbd = err == nil
		if err != nil {
			resp.Connections.SABErr = err.Error()
		}
	}

	// Get downloads to verify
	downloads, err := s.downloads.List(download.DownloadFilter{Active: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list downloads: "+err.Error(), "DB_ERROR")
		return
	}

	// Filter if specific ID requested
	if filterID != nil {
		filtered := make([]*download.Download, 0)
		for _, dl := range downloads {
			if dl.ID == *filterID {
				filtered = append(filtered, dl)
			}
		}
		downloads = filtered
	}

	resp.Checked = len(downloads)

	for _, dl := range downloads {
		problem := s.verifyDownload(ctx, dl)
		if problem != nil {
			resp.Problems = append(resp.Problems, *problem)
		} else {
			resp.Passed++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) verifyDownload(ctx context.Context, dl *download.Download) *VerifyProblem {
	content, _ := s.library.GetContent(dl.ContentID)
	title := dl.ReleaseName
	if content != nil {
		title = content.Title
	}

	since := ""
	if !dl.LastTransitionAt.IsZero() {
		since = time.Since(dl.LastTransitionAt).Round(time.Minute).String()
	}

	switch dl.Status {
	case download.StatusDownloading:
		// Check if actually in SABnzbd
		if s.manager != nil {
			status, err := s.manager.Client().Status(ctx, dl.ClientID)
			if err != nil || status == nil {
				return &VerifyProblem{
					DownloadID: dl.ID,
					Status:     string(dl.Status),
					Title:      title,
					Since:      since,
					Issue:      "Not found in SABnzbd queue",
					Checks:     []string{"SABnzbd queue: not found"},
					Likely:     "Download was cancelled or SABnzbd cleared it",
					Fixes:      []string{"arrgo retry " + strconv.FormatInt(dl.ID, 10), "arrgo skip " + strconv.FormatInt(dl.ID, 10)},
				}
			}
		}

	case download.StatusCompleted:
		// Check if source file exists
		if s.manager != nil {
			status, _ := s.manager.Client().Status(ctx, dl.ClientID)
			if status != nil && status.Path != "" {
				if _, err := os.Stat(status.Path); os.IsNotExist(err) {
					return &VerifyProblem{
						DownloadID: dl.ID,
						Status:     string(dl.Status),
						Title:      title,
						Since:      since,
						Issue:      "Source file not found",
						Checks:     []string{"File at " + status.Path + ": missing"},
						Likely:     "File was manually deleted or moved",
						Fixes:      []string{"arrgo retry " + strconv.FormatInt(dl.ID, 10), "arrgo skip " + strconv.FormatInt(dl.ID, 10)},
					}
				}
			}
		}

	case download.StatusImported:
		// Check if in Plex
		if s.plex != nil && content != nil {
			found, _ := s.plex.HasMovie(ctx, content.Title, content.Year)
			if !found {
				return &VerifyProblem{
					DownloadID: dl.ID,
					Status:     string(dl.Status),
					Title:      title,
					Since:      since,
					Issue:      "Not found in Plex library",
					Checks:     []string{"Plex search for '" + content.Title + "': not found"},
					Likely:     "Plex hasn't scanned yet",
					Fixes:      []string{"arrgo plex scan", "Wait for automatic scan"},
				}
			}
		}
	}

	return nil
}
```

**Step 2: Register route**

In `internal/api/v1/api.go`, add:

```go
mux.HandleFunc("GET /api/v1/verify", s.verify)
```

**Step 3: Run tests**

```bash
go test ./internal/api/v1/... -v
```

**Step 4: Commit**

```bash
git add internal/api/v1/
git commit -m "feat(api): add verify endpoint for reality checking"
```

---

## Task 6: Add Verify CLI Command

**Files:**
- Create: `cmd/arrgo/verify.go`
- Modify: `cmd/arrgo/client.go`

**Step 1: Add client method**

In `cmd/arrgo/client.go`, add:

```go
type VerifyProblem struct {
	DownloadID int64    `json:"download_id"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Since      string   `json:"since"`
	Issue      string   `json:"issue"`
	Checks     []string `json:"checks"`
	Likely     string   `json:"likely_cause"`
	Fixes      []string `json:"suggested_fixes"`
}

type VerifyResponse struct {
	Connections struct {
		Plex    bool   `json:"plex"`
		PlexErr string `json:"plex_error,omitempty"`
		SABnzbd bool   `json:"sabnzbd"`
		SABErr  string `json:"sabnzbd_error,omitempty"`
	} `json:"connections"`
	Checked  int             `json:"checked"`
	Passed   int             `json:"passed"`
	Problems []VerifyProblem `json:"problems"`
}

func (c *Client) Verify(id *int64) (*VerifyResponse, error) {
	path := "/api/v1/verify"
	if id != nil {
		path += fmt.Sprintf("?id=%d", *id)
	}
	var resp VerifyResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

**Step 2: Create verify command**

Create `cmd/arrgo/verify.go`:

```go
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify [download-id]",
	Short: "Verify download states against live systems",
	Long:  "Compare what arrgo thinks vs reality (SABnzbd, filesystem, Plex)",
	RunE:  runVerifyCmd,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerifyCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var id *int64
	if len(args) > 0 {
		parsed, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid download ID: %s", args[0])
		}
		id = &parsed
	}

	result, err := client.Verify(id)
	if err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}

	if jsonOutput {
		printJSON(result)
		return nil
	}

	printVerifyResult(result)
	return nil
}

func printVerifyResult(r *VerifyResponse) {
	fmt.Printf("Checking %d downloads...\n\n", r.Checked)

	// Connection status
	plexStatus := "✓"
	if !r.Connections.Plex {
		plexStatus = "✗ " + r.Connections.PlexErr
	}
	sabStatus := "✓"
	if !r.Connections.SABnzbd {
		sabStatus = "✗ " + r.Connections.SABErr
	}
	fmt.Printf("%s SABnzbd connection\n", sabStatus)
	fmt.Printf("%s Plex connection\n", plexStatus)
	fmt.Printf("✓ %d downloads verified\n", r.Passed)
	fmt.Println()

	if len(r.Problems) == 0 {
		fmt.Println("No problems detected.")
		return
	}

	fmt.Printf("Problems (%d):\n\n", len(r.Problems))

	for _, p := range r.Problems {
		fmt.Printf("  ID %d | %s | %s\n", p.DownloadID, p.Status, p.Title)
		fmt.Printf("    State: %s (%s)\n", p.Status, p.Since)
		fmt.Printf("    Issue: %s\n", p.Issue)
		for _, check := range p.Checks {
			fmt.Printf("    Check: %s\n", check)
		}
		fmt.Printf("    Likely: %s\n", p.Likely)
		fmt.Printf("    Fix: %s\n", strings.Join(p.Fixes, "\n         "))
		fmt.Println()
	}

	fmt.Printf("%d problems found. Run suggested commands or 'arrgo verify --help' for options.\n", len(r.Problems))
}
```

**Step 3: Build and test**

```bash
go build -o arrgo ./cmd/arrgo
./arrgo verify
./arrgo verify 1
```

**Step 4: Commit**

```bash
git add cmd/arrgo/
git commit -m "feat(cli): add verify command for reality checking"
```

---

## Task 7: Final Integration Test

**Step 1: Build everything**

```bash
go build -o arrgo ./cmd/arrgo
go build -o arrgod ./cmd/arrgod
```

**Step 2: Run full test suite**

```bash
go test ./... -v
```

**Step 3: Manual integration test**

```bash
# Start server
./arrgod &

# Test all new commands
./arrgo status
./arrgo queue
./arrgo queue --all
./arrgo queue --state=cleaned
./arrgo imports
./arrgo imports --pending
./arrgo imports --recent
./arrgo verify

# Stop server
pkill arrgod
```

**Step 4: Final commit**

```bash
git add -A
git commit -m "test: verify CLI visibility features work end-to-end"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Dashboard API endpoint | `internal/api/v1/status.go`, `api.go` |
| 2 | Enhanced status CLI | `cmd/arrgo/status.go`, `client.go` |
| 3 | Enhanced queue CLI | `cmd/arrgo/queue.go` |
| 4 | New imports command | `cmd/arrgo/imports.go` |
| 5 | Verify API endpoint | `internal/api/v1/verify.go`, `api.go` |
| 6 | Verify CLI command | `cmd/arrgo/verify.go`, `client.go` |
| 7 | Integration test | All |
