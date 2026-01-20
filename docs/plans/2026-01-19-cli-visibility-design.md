# CLI Visibility Design

> **Status:** ✅ COMPLETED (2026-01-19) - Merged in PR #22

> **For Claude:** Use superpowers:writing-plans to create implementation plan from this design.

**Goal:** Give users clear visibility into the download pipeline state, with actionable diagnostics when things don't match reality.

**Approach:** Dashboard for quick health check, drill-down commands for details, verify command to detect drift between state machine and live systems.

---

## Command Structure

```
arrgo status          # Dashboard overview
arrgo queue           # Download details (enhanced)
arrgo imports         # Import pipeline + recent successes
arrgo verify [id]     # Reality check (all, or specific download)
```

Hierarchy: `status` is the entry point (quick health check), then drill down into `queue`/`imports`/`verify` for details.

---

## Commands

### `arrgo status` (Dashboard)

Quick health check showing summary stats plus anything needing attention.

```
$ arrgo status

arrgo v0.1.0 | Server: localhost:8484 | Plex: connected | SABnzbd: connected

Downloads
  Queued:       0
  Downloading:  2  (Back to the Future 45%, Inception 12%)
  Completed:    1  (awaiting import)

Imports
  Pending:      3  (awaiting Plex verification)
  Stuck:        1  (>1hr without progress)

Library
  Movies:     142 tracked
  Series:      23 tracked

Problems: 1 detected
  → Run 'arrgo verify' for details
```

**Principles:**
- Connection health up front (server, Plex, SABnzbd)
- Numbers at a glance for each pipeline stage
- Inline preview of active downloads (name + progress)
- Calls out "stuck" items (using `last_transition_at` threshold)
- Nudges toward `verify` if problems detected

**Flags:**
- `--json` - Structured output for scripting

---

### `arrgo queue` (Enhanced)

Currently shows basic list. Enhanced version adds state machine visibility and progress.

```
$ arrgo queue

Active Downloads (3):

  ID  STATE        RELEASE                                      PROGRESS  ETA
   5  downloading  Back.to.the.Future.1985.1080p.BluRay         45%       3m
   6  downloading  Inception.2010.1080p.WEB-DL                  12%       8m
   7  completed    The.Matrix.1999.2160p.UHD.BluRay             100%      -

$ arrgo queue --all

All Downloads (50):

  ID  STATE      RELEASE                                      COMPLETED      CLEANED
   1  cleaned    Alien.1979.1080p.BluRay                      2h ago         2h ago
   2  cleaned    Blade.Runner.1982.1080p.BluRay               1d ago         1d ago
   ...
   7  completed  The.Matrix.1999.2160p.UHD.BluRay             5m ago         -
```

**Flags:**
- `--all` - Include terminal states (cleaned, failed)
- `--state=X` - Filter by state (queued, downloading, completed, imported, cleaned, failed)
- `--stuck` - Only show items exceeding time threshold
- `--json` - Structured output

---

### `arrgo imports` (New)

Shows downloads in post-completion pipeline plus recent successes for confidence.

```
$ arrgo imports

Pending (3):

  ID  TITLE                    IMPORTED    PLEX     DEST PATH
   8  The Matrix (1999)        5m ago      waiting  /movies/The Matrix (1999)/
   9  Alien (1979)             12m ago     waiting  /movies/Alien (1979)/
  10  Blade Runner (1982)      1h ago      waiting  /movies/Blade Runner (1982)/

  ⚠ ID 10 has been waiting >1hr - run 'arrgo verify 10' to check

Recent (last 24h):

  ID  TITLE                    IMPORTED    CLEANED
   7  Inception (2010)         2h ago      2h ago    ✓
   6  Back to the Future       4h ago      4h ago    ✓
   5  KPop Demon Hunters       6h ago      6h ago    ✓
```

Top half: what's in flight. Bottom half: proof the pipeline works.

**Flags:**
- `--pending` - Only show pending imports
- `--recent` - Only show recent completions
- `--json` - Structured output

---

### `arrgo verify [id]` (New)

Reality check comparing database state against live systems. Suggests fixes.

```
$ arrgo verify

Checking 12 active downloads...

✓ SABnzbd connection OK
✓ Plex connection OK
✓ 2 downloading - confirmed in SABnzbd queue
✓ 1 completed - source file exists
✓ 3 imported - files in library

Problems (2):

  ID 10 | imported | Blade Runner (1982)
    State: imported (1h ago)
    Issue: Not found in Plex library
    Check: File exists at /movies/Blade Runner (1982)/Blade Runner (1982) [2160p].mkv ✓
    Likely: Plex hasn't scanned yet
    Fix: arrgo plex scan "/movies/Blade Runner (1982)/"
         or wait for automatic scan

  ID 4 | completed | The Godfather (1972)
    State: completed (3h ago)
    Issue: Source file missing from /srv/data/usenet/The.Godfather.../
    Likely: Manually deleted or SABnzbd cleared history
    Fix: arrgo retry 4    (re-grab from indexer)
         arrgo skip 4     (mark as failed, stop retrying)

2 problems found. Run suggested commands or 'arrgo verify --help' for options.
```

**Verification checks by state:**

| State | Checks |
|-------|--------|
| `queued` | Exists in SABnzbd queue |
| `downloading` | Exists in SABnzbd queue, progress updating |
| `completed` | Source file exists at expected path |
| `imported` | Destination file exists, Plex has indexed it |
| `cleaned` | Source directory removed |
| `failed` | (informational only) |

**Each problem shows:**
- Current state and how long it's been there
- What's wrong (the drift)
- What we checked to confirm
- Likely cause
- Command(s) to fix

**Flags:**
- `--json` - Structured output
- Positional `[id]` - Check specific download only

---

## Future: Control Commands

These commands make `verify` suggestions actionable (implement in phase 2):

```
arrgo retry <id>      # Re-grab from indexer (for failed/missing downloads)
arrgo skip <id>       # Mark as failed, stop processing
arrgo repair <id>     # Attempt to fix state (e.g., re-trigger Plex scan)
arrgo cleanup <id>    # Force cleanup without Plex verification
```

---

## Design Principles

1. **Dashboard first** - `status` is the entry point, always tells you if something needs attention
2. **Drill down** - Specific commands for specific concerns
3. **Show the happy path** - Recent successes build confidence the system works
4. **Actionable diagnostics** - Every problem includes suggested fix commands
5. **State machine is truth** - `verify` compares against live systems, not the other way around
6. **Read-only for v1** - Verify reports and suggests, doesn't auto-fix (control commands come later)

---

## Implementation Notes

- `status` needs aggregate queries: count by state, stuck detection via `last_transition_at`
- `queue` enhancement: add progress/ETA from live SABnzbd status
- `imports` needs join: downloads + content for title, plus Plex check
- `verify` reuses same validation logic as unit tests, but against live clients
- All commands support `--json` for scripting
