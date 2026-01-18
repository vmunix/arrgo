# CLI Commands Design

**Date:** 2026-01-18
**Status:** Approved

## Overview

Implement CLI commands (status, search, queue) as HTTP clients to the running arrgo server. The CLI provides a complete standalone experience for end-to-end media automation.

## Design Decisions

1. **HTTP client approach** - CLI calls running server at `http://localhost:8484`
2. **Human-readable output** by default, `--json` flag for scripting
3. **Default localhost** with `--server` flag for remote servers
4. **Search-first UX** - search is the entry point, grab creates content automatically

## Commands

| Command | Purpose | API Endpoint |
|---------|---------|--------------|
| `arrgo status` | System health, counts | `GET /api/v1/status` |
| `arrgo search <query>` | Search + optional grab | `POST /api/v1/search`, `POST /api/v1/content`, `POST /api/v1/grab` |
| `arrgo queue` | List downloads | `GET /api/v1/downloads` |

## Flags

**Shared:**
- `--server URL` - Server address (default: `http://localhost:8484`)
- `--json` - Output as JSON, no interactive prompts

**search:**
- `--type movie|series` - Content type (required for grab)
- `--grab N` - Grab Nth result without prompting
- `--grab best` - Auto-grab highest scored result

**queue:**
- `--all` - Include completed/imported downloads

## Search Flow

```
$ arrgo search "the matrix 1999" --type movie

Searching indexers...
Found 8 releases:

  # │ TITLE                              │ SIZE   │ INDEXER │ SCORE
────┼────────────────────────────────────┼────────┼─────────┼───────
  1 │ The.Matrix.1999.2160p.UHD.BluRay   │ 45.2GB │ nzbgeek │ 850
  2 │ The.Matrix.1999.1080p.BluRay.x264  │ 12.1GB │ drunken │ 720

Grab? [1-8, n]: 1

Parsed: The Matrix (1999) → /movies/The Matrix (1999)/
Confirm? [Y/n]: y

✓ Added to library
✓ Download started
```

**When grabbing:**
1. Parse release name with `pkg/release` to extract title/year
2. Create content entry via `POST /api/v1/content`
3. Grab release via `POST /api/v1/grab`

## Output Formats

**status:**
```
Server:     http://localhost:8484 (ok)
Version:    0.1.0
Library:    42 movies, 8 series
Downloads:  2 active, 0 completed
Indexers:   prowlarr (connected)
Downloader: sabnzbd (connected)
```

**queue:**
```
Active downloads (2):

  # │ TITLE                              │ STATUS      │ PROGRESS
────┼────────────────────────────────────┼─────────────┼──────────
  1 │ The.Matrix.1999.2160p.UHD.BluRay   │ downloading │ 45%
  2 │ Inception.2010.1080p.BluRay        │ queued      │ -
```

## Implementation

**Files:**
- Create: `cmd/arrgo/client.go` - HTTP client wrapper
- Create: `cmd/arrgo/commands.go` - command implementations
- Modify: `cmd/arrgo/main.go` - wire up commands

**No backend changes required** - uses existing API endpoints.

## Future (v2)

The interactive prompts are a foundation for a full TUI experience.
