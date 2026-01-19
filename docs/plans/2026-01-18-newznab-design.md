# Direct Newznab Support Design

**Date:** 2026-01-18
**Status:** ✅ Complete

## Overview

Replace Prowlarr integration with direct Newznab protocol support. arrgo talks directly to usenet indexers (NZBgeek, DrunkenSlug, etc.) without needing Prowlarr as a middleman.

## Design Decisions

1. **Remove Prowlarr support** - Direct Newznab is cleaner, eliminates a dependency
2. **Named indexers in config** - `[indexers.nzbgeek]` style, consistent with downloaders
3. **Parallel search** - Query all indexers concurrently, merge results
4. **Partial failure tolerance** - Return results from working indexers even if some fail

## Config Format

```toml
[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "your-key"

[indexers.drunkenslug]
url = "https://api.drunkenslug.com"
api_key = "your-key"
```

## Implementation

### pkg/newznab/client.go

Reusable Newznab protocol client:

```go
package newznab

type Client struct {
    name    string
    baseURL string
    apiKey  string
}

type Release struct {
    Title       string
    GUID        string
    DownloadURL string
    Size        int64
    PublishDate time.Time
    Category    int
    Indexer     string
}

func (c *Client) Search(ctx context.Context, query string, categories []int) ([]Release, error)
```

Parses Newznab RSS/XML responses. Parameters:
- `t=search|movie|tvsearch`
- `q=query`
- `cat=2000,2040` (categories)
- `apikey=...`

### internal/search/indexer.go

Manages multiple indexers:

```go
type IndexerPool struct {
    clients []*newznab.Client
}

func (p *IndexerPool) Search(ctx context.Context, q Query) ([]Release, []error)
```

- Spawns goroutines for parallel queries
- 30s timeout per indexer
- Returns partial results + errors

### internal/search/search.go

Update interface:

```go
// Before
type ProwlarrAPI interface {
    Search(ctx context.Context, q Query) ([]ProwlarrRelease, error)
}

// After
type IndexerAPI interface {
    Search(ctx context.Context, q Query) ([]Release, []error)
}
```

Searcher unchanged - still scores/filters/sorts.

## Files Changed

| Action | File |
|--------|------|
| Create | `pkg/newznab/client.go` |
| Create | `pkg/newznab/client_test.go` |
| Create | `internal/search/indexer.go` |
| Modify | `internal/search/search.go` |
| Delete | `internal/search/prowlarr.go` |
| Delete | `internal/search/prowlarr_test.go` |
| Modify | `internal/config/config.go` |
| Modify | `cmd/arrgo/init.go` |

## Init Wizard

```
arrgo setup wizard

Indexer URL [https://api.nzbgeek.info]:
Indexer API Key: ****
Indexer Name [nzbgeek]:

SABnzbd URL [http://localhost:8085]:
...
```

## Error Handling

- Individual indexer failures don't fail the search
- Return partial results + error list
- Log errors, show whatever results we got
- 30s timeout per indexer

## Newznab Protocol Reference

Standard parameters:
- `t` - Search type: `search`, `movie`, `tvsearch`, `music`
- `q` - Query string
- `apikey` - API key
- `cat` - Category IDs (comma-separated)
- `imdbid` - IMDB ID (for movies)
- `tvdbid` - TVDB ID (for TV)
- `season` / `ep` - Season/episode numbers

Category IDs:
- 2000 = Movies
- 2040 = Movies/HD
- 2045 = Movies/UHD
- 5000 = TV
- 5040 = TV/HD
