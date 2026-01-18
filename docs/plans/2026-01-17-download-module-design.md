# Download Module Design

**Date:** 2026-01-17
**Status:** Approved

## Overview

The download module sends releases to download clients (SABnzbd), tracks downloads in the database, and provides status updates. It handles the lifecycle from grab to completion, preparing downloads for the import module.

## Architecture

Three components:

**`SABnzbdClient`** - HTTP client for SABnzbd API
- Implements `Downloader` interface (Add, Status, List, Remove)
- Thin wrapper around SABnzbd API
- Maps SABnzbd states to coarse status enum

**`Store`** - Database persistence for downloads table
- CRUD operations: Add, Get, Update, List, Delete
- Query by content ID, status, client ID
- Idempotent Add using (content_id, release_name) as natural dedup key

**`Manager`** - Orchestration layer
- `Grab()` - sends to SABnzbd + records in DB
- `Refresh()` - polls SABnzbd for status updates, syncs to DB
- `Cancel()` - removes from SABnzbd + updates DB
- `GetActive()` - returns in-progress downloads with live status

### Data Flow

Grab:
```
Manager.Grab() → SABnzbdClient.Add() → get nzo_id
              → Store.Add() → record in DB
              → return Download
```

Status refresh:
```
Manager.Refresh() → Store.ListActive() → get tracked downloads
                 → SABnzbdClient.Status() for each
                 → Store.UpdateStatus() if changed
```

## SABnzbd Client

### API Endpoints

SABnzbd uses a single endpoint with `mode` parameter:
- `GET /api?mode=addurl&name=<url>&cat=<category>` - Add NZB by URL
- `GET /api?mode=queue` - List queue (downloading)
- `GET /api?mode=history` - List history (completed/failed)
- `GET /api?mode=queue&name=delete&value=<nzo_id>` - Remove from queue

All requests include `apikey=<key>&output=json`.

### Status Mapping

| SABnzbd State | Our Status |
|---------------|------------|
| Queued, Paused, Downloading, Fetching | `downloading` |
| Extracting, Verifying, Repairing, Moving | `downloading` |
| Completed | `completed` |
| Failed | `failed` |

We check both queue (active) and history (finished) to find a download.

### Client Struct

```go
type SABnzbdClient struct {
    baseURL    string
    apiKey     string
    category   string
    httpClient *http.Client
}

func NewSABnzbdClient(baseURL, apiKey, category string) *SABnzbdClient
```

## Store (Database Layer)

### Operations

```go
type Store struct {
    db *sql.DB
}

func NewStore(db *sql.DB) *Store

// Add inserts a new download. Idempotent - returns existing if (content_id, release_name) match.
func (s *Store) Add(d *Download) error

// Get retrieves a download by ID.
func (s *Store) Get(id int64) (*Download, error)

// GetByClientID finds a download by client type and client's ID.
func (s *Store) GetByClientID(client Client, clientID string) (*Download, error)

// Update updates a download record (status, completed_at).
func (s *Store) Update(d *Download) error

// List returns downloads matching the filter.
func (s *Store) List(f DownloadFilter) ([]*Download, error)

// Delete removes a download record.
func (s *Store) Delete(id int64) error
```

### DownloadFilter

```go
type DownloadFilter struct {
    ContentID *int64
    EpisodeID *int64
    Status    *Status
    Client    *Client
    Active    bool  // If true, exclude "imported" status
}
```

### Idempotent Add

```go
func (s *Store) Add(d *Download) error {
    // Check for existing by (content_id, release_name)
    existing := s.findByContentAndRelease(d.ContentID, d.ReleaseName)
    if existing != nil {
        d.ID = existing.ID  // Return existing ID
        return nil
    }
    // Insert new record
}
```

## Manager (Orchestration)

### Struct

```go
type Manager struct {
    client Downloader
    store  *Store
}

func NewManager(client Downloader, store *Store) *Manager
```

### Grab

```go
func (m *Manager) Grab(ctx context.Context, contentID int64, episodeID *int64,
    downloadURL, releaseName, indexer string) (*Download, error)
```

Flow:
1. Call `client.Add(ctx, downloadURL, category)` → get clientID
2. Create `Download` struct with status=queued
3. Call `store.Add(d)` → idempotent insert
4. Return the Download

If client.Add fails, return error (nothing in DB).
If store.Add fails, orphan in SABnzbd is acceptable - next Refresh will find it.

### Refresh

```go
func (m *Manager) Refresh(ctx context.Context) error
```

Flow:
1. `store.List(DownloadFilter{Active: true})` → tracked downloads
2. For each, `client.Status(ctx, clientID)`
3. If status changed, `store.Update(d)`
4. Return any errors (partial refresh is OK)

### Cancel

```go
func (m *Manager) Cancel(ctx context.Context, downloadID int64, deleteFiles bool) error
```

Flow:
1. `store.Get(downloadID)` → get download
2. `client.Remove(ctx, clientID, deleteFiles)`
3. `store.Delete(downloadID)` or update status to failed

### GetActive

```go
func (m *Manager) GetActive(ctx context.Context) ([]*ActiveDownload, error)

type ActiveDownload struct {
    Download *Download
    Live     *ClientStatus  // Progress, speed, ETA from client
}
```

## Error Handling

```go
var (
    ErrClientUnavailable = errors.New("download client unavailable")
    ErrInvalidAPIKey     = errors.New("invalid api key")
    ErrDownloadNotFound  = errors.New("download not found in client")
    ErrNotFound          = errors.New("download not found")  // DB not found
)
```

`ErrDownloadNotFound` is from client (SABnzbd doesn't have it).
`ErrNotFound` is from Store (DB doesn't have it).

## Testing Strategy

### SABnzbdClient
- Use `httptest.Server` to mock SABnzbd API responses
- Test Add, Status, List, Remove with various responses
- Test error cases: 401 (invalid key), connection refused, malformed JSON

### Store
- Use in-memory SQLite like library module
- Test CRUD operations
- Test idempotent Add behavior
- Test filter queries

### Manager
- Mock both `Downloader` interface and `Store`
- Test Grab flow: success, client error, store error
- Test Refresh flow: status transitions
- Test Cancel flow

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scope | SABnzbd + Store + Manager | Self-contained, no background poller |
| Grab flow | Idempotent with dedup | SABnzbd dedups internally, DB uses natural key |
| Status granularity | Coarse mapping | Import module only needs "is it done?" |
| Error handling | Typed errors | Consistent with search module |
| Store transactions | None | Single-row operations, keep simple |
| Manager.Grab params | Minimal | Explicit fields, no coupling to search module |

## Out of Scope (v2+)

- qBittorrent support (interface is ready, just needs implementation)
- Background polling goroutine
- Download priority/ordering
- Bandwidth scheduling
- Multiple SABnzbd instances
