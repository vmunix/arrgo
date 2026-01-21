# Client-Server Design Notes

## Context

Observed while implementing Radarr/Overseerr compatibility: Radarr's API design assumes a **stateful client** that manages complex decision logic:

1. Client queries state (lookup movie)
2. Client decides action based on state (POST vs PUT vs skip)
3. Client issues appropriate API call with flags like `searchForMovie`

This works for long-running clients (Overseerr, web UIs) but is awkward for CLI tools.

## Current State

- **CLI (arrgo)**: Stateless, only runs when user is present
- **Compat API**: Mirrors Radarr's client-driven model (required for Overseerr)
- **Native API**: Currently simple CRUD, client must orchestrate

## Proposed Direction: Server-Side Intelligence

**Principle**: arrgod handles all complexity; clients are thin state displayers and command issuers.

### Atomic Operations

Instead of clients orchestrating multi-step flows:

```bash
# Client doesn't need to know: lookup → check state → POST or PUT → maybe search
arrgo want "Deadpool & Wolverine"  # Server figures out the right action
```

Server handles:
- Check if exists → create if not
- Check if downloading → no-op if yes
- Check if available → report if yes
- Otherwise → trigger search

### Event/Callback Model

For TUI and real-time clients:

```
Client                          Server
  |                               |
  |-- subscribe(content:7) ------>|
  |                               |
  |<-- event: search_started -----|
  |<-- event: release_found ------|
  |<-- event: download_queued ----|
  |<-- event: download_progress --|  (periodic)
  |<-- event: download_complete --|
  |<-- event: import_complete ----|
  |                               |
```

Options:
- **WebSocket**: Real-time bidirectional
- **SSE (Server-Sent Events)**: Simpler, one-way push
- **Polling with ETag**: Stateless fallback

### Background Automation

Server-side tasks (already partially implemented):
- Download status polling ✓
- Auto-import on completion ✓
- **TODO**: Periodic search for stale "wanted" items
- **TODO**: RSS feed monitoring
- **TODO**: Retry failed downloads

## API Design Implications

### Native API (v1) - Target Design

```
POST /api/v1/want
{
  "title": "Movie Name",
  "year": 2024,
  "tmdb_id": 12345
}

Response:
{
  "content_id": 7,
  "action": "search_triggered",  // or "already_downloading", "already_available"
  "download_id": 42              // if search found something
}
```

Single call, server decides everything.

### Compat API (v3) - Constrained by Radarr

Must maintain client-driven model for Overseerr compatibility.
Workarounds (like returning `monitored: false` to trigger PUT) are acceptable here.

## Future Client Types

| Client | Model | Notes |
|--------|-------|-------|
| CLI (arrgo) | Stateless commands | Atomic operations, exits after each command |
| TUI | Event-driven | WebSocket/SSE for real-time updates |
| Web UI | Event-driven | Similar to TUI |
| Overseerr | Compat API | Client-driven, we adapt |
| Mobile app | Event-driven | Push notifications possible |

## Summary

- Keep compat API client-driven (necessary evil)
- Design native API for atomic, server-intelligent operations
- Add event streaming for rich clients (TUI, web)
- All business logic lives in arrgod; clients are presentation layer
