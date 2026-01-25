# CLAUDE.md

Guidance for Claude Code when working on arrgo.

## Project Overview

arrgo is a unified media automation system in Go, replacing the *arr stack (Radarr, Sonarr, Prowlarr, etc.) with a single coherent system.

See `docs/design.md` for the full design document.

## Architecture

```
arrgo/
├── cmd/
│   ├── arrgo/           # CLI client
│   └── arrgod/          # Server daemon
├── internal/
│   ├── events/          # Event bus + SQLite event log
│   ├── handlers/        # Event handlers (download, import, cleanup)
│   ├── adapters/        # External system adapters (sabnzbd, plex)
│   ├── server/          # Runner orchestrating event-driven components
│   ├── library/         # Content tracking (movies, series, episodes)
│   ├── search/          # Indexer queries (direct Newznab)
│   ├── download/        # Download clients (SABnzbd, future qBittorrent)
│   ├── importer/        # File import, rename, Plex notification
│   ├── api/
│   │   ├── v1/          # Native REST API
│   │   └── compat/      # Radarr/Sonarr compatibility shim
│   ├── ai/              # LLM integration (Ollama, Anthropic)
│   └── config/          # TOML configuration loading
├── pkg/
│   ├── newznab/         # Newznab protocol client
│   └── release/         # Release name parsing
└── migrations/          # SQLite schema migrations
```

### Event-Driven Architecture

The download pipeline uses an event-driven architecture with Go channels + SQLite persistence:

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Adapters  │────▶│  Event Bus  │────▶│  Handlers   │
│ (SABnzbd,   │     │  (pub/sub)  │     │ (download,  │
│  Plex)      │◀────│             │◀────│  import,    │
└─────────────┘     └─────────────┘     │  cleanup)   │
                          │             └─────────────┘
                          ▼
                    ┌─────────────┐
                    │  EventLog   │
                    │  (SQLite)   │
                    └─────────────┘
```

**Key Components:**
- **Event Bus** (`internal/events/bus.go`) - In-process pub/sub with typed events
- **EventLog** (`internal/events/log.go`) - SQLite persistence for audit/replay
- **Handlers** (`internal/handlers/`) - React to events, emit new events
- **Adapters** (`internal/adapters/`) - Poll external systems, emit events
- **Runner** (`internal/server/runner.go`) - Orchestrates component lifecycle

## Key Principles

- **Modular monolith** — Clear module boundaries, two binaries: `arrgod` (server daemon) + `arrgo` (CLI client)
- **internal/** for project-specific code, **pkg/** for reusable libraries
- **TOML configuration** with `${ENV_VAR}` substitution
- **SQLite** embedded database
- **Two API surfaces**: clean native API (`/api/v1`) + compat shim (`/api/v3`)

## Development Setup

### Required Tools

- Go 1.24+
- golangci-lint (linting)
- task (task runner, recommended)
- mockgen (mock generation for tests)

See README.md for installation commands.

### Commands

Using Task (recommended):
```bash
task build        # Build both arrgo and arrgod
task build:client # Build arrgo (CLI) only
task build:server # Build arrgod (server) only
task test         # Run tests
task lint         # Run linter
task check        # fmt + lint + test
task dev          # Run arrgod with live reload (air)
task test:cover   # Tests with coverage report
```

### Configuration

Development config: copy `config.example.toml` to `config.toml` and set env vars:
```bash
export NZBGEEK_API_KEY="your-key"
export SABNZBD_API_KEY="your-key"
# etc.
```

Or use defaults syntax in config: `${VAR:-default_value}`

### Running the Application

```bash
# Start the server (required for most CLI commands)
./arrgod                 # Or: task dev (with live reload)

# In another terminal, use CLI commands
./arrgo status           # Dashboard (connections, pipeline state, problems)
./arrgo queue            # Show active downloads
./arrgo queue --all      # Include terminal states (cleaned, failed)
./arrgo queue -s failed  # Filter by state
./arrgo queue cancel <id>         # Cancel a download
./arrgo queue cancel <id> --delete # Cancel and delete files

# Library management
./arrgo library list              # List all tracked content
./arrgo library delete <id>       # Remove content from library
./arrgo library check             # Verify files exist and Plex awareness

# Search and grab
./arrgo search "Movie Name"              # Search indexers
./arrgo search -v "Movie Name"           # Verbose (show indexer, group, service)
./arrgo search "Movie" --grab best       # Auto-grab best result
./arrgo search "Movie" --grab 1          # Grab specific result by number

# Import
./arrgo import list                                    # Pending imports and recent completions
./arrgo import <download_id>                           # Import tracked download
./arrgo import --manual "/path/to/file.mkv"            # Manual import
./arrgo import --manual "/path/to/file.mkv" --dry-run  # Preview import

# Verification
./arrgo verify           # Reality-check against SABnzbd/filesystem/Plex
./arrgo verify <id>      # Verify specific download

# Plex integration
./arrgo plex status      # Check Plex connection and libraries
./arrgo plex list movies # List Plex library contents
./arrgo plex search "Movie Name"  # Search Plex
./arrgo plex scan movies          # Trigger library scan

# Local (no server needed)
./arrgo parse "Release.Name.2024.1080p.WEB-DL.mkv"     # Parse release name
./arrgo parse --score hd "Release.Name.1080p.mkv"     # Parse and score against profile
./arrgo init             # Interactive setup wizard
```

Note: Most commands require `arrgod` running. `parse` and `init` work standalone.

## Module Responsibilities

| Module | Purpose |
|--------|---------|
| events | Event bus (pub/sub), event log (SQLite), typed event definitions |
| handlers | Event handlers: DownloadHandler (grabs), ImportHandler (imports), CleanupHandler (post-Plex cleanup) |
| adapters | External system polling: SABnzbd (download progress), Plex (library detection) |
| server | Runner orchestrating handlers/adapters lifecycle with errgroup |
| library | Content CRUD, wanted/available status, file tracking |
| search | Query indexers via Newznab, parse releases, score quality |
| download | Send to SABnzbd, track download status, state machine transitions |
| importer | Move files, rename, update DB, notify Plex |
| api/v1 | Native REST endpoints |
| api/compat | Radarr/Sonarr API translation for Overseerr |
| ai | LLM tool definitions, chat mode, provider abstraction |
| config | Load TOML, env substitution, validation |
| pkg/newznab | Newznab protocol client |
| pkg/release | Release name parsing (resolution, source, codec, HDR, audio, edition, service) |

## v1 Scope

**In scope:**
- Usenet downloads (SABnzbd)
- Direct Newznab indexer queries (NZBgeek, DrunkenSlug, etc.)
- Manual/Overseerr-triggered searches
- Auto-import on completion
- Basic content tracking
- Plex integration (status, scan, list, search)

**Out of scope (v2+):**
- AI chat CLI (get core flows working first)
- Torrent support (Torznab + qBittorrent)
- RSS/auto-grab
- Web UI

## Database

SQLite with these core tables:
- `content` — Movies and series (unified)
- `episodes` — Series episodes
- `files` — Tracked media files
- `downloads` — Active/recent downloads (state machine: queued → downloading → completed → importing → imported → cleaned)
- `events` — Event log for audit/replay (auto-pruned after 90 days)
- `history` — Audit trail
- `quality_profiles` — Quality definitions

**SQLite driver:** Uses `modernc.org/sqlite` (pure Go, no CGO). Error detection uses string matching on error messages (see `internal/library/content.go:mapSQLiteError`) since the driver wraps errors without exposing typed error codes. This is tested in `TestSQLiteCompat_ConstraintErrors`.

## API Design

Native API conventions:
- JSON bodies
- Plural noun resources
- Standard HTTP verbs
- `?limit=20&offset=0` pagination
- `{"error": "msg", "code": "CODE"}` errors
- Integer IDs, RFC3339 timestamps

## Testing

### Patterns

**Assertions** — Use [testify](https://github.com/stretchr/testify):
```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
    result, err := DoSomething()
    require.NoError(t, err)           // Fatal if fails (test stops)
    assert.Equal(t, expected, result) // Non-fatal (test continues)
}
```

- `require.X` — Fatal assertions (setup, preconditions)
- `assert.X` — Non-fatal assertions (verify multiple things)

**Mocks** — Use [mockgen](https://github.com/uber-go/mock):
```go
import (
    "github.com/vmunix/arrgo/internal/search/mocks"
    "go.uber.org/mock/gomock"
)

func TestWithMock(t *testing.T) {
    ctrl := gomock.NewController(t)
    mockIndexer := mocks.NewMockIndexerAPI(ctrl)

    mockIndexer.EXPECT().
        Search(gomock.Any(), gomock.Any()).
        Return([]Release{{Title: "Test"}}, nil)

    // ... use mockIndexer
}
```

Generated mocks live in `mocks/` subdirectories:
- `internal/search/mocks/` — IndexerAPI
- `internal/download/mocks/` — Downloader
- `internal/api/v1/mocks/` — Searcher, DownloadManager, PlexClient, FileImporter

To regenerate mocks after interface changes:
```bash
go generate ./...
```

### Test Organization

- Unit tests for business logic (parsing, scoring, etc.)
- Integration tests for API endpoints
- Mock external services (indexers, SABnzbd, Plex)

### Architecture Principles

Follow Eskil Steenberg's black-box architecture:
1. **Black Box Interfaces**: Every module has a clean API with hidden implementation
2. **Replaceable Components**: Any module can be rewritten using only its interface
3. **Single Responsibility**: One module = one person can build/maintain it
4. **Primitive-First Design**: Core types flow consistently through the system

## GitHub Issues

GitHub issues is the primary work tracking system. All bugs, features, and tech debt are tracked there.

**Workflow:**
- Check open issues with `gh issue list`
- Reference issues in commits (e.g., "fix: resolve search bug (#42)")
- Close issues when work is complete with `gh issue close N --comment "reason"`

**Labels:**
- `bug` — Something isn't working
- `enhancement` — New feature or improvement
- `tech-debt` — Code quality, refactoring
- `testing` — Test coverage, test infrastructure
- `v2` — Deferred to v2 (torrents, RSS, web UI, AI chat)

**Creating issues:**
- Always add relevant labels: `gh issue create --label "bug"`
- Reference related issues/commits where applicable
