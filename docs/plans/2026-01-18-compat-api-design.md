# Compat API Wiring Design

**Date:** 2026-01-18
**Status:** Approved

## Overview

Wire Radarr/Sonarr compat API handlers to actual stores for Overseerr integration. One-way flow: Overseerr adds content, arrgo handles the rest.

## Dependencies

- `library.Store` - add content
- `download.Store` - list queue
- `search.Searcher` - trigger searches (optional)
- `download.Manager` - grab releases (optional)
- Config: movie root, series root, quality profiles

## Endpoints

| Endpoint | Action |
|----------|--------|
| `POST /api/v3/movie` | Add movie, auto-search |
| `POST /api/v3/series` | Add series, auto-search |
| `GET /api/v3/rootfolder` | Return configured roots |
| `GET /api/v3/qualityprofile` | Return configured profiles |
| `GET /api/v3/queue` | Return active downloads |

## Add Flow

1. Parse request (tmdbId/tvdbId, title, qualityProfileId)
2. Map qualityProfileId to profile name
3. Add to library
4. If searcher configured: search and grab best match
5. Return Radarr-format response

## Out of Scope

- `GET /api/v3/movie` - return empty (one-way)
- `GET /api/v3/series` - return empty (one-way)
- Full Radarr/Sonarr field compatibility
