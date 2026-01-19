# Backlog

Future work items, organized by category.

## v2 Features

- **Subprocess smoke test** - Full end-to-end test starting `arrgo serve` as subprocess with real ports, validating live system health before releases
- **Config init connection validation** - Test Prowlarr/SABnzbd reachability before writing config.toml, with clear error messages if services are unreachable

## Tech Debt

_(Items identified by code review)_

- **Unused `sabnzbdErr` field** - `integration_test.go` has `sabnzbdErr error` in testEnv but it's never used. Either use it to simulate error conditions or remove it.
- **Ignored errors in test helpers** - `httpPost` ignores `json.Marshal` error, `decodeJSON` ignores `io.ReadAll` error, DB helpers ignore `LastInsertId` error. Consider handling for better debugging.

## Nice-to-Haves

_(Ideas that aren't blocking v1)_

- **Integration test: verify indexer propagation** - `TestIntegration_SearchAndGrab` could also verify `dl.Indexer` matches the grabbed release
- **Integration test: negative/edge cases** - Add tests for error paths (e.g., grab with non-existent content_id, missing required fields)
- **Integration test: Plex/import flow** - When import is fully wired, add tests verifying file import and Plex notification
- **Mock server improvements** - Add API key validation and HTTP method checks to mock Prowlarr/SABnzbd for more realistic testing
