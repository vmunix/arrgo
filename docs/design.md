# arrgo: Unified Media Automation in Go

**Date:** 2026-01-25
**Status:** Active Development
**Author:** Mark + Claude

## Overview

arrgo is a unified media automation system written in Go, designed to replace the complexity of the traditional *arr stack (Radarr, Sonarr, Prowlarr, etc.) with a single, coherent system built on modern principles.

### Goals

1. **Reduce operational complexity** — One system instead of 6+ services
2. **Simplify deployment** — Two binaries (`arrgod` + `arrgo`), single config file, single database
3. **Modern API design** — Clean, consistent REST API (not inherited from legacy)
4. **Plex integration** — Status, scanning, library listing and search with tracking status
5. **Maintain compatibility** — Overseerr integration via API compatibility shim

### Non-Goals (v1)

- Full RSS automation (manual/Overseerr-triggered searches only)
- Torrent support (stubbed, designed for future addition)
- Replace Plex as metadata/browsing UI
- Music or books (movies and TV only)

## Architecture

### Module Overview

```
┌────────────────────────────────────────────────────────────────────┐
│                            arrgo                                   │
│  ┌────────────────────────────────────────────────────────────────┐│
│  │                        API Layer                               ││
│  │  /api/v1/*  (native)              /api/v3/*  (compat shim)     ││
│  └────────────────────────────────────────────────────────────────┘│
│  ┌──────────┐ ┌──────────┐ ┌────────────────────────────────────┐  │
│  │ Library  │ │  Search  │ │       Event-Driven Pipeline        │  │
│  │ Module   │ │  Module  │ │  ┌─────────┐    ┌──────────────┐   │  │
│  │          │ │          │ │  │  Event  │───▶│   Handlers   │   │  │
│  │ -Movies  │ │ -Newznab │ │  │   Bus   │    │  -Download   │   │  │
│  │ -Series  │ │ -Parallel│ │  │         │◀───│  -Import     │   │  │
│  │ -Wanted  │ │  search  │ │  └────┬────┘    │  -Cleanup    │   │  │
│  └──────────┘ └──────────┘ │       │         └──────────────┘   │  │
│                            │       ▼         ┌──────────────┐   │  │
│                            │  ┌─────────┐    │   Adapters   │   │  │
│                            │  │EventLog │    │  -SABnzbd    │   │  │
│                            │  │(SQLite) │    │  -Plex       │   │  │
│                            │  └─────────┘    └──────────────┘   │  │
│                            └────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────────────┐│
│  │                  SQLite  +  Runner (errgroup)                  ││
│  └────────────────────────────────────────────────────────────────┘│
└────────────────────────────────────────────────────────────────────┘
         │              │              │
    ┌────▼────┐   ┌─────▼─────┐  ┌────▼────┐
    │ NZBgeek │   │  SABnzbd  │  │  Plex   │
    │ et al.  │   │           │  │         │
    └─────────┘   └───────────┘  └─────────┘
```

### Module Responsibilities

**Event Bus & EventLog** (`internal/events/`)
- In-process pub/sub with typed events (Go channels)
- SQLite persistence for audit trail and replay
- Auto-pruning of old events (90 days retention)

**Handlers** (`internal/handlers/`)
- **DownloadHandler**: Listens for `GrabRequested`, sends to SABnzbd, emits `DownloadCreated`
- **ImportHandler**: Listens for `DownloadCompleted`, imports files, emits `ImportCompleted`
- **CleanupHandler**: Listens for `PlexItemDetected`, cleans up source files after Plex verification

**Adapters** (`internal/adapters/`)
- **SABnzbd Adapter**: Polls SABnzbd queue, emits `DownloadProgress`/`DownloadCompleted`
- **Plex Adapter**: Polls Plex library, emits `PlexItemDetected` when imports appear

**Runner** (`internal/server/`)
- Orchestrates handler and adapter lifecycle using errgroup
- Exposes event bus for API access
- Manages graceful shutdown

**Library Module**
- Tracks content: movies, series, episodes
- Manages states: wanted, available, unmonitored
- Stores minimal metadata (TMDB/TVDB ID, title, year, quality)
- Plex owns rich metadata and browsing

**Search Module**
- Queries indexers for releases via direct Newznab protocol
- Parallel search across multiple indexers (IndexerPool)
- Partial failure tolerance — returns results from working indexers
- Parses release names extracting resolution, source, codec, HDR format, audio codec, edition, streaming service, and release group
- Scores releases against quality profiles

**Download Module**
- Sends NZBs to download clients
- Tracks download ID ↔ content mapping
- State machine: queued → downloading → completed → importing → imported → cleaned (or failed/skipped)
- Initially SABnzbd only; qBittorrent stubbed

**Import Module**
- Renames and moves files to library
- Updates database records
- Triggers Plex library scan

**API Module**
- Native REST API (`/api/v1/*`)
- Compatibility shim for Overseerr (`/api/v3/*`)
- Can publish events for grab requests
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

-- Downloads: active and recent (state machine lifecycle)
downloads (
    id              INTEGER PRIMARY KEY,
    content_id      INTEGER NOT NULL REFERENCES content(id),
    episode_id      INTEGER REFERENCES episodes(id),
    client          TEXT NOT NULL,          -- 'sabnzbd' | 'qbittorrent' | 'manual'
    client_id       TEXT NOT NULL,
    status          TEXT NOT NULL,          -- 'queued' | 'downloading' | 'completed' | 'importing' | 'imported' | 'cleaned' | 'failed' | 'skipped'
    release_name    TEXT,
    indexer         TEXT,
    added_at        TIMESTAMP,
    completed_at    TIMESTAMP,
    last_transition_at TIMESTAMP            -- For stuck detection
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

-- Events: event-driven pipeline log (auto-pruned after 90 days)
events (
    id              INTEGER PRIMARY KEY,
    event_type      TEXT NOT NULL,          -- 'grab.requested' | 'download.created' | 'download.completed' | etc.
    entity_type     TEXT NOT NULL,          -- 'download' | 'content'
    entity_id       INTEGER NOT NULL,
    payload         TEXT NOT NULL,          -- JSON event data
    occurred_at     TIMESTAMP NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

# Minimal profile - just resolution
[quality.profiles.sd]
resolution = ["720p", "480p"]

# Standard HD
[quality.profiles.hd]
resolution = ["1080p", "720p"]
sources = ["bluray", "webdl"]

# Premium 4K with HDR/audio preferences
[quality.profiles.uhd]
resolution = ["2160p", "1080p"]
sources = ["bluray", "webdl"]
hdr = ["dolby-vision", "hdr10+", "hdr10"]
audio = ["atmos", "truehd", "dtshd"]
reject = ["hdtv", "cam", "ts"]

# Named indexers (add as many as needed)
[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "${NZBGEEK_API_KEY}"

[indexers.drunkenslug]
url = "https://api.drunkenslug.com"
api_key = "${DRUNKENSLUG_API_KEY}"

[downloaders.sabnzbd]
url = "http://localhost:8085"
api_key = "${SABNZBD_API_KEY}"
category = "arrgo"

[notifications.plex]
url = "http://localhost:32400"
token = "${PLEX_TOKEN}"
libraries = ["Movies", "TV Shows"]
remote_path = "/data/media"        # Path as seen by Plex (for path translation)
local_path = "/srv/data/media"     # Corresponding local path

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
POST    /api/v1/content/:id/sync-episodes  Sync episodes from TVDB
PUT     /api/v1/episodes/:id            Update episode

# Search & grab
POST    /api/v1/search                  Search indexers
POST    /api/v1/grab                    Grab a release

# Downloads
GET     /api/v1/downloads               Active + recent
GET     /api/v1/downloads/:id           Single download
GET     /api/v1/downloads/:id/events    Events for a download
DELETE  /api/v1/downloads/:id           Cancel download
POST    /api/v1/downloads/:id/retry     Retry failed download

# History & Events
GET     /api/v1/history                 Audit log
GET     /api/v1/events                  Event log

# Files
GET     /api/v1/files                   All tracked files
DELETE  /api/v1/files/:id               Remove file

# Library
GET     /api/v1/library/check           Verify files exist and Plex awareness
POST    /api/v1/library/import          Import existing Plex library into arrgo

# Import
POST    /api/v1/import                  Import tracked download or manual file

# Plex
GET     /api/v1/plex/status             Plex connection status and libraries
POST    /api/v1/plex/scan               Scan specific libraries or all
GET     /api/v1/plex/libraries/:name/items  List library contents
GET     /api/v1/plex/search             Search Plex with tracking status

# TVDB
GET     /api/v1/tvdb/search             Search TVDB for series

# System
GET     /api/v1/status                  Health, version
GET     /api/v1/dashboard               Aggregated stats (connections, pipeline, stuck, library)
GET     /api/v1/verify                  Reality-check downloads against live systems
GET     /api/v1/profiles                Quality profiles
GET     /api/v1/indexers                Configured indexers (with optional connectivity test)
POST    /api/v1/scan                    Trigger Plex scan by path
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
    │   {name: "MoviesSearch"}  ├── Query indexers (parallel) ─►│
    │                           │◄── releases[] ────────────────┤
    │                           ├── Score & pick best           │
    │                           ├── Send NZB to SABnzbd ───────►│
    │                           │◄── nzo_id ────────────────────┤
    │◄── 200 OK ────────────────┤                               │
```

### Download Completion → Import (Event-Driven)

```
SABnzbd Adapter          Event Bus              Handlers
    │                        │                      │
    ├─ poll SABnzbd ─────────┤                      │
    │                        │                      │
    ├─ DownloadCompleted ───►│                      │
    │                        ├─ DownloadCompleted ─►│ ImportHandler
    │                        │                      ├─ Find video files
    │                        │                      ├─ Rename & move
    │                        │                      ├─ Update database
    │                        │                      ├─ Trigger Plex scan
    │                        │◄─ ImportCompleted ───┤
    │                        │                      │
Plex Adapter                 │                      │
    │                        │                      │
    ├─ poll Plex library ────┤                      │
    ├─ PlexItemDetected ────►│                      │
    │                        ├─ PlexItemDetected ──►│ CleanupHandler
    │                        │                      ├─ Delete source files
    │                        │◄─ CleanupCompleted ──┤
```

### Event Types

| Event | Emitted By | Handled By |
|-------|-----------|------------|
| `GrabRequested` | API, Compat layer | DownloadHandler |
| `GrabSkipped` | DownloadHandler | (logged) - when existing quality is better |
| `DownloadCreated` | DownloadHandler | (logged) |
| `DownloadProgressed` | SABnzbd Adapter | (logged) |
| `DownloadCompleted` | SABnzbd Adapter | ImportHandler |
| `DownloadFailed` | SABnzbd Adapter | (logged) |
| `ImportStarted` | ImportHandler | (logged) |
| `ImportCompleted` | ImportHandler | CleanupHandler |
| `ImportFailed` | ImportHandler | (logged) |
| `ImportSkipped` | ImportHandler | (logged) - when existing quality is better |
| `PlexItemDetected` | Plex Adapter | CleanupHandler |
| `CleanupStarted` | CleanupHandler | (logged) |
| `CleanupCompleted` | CleanupHandler | (logged) |
| `ContentAdded` | API | (logged) |
| `ContentStatusChanged` | API | (logged) |

### Background Jobs

| Job | Interval | Purpose |
|-----|----------|---------|
| SABnzbd Adapter | 30s | Poll for download progress/completion |
| Plex Adapter | 30s | Poll for newly imported items |
| Event log pruning | 24h | Remove events older than 90 days |
| Health check | 1m | Verify client connectivity |

## AI-Powered CLI (v2+)

> **Note:** AI chat is planned for v2+. Get core flows working well first.

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
│   ├── arrgo/                   # CLI client
│   │   └── main.go
│   └── arrgod/                  # Server daemon
│       └── main.go
├── internal/
│   ├── events/                  # Event bus + SQLite event log
│   ├── handlers/                # Event handlers (download, import, cleanup)
│   ├── adapters/                # External system adapters
│   │   ├── sabnzbd/             # SABnzbd polling adapter
│   │   └── plex/                # Plex polling adapter
│   ├── server/                  # Runner orchestrating event-driven components
│   ├── library/                 # Content tracking
│   ├── search/                  # Indexer queries
│   ├── download/                # Download client integration (SABnzbd)
│   ├── importer/                # File import, rename, Plex notification
│   ├── api/
│   │   ├── v1/                  # Native API
│   │   └── compat/              # Radarr/Sonarr shim
│   ├── ai/                      # LLM integration
│   ├── tmdb/                    # TMDB metadata client
│   └── config/                  # Configuration loading
├── pkg/
│   ├── newznab/                 # Newznab protocol client
│   └── release/                 # Release name parsing
├── migrations/                  # SQLite schema
├── docs/
│   └── design.md                # This design document
├── config.example.toml
├── Taskfile.yml                 # Task runner commands
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

### CLI Design

The CLI (`arrgo`) is a thin client over the REST API, designed around two principles:

**1. Full observability** — Every state transition in the event-driven pipeline is observable via CLI. Users can see what's happening at each stage: grab → download → import → plex detection → cleanup.

**2. Scriptable by humans and machines** — All output supports `--json` for automation. An LLM, shell script, or human can equally drive the system through the same interface.

```
arrgo status              # What's happening now?
arrgo downloads           # What's in the pipeline?
arrgo downloads show 42   # What happened to this specific download?
arrgo search "Movie"      # What's available?
arrgo search --grab best  # Trigger a state transition
```

The CLI is intentionally simple: read state, trigger transitions, observe results. Complex orchestration (retry logic, upgrade decisions) is either in the server or left to the caller.

See `CLAUDE.md` or `arrgo --help` for the full command reference.

### Init Wizard

`arrgo init` provides interactive first-time setup, walking users through paths, download client configuration, and indexer setup. Creates a working `config.toml` and initializes the database.

## Design Decisions

### Why unified content table?

Movies and series are conceptually similar — tracked content with wanted/available status. Separate tables (like Radarr/Sonarr) was historical accident from forking. Episodes table handles series-specific needs.

### Why TOML over YAML?

- No whitespace sensitivity footguns
- Explicit types (no "Norway problem" where `NO` becomes boolean)
- Popular in Go ecosystem
- First-class comments

### Why direct Newznab instead of Prowlarr?

We initially planned to use Prowlarr, but discovered its internal search API doesn't reliably return usenet results. Radarr/Sonarr actually bypass Prowlarr's API and call each indexer's Newznab endpoint directly. Since our goal is to eliminate dependencies, we implemented direct Newznab support from the start. The IndexerPool provides parallel search across multiple indexers with graceful partial failure handling.

### Why stub torrents?

Usenet is simpler (download → done). Torrents have seeding lifecycle complexity. Get v1 working with usenet, add torrents in v2 with proper design for seeding state management.

### Why event-driven architecture?

The download pipeline (grab → download → import → cleanup) is naturally event-driven:
- **Loose coupling** — Handlers don't know about each other, just events
- **Testability** — Easy to test handlers in isolation by publishing test events
- **Observability** — All events persisted to SQLite for audit/debugging
- **Extensibility** — Add new handlers without modifying existing code
- **Reliability** — Events can be replayed if handlers fail

We use Go channels (not Kafka/Redis) because:
- Single process, no network overhead
- SQLite persistence sufficient for audit needs
- Simpler deployment (no external dependencies)

### Why AI built-in?

- Differentiation from existing *arr tools
- Natural fit for orchestration/debugging tasks
- Small local models (Llama 3.1 8B) handle this well
- Makes the tool more approachable

## Future Considerations

### v2 Candidates

- AI-powered CLI (chat mode, one-shot questions, LLM integration)
- Torrent support with seeding lifecycle (Torznab for indexers, qBittorrent client)
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

## Status

v1 core is functional:
- ✅ Event-driven download pipeline (grab → download → import → cleanup)
- ✅ Newznab indexer integration with parallel search
- ✅ SABnzbd integration with progress tracking
- ✅ Plex integration (status, scan, search, library import)
- ✅ Overseerr compatibility API
- ✅ CLI with full observability
- ✅ Library import from Plex

Remaining v1 work:
- Series/episode support (parsing works, episode import needs work)
- Quality upgrade logic

## Open Questions

1. **Episode metadata** — How much to fetch from TVDB? Just IDs and air dates, or full episode details?
2. **Failed download handling** — Currently manual retry. Auto-retry with blacklist?

## References

- [Radarr GitHub](https://github.com/Radarr/Radarr) — 12.9k stars, C#, 13k+ commits
- [Sonarr GitHub](https://github.com/Sonarr/Sonarr) — Original *arr, C#
- [go_media_downloader](https://github.com/Kellerman81/go_media_downloader) — Existing Go alternative, 374 commits
- [Servarr Wiki](https://wiki.servarr.com/) — *arr documentation
