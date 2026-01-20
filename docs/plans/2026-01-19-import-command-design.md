# Import Command Design

**Status:** ✅ Complete

## Overview

Add `arrgo import` command to trigger file imports into the library with Plex notification.

**Two modes:**
- **Tracked**: `arrgo import <download_id>` - for downloads already in the system
- **Manual**: `arrgo import --manual <path>` - for files not tracked by arrgo

## Command Structure

```
arrgo import <download_id>              # Tracked download
arrgo import --manual <path>            # Manual import
arrgo import --manual <path> --dry-run  # Preview without moving
```

## API Endpoint

```
POST /api/v1/import
```

### Request - Tracked

```json
{
  "download_id": 123
}
```

### Request - Manual

```json
{
  "path": "/srv/data/usenet/Back.To.The.Future.1985.1080p...",
  "title": "Back to the Future",
  "year": 1985,
  "type": "movie",
  "quality": "1080p"
}
```

### Response

```json
{
  "file_id": 456,
  "content_id": 789,
  "source_path": "/srv/data/usenet/Back.To.The.Future.../movie.mkv",
  "dest_path": "/srv/data/arrgo-test/movies/Back to the Future (1985)/Back to the Future (1985) [1080p].mkv",
  "size_bytes": 6543210000,
  "plex_notified": true
}
```

### Errors

- `400` - missing required fields, path doesn't exist, no video file found
- `404` - download_id not found
- `409` - destination file already exists

## Server Implementation

### Tracked Flow (download_id provided)

1. Validate download exists and status = "completed"
2. Call existing `Importer.Import(ctx, downloadID, download.Path)`

### Manual Flow (path + metadata provided)

1. Validate path exists and contains video file
2. Find or create content record:
   - Query: `SELECT id FROM content WHERE title = ? AND year = ?`
   - If not found: `INSERT INTO content (type, title, year, status, quality_profile)`
3. Create download record for audit trail:
   - Status = "completed", indexer = "manual"
4. Call `Importer.Import(ctx, downloadID, path)`

## CLI Implementation

### Flags

- `--manual <path>` - import from arbitrary path
- `--dry-run` - preview without moving files

### Dry-Run Behavior

- Parse release name locally (no server call)
- Show: detected title, year, quality, type
- Show: would create content record (or link to existing ID)
- Show: source path → destination path
- Does NOT move files or update database

### Output Examples

```
$ arrgo import --manual "/srv/data/usenet/Back.To.The.Future.1985..." --dry-run

Detected: Back to the Future (1985) [movie, 1080p]
Source:   /srv/data/usenet/Back.To.The.Future.../Back.To.The.Future...mkv
Dest:     /srv/data/arrgo-test/movies/Back to the Future (1985)/Back to the Future (1985) [1080p].mkv
Content:  Would create new record

Run without --dry-run to import.

$ arrgo import --manual "/srv/data/usenet/Back.To.The.Future.1985..."

Imported: Back to the Future (1985) [1080p]
  → /srv/data/arrgo-test/movies/Back to the Future (1985)/Back to the Future (1985) [1080p].mkv
  Plex notified ✓
```

## Automatic Detection

**Content type inference:**
- Filename contains S##E## pattern → series
- Otherwise → movie

**Metadata extraction:**
- Title, year, quality parsed from release name using existing `pkg/release` parser

**Content matching:**
- If content with same title + year exists → link to it
- Otherwise → create new content record
