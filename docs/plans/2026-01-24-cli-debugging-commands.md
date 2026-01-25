# CLI Debugging Commands

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add three high-priority CLI commands for debugging the release flow: `events`, `queue show`, `queue retry`.

**Architecture:** Add API endpoints for events and retry, extend CLI with new commands and subcommands.

**Tech Stack:** Go, Cobra CLI, REST API

---

### Task 1: Add Recent method to EventLog

**Files:**
- Modify: `internal/events/log.go`
- Modify: `internal/events/log_test.go`

**Step 1: Write failing test**

```go
func TestEventLog_Recent(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)

	// Insert 5 events
	for i := 0; i < 5; i++ {
		evt := &ContentAdded{
			BaseEvent: NewBaseEvent(EventContentAdded, EntityContent, int64(i+1)),
			ContentID: int64(i + 1),
			Title:     fmt.Sprintf("Movie %d", i+1),
		}
		_, err := log.Append(evt)
		require.NoError(t, err)
	}

	// Get last 3
	events, err := log.Recent(3)
	require.NoError(t, err)
	assert.Len(t, events, 3)
	// Should be in reverse chronological order (newest first)
	assert.Equal(t, int64(5), events[0].EntityID)
	assert.Equal(t, int64(4), events[1].EntityID)
	assert.Equal(t, int64(3), events[2].EntityID)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events -run TestEventLog_Recent -v`
Expected: FAIL - method doesn't exist

**Step 3: Implement Recent method**

```go
// Recent returns the last N events in reverse chronological order.
func (l *EventLog) Recent(limit int) ([]RawEvent, error) {
	rows, err := l.db.Query(`
		SELECT id, event_type, entity_type, entity_id, payload, occurred_at, created_at
		FROM events
		ORDER BY id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events -run TestEventLog_Recent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/log.go internal/events/log_test.go
git commit -m "feat: add Recent method to EventLog (#58)"
```

---

### Task 2: Add GET /api/v1/events endpoint

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/types.go`
- Create: `internal/api/v1/events.go`

**Step 1: Add EventLog to Server struct**

In `api.go`, add to Deps struct:

```go
type Deps struct {
	// ... existing fields
	EventLog *events.EventLog
}
```

**Step 2: Add response type in types.go**

```go
// EventResponse represents an event in API responses.
type EventResponse struct {
	ID         int64  `json:"id"`
	EventType  string `json:"event_type"`
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	OccurredAt string `json:"occurred_at"`
}

// listEventsResponse is the response for GET /events.
type listEventsResponse struct {
	Items []EventResponse `json:"items"`
	Total int             `json:"total"`
}
```

**Step 3: Create events.go with handler**

```go
package v1

import (
	"net/http"
	"strconv"
)

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if s.deps.EventLog == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_EVENT_LOG", "Event log not configured")
		return
	}

	events, err := s.deps.EventLog.Recent(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	resp := listEventsResponse{
		Items: make([]EventResponse, len(events)),
		Total: len(events),
	}
	for i, e := range events {
		resp.Items[i] = EventResponse{
			ID:         e.ID,
			EventType:  e.EventType,
			EntityType: e.EntityType,
			EntityID:   e.EntityID,
			OccurredAt: e.OccurredAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Register route in api.go**

Add to RegisterRoutes:

```go
mux.HandleFunc("GET /api/v1/events", s.listEvents)
```

**Step 5: Run tests**

Run: `go test ./internal/api/v1 -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/types.go internal/api/v1/events.go
git commit -m "feat: add GET /api/v1/events endpoint (#58)"
```

---

### Task 3: Add GET /api/v1/downloads/{id}/events endpoint

**Files:**
- Modify: `internal/api/v1/events.go`
- Modify: `internal/api/v1/api.go`

**Step 1: Add handler in events.go**

```go
func (s *Server) listDownloadEvents(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	if s.deps.EventLog == nil {
		writeError(w, http.StatusServiceUnavailable, "NO_EVENT_LOG", "Event log not configured")
		return
	}

	// Verify download exists
	if _, err := s.deps.Downloads.Get(id); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
		return
	}

	events, err := s.deps.EventLog.ForEntity("download", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "EVENT_ERROR", err.Error())
		return
	}

	resp := listEventsResponse{
		Items: make([]EventResponse, len(events)),
		Total: len(events),
	}
	for i, e := range events {
		resp.Items[i] = EventResponse{
			ID:         e.ID,
			EventType:  e.EventType,
			EntityType: e.EntityType,
			EntityID:   e.EntityID,
			OccurredAt: e.OccurredAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 2: Register route in api.go**

```go
mux.HandleFunc("GET /api/v1/downloads/{id}/events", s.listDownloadEvents)
```

**Step 3: Run tests**

Run: `go test ./internal/api/v1 -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/api/v1/events.go internal/api/v1/api.go
git commit -m "feat: add GET /api/v1/downloads/{id}/events endpoint (#58)"
```

---

### Task 4: Add POST /api/v1/downloads/{id}/retry endpoint

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add response type in types.go**

```go
// retryResponse is the response for POST /downloads/{id}/retry.
type retryResponse struct {
	NewDownloadID int64  `json:"new_download_id"`
	ReleaseName   string `json:"release_name"`
	Message       string `json:"message"`
}
```

**Step 2: Add handler in api.go**

```go
func (s *Server) retryDownload(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}

	// Get the failed download
	dl, err := s.deps.Downloads.Get(id)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Download not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Only allow retry on failed downloads
	if dl.Status != download.StatusFailed {
		writeError(w, http.StatusBadRequest, "INVALID_STATE",
			fmt.Sprintf("Can only retry failed downloads, current status: %s", dl.Status))
		return
	}

	// Get content to search for
	content, err := s.deps.Content.Get(dl.ContentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONTENT_ERROR", err.Error())
		return
	}

	// Build search query
	query := content.Title
	if content.Year > 0 {
		query = fmt.Sprintf("%s %d", content.Title, content.Year)
	}

	// Search indexers
	results, err := s.deps.Searcher.Search(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SEARCH_ERROR", err.Error())
		return
	}

	if len(results) == 0 {
		writeError(w, http.StatusNotFound, "NO_RESULTS", "No releases found")
		return
	}

	// Grab best result (first one, already sorted by score)
	best := results[0]
	newDL, err := s.deps.Manager.Grab(r.Context(), best, dl.ContentID, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "GRAB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, retryResponse{
		NewDownloadID: newDL.ID,
		ReleaseName:   best.Title,
		Message:       "Download queued",
	})
}
```

**Step 3: Register route in api.go**

```go
mux.HandleFunc("POST /api/v1/downloads/{id}/retry", s.requireManager(s.requireSearcher(s.retryDownload)))
```

**Step 4: Run tests**

Run: `go test ./internal/api/v1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/types.go
git commit -m "feat: add POST /api/v1/downloads/{id}/retry endpoint (#58)"
```

---

### Task 5: Add CLI events command

**Files:**
- Create: `cmd/arrgo/events.go`
- Modify: `cmd/arrgo/client.go`

**Step 1: Add client method in client.go**

```go
type EventResponse struct {
	ID         int64  `json:"id"`
	EventType  string `json:"event_type"`
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	OccurredAt string `json:"occurred_at"`
}

type ListEventsResponse struct {
	Items []EventResponse `json:"items"`
	Total int             `json:"total"`
}

func (c *Client) Events(limit int) (*ListEventsResponse, error) {
	url := fmt.Sprintf("%s/api/v1/events?limit=%d", c.baseURL, limit)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}

	var result ListEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
```

**Step 2: Create events.go**

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show recent events",
	RunE:  runEventsCmd,
}

func init() {
	rootCmd.AddCommand(eventsCmd)
	eventsCmd.Flags().IntP("limit", "n", 20, "Number of events to show")
}

func runEventsCmd(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")

	client := NewClient(serverURL)
	events, err := client.Events(limit)
	if err != nil {
		return fmt.Errorf("failed to fetch events: %w", err)
	}

	if jsonOutput {
		printJSON(events)
		return nil
	}

	if len(events.Items) == 0 {
		fmt.Println("No events")
		return nil
	}

	fmt.Printf("Recent Events (%d):\n\n", events.Total)
	fmt.Printf("  %-12s %-24s %-15s %s\n", "TIME", "TYPE", "ENTITY", "ID")
	fmt.Println("  " + strings.Repeat("-", 60))

	for _, e := range events.Items {
		t, _ := time.Parse(time.RFC3339, e.OccurredAt)
		ago := formatTimeAgo(t.Unix())
		entity := fmt.Sprintf("%s/%d", e.EntityType, e.EntityID)
		fmt.Printf("  %-12s %-24s %-15s\n", ago, e.EventType, entity)
	}

	return nil
}
```

**Step 3: Run build to verify**

Run: `go build ./cmd/arrgo`
Expected: Success

**Step 4: Commit**

```bash
git add cmd/arrgo/events.go cmd/arrgo/client.go
git commit -m "feat: add arrgo events command (#58)"
```

---

### Task 6: Add CLI queue show subcommand

**Files:**
- Modify: `cmd/arrgo/queue.go`
- Modify: `cmd/arrgo/client.go`

**Step 1: Add client methods in client.go**

```go
func (c *Client) Download(id int64) (*DownloadResponse, error) {
	url := fmt.Sprintf("%s/api/v1/downloads/%d", c.baseURL, id)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}

	var result DownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DownloadEvents(id int64) (*ListEventsResponse, error) {
	url := fmt.Sprintf("%s/api/v1/downloads/%d/events", c.baseURL, id)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}

	var result ListEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
```

**Step 2: Add show subcommand in queue.go**

```go
var queueShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show detailed download info",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueShow,
}

func init() {
	// ... existing init
	queueCmd.AddCommand(queueShowCmd)
}

func runQueueShow(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	client := NewClient(serverURL)
	dl, err := client.Download(id)
	if err != nil {
		return fmt.Errorf("failed to fetch download: %w", err)
	}

	if jsonOutput {
		printJSON(dl)
		return nil
	}

	fmt.Printf("Download #%d\n\n", dl.ID)
	fmt.Printf("  %-12s %s\n", "Release:", dl.ReleaseName)
	fmt.Printf("  %-12s %d\n", "Content ID:", dl.ContentID)
	fmt.Printf("  %-12s %s\n", "Status:", dl.Status)
	fmt.Printf("  %-12s %s\n", "Indexer:", dl.Indexer)
	fmt.Printf("  %-12s %s (%s)\n", "Client:", dl.Client, dl.ClientID)
	fmt.Printf("  %-12s %s\n", "Added:", dl.AddedAt)
	if dl.CompletedAt != nil {
		fmt.Printf("  %-12s %s\n", "Completed:", *dl.CompletedAt)
	}

	// Fetch and display events
	events, err := client.DownloadEvents(id)
	if err == nil && len(events.Items) > 0 {
		fmt.Printf("\n  Event History:\n")
		for _, e := range events.Items {
			t, _ := time.Parse(time.RFC3339, e.OccurredAt)
			fmt.Printf("    %s  %s\n", t.Format("15:04:05"), e.EventType)
		}
	}

	return nil
}
```

**Step 3: Run build to verify**

Run: `go build ./cmd/arrgo`
Expected: Success

**Step 4: Commit**

```bash
git add cmd/arrgo/queue.go cmd/arrgo/client.go
git commit -m "feat: add arrgo queue show subcommand (#58)"
```

---

### Task 7: Add CLI queue retry subcommand

**Files:**
- Modify: `cmd/arrgo/queue.go`
- Modify: `cmd/arrgo/client.go`

**Step 1: Add client method in client.go**

```go
type RetryResponse struct {
	NewDownloadID int64  `json:"new_download_id"`
	ReleaseName   string `json:"release_name"`
	Message       string `json:"message"`
}

func (c *Client) RetryDownload(id int64) (*RetryResponse, error) {
	url := fmt.Sprintf("%s/api/v1/downloads/%d/retry", c.baseURL, id)
	resp, err := c.httpClient.Post(url, "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}

	var result RetryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
```

**Step 2: Add retry subcommand in queue.go**

```go
var queueRetryCmd = &cobra.Command{
	Use:   "retry <id>",
	Short: "Retry a failed download",
	Long:  "Re-searches indexers for the content and grabs the best matching release.",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueRetry,
}

func init() {
	// ... existing init
	queueCmd.AddCommand(queueRetryCmd)
}

func runQueueRetry(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	client := NewClient(serverURL)

	fmt.Printf("Retrying download #%d...\n", id)
	result, err := client.RetryDownload(id)
	if err != nil {
		return fmt.Errorf("retry failed: %w", err)
	}

	if jsonOutput {
		printJSON(result)
		return nil
	}

	fmt.Printf("New download queued: #%d\n", result.NewDownloadID)
	fmt.Printf("Release: %s\n", result.ReleaseName)
	return nil
}
```

**Step 3: Run build to verify**

Run: `go build ./cmd/arrgo`
Expected: Success

**Step 4: Commit**

```bash
git add cmd/arrgo/queue.go cmd/arrgo/client.go
git commit -m "feat: add arrgo queue retry subcommand (#58)"
```

---

### Task 8: Wire EventLog into server

**Files:**
- Modify: `internal/server/runner.go` or server initialization
- Modify: `cmd/arrgod/main.go`

**Step 1: Find where API deps are created**

Look for where `v1.Deps{}` is constructed and add EventLog.

**Step 2: Create EventLog and pass to deps**

```go
eventLog := events.NewEventLog(db)
// ...
deps := v1.Deps{
	// ... existing
	EventLog: eventLog,
}
```

**Step 3: Run server and test manually**

Run: `go build ./cmd/arrgod && ./arrgod`
Test: `curl http://localhost:8484/api/v1/events?limit=5`
Expected: JSON response with events

**Step 4: Commit**

```bash
git add cmd/arrgod/main.go internal/server/runner.go
git commit -m "feat: wire EventLog into API server (#58)"
```

---

### Task 9: Final verification and testing

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Run linter**

Run: `golangci-lint run`
Expected: No issues (or acceptable warnings)

**Step 3: Manual testing**

```bash
# Build
go build ./cmd/arrgo ./cmd/arrgod

# Test events command
./arrgo events
./arrgo events --limit 5
./arrgo events --json

# Test queue show
./arrgo queue show 1

# Test queue retry (need a failed download)
./arrgo queue retry <failed-id>
```

**Step 4: Commit any fixes**

If fixes needed, commit them.
