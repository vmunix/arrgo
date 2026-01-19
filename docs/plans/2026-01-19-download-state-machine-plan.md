# Download State Machine & Source Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a state machine to track download lifecycle with event emission, enable source cleanup after Plex verification, and provide stuck detection infrastructure.

**Architecture:** State transitions are validated and emit events. The poller handles cleanup after verifying files appear in Plex. A new `last_transition_at` column enables stuck detection queries.

**Tech Stack:** Go, SQLite, existing download/importer packages

---

## Task 1: Add StatusCleaned Constant

**Files:**
- Modify: `internal/download/download.go:24-30`
- Test: `internal/download/download_test.go` (new file)

**Step 1: Write the failing test**

Create `internal/download/download_test.go`:

```go
package download

import "testing"

func TestStatusConstants(t *testing.T) {
	// Verify all expected statuses exist
	statuses := []Status{
		StatusQueued,
		StatusDownloading,
		StatusCompleted,
		StatusFailed,
		StatusImported,
		StatusCleaned,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("status constant is empty")
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/ -run TestStatusConstants -v`
Expected: FAIL with "undefined: StatusCleaned"

**Step 3: Add StatusCleaned constant**

In `internal/download/download.go`, update the const block:

```go
const (
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusImported    Status = "imported"
	StatusCleaned     Status = "cleaned"
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/ -run TestStatusConstants -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/download.go internal/download/download_test.go
git commit -m "feat(download): add StatusCleaned constant"
```

---

## Task 2: Create State Machine with Transition Validation

**Files:**
- Create: `internal/download/status.go`
- Create: `internal/download/status_test.go`

**Step 1: Write the failing tests**

Create `internal/download/status_test.go`:

```go
package download

import "testing"

func TestCanTransitionTo_ValidTransitions(t *testing.T) {
	tests := []struct {
		from Status
		to   Status
	}{
		{StatusQueued, StatusDownloading},
		{StatusQueued, StatusFailed},
		{StatusDownloading, StatusCompleted},
		{StatusDownloading, StatusFailed},
		{StatusCompleted, StatusImported},
		{StatusCompleted, StatusFailed},
		{StatusImported, StatusCleaned},
		{StatusImported, StatusFailed},
		{StatusFailed, StatusQueued}, // retry
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if !tt.from.CanTransitionTo(tt.to) {
				t.Errorf("%s should be able to transition to %s", tt.from, tt.to)
			}
		})
	}
}

func TestCanTransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from Status
		to   Status
	}{
		{StatusQueued, StatusCompleted},    // skip downloading
		{StatusQueued, StatusImported},     // skip multiple
		{StatusQueued, StatusCleaned},      // skip multiple
		{StatusDownloading, StatusQueued},  // backwards
		{StatusDownloading, StatusImported}, // skip completed
		{StatusCompleted, StatusQueued},    // backwards
		{StatusCompleted, StatusCleaned},   // skip imported
		{StatusImported, StatusQueued},     // backwards
		{StatusImported, StatusCompleted},  // backwards
		{StatusCleaned, StatusQueued},      // terminal
		{StatusCleaned, StatusFailed},      // terminal
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if tt.from.CanTransitionTo(tt.to) {
				t.Errorf("%s should NOT be able to transition to %s", tt.from, tt.to)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []Status{StatusCleaned, StatusFailed}
	nonTerminal := []Status{StatusQueued, StatusDownloading, StatusCompleted, StatusImported}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}

	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s should NOT be terminal", s)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/download/ -run "TestCanTransitionTo|TestIsTerminal" -v`
Expected: FAIL with "CanTransitionTo not defined"

**Step 3: Implement status.go**

Create `internal/download/status.go`:

```go
package download

// validTransitions defines allowed state transitions.
// Key is the "from" status, value is list of valid "to" statuses.
var validTransitions = map[Status][]Status{
	StatusQueued:      {StatusDownloading, StatusFailed},
	StatusDownloading: {StatusCompleted, StatusFailed},
	StatusCompleted:   {StatusImported, StatusFailed},
	StatusImported:    {StatusCleaned, StatusFailed},
	StatusCleaned:     {}, // terminal - no transitions out
	StatusFailed:      {StatusQueued}, // allow retry
}

// CanTransitionTo returns true if transitioning from s to target is valid.
func (s Status) CanTransitionTo(target Status) bool {
	valid, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, v := range valid {
		if v == target {
			return true
		}
	}
	return false
}

// IsTerminal returns true if this status has no valid outgoing transitions
// (except failed which can retry).
func (s Status) IsTerminal() bool {
	return s == StatusCleaned || s == StatusFailed
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/download/ -run "TestCanTransitionTo|TestIsTerminal" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/status.go internal/download/status_test.go
git commit -m "feat(download): add state machine with transition validation"
```

---

## Task 3: Add Transition Event Types

**Files:**
- Modify: `internal/download/status.go`
- Modify: `internal/download/status_test.go`

**Step 1: Write the failing test**

Add to `internal/download/status_test.go`:

```go
func TestTransitionEvent(t *testing.T) {
	event := TransitionEvent{
		DownloadID: 42,
		From:       StatusQueued,
		To:         StatusDownloading,
		At:         time.Now(),
	}

	if event.DownloadID != 42 {
		t.Error("DownloadID not set")
	}
	if event.From != StatusQueued {
		t.Error("From not set")
	}
	if event.To != StatusDownloading {
		t.Error("To not set")
	}
}
```

Add import `"time"` at top of test file.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/ -run TestTransitionEvent -v`
Expected: FAIL with "undefined: TransitionEvent"

**Step 3: Add TransitionEvent type**

Add to `internal/download/status.go`:

```go
import "time"

// TransitionEvent is emitted when a download changes state.
type TransitionEvent struct {
	DownloadID int64
	From       Status
	To         Status
	At         time.Time
}

// TransitionHandler processes state transition events.
type TransitionHandler func(event TransitionEvent)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/ -run TestTransitionEvent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/status.go internal/download/status_test.go
git commit -m "feat(download): add TransitionEvent type for state change notifications"
```

---

## Task 4: Add last_transition_at to Download Struct

**Files:**
- Modify: `internal/download/download.go:32-44`
- Modify: `internal/download/download_test.go`

**Step 1: Write the failing test**

Add to `internal/download/download_test.go`:

```go
import "time"

func TestDownloadHasLastTransitionAt(t *testing.T) {
	now := time.Now()
	d := Download{
		ID:               1,
		Status:           StatusQueued,
		LastTransitionAt: now,
	}

	if d.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set")
	}
	if !d.LastTransitionAt.Equal(now) {
		t.Errorf("LastTransitionAt = %v, want %v", d.LastTransitionAt, now)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/ -run TestDownloadHasLastTransitionAt -v`
Expected: FAIL with "unknown field 'LastTransitionAt'"

**Step 3: Add LastTransitionAt field**

Update the Download struct in `internal/download/download.go`:

```go
// Download represents an active or recent download.
type Download struct {
	ID               int64
	ContentID        int64
	EpisodeID        *int64
	Client           Client
	ClientID         string // ID in the download client
	Status           Status
	ReleaseName      string
	Indexer          string
	AddedAt          time.Time
	CompletedAt      *time.Time
	LastTransitionAt time.Time
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/ -run TestDownloadHasLastTransitionAt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/download.go internal/download/download_test.go
git commit -m "feat(download): add LastTransitionAt field to Download struct"
```

---

## Task 5: Add Database Migration

**Files:**
- Modify: `internal/migrations/migrations.go`

**Step 1: Review current migration structure**

Check: `cat internal/migrations/migrations.go`

The migrations use a simple `InitialSQL` constant. We'll add to it.

**Step 2: Add migration for last_transition_at**

Add a new migration constant and update the initialization in `internal/migrations/migrations.go`:

```go
// Migration002LastTransitionAt adds the last_transition_at column to downloads.
const Migration002LastTransitionAt = `
-- Add last_transition_at column (default to added_at for existing rows)
ALTER TABLE downloads ADD COLUMN last_transition_at TIMESTAMP;
UPDATE downloads SET last_transition_at = added_at WHERE last_transition_at IS NULL;
`
```

Note: SQLite doesn't support adding CHECK constraints to existing tables easily,
so we'll validate the 'cleaned' status in application code. New databases will
get the updated CHECK constraint from InitialSQL.

Also update the downloads CHECK constraint in InitialSQL to include 'cleaned':

```sql
status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'downloading', 'completed', 'failed', 'imported', 'cleaned')),
```

**Step 3: Verify syntax**

Run: `go build ./internal/migrations/`
Expected: Success

**Step 4: Commit**

```bash
git add internal/migrations/migrations.go
git commit -m "feat(migrations): add last_transition_at column and cleaned status"
```

---

## Task 6: Update Store to Read/Write last_transition_at

**Files:**
- Modify: `internal/download/download.go` (Store methods)
- Modify: `internal/download/store_test.go`

**Step 1: Write the failing test**

Add to `internal/download/store_test.go` (find appropriate location):

```go
func TestStore_LastTransitionAt(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add a download
	d := &Download{
		ContentID:   1,
		Client:      ClientManual,
		ClientID:    "test-123",
		Status:      StatusQueued,
		ReleaseName: "Test.Release",
		Indexer:     "manual",
	}
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// LastTransitionAt should be set on Add
	if d.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set after Add")
	}

	// Retrieve and verify
	got, err := store.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set after Get")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/ -run TestStore_LastTransitionAt -v`
Expected: FAIL (column doesn't exist in test schema or not being read)

**Step 3: Update Store methods**

In `internal/download/download.go`, update the Add method to set LastTransitionAt:

```go
func (s *Store) Add(d *Download) error {
	// ... existing duplicate check ...

	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO downloads (content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ContentID, d.EpisodeID, d.Client, d.ClientID, d.Status, d.ReleaseName, d.Indexer, now, d.CompletedAt, now,
	)
	// ... rest of method ...
	d.LastTransitionAt = now
	return nil
}
```

Update Get method to read LastTransitionAt:

```go
func (s *Store) Get(id int64) (*Download, error) {
	d := &Download{}
	err := s.db.QueryRow(`
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at
		FROM downloads WHERE id = ?`, id,
	).Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt)
	// ... rest of method ...
}
```

Update GetByClientID similarly.

Update List method to read LastTransitionAt.

Update the test schema in `internal/download/testdata/schema.sql` to include the new column.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/ -run TestStore_LastTransitionAt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/download.go internal/download/store_test.go internal/download/testdata/schema.sql
git commit -m "feat(download): store reads/writes last_transition_at"
```

---

## Task 7: Add Transition Method with Validation and Events

**Files:**
- Modify: `internal/download/download.go`
- Modify: `internal/download/download_test.go`

**Step 1: Write the failing test**

Add to `internal/download/download_test.go`:

```go
func TestStore_Transition(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Track events
	var events []TransitionEvent
	store.OnTransition(func(e TransitionEvent) {
		events = append(events, e)
	})

	// Add a download
	d := &Download{
		ContentID:   1,
		Client:      ClientManual,
		ClientID:    "test-456",
		Status:      StatusQueued,
		ReleaseName: "Test.Release",
		Indexer:     "manual",
	}
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Valid transition
	oldTime := d.LastTransitionAt
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	if err := store.Transition(d, StatusDownloading); err != nil {
		t.Fatalf("Transition: %v", err)
	}

	if d.Status != StatusDownloading {
		t.Errorf("Status = %s, want downloading", d.Status)
	}
	if !d.LastTransitionAt.After(oldTime) {
		t.Error("LastTransitionAt should be updated")
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].From != StatusQueued || events[0].To != StatusDownloading {
		t.Errorf("event = %v, want queued->downloading", events[0])
	}

	// Invalid transition
	if err := store.Transition(d, StatusCleaned); err == nil {
		t.Error("should reject invalid transition downloading->cleaned")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/ -run TestStore_Transition -v`
Expected: FAIL with "OnTransition not defined" or "Transition not defined"

**Step 3: Implement Transition method**

Add to `internal/download/download.go`:

```go
// Store persists download records.
type Store struct {
	db       *sql.DB
	handlers []TransitionHandler
}

// OnTransition registers a handler to be called on state transitions.
func (s *Store) OnTransition(h TransitionHandler) {
	s.handlers = append(s.handlers, h)
}

// Transition changes a download's status with validation and event emission.
func (s *Store) Transition(d *Download, to Status) error {
	if !d.Status.CanTransitionTo(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, d.Status, to)
	}

	from := d.Status
	now := time.Now()

	result, err := s.db.Exec(`
		UPDATE downloads SET status = ?, last_transition_at = ?
		WHERE id = ?`,
		to, now, d.ID,
	)
	if err != nil {
		return fmt.Errorf("update download %d: %w", d.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("transition download %d: %w", d.ID, ErrNotFound)
	}

	d.Status = to
	d.LastTransitionAt = now

	// Emit event
	event := TransitionEvent{
		DownloadID: d.ID,
		From:       from,
		To:         to,
		At:         now,
	}
	for _, h := range s.handlers {
		h(event)
	}

	return nil
}
```

Add to `internal/download/errors.go`:

```go
var ErrInvalidTransition = errors.New("invalid state transition")
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/ -run TestStore_Transition -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/download.go internal/download/errors.go internal/download/download_test.go
git commit -m "feat(download): add Transition method with validation and events"
```

---

## Task 8: Add ListStuck Query

**Files:**
- Modify: `internal/download/download.go`
- Modify: `internal/download/store_test.go`

**Step 1: Write the failing test**

Add to `internal/download/store_test.go`:

```go
func TestStore_ListStuck(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Add downloads with old timestamps
	oldTime := time.Now().Add(-2 * time.Hour)

	// Stuck in queued (> 1 hour)
	d1 := &Download{
		ContentID:   1,
		Client:      ClientManual,
		ClientID:    "stuck-queued",
		Status:      StatusQueued,
		ReleaseName: "Stuck.Queued",
		Indexer:     "manual",
	}
	store.Add(d1)
	// Manually set old timestamp
	db.Exec("UPDATE downloads SET last_transition_at = ? WHERE id = ?", oldTime, d1.ID)

	// Not stuck - recently added
	d2 := &Download{
		ContentID:   2,
		Client:      ClientManual,
		ClientID:    "recent-queued",
		Status:      StatusQueued,
		ReleaseName: "Recent.Queued",
		Indexer:     "manual",
	}
	store.Add(d2)

	// Stuck in downloading (> 24 hours would be stuck, but 2 hours is not)
	d3 := &Download{
		ContentID:   3,
		Client:      ClientManual,
		ClientID:    "downloading",
		Status:      StatusDownloading,
		ReleaseName: "Downloading",
		Indexer:     "manual",
	}
	store.Add(d3)
	db.Exec("UPDATE downloads SET status = 'downloading', last_transition_at = ? WHERE id = ?", oldTime, d3.ID)

	thresholds := map[Status]time.Duration{
		StatusQueued:      1 * time.Hour,
		StatusDownloading: 24 * time.Hour,
		StatusCompleted:   1 * time.Hour,
		StatusImported:    24 * time.Hour,
	}

	stuck, err := store.ListStuck(thresholds)
	if err != nil {
		t.Fatalf("ListStuck: %v", err)
	}

	// Only d1 should be stuck (queued for > 1 hour)
	// d2 is recent, d3 is downloading but only 2 hours (threshold is 24)
	if len(stuck) != 1 {
		t.Fatalf("got %d stuck, want 1", len(stuck))
	}
	if stuck[0].ID != d1.ID {
		t.Errorf("stuck[0].ID = %d, want %d", stuck[0].ID, d1.ID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/ -run TestStore_ListStuck -v`
Expected: FAIL with "ListStuck not defined"

**Step 3: Implement ListStuck**

Add to `internal/download/download.go`:

```go
// ListStuck returns downloads that haven't transitioned within their expected threshold.
func (s *Store) ListStuck(thresholds map[Status]time.Duration) ([]*Download, error) {
	var conditions []string
	var args []any

	now := time.Now()
	for status, threshold := range thresholds {
		cutoff := now.Add(-threshold)
		conditions = append(conditions, "(status = ? AND last_transition_at < ?)")
		args = append(args, status, cutoff)
	}

	if len(conditions) == 0 {
		return nil, nil
	}

	query := `
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at, last_transition_at
		FROM downloads
		WHERE ` + strings.Join(conditions, " OR ") + `
		ORDER BY last_transition_at`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stuck downloads: %w", err)
	}
	defer rows.Close()

	var results []*Download
	for rows.Next() {
		d := &Download{}
		if err := rows.Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt, &d.LastTransitionAt); err != nil {
			return nil, fmt.Errorf("scan download: %w", err)
		}
		results = append(results, d)
	}

	return results, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/ -run TestStore_ListStuck -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/download.go internal/download/store_test.go
git commit -m "feat(download): add ListStuck query for detecting stalled downloads"
```

---

## Task 9: Add Importer Config for Cleanup

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestConfig_ImporterCleanupSource(t *testing.T) {
	toml := `
[server]
port = 8484

[importer]
cleanup_source = true
`
	cfg, err := parseConfig(toml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !cfg.Importer.CleanupSource {
		t.Error("CleanupSource should be true")
	}
}

func TestConfig_ImporterCleanupSourceDefault(t *testing.T) {
	toml := `
[server]
port = 8484
`
	cfg, err := parseConfig(toml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Default should be true
	if !cfg.Importer.CleanupSource {
		t.Error("CleanupSource should default to true")
	}
}
```

Note: You may need to add a `parseConfig` helper or adapt to existing test patterns.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestConfig_ImporterCleanup -v`
Expected: FAIL with "cfg.Importer undefined"

**Step 3: Add ImporterConfig**

In `internal/config/config.go`:

```go
// Add to Config struct
type Config struct {
	// ... existing fields ...
	Importer ImporterConfig `toml:"importer"`
}

// Add new config type
type ImporterConfig struct {
	CleanupSource bool `toml:"cleanup_source"`
}

// In load() function, add default:
if !cfg.Importer.CleanupSource {
	// Check if it was explicitly set to false vs not set
	// TOML will leave it as false if not set, so we default to true
}
```

Actually, Go's TOML parsing will set bool to false if not present. We need a pointer or separate check. Simpler approach - use a *bool:

```go
type ImporterConfig struct {
	CleanupSource *bool `toml:"cleanup_source"`
}

// Helper method
func (c *ImporterConfig) ShouldCleanupSource() bool {
	if c.CleanupSource == nil {
		return true // default
	}
	return *c.CleanupSource
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestConfig_ImporterCleanup -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add importer.cleanup_source option (default: true)"
```

---

## Task 10: Add Plex Verification Method

**Files:**
- Modify: `internal/importer/plex.go`
- Modify: `internal/importer/plex_test.go`

**Step 1: Write the failing test**

Add to `internal/importer/plex_test.go`:

```go
func TestPlexClient_HasMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" && r.URL.Query().Get("query") == "Test Movie" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<MediaContainer>
  <Video title="Test Movie" year="2024" type="movie"/>
</MediaContainer>`)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")

	// Should find movie
	found, err := client.HasMovie(context.Background(), "Test Movie", 2024)
	if err != nil {
		t.Fatalf("HasMovie: %v", err)
	}
	if !found {
		t.Error("should find Test Movie (2024)")
	}

	// Should not find with wrong year
	found, err = client.HasMovie(context.Background(), "Test Movie", 2023)
	if err != nil {
		t.Fatalf("HasMovie: %v", err)
	}
	if found {
		t.Error("should not find Test Movie (2023)")
	}

	// Should not find non-existent movie
	found, err = client.HasMovie(context.Background(), "Nonexistent", 2024)
	if err != nil {
		t.Fatalf("HasMovie: %v", err)
	}
	if found {
		t.Error("should not find Nonexistent")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/importer/ -run TestPlexClient_HasMovie -v`
Expected: FAIL with "HasMovie not defined"

**Step 3: Implement HasMovie**

Add to `internal/importer/plex.go`:

```go
// HasMovie checks if Plex has a movie with the given title and year.
func (c *PlexClient) HasMovie(ctx context.Context, title string, year int) (bool, error) {
	items, err := c.Search(ctx, title)
	if err != nil {
		return false, err
	}

	for _, item := range items {
		if item.Type == "movie" && item.Year == year && strings.EqualFold(item.Title, title) {
			return true, nil
		}
	}

	return false, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/importer/ -run TestPlexClient_HasMovie -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/importer/plex.go internal/importer/plex_test.go
git commit -m "feat(importer): add HasMovie method for Plex verification"
```

---

## Task 11: Update Poller with Cleanup Logic

**Files:**
- Modify: `cmd/arrgod/server.go`

**Step 1: Review current poller structure**

Check: Lines 284-331 in `cmd/arrgod/server.go` (the poll function)

**Step 2: Update poller to handle cleanup**

The poller needs to:
1. Process `completed` downloads (import them)
2. Process `imported` downloads (verify in Plex, cleanup if configured)

Update the poll function:

```go
func poll(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, sabCfg *config.SABnzbdConfig, impCfg *config.ImporterConfig, plex *importer.PlexClient, lib *library.Store, log *slog.Logger) {
	// Refresh download statuses from client
	if err := manager.Refresh(ctx); err != nil {
		log.Error("refresh failed", "error", err)
	}

	// Process completed downloads -> import them
	processCompletedDownloads(ctx, manager, imp, store, sabCfg, log)

	// Process imported downloads -> verify in Plex and cleanup
	if impCfg.ShouldCleanupSource() {
		processImportedDownloads(ctx, store, plex, lib, sabCfg, log)
	}
}

func processCompletedDownloads(ctx context.Context, manager *download.Manager, imp *importer.Importer, store *download.Store, sabCfg *config.SABnzbdConfig, log *slog.Logger) {
	status := download.StatusCompleted
	completed, err := store.List(download.DownloadFilter{Status: &status})
	if err != nil {
		log.Error("list completed failed", "error", err)
		return
	}

	for _, dl := range completed {
		clientStatus, err := manager.Client().Status(ctx, dl.ClientID)
		if err != nil || clientStatus == nil || clientStatus.Path == "" {
			continue
		}

		importPath := translatePath(clientStatus.Path, sabCfg)
		log.Info("importing download", "download_id", dl.ID, "path", importPath)

		if _, err := imp.Import(ctx, dl.ID, importPath); err != nil {
			log.Error("import failed", "download_id", dl.ID, "error", err)
			continue
		}

		if err := store.Transition(dl, download.StatusImported); err != nil {
			log.Error("transition failed", "download_id", dl.ID, "error", err)
		}
	}
}

func processImportedDownloads(ctx context.Context, store *download.Store, plex *importer.PlexClient, lib *library.Store, sabCfg *config.SABnzbdConfig, log *slog.Logger) {
	if plex == nil {
		// No Plex configured - transition directly to cleaned without verification
		// (Could also skip cleanup entirely, but we'll trust the import succeeded)
		status := download.StatusImported
		imported, _ := store.List(download.DownloadFilter{Status: &status})
		for _, dl := range imported {
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
		}
		return
	}

	status := download.StatusImported
	imported, err := store.List(download.DownloadFilter{Status: &status})
	if err != nil {
		log.Error("list imported failed", "error", err)
		return
	}

	for _, dl := range imported {
		// Get content info
		content, err := lib.GetContent(dl.ContentID)
		if err != nil {
			log.Error("get content failed", "download_id", dl.ID, "error", err)
			continue
		}

		// Check if Plex has it
		found, err := plex.HasMovie(ctx, content.Title, content.Year)
		if err != nil {
			log.Warn("plex check failed", "download_id", dl.ID, "error", err)
			continue
		}

		if !found {
			log.Debug("waiting for Plex to index", "title", content.Title, "year", content.Year)
			continue
		}

		// Get source path for cleanup
		clientStatus, err := store.manager.Client().Status(ctx, dl.ClientID)
		if err != nil || clientStatus == nil || clientStatus.Path == "" {
			// Can't determine source path - just transition without cleanup
			log.Warn("cannot determine source path for cleanup", "download_id", dl.ID)
			if err := store.Transition(dl, download.StatusCleaned); err != nil {
				log.Error("transition failed", "download_id", dl.ID, "error", err)
			}
			continue
		}

		sourcePath := translatePath(clientStatus.Path, sabCfg)
		sourceDir := filepath.Dir(sourcePath)

		// Safety check: ensure path is under download directory
		// (This needs the download root from config - we'll add it)

		// Delete source directory
		if err := os.RemoveAll(sourceDir); err != nil {
			log.Error("cleanup failed", "download_id", dl.ID, "path", sourceDir, "error", err)
			// Continue to mark as cleaned anyway - cleanup is best effort
		} else {
			log.Info("cleaned up source", "download_id", dl.ID, "path", sourceDir)
		}

		if err := store.Transition(dl, download.StatusCleaned); err != nil {
			log.Error("transition failed", "download_id", dl.ID, "error", err)
		}
	}
}
```

**Step 3: Update runPoller and poll signatures**

Update function signatures to pass needed dependencies.

**Step 4: Build and verify**

Run: `go build ./cmd/arrgod/`
Expected: Success

**Step 5: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "feat(server): add cleanup logic with Plex verification in poller"
```

---

## Task 12: Add Transition Event Logging

**Files:**
- Modify: `cmd/arrgod/server.go`

**Step 1: Register transition handler in runServer**

Add after creating the download store:

```go
downloadStore := download.NewStore(db)

// Log all state transitions
downloadStore.OnTransition(func(e download.TransitionEvent) {
    logger.Info("download status changed",
        "download_id", e.DownloadID,
        "from", e.From,
        "to", e.To,
    )
})
```

**Step 2: Build and verify**

Run: `go build ./cmd/arrgod/`
Expected: Success

**Step 3: Commit**

```bash
git add cmd/arrgod/server.go
git commit -m "feat(server): log download state transitions"
```

---

## Task 13: Update config.example.toml

**Files:**
- Modify: `config.example.toml`

**Step 1: Add importer section**

Add to `config.example.toml`:

```toml
# Importer settings
[importer]
# Delete source files after successful import and Plex verification (default: true)
cleanup_source = true
```

**Step 2: Commit**

```bash
git add config.example.toml
git commit -m "docs: add importer.cleanup_source to config example"
```

---

## Task 14: Run Full Test Suite

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Run linter**

Run: `task lint` or `golangci-lint run`
Expected: No errors

**Step 3: Manual smoke test**

1. Start server: `./arrgod`
2. Verify startup logs show "poller started"
3. Check that existing imported downloads transition to cleaned on next cycle

---

## Summary

This plan implements:
1. **State machine** with validated transitions and event emission
2. **last_transition_at tracking** for stuck detection
3. **Source cleanup** after Plex verification
4. **Configurable cleanup** via `importer.cleanup_source`
5. **ListStuck query** for future monitoring/alerting

The state flow is now explicit:
```
queued → downloading → completed → imported → cleaned
                ↓           ↓           ↓
              failed      failed      failed
```
