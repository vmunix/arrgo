# Release Parser Improvements Design

**Date:** 2026-01-18
**Status:** ✅ Complete (2026-01-19)

## Overview

Improve `pkg/release` parser to handle real-world scene release names more robustly, based on analysis of Radarr's battle-tested implementation and 1824 collected release titles.

## Current State

Our parser (`pkg/release/parser.go`) handles basic cases:
- Resolution: 720p, 1080p, 2160p, 4K, UHD
- Source: BluRay, WEB-DL, WEBRip, HDTV
- Codec: x264/H264, x265/H265/HEVC
- Year: 4-digit year extraction
- Season/Episode: S01E01 format
- Group: After last hyphen
- Flags: PROPER, REPACK

**129 lines of code**, handles ~80% of standard releases.

## Gaps Identified

### Missing from Real-World Data

From our 1824-title corpus:

| Feature | Example | Current Support |
|---------|---------|-----------------|
| **Remux** | `AVC.REMUX-FraMeSToR` | ❌ |
| **HDR/DV** | `DV.HDR.HEVC`, `HDR10+` | ❌ |
| **Audio** | `DTS-HD.MA.5.1`, `TrueHD.Atmos` | ❌ |
| **Edition** | `Theatrical.Cut`, `IMAX`, `Extended` | ❌ |
| **Anime** | `[SubGroup] Title v2 [HASH]` | ❌ |
| **Daily shows** | `2026.01.16` date format | ❌ |
| **Multi-episode** | `S01E01E02`, `S01E01-E03` | ❌ |
| **Country codes** | `GBR`, `USA`, `GER` | ❌ |
| **Streaming services** | `DSNP`, `NF`, `AMZN`, `ATVP` | ❌ |

### Radarr Features We Lack

1. **Anchor-based parsing** - Scan for year/SxxExx as pivot, don't parse left-to-right
2. **Title normalization** - `CleanMovieTitle()` for matching (remove articles, accents, punctuation)
3. **Roman numeral handling** - `Part II` ↔ `Part 2`
4. **Alternative title extraction** - AKA support
5. **Release group validation** - Reject numeric-only, hash-like groups

## Design Principles

1. **Incremental improvement** - Don't rewrite, extend
2. **Token-based approach** - Split on delimiters, classify each token
3. **Anchor strategy** - Find year/SxxExx first, then work outward
4. **No AI/ML** - Pure deterministic parsing is sufficient
5. **Extensive test coverage** - Use collected corpus for regression testing

## Proposed Changes

### Phase 1: Core Improvements (v1 scope)

#### 1.1 Extended Info Struct

```go
type Info struct {
    // Existing
    Title      string
    Year       int
    Season     int
    Episode    int
    Resolution Resolution
    Source     Source
    Codec      Codec
    Group      string
    Proper     bool
    Repack     bool

    // New fields
    Edition    string     // "Directors Cut", "Extended", "IMAX", etc.
    IsRemux    bool
    HDR        HDRFormat  // None, HDR, HDR10, HDR10Plus, DolbyVision
    Audio      AudioCodec // Unknown, DTS, DTSHD, TrueHD, Atmos, etc.
    Service    string     // Streaming service: NF, AMZN, DSNP, etc.

    // Series-specific
    Episodes   []int      // For multi-episode: [1, 2, 3]
    AbsoluteEp int        // For anime absolute numbering
    DailyDate  string     // For daily shows: "2026-01-16"

    // Matching helpers
    CleanTitle string     // Normalized for matching
}
```

#### 1.2 New Enums

```go
type HDRFormat int
const (
    HDRNone HDRFormat = iota
    HDRGeneric    // "HDR"
    HDR10
    HDR10Plus
    DolbyVision
    HLG
)

type AudioCodec int
const (
    AudioUnknown AudioCodec = iota
    AudioAAC
    AudioAC3      // DD, Dolby Digital
    AudioEAC3     // DD+, DDP
    AudioDTS
    AudioDTSHD    // DTS-HD MA
    AudioTrueHD
    AudioAtmos    // Can be TrueHD Atmos or DD+ Atmos
    AudioFLAC
    AudioOpus
)
```

#### 1.3 Anchor-Based Parsing

Replace current left-to-right with:

```go
func Parse(name string) *Info {
    tokens := tokenize(name)  // Split on . _ - [ ]

    // Find anchor (year or SxxExx)
    anchorIdx := findAnchor(tokens)

    // Everything left of anchor = title candidate
    // Everything right of anchor = metadata

    titleTokens := tokens[:anchorIdx]
    metaTokens := tokens[anchorIdx:]

    // Classify metadata tokens
    info := classifyMetadata(metaTokens)

    // Build title from remaining
    info.Title = buildTitle(titleTokens)
    info.CleanTitle = cleanTitle(info.Title)

    return info
}
```

#### 1.4 Token Classification

Use map lookups instead of sequential regex:

```go
var sourceMap = map[string]Source{
    "bluray":  SourceBluRay,
    "blu-ray": SourceBluRay,
    "bdrip":   SourceBluRay,
    "brrip":   SourceBluRay,
    "web-dl":  SourceWEBDL,
    "webdl":   SourceWEBDL,
    "webrip":  SourceWEBRip,
    "hdtv":    SourceHDTV,
    "pdtv":    SourceHDTV,
    // ... etc
}

var serviceMap = map[string]string{
    "nf":    "Netflix",
    "amzn":  "Amazon",
    "dsnp":  "Disney+",
    "atvp":  "AppleTV+",
    "hmax":  "HBOMax",
    "pcok":  "Peacock",
    // ... etc
}
```

#### 1.5 Title Normalization

Port Radarr's `CleanMovieTitle()`:

```go
func cleanTitle(title string) string {
    s := strings.ToLower(title)

    // Remove articles
    s = stripArticles(s)  // "the", "a", "an"

    // Normalize punctuation
    s = normalizePunctuation(s)  // & -> and, etc.

    // Remove accents
    s = removeAccents(s)  // é -> e, ü -> u

    // Collapse whitespace
    s = strings.Join(strings.Fields(s), " ")

    return s
}
```

### Phase 2: Series Enhancements (v1 scope)

#### 2.1 Multi-Episode Patterns

```go
// S01E01E02, S01E01-E02, S01E01-03
var multiEpRegex = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,2})(?:E(\d{1,2})|-E?(\d{1,2}))`)
```

#### 2.2 Daily Show Format

```go
// Show.2026.01.16.Episode.Title
var dailyRegex = regexp.MustCompile(`(\d{4})\.(\d{2})\.(\d{2})`)
```

#### 2.3 Anime Detection

```go
func isAnimeRelease(name string) bool {
    // Starts with [SubGroup]
    if strings.HasPrefix(name, "[") {
        return true
    }
    // Has CRC32 hash at end
    if crcRegex.MatchString(name) {
        return true
    }
    return false
}
```

### Phase 3: Quality Scoring (v1 scope)

#### 3.1 Quality Hierarchy

```go
type QualityScore int

// Higher = better
var qualityScores = map[string]QualityScore{
    "remux-2160p":  100,
    "bluray-2160p": 95,
    "webdl-2160p":  90,
    "remux-1080p":  85,
    "bluray-1080p": 80,
    "webdl-1080p":  75,
    "webrip-1080p": 70,
    "bluray-720p":  65,
    "webdl-720p":   60,
    "hdtv-1080p":   55,
    "hdtv-720p":    50,
    // ... etc
}

func (i *Info) QualityScore() QualityScore {
    key := fmt.Sprintf("%s-%s", i.Source, i.Resolution)
    if i.IsRemux {
        key = "remux-" + i.Resolution.String()
    }
    return qualityScores[key]
}
```

#### 3.2 Version Ranking

```go
func (i *Info) VersionScore() int {
    score := 0
    if i.Proper { score += 2 }
    if i.Repack { score += 1 }
    // REAL.PROPER = +3, etc.
    return score
}
```

## File Changes

| File | Change |
|------|--------|
| `pkg/release/release.go` | Add new enums and Info fields |
| `pkg/release/parser.go` | Rewrite with anchor-based approach |
| `pkg/release/normalize.go` | New file for title normalization |
| `pkg/release/tokens.go` | New file for token classification maps |
| `pkg/release/parser_test.go` | Expand with corpus-based tests |

## Test Strategy

1. **Keep existing tests** - They should still pass
2. **Add corpus tests** - Parse all 1824 titles, verify no panics
3. **Add golden tests** - Hand-curate ~50 titles with expected output
4. **Add edge case tests** - Daily shows, anime, multi-episode, etc.

## Non-Goals (v2+)

- Language detection
- XEM mapping for anime
- Vector/semantic matching
- LLM-based parsing
- Full Radarr regex parity (they have 20+ patterns, we can start with fewer)

## Success Criteria

1. Parse 95%+ of corpus titles without returning empty Title
2. Correctly extract resolution/source/codec for 99%+ of standard releases
3. Handle REMUX, HDR, Atmos releases
4. Support daily show date format
5. No performance regression (stay under 100µs per parse)

## References

- Radarr parser: `~/code/Radarr/src/NzbDrone.Core/Parser/Parser.cs`
- Our corpus: `testdata/releases.csv`
- Research doc: `docs/scene_matching_research.md` (for context, not implementation)
