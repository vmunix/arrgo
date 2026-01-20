# Download State Machine & Source Cleanup Design

> **Status:** ✅ COMPLETED (2026-01-19) - Merged to main

## Overview

Add a lightweight state machine to track download lifecycle, enable source cleanup after successful import, and provide infrastructure for stuck detection.

## Goals

1. **Explicit state transitions** - Prevent invalid state changes, catch bugs early
2. **Source cleanup** - Delete downloaded files after successful import to Plex
3. **Stuck detection** - Identify downloads that haven't progressed as expected
4. **Audit trail** - Track when each state change occurred

## State Machine

### States

```
queued ──────► downloading ──────► completed ──────► imported ──────► cleaned
   │               │                   │                │
   └───────────────┴───────────────────┴────────────────┴──────────► failed
```

| State | Description |
|-------|-------------|
| `queued` | Sent to download client, waiting to start |
| `downloading` | Actively downloading |
| `completed` | Download finished, ready for import |
| `imported` | File copied to library, DB updated, Plex notified |
| `cleaned` | Verified in Plex, source deleted |
| `failed` | Error occurred (can retry → queued) |

### Valid Transitions

```go
var validTransitions = map[Status][]Status{
    StatusQueued:      {StatusDownloading, StatusFailed},
    StatusDownloading: {StatusCompleted, StatusFailed},
    StatusCompleted:   {StatusImported, StatusFailed},
    StatusImported:    {StatusCleaned, StatusFailed},
    StatusCleaned:     {},                              // terminal
    StatusFailed:      {StatusQueued},                  // retry
}
```

### Terminal States

- `cleaned` - Success, nothing more to do
- `failed` - Error, requires manual intervention or retry

## Database Changes

### New Column

```sql
ALTER TABLE downloads ADD COLUMN last_transition_at TIMESTAMP;
UPDATE downloads SET last_transition_at = added_at WHERE last_transition_at IS NULL;
```

### New Status Value

Add `cleaned` to the status CHECK constraint.

## Configuration

```toml
[importer]
cleanup_source = true  # Delete source files after successful import (default: true)
```

## Implementation

### State Validation

New file `internal/download/status.go`:

```go
func (s Status) CanTransitionTo(to Status) bool {
    valid, ok := validTransitions[s]
    if !ok {
        return false
    }
    for _, v := range valid {
        if v == to {
            return true
        }
    }
    return false
}

func (s Status) IsTerminal() bool {
    return s == StatusCleaned || s == StatusFailed
}
```

### Store Changes

`Update()` method:
1. Validate transition is allowed
2. Set `last_transition_at = NOW()`
3. Persist to database

New method:
```go
func (s *Store) ListStuck(thresholds map[Status]time.Duration) ([]*Download, error)
```

### Cleanup Flow (in poller)

```
For each download with status = 'completed':
    1. Import file (copy, rename, update DB)
    2. Notify Plex
    3. Transition to 'imported'

For each download with status = 'imported':
    1. If cleanup disabled: transition to 'cleaned' immediately
    2. Search Plex for the title
    3. If found:
        a. Validate source path is under download root (safety)
        b. Delete source directory
        c. Transition to 'cleaned'
    4. If not found:
        a. Log debug message
        b. Will retry on next poller cycle
```

### Safety Checks Before Delete

1. Path must start with configured download directory
2. Import must have succeeded (file exists at destination)
3. Plex must have indexed the file

## Stuck Detection

### Thresholds

| State | Stuck After |
|-------|-------------|
| queued | 1 hour |
| downloading | 24 hours |
| completed | 1 hour |
| imported | 24 hours |

### Query

```sql
SELECT * FROM downloads
WHERE (status = 'queued' AND last_transition_at < datetime('now', '-1 hour'))
   OR (status = 'downloading' AND last_transition_at < datetime('now', '-24 hours'))
   OR (status = 'completed' AND last_transition_at < datetime('now', '-1 hour'))
   OR (status = 'imported' AND last_transition_at < datetime('now', '-24 hours'))
```

### Future Use

- CLI: `arrgo downloads --stuck`
- Startup warning if stuck downloads exist
- Alerting integration

## Testing

### Unit Tests

- Valid transitions succeed
- Invalid transitions return error
- `IsTerminal()` returns correct values
- `ListStuck()` returns expected results

### Integration Tests

- Full flow: queued → downloading → completed → imported → cleaned
- Cleanup only happens after Plex verification
- Failed cleanup doesn't block state machine
- Retry from failed state works

## Migration Path

1. Add `last_transition_at` column with default = `added_at`
2. Add `cleaned` status to CHECK constraint
3. Existing `imported` downloads will transition to `cleaned` on next poller cycle (after Plex verification)
