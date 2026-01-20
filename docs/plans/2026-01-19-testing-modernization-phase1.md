# Testing Modernization (Phase 1) Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add testify for cleaner assertions and mockgen for generated interface mocks.

**Architecture:** Add testify/require for fatal assertions and testify/assert for non-fatal. Generate mocks for the four optional service interfaces in `internal/api/v1/deps.go`. New/modified tests adopt testify; existing tests unchanged unless touched.

**Tech Stack:** `github.com/stretchr/testify`, `go.uber.org/mock/mockgen`

---

### Task 1: Add Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add testify and mockgen to go.mod**

```bash
go get github.com/stretchr/testify
go get go.uber.org/mock/mockgen
```

**Step 2: Verify dependencies added**

Run: `grep -E "testify|uber.*mock" go.mod`

Expected: Both dependencies appear in require block.

**Step 3: Tidy and verify**

Run: `go mod tidy && go build ./...`

Expected: Clean build, no errors.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add testify and mockgen for testing improvements"
```

---

### Task 2: Create Mock Generation Infrastructure

**Files:**
- Create: `internal/api/v1/generate.go`
- Create: `internal/api/v1/mocks/` directory

**Step 1: Create generate.go with mockgen directive**

Create `internal/api/v1/generate.go`:

```go
package v1

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/api/v1 Searcher,DownloadManager,PlexClient,FileImporter
```

**Step 2: Create mocks directory**

```bash
mkdir -p internal/api/v1/mocks
```

**Step 3: Run go generate**

Run: `go generate ./internal/api/v1/...`

Expected: Creates `internal/api/v1/mocks/mocks.go` with mock implementations.

**Step 4: Verify mocks compile**

Run: `go build ./internal/api/v1/mocks/...`

Expected: Clean build.

**Step 5: Commit**

```bash
git add internal/api/v1/generate.go internal/api/v1/mocks/
git commit -m "test: add mockgen infrastructure for API interfaces"
```

---

### Task 3: Demonstrate testify in API Tests

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Add testify imports**

Add to imports:

```go
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
```

**Step 2: Convert TestNew to use testify**

Before:
```go
func TestNew(t *testing.T) {
	db := setupTestDB(t)
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
	}

	srv := New(db, cfg)
	if srv == nil {
		t.Fatal("New returned nil")
	}
	if srv.deps.Library == nil {
		t.Error("library store not initialized")
	}
	if srv.deps.Downloads == nil {
		t.Error("download store not initialized")
	}
	if srv.deps.History == nil {
		t.Error("history store not initialized")
	}
}
```

After:
```go
func TestNew(t *testing.T) {
	db := setupTestDB(t)
	cfg := Config{
		MovieRoot:  "/movies",
		SeriesRoot: "/tv",
	}

	srv := New(db, cfg)
	require.NotNil(t, srv, "New returned nil")
	assert.NotNil(t, srv.deps.Library, "library store not initialized")
	assert.NotNil(t, srv.deps.Downloads, "download store not initialized")
	assert.NotNil(t, srv.deps.History, "history store not initialized")
}
```

**Step 3: Convert TestListContent_Empty to use testify**

Before:
```go
func TestListContent_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %d, want 0", len(resp.Items))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}
```

After:
```go
func TestListContent_Empty(t *testing.T) {
	db := setupTestDB(t)
	srv := New(db, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
	w := httptest.NewRecorder()

	srv.listContent(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp listContentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
	assert.Zero(t, resp.Total)
}
```

**Step 4: Run tests to verify**

Run: `go test ./internal/api/v1/... -v -run "TestNew|TestListContent_Empty"`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test: demonstrate testify usage in API tests"
```

---

### Task 4: Demonstrate Mock Usage

**Files:**
- Modify: `internal/api/v1/api_test.go`

**Step 1: Add mock import**

Add to imports:

```go
"github.com/vmunix/arrgo/internal/api/v1/mocks"
"go.uber.org/mock/gomock"
```

**Step 2: Add test using mock Searcher**

Add new test after existing tests:

```go
func TestSearch_WithMockSearcher(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := setupTestDB(t)
	srv := New(db, Config{})

	// Create mock searcher
	mockSearcher := mocks.NewMockSearcher(ctrl)
	srv.deps.Searcher = mockSearcher

	// Set up expectation
	mockSearcher.EXPECT().
		Search(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&search.SearchResult{
			Results: []search.Result{
				{Title: "Test Movie", Year: 2024, Indexer: "TestIndexer"},
			},
			TotalResults: 1,
		}, nil)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body := `{"query":"test movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp searchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "Test Movie", resp.Results[0].Title)
}
```

**Step 3: Add search import if missing**

Ensure imports include:

```go
"github.com/vmunix/arrgo/internal/search"
```

**Step 4: Run test to verify mock works**

Run: `go test ./internal/api/v1/... -v -run TestSearch_WithMockSearcher`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/v1/api_test.go
git commit -m "test: demonstrate mockgen usage with Searcher interface"
```

---

### Task 5: Final Verification

**Step 1: Run full test suite**

Run: `task test`

Expected: All tests pass.

**Step 2: Run linter**

Run: `task lint`

Expected: No lint errors.

**Step 3: Verify generated mocks are up to date**

Run: `go generate ./... && git diff --exit-code`

Expected: No changes (mocks already current).

**Step 4: Final commit if any cleanup needed**

If any issues found, fix and commit.

---

## Summary

After completing these tasks:

1. `testify` available for cleaner assertions (`assert.Equal`, `require.NoError`)
2. `mockgen` generates type-safe mocks for `Searcher`, `DownloadManager`, `PlexClient`, `FileImporter`
3. Two example tests demonstrate testify usage pattern
4. One example test demonstrates mock usage pattern
5. Existing tests untouched - gradual adoption as files are modified
