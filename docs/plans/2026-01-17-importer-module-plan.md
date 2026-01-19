# Importer Module Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the importer module to process completed downloads: find video files, copy to library with proper naming, update database, and notify Plex.

**Architecture:** Three components: `Renamer` for filename templates/sanitization, `PlexClient` for Plex API, and `Importer` orchestrator. Uses existing `library.Store` for files, `download.Store` for downloads, and new `HistoryStore` for audit trail.

**Tech Stack:** Go stdlib (os, io, path/filepath, net/http, encoding/json), existing library/download modules, SQLite.

---

### Task 1: Error Types

**Files:**
- Create: `internal/importer/errors.go`
- Create: `internal/importer/errors_test.go`

**Step 1: Write the test**

```go
// internal/importer/errors_test.go
package importer

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	if errors.Is(ErrDownloadNotFound, ErrDownloadNotReady) {
		t.Error("errors should be distinct")
	}
	if errors.Is(ErrNoVideoFile, ErrCopyFailed) {
		t.Error("errors should be distinct")
	}

	// Verify all errors have messages
	errs := []error{
		ErrDownloadNotFound,
		ErrDownloadNotReady,
		ErrNoVideoFile,
		ErrCopyFailed,
		ErrDestinationExists,
		ErrPathTraversal,
	}
	for _, err := range errs {
		if err.Error() == "" {
			t.Errorf("error %v should have a message", err)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/importer/... -v -run TestErrors`
Expected: FAIL (undefined errors)

**Step 3: Write implementation**

```go
// internal/importer/errors.go
package importer

import "errors"

var (
	// ErrDownloadNotFound indicates the download record doesn't exist.
	ErrDownloadNotFound = errors.New("download not found")

	// ErrDownloadNotReady indicates the download is not in completed status.
	ErrDownloadNotReady = errors.New("download not in completed status")

	// ErrNoVideoFile indicates no video file was found in the download.
	ErrNoVideoFile = errors.New("no video file found in download")

	// ErrCopyFailed indicates the file copy operation failed.
	ErrCopyFailed = errors.New("failed to copy file")

	// ErrDestinationExists indicates the destination file already exists.
	ErrDestinationExists = errors.New("destination file already exists")

	// ErrPathTraversal indicates a path traversal attack was detected.
	ErrPathTraversal = errors.New("path traversal detected")
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/importer/... -v -run TestErrors`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/errors.go internal/importer/errors_test.go
git commit -m "feat(importer): add error types"
```

---

### Task 2: Filename Sanitization

**Files:**
- Create: `internal/importer/sanitize.go`
- Create: `internal/importer/sanitize_test.go`

**Step 1: Write the tests**

```go
// internal/importer/sanitize_test.go
package importer

import (
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "Movie Name", "Movie Name"},
		{"path separators", "Movie/Name\\Here", "Movie Name Here"},
		{"path traversal", "../../../etc/passwd", "etc passwd"},
		{"double dots", "Movie..Name", "Movie.Name"},
		{"illegal chars", "Movie: The *Best* <One>", "Movie The Best One"},
		{"null bytes", "Movie\x00Name", "MovieName"},
		{"multiple spaces", "Movie   Name", "Movie Name"},
		{"leading/trailing", "  .Movie Name.  ", "Movie Name"},
		{"question mark", "What?", "What"},
		{"pipe", "This|That", "This That"},
		{"quotes", `Movie "Name"`, "Movie Name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	root := "/movies"

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid subpath", "/movies/Movie (2024)/movie.mkv", false},
		{"valid nested", "/movies/A/B/C/movie.mkv", false},
		{"exact root", "/movies", false},
		{"traversal attempt", "/movies/../etc/passwd", true},
		{"outside root", "/tv/show.mkv", true},
		{"sneaky traversal", "/movies/foo/../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path, root)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q, %q) error = %v, wantErr %v", tt.path, root, err, tt.wantErr)
			}
		})
	}
}

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"movie.mkv", true},
		{"movie.MKV", true},
		{"movie.mp4", true},
		{"movie.avi", true},
		{"movie.m4v", true},
		{"movie.txt", false},
		{"movie.nfo", false},
		{"movie.srt", false},
		{".mkv", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsVideoFile(tt.path); got != tt.want {
				t.Errorf("IsVideoFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/importer/... -v -run "TestSanitize|TestValidate|TestIsVideo"`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/importer/sanitize.go
package importer

import (
	"path/filepath"
	"regexp"
	"strings"
)

// illegalChars are characters not allowed in filenames on common filesystems.
var illegalChars = regexp.MustCompile(`[<>:"/\\|?*\x00]`)

// multiSpace matches multiple consecutive spaces.
var multiSpace = regexp.MustCompile(`\s+`)

// multiDot matches multiple consecutive dots.
var multiDot = regexp.MustCompile(`\.{2,}`)

// SanitizeFilename removes or replaces characters that are unsafe for filenames.
// This prevents path traversal attacks and filesystem errors.
func SanitizeFilename(name string) string {
	// Remove null bytes
	name = strings.ReplaceAll(name, "\x00", "")

	// Replace path separators with space
	name = strings.ReplaceAll(name, "/", " ")
	name = strings.ReplaceAll(name, "\\", " ")

	// Replace illegal characters with space
	name = illegalChars.ReplaceAllString(name, " ")

	// Collapse multiple dots to single dot
	name = multiDot.ReplaceAllString(name, ".")

	// Collapse multiple spaces to single space
	name = multiSpace.ReplaceAllString(name, " ")

	// Trim leading/trailing whitespace and dots
	name = strings.Trim(name, " .")

	return name
}

// ValidatePath ensures the path is within the expected root directory.
// Returns ErrPathTraversal if the path would escape the root.
func ValidatePath(path, expectedRoot string) error {
	// Clean both paths to resolve any . or .. components
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(expectedRoot)

	// Ensure root ends with separator for prefix check
	if !strings.HasSuffix(cleanRoot, string(filepath.Separator)) {
		cleanRoot += string(filepath.Separator)
	}

	// Path must start with root (or be exactly root without trailing slash)
	if cleanPath != filepath.Clean(expectedRoot) && !strings.HasPrefix(cleanPath, cleanRoot) {
		return ErrPathTraversal
	}

	return nil
}

// VideoExtensions contains recognized video file extensions.
var VideoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".mov":  true,
	".wmv":  true,
	".ts":   true,
	".webm": true,
}

// IsVideoFile checks if the path has a video file extension.
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return VideoExtensions[ext]
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/importer/... -v -run "TestSanitize|TestValidate|TestIsVideo"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/sanitize.go internal/importer/sanitize_test.go
git commit -m "feat(importer): add filename sanitization and path validation"
```

---

### Task 3: Renamer Component

**Files:**
- Create: `internal/importer/renamer.go`
- Create: `internal/importer/renamer_test.go`

**Step 1: Write the tests**

```go
// internal/importer/renamer_test.go
package importer

import "testing"

func TestRenamer_MoviePath(t *testing.T) {
	r := NewRenamer("", "") // Use defaults

	tests := []struct {
		name    string
		title   string
		year    int
		quality string
		ext     string
		want    string
	}{
		{
			name:    "basic movie",
			title:   "The Matrix",
			year:    1999,
			quality: "1080p",
			ext:     "mkv",
			want:    "The Matrix (1999)/The Matrix (1999) - 1080p.mkv",
		},
		{
			name:    "movie with special chars",
			title:   "What If...?",
			year:    2024,
			quality: "720p",
			ext:     "mp4",
			want:    "What If (2024)/What If (2024) - 720p.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.MoviePath(tt.title, tt.year, tt.quality, tt.ext)
			if got != tt.want {
				t.Errorf("MoviePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenamer_EpisodePath(t *testing.T) {
	r := NewRenamer("", "") // Use defaults

	tests := []struct {
		name    string
		title   string
		season  int
		episode int
		quality string
		ext     string
		want    string
	}{
		{
			name:    "basic episode",
			title:   "Breaking Bad",
			season:  1,
			episode: 5,
			quality: "1080p",
			ext:     "mkv",
			want:    "Breaking Bad/Season 01/Breaking Bad - S01E05 - 1080p.mkv",
		},
		{
			name:    "double digit season",
			title:   "Supernatural",
			season:  15,
			episode: 20,
			quality: "720p",
			ext:     "mp4",
			want:    "Supernatural/Season 15/Supernatural - S15E20 - 720p.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.EpisodePath(tt.title, tt.season, tt.episode, tt.quality, tt.ext)
			if got != tt.want {
				t.Errorf("EpisodePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenamer_CustomTemplate(t *testing.T) {
	r := NewRenamer(
		"{title}/{title}.{ext}",
		"{title}/S{season:02}E{episode:02}.{ext}",
	)

	moviePath := r.MoviePath("Movie", 2024, "1080p", "mkv")
	if moviePath != "Movie/Movie.mkv" {
		t.Errorf("custom movie template: got %q", moviePath)
	}

	episodePath := r.EpisodePath("Show", 1, 5, "720p", "mkv")
	if episodePath != "Show/S01E05.mkv" {
		t.Errorf("custom episode template: got %q", episodePath)
	}
}

func TestApplyTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]any
		want     string
	}{
		{
			name:     "simple substitution",
			template: "{title} ({year})",
			vars:     map[string]any{"title": "Movie", "year": 2024},
			want:     "Movie (2024)",
		},
		{
			name:     "zero padding",
			template: "S{season:02}E{episode:02}",
			vars:     map[string]any{"season": 1, "episode": 5},
			want:     "S01E05",
		},
		{
			name:     "three digit padding",
			template: "E{episode:03}",
			vars:     map[string]any{"episode": 7},
			want:     "E007",
		},
		{
			name:     "no padding needed",
			template: "S{season:02}",
			vars:     map[string]any{"season": 12},
			want:     "S12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyTemplate(tt.template, tt.vars)
			if got != tt.want {
				t.Errorf("applyTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/importer/... -v -run "TestRenamer|TestApplyTemplate"`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/importer/renamer.go
package importer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Default naming templates.
const (
	DefaultMovieTemplate  = "{title} ({year})/{title} ({year}) - {quality}.{ext}"
	DefaultSeriesTemplate = "{title}/Season {season:02}/{title} - S{season:02}E{episode:02} - {quality}.{ext}"
)

// Renamer applies naming templates to generate file paths.
type Renamer struct {
	movieTemplate  string
	seriesTemplate string
}

// NewRenamer creates a new Renamer with the given templates.
// Empty strings use default templates.
func NewRenamer(movieTemplate, seriesTemplate string) *Renamer {
	if movieTemplate == "" {
		movieTemplate = DefaultMovieTemplate
	}
	if seriesTemplate == "" {
		seriesTemplate = DefaultSeriesTemplate
	}
	return &Renamer{
		movieTemplate:  movieTemplate,
		seriesTemplate: seriesTemplate,
	}
}

// MoviePath generates the relative path for a movie file.
func (r *Renamer) MoviePath(title string, year int, quality, ext string) string {
	title = SanitizeFilename(title)
	vars := map[string]any{
		"title":   title,
		"year":    year,
		"quality": quality,
		"ext":     ext,
	}
	return applyTemplate(r.movieTemplate, vars)
}

// EpisodePath generates the relative path for an episode file.
func (r *Renamer) EpisodePath(title string, season, episode int, quality, ext string) string {
	title = SanitizeFilename(title)
	vars := map[string]any{
		"title":   title,
		"season":  season,
		"episode": episode,
		"quality": quality,
		"ext":     ext,
	}
	return applyTemplate(r.seriesTemplate, vars)
}

// formatPattern matches {name} or {name:02} style placeholders.
var formatPattern = regexp.MustCompile(`\{(\w+)(?::(\d+))?\}`)

// applyTemplate substitutes variables into a template string.
// Supports {name} for simple substitution and {name:02} for zero-padded integers.
func applyTemplate(template string, vars map[string]any) string {
	return formatPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := formatPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		name := parts[1]
		val, ok := vars[name]
		if !ok {
			return match
		}

		// Check for format specifier (e.g., :02)
		if len(parts) >= 3 && parts[2] != "" {
			width, err := strconv.Atoi(parts[2])
			if err == nil {
				// Zero-pad integer values
				switch v := val.(type) {
				case int:
					return fmt.Sprintf("%0*d", width, v)
				case int64:
					return fmt.Sprintf("%0*d", width, v)
				}
			}
		}

		// Simple string conversion
		return fmt.Sprintf("%v", val)
	})
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/importer/... -v -run "TestRenamer|TestApplyTemplate"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/renamer.go internal/importer/renamer_test.go
git commit -m "feat(importer): add Renamer with template substitution"
```

---

### Task 4: PlexClient Component

**Files:**
- Create: `internal/importer/plex.go`
- Create: `internal/importer/plex_test.go`

**Step 1: Write the tests**

```go
// internal/importer/plex_test.go
package importer

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlexClient_GetSections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Plex-Token") != "test-token" {
			t.Error("missing or wrong token")
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
  <Directory key="2" title="TV Shows" type="show">
    <Location path="/tv"/>
  </Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	sections, err := client.GetSections(context.Background())
	if err != nil {
		t.Fatalf("GetSections: %v", err)
	}

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Key != "1" || sections[0].Title != "Movies" {
		t.Errorf("section 0: got %+v", sections[0])
	}
	if sections[0].Locations[0].Path != "/movies" {
		t.Errorf("section 0 path: got %s", sections[0].Locations[0].Path)
	}
}

func TestPlexClient_ScanPath(t *testing.T) {
	scanCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/sections" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
</MediaContainer>`))
			return
		}

		if r.URL.Path == "/library/sections/1/refresh" {
			scanCalled = true
			if r.URL.Query().Get("path") != "/movies/Test Movie (2024)" {
				t.Errorf("wrong path: %s", r.URL.Query().Get("path"))
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	err := client.ScanPath(context.Background(), "/movies/Test Movie (2024)/movie.mkv")
	if err != nil {
		t.Fatalf("ScanPath: %v", err)
	}

	if !scanCalled {
		t.Error("scan endpoint was not called")
	}
}

func TestPlexClient_ScanPath_NoMatchingSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	err := client.ScanPath(context.Background(), "/other/path/movie.mkv")
	if err == nil {
		t.Error("expected error for non-matching path")
	}
}

func TestPlexClient_ConnectionError(t *testing.T) {
	client := NewPlexClient("http://localhost:99999", "token")
	_, err := client.GetSections(context.Background())
	if err == nil {
		t.Error("expected connection error")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/importer/... -v -run "TestPlexClient"`
Expected: FAIL

**Step 3: Write implementation**

```go
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

// Section represents a Plex library section.
type Section struct {
	Key       string     `xml:"key,attr"`
	Title     string     `xml:"title,attr"`
	Type      string     `xml:"type,attr"`
	Locations []Location `xml:"Location"`
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result sectionsResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Sections, nil
}

// ScanPath triggers a partial scan of the directory containing the given file path.
func (c *PlexClient) ScanPath(ctx context.Context, filePath string) error {
	// Get directory containing the file
	dir := filepath.Dir(filePath)

	// Find the section that contains this path
	sections, err := c.GetSections(ctx)
	if err != nil {
		return fmt.Errorf("get sections: %w", err)
	}

	var sectionKey string
	for _, section := range sections {
		for _, loc := range section.Locations {
			if strings.HasPrefix(dir, loc.Path) || strings.HasPrefix(filePath, loc.Path) {
				sectionKey = section.Key
				break
			}
		}
		if sectionKey != "" {
			break
		}
	}

	if sectionKey == "" {
		return fmt.Errorf("no library section found for path: %s", filePath)
	}

	// Trigger partial scan
	scanURL := fmt.Sprintf("%s/library/sections/%s/refresh?path=%s",
		c.baseURL, sectionKey, url.QueryEscape(dir))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scanURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scan failed with status: %d", resp.StatusCode)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/importer/... -v -run "TestPlexClient"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/plex.go internal/importer/plex_test.go
git commit -m "feat(importer): add PlexClient for library scans"
```

---

### Task 5: HistoryStore Component

**Files:**
- Create: `internal/importer/history.go`
- Create: `internal/importer/history_test.go`
- Create: `internal/importer/testutil_test.go`

**Step 1: Create test utilities**

```go
// internal/importer/testutil_test.go
package importer

import (
	"database/sql"
	_ "embed"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db
}

func insertTestContent(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', ?, 2024, 'wanted', 'hd', '/movies')`,
		title,
	)
	if err != nil {
		t.Fatalf("insert test content: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}
```

Copy the schema:

```bash
mkdir -p internal/importer/testdata
cp migrations/001_initial.sql internal/importer/testdata/schema.sql
```

**Step 2: Write the tests**

```go
// internal/importer/history_test.go
package importer

import (
	"testing"
	"time"
)

func TestHistoryStore_Add(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	h := &HistoryEntry{
		ContentID: contentID,
		Event:     EventImported,
		Data:      `{"source_path": "/downloads/movie.mkv"}`,
	}

	if err := store.Add(h); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if h.ID == 0 {
		t.Error("ID should be set after Add")
	}
	if h.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestHistoryStore_List(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	// Add multiple entries
	events := []string{EventGrabbed, EventImported, EventGrabbed}
	for _, event := range events {
		h := &HistoryEntry{ContentID: contentID, Event: event, Data: "{}"}
		if err := store.Add(h); err != nil {
			t.Fatalf("Add: %v", err)
		}
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	// List all
	entries, err := store.List(HistoryFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// List by content
	entries, err = store.List(HistoryFilter{ContentID: &contentID})
	if err != nil {
		t.Fatalf("List by content: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// List by event
	event := EventGrabbed
	entries, err = store.List(HistoryFilter{Event: &event})
	if err != nil {
		t.Fatalf("List by event: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 grabbed entries, got %d", len(entries))
	}

	// List with limit
	entries, err = store.List(HistoryFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List with limit: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(entries))
	}
}

func TestHistoryStore_List_OrderByRecent(t *testing.T) {
	db := setupTestDB(t)
	store := NewHistoryStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	// Add entries
	for i := 0; i < 3; i++ {
		h := &HistoryEntry{ContentID: contentID, Event: EventImported, Data: "{}"}
		_ = store.Add(h)
		time.Sleep(time.Millisecond)
	}

	entries, _ := store.List(HistoryFilter{})

	// Should be ordered by most recent first
	for i := 1; i < len(entries); i++ {
		if entries[i].CreatedAt.After(entries[i-1].CreatedAt) {
			t.Error("entries should be ordered by most recent first")
		}
	}
}
```

**Step 3: Write implementation**

```go
// internal/importer/history.go
package importer

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Event types for history records.
const (
	EventGrabbed  = "grabbed"
	EventImported = "imported"
	EventDeleted  = "deleted"
	EventUpgraded = "upgraded"
	EventFailed   = "failed"
)

// HistoryEntry represents a history record.
type HistoryEntry struct {
	ID        int64
	ContentID int64
	EpisodeID *int64
	Event     string
	Data      string // JSON blob
	CreatedAt time.Time
}

// HistoryFilter specifies criteria for listing history.
type HistoryFilter struct {
	ContentID *int64
	EpisodeID *int64
	Event     *string
	Limit     int
}

// HistoryStore persists history records.
type HistoryStore struct {
	db *sql.DB
}

// NewHistoryStore creates a history store.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// Add inserts a new history entry.
func (s *HistoryStore) Add(h *HistoryEntry) error {
	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO history (content_id, episode_id, event, data, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		h.ContentID, h.EpisodeID, h.Event, h.Data, now,
	)
	if err != nil {
		return fmt.Errorf("insert history: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	h.ID = id
	h.CreatedAt = now
	return nil
}

// List returns history entries matching the filter.
// Results are ordered by most recent first.
func (s *HistoryStore) List(f HistoryFilter) ([]*HistoryEntry, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Event != nil {
		conditions = append(conditions, "event = ?")
		args = append(args, *f.Event)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := `SELECT id, content_id, episode_id, event, data, created_at
		FROM history ` + whereClause + ` ORDER BY created_at DESC`

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	var results []*HistoryEntry
	for rows.Next() {
		h := &HistoryEntry{}
		if err := rows.Scan(&h.ID, &h.ContentID, &h.EpisodeID, &h.Event, &h.Data, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		results = append(results, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}

	return results, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/importer/... -v -run "TestHistoryStore"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/history.go internal/importer/history_test.go internal/importer/testutil_test.go internal/importer/testdata/
git commit -m "feat(importer): add HistoryStore for audit trail"
```

---

### Task 6: File Copy Utility

**Files:**
- Create: `internal/importer/copy.go`
- Create: `internal/importer/copy_test.go`

**Step 1: Write the tests**

```go
// internal/importer/copy_test.go
package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	// Create temp directories
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(srcDir, "test.mkv")
	content := []byte("test video content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Copy file
	dstPath := filepath.Join(dstDir, "copied.mkv")
	size, err := CopyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	// Verify content
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Error("content mismatch")
	}
}

func TestCopyFile_CreatesDirectory(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.mkv")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Destination in nested directory that doesn't exist
	dstPath := filepath.Join(dstDir, "nested", "deep", "copied.mkv")
	_, err := CopyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("destination file should exist")
	}
}

func TestCopyFile_DestinationExists(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.mkv")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	dstPath := filepath.Join(dstDir, "existing.mkv")
	if err := os.WriteFile(dstPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	_, err := CopyFile(srcPath, dstPath)
	if err != ErrDestinationExists {
		t.Errorf("expected ErrDestinationExists, got %v", err)
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	dstDir := t.TempDir()
	_, err := CopyFile("/nonexistent/file.mkv", filepath.Join(dstDir, "out.mkv"))
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestFindLargestVideo(t *testing.T) {
	dir := t.TempDir()

	// Create files of different sizes
	files := map[string]int{
		"small.mkv":  100,
		"large.mkv":  1000,
		"medium.mp4": 500,
		"readme.txt": 50,
		"sample.mkv": 10, // Sample files should be skipped
	}

	for name, size := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	// Also create nested video
	nested := filepath.Join(dir, "subdir", "nested.mkv")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(nested, make([]byte, 2000), 0644); err != nil {
		t.Fatalf("create nested: %v", err)
	}

	path, size, err := FindLargestVideo(dir)
	if err != nil {
		t.Fatalf("FindLargestVideo: %v", err)
	}

	if filepath.Base(path) != "nested.mkv" {
		t.Errorf("expected nested.mkv, got %s", filepath.Base(path))
	}
	if size != 2000 {
		t.Errorf("size = %d, want 2000", size)
	}
}

func TestFindLargestVideo_NoVideos(t *testing.T) {
	dir := t.TempDir()

	// Create only non-video files
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	_, _, err := FindLargestVideo(dir)
	if err != ErrNoVideoFile {
		t.Errorf("expected ErrNoVideoFile, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/importer/... -v -run "TestCopyFile|TestFindLargest"`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/importer/copy.go
package importer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyFile copies a file from src to dst.
// Creates destination directory if it doesn't exist.
// Returns ErrDestinationExists if dst already exists.
func CopyFile(src, dst string) (int64, error) {
	// Check if destination exists
	if _, err := os.Stat(dst); err == nil {
		return 0, ErrDestinationExists
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, fmt.Errorf("%w: create directory: %v", ErrCopyFailed, err)
	}

	// Open source
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("%w: open source: %v", ErrCopyFailed, err)
	}
	defer srcFile.Close()

	// Create destination
	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("%w: create destination: %v", ErrCopyFailed, err)
	}
	defer dstFile.Close()

	// Copy content
	size, err := io.Copy(dstFile, srcFile)
	if err != nil {
		// Clean up partial file on error
		os.Remove(dst)
		return 0, fmt.Errorf("%w: copy content: %v", ErrCopyFailed, err)
	}

	// Sync to disk
	if err := dstFile.Sync(); err != nil {
		return 0, fmt.Errorf("%w: sync: %v", ErrCopyFailed, err)
	}

	return size, nil
}

// FindLargestVideo finds the largest video file in a directory tree.
// Returns ErrNoVideoFile if no video files are found.
// Skips files with "sample" in the name.
func FindLargestVideo(dir string) (string, int64, error) {
	var largestPath string
	var largestSize int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if info.IsDir() {
			return nil
		}

		// Skip non-video files
		if !IsVideoFile(path) {
			return nil
		}

		// Skip sample files
		name := strings.ToLower(info.Name())
		if strings.Contains(name, "sample") {
			return nil
		}

		// Track largest
		if info.Size() > largestSize {
			largestSize = info.Size()
			largestPath = path
		}

		return nil
	})

	if err != nil {
		return "", 0, fmt.Errorf("walk directory: %w", err)
	}

	if largestPath == "" {
		return "", 0, ErrNoVideoFile
	}

	return largestPath, largestSize, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/importer/... -v -run "TestCopyFile|TestFindLargest"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/copy.go internal/importer/copy_test.go
git commit -m "feat(importer): add file copy and video discovery utilities"
```

---

### Task 7: Importer Orchestrator

**Files:**
- Modify: `internal/importer/importer.go` (replace stub)
- Create: `internal/importer/importer_test.go`

**Step 1: Write the tests**

```go
// internal/importer/importer_test.go
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/library"
)

func setupTestImporter(t *testing.T) (*Importer, *sql.DB, string, string) {
	t.Helper()

	db := setupTestDB(t)
	downloadDir := t.TempDir()
	movieRoot := t.TempDir()

	imp := &Importer{
		downloads: download.NewStore(db),
		library:   library.NewStore(db),
		history:   NewHistoryStore(db),
		renamer:   NewRenamer("", ""),
		plex:      nil, // No Plex in tests
		movieRoot: movieRoot,
		seriesRoot: t.TempDir(),
	}

	return imp, db, downloadDir, movieRoot
}

func createTestDownload(t *testing.T, db *sql.DB, contentID int64, status download.Status) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO downloads (content_id, client, client_id, status, release_name, indexer, added_at)
		VALUES (?, 'sabnzbd', 'nzo_test', ?, 'Test.Movie.2024.1080p.BluRay', 'TestIndexer', CURRENT_TIMESTAMP)`,
		contentID, status,
	)
	if err != nil {
		t.Fatalf("create download: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func TestImporter_Import_Movie(t *testing.T) {
	imp, db, downloadDir, movieRoot := setupTestImporter(t)

	// Create content
	contentID := insertTestContent(t, db, "Test Movie")

	// Create completed download
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	// Update download with path (simulating SABnzbd completion)
	downloadPath := filepath.Join(downloadDir, "Test.Movie.2024.1080p.BluRay")
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		t.Fatalf("create download dir: %v", err)
	}

	// Create video file
	videoPath := filepath.Join(downloadPath, "test.movie.mkv")
	if err := os.WriteFile(videoPath, make([]byte, 1000), 0644); err != nil {
		t.Fatalf("create video: %v", err)
	}

	// Import
	result, err := imp.Import(context.Background(), downloadID, downloadPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Verify result
	if result.FileID == 0 {
		t.Error("FileID should be set")
	}
	if result.SourcePath != videoPath {
		t.Errorf("SourcePath = %q, want %q", result.SourcePath, videoPath)
	}
	expectedDest := filepath.Join(movieRoot, "Test Movie (2024)", "Test Movie (2024) - 1080p.mkv")
	if result.DestPath != expectedDest {
		t.Errorf("DestPath = %q, want %q", result.DestPath, expectedDest)
	}

	// Verify file was copied
	if _, err := os.Stat(result.DestPath); os.IsNotExist(err) {
		t.Error("destination file should exist")
	}

	// Verify download status updated
	var status string
	db.QueryRow("SELECT status FROM downloads WHERE id = ?", downloadID).Scan(&status)
	if status != "imported" {
		t.Errorf("download status = %q, want imported", status)
	}

	// Verify content status updated
	db.QueryRow("SELECT status FROM content WHERE id = ?", contentID).Scan(&status)
	if status != "available" {
		t.Errorf("content status = %q, want available", status)
	}

	// Verify history entry
	entries, _ := imp.history.List(HistoryFilter{ContentID: &contentID})
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	if entries[0].Event != EventImported {
		t.Errorf("history event = %q, want imported", entries[0].Event)
	}

	// Verify file record
	var filePath string
	db.QueryRow("SELECT path FROM files WHERE content_id = ?", contentID).Scan(&filePath)
	if filePath != result.DestPath {
		t.Errorf("file path = %q, want %q", filePath, result.DestPath)
	}
}

func TestImporter_Import_DownloadNotFound(t *testing.T) {
	imp, _, _, _ := setupTestImporter(t)

	_, err := imp.Import(context.Background(), 9999, "/some/path")
	if !errors.Is(err, ErrDownloadNotFound) {
		t.Errorf("expected ErrDownloadNotFound, got %v", err)
	}
}

func TestImporter_Import_NotCompleted(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	contentID := insertTestContent(t, db, "Test Movie")
	downloadID := createTestDownload(t, db, contentID, download.StatusDownloading)

	_, err := imp.Import(context.Background(), downloadID, downloadDir)
	if !errors.Is(err, ErrDownloadNotReady) {
		t.Errorf("expected ErrDownloadNotReady, got %v", err)
	}
}

func TestImporter_Import_NoVideoFile(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	contentID := insertTestContent(t, db, "Test Movie")
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	downloadPath := filepath.Join(downloadDir, "empty")
	os.MkdirAll(downloadPath, 0755)

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	if !errors.Is(err, ErrNoVideoFile) {
		t.Errorf("expected ErrNoVideoFile, got %v", err)
	}
}

func TestImporter_Import_PathTraversal(t *testing.T) {
	imp, db, downloadDir, _ := setupTestImporter(t)

	// Create content with malicious title
	result, _ := db.Exec(`
		INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', '../../../etc/passwd', 2024, 'wanted', 'hd', '/movies')`)
	contentID, _ := result.LastInsertId()

	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	downloadPath := filepath.Join(downloadDir, "download")
	os.MkdirAll(downloadPath, 0755)
	os.WriteFile(filepath.Join(downloadPath, "movie.mkv"), make([]byte, 100), 0644)

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	if !errors.Is(err, ErrPathTraversal) {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
}

func TestImporter_Import_DestinationExists(t *testing.T) {
	imp, db, downloadDir, movieRoot := setupTestImporter(t)

	contentID := insertTestContent(t, db, "Test Movie")
	downloadID := createTestDownload(t, db, contentID, download.StatusCompleted)

	// Create download with video
	downloadPath := filepath.Join(downloadDir, "download")
	os.MkdirAll(downloadPath, 0755)
	os.WriteFile(filepath.Join(downloadPath, "movie.mkv"), make([]byte, 100), 0644)

	// Pre-create destination
	destDir := filepath.Join(movieRoot, "Test Movie (2024)")
	os.MkdirAll(destDir, 0755)
	os.WriteFile(filepath.Join(destDir, "Test Movie (2024) - 1080p.mkv"), []byte("existing"), 0644)

	_, err := imp.Import(context.Background(), downloadID, downloadPath)
	if !errors.Is(err, ErrDestinationExists) {
		t.Errorf("expected ErrDestinationExists, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/importer/... -v -run "TestImporter_Import"`
Expected: FAIL

**Step 3: Write implementation**

Replace `internal/importer/importer.go`:

```go
// Package importer handles file import, renaming, and media server notification.
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/library"
)

// Importer processes completed downloads.
type Importer struct {
	downloads  *download.Store
	library    *library.Store
	history    *HistoryStore
	renamer    *Renamer
	plex       *PlexClient // nil if not configured
	movieRoot  string
	seriesRoot string
}

// Config for the importer.
type Config struct {
	MovieRoot      string
	SeriesRoot     string
	MovieTemplate  string
	SeriesTemplate string
	PlexURL        string
	PlexToken      string
}

// New creates a new importer.
func New(db *sql.DB, cfg Config) *Importer {
	var plex *PlexClient
	if cfg.PlexURL != "" && cfg.PlexToken != "" {
		plex = NewPlexClient(cfg.PlexURL, cfg.PlexToken)
	}

	return &Importer{
		downloads:  download.NewStore(db),
		library:    library.NewStore(db),
		history:    NewHistoryStore(db),
		renamer:    NewRenamer(cfg.MovieTemplate, cfg.SeriesTemplate),
		plex:       plex,
		movieRoot:  cfg.MovieRoot,
		seriesRoot: cfg.SeriesRoot,
	}
}

// ImportResult is the result of an import operation.
type ImportResult struct {
	FileID       int64
	SourcePath   string
	DestPath     string
	SizeBytes    int64
	Quality      string
	PlexNotified bool
	PlexError    error
}

// Import processes a completed download.
func (i *Importer) Import(ctx context.Context, downloadID int64, downloadPath string) (*ImportResult, error) {
	// Get download record
	dl, err := i.downloads.Get(downloadID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("%w: %v", ErrDownloadNotFound, err)
		}
		return nil, fmt.Errorf("get download: %w", err)
	}

	// Verify download is completed
	if dl.Status != download.StatusCompleted {
		return nil, fmt.Errorf("%w: status is %s", ErrDownloadNotReady, dl.Status)
	}

	// Get content record
	content, err := i.library.GetContent(dl.ContentID)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}

	// Find largest video file
	srcPath, srcSize, err := FindLargestVideo(downloadPath)
	if err != nil {
		return nil, err
	}

	// Extract quality from release name (simplified - just look for resolution)
	quality := extractQuality(dl.ReleaseName)

	// Build destination path
	ext := strings.TrimPrefix(filepath.Ext(srcPath), ".")
	var relPath string
	var root string

	if content.Type == library.ContentTypeMovie {
		relPath = i.renamer.MoviePath(content.Title, content.Year, quality, ext)
		root = i.movieRoot
	} else {
		// For series, we'd need episode info - simplified for now
		return nil, fmt.Errorf("series import not yet implemented")
	}

	destPath := filepath.Join(root, relPath)

	// Validate path is within root (security check)
	if err := ValidatePath(destPath, root); err != nil {
		return nil, err
	}

	// Copy file
	size, err := CopyFile(srcPath, destPath)
	if err != nil {
		return nil, err
	}

	// Update database in transaction
	tx, err := i.library.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert file record
	file := &library.File{
		ContentID: content.ID,
		Path:      destPath,
		SizeBytes: size,
		Quality:   quality,
		Source:    dl.Indexer,
	}
	if err := tx.AddFile(file); err != nil {
		return nil, fmt.Errorf("add file: %w", err)
	}

	// Update content status
	content.Status = library.StatusAvailable
	content.UpdatedAt = time.Now()
	if err := tx.UpdateContent(content); err != nil {
		return nil, fmt.Errorf("update content: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Update download status (separate from library transaction)
	dl.Status = download.StatusImported
	now := time.Now()
	dl.CompletedAt = &now
	if err := i.downloads.Update(dl); err != nil {
		// Log but don't fail - file is already imported
	}

	// Add history entry
	historyData, _ := json.Marshal(map[string]any{
		"source_path":  srcPath,
		"dest_path":    destPath,
		"size_bytes":   size,
		"quality":      quality,
		"indexer":      dl.Indexer,
		"release_name": dl.ReleaseName,
	})
	_ = i.history.Add(&HistoryEntry{
		ContentID: content.ID,
		Event:     EventImported,
		Data:      string(historyData),
	})

	result := &ImportResult{
		FileID:     file.ID,
		SourcePath: srcPath,
		DestPath:   destPath,
		SizeBytes:  size,
		Quality:    quality,
	}

	// Notify Plex (best effort)
	if i.plex != nil {
		if err := i.plex.ScanPath(ctx, destPath); err != nil {
			result.PlexError = err
		} else {
			result.PlexNotified = true
		}
	}

	return result, nil
}

// extractQuality extracts resolution from a release name.
func extractQuality(releaseName string) string {
	lower := strings.ToLower(releaseName)
	switch {
	case strings.Contains(lower, "2160p") || strings.Contains(lower, "4k"):
		return "2160p"
	case strings.Contains(lower, "1080p"):
		return "1080p"
	case strings.Contains(lower, "720p"):
		return "720p"
	case strings.Contains(lower, "480p"):
		return "480p"
	default:
		return "unknown"
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/importer/... -v -run "TestImporter_Import"`
Expected: PASS

**Step 5: Run linter**

Run: `golangci-lint run ./internal/importer/...`
Expected: No issues

**Step 6: Commit**

```bash
git add internal/importer/importer.go internal/importer/importer_test.go
git commit -m "feat(importer): implement Import orchestrator"
```

---

### Task 8: Final Verification

**Step 1: Run all tests**

Run: `go test ./internal/importer/... -v`
Expected: All tests PASS

**Step 2: Run linter**

Run: `golangci-lint run ./internal/importer/...`
Expected: No issues

**Step 3: Verify build**

Run: `go build ./...`
Expected: Success

**Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

---

## Summary

| Task | Component | Description |
|------|-----------|-------------|
| 1 | errors.go | Error types |
| 2 | sanitize.go | Filename sanitization, path validation, video detection |
| 3 | renamer.go | Template-based file naming |
| 4 | plex.go | Plex API client for partial scans |
| 5 | history.go | HistoryStore for audit trail |
| 6 | copy.go | File copy utility, largest video finder |
| 7 | importer.go | Main Import orchestrator |
| 8 | - | Final verification |
