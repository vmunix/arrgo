# Plex Status Command Design

**Status:** ✅ Complete

## Goal

Add `arrgo plex status` command to verify Plex connection and display library information.

## Architecture

```
arrgo plex status → HTTP GET /api/v1/plex/status → PlexClient → Plex API
```

## Components

### 1. CLI (`cmd/arrgo/plex.go`)

- `plex` command group for future expansion
- `plex status` subcommand
- Human-readable and JSON output support

### 2. API Endpoint (`internal/api/v1/plex.go`)

```go
// GET /api/v1/plex/status
type PlexStatusResponse struct {
    Connected   bool          `json:"connected"`
    ServerName  string        `json:"server_name,omitempty"`
    Version     string        `json:"version,omitempty"`
    Libraries   []PlexLibrary `json:"libraries,omitempty"`
    Error       string        `json:"error,omitempty"`
}

type PlexLibrary struct {
    Key        string `json:"key"`
    Title      string `json:"title"`
    Type       string `json:"type"`       // "movie" or "show"
    ItemCount  int    `json:"item_count"`
    Location   string `json:"location"`
    ScannedAt  string `json:"scanned_at"` // RFC3339
    Refreshing bool   `json:"refreshing"`
}
```

### 3. PlexClient Enhancement (`internal/importer/plex.go`)

Add `GetIdentity()` method:

```go
type Identity struct {
    Name    string
    Version string
}

func (c *PlexClient) GetIdentity(ctx context.Context) (*Identity, error)
```

## CLI Output

**Success:**
```
Plex: velcro (1.42.2.10156) ✓

Libraries:
  Movies      32 items   /data/media/movies   scanned 2h ago
  TV Shows     6 items   /data/media/tv       scanned 1h ago
```

**Not configured:**
```
Plex: not configured

Configure in config.toml:
  [notifications.plex]
  url = "http://localhost:32400"
  token = "your-token"
```

**Unreachable:**
```
Plex: connection failed ✗
  Error: connection refused
```

## Testing

- Unit test for `GetIdentity()` with mock HTTP server
- Integration test for `/api/v1/plex/status` endpoint
- CLI output formatting tests
