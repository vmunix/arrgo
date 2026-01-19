# Binary Split Design: arrgod + arrgo

**Status:** ✅ Complete

> **For Claude:** Use superpowers:writing-plans to create the implementation plan from this design.

**Goal:** Split the single `arrgo` binary into two: `arrgod` (server daemon) and `arrgo` (CLI client).

**Motivation:**
- Clean separation of concerns between server and client code
- Enable `air` live reloading during server development
- Smaller client binary without server dependencies
- Follow Unix daemon conventions

---

## Directory Structure

```
cmd/
├── arrgo/           # CLI client
│   ├── main.go      # Entry point, command routing
│   ├── client.go    # HTTP client wrapper
│   ├── commands.go  # status, search, queue
│   └── init.go      # Setup wizard
│
└── arrgod/          # Server daemon
    ├── main.go      # Entry point, starts server immediately
    └── server.go    # Server setup (moved from serve.go)
```

The `internal/` and `pkg/` packages remain unchanged. Both binaries import as needed:
- `arrgod` imports: `config`, `api`, `search`, `download`, `importer`, `library`, etc.
- `arrgo` imports: `pkg/release` (for parsing in grab flow), nothing else from internal

---

## Binary Behavior

### arrgod (server daemon)

```bash
arrgod                                   # Start with default config.toml
arrgod --config /etc/arrgo/config.toml   # Custom config path
arrgod --version                         # Print version and exit
```

No subcommands. Starts server immediately, runs until SIGINT/SIGTERM.

### arrgo (CLI client)

```bash
arrgo status               # System status
arrgo search "The Matrix"  # Search indexers
arrgo queue                # Show downloads
arrgo init                 # Setup wizard
arrgo chat                 # AI chat (future)
arrgo ask "question"       # AI one-shot (future)
arrgo --server URL         # Override server URL
arrgo --version            # Print version
```

Default server URL: `http://localhost:8484`

Override via `--server` flag or `ARRGO_SERVER` environment variable.

---

## Development Workflow

### Live reloading with air

`.air.toml` in project root:

```toml
[build]
cmd = "go build -o ./tmp/arrgod ./cmd/arrgod"
bin = "./tmp/arrgod"
include_ext = ["go", "toml"]
exclude_dir = ["tmp", "docs", "cmd/arrgo"]

[misc]
clean_on_exit = true
```

Development workflow:

```bash
# Terminal 1: Server with live reload
air

# Terminal 2: Test CLI commands
go run ./cmd/arrgo search "Back to the Future"
```

### Build commands

```bash
task build                 # Builds both binaries
go build -o arrgo ./cmd/arrgo
go build -o arrgod ./cmd/arrgod
```

---

## Migration Path

### Changes required

1. Move `cmd/arrgo/serve.go` → `cmd/arrgod/server.go`
2. Create `cmd/arrgod/main.go` - minimal entry point
3. Simplify `cmd/arrgo/main.go` - remove `serve` command
4. Update `Taskfile.yml` - build both binaries
5. Add `.air.toml` for live reload

### Unchanged

- All `internal/` packages
- All `pkg/` packages
- Config format
- API endpoints
- Tests

### Backward compatibility

Remove `arrgo serve` cleanly. No deprecation period needed for pre-1.0 software.

---

## Summary

| Aspect | arrgod | arrgo |
|--------|--------|-------|
| Purpose | Server daemon | CLI client |
| Startup | Immediate | Subcommand required |
| Config | `--config` flag | `--server` flag |
| Dev workflow | `air` live reload | `go run` |
