# Testing Infrastructure Improvements Design

**Status:** ✅ Complete

## Overview

Consolidate testing patterns before adding series handling tests. Complete migration to mockgen for all mocks and testify for all assertions.

## Goals

- Consistent mocking patterns across codebase (mockgen)
- Consistent assertion patterns across codebase (testify)
- Clean foundation for series testing work

## Phase A: Mock Migration (#28)

### New Mock Packages

| Package | Interface | Mock Location |
|---------|-----------|---------------|
| `internal/search` | `IndexerAPI` | `internal/search/mocks/` |
| `internal/download` | `Downloader` | `internal/download/mocks/` |

### Files to Create

**`internal/search/generate.go`:**
```go
package search

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/search IndexerAPI
```

**`internal/download/generate.go`:**
```go
package download

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/download Downloader
```

### Files to Update

- `internal/search/search_test.go` - Replace manual `mockIndexerAPI` with generated mock
- `internal/download/manager_test.go` - Replace manual `mockDownloader` with generated mock
- `internal/api/v1/integration_test.go` - Import `search/mocks` instead of defining duplicate

### Removed

- Manual `mockIndexerAPI` struct from `search_test.go`
- Manual `mockIndexerAPI` struct from `integration_test.go`
- Manual `mockDownloader` struct from `manager_test.go`

## Phase B: Testify Migration (#29)

### Scope

- 47 test files
- ~1,178 manual assertions

### Conversion Patterns

| Before | After |
|--------|-------|
| `if err != nil { t.Fatalf(...) }` | `require.NoError(t, err)` |
| `if got != want { t.Errorf(...) }` | `assert.Equal(t, want, got)` |
| `if x == nil { t.Fatal(...) }` | `require.NotNil(t, x)` |
| `if len(x) != n { t.Errorf(...) }` | `assert.Len(t, x, n)` |
| `if !ok { t.Errorf(...) }` | `assert.True(t, ok)` |
| `if err == nil { t.Fatal(...) }` | `require.Error(t, err)` |
| `if !errors.Is(err, X) {...}` | `assert.ErrorIs(t, err, X)` |

### When to Use Which

- `require.X` - Fatal assertions (test stops on failure)
- `assert.X` - Non-fatal assertions (test continues, shows all failures)

### Package Order

1. `pkg/release` (6 files)
2. `pkg/newznab` (1 file)
3. `internal/config` (8 files)
4. `internal/library` (5 files - 1 already done)
5. `internal/download` (8 files)
6. `internal/importer` (8 files)
7. `internal/search` (3 files)
8. `internal/api/compat` (1 file)
9. `internal/api/v1` (1 file - partial already)
10. `cmd/arrgo` (6 files)

One commit per package. Tests run after each.

## Execution Order

### Phase A: Mock Migration (~1 hour)

1. Create `internal/search/generate.go` + generate mocks
2. Update `internal/search/search_test.go`
3. Create `internal/download/generate.go` + generate mocks
4. Update `internal/download/manager_test.go`
5. Update `internal/api/v1/integration_test.go`
6. Verify all tests pass, commit

### Phase B: Testify Migration (~2-3 hours)

Batch by package, one commit each, tests after each conversion.

## Success Criteria

After completion:
- Zero manual mocks (all use mockgen)
- Zero `t.Errorf`/`t.Fatalf` (all use testify)
- All tests pass
- Lint clean

## New Test Pattern

Series tests will follow this pattern:

```go
func TestSeriesEpisode_Something(t *testing.T) {
    ctrl := gomock.NewController(t)
    mockIndexer := mocks.NewMockIndexerAPI(ctrl)

    mockIndexer.EXPECT().
        Search(gomock.Any(), gomock.Any()).
        Return([]Release{{Title: "Show.S01E01"}}, nil)

    // ... test logic ...

    require.NoError(t, err)
    assert.Equal(t, expected, got)
}
```

## Related Issues

- #28 - Migrate remaining manual mocks to mockgen
- #29 - Migrate tests to testify assertions
