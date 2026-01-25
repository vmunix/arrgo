# Library Import Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Import existing Plex libraries into arrgo for tracking.

**Architecture:** CLI command calls new API endpoint which queries Plex, creates content + file records.

**Tech Stack:** Go, existing Plex client, pkg/release for quality parsing.

---

## CLI Interface

```bash
# Import all untracked items from a Plex library
arrgo library import --from-plex Movies
arrgo library import --from-plex "TV Shows"

# Override quality profile for all imports
arrgo library import --from-plex Movies --quality uhd

# Preview without making changes
arrgo library import --from-plex Movies --dry-run
```

Output:
```
Importing from Plex library "Movies"...

  + 12 Monkeys (1995) - uhd (parsed from filename)
  + Alien (1979) - hd (parsed from filename)
  - Inception (2010) - already tracked
  + The Matrix (1999) - uhd (override)

Imported: 3 new, 1 skipped (already tracked)
```

## API Endpoint

```
POST /api/v1/library/import
```

Request:
```json
{
  "source": "plex",
  "library": "Movies",
  "quality_override": "uhd",
  "dry_run": false
}
```

Response:
```json
{
  "imported": [
    {"title": "12 Monkeys", "year": 1995, "type": "movie", "quality": "uhd", "content_id": 25}
  ],
  "skipped": [
    {"title": "Inception", "year": 2010, "reason": "already tracked", "content_id": 5}
  ],
  "errors": [
    {"title": "Bad Movie", "year": 2020, "error": "file not accessible"}
  ],
  "summary": {
    "imported": 3,
    "skipped": 1,
    "errors": 1
  }
}
```

## Implementation Flow

1. Validate request (source must be "plex", library required)
2. Find Plex library section by name
3. Get all items from library via `PlexClient.ListLibraryItems()`
4. For each item:
   a. Check if already tracked (match by title + year + type)
   b. If tracked, add to skipped list
   c. Translate Plex path to local path
   d. Stat file to get size (skip with error if fails)
   e. Parse quality from filename using `pkg/release`
   f. Apply quality override if specified
   g. If not dry_run:
      - Create content record (status: "available")
      - Create file record (path, size, quality, source: "plex-import")
   h. Add to imported list
5. Return results

## Data Model

Content record:
- Type: "movie" or "series" (from Plex item type)
- Title, Year: from Plex
- Status: "available"
- QualityProfile: parsed or override
- RootPath: derived from file path

File record:
- ContentID: links to content
- Path: local file path
- SizeBytes: from stat
- Quality: parsed from filename
- Source: "plex-import"

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Library not found | Error: "Plex library 'X' not found" |
| Plex not configured | Error: "Plex not configured" |
| Already tracked | Skip, include in skipped list |
| Quality parse fails | Default to "hd", continue |
| File stat fails | Skip with error, continue others |
| Dry run | Return results without DB changes |

## Out of Scope (v1)

- TMDB/TVDB metadata lookup
- Episode-level tracking for series
- `--path` directory scanning (Plex-only)
- File records for series (no single file path)
