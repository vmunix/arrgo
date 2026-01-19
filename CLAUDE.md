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

## Key Principles

- **Modular monolith** — Clear module boundaries, two binaries: `arrgod` (server daemon) + `arrgo` (CLI client)
- **internal/** for project-specific code, **pkg/** for reusable libraries
- **TOML configuration** with `${ENV_VAR}` substitution
- **SQLite** embedded database
- **Two API surfaces**: clean native API (`/api/v1`) + compat shim (`/api/v3`)

## Development Setup

### Required Tools

- Go 1.25+
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

## Module Responsibilities

| Module | Purpose |
|--------|---------|
| library | Content CRUD, wanted/available status, file tracking |
| search | Query indexers via Newznab, parse releases, score quality |
| download | Send to SABnzbd, track download status, poll completion |
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
- AI chat CLI

**Out of scope (v2+):**
- Torrent support (Torznab + qBittorrent)
- RSS/auto-grab
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
- Mock external services (indexers, SABnzbd, Plex)

Follow Eskil Steenberg's black-box architecture:
1. **Black Box Interfaces**: Every module has a clean API with hidden implementation
2. **Replaceable Components**: Any module can be rewritten using only its interface
3. **Single Responsibility**: One module = one person can build/maintain it
4. **Primitive-First Design**: Core types flow consistently through the system

## GitHub Issues

When code quality reviewers identify issues that aren't immediately addressed:
- Create GitHub issues to track them
- Use appropriate labels: `tech-debt`, `testing`, `refactor`, `bug`, `enhancement`, `docs`

When creating any GitHub issue:
- Always add relevant labels via `gh issue create --label "label-name"` or `gh issue edit N --add-label "label-name"`
- Reference related issues/commits where applicable
