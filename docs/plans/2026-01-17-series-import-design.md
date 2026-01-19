# Series Episode Import Support

**Date:** 2026-01-17
**Status:** ✅ Complete

## Overview

Extend the importer to handle series episode downloads. Single episode per download only - season packs deferred to v2+.

## Behavior

When importing a series download:

1. Require `EpisodeID` to be set on download record (error if not)
2. Look up episode to get season/episode numbers
3. Generate path using `EpisodePath()` template (e.g., `Show/Season 01/Show - S01E05 - 1080p.mkv`)
4. Copy file to `seriesRoot`
5. Link file record to episode via `EpisodeID`
6. Update episode status to "available"
7. Leave series (content) status unchanged

## Error Types

```go
ErrEpisodeNotSpecified = errors.New("episode not specified for series download")
```

## Code Changes

**`internal/importer/errors.go`**
- Add `ErrEpisodeNotSpecified`

**`internal/importer/importer.go`**

In `Import()`, replace the series stub with:
- Validate `dl.EpisodeID` is set
- Fetch episode record via `i.library.GetEpisode()`
- Use `i.renamer.EpisodePath()` for destination
- Use `i.seriesRoot` as root directory
- Set `file.EpisodeID` on the file record
- Update episode status (not content status) in transaction
- Include season/episode in history JSON

## Testing

| Test | Description |
|------|-------------|
| `TestImporter_Import_Episode` | Happy path: file copied, episode status updated, series unchanged |
| `TestImporter_Import_Episode_NoEpisodeID` | Returns `ErrEpisodeNotSpecified` |
| `TestImporter_Import_Episode_EpisodeNotFound` | Returns error for invalid episode |

## Out of Scope

- Season packs (multiple episodes per download)
- Release name parsing to identify episode
- Automatic episode matching
