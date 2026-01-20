# Testing Infrastructure Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete migration to mockgen for all mocks and testify for all assertions.

**Architecture:** Phase A adds mockgen infrastructure for search and download packages, updates tests to use generated mocks. Phase B converts all 47 test files to use testify assertions, one package at a time.

**Tech Stack:** `go.uber.org/mock/mockgen`, `github.com/stretchr/testify`

---

## Phase A: Mock Migration

### Task 1: Generate IndexerAPI Mock

**Files:**
- Create: `internal/search/generate.go`
- Create: `internal/search/mocks/mocks.go` (generated)

**Step 1: Create generate.go**

Create `internal/search/generate.go`:

```go
package search

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/search IndexerAPI
```

**Step 2: Create mocks directory and generate**

```bash
mkdir -p internal/search/mocks
go generate ./internal/search/...
```

**Step 3: Verify mock was generated**

Run: `ls internal/search/mocks/`

Expected: `mocks.go` exists

**Step 4: Verify build**

Run: `go build ./internal/search/...`

Expected: Clean build

**Step 5: Commit**

```bash
git add internal/search/generate.go internal/search/mocks/
git commit -m "test: add mockgen infrastructure for IndexerAPI"
```

---

### Task 2: Update Search Tests to Use Generated Mock

**Files:**
- Modify: `internal/search/search_test.go`

**Step 1: Update imports**

Add to imports:

```go
"github.com/vmunix/arrgo/internal/search/mocks"
"go.uber.org/mock/gomock"
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
```

**Step 2: Remove manual mock**

Delete lines 20-28 (the manual `mockIndexerAPI` struct and method).

**Step 3: Update TestSearcher_Search**

Replace the test function with:

```go
func TestSearcher_Search(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p", "720p"},
			Sources:    []string{"bluray", "webdl"},
		},
	}
	scorer := NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1", Indexer: "nzbgeek"},
			{Title: "Movie.2024.720p.BluRay.x264-OTHER", GUID: "2", Indexer: "nzbgeek"},
			{Title: "Movie.2024.480p.DVDRip.x264-BAD", GUID: "3", Indexer: "nzbgeek"},
			{Title: "Movie.2024.1080p.WEB-DL.x264-WEB", GUID: "4", Indexer: "nzbgeek"},
		}, nil)

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")

	require.NoError(t, err)
	assert.Len(t, result.Releases, 3, "should filter out 480p DVDRip")

	// Verify sorted by score (1080p BluRay first)
	require.NotEmpty(t, result.Releases)
	assert.Equal(t, "1", result.Releases[0].GUID)
}
```

**Step 4: Update TestSearcher_Search_NoMatches**

Replace with:

```go
func TestSearcher_Search_NoMatches(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Sources:    []string{"bluray"},
		},
	}
	scorer := NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]Release{
			{Title: "Movie.2024.480p.DVDRip.x264-BAD", GUID: "1", Indexer: "nzbgeek"},
		}, nil)

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")

	require.NoError(t, err)
	assert.Empty(t, result.Releases)
}
```

**Step 5: Update TestSearcher_Search_Error**

Replace with:

```go
func TestSearcher_Search_Error(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}, Sources: []string{"bluray"}},
	}
	scorer := NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return(nil, []error{errors.New("indexer unavailable")})

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")

	require.NoError(t, err) // errors are collected, not returned
	assert.Empty(t, result.Releases)
	assert.Len(t, result.Errors, 1)
}
```

**Step 6: Update TestSearcher_Search_InvalidProfile**

Replace with:

```go
func TestSearcher_Search_InvalidProfile(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}, Sources: []string{"bluray"}},
	}
	scorer := NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	// No EXPECT - should fail before calling indexer

	searcher := NewSearcher(mockClient, scorer, testLogger())
	_, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown quality profile")
}
```

**Step 7: Run tests**

Run: `go test ./internal/search/... -v`

Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/search/search_test.go
git commit -m "test: migrate search tests to mockgen and testify"
```

---

### Task 3: Generate Downloader Mock

**Files:**
- Create: `internal/download/generate.go`
- Create: `internal/download/mocks/mocks.go` (generated)

**Step 1: Create generate.go**

Create `internal/download/generate.go`:

```go
package download

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/download Downloader
```

**Step 2: Create mocks directory and generate**

```bash
mkdir -p internal/download/mocks
go generate ./internal/download/...
```

**Step 3: Verify mock was generated**

Run: `ls internal/download/mocks/`

Expected: `mocks.go` exists

**Step 4: Verify build**

Run: `go build ./internal/download/...`

Expected: Clean build

**Step 5: Commit**

```bash
git add internal/download/generate.go internal/download/mocks/
git commit -m "test: add mockgen infrastructure for Downloader"
```

---

### Task 4: Update Download Manager Tests to Use Generated Mock

**Files:**
- Modify: `internal/download/manager_test.go`

**Step 1: Update imports**

Add to imports:

```go
"github.com/vmunix/arrgo/internal/download/mocks"
"go.uber.org/mock/gomock"
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
```

**Step 2: Remove manual mock**

Delete lines 17-43 (the manual `mockDownloader` struct and methods).

**Step 3: Update TestManager_Grab**

Replace with:

```go
func TestManager_Grab(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Add(gomock.Any(), "http://example.com/test.nzb", "").
		Return("nzo_abc123", nil)

	mgr := NewManager(client, store, testLogger())

	d, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024.1080p", "TestIndexer")

	require.NoError(t, err)
	assert.Equal(t, "nzo_abc123", d.ClientID)
	assert.Equal(t, contentID, d.ContentID)
	assert.Equal(t, StatusQueued, d.Status)
}
```

**Step 4: Update TestManager_Grab_Error**

Replace with:

```go
func TestManager_Grab_Error(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Add(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("connection refused"))

	mgr := NewManager(client, store, testLogger())

	_, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test.Movie.2024.1080p", "TestIndexer")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}
```

**Step 5: Update TestManager_Cancel**

Replace with:

```go
func TestManager_Cancel(t *testing.T) {
	ctrl := gomock.NewController(t)

	db := setupTestDB(t)
	store := NewStore(db)
	contentID := insertTestContent(t, db, "Test Movie")

	client := mocks.NewMockDownloader(ctrl)
	client.EXPECT().
		Add(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("nzo_abc123", nil)
	client.EXPECT().
		Remove(gomock.Any(), "nzo_abc123", true).
		Return(nil)

	mgr := NewManager(client, store, testLogger())

	d, err := mgr.Grab(context.Background(), contentID, nil, "http://example.com/test.nzb", "Test", "Indexer")
	require.NoError(t, err)

	err = mgr.Cancel(context.Background(), d.ID, true)
	require.NoError(t, err)

	// Verify status updated
	updated, err := store.GetDownload(d.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, updated.Status)
}
```

**Step 6: Update remaining tests similarly**

Apply the same pattern to:
- `TestManager_Poll`
- `TestManager_Poll_Completed`
- `TestManager_Poll_Failed`

Each test should:
1. Create `gomock.NewController(t)`
2. Use `mocks.NewMockDownloader(ctrl)`
3. Set up `EXPECT()` calls
4. Use `require.NoError`/`assert.Equal` for assertions

**Step 7: Run tests**

Run: `go test ./internal/download/... -v`

Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/download/manager_test.go
git commit -m "test: migrate download manager tests to mockgen and testify"
```

---

### Task 5: Update Integration Tests to Use Search Mock

**Files:**
- Modify: `internal/api/v1/integration_test.go`

**Step 1: Update imports**

Add:

```go
searchmocks "github.com/vmunix/arrgo/internal/search/mocks"
```

**Step 2: Remove duplicate mockIndexerAPI**

Delete lines 29-40 (the duplicate `mockIndexerAPI` struct).

**Step 3: Update mock usage**

Find where `mockIndexerAPI` is used and replace with `searchmocks.NewMockIndexerAPI(ctrl)`.

Note: Integration tests may need more substantial updates depending on how mocks are constructed. Review the test setup carefully.

**Step 4: Run tests**

Run: `go test ./internal/api/v1/... -v`

Expected: All tests pass

**Step 5: Commit**

```bash
git add internal/api/v1/integration_test.go
git commit -m "test: use search mock in integration tests"
```

---

### Task 6: Phase A Verification

**Step 1: Run full test suite**

Run: `task test`

Expected: All tests pass

**Step 2: Run linter**

Run: `task lint`

Expected: No lint errors

**Step 3: Verify no manual mocks remain**

Run: `grep -rn "type mock" --include="*_test.go" .`

Expected: No matches (or only in mocks/ directories)

**Step 4: Verify mocks are up to date**

Run: `go generate ./... && git diff --exit-code`

Expected: No changes

---

## Phase B: Testify Migration

### Task 7: Convert pkg/release Tests

**Files:**
- Modify: `pkg/release/release_test.go`
- Modify: `pkg/release/corpus_test.go`
- Modify: `pkg/release/normalize_test.go`
- Modify: `pkg/release/snapshot_test.go`
- Modify: `pkg/release/golden_test.go`

**Step 1: Add testify imports to each file**

```go
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
```

**Step 2: Convert assertions**

Pattern replacements:
- `if err != nil { t.Fatalf(...) }` → `require.NoError(t, err)`
- `if got != want { t.Errorf(...) }` → `assert.Equal(t, want, got)`
- `if !reflect.DeepEqual(got, want) { t.Errorf(...) }` → `assert.Equal(t, want, got)`

**Step 3: Run tests**

Run: `go test ./pkg/release/... -v`

Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/release/
git commit -m "test: convert pkg/release to testify"
```

---

### Task 8: Convert pkg/newznab Tests

**Files:**
- Modify: `pkg/newznab/client_test.go`

**Step 1: Add testify imports**

**Step 2: Convert assertions**

**Step 3: Run tests**

Run: `go test ./pkg/newznab/... -v`

**Step 4: Commit**

```bash
git add pkg/newznab/
git commit -m "test: convert pkg/newznab to testify"
```

---

### Task 9: Convert internal/config Tests

**Files:**
- Modify: All `*_test.go` files in `internal/config/`

**Step 1: Add testify imports to each file**

**Step 2: Convert assertions**

**Step 3: Run tests**

Run: `go test ./internal/config/... -v`

**Step 4: Commit**

```bash
git add internal/config/
git commit -m "test: convert internal/config to testify"
```

---

### Task 10: Convert internal/library Tests

**Files:**
- Modify: `internal/library/store_test.go`
- Modify: `internal/library/episode_test.go`
- Modify: `internal/library/file_test.go`
- Modify: `internal/library/tx_test.go`
- Modify: `internal/library/errors_test.go`
- Skip: `internal/library/sqlite_compat_test.go` (already uses testify)

**Step 1-4: Same pattern as above**

**Commit:**

```bash
git add internal/library/
git commit -m "test: convert internal/library to testify"
```

---

### Task 11: Convert internal/download Tests

**Files:**
- Modify: All `*_test.go` files in `internal/download/` except `manager_test.go` (already done)

**Commit:**

```bash
git add internal/download/
git commit -m "test: convert internal/download to testify"
```

---

### Task 12: Convert internal/importer Tests

**Files:**
- Modify: All `*_test.go` files in `internal/importer/`

**Commit:**

```bash
git add internal/importer/
git commit -m "test: convert internal/importer to testify"
```

---

### Task 13: Convert internal/search Tests

**Files:**
- Modify: `internal/search/scorer_test.go`
- Modify: `internal/search/errors_test.go`
- Skip: `internal/search/search_test.go` (already done in Task 2)

**Commit:**

```bash
git add internal/search/
git commit -m "test: convert internal/search to testify"
```

---

### Task 14: Convert internal/api Tests

**Files:**
- Modify: `internal/api/compat/radarr_test.go`
- Modify: `internal/api/v1/api_test.go` (remaining unconverted tests)
- Modify: `internal/api/v1/integration_test.go`

**Commit:**

```bash
git add internal/api/
git commit -m "test: convert internal/api to testify"
```

---

### Task 15: Convert cmd/arrgo Tests

**Files:**
- Modify: `cmd/arrgo/search_test.go`
- Modify: `cmd/arrgo/queue_test.go`
- Modify: `cmd/arrgo/status_test.go`
- Modify: `cmd/arrgo/parse_test.go`
- Modify: `cmd/arrgo/commands_test.go`

**Commit:**

```bash
git add cmd/arrgo/
git commit -m "test: convert cmd/arrgo to testify"
```

---

### Task 16: Final Verification

**Step 1: Run full test suite**

Run: `task test`

Expected: All tests pass

**Step 2: Run linter**

Run: `task lint`

Expected: No lint errors

**Step 3: Verify no manual assertions remain**

Run: `grep -rE "t\.Errorf|t\.Fatalf|t\.Error\(|t\.Fatal\(" --include="*_test.go" . | wc -l`

Expected: 0

**Step 4: Count testify usage**

Run: `grep -rE "assert\.|require\." --include="*_test.go" . | wc -l`

Expected: ~1200+ (all assertions now use testify)

---

## Summary

After completing all tasks:
- All mocks use mockgen (3 packages: search, download, api/v1)
- All tests use testify assertions
- Consistent patterns ready for series testing work
