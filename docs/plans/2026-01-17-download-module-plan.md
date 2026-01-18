# Download Module Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement download module with SABnzbd client, database store, and manager orchestration.

**Architecture:** Three components: `SABnzbdClient` for HTTP API calls, `Store` for database persistence with idempotent add, and `Manager` for orchestrating grab/refresh/cancel operations.

**Tech Stack:** Go stdlib (net/http, database/sql, encoding/json), SQLite via go-sqlite3, existing schema from migrations.

---

### Task 1: Error Types

**Files:**
- Create: `internal/download/errors.go`
- Create: `internal/download/errors_test.go`

**Step 1: Write the test**

```go
// internal/download/errors_test.go
package download

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	if errors.Is(ErrClientUnavailable, ErrInvalidAPIKey) {
		t.Error("ErrClientUnavailable should not equal ErrInvalidAPIKey")
	}
	if errors.Is(ErrDownloadNotFound, ErrNotFound) {
		t.Error("ErrDownloadNotFound should not equal ErrNotFound")
	}

	// Verify error messages are non-empty
	errs := []error{ErrClientUnavailable, ErrInvalidAPIKey, ErrDownloadNotFound, ErrNotFound}
	for _, err := range errs {
		if err.Error() == "" {
			t.Errorf("error %v should have a message", err)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/download/... -v -run TestErrors`
Expected: FAIL (undefined errors)

**Step 3: Write implementation**

```go
// internal/download/errors.go
package download

import "errors"

var (
	// ErrClientUnavailable indicates the download client could not be reached.
	ErrClientUnavailable = errors.New("download client unavailable")

	// ErrInvalidAPIKey indicates the API key is invalid.
	ErrInvalidAPIKey = errors.New("invalid api key")

	// ErrDownloadNotFound indicates the download was not found in the client.
	ErrDownloadNotFound = errors.New("download not found in client")

	// ErrNotFound indicates the download was not found in the database.
	ErrNotFound = errors.New("download not found")
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/download/... -v -run TestErrors`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/download/errors.go internal/download/errors_test.go
git commit -m "feat(download): add error types"
```

---

### Task 2: Download Filter

**Files:**
- Modify: `internal/download/download.go`

**Step 1: Add DownloadFilter struct**

Add after the `Download` struct in `download.go`:

```go
// DownloadFilter specifies criteria for listing downloads.
type DownloadFilter struct {
	ContentID *int64
	EpisodeID *int64
	Status    *Status
	Client    *Client
	Active    bool // If true, exclude "imported" status
}
```

**Step 2: Run build to verify it compiles**

Run: `go build ./internal/download/...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/download/download.go
git commit -m "feat(download): add DownloadFilter struct"
```

---

### Task 3: Store Implementation

**Files:**
- Modify: `internal/download/download.go` (replace Store stubs)
- Create: `internal/download/store_test.go`
- Create: `internal/download/testutil_test.go`
- Create: `internal/download/testdata/schema.sql`

**Step 1: Create test utilities**

```go
// internal/download/testutil_test.go
package download

import (
	"database/sql"
	_ "embed"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed testdata/schema.sql
var testSchema string

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db
}

func ptr[T any](v T) *T {
	return &v
}
```

Copy the schema from migrations:

```bash
cp migrations/001_initial.sql internal/download/testdata/schema.sql
```

**Step 2: Write Store tests**

```go
// internal/download/store_test.go
package download

import (
	"errors"
	"testing"
	"time"
)

func TestStore_Add(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// First, create content for foreign key
	_, err := db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)
	if err != nil {
		t.Fatalf("create content: %v", err)
	}

	d := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Test.Movie.2024.1080p.BluRay.x264-GROUP",
		Indexer:     "TestIndexer",
	}

	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if d.ID == 0 {
		t.Error("ID should be set after Add")
	}
	if d.AddedAt.IsZero() {
		t.Error("AddedAt should be set")
	}
}

func TestStore_Add_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	d1 := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Test.Movie.2024.1080p.BluRay.x264-GROUP",
	}
	if err := store.Add(d1); err != nil {
		t.Fatalf("Add first: %v", err)
	}
	firstID := d1.ID

	// Add same content + release_name should return existing
	d2 := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_different",
		Status:      StatusQueued,
		ReleaseName: "Test.Movie.2024.1080p.BluRay.x264-GROUP",
	}
	if err := store.Add(d2); err != nil {
		t.Fatalf("Add second: %v", err)
	}

	if d2.ID != firstID {
		t.Errorf("expected same ID %d, got %d", firstID, d2.ID)
	}
}

func TestStore_Get(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	original := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie.2024.1080p.BluRay.x264-GROUP",
		Indexer:     "TestIndexer",
	}
	if err := store.Add(original); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := store.Get(original.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ClientID != original.ClientID {
		t.Errorf("ClientID = %q, want %q", got.ClientID, original.ClientID)
	}
	if got.Status != original.Status {
		t.Errorf("Status = %q, want %q", got.Status, original.Status)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, err := store.Get(9999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(9999) error = %v, want ErrNotFound", err)
	}
}

func TestStore_GetByClientID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	d := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_findme",
		Status:      StatusQueued,
		ReleaseName: "Test.Movie",
	}
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := store.GetByClientID(ClientSABnzbd, "nzo_findme")
	if err != nil {
		t.Fatalf("GetByClientID: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("ID = %d, want %d", got.ID, d.ID)
	}
}

func TestStore_Update(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	d := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusQueued,
		ReleaseName: "Test.Movie",
	}
	if err := store.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	now := time.Now()
	d.Status = StatusCompleted
	d.CompletedAt = &now

	if err := store.Update(d); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := store.Get(d.ID)
	if got.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestStore_List_Active(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	// Add active download
	d1 := &Download{ContentID: 1, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusDownloading, ReleaseName: "Active"}
	_ = store.Add(d1)

	// Add imported download
	d2 := &Download{ContentID: 1, Client: ClientSABnzbd, ClientID: "nzo_2", Status: StatusImported, ReleaseName: "Imported"}
	_ = store.Add(d2)

	downloads, err := store.List(DownloadFilter{Active: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(downloads) != 1 {
		t.Errorf("expected 1 active download, got %d", len(downloads))
	}
	if downloads[0].Status != StatusDownloading {
		t.Errorf("expected downloading status, got %s", downloads[0].Status)
	}
}

func TestStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	d := &Download{ContentID: 1, Client: ClientSABnzbd, ClientID: "nzo_1", Status: StatusQueued, ReleaseName: "Test"}
	_ = store.Add(d)

	if err := store.Delete(d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(d.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: error = %v, want ErrNotFound", err)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/download/... -v -run "TestStore"`
Expected: FAIL (methods not implemented)

**Step 4: Implement Store methods**

Replace the Store methods in `download.go`:

```go
import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Add inserts a new download. Idempotent - returns existing if (content_id, release_name) match.
func (s *Store) Add(d *Download) error {
	// Check for existing by content_id and release_name
	var existingID int64
	err := s.db.QueryRow(`SELECT id FROM downloads WHERE content_id = ? AND release_name = ?`,
		d.ContentID, d.ReleaseName).Scan(&existingID)
	if err == nil {
		d.ID = existingID
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check existing: %w", err)
	}

	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO downloads (content_id, episode_id, client, client_id, status, release_name, indexer, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ContentID, d.EpisodeID, d.Client, d.ClientID, d.Status, d.ReleaseName, d.Indexer, now)
	if err != nil {
		return fmt.Errorf("insert download: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	d.ID = id
	d.AddedAt = now
	return nil
}

// Get retrieves a download by ID.
func (s *Store) Get(id int64) (*Download, error) {
	d := &Download{}
	err := s.db.QueryRow(`
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at
		FROM downloads WHERE id = ?`, id,
	).Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get download %d: %w", id, err)
	}
	return d, nil
}

// GetByClientID finds a download by client type and client's ID.
func (s *Store) GetByClientID(client Client, clientID string) (*Download, error) {
	d := &Download{}
	err := s.db.QueryRow(`
		SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at
		FROM downloads WHERE client = ? AND client_id = ?`, client, clientID,
	).Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get download by client id: %w", err)
	}
	return d, nil
}

// Update updates a download record.
func (s *Store) Update(d *Download) error {
	result, err := s.db.Exec(`
		UPDATE downloads SET status = ?, completed_at = ? WHERE id = ?`,
		d.Status, d.CompletedAt, d.ID)
	if err != nil {
		return fmt.Errorf("update download %d: %w", d.ID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns downloads matching the filter.
func (s *Store) List(f DownloadFilter) ([]*Download, error) {
	var conditions []string
	var args []any

	if f.ContentID != nil {
		conditions = append(conditions, "content_id = ?")
		args = append(args, *f.ContentID)
	}
	if f.EpisodeID != nil {
		conditions = append(conditions, "episode_id = ?")
		args = append(args, *f.EpisodeID)
	}
	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}
	if f.Client != nil {
		conditions = append(conditions, "client = ?")
		args = append(args, *f.Client)
	}
	if f.Active {
		conditions = append(conditions, "status != ?")
		args = append(args, StatusImported)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := `SELECT id, content_id, episode_id, client, client_id, status, release_name, indexer, added_at, completed_at
		FROM downloads ` + whereClause + ` ORDER BY id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list downloads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*Download
	for rows.Next() {
		d := &Download{}
		if err := rows.Scan(&d.ID, &d.ContentID, &d.EpisodeID, &d.Client, &d.ClientID, &d.Status, &d.ReleaseName, &d.Indexer, &d.AddedAt, &d.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan download: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate downloads: %w", err)
	}
	return results, nil
}

// Delete removes a download record.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM downloads WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete download %d: %w", id, err)
	}
	return nil
}
```

Also remove the old `UpdateStatus` and `ListActive` methods since they're replaced by `Update` and `List`.

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/download/... -v -run "TestStore"`
Expected: PASS

**Step 6: Run linter**

Run: `golangci-lint run ./internal/download/...`
Expected: No issues

**Step 7: Commit**

```bash
git add internal/download/
git commit -m "feat(download): implement Store with idempotent Add and filters"
```

---

### Task 4: SABnzbd Client Implementation

**Files:**
- Create: `internal/download/sabnzbd.go` (move and expand SABnzbdClient)
- Create: `internal/download/sabnzbd_test.go`
- Modify: `internal/download/download.go` (remove SABnzbdClient, keep interface)

**Step 1: Write SABnzbd tests**

```go
// internal/download/sabnzbd_test.go
package download

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSABnzbdClient_Add(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "test-key" {
			json.NewEncoder(w).Encode(map[string]any{"status": false, "error": "API Key Incorrect"})
			return
		}
		if r.URL.Query().Get("mode") != "addurl" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": true,
			"nzo_ids": []string{"SABnzbd_nzo_abc123"},
		})
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "arrgo")
	nzoID, err := client.Add(context.Background(), "http://example.com/test.nzb", "arrgo")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if nzoID != "SABnzbd_nzo_abc123" {
		t.Errorf("nzoID = %q, want SABnzbd_nzo_abc123", nzoID)
	}
}

func TestSABnzbdClient_Add_InvalidKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": false, "error": "API Key Incorrect"})
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "bad-key", "arrgo")
	_, err := client.Add(context.Background(), "http://example.com/test.nzb", "arrgo")
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Errorf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestSABnzbdClient_Add_Unavailable(t *testing.T) {
	client := NewSABnzbdClient("http://localhost:99999", "key", "arrgo")
	_, err := client.Add(context.Background(), "http://example.com/test.nzb", "arrgo")
	if !errors.Is(err, ErrClientUnavailable) {
		t.Errorf("expected ErrClientUnavailable, got %v", err)
	}
}

func TestSABnzbdClient_Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		if mode == "queue" {
			json.NewEncoder(w).Encode(map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{
						{
							"nzo_id":     "SABnzbd_nzo_abc123",
							"filename":   "Test.Movie.2024",
							"status":     "Downloading",
							"percentage": "45",
							"mb":         "5000",
							"mbleft":     "2750",
							"speed":      "10.5 M",
							"timeleft":   "0:04:30",
						},
					},
				},
			})
			return
		}
		if mode == "history" {
			json.NewEncoder(w).Encode(map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "arrgo")
	status, err := client.Status(context.Background(), "SABnzbd_nzo_abc123")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.ID != "SABnzbd_nzo_abc123" {
		t.Errorf("ID = %q, want SABnzbd_nzo_abc123", status.ID)
	}
	if status.Status != StatusDownloading {
		t.Errorf("Status = %q, want downloading", status.Status)
	}
	if status.Progress < 44 || status.Progress > 46 {
		t.Errorf("Progress = %f, want ~45", status.Progress)
	}
}

func TestSABnzbdClient_Status_Completed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		if mode == "queue" {
			json.NewEncoder(w).Encode(map[string]any{
				"queue": map[string]any{"slots": []map[string]any{}},
			})
			return
		}
		if mode == "history" {
			json.NewEncoder(w).Encode(map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{
						{
							"nzo_id":       "SABnzbd_nzo_abc123",
							"name":         "Test.Movie.2024",
							"status":       "Completed",
							"bytes":        5000000000,
							"storage":      "/downloads/complete/Test.Movie.2024",
						},
					},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "arrgo")
	status, err := client.Status(context.Background(), "SABnzbd_nzo_abc123")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", status.Status)
	}
	if status.Path != "/downloads/complete/Test.Movie.2024" {
		t.Errorf("Path = %q, want /downloads/complete/Test.Movie.2024", status.Path)
	}
}

func TestSABnzbdClient_Status_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		if mode == "queue" {
			json.NewEncoder(w).Encode(map[string]any{"queue": map[string]any{"slots": []map[string]any{}}})
			return
		}
		if mode == "history" {
			json.NewEncoder(w).Encode(map[string]any{"history": map[string]any{"slots": []map[string]any{}}})
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "arrgo")
	_, err := client.Status(context.Background(), "nonexistent")
	if !errors.Is(err, ErrDownloadNotFound) {
		t.Errorf("expected ErrDownloadNotFound, got %v", err)
	}
}

func TestSABnzbdClient_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		if mode == "queue" {
			json.NewEncoder(w).Encode(map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{
						{"nzo_id": "nzo_1", "filename": "Movie1", "status": "Downloading", "percentage": "50", "mb": "1000", "mbleft": "500"},
					},
				},
			})
			return
		}
		if mode == "history" {
			json.NewEncoder(w).Encode(map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{
						{"nzo_id": "nzo_2", "name": "Movie2", "status": "Completed", "bytes": 2000000000, "storage": "/complete/Movie2"},
					},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "arrgo")
	list, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 downloads, got %d", len(list))
	}
}

func TestSABnzbdClient_Remove(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") == "queue" && r.URL.Query().Get("name") == "delete" {
			called = true
			json.NewEncoder(w).Encode(map[string]any{"status": true})
			return
		}
	}))
	defer server.Close()

	client := NewSABnzbdClient(server.URL, "test-key", "arrgo")
	err := client.Remove(context.Background(), "nzo_abc123", false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !called {
		t.Error("delete endpoint was not called")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/download/... -v -run "TestSABnzbd"`
Expected: FAIL

**Step 3: Create sabnzbd.go with implementation**

```go
// internal/download/sabnzbd.go
package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SABnzbdClient interacts with SABnzbd via its API.
type SABnzbdClient struct {
	baseURL    string
	apiKey     string
	category   string
	httpClient *http.Client
}

// NewSABnzbdClient creates a new SABnzbd client.
func NewSABnzbdClient(baseURL, apiKey, category string) *SABnzbdClient {
	return &SABnzbdClient{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		apiKey:   apiKey,
		category: category,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *SABnzbdClient) apiURL(mode string, extra url.Values) string {
	params := url.Values{}
	params.Set("apikey", c.apiKey)
	params.Set("output", "json")
	params.Set("mode", mode)
	for k, v := range extra {
		params[k] = v
	}
	return c.baseURL + "/api?" + params.Encode()
}

func (c *SABnzbdClient) doRequest(ctx context.Context, url string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrClientUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Add sends an NZB URL to SABnzbd and returns the nzo_id.
func (c *SABnzbdClient) Add(ctx context.Context, nzbURL, category string) (string, error) {
	if category == "" {
		category = c.category
	}

	extra := url.Values{}
	extra.Set("name", nzbURL)
	extra.Set("cat", category)

	var result struct {
		Status bool     `json:"status"`
		NzoIDs []string `json:"nzo_ids"`
		Error  string   `json:"error"`
	}

	if err := c.doRequest(ctx, c.apiURL("addurl", extra), &result); err != nil {
		return "", err
	}

	if !result.Status {
		if strings.Contains(strings.ToLower(result.Error), "api key") {
			return "", ErrInvalidAPIKey
		}
		return "", fmt.Errorf("sabnzbd: %s", result.Error)
	}

	if len(result.NzoIDs) == 0 {
		return "", fmt.Errorf("sabnzbd: no nzo_id returned")
	}

	return result.NzoIDs[0], nil
}

// Status returns the status of a download by nzo_id.
func (c *SABnzbdClient) Status(ctx context.Context, nzoID string) (*ClientStatus, error) {
	// Check queue first
	status, err := c.findInQueue(ctx, nzoID)
	if err != nil {
		return nil, err
	}
	if status != nil {
		return status, nil
	}

	// Check history
	status, err = c.findInHistory(ctx, nzoID)
	if err != nil {
		return nil, err
	}
	if status != nil {
		return status, nil
	}

	return nil, ErrDownloadNotFound
}

func (c *SABnzbdClient) findInQueue(ctx context.Context, nzoID string) (*ClientStatus, error) {
	var result struct {
		Queue struct {
			Slots []struct {
				NzoID      string `json:"nzo_id"`
				Filename   string `json:"filename"`
				Status     string `json:"status"`
				Percentage string `json:"percentage"`
				MB         string `json:"mb"`
				MBLeft     string `json:"mbleft"`
				Speed      string `json:"speed"`
				TimeLeft   string `json:"timeleft"`
			} `json:"slots"`
		} `json:"queue"`
	}

	if err := c.doRequest(ctx, c.apiURL("queue", nil), &result); err != nil {
		return nil, err
	}

	for _, slot := range result.Queue.Slots {
		if slot.NzoID == nzoID {
			progress, _ := strconv.ParseFloat(slot.Percentage, 64)
			sizeMB, _ := strconv.ParseFloat(slot.MB, 64)
			return &ClientStatus{
				ID:       slot.NzoID,
				Name:     slot.Filename,
				Status:   mapSABnzbdStatus(slot.Status),
				Progress: progress,
				Size:     int64(sizeMB * 1024 * 1024),
				ETA:      parseTimeLeft(slot.TimeLeft),
			}, nil
		}
	}
	return nil, nil
}

func (c *SABnzbdClient) findInHistory(ctx context.Context, nzoID string) (*ClientStatus, error) {
	var result struct {
		History struct {
			Slots []struct {
				NzoID   string `json:"nzo_id"`
				Name    string `json:"name"`
				Status  string `json:"status"`
				Bytes   int64  `json:"bytes"`
				Storage string `json:"storage"`
			} `json:"slots"`
		} `json:"history"`
	}

	if err := c.doRequest(ctx, c.apiURL("history", nil), &result); err != nil {
		return nil, err
	}

	for _, slot := range result.History.Slots {
		if slot.NzoID == nzoID {
			return &ClientStatus{
				ID:       slot.NzoID,
				Name:     slot.Name,
				Status:   mapSABnzbdStatus(slot.Status),
				Progress: 100,
				Size:     slot.Bytes,
				Path:     slot.Storage,
			}, nil
		}
	}
	return nil, nil
}

// List returns all downloads from queue and history.
func (c *SABnzbdClient) List(ctx context.Context) ([]*ClientStatus, error) {
	var results []*ClientStatus

	// Get queue
	var queueResult struct {
		Queue struct {
			Slots []struct {
				NzoID      string `json:"nzo_id"`
				Filename   string `json:"filename"`
				Status     string `json:"status"`
				Percentage string `json:"percentage"`
				MB         string `json:"mb"`
			} `json:"slots"`
		} `json:"queue"`
	}
	if err := c.doRequest(ctx, c.apiURL("queue", nil), &queueResult); err != nil {
		return nil, err
	}
	for _, slot := range queueResult.Queue.Slots {
		progress, _ := strconv.ParseFloat(slot.Percentage, 64)
		sizeMB, _ := strconv.ParseFloat(slot.MB, 64)
		results = append(results, &ClientStatus{
			ID:       slot.NzoID,
			Name:     slot.Filename,
			Status:   mapSABnzbdStatus(slot.Status),
			Progress: progress,
			Size:     int64(sizeMB * 1024 * 1024),
		})
	}

	// Get history
	var histResult struct {
		History struct {
			Slots []struct {
				NzoID   string `json:"nzo_id"`
				Name    string `json:"name"`
				Status  string `json:"status"`
				Bytes   int64  `json:"bytes"`
				Storage string `json:"storage"`
			} `json:"slots"`
		} `json:"history"`
	}
	if err := c.doRequest(ctx, c.apiURL("history", nil), &histResult); err != nil {
		return nil, err
	}
	for _, slot := range histResult.History.Slots {
		results = append(results, &ClientStatus{
			ID:       slot.NzoID,
			Name:     slot.Name,
			Status:   mapSABnzbdStatus(slot.Status),
			Progress: 100,
			Size:     slot.Bytes,
			Path:     slot.Storage,
		})
	}

	return results, nil
}

// Remove deletes a download from the queue.
func (c *SABnzbdClient) Remove(ctx context.Context, nzoID string, deleteFiles bool) error {
	extra := url.Values{}
	extra.Set("name", "delete")
	extra.Set("value", nzoID)
	if deleteFiles {
		extra.Set("del_files", "1")
	}

	var result struct {
		Status bool   `json:"status"`
		Error  string `json:"error"`
	}

	if err := c.doRequest(ctx, c.apiURL("queue", extra), &result); err != nil {
		return err
	}

	if !result.Status && result.Error != "" {
		return fmt.Errorf("sabnzbd: %s", result.Error)
	}
	return nil
}

func mapSABnzbdStatus(s string) Status {
	switch strings.ToLower(s) {
	case "completed":
		return StatusCompleted
	case "failed":
		return StatusFailed
	default:
		// Queued, Downloading, Paused, Extracting, Verifying, Repairing, etc.
		return StatusDownloading
	}
}

func parseTimeLeft(s string) time.Duration {
	// Format: "H:MM:SS" or "0:04:30"
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	sec, _ := strconv.Atoi(parts[2])
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second
}
```

**Step 4: Remove SABnzbdClient from download.go**

Remove the `SABnzbdClient` struct and its methods from `download.go` (keep only the interface and types). The `NewSABnzbdClient` and methods are now in `sabnzbd.go`.

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/download/... -v -run "TestSABnzbd"`
Expected: PASS

**Step 6: Run linter**

Run: `golangci-lint run ./internal/download/...`
Expected: No issues

**Step 7: Commit**

```bash
git add internal/download/
git commit -m "feat(download): implement SABnzbd client"
```

---

### Task 5: Manager Implementation

**Files:**
- Create: `internal/download/manager.go`
- Create: `internal/download/manager_test.go`

**Step 1: Write Manager tests**

```go
// internal/download/manager_test.go
package download

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockDownloader struct {
	addResult    string
	addErr       error
	statusResult *ClientStatus
	statusErr    error
	listResult   []*ClientStatus
	listErr      error
	removeErr    error
}

func (m *mockDownloader) Add(ctx context.Context, url, category string) (string, error) {
	return m.addResult, m.addErr
}

func (m *mockDownloader) Status(ctx context.Context, clientID string) (*ClientStatus, error) {
	return m.statusResult, m.statusErr
}

func (m *mockDownloader) List(ctx context.Context) ([]*ClientStatus, error) {
	return m.listResult, m.listErr
}

func (m *mockDownloader) Remove(ctx context.Context, clientID string, deleteFiles bool) error {
	return m.removeErr
}

func TestManager_Grab(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	client := &mockDownloader{addResult: "nzo_abc123"}
	mgr := NewManager(client, store)

	d, err := mgr.Grab(context.Background(), 1, nil, "http://example.com/test.nzb", "Test.Movie.2024.1080p", "TestIndexer")
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}

	if d.ClientID != "nzo_abc123" {
		t.Errorf("ClientID = %q, want nzo_abc123", d.ClientID)
	}
	if d.Status != StatusQueued {
		t.Errorf("Status = %q, want queued", d.Status)
	}
	if d.ID == 0 {
		t.Error("download should be saved to DB")
	}
}

func TestManager_Grab_ClientError(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	client := &mockDownloader{addErr: ErrClientUnavailable}
	mgr := NewManager(client, store)

	_, err := mgr.Grab(context.Background(), 1, nil, "http://example.com/test.nzb", "Test.Movie", "Indexer")
	if !errors.Is(err, ErrClientUnavailable) {
		t.Errorf("expected ErrClientUnavailable, got %v", err)
	}

	// Should not have saved to DB
	downloads, _ := store.List(DownloadFilter{})
	if len(downloads) != 0 {
		t.Error("download should not be in DB after client error")
	}
}

func TestManager_Refresh(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	// Add a download
	d := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	// Mock client returns completed status
	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:       "nzo_abc123",
			Status:   StatusCompleted,
			Progress: 100,
			Path:     "/complete/Test.Movie",
		},
	}
	mgr := NewManager(client, store)

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Should have updated status in DB
	updated, _ := store.Get(d.ID)
	if updated.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", updated.Status)
	}
	if updated.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestManager_Cancel(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	d := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	client := &mockDownloader{}
	mgr := NewManager(client, store)

	if err := mgr.Cancel(context.Background(), d.ID, false); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// Should be deleted from DB
	_, err := store.Get(d.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after cancel, got %v", err)
	}
}

func TestManager_GetActive(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	_, _ = db.Exec(`INSERT INTO content (type, title, year, status, quality_profile, root_path)
		VALUES ('movie', 'Test Movie', 2024, 'wanted', 'hd', '/movies')`)

	d := &Download{
		ContentID:   1,
		Client:      ClientSABnzbd,
		ClientID:    "nzo_abc123",
		Status:      StatusDownloading,
		ReleaseName: "Test.Movie",
	}
	_ = store.Add(d)

	client := &mockDownloader{
		statusResult: &ClientStatus{
			ID:       "nzo_abc123",
			Status:   StatusDownloading,
			Progress: 50,
			Speed:    10000000,
			ETA:      5 * time.Minute,
		},
	}
	mgr := NewManager(client, store)

	active, err := mgr.GetActive(context.Background())
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active download, got %d", len(active))
	}
	if active[0].Live.Progress != 50 {
		t.Errorf("Progress = %f, want 50", active[0].Live.Progress)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/download/... -v -run "TestManager"`
Expected: FAIL

**Step 3: Implement Manager**

```go
// internal/download/manager.go
package download

import (
	"context"
	"fmt"
	"time"
)

// ActiveDownload combines database record with live client status.
type ActiveDownload struct {
	Download *Download
	Live     *ClientStatus
}

// Manager orchestrates download operations.
type Manager struct {
	client Downloader
	store  *Store
}

// NewManager creates a new download manager.
func NewManager(client Downloader, store *Store) *Manager {
	return &Manager{
		client: client,
		store:  store,
	}
}

// Grab sends a release to the download client and records it in the database.
func (m *Manager) Grab(ctx context.Context, contentID int64, episodeID *int64,
	downloadURL, releaseName, indexer string) (*Download, error) {

	// Send to download client first
	clientID, err := m.client.Add(ctx, downloadURL, "")
	if err != nil {
		return nil, fmt.Errorf("add to client: %w", err)
	}

	// Record in database (idempotent)
	d := &Download{
		ContentID:   contentID,
		EpisodeID:   episodeID,
		Client:      ClientSABnzbd, // TODO: make configurable when adding other clients
		ClientID:    clientID,
		Status:      StatusQueued,
		ReleaseName: releaseName,
		Indexer:     indexer,
	}

	if err := m.store.Add(d); err != nil {
		// Orphan in client is acceptable - Refresh will find it
		return nil, fmt.Errorf("save download: %w", err)
	}

	return d, nil
}

// Refresh polls the download client for status updates and syncs to the database.
func (m *Manager) Refresh(ctx context.Context) error {
	downloads, err := m.store.List(DownloadFilter{Active: true})
	if err != nil {
		return fmt.Errorf("list active: %w", err)
	}

	var lastErr error
	for _, d := range downloads {
		status, err := m.client.Status(ctx, d.ClientID)
		if err != nil {
			lastErr = err
			continue
		}

		if status.Status != d.Status {
			d.Status = status.Status
			if status.Status == StatusCompleted || status.Status == StatusFailed {
				now := time.Now()
				d.CompletedAt = &now
			}
			if err := m.store.Update(d); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}

// Cancel removes a download from the client and database.
func (m *Manager) Cancel(ctx context.Context, downloadID int64, deleteFiles bool) error {
	d, err := m.store.Get(downloadID)
	if err != nil {
		return fmt.Errorf("get download: %w", err)
	}

	// Remove from client (best effort - may already be gone)
	_ = m.client.Remove(ctx, d.ClientID, deleteFiles)

	// Remove from database
	if err := m.store.Delete(downloadID); err != nil {
		return fmt.Errorf("delete download: %w", err)
	}

	return nil
}

// GetActive returns active downloads with live status from the client.
func (m *Manager) GetActive(ctx context.Context) ([]*ActiveDownload, error) {
	downloads, err := m.store.List(DownloadFilter{Active: true})
	if err != nil {
		return nil, fmt.Errorf("list active: %w", err)
	}

	results := make([]*ActiveDownload, 0, len(downloads))
	for _, d := range downloads {
		live, err := m.client.Status(ctx, d.ClientID)
		if err != nil {
			// Include download without live status
			results = append(results, &ActiveDownload{Download: d})
			continue
		}
		results = append(results, &ActiveDownload{Download: d, Live: live})
	}

	return results, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/download/... -v -run "TestManager"`
Expected: PASS

**Step 5: Run linter**

Run: `golangci-lint run ./internal/download/...`
Expected: No issues

**Step 6: Commit**

```bash
git add internal/download/manager.go internal/download/manager_test.go
git commit -m "feat(download): implement Manager with Grab, Refresh, Cancel, GetActive"
```

---

### Task 6: Final Verification

**Step 1: Run all tests**

Run: `go test ./internal/download/... -v`
Expected: All tests PASS

**Step 2: Run linter**

Run: `golangci-lint run ./internal/download/...`
Expected: No issues

**Step 3: Verify build**

Run: `go build ./...`
Expected: Success

**Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix(download): resolve any lint issues"
```

---

## Summary

| Task | Component | Description |
|------|-----------|-------------|
| 1 | errors.go | Error types (ErrClientUnavailable, ErrInvalidAPIKey, etc.) |
| 2 | download.go | DownloadFilter struct |
| 3 | download.go + store_test.go | Store implementation with idempotent Add |
| 4 | sabnzbd.go + sabnzbd_test.go | SABnzbd HTTP client |
| 5 | manager.go + manager_test.go | Manager orchestration |
| 6 | - | Final verification |
