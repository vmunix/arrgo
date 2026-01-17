# arrgo: Unified Media Automation in Go

**Date:** 2026-01-17
**Status:** Draft
**Author:** Mark + Claude

## Overview

arrgo is a unified media automation system written in Go, designed to replace the complexity of the traditional *arr stack (Radarr, Sonarr, Prowlarr, etc.) with a single, coherent system built on modern principles.

### Goals

1. **Reduce operational complexity** — One system instead of 6+ services
2. **Simplify deployment** — Single binary, single config file, single database
3. **Modern API design** — Clean, consistent REST API (not inherited from legacy)
4. **AI-powered CLI** — Built-in conversational interface for management and debugging
5. **Maintain compatibility** — Overseerr integration via API compatibility shim

### Non-Goals (v1)

- Full RSS automation (manual/Overseerr-triggered searches only)
- Torrent support (stubbed, designed for future addition)
- Replace Plex as metadata/browsing UI
- Music or books (movies and TV only)

## Architecture

### Module Overview

```
┌──────────────────────────────────────────────────────────────┐
│                          arrgo                                │
│  ┌──────────────────────────────────────────────────────────┐│
│  │                      API Layer                            ││
│  │  /api/v1/*  (native)         /api/v3/*  (compat shim)    ││
│  └──────────────────────────────────────────────────────────┘│
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │
│  │ Library  │ │  Search  │ │ Download │ │     Import       │ │
│  │ Module   │ │  Module  │ │  Module  │ │     Module       │ │
│  │          │ │          │ │          │ │                  │ │
│  │ -Movies  │ │ -Prowlarr│ │ -SABnzbd │ │ -File rename     │ │
│  │ -Series  │ │ -Newznab │ │ -qBit    │ │ -Plex notify     │ │
│  │ -Wanted  │ │  (future)│ │  (stub)  │ │ -History         │ │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘ │
│  ┌──────────────────────────────────────────────────────────┐│
│  │          SQLite  +  Background Jobs  +  AI Chat          ││
│  └──────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────┘
         │              │              │              │
    ┌────▼────┐   ┌─────▼─────┐  ┌────▼────┐   ┌─────▼─────┐
    │Prowlarr │   │  SABnzbd  │  │  Plex   │   │  Ollama/  │
    │         │   │           │  │         │   │  Claude   │
    └─────────┘   └───────────┘  └─────────┘   └───────────┘
```

### Module Responsibilities

**Library Module**
- Tracks content: movies, series, episodes
- Manages states: wanted, available, unmonitored
- Stores minimal metadata (TMDB/TVDB ID, title, year, quality)
- Plex owns rich metadata and browsing

**Search Module**
- Queries indexers for releases
- Initially via Prowlarr API
- Future: direct Newznab/Torznab support
- Parses release names, scores against quality profiles

**Download Module**
- Sends NZBs to download clients
- Tracks download ID ↔ content mapping
- Polls for completion status
- Initially SABnzbd only; qBittorrent stubbed

**Import Module**
- Detects completed downloads
- Renames and moves files to library
- Updates database records
- Triggers Plex library scan

**API Module**
- Native REST API (`/api/v1/*`)
- Compatibility shim for Overseerr (`/api/v3/*`)
- WebSocket/SSE for real-time updates (future)

## Data Model

```sql
-- Content: movies and series unified
content (
    id              INTEGER PRIMARY KEY,
    type            TEXT NOT NULL,          -- 'movie' | 'series'
    tmdb_id         INTEGER,
    tvdb_id         INTEGER,
    title           TEXT NOT NULL,
    year            INTEGER,
    status          TEXT NOT NULL,          -- 'wanted' | 'available' | 'unmonitored'
    quality_profile TEXT NOT NULL,
    root_path       TEXT NOT NULL,
    added_at        TIMESTAMP,
    updated_at      TIMESTAMP
)

-- Episodes: only for series
episodes (
    id              INTEGER PRIMARY KEY,
    content_id      INTEGER NOT NULL REFERENCES content(id),
    season          INTEGER NOT NULL,
    episode         INTEGER NOT NULL,
    title           TEXT,
    status          TEXT NOT NULL,
    air_date        DATE,
    UNIQUE(content_id, season, episode)
)

-- Files: what's on disk
files (
    id              INTEGER PRIMARY KEY,
    content_id      INTEGER NOT NULL REFERENCES content(id),
    episode_id      INTEGER REFERENCES episodes(id),
    path            TEXT NOT NULL UNIQUE,
    size_bytes      INTEGER,
    quality         TEXT,
    source          TEXT,
    added_at        TIMESTAMP
)

-- Downloads: active and recent
downloads (
    id              INTEGER PRIMARY KEY,
    content_id      INTEGER NOT NULL REFERENCES content(id),
    episode_id      INTEGER REFERENCES episodes(id),
    client          TEXT NOT NULL,          -- 'sabnzbd' | 'qbittorrent'
    client_id       TEXT NOT NULL,
    status          TEXT NOT NULL,          -- 'queued' | 'downloading' | 'completed' | 'failed' | 'imported'
    release_name    TEXT,
    indexer         TEXT,
    added_at        TIMESTAMP,
    completed_at    TIMESTAMP
)

-- History: audit trail
history (
    id              INTEGER PRIMARY KEY,
    content_id      INTEGER NOT NULL REFERENCES content(id),
    episode_id      INTEGER REFERENCES episodes(id),
    event           TEXT NOT NULL,          -- 'grabbed' | 'imported' | 'deleted' | 'upgraded' | 'failed'
    data            TEXT,                   -- JSON
    created_at      TIMESTAMP
)

-- Quality profiles
quality_profiles (
    name            TEXT PRIMARY KEY,
    definition      TEXT NOT NULL           -- JSON
)
```

## Configuration

Single TOML file with environment variable substitution:

```toml
[server]
host = "0.0.0.0"
port = 8484
log_level = "info"

[database]
path = "./data/arrgo.db"

[libraries.movies]
root = "/srv/data/media/movies"
naming = "{title} ({year})/{title} ({year}) [{quality}].{ext}"

[libraries.series]
root = "/srv/data/media/tv"
naming = "{title}/Season {season:02d}/{title} - S{season:02d}E{episode:02d} [{quality}].{ext}"

[quality]
default = "hd"

[quality.profiles.hd]
accept = ["1080p bluray", "1080p webdl", "1080p hdtv", "720p bluray"]

[quality.profiles.uhd]
accept = ["2160p bluray", "2160p webdl", "1080p bluray"]

[indexers.prowlarr]
url = "http://localhost:9696"
api_key = "${PROWLARR_API_KEY}"

[downloaders.sabnzbd]
url = "http://localhost:8085"
api_key = "${SABNZBD_API_KEY}"
category = "arrgo"

[notifications.plex]
url = "http://localhost:32400"
token = "${PLEX_TOKEN}"
libraries = ["Movies", "TV Shows"]

[overseerr]
enabled = true
url = "http://localhost:5055"
api_key = "${OVERSEERR_API_KEY}"
sync_interval = "5m"

[compat]
api_key = "${ARRGO_API_KEY}"
radarr = true
sonarr = true

[ai]
enabled = true
provider = "ollama"

[ai.ollama]
url = "http://localhost:11434"
model = "llama3.1:8b"

[ai.anthropic]
api_key = "${ANTHROPIC_API_KEY}"
model = "claude-3-haiku"
```

## API Design

### Native API (`/api/v1`)

Conventions:
- JSON request/response
- Plural noun resources
- Standard HTTP verbs
- Pagination: `?limit=20&offset=0`
- Filtering: `?status=wanted&type=movie`
- Errors: `{"error": "message", "code": "CODE"}`
- All IDs are integers
- Timestamps are RFC3339

#### Endpoints

```
# Content
GET     /api/v1/content                 List all (filterable)
GET     /api/v1/content/:id             Get one
POST    /api/v1/content                 Add movie or series
PUT     /api/v1/content/:id             Update
DELETE  /api/v1/content/:id             Remove

# Episodes
GET     /api/v1/content/:id/episodes    List episodes for series
PUT     /api/v1/episodes/:id            Update episode

# Search & grab
POST    /api/v1/search                  Search indexers
POST    /api/v1/grab                    Grab a release

# Downloads
GET     /api/v1/downloads               Active + recent
GET     /api/v1/downloads/:id           Single download
DELETE  /api/v1/downloads/:id           Cancel download

# History
GET     /api/v1/history                 Audit log

# Files
GET     /api/v1/files                   All tracked files
DELETE  /api/v1/files/:id               Remove file

# System
GET     /api/v1/status                  Health, disk, connectivity
GET     /api/v1/profiles                Quality profiles
POST    /api/v1/scan                    Trigger Plex scan
```

### Compatibility API (`/api/v3`)

Minimal subset for Overseerr integration:

```
# Radarr compat
GET     /api/v3/movie                   → /content?type=movie
POST    /api/v3/movie                   → POST /content
GET     /api/v3/movie/:id               → /content/:id
GET     /api/v3/rootfolder              → configured movie root
GET     /api/v3/qualityprofile          → profiles in Radarr format
GET     /api/v3/queue                   → /downloads reformatted
POST    /api/v3/command                 → handles MoviesSearch, etc.

# Sonarr compat (same pattern)
GET     /api/v3/series                  → /content?type=series
...
```

## Core Workflows

### Add Content via Overseerr

```
Overseerr                     arrgo                          External
    │                           │                               │
    ├── POST /api/v3/movie ────►│                               │
    │                           ├── Create content (wanted)     │
    │◄── 201 Created ───────────┤                               │
    │                           │                               │
    ├── POST /api/v3/command ──►│                               │
    │   {name: "MoviesSearch"}  ├── Query Prowlarr ────────────►│
    │                           │◄── releases[] ────────────────┤
    │                           ├── Score & pick best           │
    │                           ├── Send NZB to SABnzbd ───────►│
    │                           │◄── nzo_id ────────────────────┤
    │◄── 200 OK ────────────────┤                               │
```

### Download Completion → Import

```
arrgo (poller)                 External
    │                             │
    ├── GET /api/queue ──────────►│ SABnzbd
    │◄── {status: "Completed"} ───┤
    │                             │
    ├── Find video files          │
    ├── Rename & move to library  │
    ├── Update database           │
    ├── POST /library/refresh ───►│ Plex
    │◄── 200 OK ──────────────────┤
```

### Background Jobs

| Job | Interval | Purpose |
|-----|----------|---------|
| Download poller | 30s | Check for completed downloads |
| Overseerr sync | 5m | Pull new approved requests |
| Stuck checker | 10m | Detect stalled downloads |
| Health check | 1m | Verify client connectivity |

## AI-Powered CLI

### Modes

1. **Direct commands** — `arrgo status`, `arrgo search "Matrix"`
2. **Chat mode** — `arrgo chat` (conversational)
3. **One-shot** — `arrgo ask "why is my download stuck?"`

### Example Interaction

```
$ arrgo chat
🎬 arrgo> why haven't my downloads finished?

Looking at your queue...

Found 3 active downloads:
  • "Dune Part Two" - downloading, 67% - ~4 min remaining
  • "Shogun S01E05" - completed, waiting for import
  • "The Matrix" - stalled since 2h ago

Issue detected: "The Matrix" download failed extraction.
SABnzbd reports: "Unpacking failed, archive is corrupt"

Would you like me to:
  1. Retry with a different release
  2. Blacklist this release and search for alternatives
  3. Show available releases to pick manually

🎬 arrgo> 2

Blacklisting "The.Matrix.1999.2160p.UHD.BluRay-GROUPNAME"
Searching for alternatives...

Found 8 releases. Best match:
  "The.Matrix.1999.1080p.BluRay.x264-SPARKS" (score: 92)

Grab this release? (y/n)
```

### LLM Backend Options

- **Ollama** (default) — Local, free, private. Llama 3.1 8B handles this well.
- **Anthropic** — Claude Haiku for polished experience
- **OpenAI** (future) — Alternative cloud option

The task is constrained (~15 tools, narrow domain), so small models work fine.

## Repository Structure

```
arrgo/
├── cmd/
│   └── arrgo/
│       └── main.go              # Entry point
├── internal/
│   ├── library/                 # Content tracking
│   ├── search/                  # Indexer queries
│   ├── download/                # Download client integration
│   ├── importer/                # File import & rename
│   ├── api/
│   │   ├── v1/                  # Native API
│   │   └── compat/              # Radarr/Sonarr shim
│   ├── ai/                      # LLM integration
│   └── config/                  # Configuration loading
├── pkg/
│   ├── newznab/                 # Newznab client (future)
│   └── release/                 # Release name parsing
├── migrations/                  # SQLite schema
├── docs/
│   └── api.yaml                 # OpenAPI spec
├── config.example.toml
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## Packaging & Distribution

### Installation Methods

```bash
# Single binary (preferred)
curl -fsSL https://arrgo.dev/install.sh | sh
arrgo init

# Docker
docker run -v ./config:/config ghcr.io/arrgo/arrgo

# Package managers
brew install arrgo
yay -S arrgo
```

### CLI Commands

```bash
# Server
arrgo serve                      # Start API + background jobs

# Direct commands
arrgo status                     # System status
arrgo search "Movie Name"        # Search indexers
arrgo queue                      # Show downloads
arrgo add movie 123456           # Add by TMDB ID
arrgo grab <release-id>          # Grab release

# AI chat
arrgo chat                       # Interactive session
arrgo ask "why is X stuck?"      # One-shot question

# Setup
arrgo init                       # Interactive wizard
arrgo config check               # Validate config
arrgo migrate                    # Run migrations
```

### Init Wizard

```
$ arrgo init

Welcome to arrgo! Let's get you set up.

Where should arrgo store data? [~/.config/arrgo]:
Movies root [/srv/media/movies]:
TV root [/srv/media/tv]:

Download client:
  [1] SABnzbd
  [2] NZBGet
  > 1

SABnzbd URL [http://localhost:8085]:
SABnzbd API key: ********

...

✓ Configuration written to ~/.config/arrgo/config.toml
✓ Database initialized

Start arrgo now? [Y/n]:
```

## Design Decisions

### Why unified content table?

Movies and series are conceptually similar — tracked content with wanted/available status. Separate tables (like Radarr/Sonarr) was historical accident from forking. Episodes table handles series-specific needs.

### Why TOML over YAML?

- No whitespace sensitivity footguns
- Explicit types (no "Norway problem" where `NO` becomes boolean)
- Popular in Go ecosystem
- First-class comments

### Why Prowlarr initially?

Prowlarr handles indexer quirks (auth, rate limits, response parsing). Building direct Newznab/Torznab is scope creep for v1. Abstraction layer allows replacing later.

### Why stub torrents?

Usenet is simpler (download → done). Torrents have seeding lifecycle complexity. Get v1 working with usenet, add torrents in v2 with proper design for seeding state management.

### Why AI built-in?

- Differentiation from existing *arr tools
- Natural fit for orchestration/debugging tasks
- Small local models (Llama 3.1 8B) handle this well
- Makes the tool more approachable

## Future Considerations

### v2 Candidates

- Torrent support with seeding lifecycle
- Direct Newznab/Torznab (drop Prowlarr dependency)
- RSS monitoring and auto-grab
- Quality upgrades
- Embedded web UI
- Multi-user support

### Torrent Design Notes

When adding torrent support, key considerations:
- Downloads have three phases: downloading → seeding → removable
- Need minimum seed time/ratio configuration
- Cross-reference with import status (don't remove until imported)
- Hardlink detection (files in library = safe to remove torrent)

## Effort Estimate

| Component | Scope |
|-----------|-------|
| Project setup, config, DB | Small |
| Library module | Medium |
| Search module (Prowlarr) | Medium |
| Download module (SABnzbd) | Medium |
| Import module | Medium |
| Native API | Medium |
| Compat API shim | Medium |
| CLI (direct commands) | Small |
| CLI (AI chat) | Medium |
| Init wizard | Small |
| Documentation | Small |

Total: 4-8 weeks for working v1, depending on pace.

## Open Questions

1. **Naming templates** — Should we match Radarr/Sonarr syntax or design our own?
2. **Episode metadata** — How much to fetch from TVDB? Just IDs and air dates, or more?
3. **Failed download handling** — Auto-retry with different release, or require manual intervention?
4. **Overseerr sync mode** — Poll for requests, or only respond to API calls?

## References

- [Radarr GitHub](https://github.com/Radarr/Radarr) — 12.9k stars, C#, 13k+ commits
- [Sonarr GitHub](https://github.com/Sonarr/Sonarr) — Original *arr, C#
- [go_media_downloader](https://github.com/Kellerman81/go_media_downloader) — Existing Go alternative, 374 commits
- [Servarr Wiki](https://wiki.servarr.com/) — *arr documentation
