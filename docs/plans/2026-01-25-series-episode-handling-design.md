# Series Episode Handling Design

> **Status:** READY FOR IMPLEMENTATION
> **Related Issue:** #67 (Import handler doesn't support series season packs)

**Goal:** Fix the disconnect between release parsing and episode tracking so that season packs and multi-episode releases import correctly.

**Architecture:** Parse release names at grab time to detect episodes, create episode records on-demand, and map individual files to episodes during import.

**Tech Stack:** Go, SQLite (junction table for download-to-episodes)

---

## Problem Statement

Today there's a disconnect between release parsing and episode tracking:

```
Search → Parser extracts "S01E05" → Works
Grab   → Creates download with episode_id=??? → Requires pre-existing episode
Import → Needs episode_id from download → Fails for season packs
```

The parser knows what episodes are in a release, but that info isn't used to find/create the right episode records. This causes:

- Season pack imports fail with "episode not specified for series download"
- Multi-episode releases (S01E05E06E07) can't be tracked properly
- Downloads can be imported to wrong episode slots (no validation)

---

## Core Principle

**The release name is the source of truth** for what episodes are being grabbed. Database episode records are created/matched based on parsed info, not the other way around.

```
Grab Request (release name, content_id)
    │
    ▼
Parse release name → Info{Season:1, Episodes:[5,6,7], IsCompleteSeason:false}
    │
    ▼
Find or create episode records for S01E05, S01E06, S01E07
    │
    ▼
Create download with episode references
    │
    ▼
Import: For each episode in download, create file record
```

---

## Data Model Changes

### New Junction Table

Downloads can reference multiple episodes. Replace single `episode_id` with a junction table:

```sql
CREATE TABLE download_episodes (
    download_id INTEGER NOT NULL REFERENCES downloads(id) ON DELETE CASCADE,
    episode_id  INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    PRIMARY KEY (download_id, episode_id)
);
```

### Migration Path

1. Create `download_episodes` table
2. Migrate existing data: `INSERT INTO download_episodes SELECT id, episode_id FROM downloads WHERE episode_id IS NOT NULL`
3. Keep `episode_id` column temporarily for backward compat
4. Drop `episode_id` column in later migration

### Download Struct Changes

```go
type Download struct {
    // ... existing fields ...
    EpisodeID        *int64   // Deprecated, keep for migration
    EpisodeIDs       []int64  // New: populated from junction table
    Season           *int     // For season packs: which season
    IsCompleteSeason bool     // True if season pack
}
```

---

## Grab Flow

### Current Flow (Broken)

```
POST /api/v1/grab {content_id, download_url, title, indexer}
    → Emit GrabRequested (content_id, episode_id from request body)
    → DownloadHandler creates download with episode_id
```

Relies on caller to provide episode_id, which is often missing.

### New Flow

```
POST /api/v1/grab {content_id, download_url, title, indexer}
    │
    ▼
Parse title → Info{Season, Episodes, IsCompleteSeason}
    │
    ▼
If series content:
    ├─ IsCompleteSeason? → Store season in download, defer episode creation to import
    ├─ Episodes=[5,6,7]? → Find/create those specific episodes now
    └─ No episode info?  → Error: "cannot determine episodes from release"
    │
    ▼
Emit GrabRequested event (content_id, episode_ids[])
    │
    ▼
DownloadHandler creates download + download_episodes records
```

### API Compatibility

Request body stays the same. Optional explicit override for edge cases:

```json
{
  "content_id": 42,
  "download_url": "...",
  "title": "Show.S01.1080p.WEB",
  "indexer": "nzbgeek",
  "season": 1,
  "episodes": [1,2,3]
}
```

---

## Import Flow

### Current Flow (Broken for Season Packs)

```
DownloadCompleted → Find source files → Import single file with episode_id
```

Fails for season packs with multiple video files.

### New Flow

```
DownloadCompleted event (download has episode_ids[] or IsCompleteSeason)
    │
    ▼
Find source files in download folder
    │
    ▼
For each video file:
    ├─ Parse filename → extract season/episode
    ├─ Match to expected episode OR create new (for season packs)
    ├─ Import file, create file record with episode_id
    └─ Update episode.status = 'available'
    │
    ▼
ImportCompleted with per-episode results
```

### File-to-Episode Matching

```go
func matchFileToEpisode(filename string, expectedEpisodes []Episode) (*Episode, error) {
    info := release.Parse(filename)

    if len(info.Episodes) >= 1 {
        for _, ep := range expectedEpisodes {
            if ep.Season == info.Season && ep.Episode == info.Episodes[0] {
                return &ep, nil
            }
        }
    }

    return nil, fmt.Errorf("no matching episode for %s", filename)
}
```

### Season Pack Episode Discovery

For season packs, episodes are created during import based on actual files:

```
Grab "Show.S01.1080p":
  → IsCompleteSeason=true, Season=1
  → No episode records created yet

Import:
  → Scan files: S01E01.mkv, S01E02.mkv, ... S01E10.mkv
  → For each file, FindOrCreateEpisode
  → Episodes created based on what's in the pack
```

This avoids external API dependencies and handles packs with non-standard episode counts.

### Edge Cases

| Scenario | Handling |
|----------|----------|
| File matches no expected episode | Log warning, skip file |
| Extra episodes in pack | Import anyway (bonus content) |
| Missing episodes from pack | Those episodes remain 'wanted' |
| Multi-episode file (S01E05E06.mkv) | Link to first episode |

---

## Event Changes

### GrabRequested

```go
type GrabRequested struct {
    ContentID        int64
    EpisodeIDs       []int64  // Multiple episodes (empty for season packs)
    Season           *int     // Set for season packs
    IsCompleteSeason bool
    ReleaseName      string
    DownloadURL      string
    Indexer          string
}
```

### ImportCompleted

```go
type ImportCompleted struct {
    ContentID      int64
    DownloadID     int64
    EpisodeResults []EpisodeImportResult
}

type EpisodeImportResult struct {
    EpisodeID int64
    Season    int
    Episode   int
    Success   bool
    FilePath  string  // Empty if failed
    Error     string  // Empty if success
}

// Convenience methods
func (e *ImportCompleted) AllSucceeded() bool
func (e *ImportCompleted) SuccessCount() int
func (e *ImportCompleted) FailedCount() int
```

### Downstream Impact

- **CleanupHandler**: Keys on ContentID, no change needed
- **PlexAdapter**: Checks content-level, no change needed
- **History**: Update to record per-episode results in metadata

---

## Error Handling

### Grab-Time Errors

| Condition | Response |
|-----------|----------|
| Series + unparseable release name | 400: "cannot determine season/episodes from release title" |
| Series + no season detected | 400: "release title missing season info" |
| Movie + episode info detected | Warning log, proceed as movie |

### Import-Time Errors

| Condition | Handling |
|-----------|----------|
| No video files found | ImportFailed event |
| Some files match, some don't | Import matched, log warnings for unmatched |
| All files fail to match | ImportFailed with details |

---

## Files to Modify

| File | Changes |
|------|---------|
| `migrations/00X_download_episodes.sql` | New junction table, migrate existing data |
| `internal/download/store.go` | Add `download_episodes` CRUD |
| `internal/download/download.go` | Add `EpisodeIDs`, `Season`, `IsCompleteSeason` |
| `internal/events/download.go` | Update event structs with episode arrays |
| `internal/api/v1/api.go` | Grab endpoint: parse release, find/create episodes |
| `internal/handlers/download.go` | Create download_episodes records |
| `internal/handlers/import.go` | Multi-file import, file-to-episode matching |
| `internal/library/episodes.go` | Add `FindOrCreateEpisode` helper |

## Files to Add

| File | Purpose |
|------|---------|
| `internal/importer/episode_matcher.go` | `matchFileToEpisode` logic |

---

## Testing Strategy

### Unit Tests

- Release parser: verify season pack detection flags
- `FindOrCreateEpisode`: create new, find existing, handle duplicates
- `matchFileToEpisode`: single ep, multi-ep file, no match, extra episodes
- Download store: CRUD with junction table

### Integration Tests

- `TestGrab_SeasonPack`: verify IsCompleteSeason flag, no premature episode creation
- `TestGrab_MultiEpisode`: verify multiple episode records created
- `TestImport_SeasonPack`: verify per-file episode creation and file records
- `TestImport_SeasonPack_PartialMatch`: verify partial success handling
- `TestImport_SingleEpisode`: verify existing flow still works

### Manual Testing

- [ ] Grab season pack from search results
- [ ] Verify SABnzbd completes download
- [ ] Verify import creates correct episode/file records
- [ ] Verify Plex detection triggers cleanup
- [ ] Test with Peaky Blinders S01 (existing test case #28)

---

## Out of Scope (Stage 2 / v2)

- Per-episode quality tracking and upgrades (#55)
- Episode-level monitoring (wanted/unmonitored per episode)
- CLI episode management commands
- TVDB/TMDB episode metadata sync

---

## Implementation Order

1. Migration + download store changes (foundation)
2. Event struct updates (GrabRequested, ImportCompleted)
3. Episode finder/creator in library package
4. Grab endpoint changes (parse → find/create → emit)
5. DownloadHandler changes (junction table writes)
6. ImportHandler changes (multi-file, episode matching)
7. Integration tests with test case #28
