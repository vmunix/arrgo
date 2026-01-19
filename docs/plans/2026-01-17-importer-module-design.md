# Importer Module Design

**Date:** 2026-01-17
**Status:** ✅ Complete

## Overview

The importer module processes completed downloads: finds video files, copies them to the library with proper naming, updates database records, and notifies Plex. It operates on-demand (no background polling) and handles all DB updates atomically.

## Architecture

Three components:

**`Importer`** — Main orchestrator
- `Import(ctx, downloadID)` — Main entry point
- Coordinates file discovery, copy, DB updates, Plex notification
- Returns ImportResult with success details or typed error

**`Renamer`** — File naming logic
- Simple variable substitution templates
- Filename sanitization for filesystem safety
- Format specifiers for zero-padding

**`PlexClient`** — HTTP client for Plex API
- Partial library scan of imported path
- Failure is warning only, doesn't block import

### Data Flow

```
Import(downloadID)
  → download.Store.Get(downloadID) → get download record
  → library.Store.GetContent(contentID) → get content metadata
  → FindVideoFiles(downloadPath) → find .mkv/.mp4/etc
  → Renamer.BuildPath(content, quality) → generate destination path
  → CopyFile(src, dest) → copy to library
  → DB transaction: insert file, update download, update content, insert history
  → PlexClient.ScanPath(destDir) → notify Plex (best-effort)
  → return ImportResult
```

## Security Considerations

**Path Traversal Prevention**

Release names and filenames from external sources (indexers, download clients) may contain malicious patterns like `../../../etc/passwd` or similar path traversal attempts.

All path operations MUST:
1. Sanitize filenames before use (remove `..`, `/`, `\`, null bytes)
2. Validate final paths are within expected root directories
3. Use `filepath.Clean()` and verify result starts with expected prefix
4. Never directly concatenate untrusted input into paths

```go
func ValidatePath(path, expectedRoot string) error {
    cleaned := filepath.Clean(path)
    if !strings.HasPrefix(cleaned, filepath.Clean(expectedRoot)) {
        return ErrPathTraversal
    }
    return nil
}
```

**Filename Sanitization**

Remove or replace:
- Path separators: `/`, `\`
- Path traversal: `..`
- Null bytes: `\x00`
- Shell metacharacters in contexts where names might be logged/displayed
- Illegal filesystem characters: `: * ? " < > |`

Apply sanitization at the boundary (when receiving data) and verify again before filesystem operations.

## Importer Component

### Struct

```go
type Importer struct {
    downloads    *download.Store
    library      *library.Store
    files        *FileStore
    history      *HistoryStore
    renamer      *Renamer
    plex         *PlexClient     // nil if not configured
    movieRoot    string
    seriesRoot   string
}
```

### ImportResult

```go
type ImportResult struct {
    FileID       int64
    SourcePath   string
    DestPath     string
    SizeBytes    int64
    Quality      string
    PlexNotified bool   // false if Plex failed or not configured
    PlexError    error  // non-nil if Plex notification failed
}
```

### Import Method Flow

1. Get download record by ID (must be status "completed")
2. Get content record (movie or series)
3. For episodes: get episode record
4. Find largest video file in download path
5. Build destination path using Renamer
6. Validate destination path is within expected root (security)
7. Create destination directory if needed
8. Copy file to destination
9. In single transaction:
   - Insert into `files` table
   - Update `downloads.status` = "imported"
   - Update `content.status` = "available"
   - Insert history record (event = "imported")
10. Attempt Plex notification (capture error but don't fail)
11. Return ImportResult with PlexError if notification failed

### Error Types

```go
var (
    ErrDownloadNotFound   = errors.New("download not found")
    ErrDownloadNotReady   = errors.New("download not in completed status")
    ErrNoVideoFile        = errors.New("no video file found in download")
    ErrCopyFailed         = errors.New("failed to copy file")
    ErrDestinationExists  = errors.New("destination file already exists")
    ErrPathTraversal      = errors.New("path traversal detected")
)
```

## Renamer Component

### Template Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `{title}` | Content title | `The Matrix` |
| `{year}` | Release year | `1999` |
| `{season}` | Season number | `1` |
| `{season:02}` | Zero-padded season | `01` |
| `{episode}` | Episode number | `5` |
| `{episode:02}` | Zero-padded episode | `05` |
| `{quality}` | Quality from release | `1080p` |
| `{ext}` | File extension (no dot) | `mkv` |

### Default Templates

```go
const (
    DefaultMovieTemplate  = "{title} ({year})/{title} ({year}) - {quality}.{ext}"
    DefaultSeriesTemplate = "{title}/Season {season:02}/{title} - S{season:02}E{episode:02} - {quality}.{ext}"
)
```

### Example Outputs

Movie: `The Matrix (1999)/The Matrix (1999) - 1080p.mkv`

Series: `Breaking Bad/Season 01/Breaking Bad - S01E05 - 720p.mkv`

### Filename Sanitization

```go
func SanitizeFilename(name string) string
```

- Replace path separators (`/`, `\`) with space
- Remove `..` sequences
- Remove null bytes
- Replace illegal chars (`: * ? " < > |`) with space
- Collapse multiple spaces
- Trim leading/trailing whitespace and dots

### Interface

```go
type Renamer struct {
    movieTemplate  string
    seriesTemplate string
}

func NewRenamer(movieTemplate, seriesTemplate string) *Renamer
func (r *Renamer) MoviePath(title string, year int, quality, ext string) string
func (r *Renamer) EpisodePath(title string, season, episode int, quality, ext string) string
```

## PlexClient Component

### Struct

```go
type PlexClient struct {
    baseURL    string
    token      string
    httpClient *http.Client
}

func NewPlexClient(baseURL, token string) *PlexClient
```

### Methods

```go
// ScanPath triggers a partial scan of the directory containing the imported file.
// Uses: GET /library/sections/{sectionID}/refresh?path={encodedPath}
func (c *PlexClient) ScanPath(ctx context.Context, path string) error

// GetSections returns library sections to find the right section ID for a path.
// Uses: GET /library/sections
func (c *PlexClient) GetSections(ctx context.Context) ([]Section, error)
```

### Section Matching

To scan a path, we need the library section ID. GetSections returns all sections with their paths. Match the import destination against section paths to find the right section ID.

## Database Stores

### FileStore

```go
type File struct {
    ID        int64
    ContentID int64
    EpisodeID *int64
    Path      string
    SizeBytes int64
    Quality   string
    Source    string
    AddedAt   time.Time
}

type FileStore struct {
    db *sql.DB
}

func (s *FileStore) Add(f *File) error
func (s *FileStore) Get(id int64) (*File, error)
func (s *FileStore) GetByPath(path string) (*File, error)
func (s *FileStore) ListByContent(contentID int64) ([]*File, error)
func (s *FileStore) Delete(id int64) error
```

### HistoryStore

```go
type HistoryEntry struct {
    ID        int64
    ContentID int64
    EpisodeID *int64
    Event     string    // "grabbed", "imported", "deleted", "upgraded", "failed"
    Data      string    // JSON blob with event details
    CreatedAt time.Time
}

type HistoryFilter struct {
    ContentID *int64
    EpisodeID *int64
    Event     *string
    Limit     int
}

type HistoryStore struct {
    db *sql.DB
}

func (s *HistoryStore) Add(h *HistoryEntry) error
func (s *HistoryStore) List(filter HistoryFilter) ([]*HistoryEntry, error)
```

### History Data JSON

For "imported" events:
```json
{
    "source_path": "/downloads/complete/Movie.2024.1080p.BluRay/movie.mkv",
    "dest_path": "/movies/Movie (2024)/Movie (2024) - 1080p.mkv",
    "size_bytes": 8500000000,
    "quality": "1080p",
    "indexer": "NZBgeek",
    "release_name": "Movie.2024.1080p.BluRay.x264-GROUP"
}
```

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Trigger model | On-demand only | Matches download module, simple, no background complexity |
| File transfer | Copy only | Common setup: SSD downloads, slow disk library |
| Post-copy cleanup | Leave source | SABnzbd handles cleanup via its own settings |
| Naming templates | Simple variables | Covers 95% of cases without template engine complexity |
| Plex notification | Partial scan | Fast, minimal Plex load |
| DB updates | All in importer | Atomic, consistent state |
| Error handling | Typed, no partial | Easy rollback, clear failure modes |
| Plex failure | Warning only | External service shouldn't block imports |

## Testing Strategy

### Unit Tests

**Renamer**
- Variable substitution
- Format specifiers (zero-padding)
- Filename sanitization (all edge cases)
- Path traversal prevention

**PlexClient**
- httptest.Server mocks
- Successful scan
- Section lookup
- Error cases

**FileStore / HistoryStore**
- In-memory SQLite
- CRUD operations

### Integration Tests

**Importer**
- Temp directories for source/dest
- Mock or real stores with test DB
- Full import flow
- Error cases: not found, not ready, no video, exists, copy fail
- Plex failure doesn't block import

## Out of Scope (v2+)

- Hardlink support (for same-filesystem setups)
- Upgrade detection (replacing lower quality files)
- Sample file detection and exclusion
- Subtitle file handling
- Multiple video files per download (season packs)
- Post-import scripts/hooks
