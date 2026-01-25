# Library Import Implementation Plan

> **Status:** ✅ COMPLETED (2026-01-25) - Issue #50

**Goal:** Add CLI command and API endpoint to bulk import existing Plex libraries into arrgo.

**Architecture:** New `arrgo library import --from-plex <library>` command calls `POST /api/v1/library/import` which queries Plex, stats files, parses quality, and creates content + file records.

**Tech Stack:** Go, Cobra CLI, existing PlexClient, pkg/release for quality parsing.

---

### Task 1: Add translateToLocal path helper to PlexClient

**Files:**
- Modify: `internal/importer/plex.go`
- Test: `internal/importer/plex_test.go`

**Step 1: Write the failing test**

```go
func TestPlexClient_TranslateToLocal(t *testing.T) {
	client := NewPlexClientWithPathMapping(
		"http://plex:32400",
		"token",
		"/srv/media",      // local
		"/data/media",     // remote (Plex sees this)
		nil,
	)

	tests := []struct {
		remote string
		local  string
	}{
		{"/data/media/movies/Test.mkv", "/srv/media/movies/Test.mkv"},
		{"/data/media/tv/Show/S01E01.mkv", "/srv/media/tv/Show/S01E01.mkv"},
		{"/other/path/file.mkv", "/other/path/file.mkv"}, // no match, unchanged
	}

	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			result := client.TranslateToLocal(tt.remote)
			assert.Equal(t, tt.local, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/importer/... -run TestPlexClient_TranslateToLocal -v`
Expected: FAIL with "client.TranslateToLocal undefined"

**Step 3: Write minimal implementation**

Add to `internal/importer/plex.go` after `translateToRemote`:

```go
// TranslateToLocal converts a Plex path to the local filesystem path.
func (c *PlexClient) TranslateToLocal(path string) string {
	if c.localPath == "" || c.remotePath == "" {
		return path
	}
	if strings.HasPrefix(path, c.remotePath) {
		return c.localPath + path[len(c.remotePath):]
	}
	return path
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/importer/... -run TestPlexClient_TranslateToLocal -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/plex.go internal/importer/plex_test.go
git commit -m "feat(plex): add TranslateToLocal path helper"
```

---

### Task 2: Add import types to API

**Files:**
- Modify: `internal/api/v1/types.go`

**Step 1: Add request/response types**

Add to `internal/api/v1/types.go`:

```go
// libraryImportRequest is the request body for POST /library/import.
type libraryImportRequest struct {
	Source          string `json:"source"`                     // "plex"
	Library         string `json:"library"`                    // Plex library name
	QualityOverride string `json:"quality_override,omitempty"` // Override parsed quality
	DryRun          bool   `json:"dry_run,omitempty"`
}

// libraryImportItem represents a single imported or skipped item.
type libraryImportItem struct {
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Quality   string `json:"quality,omitempty"`
	ContentID int64  `json:"content_id,omitempty"`
	Reason    string `json:"reason,omitempty"` // for skipped items
	Error     string `json:"error,omitempty"`  // for errored items
}

// libraryImportResponse is the response for POST /library/import.
type libraryImportResponse struct {
	Imported []libraryImportItem `json:"imported"`
	Skipped  []libraryImportItem `json:"skipped"`
	Errors   []libraryImportItem `json:"errors"`
	Summary  struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
		Errors   int `json:"errors"`
	} `json:"summary"`
}
```

**Step 2: Verify it compiles**

Run: `go build ./internal/api/v1/...`
Expected: Success, no errors

**Step 3: Commit**

```bash
git add internal/api/v1/types.go
git commit -m "feat(api): add library import request/response types"
```

---

### Task 3: Add import handler with validation

**Files:**
- Modify: `internal/api/v1/api.go`
- Test: `internal/api/v1/api_test.go`

**Step 1: Write the failing test**

Add to `internal/api/v1/api_test.go`:

```go
func TestLibraryImport_ValidationErrors(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing source",
			body:     `{"library": "Movies"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "source is required",
		},
		{
			name:     "invalid source",
			body:     `{"source": "invalid", "library": "Movies"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "unsupported source",
		},
		{
			name:     "missing library",
			body:     `{"source": "plex"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "library is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.mux.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantErr)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_ValidationErrors -v`
Expected: FAIL with 404 (route not found)

**Step 3: Write minimal implementation**

Add route in `RegisterRoutes` in `internal/api/v1/api.go`:

```go
mux.HandleFunc("POST /api/v1/library/import", s.importLibrary)
```

Add handler:

```go
func (s *Server) importLibrary(w http.ResponseWriter, r *http.Request) {
	var req libraryImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// Validate source
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "MISSING_SOURCE", "source is required")
		return
	}
	if req.Source != "plex" {
		writeError(w, http.StatusBadRequest, "INVALID_SOURCE", "unsupported source: "+req.Source)
		return
	}

	// Validate library
	if req.Library == "" {
		writeError(w, http.StatusBadRequest, "MISSING_LIBRARY", "library is required")
		return
	}

	// TODO: implement import logic
	writeJSON(w, http.StatusOK, libraryImportResponse{
		Imported: []libraryImportItem{},
		Skipped:  []libraryImportItem{},
		Errors:   []libraryImportItem{},
	})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_ValidationErrors -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go
git commit -m "feat(api): add library import endpoint with validation"
```

---

### Task 4: Implement Plex import logic

**Files:**
- Modify: `internal/api/v1/api.go`
- Test: `internal/api/v1/api_test.go`

**Step 1: Write the failing test**

Add to `internal/api/v1/api_test.go`:

```go
func TestLibraryImport_PlexNotConfigured(t *testing.T) {
	srv := newTestServerWithoutPlex(t) // helper that doesn't set up Plex

	body := `{"source": "plex", "library": "Movies"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Plex not configured")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_PlexNotConfigured -v`
Expected: FAIL (returns 200 instead of 503)

**Step 3: Update handler to check Plex**

Update `importLibrary` in `internal/api/v1/api.go`:

```go
func (s *Server) importLibrary(w http.ResponseWriter, r *http.Request) {
	var req libraryImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// Validate source
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "MISSING_SOURCE", "source is required")
		return
	}
	if req.Source != "plex" {
		writeError(w, http.StatusBadRequest, "INVALID_SOURCE", "unsupported source: "+req.Source)
		return
	}

	// Validate library
	if req.Library == "" {
		writeError(w, http.StatusBadRequest, "MISSING_LIBRARY", "library is required")
		return
	}

	// Check Plex is configured
	if s.deps.Plex == nil {
		writeError(w, http.StatusServiceUnavailable, "PLEX_NOT_CONFIGURED", "Plex not configured")
		return
	}

	// Find the library section
	section, err := s.deps.Plex.FindSectionByName(r.Context(), req.Library)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}
	if section == nil {
		writeError(w, http.StatusNotFound, "LIBRARY_NOT_FOUND", "Plex library not found: "+req.Library)
		return
	}

	// Get all items from library
	items, err := s.deps.Plex.ListLibraryItems(r.Context(), section.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PLEX_ERROR", err.Error())
		return
	}

	resp := s.processPlexImport(r.Context(), items, req.QualityOverride, req.DryRun)
	writeJSON(w, http.StatusOK, resp)
}
```

Add the processing function (stub for now):

```go
func (s *Server) processPlexImport(ctx context.Context, items []importer.PlexItem, qualityOverride string, dryRun bool) libraryImportResponse {
	resp := libraryImportResponse{
		Imported: []libraryImportItem{},
		Skipped:  []libraryImportItem{},
		Errors:   []libraryImportItem{},
	}
	// TODO: process items
	return resp
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_PlexNotConfigured -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go
git commit -m "feat(api): add Plex library lookup for import"
```

---

### Task 5: Implement item processing with quality parsing

**Files:**
- Modify: `internal/api/v1/api.go`
- Test: `internal/api/v1/api_test.go`

**Step 1: Write the failing test**

Add to `internal/api/v1/api_test.go`:

```go
func TestLibraryImport_DryRun(t *testing.T) {
	srv := newTestServerWithMockPlex(t, []importer.PlexItem{
		{Title: "Test Movie", Year: 2024, Type: "movie", FilePath: "/data/media/movies/Test.Movie.2024.2160p.BluRay.mkv"},
	})

	body := `{"source": "plex", "library": "Movies", "dry_run": true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Dry run should show what would be imported
	assert.Len(t, resp.Imported, 1)
	assert.Equal(t, "Test Movie", resp.Imported[0].Title)
	assert.Equal(t, 2024, resp.Imported[0].Year)
	assert.Equal(t, "uhd", resp.Imported[0].Quality) // parsed from 2160p

	// But content should NOT be created
	contents, _, _ := srv.library.ListContent(library.ContentFilter{})
	assert.Empty(t, contents)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_DryRun -v`
Expected: FAIL (empty imported list)

**Step 3: Implement processPlexImport**

Update in `internal/api/v1/api.go`. Add import for release package:

```go
import (
	// ... existing imports
	"github.com/vmunix/arrgo/pkg/release"
)
```

Implement the function:

```go
func (s *Server) processPlexImport(ctx context.Context, items []importer.PlexItem, qualityOverride string, dryRun bool) libraryImportResponse {
	resp := libraryImportResponse{
		Imported: []libraryImportItem{},
		Skipped:  []libraryImportItem{},
		Errors:   []libraryImportItem{},
	}

	for _, item := range items {
		// Map Plex type to our type
		contentType := library.ContentTypeMovie
		if item.Type == "show" {
			contentType = library.ContentTypeSeries
		}

		// Check if already tracked
		existing, _, _ := s.deps.Library.ListContent(library.ContentFilter{
			Type:  &contentType,
			Title: item.Title,
			Year:  &item.Year,
			Limit: 1,
		})
		if len(existing) > 0 {
			resp.Skipped = append(resp.Skipped, libraryImportItem{
				Title:     item.Title,
				Year:      item.Year,
				Type:      string(contentType),
				ContentID: existing[0].ID,
				Reason:    "already tracked",
			})
			continue
		}

		// Parse quality from filename
		quality := "hd" // default
		if item.FilePath != "" {
			parsed := release.Parse(filepath.Base(item.FilePath))
			quality = mapResolutionToProfile(parsed.Resolution)
		}
		if qualityOverride != "" {
			quality = qualityOverride
		}

		importItem := libraryImportItem{
			Title:   item.Title,
			Year:    item.Year,
			Type:    string(contentType),
			Quality: quality,
		}

		if !dryRun {
			// Create content and file records
			contentID, err := s.createImportedContent(ctx, item, contentType, quality)
			if err != nil {
				importItem.Error = err.Error()
				resp.Errors = append(resp.Errors, importItem)
				continue
			}
			importItem.ContentID = contentID
		}

		resp.Imported = append(resp.Imported, importItem)
	}

	resp.Summary.Imported = len(resp.Imported)
	resp.Summary.Skipped = len(resp.Skipped)
	resp.Summary.Errors = len(resp.Errors)

	return resp
}

func mapResolutionToProfile(resolution string) string {
	switch resolution {
	case "2160p", "4K":
		return "uhd"
	case "1080p":
		return "hd"
	case "720p":
		return "hd720"
	case "480p", "576p":
		return "sd"
	default:
		return "hd"
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_DryRun -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go
git commit -m "feat(api): implement import item processing with quality parsing"
```

---

### Task 6: Create content and file records

**Files:**
- Modify: `internal/api/v1/api.go`
- Test: `internal/api/v1/api_test.go`

**Step 1: Write the failing test**

```go
func TestLibraryImport_CreatesRecords(t *testing.T) {
	srv := newTestServerWithMockPlex(t, []importer.PlexItem{
		{Title: "New Movie", Year: 2024, Type: "movie", FilePath: "/data/media/movies/New.Movie.2024.1080p.BluRay.mkv"},
	})

	// Mock file stat (or use real file in test)
	body := `{"source": "plex", "library": "Movies"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Imported, 1)
	assert.NotZero(t, resp.Imported[0].ContentID)

	// Verify content was created
	content, err := srv.library.GetContent(resp.Imported[0].ContentID)
	require.NoError(t, err)
	assert.Equal(t, "New Movie", content.Title)
	assert.Equal(t, 2024, content.Year)
	assert.Equal(t, library.StatusAvailable, content.Status)
	assert.Equal(t, "hd", content.QualityProfile)

	// Verify file was created
	files, _ := srv.library.GetFilesForContent(resp.Imported[0].ContentID)
	require.Len(t, files, 1)
	assert.Equal(t, "1080p", files[0].Quality)
	assert.Equal(t, "plex-import", files[0].Source)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_CreatesRecords -v`
Expected: FAIL (createImportedContent undefined or not creating records)

**Step 3: Implement createImportedContent**

Add to `internal/api/v1/api.go`:

```go
func (s *Server) createImportedContent(ctx context.Context, item importer.PlexItem, contentType library.ContentType, qualityProfile string) (int64, error) {
	// Translate Plex path to local path
	localPath := item.FilePath
	if s.deps.Plex != nil {
		localPath = s.deps.Plex.TranslateToLocal(item.FilePath)
	}

	// Stat file to get size
	var fileSize int64
	if localPath != "" {
		info, err := os.Stat(localPath)
		if err != nil {
			return 0, fmt.Errorf("cannot access file: %w", err)
		}
		fileSize = info.Size()
	}

	// Derive root path from file path
	rootPath := filepath.Dir(filepath.Dir(localPath)) // /movies/Title (Year)/file.mkv -> /movies

	// Create content record
	content := &library.Content{
		Type:           contentType,
		Title:          item.Title,
		Year:           item.Year,
		Status:         library.StatusAvailable,
		QualityProfile: qualityProfile,
		RootPath:       rootPath,
	}

	if err := s.deps.Library.AddContent(content); err != nil {
		return 0, fmt.Errorf("create content: %w", err)
	}

	// Create file record (for movies only - series don't have single file)
	if contentType == library.ContentTypeMovie && localPath != "" {
		parsed := release.Parse(filepath.Base(localPath))
		file := &library.File{
			ContentID: content.ID,
			Path:      localPath,
			SizeBytes: fileSize,
			Quality:   parsed.Resolution,
			Source:    "plex-import",
		}
		if err := s.deps.Library.AddFile(file); err != nil {
			// Log but don't fail - content was created
			s.log.Warn("failed to create file record", "error", err, "content_id", content.ID)
		}
	}

	return content.ID, nil
}
```

Add os import if not present.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_CreatesRecords -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api.go internal/api/v1/api_test.go
git commit -m "feat(api): create content and file records for import"
```

---

### Task 7: Add CLI command

**Files:**
- Modify: `cmd/arrgo/library.go`

**Step 1: Add the import subcommand**

Add to `init()` in `cmd/arrgo/library.go`:

```go
importCmd := &cobra.Command{
	Use:   "import",
	Short: "Import existing media library",
	Long:  "Import untracked items from Plex or a directory into arrgo for tracking.",
	RunE:  runLibraryImport,
}

importCmd.Flags().String("from-plex", "", "Import from Plex library by name")
importCmd.Flags().String("quality", "", "Override quality profile for all imports")
importCmd.Flags().Bool("dry-run", false, "Preview import without making changes")

libraryCmd.AddCommand(importCmd)
```

**Step 2: Implement the handler**

Add to `cmd/arrgo/library.go`:

```go
// LibraryImportResponse matches the API response for library import.
type LibraryImportResponse struct {
	Imported []struct {
		Title     string `json:"title"`
		Year      int    `json:"year"`
		Type      string `json:"type"`
		Quality   string `json:"quality"`
		ContentID int64  `json:"content_id"`
	} `json:"imported"`
	Skipped []struct {
		Title     string `json:"title"`
		Year      int    `json:"year"`
		ContentID int64  `json:"content_id"`
		Reason    string `json:"reason"`
	} `json:"skipped"`
	Errors []struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		Error string `json:"error"`
	} `json:"errors"`
	Summary struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
		Errors   int `json:"errors"`
	} `json:"summary"`
}

func runLibraryImport(cmd *cobra.Command, args []string) error {
	plexLibrary, _ := cmd.Flags().GetString("from-plex")
	quality, _ := cmd.Flags().GetString("quality")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if plexLibrary == "" {
		return fmt.Errorf("--from-plex is required")
	}

	// Build request
	reqBody := map[string]any{
		"source":  "plex",
		"library": plexLibrary,
		"dry_run": dryRun,
	}
	if quality != "" {
		reqBody["quality_override"] = quality
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	urlStr := fmt.Sprintf("%s/api/v1/library/import", serverURL)
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("import failed: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result LibraryImportResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if jsonOutput {
		return printJSON(result)
	}

	printLibraryImport(&result, plexLibrary, dryRun)
	return nil
}

func printLibraryImport(r *LibraryImportResponse, library string, dryRun bool) {
	action := "Importing"
	if dryRun {
		action = "Would import"
	}
	fmt.Printf("%s from Plex library %q...\n\n", action, library)

	for _, item := range r.Imported {
		fmt.Printf("  + %s (%d) - %s\n", item.Title, item.Year, item.Quality)
	}
	for _, item := range r.Skipped {
		fmt.Printf("  - %s (%d) - %s\n", item.Title, item.Year, item.Reason)
	}
	for _, item := range r.Errors {
		fmt.Printf("  ! %s (%d) - %s\n", item.Title, item.Year, item.Error)
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Would import: %d new, %d skipped, %d errors\n",
			r.Summary.Imported, r.Summary.Skipped, r.Summary.Errors)
	} else {
		fmt.Printf("Imported: %d new, %d skipped, %d errors\n",
			r.Summary.Imported, r.Summary.Skipped, r.Summary.Errors)
	}
}
```

**Step 3: Build and test manually**

Run: `go build -o arrgo ./cmd/arrgo && ./arrgo library import --help`
Expected: Shows import command help with flags

**Step 4: Commit**

```bash
git add cmd/arrgo/library.go
git commit -m "feat(cli): add library import command"
```

---

### Task 8: Integration test with real Plex

**Files:**
- Test: `internal/api/v1/integration_test.go`

**Step 1: Write integration test**

Add to `internal/api/v1/integration_test.go`:

```go
func TestLibraryImport_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	srv := newIntegrationTestServer(t)

	// Test dry run first
	body := `{"source": "plex", "library": "Movies", "dry_run": true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp libraryImportResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Should have found some items
	t.Logf("Dry run: %d to import, %d skipped", resp.Summary.Imported, resp.Summary.Skipped)
}
```

**Step 2: Run test**

Run: `go test ./internal/api/v1/... -run TestLibraryImport_Integration -v`
Expected: PASS (or SKIP if short mode)

**Step 3: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test(api): add library import integration test"
```

---

### Task 9: Update GitHub issue

**Step 1: Update issue #32 with progress**

```bash
gh issue comment 32 --body "Implemented library import feature:
- \`arrgo library import --from-plex Movies\` imports untracked Plex items
- Parses quality from filenames, supports \`--quality\` override
- Creates content + file records with actual file sizes
- \`--dry-run\` mode to preview
- Also fixed: Plex TV show listing was broken (now works)

Remaining for v2:
- Episode-level tracking for series
- \`--path\` directory scanning
- TMDB/TVDB metadata lookup"
```

**Step 2: Close issue if complete**

```bash
gh issue close 32
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | TranslateToLocal helper | plex.go |
| 2 | Import API types | types.go |
| 3 | Import handler + validation | api.go |
| 4 | Plex library lookup | api.go |
| 5 | Item processing + quality parsing | api.go |
| 6 | Create content/file records | api.go |
| 7 | CLI command | library.go |
| 8 | Integration test | integration_test.go |
| 9 | Update GitHub issue | - |
