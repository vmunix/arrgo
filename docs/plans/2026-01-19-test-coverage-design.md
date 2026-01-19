# Test Coverage Improvements Design

**Date:** 2026-01-19
**Status:** ✅ Complete (2026-01-19)
**Author:** Mark + Claude

## Overview

Improve test coverage and robustness through two initiatives:

1. **Increase coverage for 0% packages** — Add tests for cmd/, internal/api/compat
2. **Data-driven parser tests** — Golden tests with curated cases, snapshot tests for regression

## Goals

- Get cmd/arrgo from 0% to reasonable coverage
- Get internal/api/compat from 0% to reasonable coverage
- Add ~115 curated golden test cases for release parser
- Add snapshot regression tests for the full 1824-release corpus
- Do NOT copy Radarr test data (GPL-3.0 contamination risk)

## Non-Goals

- Subprocess smoke tests (deferred to post-v1, see BACKLOG.md)
- 100% coverage (diminishing returns)
- Testing internal/ai (stub implementation, test when built)

## 1. CLI Test Coverage (cmd/arrgo)

### Approach

Combine unit tests for formatting helpers with integration tests using HTTP mocks.

### Unit Tests

Test pure functions in isolation:

```go
// cmd/arrgo/commands_test.go

func TestFormatSize(t *testing.T) {
    tests := []struct {
        bytes int64
        want  string
    }{
        {0, "0 B"},
        {1024, "1.0 KB"},
        {1536, "1.5 KB"},
        {1073741824, "1.0 GB"},
        {12000000000, "11.2 GB"},
    }
    for _, tt := range tests {
        if got := formatSize(tt.bytes); got != tt.want {
            t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
        }
    }
}

func TestBuildBadges(t *testing.T) {
    // Test badge generation for various release.Info combinations
}
```

### Integration Tests (HTTP Mocks)

Test full command flow without a real server:

```go
// cmd/arrgo/search_test.go

func TestSearchCommand(t *testing.T) {
    // Mock HTTP server returning canned search results
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify correct endpoint called
        if r.URL.Path != "/api/v1/search" {
            t.Errorf("unexpected path: %s", r.URL.Path)
        }
        // Return mock response
        json.NewEncoder(w).Encode(SearchResponse{...})
    }))
    defer srv.Close()

    // Set server URL and run command
    serverURL = srv.URL
    err := searchCmd.RunE(searchCmd, []string{"test query"})

    // Verify behavior
}
```

### Files to Create

- `cmd/arrgo/commands_test.go` — Unit tests for helpers
- `cmd/arrgo/search_test.go` — Search command integration
- `cmd/arrgo/status_test.go` — Status command integration
- `cmd/arrgo/queue_test.go` — Queue command integration

## 2. Compat API Test Coverage (internal/api/compat)

### Approach

Test the Radarr/Sonarr API translation layer with mock stores.

### Tests Needed

```go
// internal/api/compat/radarr_test.go

func TestGetMovie_TranslatesFromContent(t *testing.T) {
    // Mock library store with content
    // Call GET /api/v3/movie/:id
    // Verify response matches Radarr format
}

func TestPostMovie_CreatesContent(t *testing.T) {
    // POST Radarr-format movie
    // Verify content created in library store
}

func TestQualityProfiles_ReturnsRadarrFormat(t *testing.T) {
    // Verify profile translation
}
```

## 3. Parser Golden Tests

### Structure

```
pkg/release/
├── golden_test.go      # Curated test cases with verified expected values
├── snapshot_test.go    # Regression tests against corpus
└── testdata/
    └── snapshots/
        └── corpus.json # Auto-generated parser output snapshot
```

### Golden Test Implementation

```go
// pkg/release/golden_test.go

var goldenCases = []struct {
    name       string
    input      string
    resolution Resolution
    source     Source
    codec      Codec
    hdr        HDRFormat
    audio      AudioCodec
    isRemux    bool
    edition    string
    title      string
    year       int
    group      string
    service    string
}{
    // === Resolution Variants ===
    {
        name:       "720p BluRay",
        input:      "Movie.Title.2020.720p.BluRay.x264-GROUP",
        resolution: Resolution720p,
        source:     SourceBluRay,
        codec:      CodecX264,
        title:      "Movie Title",
        year:       2020,
        group:      "GROUP",
    },
    {
        name:       "1080p WEB-DL",
        input:      "Movie.Title.2020.1080p.WEB-DL.DD5.1.H.264-GROUP",
        resolution: Resolution1080p,
        source:     SourceWEBDL,
        codec:      CodecX264,
        title:      "Movie Title",
        year:       2020,
        group:      "GROUP",
    },
    {
        name:       "2160p UHD BluRay",
        input:      "Movie.Title.2020.2160p.UHD.BluRay.x265-GROUP",
        resolution: Resolution2160p,
        source:     SourceBluRay,
        codec:      CodecX265,
        title:      "Movie Title",
        year:       2020,
        group:      "GROUP",
    },
    {
        name:       "4K synonym",
        input:      "Movie.Title.2020.4K.WEB-DL.x265-GROUP",
        resolution: Resolution2160p,
        source:     SourceWEBDL,
        codec:      CodecX265,
    },

    // === HDR Variants ===
    {
        name:       "Dolby Vision",
        input:      "Movie.2020.2160p.WEB-DL.DV.H.265-GROUP",
        resolution: Resolution2160p,
        hdr:        DolbyVision,
    },
    {
        name:       "HDR10+",
        input:      "Movie.2020.2160p.BluRay.HDR10Plus.x265-GROUP",
        resolution: Resolution2160p,
        hdr:        HDR10Plus,
    },
    {
        name:       "HDR10",
        input:      "Movie.2020.2160p.BluRay.HDR10.x265-GROUP",
        resolution: Resolution2160p,
        hdr:        HDR10,
    },
    {
        name:       "DV HDR combo",
        input:      "Movie.2020.2160p.BluRay.DV.HDR.x265-GROUP",
        resolution: Resolution2160p,
        hdr:        DolbyVision, // DV takes precedence
    },

    // === Audio Variants ===
    {
        name:       "Atmos",
        input:      "Movie.2020.2160p.BluRay.TrueHD.Atmos.7.1.x265-GROUP",
        audio:      AudioAtmos,
    },
    {
        name:       "TrueHD",
        input:      "Movie.2020.1080p.BluRay.TrueHD.5.1.x264-GROUP",
        audio:      AudioTrueHD,
    },
    {
        name:       "DTS-HD MA",
        input:      "Movie.2020.1080p.BluRay.DTS-HD.MA.5.1.x264-GROUP",
        audio:      AudioDTSHD,
    },
    {
        name:       "DD+ / EAC3",
        input:      "Movie.2020.1080p.WEB-DL.DDP5.1.H.264-GROUP",
        audio:      AudioEAC3,
    },

    // === Remux Patterns ===
    {
        name:       "REMUX keyword",
        input:      "Movie.2020.1080p.BluRay.REMUX.AVC.DTS-HD.MA-GROUP",
        isRemux:    true,
        source:     SourceBluRay,
    },
    {
        name:       "BDRemux",
        input:      "Movie.2020.1080p.BDRemux.AVC.DTS-HD.MA-GROUP",
        isRemux:    true,
    },

    // === Edition Patterns ===
    {
        name:       "Directors Cut",
        input:      "Movie.2020.Directors.Cut.1080p.BluRay.x264-GROUP",
        edition:    "Directors Cut",
    },
    {
        name:       "Extended Edition",
        input:      "Movie.2020.Extended.Edition.1080p.BluRay.x264-GROUP",
        edition:    "Extended Edition",
    },
    {
        name:       "IMAX",
        input:      "Movie.2020.IMAX.2160p.WEB-DL.x265-GROUP",
        edition:    "IMAX",
    },
    {
        name:       "Theatrical",
        input:      "Movie.2020.Theatrical.Cut.1080p.BluRay.x264-GROUP",
        edition:    "Theatrical Cut",
    },

    // === Streaming Services ===
    {
        name:       "Netflix (NF)",
        input:      "Movie.2020.1080p.NF.WEB-DL.DDP5.1.x264-GROUP",
        service:    "NF",
        source:     SourceWEBDL,
    },
    {
        name:       "Amazon (AMZN)",
        input:      "Movie.2020.1080p.AMZN.WEB-DL.DDP5.1.H.264-GROUP",
        service:    "AMZN",
    },
    {
        name:       "Disney+ (DSNP)",
        input:      "Movie.2020.2160p.DSNP.WEB-DL.DDP5.1.DV.H.265-GROUP",
        service:    "DSNP",
    },
    {
        name:       "Apple TV+ (ATVP)",
        input:      "Movie.2020.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265-GROUP",
        service:    "ATVP",
    },

    // === Tricky Titles ===
    {
        name:       "Year in title",
        input:      "2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GROUP",
        title:      "2001 A Space Odyssey",
        year:       1968,
    },
    {
        name:       "Sequel number",
        input:      "Back.to.the.Future.Part.II.1989.1080p.BluRay.x264-GROUP",
        title:      "Back to the Future Part II",
        year:       1989,
    },
    {
        name:       "Movie with number",
        input:      "Se7en.1995.1080p.BluRay.x264-GROUP",
        title:      "Se7en",
        year:       1995,
    },

    // ... more cases curated from corpus
}

func TestParse_Golden(t *testing.T) {
    for _, tc := range goldenCases {
        t.Run(tc.name, func(t *testing.T) {
            info := Parse(tc.input)

            if tc.resolution != ResolutionUnknown && info.Resolution != tc.resolution {
                t.Errorf("resolution: got %v, want %v", info.Resolution, tc.resolution)
            }
            if tc.source != SourceUnknown && info.Source != tc.source {
                t.Errorf("source: got %v, want %v", info.Source, tc.source)
            }
            if tc.codec != CodecUnknown && info.Codec != tc.codec {
                t.Errorf("codec: got %v, want %v", info.Codec, tc.codec)
            }
            if tc.hdr != HDRNone && info.HDR != tc.hdr {
                t.Errorf("hdr: got %v, want %v", info.HDR, tc.hdr)
            }
            if tc.audio != AudioUnknown && info.Audio != tc.audio {
                t.Errorf("audio: got %v, want %v", info.Audio, tc.audio)
            }
            if tc.isRemux && !info.IsRemux {
                t.Errorf("isRemux: got false, want true")
            }
            if tc.edition != "" && info.Edition != tc.edition {
                t.Errorf("edition: got %q, want %q", info.Edition, tc.edition)
            }
            if tc.title != "" && info.Title != tc.title {
                t.Errorf("title: got %q, want %q", info.Title, tc.title)
            }
            if tc.year != 0 && info.Year != tc.year {
                t.Errorf("year: got %d, want %d", info.Year, tc.year)
            }
            if tc.group != "" && info.Group != tc.group {
                t.Errorf("group: got %q, want %q", info.Group, tc.group)
            }
            if tc.service != "" && info.Service != tc.service {
                t.Errorf("service: got %q, want %q", info.Service, tc.service)
            }
        })
    }
}
```

### Snapshot Test Implementation

```go
// pkg/release/snapshot_test.go

var updateSnapshots = flag.Bool("update", false, "update snapshot files")

func TestParse_Snapshot(t *testing.T) {
    // Load corpus
    f, err := os.Open("../../testdata/releases.csv")
    if err != nil {
        t.Skipf("corpus not found: %v", err)
    }
    defer f.Close()

    r := csv.NewReader(f)
    records, _ := r.ReadAll()

    // Parse all releases
    results := make([]SnapshotEntry, 0, len(records)-1)
    for i, rec := range records {
        if i == 0 {
            continue // skip header
        }
        info := Parse(rec[0])
        results = append(results, SnapshotEntry{
            Input: rec[0],
            Info:  info,
        })
    }

    snapshotPath := "testdata/snapshots/corpus.json"

    if *updateSnapshots {
        // Write new snapshot
        data, _ := json.MarshalIndent(results, "", "  ")
        os.MkdirAll("testdata/snapshots", 0755)
        os.WriteFile(snapshotPath, data, 0644)
        t.Log("snapshot updated")
        return
    }

    // Compare against stored snapshot
    expected, err := os.ReadFile(snapshotPath)
    if err != nil {
        t.Fatalf("snapshot not found (run with -update to create): %v", err)
    }

    var expectedResults []SnapshotEntry
    json.Unmarshal(expected, &expectedResults)

    for i, got := range results {
        if i >= len(expectedResults) {
            t.Errorf("extra result[%d]: %s", i, got.Input)
            continue
        }
        want := expectedResults[i]
        if !reflect.DeepEqual(got.Info, want.Info) {
            t.Errorf("result[%d] %s:\n  got:  %+v\n  want: %+v", i, got.Input, got.Info, want.Info)
        }
    }
}

type SnapshotEntry struct {
    Input string `json:"input"`
    Info  *Info  `json:"info"`
}
```

## 4. Category Coverage Matrix

Cases to curate from our 1824-release corpus:

| Category | Target Cases | Source |
|----------|-------------|--------|
| Resolution (720p, 1080p, 2160p, 4K, UHD) | 10 | corpus |
| Source (BluRay, WEB-DL, WEBRip, HDTV) | 15 | corpus |
| Codec (x264, x265, HEVC, AVC, H.264, H.265) | 10 | corpus |
| HDR (HDR, HDR10, HDR10+, DV, HLG, combo) | 15 | corpus |
| Audio (Atmos, TrueHD, DTS-HD, DD+, AAC, FLAC) | 15 | corpus |
| Remux patterns | 10 | corpus |
| Edition patterns | 15 | corpus |
| Streaming services | 10 | corpus |
| Tricky titles (years, numbers, sequels) | 15 | corpus + manual |

**Total: ~115 golden cases**

## 5. Implementation Tasks

### Task 1: CLI Helper Unit Tests
- Create `cmd/arrgo/commands_test.go`
- Test `formatSize()`, `buildBadges()`, `prompt()`
- Target: 10-15 test cases

### Task 2: CLI Command Integration Tests
- Create `cmd/arrgo/search_test.go`, `status_test.go`, `queue_test.go`
- Use httptest mock server
- Target: 3-5 tests per command

### Task 3: Compat API Tests
- Create `internal/api/compat/radarr_test.go`
- Test translation layer with mock stores
- Target: 10-15 tests

### Task 4: Parser Golden Tests
- Create `pkg/release/golden_test.go`
- Curate ~115 cases from corpus
- Manually verify expected values

### Task 5: Parser Snapshot Tests
- Create `pkg/release/snapshot_test.go`
- Generate initial snapshot from corpus
- Add -update flag support

### Task 6: CI Integration
- Ensure `go test ./...` runs all new tests
- Snapshot tests should fail CI if output changes unexpectedly

## Success Criteria

| Package | Before | Target | Actual |
|---------|--------|--------|--------|
| cmd/arrgo | 0% | 40%+ | 16% |
| internal/api/compat | 0% | 50%+ | **60.4%** ✅ |
| pkg/release | 89% | 92%+ | 89.2% |

Golden tests catch parser bugs with clear failure messages.
Snapshot tests catch unintended parser changes.

**Notes:**
- cmd/arrgo: 16% covers Client HTTP methods and helper functions. Higher coverage requires subprocess testing (deferred to post-v1).
- internal/api/compat: Exceeds target with comprehensive endpoint coverage.
- pkg/release: 118 golden test cases provide strong regression protection beyond raw coverage number.

**Follow-up issues:** #8, #9, #10

## References

- Radarr test methodology (inspiration only, no code copying due to GPL-3.0)
- Existing corpus: `testdata/releases.csv` (1824 releases)
- Existing corpus test: `pkg/release/corpus_test.go`
