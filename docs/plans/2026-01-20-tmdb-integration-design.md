# TMDB Integration Design

**Status:** ✅ Complete

## Overview

Minimal TMDB API integration to enrich Overseerr compatibility responses with real metadata.

## Scope

**In scope:**
- TMDB API client that fetches movie metadata by TMDB ID
- Response caching to avoid rate limits
- Wire into Overseerr compat `/api/v3/movie/lookup` endpoint

**Out of scope (future work):**
- Search by title
- TV series metadata
- Storing metadata in SQLite
- Rich CLI output

## Architecture

### New Package: `internal/tmdb/`

```
internal/tmdb/
├── client.go      # HTTP client, API calls
├── types.go       # Response structs
└── cache.go       # In-memory cache with TTL
```

### TMDB API Usage

```
GET https://api.themoviedb.org/3/movie/{tmdb_id}?api_key=XXX
```

### Configuration

```toml
[tmdb]
api_key = "${TMDB_API_KEY}"
```

TMDB API keys are free—requires account creation at themoviedb.org.

### Cache Strategy

- In-memory map with TTL (24 hours default)
- Key: TMDB ID
- No persistence—cache rebuilds on restart
- Simple mutex-protected map

## Data Types

### TMDB Response

```go
type Movie struct {
    ID          int64   `json:"id"`
    Title       string  `json:"title"`
    Overview    string  `json:"overview"`
    ReleaseDate string  `json:"release_date"`  // "2024-03-01"
    PosterPath  string  `json:"poster_path"`   // "/abc123.jpg"
    VoteAverage float64 `json:"vote_average"`  // 8.5
    Runtime     int     `json:"runtime"`       // minutes
    Genres      []Genre `json:"genres"`
}

type Genre struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}
```

### Radarr Response Mapping

```go
map[string]any{
    "tmdbId":      tmdbID,
    "title":       metadata.Title,
    "year":        parseYear(metadata.ReleaseDate),
    "overview":    metadata.Overview,
    "runtime":     metadata.Runtime,
    "ratings":     map[string]any{"tmdb": map[string]any{"value": metadata.VoteAverage}},
    "images":      buildImageList(metadata.PosterPath),
    "genres":      metadata.Genres,
    "monitored":   false,
    "hasFile":     false,
    "isAvailable": false,
}
```

### Image URLs

TMDB returns relative paths. Full URL format:
```
https://image.tmdb.org/t/p/w500/abc123.jpg
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Invalid API key | Log warning, return stub response |
| Rate limited (429) | Log warning, return stub response |
| Movie not found (404) | Return empty result |
| Network timeout | Log warning, return stub response |
| No API key configured | Skip TMDB lookup entirely, use stub |

**Key principle:** TMDB is an enhancement, not a requirement. Overseerr flow must never break due to TMDB issues.

## Files to Create/Modify

### New Files
- `internal/tmdb/client.go` — HTTP client with GetMovie method
- `internal/tmdb/types.go` — Response structs
- `internal/tmdb/cache.go` — TTL cache
- `internal/tmdb/client_test.go` — Unit tests

### Modified Files
- `internal/config/config.go` — Add TMDB config section
- `internal/api/compat/compat.go` — Wire TMDB into lookupMovie
- `cmd/arrgod/main.go` — Initialize TMDB client
- `config.example.toml` — Add TMDB example config

## Testing Strategy

- Use `httptest` server to mock TMDB API responses
- Test cache TTL behavior
- Test graceful degradation on errors
- Integration test with compat API
