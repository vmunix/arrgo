# Backlog

Future work items, organized by category.

## Post-v1 Feature Complete

- **Subprocess smoke test** - Full end-to-end test starting `arrgod` as subprocess with real ports, validating live system health before releases. Pattern: Radarr's `NzbDroneRunner` spawns app, polls `/api/v1/status` until ready, runs tests against real HTTP server, kills on teardown.
- **Config init connection validation** - Test indexer/SABnzbd reachability before writing config.toml, with clear error messages if services are unreachable

## v2 Features

- Torrent support (Torznab + qBittorrent)
- RSS monitoring and auto-grab
- Quality upgrades
- Web UI

## Tech Debt

_(Items identified by code review)_

- **Unused `sabnzbdErr` field** - `integration_test.go` has `sabnzbdErr error` in testEnv but it's never used. Either use it to simulate error conditions or remove it.
- **Ignored errors in test helpers** - `httpPost` ignores `json.Marshal` error, `decodeJSON` ignores `io.ReadAll` error, DB helpers ignore `LastInsertId` error. Consider handling for better debugging.
- **Scoring logic duplication** - `cmd/arrgo/parse.go` duplicates functions from `internal/search/scorer.go`: `hdrMatches()`, `audioMatches()`, `matchesRejectList()`, `rejectMatchesSpecial()`, and score constants. Extract to shared package.
- **Parse command tests missing** - `cmd/arrgo/parse.go` has no unit tests for CLI-specific functionality (file reading, JSON output, human formatting).
- **Low-quality source detection** - Reject list supports `cam`, `ts`, `telesync`, `hdcam` but parser doesn't detect these. Would require extending `pkg/release` Source enum.

## Nice-to-Haves

_(Ideas that aren't blocking v1)_

- **Integration test: verify indexer propagation** - `TestIntegration_SearchAndGrab` could also verify `dl.Indexer` matches the grabbed release
- **Integration test: negative/edge cases** - Add tests for error paths (e.g., grab with non-existent content_id, missing required fields)
- **Integration test: Plex/import flow** - When import is fully wired, add tests verifying file import and Plex notification
- **Mock server improvements** - Add API key validation and HTTP method checks to mock Prowlarr/SABnzbd for more realistic testing
