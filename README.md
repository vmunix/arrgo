# arrgo

Unified media automation in Go. A single, coherent replacement for the *arr stack (Radarr, Sonarr, Prowlarr, etc.).

## Status

**Early development** — not yet functional.

## Goals

- **One system** instead of 6+ services
- **Single binary** with embedded SQLite
- **Single config file** (TOML)
- **Clean API design** with Radarr/Sonarr compatibility shim for Overseerr
- **AI-powered CLI** for conversational management

## Quick Start

```bash
# Build
go build -o arrgo ./cmd/arrgo

# Initialize (interactive wizard)
./arrgo init

# Start server
./arrgo serve

# Or use the AI chat
./arrgo chat
```

## Configuration

Copy `config.example.toml` to `config.toml` and edit:

```bash
cp config.example.toml config.toml
# Edit config.toml with your settings
```

Environment variables can be referenced with `${VAR_NAME}` syntax.

## CLI Commands

```bash
# Server
arrgo serve              # Start API server + background jobs

# Management
arrgo status             # System health and queue summary
arrgo search "Movie"     # Search indexers
arrgo queue              # Show active downloads
arrgo add movie 123456   # Add by TMDB ID
arrgo add series 456789  # Add by TVDB ID

# AI Assistant
arrgo chat               # Interactive conversation
arrgo ask "why stuck?"   # One-shot question
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│                    arrgo                         │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌────────┐ │
│  │ Library │ │ Search  │ │Download │ │ Import │ │
│  └─────────┘ └─────────┘ └─────────┘ └────────┘ │
│  ┌─────────────────────────────────────────────┐│
│  │        REST API  +  AI Chat  +  SQLite      ││
│  └─────────────────────────────────────────────┘│
└─────────────────────────────────────────────────┘
        │            │           │
   ┌────▼────┐  ┌────▼────┐ ┌────▼────┐
   │Prowlarr │  │SABnzbd  │ │  Plex   │
   └─────────┘  └─────────┘ └─────────┘
```

## External Dependencies

arrgo integrates with (but aims to reduce over time):

- **Prowlarr** — Indexer aggregation (direct Newznab support planned)
- **SABnzbd** — Usenet downloads
- **Plex** — Media server notifications
- **Overseerr** — Request management (optional, via compat API)
- **Ollama** — Local AI for chat mode (optional)

## License

MIT
