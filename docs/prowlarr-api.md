# Prowlarr API Reference

Reference for integrating with Prowlarr.

## Two API Approaches

Prowlarr exposes two different APIs:

### 1. Internal API (`/api/v1/...`)

The Prowlarr management API for its own UI:

```bash
# Search all indexers
GET /api/v1/search?query=ninja+scroll
Header: X-Api-Key: <prowlarr-api-key>

# List indexers
GET /api/v1/indexer
```

**Problem:** This API doesn't reliably return all indexer results. In testing, usenet indexers (NZBgeek) return empty via this endpoint even when they have results.

### 2. Newznab/Torznab API (`/{indexerId}/api`)

Each synced indexer gets a dedicated Newznab (usenet) or Torznab (torrent) endpoint:

```bash
# Search specific indexer
GET /{indexerId}/api?apikey=<prowlarr-api-key>&t=search&q=ninja+scroll

# Search with movie type
GET /{indexerId}/api?apikey=<prowlarr-api-key>&t=movie&q=ninja+scroll

# Search with TV type + TVDB ID
GET /{indexerId}/api?apikey=<prowlarr-api-key>&t=tvsearch&q=show+name&tvdbid=12345
```

**This is how Radarr/Sonarr integrate with Prowlarr.** Each indexer is added as a separate source with its dedicated endpoint.

## Indexer Discovery

To get available indexers and their IDs:

```bash
# List all indexers
GET /api/v1/indexer
```

Response includes:
- `id` - Indexer ID for building the endpoint URL
- `name` - Display name
- `protocol` - "usenet" or "torrent"
- `enable` - Whether indexer is active

## Database Reference

Prowlarr stores config in SQLite at `/srv/prowlarr/prowlarr.db`:

| Table | Purpose |
|-------|---------|
| `Applications` | Connected apps (Radarr, Sonarr) with sync settings |
| `Indexers` | Configured indexers with credentials |
| `ApplicationIndexerMapping` | Which indexers sync to which apps |
| `AppSyncProfiles` | Sync level profiles |

### Sync Levels (in Applications.SyncLevel)

- `1` = Disabled (no sync)
- `2` = Add and Remove Only (manages indexer list)
- `3` = Full Sync (includes RSS sync)

### How Radarr Stores Prowlarr Indexers

In Radarr's database (`/srv/radarr/radarr.db`), the `Indexers` table shows synced indexers:

```json
{
  "baseUrl": "http://localhost:9696/1/",
  "apiPath": "/api",
  "apiKey": "<prowlarr-api-key>",
  "categories": [2000, 2010, 2020, ...]
}
```

- `baseUrl` includes the indexer ID
- Uses Prowlarr's API key (not the indexer's native key)
- Categories filter results (2000 = Movies, 5000 = TV, etc.)

## Recommended Integration

For arrgo, use the same approach as Radarr:

1. **Discovery:** Query `/api/v1/indexer` to get available indexers
2. **Filter:** Select indexers by protocol ("usenet" for SABnzbd)
3. **Search:** Call each indexer's Newznab endpoint: `/{id}/api?t=search&q=...`
4. **Merge:** Combine and deduplicate results

This ensures arrgo sees the same results Radarr would see.

## Newznab API Parameters

Standard parameters for `/{id}/api`:

| Param | Description |
|-------|-------------|
| `t` | Search type: `search`, `movie`, `tvsearch`, `music` |
| `q` | Query string |
| `apikey` | Prowlarr API key |
| `cat` | Category filter (comma-separated IDs) |
| `imdbid` | IMDB ID (for movies) |
| `tvdbid` | TVDB ID (for TV) |
| `season` | Season number |
| `ep` | Episode number |

## Category IDs

| ID | Category |
|----|----------|
| 2000 | Movies |
| 2010 | Movies/Foreign |
| 2030 | Movies/SD |
| 2040 | Movies/HD |
| 2045 | Movies/UHD |
| 2050 | Movies/BluRay |
| 5000 | TV |
| 5030 | TV/SD |
| 5040 | TV/HD |
| 5045 | TV/UHD |
| 5070 | TV/Anime |
