# TV Series Parsing Phase 2 Implementation Plan

**Status:** Pending

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend TV series parsing with anime patterns, multi-season packs, special episodes, language detection, and episode title extraction.

**Prerequisites:** Phase 1 complete (multi-episode, season packs, daily shows, alternate formats)

**Architecture:** Continue extending `pkg/release/` with new regex patterns and struct fields. All changes backward-compatible.

**Tech Stack:** Go 1.25+, regexp, TDD with table-driven tests

---

## Task 1: Add Anime-Specific Fields to Info Struct

**Files:**
- Modify: `pkg/release/release.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestInfo_AnimeFields(t *testing.T) {
	info := &Info{
		AbsoluteEpisode: 142,
		ReleaseVersion:  2,
		CRC:             "ABCD1234",
		SubGroup:        "SubsPlease",
	}
	assert.Equal(t, 142, info.AbsoluteEpisode)
	assert.Equal(t, 2, info.ReleaseVersion)
	assert.Equal(t, "ABCD1234", info.CRC)
	assert.Equal(t, "SubsPlease", info.SubGroup)
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add fields to Info struct**

```go
// Anime-specific metadata
AbsoluteEpisode int    // Episode number without season (e.g., 142)
ReleaseVersion  int    // Version number (v2 = 2)
CRC             string // CRC32 checksum from filename
SubGroup        string // Fansub/release group in brackets
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 2: Parse Anime Absolute Episode Numbers

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_AnimeAbsoluteEpisode(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		wantAbsoluteEpisode int
		wantTitle           string
		wantSubGroup        string
	}{
		{
			name:                "Standard anime format",
			input:               "[SubsPlease] Frieren - 24 (1080p) [ABCD1234].mkv",
			wantAbsoluteEpisode: 24,
			wantTitle:           "Frieren",
			wantSubGroup:        "SubsPlease",
		},
		{
			name:                "Three digit episode",
			input:               "[Erai-raws] One Piece - 1089 [1080p][Multiple Subtitle].mkv",
			wantAbsoluteEpisode: 1089,
			wantTitle:           "One Piece",
			wantSubGroup:        "Erai-raws",
		},
		{
			name:                "With version number",
			input:               "[SubsPlease] Show - 05v2 (720p).mkv",
			wantAbsoluteEpisode: 5,
			wantTitle:           "Show",
			wantSubGroup:        "SubsPlease",
		},
		{
			name:                "Batch release single",
			input:               "[SubGroup] Show - 01 [BD 1080p].mkv",
			wantAbsoluteEpisode: 1,
			wantTitle:           "Show",
			wantSubGroup:        "SubGroup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantAbsoluteEpisode, got.AbsoluteEpisode)
			assert.Equal(t, tt.wantTitle, got.Title)
			assert.Equal(t, tt.wantSubGroup, got.SubGroup)
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Implement anime parsing**

Add regex patterns:
```go
// Anime patterns
animeEpRegex     = regexp.MustCompile(`\s-\s(\d{1,4})(?:v\d)?\s`)           // " - 142 " or " - 05v2 "
animeSubGroupRegex = regexp.MustCompile(`^\[([^\]]+)\]`)                     // [SubGroup] at start
animeCRCRegex    = regexp.MustCompile(`\[([A-Fa-f0-9]{8})\]`)               // [ABCD1234]
animeVersionRegex = regexp.MustCompile(`(\d{1,4})v(\d)`)                     // 05v2
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 3: Parse Anime Batch Ranges

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_AnimeBatchRange(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantEpisodes []int
		wantTitle    string
	}{
		{
			name:         "Batch range with hyphen",
			input:        "[SubGroup] Show [01-12] [BD 1080p]",
			wantEpisodes: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantTitle:    "Show",
		},
		{
			name:         "Batch range with tilde",
			input:        "[SubGroup] Show [01~24] [1080p]",
			wantEpisodes: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},
			wantTitle:    "Show",
		},
		{
			name:         "Season batch",
			input:        "[SubGroup] Show S2 [01-13] [1080p]",
			wantEpisodes: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
			wantTitle:    "Show S2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantEpisodes, got.Episodes)
			assert.Equal(t, tt.wantTitle, got.Title)
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Implement batch range parsing**

```go
animeBatchRegex = regexp.MustCompile(`\[(\d{1,4})[-~](\d{1,4})\]`)  // [01-12] or [01~24]
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 4: Add Multi-Season Pack Detection

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_MultiSeasonPack(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantSeasonStart   int
		wantSeasonEnd     int
		wantCompleteSeries bool
	}{
		{
			name:            "Seasons 1-3",
			input:           "Breaking.Bad.Seasons.1-3.1080p.BluRay.x264-GRP",
			wantSeasonStart: 1,
			wantSeasonEnd:   3,
		},
		{
			name:            "S01-S04 format",
			input:           "The.Office.S01-S04.720p.BluRay.x264-GRP",
			wantSeasonStart: 1,
			wantSeasonEnd:   4,
		},
		{
			name:               "Complete Series",
			input:              "Breaking.Bad.The.Complete.Series.1080p.BluRay.x264-GRP",
			wantCompleteSeries: true,
		},
		{
			name:               "Complete Collection",
			input:              "Friends.Complete.Collection.720p.WEB-DL.x264-GRP",
			wantCompleteSeries: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeasonStart, got.SeasonStart)
			assert.Equal(t, tt.wantSeasonEnd, got.SeasonEnd)
			assert.Equal(t, tt.wantCompleteSeries, got.IsCompleteSeries)
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add fields and implement**

New fields:
```go
SeasonStart      int  // First season in multi-season pack
SeasonEnd        int  // Last season in multi-season pack
IsCompleteSeries bool // Complete series pack
```

Regex patterns:
```go
multiSeasonRegex    = regexp.MustCompile(`(?i)Seasons?[\s.]?(\d{1,2})[-–](\d{1,2})`)
multiSeasonSRegex   = regexp.MustCompile(`(?i)S(\d{1,2})-S?(\d{1,2})`)
completeSeriesRegex = regexp.MustCompile(`(?i)Complete[\s.](?:Series|Collection)`)
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 5: Add Special Episode Detection

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_SpecialEpisode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSpecial bool
		wantType    string
		wantSeason  int
		wantEpisode int
	}{
		{
			name:        "S00E01 special",
			input:       "Show.S00E01.Behind.The.Scenes.720p.WEB-DL.x264-GRP",
			wantSpecial: true,
			wantType:    "special",
			wantSeason:  0,
			wantEpisode: 1,
		},
		{
			name:        "OVA marker",
			input:       "[SubGroup] Show - OVA [1080p].mkv",
			wantSpecial: true,
			wantType:    "OVA",
		},
		{
			name:        "ONA marker",
			input:       "[SubGroup] Show - ONA 01 [720p].mkv",
			wantSpecial: true,
			wantType:    "ONA",
		},
		{
			name:        "Special marker",
			input:       "Show.Special.Christmas.Episode.1080p.HDTV.x264-GRP",
			wantSpecial: true,
			wantType:    "special",
		},
		{
			name:        "Pilot episode",
			input:       "Show.Pilot.720p.HDTV.x264-GRP",
			wantSpecial: true,
			wantType:    "pilot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSpecial, got.IsSpecial)
			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, got.SpecialType)
			}
			if tt.wantSeason > 0 || tt.wantEpisode > 0 {
				assert.Equal(t, tt.wantSeason, got.Season)
				assert.Equal(t, tt.wantEpisode, got.Episode)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add fields and implement**

New fields:
```go
IsSpecial   bool   // Special episode (S00, OVA, etc.)
SpecialType string // "special", "OVA", "ONA", "pilot"
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 6: Add Language and Subtitle Detection

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_LanguageSubtitle(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantLanguages []string
		wantSubtitles []string
		wantMulti     bool
		wantDual      bool
	}{
		{
			name:          "MULTi language",
			input:         "Movie.2024.MULTi.1080p.BluRay.x264-GRP",
			wantMulti:     true,
		},
		{
			name:          "DUAL audio",
			input:         "Movie.2024.DUAL.1080p.BluRay.x264-GRP",
			wantDual:      true,
		},
		{
			name:          "FRENCH language tag",
			input:         "Movie.2024.FRENCH.1080p.BluRay.x264-GRP",
			wantLanguages: []string{"French"},
		},
		{
			name:          "GERMAN language tag",
			input:         "Movie.2024.GERMAN.1080p.BluRay.x264-GRP",
			wantLanguages: []string{"German"},
		},
		{
			name:          "SUBBED marker",
			input:         "Movie.2024.SUBBED.1080p.WEB-DL.x264-GRP",
			wantSubtitles: []string{"English"},
		},
		{
			name:          "Multiple subtitles",
			input:         "[SubGroup] Show - 01 [1080p][Multiple Subtitle].mkv",
			wantSubtitles: []string{"Multiple"},
		},
		{
			name:          "VOSTFR subtitle",
			input:         "Movie.2024.VOSTFR.1080p.WEB-DL.x264-GRP",
			wantSubtitles: []string{"French"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if tt.wantMulti {
				assert.True(t, got.IsMultiLanguage)
			}
			if tt.wantDual {
				assert.True(t, got.IsDualAudio)
			}
			if len(tt.wantLanguages) > 0 {
				assert.Equal(t, tt.wantLanguages, got.Languages)
			}
			if len(tt.wantSubtitles) > 0 {
				assert.Equal(t, tt.wantSubtitles, got.Subtitles)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add fields and implement**

New fields:
```go
Languages       []string // Detected languages
Subtitles       []string // Detected subtitle languages
IsMultiLanguage bool     // MULTi tag present
IsDualAudio     bool     // DUAL tag present
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 7: Add Episode Title Extraction

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_EpisodeTitle(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantEpisodeTitle string
		wantTitle        string
	}{
		{
			name:             "Episode title after episode number",
			input:            "Breaking.Bad.S01E05.Gray.Matter.720p.BluRay.x264-GRP",
			wantEpisodeTitle: "Gray Matter",
			wantTitle:        "Breaking Bad",
		},
		{
			name:             "Multi-word episode title",
			input:            "Game.of.Thrones.S01E09.Baelor.1080p.BluRay.x264-GRP",
			wantEpisodeTitle: "Baelor",
			wantTitle:        "Game of Thrones",
		},
		{
			name:             "Episode title with numbers",
			input:            "Lost.S04E05.The.Constant.720p.BluRay.x264-GRP",
			wantEpisodeTitle: "The Constant",
			wantTitle:        "Lost",
		},
		{
			name:             "No episode title",
			input:            "Show.S01E05.720p.BluRay.x264-GRP",
			wantEpisodeTitle: "",
			wantTitle:        "Show",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantEpisodeTitle, got.EpisodeTitle)
			assert.Equal(t, tt.wantTitle, got.Title)
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add field and implement**

New field:
```go
EpisodeTitle string // Episode title if present
```

Logic: Extract text between episode number and quality/source markers.

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 8: Add Advanced Version Detection

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

```go
func TestParse_AdvancedVersioning(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProper   bool
		wantRepack   bool
		wantVersion  int
		wantInternal bool
	}{
		{
			name:        "PROPER2",
			input:       "Show.S01E05.PROPER2.720p.HDTV.x264-GRP",
			wantProper:  true,
			wantVersion: 2,
		},
		{
			name:        "REPACK2",
			input:       "Show.S01E05.REPACK2.720p.HDTV.x264-GRP",
			wantRepack:  true,
			wantVersion: 2,
		},
		{
			name:         "iNTERNAL release",
			input:        "Show.S01E05.iNTERNAL.720p.HDTV.x264-GRP",
			wantInternal: true,
		},
		{
			name:        "REAL PROPER",
			input:       "Show.S01E05.REAL.PROPER.720p.HDTV.x264-GRP",
			wantProper:  true,
			wantVersion: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantProper, got.Proper)
			assert.Equal(t, tt.wantRepack, got.Repack)
			if tt.wantVersion > 0 {
				assert.Equal(t, tt.wantVersion, got.ProperVersion)
			}
			assert.Equal(t, tt.wantInternal, got.IsInternal)
		})
	}
}
```

**Step 2: Run test to verify it fails**

**Step 3: Add fields and implement**

New fields:
```go
ProperVersion int  // PROPER2 = 2, REAL PROPER = 2
IsInternal    bool // iNTERNAL release
```

**Step 4: Run test to verify it passes**

**Step 5: Commit**

---

## Task 9: Update CLI Parse Command for Phase 2 Fields

**Files:**
- Modify: `cmd/arrgo/parse.go`

**Step 1: Add display for new fields**

```go
// Anime fields
if info.AbsoluteEpisode > 0 {
    fmt.Printf("Absolute Ep: %d\n", info.AbsoluteEpisode)
}
if info.SubGroup != "" {
    fmt.Printf("Sub Group:   %s\n", info.SubGroup)
}
if info.ReleaseVersion > 1 {
    fmt.Printf("Version:     v%d\n", info.ReleaseVersion)
}

// Multi-season
if info.SeasonEnd > 0 {
    fmt.Printf("Seasons:     %d-%d\n", info.SeasonStart, info.SeasonEnd)
}
if info.IsCompleteSeries {
    fmt.Printf("Complete Series: yes\n")
}

// Specials
if info.IsSpecial {
    fmt.Printf("Special:     %s\n", info.SpecialType)
}

// Language
if info.IsMultiLanguage {
    fmt.Printf("Multi-Lang:  yes\n")
}
if info.IsDualAudio {
    fmt.Printf("Dual Audio:  yes\n")
}

// Episode title
if info.EpisodeTitle != "" {
    fmt.Printf("Ep Title:    %s\n", info.EpisodeTitle)
}
```

**Step 2: Update JSON output struct**

**Step 3: Test manually**

**Step 4: Commit**

---

## Task 10: Add Golden Tests for Phase 2 Patterns

**Files:**
- Modify: `pkg/release/golden_test.go`

**Step 1: Add anime golden tests**

```go
// === Anime Releases ===
{
    name:  "Anime standard format",
    input: "[SubsPlease] Frieren - Beyond Journeys End - 24 (1080p) [ABCD1234].mkv",
    // ... expected fields
},
{
    name:  "Anime batch release",
    input: "[Judas] Vinland Saga [S01] [BD 1080p][HEVC x265 10bit][Eng-Subs]",
    // ... expected fields
},
```

**Step 2: Add multi-season golden tests**

**Step 3: Add special episode golden tests**

**Step 4: Add language detection golden tests**

**Step 5: Run all golden tests**

**Step 6: Commit**

---

## Task 11: Run Full Test Suite

**Step 1: Run `task check`**

**Step 2: Fix any regressions**

**Step 3: Update snapshots if needed**

**Step 4: Final commit**

---

## Summary

Phase 2 adds:

1. **Anime support**: Absolute episodes, fansub groups, CRC, version numbers, batch ranges
2. **Multi-season packs**: `Seasons 1-3`, `S01-S04`, Complete Series
3. **Special episodes**: S00, OVA, ONA, pilots, specials
4. **Language detection**: MULTi, DUAL, language tags, subtitle markers
5. **Episode titles**: Extract episode titles from release names
6. **Advanced versioning**: PROPER2, REPACK2, iNTERNAL, REAL PROPER

All changes backward-compatible with Phase 1 and movie parsing.
