# CLAUDE.md

Guidance for Claude Code when working on arrgo.

## Project Overview

arrgo is a unified media automation system in Go, replacing the *arr stack (Radarr, Sonarr, Prowlarr, etc.) with a single coherent system.

See `docs/design.md` for the full design document.

## Architecture

```
arrgo/
├── cmd/arrgo/           # CLI entry point
├── internal/
│   ├── library/         # Content tracking (movies, series, episodes)
│   ├── search/          # Indexer queries (Prowlarr, future Newznab)
│   ├── download/        # Download clients (SABnzbd, future qBittorrent)
│   ├── importer/        # File import, rename, Plex notification
│   ├── api/
│   │   ├── v1/          # Native REST API
│   │   └── compat/      # Radarr/Sonarr compatibility shim
│   ├── ai/              # LLM integration (Ollama, Anthropic)
│   └── config/          # TOML configuration loading
├── pkg/
│   ├── newznab/         # Newznab client (future)
│   └── release/         # Release name parsing
└── migrations/          # SQLite schema migrations
```

## Key Principles

- **Modular monolith** — Clear module boundaries, but single binary
- **internal/** for project-specific code, **pkg/** for reusable libraries
- **TOML configuration** with `${ENV_VAR}` substitution
- **SQLite** embedded database
- **Two API surfaces**: clean native API (`/api/v1`) + compat shim (`/api/v3`)

## Development Commands

```bash
# Build
go build -o arrgo ./cmd/arrgo

# Run
./arrgo serve

# Test
go test ./...

# Lint (if golangci-lint installed)
golangci-lint run
```

## Module Responsibilities

| Module | Purpose |
|--------|---------|
| library | Content CRUD, wanted/available status, file tracking |
| search | Query Prowlarr/indexers, parse releases, score quality |
| download | Send to SABnzbd, track download status, poll completion |
| importer | Move files, rename, update DB, notify Plex |
| api/v1 | Native REST endpoints |
| api/compat | Radarr/Sonarr API translation for Overseerr |
| ai | LLM tool definitions, chat mode, provider abstraction |
| config | Load TOML, env substitution, validation |

## v1 Scope

**In scope:**
- Usenet downloads (SABnzbd)
- Prowlarr for indexer queries
- Manual/Overseerr-triggered searches
- Auto-import on completion
- Basic content tracking
- AI chat CLI

**Out of scope (v2+):**
- Torrent support
- RSS/auto-grab
- Direct Newznab (bypass Prowlarr)
- Web UI

## Database

SQLite with these core tables:
- `content` — Movies and series (unified)
- `episodes` — Series episodes
- `files` — Tracked media files
- `downloads` — Active/recent downloads
- `history` — Audit trail
- `quality_profiles` — Quality definitions

## API Design

Native API conventions:
- JSON bodies
- Plural noun resources
- Standard HTTP verbs
- `?limit=20&offset=0` pagination
- `{"error": "msg", "code": "CODE"}` errors
- Integer IDs, RFC3339 timestamps

## Testing

- Unit tests for business logic (parsing, scoring, etc.)
- Integration tests for API endpoints
- Mock external services (Prowlarr, SABnzbd, Plex)
