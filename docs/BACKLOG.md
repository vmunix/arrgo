# Backlog

Future work items, organized by category.

## Post-v1 Feature Complete

- **Subprocess smoke test** - Full end-to-end test starting `arrgod` as subprocess with real ports, validating live system health before releases. Pattern: Radarr's `NzbDroneRunner` spawns app, polls `/api/v1/status` until ready, runs tests against real HTTP server, kills on teardown.

## v2 Features

- AI chat CLI (get core flows working first)
- Torrent support (Torznab + qBittorrent)
- RSS monitoring and auto-grab
- Quality upgrades
- Web UI

## Tech Debt

_(Moved to GitHub issues: #11, #12, #13, #14, #15)_

## Nice-to-Haves

_(Ideas that aren't blocking v1)_

- **Integration test: verify indexer propagation** - `TestIntegration_SearchAndGrab` could also verify `dl.Indexer` matches the grabbed release
- **Integration test: negative/edge cases** - Add tests for error paths (e.g., grab with non-existent content_id, missing required fields)
- **Integration test: Plex/import flow** - Add tests verifying file import and Plex notification (import is now wired)
- **Mock server improvements** - Add API key validation and HTTP method checks to mock Prowlarr/SABnzbd for more realistic testing
