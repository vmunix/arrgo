# arrgo

**Arrgo: Unified media automation in Go. A single, coherent attempt at an \*arr stack.**

**Disclaimer**

This whole thing is a learning experiment to see how well LLM agent harnesses
could take an existing, complex software stack that grew organically over a
decade+ (the kind we all have to deal with in our "day jobs") and model it's
flow control, functionality, behaviour quirks, and so on and then re-architect
/ re-implement from scratch. 

Where does it get stuck, where does it go wrong, what works well? My only goal
is learning how to work with the tools and gain a better understanding of the
class of new problems that comes out the other side of a complex stack written
entirely by LLM agents.

I'm leaving this "public" on GitHub as a learning asset. For the love of god
don't try using it yourself. 

The \*arr stack felt perfect for the job because it's both terrible and *real
world battle tested* against dirty, nasty, crazy human curated data. It's APIs
are full of both deeply subtly inconsistent API expectations. Nothing
is particuarly well specified. In other words the kind of work I avoid doing for fun at all
costs..

## Status

**Early development** — core features working, API and CLI functional.

## Goals

- **One system** instead of 6+ services
- **Two binaries**: `arrgod` (server daemon) + `arrgo` (CLI client)
- **Embedded SQLite** database
- **Single config file** (TOML)
- **Clean API design** with Radarr/Sonarr compatibility shim for Overseerr
- **Plex integration** for library management and scanning

## Quick Start

```bash
# Build both binaries
task build
# Or: go build ./cmd/...

# Initialize (interactive wizard)
./arrgo init

# Start server daemon
./arrgod

# In another terminal, use the CLI
./arrgo status
./arrgo search "The Matrix"
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
# Server daemon
arrgod                   # Start API server + background jobs
arrgod --config FILE     # Use custom config file

# CLI client
arrgo status             # System dashboard (connections, pipeline, problems)
arrgo search "Movie"     # Search indexers for releases
arrgo queue              # Show active downloads
arrgo queue --all        # Include terminal states (cleaned, failed)
arrgo queue --state X    # Filter by state
arrgo import list        # Show pending imports and recent completions
arrgo verify             # Reality-check downloads against SABnzbd/Plex
arrgo verify 42          # Verify specific download
arrgo init               # Interactive setup wizard
arrgo parse "Release.Name.2024.1080p.mkv"  # Parse release name locally

# Import content
arrgo import 42                            # Import tracked download by ID
arrgo import --manual "/path/to/file.mkv"  # Import file with auto-parsed metadata
arrgo import --manual "/path/to/file.mkv" --dry-run  # Preview without changes

# Plex integration
arrgo plex status        # Show Plex connection and libraries
arrgo plex scan movies   # Trigger library scan (case-insensitive names)
arrgo plex scan --all    # Scan all libraries
arrgo plex list          # List all Plex libraries
arrgo plex list movies   # List library contents with tracking status
arrgo plex search "Matrix"  # Search Plex with tracking status

# Global flags
--json                   # Output as JSON
--server URL             # Custom server URL (default: http://localhost:8484)
```

## Development Setup

### Requirements

- **Go 1.25+** — [golang.org/dl](https://golang.org/dl/)

### Optional Tools

```bash
# Install all dev tools
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/air-verse/air@latest
go install go.uber.org/mock/mockgen@latest
go install golang.org/x/tools/cmd/goimports@latest
```

| Tool | Purpose |
|------|---------|
| task | Task runner (recommended) |
| golangci-lint | Linting |
| air | Live reload for development |
| mockgen | Mock generation for tests |
| goimports | Import formatting |

Ensure `~/go/bin` is in your PATH:
```bash
export PATH="$PATH:$HOME/go/bin"
```

### Build & Test

```bash
# Using Task (recommended)
task build        # Build both binaries
task test         # Run tests
task lint         # Run linter
task check        # fmt + lint + test
task dev          # Live reload server

# Or directly with Go
go build ./cmd/...
go test ./...
```

## Architecture

```
┌──────────┐      ┌─────────────────────────────────────────┐
│  arrgo   │ HTTP │                 arrgod                  │
│  (CLI)   │─────▶│  ┌─────────┐ ┌─────────┐ ┌───────────┐  │
└──────────┘      │  │ Library │ │ Search  │ │ Download  │  │
                  │  └─────────┘ └─────────┘ └───────────┘  │
                  │  ┌─────────────────────────────────────┐│
                  │  │   REST API  +  Importer  +  SQLite  ││
                  │  └─────────────────────────────────────┘│
                  └─────────────────────────────────────────┘
                          │            │           │
                     ┌────▼────┐  ┌────▼────┐ ┌────▼────┐
                     │Indexers │  │SABnzbd  │ │  Plex   │
                     │(Newznab)│  └─────────┘ └─────────┘
                     └─────────┘
```

## External Dependencies

- **Usenet indexers** — Direct Newznab support (NZBgeek, DrunkenSlug, etc.)
- **SABnzbd** — Usenet downloads
- **Plex** — Media server integration (status, scanning, library queries)
- **Overseerr** — Request management (optional, via compat API)
