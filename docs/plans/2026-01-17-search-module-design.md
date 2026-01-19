# Search Module Design

**Date:** 2026-01-17
**Status:** ✅ Complete

## Overview

The search module queries indexers for releases via Prowlarr, parses release names to extract quality information, and scores releases against quality profiles.

## Architecture

Two components:

**`pkg/release/`** - Reusable release name parser
- Parses release names like `The.Matrix.1999.2160p.UHD.BluRay.x264-GROUP`
- Extracts core fields: resolution, source, codec
- Pure parsing logic, no external dependencies
- Used by search module for scoring, import module for file detection

**`internal/search/`** - Prowlarr integration and quality scoring
- `ProwlarrClient` - HTTP client for Prowlarr API
- `Scorer` - Matches releases against quality profiles
- `Searcher` - Orchestrates search flow

### Data Flow

```
Query → ProwlarrClient.Search() → raw results
     → release.Parse() each title → structured Release
     → Scorer.Score() each release → scored Release
     → sort by score descending → SearchResult
```

## Release Parser (`pkg/release`)

### Structs

```go
type Release struct {
    Title      string     // Original release name
    Resolution Resolution // r720p, r1080p, r2160p, rUnknown
    Source     Source     // BluRay, WEBDL, HDTV, etc.
    Codec      Codec      // x264, x265, HEVC, etc.
}

type Resolution int // enum with String() method
type Source int     // enum with String() method
type Codec int      // enum with String() method
```

### Parsing Approach

Regex patterns against release name, case-insensitive. Check specific patterns first.

- Resolution: `2160p|4k` → r2160p, `1080p` → r1080p, `720p` → r720p
- Source: `blu-?ray|bdrip` → BluRay, `web-?dl|webdl` → WEBDL, `webrip` → WEBRip, `hdtv` → HDTV
- Codec: `x\.?265|hevc|h\.?265` → x265, `x\.?264|h\.?264` → x264

No match returns `Unknown` variant for that field.

### Test Data

Build table-driven tests with 1000+ real release names sourced from:
- Public indexer APIs
- Prowlarr/Radarr/Sonarr logs
- *arr GitHub issues (parsing bug reports)
- Scene naming standards documentation

Store as `testdata/releases.txt` with expected parse results.

## Quality Scoring

### Profile Configuration

From config:
```toml
[quality.profiles.hd]
accept = ["1080p bluray", "1080p webdl", "1080p hdtv", "720p bluray"]
```

Each entry parsed into `QualitySpec`:
```go
type QualitySpec struct {
    Resolution Resolution // required
    Source     Source     // optional (any if not specified)
}
```

### Scoring Logic

```go
type Scorer struct {
    profiles map[string][]QualitySpec
}

func NewScorer(cfg config.Config) *Scorer

func (s *Scorer) Score(r *Release, profile string) int {
    specs := s.profiles[profile]
    for i, spec := range specs {
        if spec.Matches(r) {
            return len(specs) - i // Higher score for earlier entries
        }
    }
    return 0 // No match = rejected
}
```

Score of 0 means release doesn't match profile and is filtered out. Higher scores indicate better matches. First entry in accept list gets highest score.

## Prowlarr Client

### API Integration

Prowlarr search endpoint: `GET /api/v1/search`

Parameters:
- `query` - search text
- `type` - "search" for text, "tvsearch"/"moviesearch" for ID-based
- `categories` - Newznab categories (2000=movies, 5000=TV)
- `tmdbId` / `tvdbId` - for ID-based searches

### Client Struct

```go
type ProwlarrClient struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func NewProwlarrClient(baseURL, apiKey string) *ProwlarrClient
func (c *ProwlarrClient) Search(ctx context.Context, q Query) ([]ProwlarrRelease, error)
```

### Design Decisions

- Thin HTTP wrapper, no retry logic, no caching
- Context used for timeout/cancellation
- Search all enabled indexers - let Prowlarr handle routing

## Searcher Orchestration

### Result Struct

Partial results with errors:
```go
type SearchResult struct {
    Releases []*Release // Scored and sorted, highest first
    Errors   []error    // Any errors encountered
}
```

### Searcher Struct

```go
type Searcher struct {
    client *ProwlarrClient
    parser *release.Parser
    scorer *Scorer
}

func NewSearcher(client *ProwlarrClient, scorer *Scorer) *Searcher
func (s *Searcher) Search(ctx context.Context, q Query, profile string) (*SearchResult, error)
```

### Query Struct

```go
type Query struct {
    ContentID int64
    Text      string
    Type      string // "movie" or "series"
    TMDBID    *int64
    TVDBID    *int64
    Season    *int
    Episode   *int
}
```

## Error Handling

```go
var (
    ErrProwlarrUnavailable = errors.New("prowlarr unavailable")
    ErrInvalidAPIKey       = errors.New("invalid prowlarr api key")
    ErrNoResults           = errors.New("no matching releases found")
)
```

`ErrNoResults` is informational, not a failure.

## Testing Strategy

### `pkg/release` Parser

- Table-driven tests with 1000+ real release names
- Test unknown/malformed inputs return `Unknown` variants, not errors
- Store test data in `testdata/releases.txt`

### `internal/search`

- Mock `ProwlarrClient` via interface for unit tests:
  ```go
  type ProwlarrAPI interface {
      Search(ctx context.Context, q Query) ([]ProwlarrRelease, error)
  }
  ```
- Table-driven tests for `Scorer` with various profile configurations
- Integration test against real Prowlarr (skipped in CI, manual)

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scope | Prowlarr + pkg/release | Release parsing reusable across modules |
| Parser fields | Core only (resolution, source, codec) | Sufficient for quality scoring |
| Quality scoring | Accept list with ordering | Simple, predictable, covers 90% of use cases |
| Indexer selection | Search all enabled | Let Prowlarr handle complexity |
| Error handling | Partial results with errors | Caller decides if partial is acceptable |
| Caching | None | Prowlarr handles rate limiting, YAGNI |

## Out of Scope (v2+)

- Direct Newznab/Torznab support (bypass Prowlarr)
- Reject patterns in quality profiles
- Weighted quality scoring
- Release caching
- Extended parser fields (group, proper/repack, edition, language, HDR)
