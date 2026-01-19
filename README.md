# arrgo

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

The \*arr stack felt perfect for the job because it's both terrible and real
world battle tested against dirty, nasty, crazy human curated data. It's APIs
are full of both deeply bizarre and subtly inconsistent expectations. Nothing
is well specified. In other words the kind of work I avoid doing for fun at all
costs.. the kind of thing I would intuitively say an LLM would suck at. 

**Arrgo: Unified media automation in Go. A single, coherent replacement for the \*arr stack.**

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
│                    arrgo                        │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌────────┐ │
│  │ Library │ │ Search  │ │Download │ │ Import │ │
│  └─────────┘ └─────────┘ └─────────┘ └────────┘ │
│  ┌─────────────────────────────────────────────┐│
│  │        REST API  +  AI Chat  +  SQLite      ││
│  └─────────────────────────────────────────────┘│
└─────────────────────────────────────────────────┘
        │            │           │
   ┌────▼────┐  ┌────▼────┐ ┌────▼────┐
   │Indexers │  │SABnzbd  │ │  Plex   │
   │(Newznab)│  └─────────┘ └─────────┘
   └─────────┘
```

## External Dependencies

- **Usenet indexers** — Direct Newznab support (NZBgeek, DrunkenSlug, etc.)
- **SABnzbd** — Usenet downloads
- **Plex** — Media server notifications
- **Overseerr** — Request management (optional, via compat API)
- **Ollama** — Local AI for chat mode (optional)

## License

MIT
