# Plex Status Command Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `arrgo plex status` command to verify Plex connection and display library information.

**Architecture:** CLI command calls `/api/v1/plex/status` endpoint, which uses PlexClient to query Plex API for identity and library sections.

**Tech Stack:** Go, Cobra CLI, net/http, encoding/xml

---

## Task 1: Add GetIdentity to PlexClient

**Files:**
- Modify: `internal/importer/plex.go`
- Modify: `internal/importer/plex_test.go`

**Step 1: Write the failing test**

Add to `internal/importer/plex_test.go`:

```go
func TestPlexClient_GetIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Plex-Token") != "test-token" {
			t.Errorf("missing or wrong token")
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="0" machineIdentifier="abc123" version="1.42.2.10156">
<Server name="velcro"/>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	identity, err := client.GetIdentity(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.Name != "velcro" {
		t.Errorf("name: got %q, want %q", identity.Name, "velcro")
	}
	if identity.Version != "1.42.2.10156" {
		t.Errorf("version: got %q, want %q", identity.Version, "1.42.2.10156")
	}
}

func TestPlexClient_GetIdentity_ConnectionError(t *testing.T) {
	client := NewPlexClient("http://localhost:1", "test-token")
	_, err := client.GetIdentity(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestPlexClient_GetIdentity ./internal/importer/`
Expected: FAIL with "client.GetIdentity undefined"

**Step 3: Write minimal implementation**

Add to `internal/importer/plex.go` after the existing types:

```go
// Identity holds Plex server identity information.
type Identity struct {
	Name    string
	Version string
}

// identityResponse is the XML response from /identity.
type identityResponse struct {
	XMLName xml.Name `xml:"MediaContainer"`
	Version string   `xml:"version,attr"`
	Server  struct {
		Name string `xml:"name,attr"`
	} `xml:"Server"`
}

// GetIdentity returns the Plex server name and version.
func (c *PlexClient) GetIdentity(ctx context.Context) (*Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/identity", nil)
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
		Name:    result.Server.Name,
		Version: result.Version,
	}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestPlexClient_GetIdentity ./internal/importer/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/plex.go internal/importer/plex_test.go
git commit -m "feat(plex): add GetIdentity method to PlexClient"
```

---

## Task 2: Enhance GetSections to return item counts and scan times

**Files:**
- Modify: `internal/importer/plex.go`
- Modify: `internal/importer/plex_test.go`

**Step 1: Update Section struct and test**

The existing `Section` struct needs `ScannedAt`, `Refreshing`, and we need a way to get item counts. Update the test in `internal/importer/plex_test.go`:

```go
func TestPlexClient_GetSections_WithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2">
<Directory key="1" type="movie" title="Movies" scannedAt="1737200000" refreshing="0">
<Location path="/data/media/movies"/>
</Directory>
<Directory key="2" type="show" title="TV Shows" scannedAt="1737100000" refreshing="1">
<Location path="/data/media/tv"/>
</Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	sections, err := client.GetSections(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("sections: got %d, want 2", len(sections))
	}
	if sections[0].ScannedAt != 1737200000 {
		t.Errorf("scannedAt: got %d, want 1737200000", sections[0].ScannedAt)
	}
	if sections[1].Refreshing != true {
		t.Errorf("refreshing: got %v, want true", sections[1].Refreshing)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestPlexClient_GetSections_WithMetadata ./internal/importer/`
Expected: FAIL (ScannedAt field doesn't exist)

**Step 3: Update Section struct**

In `internal/importer/plex.go`, update the `Section` struct:

```go
// Section represents a Plex library section.
type Section struct {
	Key        string     `xml:"key,attr"`
	Title      string     `xml:"title,attr"`
	Type       string     `xml:"type,attr"`
	Locations  []Location `xml:"Location"`
	ScannedAt  int64      `xml:"scannedAt,attr"`
	Refreshing bool       `xml:"refreshing,attr"`
}
```

Note: The `refreshing` attr is "0" or "1", but Go's xml package will parse "1" as true for bool.

Actually, XML doesn't parse "0"/"1" as bool directly. We need a custom type or parse it differently. Let's use int and convert:

```go
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
```

Update the test to use `sections[1].Refreshing()` instead of `sections[1].Refreshing`.

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestPlexClient_GetSections ./internal/importer/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/plex.go internal/importer/plex_test.go
git commit -m "feat(plex): add ScannedAt and Refreshing to Section"
```

---

## Task 3: Add GetLibraryCount method

**Files:**
- Modify: `internal/importer/plex.go`
- Modify: `internal/importer/plex_test.go`

**Step 1: Write the failing test**

```go
func TestPlexClient_GetLibraryCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections/1/all" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="0" totalSize="42">
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	count, err := client.GetLibraryCount(context.Background(), "1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 42 {
		t.Errorf("count: got %d, want 42", count)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestPlexClient_GetLibraryCount ./internal/importer/`
Expected: FAIL with "client.GetLibraryCount undefined"

**Step 3: Write minimal implementation**

```go
// countResponse is the XML response for getting library item count.
type countResponse struct {
	XMLName   xml.Name `xml:"MediaContainer"`
	TotalSize int      `xml:"totalSize,attr"`
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

	return result.TotalSize, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestPlexClient_GetLibraryCount ./internal/importer/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/plex.go internal/importer/plex_test.go
git commit -m "feat(plex): add GetLibraryCount method"
```

---

## Task 4: Add Plex status API endpoint

**Files:**
- Modify: `internal/api/v1/api.go`
- Modify: `internal/api/v1/types.go`

**Step 1: Add types to types.go**

Add to `internal/api/v1/types.go`:

```go
// plexStatusResponse is the response for GET /plex/status.
type plexStatusResponse struct {
	Connected  bool          `json:"connected"`
	ServerName string        `json:"server_name,omitempty"`
	Version    string        `json:"version,omitempty"`
	Libraries  []plexLibrary `json:"libraries,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// plexLibrary represents a Plex library section.
type plexLibrary struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	ItemCount  int    `json:"item_count"`
	Location   string `json:"location"`
	ScannedAt  int64  `json:"scanned_at"`
	Refreshing bool   `json:"refreshing"`
}
```

**Step 2: Add route registration**

In `internal/api/v1/api.go`, add to `RegisterRoutes`:

```go
	// Plex
	mux.HandleFunc("GET /api/v1/plex/status", s.getPlexStatus)
```

**Step 3: Add handler**

Add to `internal/api/v1/api.go`:

```go
func (s *Server) getPlexStatus(w http.ResponseWriter, r *http.Request) {
	resp := plexStatusResponse{}

	if s.plex == nil {
		resp.Error = "Plex not configured"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	ctx := r.Context()

	// Get identity
	identity, err := s.plex.GetIdentity(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("connection failed: %v", err)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Connected = true
	resp.ServerName = identity.Name
	resp.Version = identity.Version

	// Get sections
	sections, err := s.plex.GetSections(ctx)
	if err != nil {
		resp.Error = fmt.Sprintf("failed to get libraries: %v", err)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Libraries = make([]plexLibrary, len(sections))
	for i, sec := range sections {
		location := ""
		if len(sec.Locations) > 0 {
			location = sec.Locations[0].Path
		}

		count, _ := s.plex.GetLibraryCount(ctx, sec.Key)

		resp.Libraries[i] = plexLibrary{
			Key:        sec.Key,
			Title:      sec.Title,
			Type:       sec.Type,
			ItemCount:  count,
			Location:   location,
			ScannedAt:  sec.ScannedAt,
			Refreshing: sec.Refreshing(),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Run all tests**

Run: `go test ./internal/api/v1/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/types.go
git commit -m "feat(api): add GET /api/v1/plex/status endpoint"
```

---

## Task 5: Add PlexStatus to CLI client

**Files:**
- Modify: `cmd/arrgo/client.go`

**Step 1: Add response type and method**

Add to `cmd/arrgo/client.go`:

```go
type PlexLibrary struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	ItemCount  int    `json:"item_count"`
	Location   string `json:"location"`
	ScannedAt  int64  `json:"scanned_at"`
	Refreshing bool   `json:"refreshing"`
}

type PlexStatusResponse struct {
	Connected  bool          `json:"connected"`
	ServerName string        `json:"server_name,omitempty"`
	Version    string        `json:"version,omitempty"`
	Libraries  []PlexLibrary `json:"libraries,omitempty"`
	Error      string        `json:"error,omitempty"`
}

func (c *Client) PlexStatus() (*PlexStatusResponse, error) {
	var resp PlexStatusResponse
	if err := c.get("/api/v1/plex/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

**Step 2: Run build to verify**

Run: `go build ./cmd/arrgo/`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/arrgo/client.go
git commit -m "feat(cli): add PlexStatus client method"
```

---

## Task 6: Add plex command group and status subcommand

**Files:**
- Create: `cmd/arrgo/plex.go`

**Step 1: Create the command file**

Create `cmd/arrgo/plex.go`:

```go
package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var plexCmd = &cobra.Command{
	Use:   "plex",
	Short: "Plex media server commands",
}

var plexStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Plex connection status and libraries",
	RunE:  runPlexStatusCmd,
}

func init() {
	rootCmd.AddCommand(plexCmd)
	plexCmd.AddCommand(plexStatusCmd)
}

func runPlexStatusCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	status, err := client.PlexStatus()
	if err != nil {
		return fmt.Errorf("plex status failed: %w", err)
	}

	if jsonOutput {
		printJSON(status)
		return nil
	}

	printPlexStatusHuman(status)
	return nil
}

func printPlexStatusHuman(s *PlexStatusResponse) {
	if s.Error != "" && !s.Connected {
		if s.Error == "Plex not configured" {
			fmt.Println("Plex: not configured")
			fmt.Println()
			fmt.Println("Configure in config.toml:")
			fmt.Println("  [notifications.plex]")
			fmt.Println("  url = \"http://localhost:32400\"")
			fmt.Println("  token = \"your-token\"")
		} else {
			fmt.Printf("Plex: connection failed ✗\n")
			fmt.Printf("  Error: %s\n", s.Error)
		}
		return
	}

	fmt.Printf("Plex: %s (%s) ✓\n", s.ServerName, s.Version)
	fmt.Println()

	if len(s.Libraries) == 0 {
		fmt.Println("No libraries found")
		return
	}

	fmt.Println("Libraries:")
	for _, lib := range s.Libraries {
		status := ""
		if lib.Refreshing {
			status = " (scanning)"
		}

		scannedAgo := formatTimeAgo(lib.ScannedAt)

		fmt.Printf("  %-12s %4d items   %-24s scanned %s%s\n",
			lib.Title, lib.ItemCount, lib.Location, scannedAgo, status)
	}
}

func formatTimeAgo(unixTime int64) string {
	if unixTime == 0 {
		return "never"
	}

	t := time.Unix(unixTime, 0)
	ago := time.Since(t)

	switch {
	case ago < time.Minute:
		return "just now"
	case ago < time.Hour:
		mins := int(ago.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case ago < 24*time.Hour:
		hours := int(ago.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(ago.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
```

**Step 2: Build and test**

Run: `go build ./cmd/arrgo/ && ./arrgo plex --help`
Expected: Shows plex subcommands including status

**Step 3: Commit**

```bash
git add cmd/arrgo/plex.go
git commit -m "feat(cli): add plex command group with status subcommand"
```

---

## Task 7: Add integration test

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add test for plex status endpoint**

Add to `internal/api/v1/integration_test.go`:

```go
func TestIntegration_PlexStatus_NotConfigured(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// Don't configure Plex client
	req := httptest.NewRequest("GET", "/api/v1/plex/status", nil)
	rr := httptest.NewRecorder()

	env.server.RegisterRoutes(env.mux)
	env.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["connected"] != false {
		t.Errorf("connected: got %v, want false", resp["connected"])
	}
	if resp["error"] != "Plex not configured" {
		t.Errorf("error: got %v, want 'Plex not configured'", resp["error"])
	}
}
```

**Step 2: Run test**

Run: `go test -v -run TestIntegration_PlexStatus ./internal/api/v1/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test(api): add integration test for plex status endpoint"
```

---

## Task 8: Manual verification

**Step 1: Start the server**

Run: `go run ./cmd/arrgod/`

**Step 2: Test the CLI command**

Run in another terminal: `./arrgo plex status`

Expected output (with Plex configured):
```
Plex: velcro (1.42.2.10156) ✓

Libraries:
  Movies        32 items   /data/media/movies       scanned 2h ago
  TV Shows       6 items   /data/media/tv           scanned 1h ago
```

**Step 3: Test JSON output**

Run: `./arrgo plex status --json`

Expected: JSON object with connected, server_name, version, libraries array

**Step 4: Run full test suite**

Run: `task check`
Expected: All checks pass

**Step 5: Final commit**

If any adjustments were made during manual testing, commit them.
