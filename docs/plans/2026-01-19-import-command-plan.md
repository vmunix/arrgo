# Import Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `arrgo import` command to trigger file imports (tracked by download_id or manual from arbitrary path).

**Architecture:** Single API endpoint `POST /api/v1/import` handles both modes. Manual imports parse release names, find-or-create content records, create synthetic download records, then run existing Importer. CLI provides --manual and --dry-run flags.

**Tech Stack:** Go, Cobra CLI, existing `pkg/release` parser, `internal/importer`, `internal/library`, `internal/download`

---

### Task 1: Add import request/response types

**Files:**
- Modify: `internal/api/v1/types.go`

**Step 1: Add the types at the end of the file**

```go
// importRequest is the request body for POST /import.
type importRequest struct {
	// For tracked imports
	DownloadID *int64 `json:"download_id,omitempty"`
	// For manual imports
	Path    string `json:"path,omitempty"`
	Title   string `json:"title,omitempty"`
	Year    int    `json:"year,omitempty"`
	Type    string `json:"type,omitempty"`    // "movie" or "series"
	Quality string `json:"quality,omitempty"` // "1080p", "2160p", etc.
	Season  *int   `json:"season,omitempty"`  // For series
	Episode *int   `json:"episode,omitempty"` // For series
}

// importResponse is the response for POST /import.
type importResponse struct {
	FileID       int64  `json:"file_id"`
	ContentID    int64  `json:"content_id"`
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	SizeBytes    int64  `json:"size_bytes"`
	PlexNotified bool   `json:"plex_notified"`
}
```

**Step 2: Run tests to verify no compilation errors**

Run: `go build ./...`
Expected: PASS (no errors)

**Step 3: Commit**

```bash
git add internal/api/v1/types.go
git commit -m "feat(api): add import request/response types"
```

---

### Task 2: Add GetByTitleYear to library store

**Files:**
- Modify: `internal/library/content.go`
- Test: `internal/library/store_test.go`

**Step 1: Write the failing test**

Add to `internal/library/store_test.go`:

```go
func TestStore_GetByTitleYear(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add a movie
	movie := &Content{Type: ContentTypeMovie, Title: "Back to the Future", Year: 1985, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("AddContent: %v", err)
	}

	// Find it
	found, err := store.GetByTitleYear("Back to the Future", 1985)
	if err != nil {
		t.Fatalf("GetByTitleYear: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find content")
	}
	if found.ID != movie.ID {
		t.Errorf("ID = %d, want %d", found.ID, movie.ID)
	}

	// Not found
	notFound, err := store.GetByTitleYear("Nonexistent", 2000)
	if err != nil {
		t.Fatalf("GetByTitleYear: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for nonexistent content")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/library/... -run TestStore_GetByTitleYear`
Expected: FAIL with "store.GetByTitleYear undefined"

**Step 3: Implement GetByTitleYear**

Add to `internal/library/content.go` after GetContent:

```go
// GetByTitleYear finds content by title and year.
// Returns nil, nil if not found.
func (s *Store) GetByTitleYear(title string, year int) (*Content, error) {
	contents, _, err := s.ListContent(ContentFilter{Title: &title, Year: &year, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 {
		return nil, nil
	}
	return contents[0], nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/library/... -run TestStore_GetByTitleYear`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/library/content.go internal/library/store_test.go
git commit -m "feat(library): add GetByTitleYear lookup"
```

---

### Task 3: Add import endpoint to API server

**Files:**
- Modify: `internal/api/v1/api.go`
- Test: `internal/api/v1/api_test.go` (create if needed)

**Step 1: Add the Importer field to Server struct**

In `internal/api/v1/api.go`, add to Server struct (around line 26):

```go
importer  *importer.Importer
```

Add setter method after SetPlex:

```go
// SetImporter configures the importer for file imports.
func (s *Server) SetImporter(imp *importer.Importer) {
	s.importer = imp
}
```

**Step 2: Register the route**

In RegisterRoutes, add:

```go
// Import
mux.HandleFunc("POST /api/v1/import", s.importContent)
```

**Step 3: Implement the handler**

Add the importContent handler:

```go
func (s *Server) importContent(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}

	ctx := r.Context()

	// Tracked import mode
	if req.DownloadID != nil {
		s.importTracked(ctx, w, *req.DownloadID)
		return
	}

	// Manual import mode
	if req.Path != "" {
		s.importManual(ctx, w, req)
		return
	}

	writeError(w, http.StatusBadRequest, "must provide download_id or path", "MISSING_PARAMS")
}

func (s *Server) importTracked(ctx context.Context, w http.ResponseWriter, downloadID int64) {
	if s.importer == nil {
		writeError(w, http.StatusServiceUnavailable, "importer not configured", "IMPORTER_UNAVAILABLE")
		return
	}

	// Get download to find path
	dl, err := s.downloads.Get(downloadID)
	if err != nil {
		if errors.Is(err, download.ErrNotFound) {
			writeError(w, http.StatusNotFound, "download not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
		return
	}

	if dl.Status != download.StatusCompleted {
		writeError(w, http.StatusBadRequest, "download not completed", "NOT_COMPLETED")
		return
	}

	// Get path from download client status
	// For now, require path in the request for tracked imports too
	writeError(w, http.StatusNotImplemented, "tracked import requires download path lookup - not yet implemented", "NOT_IMPLEMENTED")
}

func (s *Server) importManual(ctx context.Context, w http.ResponseWriter, req importRequest) {
	if s.importer == nil {
		writeError(w, http.StatusServiceUnavailable, "importer not configured", "IMPORTER_UNAVAILABLE")
		return
	}

	// Validate required fields
	if req.Title == "" || req.Year == 0 || req.Type == "" {
		writeError(w, http.StatusBadRequest, "manual import requires title, year, and type", "MISSING_PARAMS")
		return
	}

	// Find or create content
	content, err := s.library.GetByTitleYear(req.Title, req.Year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
		return
	}

	if content == nil {
		// Create new content
		contentType := library.ContentTypeMovie
		rootPath := s.cfg.MovieRoot
		if req.Type == "series" {
			contentType = library.ContentTypeSeries
			rootPath = s.cfg.SeriesRoot
		}

		content = &library.Content{
			Type:           contentType,
			Title:          req.Title,
			Year:           req.Year,
			Status:         library.StatusWanted,
			QualityProfile: "hd", // default
			RootPath:       rootPath,
		}
		if err := s.library.AddContent(content); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
			return
		}
	}

	// Create download record for audit trail
	releaseName := filepath.Base(req.Path)
	now := time.Now()
	dl := &download.Download{
		ContentID:   content.ID,
		Client:      "manual",
		ClientID:    fmt.Sprintf("manual-%d", now.UnixNano()),
		Status:      download.StatusCompleted,
		ReleaseName: releaseName,
		Indexer:     "manual",
		CompletedAt: &now,
	}
	if err := s.downloads.Add(dl); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INTERNAL_ERROR")
		return
	}

	// Run import
	result, err := s.importer.Import(ctx, dl.ID, req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "IMPORT_FAILED")
		return
	}

	resp := importResponse{
		FileID:       result.FileID,
		ContentID:    content.ID,
		SourcePath:   result.SourcePath,
		DestPath:     result.DestPath,
		SizeBytes:    result.SizeBytes,
		PlexNotified: result.PlexNotified,
	}
	writeJSON(w, http.StatusOK, resp)
}
```

**Step 4: Add necessary imports**

Add to imports at top of api.go:

```go
"context"
"path/filepath"
"time"
```

**Step 5: Run build to verify compilation**

Run: `go build ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/v1/api.go
git commit -m "feat(api): add POST /api/v1/import endpoint"
```

---

### Task 4: Wire up importer in server main

**Files:**
- Modify: `cmd/arrgod/main.go`

**Step 1: Find where API server is created and add importer setup**

Look for where `v1.New()` is called and add:

```go
// Create importer
imp := importer.New(db, importer.Config{
	MovieRoot:      cfg.Libraries.Movies.Root,
	SeriesRoot:     cfg.Libraries.Series.Root,
	MovieTemplate:  cfg.Libraries.Movies.Naming,
	SeriesTemplate: cfg.Libraries.Series.Naming,
	PlexURL:        cfg.Notifications.Plex.URL,
	PlexToken:      cfg.Notifications.Plex.Token,
}, logger)
apiServer.SetImporter(imp)
```

**Step 2: Verify build**

Run: `go build ./cmd/arrgod`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/arrgod/main.go
git commit -m "feat(arrgod): wire up importer to API server"
```

---

### Task 5: Add Import method to CLI client

**Files:**
- Modify: `cmd/arrgo/client.go`

**Step 1: Add ImportRequest and ImportResponse types**

```go
type ImportRequest struct {
	DownloadID *int64 `json:"download_id,omitempty"`
	Path       string `json:"path,omitempty"`
	Title      string `json:"title,omitempty"`
	Year       int    `json:"year,omitempty"`
	Type       string `json:"type,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Season     *int   `json:"season,omitempty"`
	Episode    *int   `json:"episode,omitempty"`
}

type ImportResponse struct {
	FileID       int64  `json:"file_id"`
	ContentID    int64  `json:"content_id"`
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	SizeBytes    int64  `json:"size_bytes"`
	PlexNotified bool   `json:"plex_notified"`
}
```

**Step 2: Add Import method**

```go
func (c *Client) Import(req *ImportRequest) (*ImportResponse, error) {
	var resp ImportResponse
	if err := c.post("/api/v1/import", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

**Step 3: Verify build**

Run: `go build ./cmd/arrgo`
Expected: PASS

**Step 4: Commit**

```bash
git add cmd/arrgo/client.go
git commit -m "feat(cli): add Import method to client"
```

---

### Task 6: Add import CLI command

**Files:**
- Create: `cmd/arrgo/import.go`

**Step 1: Create the import command file**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/arrgo/arrgo/pkg/release"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import [download_id]",
	Short: "Import completed download into library",
	Long: `Import a completed download into the media library.

Two modes:
  arrgo import <download_id>      Import tracked download by ID
  arrgo import --manual <path>    Import files from arbitrary path

Manual import parses the release name to detect title, year, and quality.
Use --dry-run to preview what would happen without making changes.`,
	RunE: runImportCmd,
}

var (
	manualPath string
	dryRun     bool
)

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().StringVar(&manualPath, "manual", "", "Import from arbitrary path")
	importCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview import without making changes")
}

func runImportCmd(cmd *cobra.Command, args []string) error {
	// Manual import mode
	if manualPath != "" {
		return runManualImport(manualPath, dryRun)
	}

	// Tracked import mode
	if len(args) != 1 {
		return fmt.Errorf("usage: arrgo import <download_id> or arrgo import --manual <path>")
	}

	downloadID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid download_id: %w", err)
	}

	return runTrackedImport(downloadID)
}

func runManualImport(path string, dryRun bool) error {
	// Verify path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	// Get the release name (directory or file name)
	releaseName := filepath.Base(path)
	if info.IsDir() {
		// Use directory name as release name
	} else {
		// Use parent directory name if it looks like a release
		parent := filepath.Base(filepath.Dir(path))
		if parent != "." && parent != "/" {
			releaseName = parent
		}
	}

	// Parse release name
	parsed := release.Parse(releaseName)

	// Determine content type
	contentType := "movie"
	if parsed.Season > 0 || parsed.Episode > 0 {
		contentType = "series"
	}

	// Get quality string
	quality := parsed.Resolution.String()
	if quality == "" || quality == "unknown" {
		quality = "unknown"
	}

	if dryRun {
		fmt.Printf("Detected: %s (%d) [%s, %s]\n", parsed.Title, parsed.Year, contentType, quality)
		fmt.Printf("Source:   %s\n", path)
		fmt.Printf("Content:  Would find or create record\n")
		fmt.Println()
		fmt.Println("Run without --dry-run to import.")
		return nil
	}

	// Call API
	client := NewClient(serverURL)
	req := &ImportRequest{
		Path:    path,
		Title:   parsed.Title,
		Year:    parsed.Year,
		Type:    contentType,
		Quality: quality,
	}
	if parsed.Season > 0 {
		req.Season = &parsed.Season
	}
	if parsed.Episode > 0 {
		req.Episode = &parsed.Episode
	}

	resp, err := client.Import(req)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	// Human output
	plexStatus := ""
	if resp.PlexNotified {
		plexStatus = " Plex notified"
	}

	fmt.Printf("Imported: %s (%d) [%s]\n", parsed.Title, parsed.Year, quality)
	fmt.Printf("  -> %s\n", resp.DestPath)
	if plexStatus != "" {
		fmt.Printf("  %s\n", plexStatus)
	}

	return nil
}

func runTrackedImport(downloadID int64) error {
	client := NewClient(serverURL)
	req := &ImportRequest{
		DownloadID: &downloadID,
	}

	resp, err := client.Import(req)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	fmt.Printf("Imported: %s\n", resp.DestPath)
	if resp.PlexNotified {
		fmt.Println("  Plex notified")
	}

	return nil
}
```

**Step 2: Verify build**

Run: `go build ./cmd/arrgo`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/arrgo/import.go
git commit -m "feat(cli): add import command with --manual and --dry-run"
```

---

### Task 7: Add integration test for import

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Add test for manual import**

```go
func TestIntegration_ManualImport(t *testing.T) {
	// Setup test environment with temp directories
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source", "Back.To.The.Future.1985.1080p.HULU.WEB-DL.H264-PiRaTeS")
	destDir := filepath.Join(tmpDir, "movies")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fake video file
	videoFile := filepath.Join(sourceDir, "Back.To.The.Future.1985.1080p.HULU.WEB-DL.H264-PiRaTeS.mkv")
	if err := os.WriteFile(videoFile, []byte("fake video content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create test server with importer configured
	// ... (setup code using test database and importer)

	// Make import request
	req := importRequest{
		Path:    sourceDir,
		Title:   "Back to the Future",
		Year:    1985,
		Type:    "movie",
		Quality: "1080p",
	}

	// ... (execute request and verify response)
}
```

**Step 2: Run test**

Run: `go test -v ./internal/api/v1/... -run TestIntegration_ManualImport`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test(api): add integration test for manual import"
```

---

### Task 8: Manual verification with BTTF files

**Step 1: Build and start server**

```bash
task build
./arrgod &
```

**Step 2: Test dry-run**

```bash
./arrgo import --manual "/srv/data/usenet/Back.To.The.Future.1985.1080p.HULU.WEB-DL.H264-PiRaTeS" --dry-run
```

Expected output:
```
Detected: Back to the Future (1985) [movie, 1080p]
Source:   /srv/data/usenet/Back.To.The.Future.1985.1080p.HULU.WEB-DL.H264-PiRaTeS
Content:  Would find or create record

Run without --dry-run to import.
```

**Step 3: Run actual import**

```bash
./arrgo import --manual "/srv/data/usenet/Back.To.The.Future.1985.1080p.HULU.WEB-DL.H264-PiRaTeS"
```

Expected output:
```
Imported: Back to the Future (1985) [1080p]
  -> /srv/data/arrgo-test/movies/Back to the Future (1985)/Back to the Future (1985) [1080p].mkv
  Plex notified
```

**Step 4: Verify Plex picked it up**

```bash
./arrgo plex status
```

Expected: Movies count should increase by 1

**Step 5: Import BTTF Part III**

```bash
./arrgo import --manual "/srv/data/usenet/Back.to.the.Future.Part.III.1990.1080p.HULU.WEB-DL.DDP.5.1.H.264-PiRaTeS"
```
