# Duplicate Prevention Design

## Summary

Prevent grabbing/importing content when equal or better quality already exists in the library. Allow upgrades (better quality) while blocking same or worse quality duplicates.

## Design Decisions

- **Quality comparison:** Resolution-based hierarchy (2160p > 1080p > 720p > 480p)
- **Check points:** Both grab-time and import-time
- **Skip handling:** Emit events (`GrabSkipped`, `ImportSkipped`) for tracking

## New Events

```go
EventGrabSkipped   EventType = "grab_skipped"
EventImportSkipped EventType = "import_skipped"

type GrabSkipped struct {
    *BaseEvent
    ContentID       int64
    ReleaseName     string
    ReleaseQuality  string  // e.g., "1080p"
    ExistingQuality string  // e.g., "2160p"
    Reason          string  // "existing_quality_equal_or_better"
}

type ImportSkipped struct {
    *BaseEvent
    DownloadID      int64
    ContentID       int64
    ReleaseQuality  string
    ExistingQuality string
    Reason          string
}
```

## Quality Comparison Helper

```go
// internal/handlers/quality.go

func resolutionRank(quality string) int {
    switch strings.ToLower(quality) {
    case "2160p", "4k", "uhd":
        return 4
    case "1080p", "fhd":
        return 3
    case "720p", "hd":
        return 2
    case "480p", "sd":
        return 1
    default:
        return 0
    }
}

func isBetterQuality(newQuality, existingQuality string) bool {
    return resolutionRank(newQuality) > resolutionRank(existingQuality)
}
```

## DownloadHandler Changes

Add library store dependency to check existing files before grabbing:

```go
type DownloadHandler struct {
    *BaseHandler
    store   *download.Store
    library *library.Store  // NEW
    client  download.Downloader
}

func (h *DownloadHandler) handleGrabRequested(ctx context.Context, e *events.GrabRequested) {
    // Check for existing files before grabbing
    if e.ContentID > 0 {
        files, _, _ := h.library.ListFiles(library.FileFilter{ContentID: &e.ContentID})
        if len(files) > 0 {
            parsed := release.Parse(e.ReleaseName)
            newQuality := parsed.Resolution
            bestExisting := getBestQuality(files)

            if !isBetterQuality(newQuality, bestExisting) {
                h.Logger().Warn("skipping grab, existing quality equal or better",
                    "content_id", e.ContentID,
                    "new", newQuality,
                    "existing", bestExisting)
                h.Bus().Publish(ctx, &events.GrabSkipped{...})
                return
            }
        }
    }
    // Proceed with grab...
}
```

## ImportHandler Changes

Add library store dependency to check existing files before importing:

```go
type ImportHandler struct {
    *BaseHandler
    store    *download.Store
    library  *library.Store  // NEW
    importer FileImporter
    importing sync.Map
}

func (h *ImportHandler) handleDownloadCompleted(ctx context.Context, e *events.DownloadCompleted) {
    // ... existing setup ...

    // Check for existing files before importing
    if dl.ContentID > 0 {
        files, _, _ := h.library.ListFiles(library.FileFilter{ContentID: &dl.ContentID})
        if len(files) > 0 {
            parsed := release.Parse(dl.ReleaseName)
            newQuality := parsed.Resolution
            bestExisting := getBestQuality(files)

            if !isBetterQuality(newQuality, bestExisting) {
                h.Logger().Warn("skipping import, existing quality equal or better",
                    "download_id", e.DownloadID,
                    "new", newQuality,
                    "existing", bestExisting)
                h.Bus().Publish(ctx, &events.ImportSkipped{...})
                h.store.Transition(dl, download.StatusSkipped)
                return
            }
        }
    }
    // Proceed with import...
}
```

## New Download Status

```go
const StatusSkipped Status = "skipped"
```

Add to state machine: `completed -> skipped`

## CleanupHandler Changes

Subscribe to `ImportSkipped` to clean up source files when imports are skipped:

```go
func (h *CleanupHandler) Start(ctx context.Context) error {
    plexDetected := h.Bus().Subscribe(events.EventPlexItemDetected, 100)
    importSkipped := h.Bus().Subscribe(events.EventImportSkipped, 100)  // NEW
    // ...
}
```

## Wiring (runner.go)

Pass library store to handlers:

```go
downloadHandler := handlers.NewDownloadHandler(bus, downloadStore, sabClient, libraryStore, logger)
importHandler := handlers.NewImportHandler(bus, downloadStore, libraryStore, imp, logger)
```

## Flow Diagram

```
GrabRequested
    │
    ▼
DownloadHandler checks existing files
    ├─ No files or upgrade → proceed with grab
    └─ Same/worse quality → emit GrabSkipped, return

DownloadCompleted
    │
    ▼
ImportHandler checks existing files
    ├─ No files or upgrade → proceed with import
    └─ Same/worse quality → emit ImportSkipped, mark skipped

ImportSkipped
    │
    ▼
CleanupHandler deletes source files
```

## Implementation Tasks

1. Add `GrabSkipped`, `ImportSkipped` events to `internal/events/events.go`
2. Add `StatusSkipped` to `internal/download/download.go`
3. Create `internal/handlers/quality.go` with comparison helpers
4. Modify `DownloadHandler` - add library store, implement check
5. Modify `ImportHandler` - add library store, implement check
6. Modify `CleanupHandler` - subscribe to `ImportSkipped`
7. Update `internal/server/runner.go` wiring
8. Add tests for quality comparison and skip flows
