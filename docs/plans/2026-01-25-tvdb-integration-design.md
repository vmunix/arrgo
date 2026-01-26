# TVDB Integration Design

**Issue:** #69 - Add TVDB API integration for series metadata
**Date:** 2026-01-25
**Status:** Approved

## Problem

Currently arrgo lacks external metadata for TV series:
- Year shows as 0/"-" in Overseerr because we return year=0 for unknown series
- Episode counts are incomplete - "12/12" may look complete but the series has 36 episodes
- No "missing" calculation - can't show what episodes are available to grab
- No episode metadata - missing titles, air dates

## Goals

1. **Overseerr accuracy** - Return real year and episode counts in compat API
2. **Library completeness tracking** - Know what episodes are missing across the library

## Design Decisions

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| Episode population | Eager fetch | Create all Episode records when series added. Enables immediate "missing" queries. |
| Authentication | Project API key | Users configure their own TVDB API key in `config.toml` |
| Caching | SQLite persistence | `metadata_cache` table survives restarts, reduces API calls |
| Code location | `pkg/tvdb/` + `internal/metadata/` | Client in pkg for reusability, caching in internal |
| Missing TVDB ID | Graceful degradation | Series works without TVDB ID, just no episode metadata |
| CLI integration | Full with disambiguation | TVDB lookup on search, interactive selection for multiple matches |

## Package Structure

```
pkg/tvdb/
├── client.go      # HTTP client, auth, rate limiting
├── types.go       # Series, Episode, SearchResult structs
└── tvdb_test.go   # Unit tests with mock responses

internal/metadata/
├── cache.go       # SQLite cache table operations
├── tvdb.go        # TVDB orchestration (fetch + cache)
└── metadata.go    # Shared interface for future TMDB unification
```

## API Client

```go
// pkg/tvdb/client.go
type Client struct {
    apiKey     string
    httpClient *http.Client
    baseURL    string
    token      string        // JWT from login
    tokenExp   time.Time
}

func New(apiKey string) *Client

func (c *Client) Login(ctx context.Context) error
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error)
func (c *Client) GetSeries(ctx context.Context, tvdbID int) (*Series, error)
func (c *Client) GetEpisodes(ctx context.Context, tvdbID int) ([]Episode, error)
```

```go
// pkg/tvdb/types.go
type Series struct {
    ID        int
    Name      string
    Year      int       // From firstAired
    Status    string    // "Continuing" or "Ended"
    Overview  string
}

type Episode struct {
    ID       int
    Season   int
    Episode  int
    Name     string
    AirDate  time.Time
}
```

The client handles JWT authentication automatically - calls `Login()` if token is missing or expired.

## SQLite Cache

**Migration:**
```sql
CREATE TABLE metadata_cache (
    key        TEXT PRIMARY KEY,  -- e.g., "tvdb:series:12345"
    value      TEXT NOT NULL,     -- JSON response
    expires_at DATETIME NOT NULL
);

CREATE INDEX idx_metadata_cache_expires ON metadata_cache(expires_at);
```

**Cache operations:**
```go
type Cache struct {
    db *sql.DB
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool)
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration)
func (c *Cache) Delete(ctx context.Context, key string)
func (c *Cache) Prune(ctx context.Context) (int64, error)
```

**TTL strategy:**
- Series metadata: 7 days
- Episode list: 24 hours
- Search results: 1 hour

**Cache key format:**
- `tvdb:series:{id}`
- `tvdb:episodes:{id}`
- `tvdb:search:{query_hash}`

Pruning runs on server startup and daily.

## Integration Points

### 1. Series Addition (eager fetch)

When a series is added via compat API or CLI with a TVDB ID:

```
AddSeries(tvdbID=12345)
  → metadata.GetSeries(12345)     // Fetch + cache series info
  → metadata.GetEpisodes(12345)   // Fetch + cache episode list
  → content.Create(year=2013, ...)
  → episodes.BulkCreate(...)      // Create Episode records
```

Location: `internal/library/content.go`

### 2. Compat API lookupSeries

`GET /api/v3/series/lookup?term=...` returns real year from TVDB.

Location: `internal/api/compat/series.go`

### 3. Library display

Episode stats denominator comes from TVDB-populated Episode records.
"12/36" instead of "12/12".

Location: `internal/library/content.go` - `GetSeriesStats()`

### 4. Episode status

New episodes from TVDB get `status = "wanted"`.
After import: `status = "available"`.

## CLI Integration

**Search flow:**
```
arrgo search "The Office" --type series
  → tvdb.Search("The Office")
  → If multiple matches: prompt user to select
  → If single match: auto-select with confirmation
  → Search indexers with TVDB ID
```

**Manual add:**
```
arrgo library add --series "Breaking Bad"
  → tvdb.Search("Breaking Bad")
  → Disambiguation if needed
  → Confirm: "Add Breaking Bad (2008) - 5 seasons, 62 episodes? [Y/n]"
  → Create content + Episode records
```

**Grab enhancement:**
```
arrgo search "Breaking Bad S01" --type series --grab best
  → TVDB lookup (cached)
  → Grab request includes tvdbID
  → Server creates content with full metadata
```

**Flags:**
- `--type series` - Triggers TVDB lookup
- `--tvdb <id>` - Skip search, use known ID
- `--yes` - Auto-select first match (non-interactive)

## Error Handling

**API failures:**
- TVDB down/timeout: Proceed without metadata, can enrich later
- Invalid API key: Clear error with setup instructions
- Rate limited: Exponential backoff, max 3 retries

**Data edge cases:**
- Series not found: Warn, allow add anyway
- No air date: `air_date = NULL`
- Specials: `season = 0`
- Future episodes: `status = "wanted"` with air date

**Idempotency:**
- Re-adding series: Return existing, optionally refresh
- Re-syncing episodes: Skip existing records

## Configuration

```toml
[tvdb]
api_key = "${TVDB_API_KEY}"
```

Users register at https://thetvdb.com/api-information for their own key.

## Testing Strategy

**Unit tests (`pkg/tvdb/`):**
- Mock HTTP responses
- JWT refresh logic
- Error handling (401, 404, 429)

**Integration tests (`internal/metadata/`):**
- Cache hit/miss with real SQLite
- TTL expiration and pruning
- Concurrent access

**Test fixtures:**
- Golden files for known responses
- No live API in CI
- Optional `-tags=integration` for local testing

## Files to Create/Modify

**New:**
- `pkg/tvdb/client.go`
- `pkg/tvdb/types.go`
- `pkg/tvdb/tvdb_test.go`
- `internal/metadata/cache.go`
- `internal/metadata/tvdb.go`
- `internal/metadata/metadata.go`
- `migrations/00X_metadata_cache.sql`

**Modify:**
- `internal/config/config.go` - TVDB config section
- `internal/library/content.go` - Episode creation on series add
- `internal/api/compat/series.go` - Real year in lookupSeries
- `cmd/arrgo/search.go` - TVDB lookup flow
- `cmd/arrgo/library.go` - Manual add command

## Related Issues

- #76 - Refactor TMDB client to `pkg/tmdb/` for consistency
