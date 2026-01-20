# TV Series Parsing Phase 1 Implementation Plan

**Status:** Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend the release parser to handle standard scene release naming conventions for TV series, closing critical gaps for multi-episode releases, alternate formats, season packs, and daily shows.

**Architecture:** Extend `pkg/release/` with new regex patterns and struct fields. The `Info` struct gains slice fields for multi-episode support and boolean flags for season packs. All changes are backward-compatible - existing movie parsing remains unchanged.

**Tech Stack:** Go 1.25+, regexp, TDD with table-driven tests

**Note:** This implementation is derived from analyzing actual release names from indexers and standard scene naming conventions documented in public sources (e.g., Wikipedia's "scene release" article, public indexer APIs).

---

## Task 1: Add Multi-Episode Fields to Info Struct

**Files:**
- Modify: `pkg/release/release.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestInfo_Episodes_Slice(t *testing.T) {
	info := &Info{
		Episodes: []int{5, 6, 7},
	}
	require.Len(t, info.Episodes, 3)
	assert.Equal(t, 5, info.Episodes[0])
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestInfo_Episodes_Slice -v`
Expected: FAIL with "info.Episodes undefined"

**Step 3: Write minimal implementation**

Modify `pkg/release/release.go` - add new fields to the Info struct:

```go
// Info contains parsed release information.
type Info struct {
	Title      string
	Year       int
	Season     int
	Episode    int    // Primary episode (first in range), kept for backward compatibility
	Episodes   []int  // All episodes in release (e.g., [5,6,7] for S01E05-E07)
	DailyDate  string // Daily show date in YYYY-MM-DD format (e.g., "2026-01-16")
	Resolution Resolution
	Source     Source
	Codec      Codec
	Group      string
	Proper     bool
	Repack     bool

	// Extended metadata
	HDR     HDRFormat
	Audio   AudioCodec
	IsRemux bool
	Edition string // "Directors Cut", "Extended", "IMAX", etc.
	Service string // Streaming service: NF, AMZN, DSNP, etc.

	// Season pack detection
	IsCompleteSeason bool // Complete season release (e.g., "Season 01", "S01")
	IsSplitSeason    bool // Split/partial season (e.g., "Season 1 Part 2")
	SplitPart        int  // Part number for split seasons

	// Normalized title for matching
	CleanTitle string
}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestInfo_Episodes_Slice -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/release.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
feat(release): add multi-episode and season pack fields to Info struct

Add Episodes []int slice for multi-episode releases, plus IsCompleteSeason,
IsSplitSeason, and SplitPart fields for season pack detection.
Keeps Episode int for backward compatibility.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add Alternate Episode Format Regex (1x05)

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_AlternateEpisodeFormats(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSeason  int
		wantEpisode int
	}{
		{
			name:        "1x05 format",
			input:       "Show.1x05.720p.HDTV.x264-GRP",
			wantSeason:  1,
			wantEpisode: 5,
		},
		{
			name:        "12x24 format double digit",
			input:       "Show.12x24.Episode.Title.1080p.WEB-DL.x264-GRP",
			wantSeason:  12,
			wantEpisode: 24,
		},
		{
			name:        "s01.05 format with dot",
			input:       "Show.s01.05.720p.HDTV.x264-GRP",
			wantSeason:  1,
			wantEpisode: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantEpisode, got.Episode, "Episode")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_AlternateEpisodeFormats -v`
Expected: FAIL - "1x05 format" case will have Season=0, Episode=0

**Step 3: Write minimal implementation**

Modify `pkg/release/parser.go` - add new regex patterns:

```go
var (
	yearRegex        = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	dailyRegex       = regexp.MustCompile(`\b(20\d{2})\.(\d{2})\.(\d{2})\b`)
	seasonEpRegex    = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,2})`)
	altSeasonEpRegex = regexp.MustCompile(`(?i)(\d{1,2})x(\d{1,2})`)                  // 1x05 format
	dotSeasonEpRegex = regexp.MustCompile(`(?i)s(\d{1,2})\.(\d{1,2})(?:\.|$|\s)`)     // s01.05 format
	titleMarkerRegex = regexp.MustCompile(`(?i)\b(19|20)\d{2}\b|\b\d{3,4}p\b|\bS\d{1,2}E\d{1,2}\b|\b\d{1,2}x\d{1,2}\b|\b4K\b|\bUHD\b`)
	// ... existing patterns ...
)
```

Then update the season/episode parsing section in `Parse()`:

```go
	// Season/Episode - try multiple formats in priority order
	if matches := seasonEpRegex.FindStringSubmatch(normalized); len(matches) == 3 {
		// Standard S01E01 format
		if season, err := strconv.Atoi(matches[1]); err == nil {
			info.Season = season
		}
		if episode, err := strconv.Atoi(matches[2]); err == nil {
			info.Episode = episode
			info.Episodes = []int{episode}
		}
	} else if matches := altSeasonEpRegex.FindStringSubmatch(normalized); len(matches) == 3 {
		// Alternate 1x05 format
		if season, err := strconv.Atoi(matches[1]); err == nil {
			info.Season = season
		}
		if episode, err := strconv.Atoi(matches[2]); err == nil {
			info.Episode = episode
			info.Episodes = []int{episode}
		}
	} else if matches := dotSeasonEpRegex.FindStringSubmatch(normalized); len(matches) == 3 {
		// Dot-separated s01.05 format
		if season, err := strconv.Atoi(matches[1]); err == nil {
			info.Season = season
		}
		if episode, err := strconv.Atoi(matches[2]); err == nil {
			info.Episode = episode
			info.Episodes = []int{episode}
		}
	}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_AlternateEpisodeFormats -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
feat(release): add 1x05 and s01.05 episode format support

Add alternate season/episode patterns commonly used in scene releases.
Supports both 1x05 (slash) and s01.05 (dot) formats alongside standard
SxxExx.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add Multi-Episode Range Parsing (S01E05-06, S01E05E06)

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_MultiEpisode(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantSeason   int
		wantEpisode  int // First episode
		wantEpisodes []int
	}{
		{
			name:         "S01E05-06 range with hyphen",
			input:        "Show.S01E05-06.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6},
		},
		{
			name:         "S01E05-E06 range with E prefix",
			input:        "Show.S01E05-E06.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6},
		},
		{
			name:         "S01E05E06 sequential",
			input:        "Show.S01E05E06.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6},
		},
		{
			name:         "S01E05E06E07 triple episode",
			input:        "Show.S01E05E06E07.1080p.WEB-DL.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6, 7},
		},
		{
			name:         "S01E01-03 range spanning 3",
			input:        "Show.S01E01-03.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  1,
			wantEpisodes: []int{1, 2, 3},
		},
		{
			name:         "Single episode still works",
			input:        "Show.S01E05.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantEpisode, got.Episode, "Episode")
			assert.Equal(t, tt.wantEpisodes, got.Episodes, "Episodes")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_MultiEpisode -v`
Expected: FAIL - multi-episode cases will have Episodes with wrong length

**Step 3: Write minimal implementation**

Add new regex patterns and helper functions to `pkg/release/parser.go`:

```go
var (
	// ... existing patterns ...
	// Multi-episode patterns
	multiEpRangeRegex = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,2})-E?(\d{1,2})`)  // S01E05-06 or S01E05-E06
	multiEpSeqRegex   = regexp.MustCompile(`(?i)S(\d{1,2})((?:E\d{1,2})+)`)          // S01E05E06E07
)

// parseEpisodeSequence extracts episode numbers from patterns like "E05E06E07"
func parseEpisodeSequence(s string) []int {
	re := regexp.MustCompile(`(?i)E(\d{1,2})`)
	matches := re.FindAllStringSubmatch(s, -1)
	episodes := make([]int, 0, len(matches))
	for _, m := range matches {
		if ep, err := strconv.Atoi(m[1]); err == nil {
			episodes = append(episodes, ep)
		}
	}
	return episodes
}

// expandRange creates a slice from start to end inclusive
func expandRange(start, end int) []int {
	if end < start {
		return []int{start}
	}
	episodes := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		episodes = append(episodes, i)
	}
	return episodes
}
```

Update the season/episode parsing in `Parse()`:

```go
	// Season/Episode - try multi-episode formats first, then single
	if matches := multiEpRangeRegex.FindStringSubmatch(normalized); len(matches) == 4 {
		// Range format: S01E05-06 or S01E05-E06
		if season, err := strconv.Atoi(matches[1]); err == nil {
			info.Season = season
		}
		start, _ := strconv.Atoi(matches[2])
		end, _ := strconv.Atoi(matches[3])
		info.Episode = start
		info.Episodes = expandRange(start, end)
	} else if matches := multiEpSeqRegex.FindStringSubmatch(normalized); len(matches) == 3 {
		// Sequential format: S01E05E06E07
		if season, err := strconv.Atoi(matches[1]); err == nil {
			info.Season = season
		}
		info.Episodes = parseEpisodeSequence(matches[2])
		if len(info.Episodes) > 0 {
			info.Episode = info.Episodes[0]
		}
	} else if matches := seasonEpRegex.FindStringSubmatch(normalized); len(matches) == 3 {
		// Standard S01E01 format (single episode)
		// ... existing code ...
	}
	// ... rest of formats ...
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_MultiEpisode -v`
Expected: PASS

**Step 5: Run all tests to check for regressions**

Run: `task test`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
feat(release): add multi-episode range and sequence parsing

Support S01E05-06, S01E05-E06 range formats and S01E05E06E07 sequential
formats. The Episodes slice contains all episode numbers, while Episode
retains the first for backward compatibility.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add Season Pack Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_SeasonPack(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		wantSeason         int
		wantCompleteSeason bool
		wantSplitSeason    bool
		wantSplitPart      int
	}{
		{
			name:               "Season 01 pack",
			input:              "Show.Season.01.1080p.BluRay.x264-GRP",
			wantSeason:         1,
			wantCompleteSeason: true,
		},
		{
			name:               "S01 pack no episodes",
			input:              "Show.S01.1080p.BluRay.x264-GRP",
			wantSeason:         1,
			wantCompleteSeason: true,
		},
		{
			name:               "Complete Season",
			input:              "Show.Complete.Season.2.720p.WEB-DL.x264-GRP",
			wantSeason:         2,
			wantCompleteSeason: true,
		},
		{
			name:            "Season 1 Part 2",
			input:           "Show.Season.1.Part.2.1080p.WEB-DL.x264-GRP",
			wantSeason:      1,
			wantSplitSeason: true,
			wantSplitPart:   2,
		},
		{
			name:            "S01 Vol 1",
			input:           "Show.S01.Vol.1.1080p.WEB-DL.x264-GRP",
			wantSeason:      1,
			wantSplitSeason: true,
			wantSplitPart:   1,
		},
		{
			name:               "Regular episode not a pack",
			input:              "Show.S01E05.720p.HDTV.x264-GRP",
			wantSeason:         1,
			wantCompleteSeason: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantCompleteSeason, got.IsCompleteSeason, "IsCompleteSeason")
			assert.Equal(t, tt.wantSplitSeason, got.IsSplitSeason, "IsSplitSeason")
			assert.Equal(t, tt.wantSplitPart, got.SplitPart, "SplitPart")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_SeasonPack -v`
Expected: FAIL - IsCompleteSeason and IsSplitSeason will be false

**Step 3: Write minimal implementation**

Add new regex patterns to `pkg/release/parser.go`:

```go
var (
	// ... existing patterns ...
	// Season pack patterns
	seasonPackRegex     = regexp.MustCompile(`(?i)(?:Complete\s+)?Season[\s.]?(\d{1,2})(?:\s|\.|\b)`)
	seasonOnlyRegex     = regexp.MustCompile(`(?i)\bS(\d{1,2})(?:\b|\.|\s)(?!E\d)`)  // S01 without E##
	splitSeasonRegex    = regexp.MustCompile(`(?i)(?:Season[\s.]?(\d{1,2})|S(\d{1,2}))[\s.]+(?:Part|Vol)[\s.]?(\d{1,2})`)
)
```

Add season pack detection after episode parsing in `Parse()`:

```go
	// Season pack detection (only if no episode info found)
	if info.Episode == 0 && len(info.Episodes) == 0 {
		// Check for split season first (more specific)
		if matches := splitSeasonRegex.FindStringSubmatch(normalized); len(matches) == 4 {
			season := matches[1]
			if season == "" {
				season = matches[2]
			}
			if s, err := strconv.Atoi(season); err == nil {
				info.Season = s
			}
			if part, err := strconv.Atoi(matches[3]); err == nil {
				info.SplitPart = part
			}
			info.IsSplitSeason = true
		} else if matches := seasonPackRegex.FindStringSubmatch(normalized); len(matches) == 2 {
			// Complete season pack: "Season 01" or "Complete Season 1"
			if season, err := strconv.Atoi(matches[1]); err == nil {
				info.Season = season
			}
			info.IsCompleteSeason = true
		} else if matches := seasonOnlyRegex.FindStringSubmatch(normalized); len(matches) == 2 {
			// S01 without episode number
			if season, err := strconv.Atoi(matches[1]); err == nil {
				info.Season = season
			}
			info.IsCompleteSeason = true
		}
	}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_SeasonPack -v`
Expected: PASS

**Step 5: Run all tests to check for regressions**

Run: `task test`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
feat(release): add season pack and split season detection

Detect complete season packs (Season 01, S01, Complete Season) and split
seasons (Season 1 Part 2, S01 Vol 1). Sets IsCompleteSeason, IsSplitSeason,
and SplitPart fields appropriately.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Expand Daily Show Date Formats

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_DailyShowFormats(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantDailyDate string
		wantYear      int
	}{
		{
			name:          "YYYY.MM.DD standard",
			input:         "Show.2026.01.16.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "YYYY-MM-DD with hyphens",
			input:         "Show.2026-01-16.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "YYYYMMDD compact",
			input:         "Show.20260116.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "DD.MM.YYYY European",
			input:         "Show.16.01.2026.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "16 Jan 2026 word month",
			input:         "Show.16.Jan.2026.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "Jan 16 2026 US word format",
			input:         "Show.Jan.16.2026.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "Movie with year (not daily)",
			input:         "Movie.2024.1080p.BluRay.x264-GRP",
			wantDailyDate: "",
			wantYear:      2024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantDailyDate, got.DailyDate, "DailyDate")
			assert.Equal(t, tt.wantYear, got.Year, "Year")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_DailyShowFormats -v`
Expected: FAIL - new formats will not be detected

**Step 3: Write minimal implementation**

Update `pkg/release/parser.go`:

```go
var (
	// ... existing patterns ...
	// Daily show patterns (multiple formats)
	dailyYMDDotRegex     = regexp.MustCompile(`\b(20\d{2})\.(\d{2})\.(\d{2})\b`)           // 2026.01.16
	dailyYMDHyphenRegex  = regexp.MustCompile(`\b(20\d{2})-(\d{2})-(\d{2})\b`)             // 2026-01-16
	dailyYMDCompactRegex = regexp.MustCompile(`\b(20\d{2})(\d{2})(\d{2})\b`)               // 20260116
	dailyDMYRegex        = regexp.MustCompile(`\b(\d{2})\.(\d{2})\.(20\d{2})\b`)           // 16.01.2026
	dailyWordMonthRegex  = regexp.MustCompile(`(?i)\b(\d{1,2})[\s.]?(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\s.]?(20\d{2})\b`)  // 16 Jan 2026
	dailyUSWordRegex     = regexp.MustCompile(`(?i)\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\s.]?(\d{1,2})[\s.,]+(20\d{2})\b`) // Jan 16, 2026
)

// monthToNumber converts month abbreviation to zero-padded number string
func monthToNumber(month string) string {
	mapping := map[string]string{
		"jan": "01", "feb": "02", "mar": "03", "apr": "04",
		"may": "05", "jun": "06", "jul": "07", "aug": "08",
		"sep": "09", "oct": "10", "nov": "11", "dec": "12",
	}
	return mapping[strings.ToLower(month)]
}

// isValidDate checks if month and day values are reasonable
func isValidDate(month, day string) bool {
	return month >= "01" && month <= "12" && day >= "01" && day <= "31"
}

// parseDailyDate detects daily show date formats from release name.
// Returns date in YYYY-MM-DD format if valid, empty string otherwise.
func parseDailyDate(name string) string {
	// Try YYYY.MM.DD format
	if matches := dailyYMDDotRegex.FindStringSubmatch(name); len(matches) == 4 {
		if isValidDate(matches[2], matches[3]) {
			return matches[1] + "-" + matches[2] + "-" + matches[3]
		}
	}

	// Try YYYY-MM-DD format
	if matches := dailyYMDHyphenRegex.FindStringSubmatch(name); len(matches) == 4 {
		if isValidDate(matches[2], matches[3]) {
			return matches[1] + "-" + matches[2] + "-" + matches[3]
		}
	}

	// Try YYYYMMDD compact format
	if matches := dailyYMDCompactRegex.FindStringSubmatch(name); len(matches) == 4 {
		if isValidDate(matches[2], matches[3]) {
			return matches[1] + "-" + matches[2] + "-" + matches[3]
		}
	}

	// Try DD.MM.YYYY European format
	if matches := dailyDMYRegex.FindStringSubmatch(name); len(matches) == 4 {
		day, month, year := matches[1], matches[2], matches[3]
		// Swap if month > 12 (likely day/month swapped)
		if month > "12" && day <= "12" {
			day, month = month, day
		}
		if isValidDate(month, day) {
			return year + "-" + month + "-" + day
		}
	}

	// Try "16 Jan 2026" format
	if matches := dailyWordMonthRegex.FindStringSubmatch(name); len(matches) == 4 {
		day := matches[1]
		if len(day) == 1 {
			day = "0" + day
		}
		month := monthToNumber(matches[2])
		year := matches[3]
		if month != "" && isValidDate(month, day) {
			return year + "-" + month + "-" + day
		}
	}

	// Try "Jan 16, 2026" US format
	if matches := dailyUSWordRegex.FindStringSubmatch(name); len(matches) == 4 {
		month := monthToNumber(matches[1])
		day := matches[2]
		if len(day) == 1 {
			day = "0" + day
		}
		year := matches[3]
		if month != "" && isValidDate(month, day) {
			return year + "-" + month + "-" + day
		}
	}

	return ""
}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_DailyShowFormats -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
feat(release): expand daily show date format support

Add support for multiple date formats commonly used in daily shows:
- YYYY-MM-DD (hyphen-separated)
- YYYYMMDD (compact)
- DD.MM.YYYY (European)
- 16 Jan 2026 (word month)
- Jan 16, 2026 (US word format)

Includes date validation and day/month swap for ambiguous European dates.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Fix Audio Detection Gaps (DD.5.1, DD.2.0)

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_AudioGaps(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantAudio AudioCodec
	}{
		{
			name:      "DD.5.1 with dots",
			input:     "Movie.2024.1080p.BluRay.DD.5.1.x264-GRP",
			wantAudio: AudioAC3,
		},
		{
			name:      "DD.2.0 stereo",
			input:     "Movie.2024.1080p.BluRay.DD.2.0.x264-GRP",
			wantAudio: AudioAC3,
		},
		{
			name:      "DD 5.1 with space",
			input:     "Movie.2024.1080p.BluRay.DD 5.1.x264-GRP",
			wantAudio: AudioAC3,
		},
		{
			name:      "DD+ 5.1 (should be EAC3)",
			input:     "Movie.2024.1080p.WEB-DL.DD+.5.1.x264-GRP",
			wantAudio: AudioEAC3,
		},
		{
			name:      "Dolby Digital explicit",
			input:     "Movie.2024.1080p.BluRay.Dolby.Digital.5.1.x264-GRP",
			wantAudio: AudioAC3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantAudio, got.Audio, "Audio")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_AudioGaps -v`
Expected: FAIL - DD.5.1 and DD.2.0 cases will return AudioUnknown

**Step 3: Write minimal implementation**

Update `parseAudio` function in `pkg/release/parser.go` to use regex for flexible DD matching:

```go
var (
	ddPlusRegex = regexp.MustCompile(`(?i)\bdd\+[\s.]?\d`)
	ddRegex     = regexp.MustCompile(`(?i)\bdd[\s.]?\d\.\d`)
)

func parseAudio(name string) AudioCodec {
	lower := strings.ToLower(name)

	// Check Atmos first (can be combined with TrueHD or DD+)
	if strings.Contains(lower, "atmos") {
		return AudioAtmos
	}

	// DTS-HD MA before plain DTS
	if strings.Contains(lower, "dts-hd") || strings.Contains(lower, "dts hd") || strings.Contains(lower, "dtshdma") {
		return AudioDTSHD
	}

	// TrueHD
	if strings.Contains(lower, "truehd") {
		return AudioTrueHD
	}

	// DD+ / DDP / EAC3 before DD / AC3
	if containsAny(lower, "ddp", "dd+", "eac3", "e-ac3") || ddPlusRegex.MatchString(lower) {
		return AudioEAC3
	}

	// DD / AC3 - improved pattern matching for dd5.1, dd.5.1, dd 5.1
	if containsAny(lower, "ac3", "dolby digital") || ddRegex.MatchString(lower) {
		return AudioAC3
	}

	// Plain DTS
	if strings.Contains(lower, "dts") {
		return AudioDTS
	}

	// Lossless
	if strings.Contains(lower, "flac") {
		return AudioFLAC
	}

	// Others
	if strings.Contains(lower, "opus") {
		return AudioOpus
	}
	if strings.Contains(lower, "aac") {
		return AudioAAC
	}

	return AudioUnknown
}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_AudioGaps -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
fix(release): improve DD audio detection for dot-separated patterns

Fix detection of DD.5.1, DD.2.0, and similar patterns where Dolby Digital
is followed by dots instead of no separator. Uses regex to match the
pattern \bdd[\s.]?\d\.\d for flexible whitespace/dot handling.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Fix DoVi HDR Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_DoViHDR(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantHDR HDRFormat
	}{
		{
			name:    "DoVi variant",
			input:   "Movie.2024.2160p.UHD.BluRay.DoVi.HDR10.x265-GRP",
			wantHDR: DolbyVision,
		},
		{
			name:    "DOVI uppercase",
			input:   "Movie.2024.2160p.UHD.BluRay.DOVI.x265-GRP",
			wantHDR: DolbyVision,
		},
		{
			name:    "DV standard",
			input:   "Movie.2024.2160p.WEB-DL.DV.H265-GRP",
			wantHDR: DolbyVision,
		},
		{
			name:    "Dolby.Vision with dot",
			input:   "Movie.2024.2160p.WEB-DL.Dolby.Vision.H265-GRP",
			wantHDR: DolbyVision,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantHDR, got.HDR, "HDR")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_DoViHDR -v`
Expected: FAIL - DoVi case will return HDR10 instead of DolbyVision

**Step 3: Write minimal implementation**

Update the HDR regex in `pkg/release/parser.go`:

```go
var (
	// ... other patterns ...
	hdrRegex = regexp.MustCompile(`(?i)\bHDR10\+|\b(HDR10Plus|HDR10|HDR|DV|DoVi|DOVI|Dolby\.?Vision|HLG)\b`)
)
```

Update the `parseHDR` function to handle DoVi:

```go
func parseHDR(name string) HDRFormat {
	matches := hdrRegex.FindAllString(name, -1)
	if len(matches) == 0 {
		return HDRNone
	}

	// Check in priority order (most specific first)
	// DolbyVision takes priority as DV releases often also have HDR metadata
	for _, m := range matches {
		lower := strings.ToLower(m)
		switch {
		case lower == "dv" || lower == "dovi" || strings.Contains(lower, "dolby"):
			return DolbyVision
		}
	}
	for _, m := range matches {
		lower := strings.ToLower(m)
		switch {
		case lower == "hdr10+" || lower == "hdr10plus":
			return HDR10Plus
		case lower == "hdr10":
			return HDR10
		case lower == "hlg":
			return HLG
		case lower == "hdr":
			return HDRGeneric
		}
	}
	return HDRNone
}
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_DoViHDR -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
fix(release): add DoVi and DOVI to HDR detection patterns

Add DoVi and DOVI variants to the HDR regex pattern. These are common
alternative spellings for Dolby Vision in release names.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Update Title Marker Regex for New Patterns

**Files:**
- Modify: `pkg/release/parser.go`
- Test: `pkg/release/release_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/release_test.go`:

```go
func TestParse_TitleWithNewFormats(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
	}{
		{
			name:      "Title with 1x05 format",
			input:     "Some.Show.1x05.Episode.Title.720p.HDTV.x264-GRP",
			wantTitle: "Some Show",
		},
		{
			name:      "Title with Season pack",
			input:     "Some.Show.Season.01.1080p.BluRay.x264-GRP",
			wantTitle: "Some Show",
		},
		{
			name:      "Title with S01 pack",
			input:     "Some.Show.S01.1080p.BluRay.x264-GRP",
			wantTitle: "Some Show",
		},
		{
			name:      "Title with daily date",
			input:     "Daily.Show.2026.01.16.Episode.720p.HDTV.x264-GRP",
			wantTitle: "Daily Show",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantTitle, got.Title, "Title")
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test -- -run TestParse_TitleWithNewFormats -v`
Expected: Some tests may FAIL if title extraction doesn't stop at new markers

**Step 3: Write minimal implementation**

Update `titleMarkerRegex` in `pkg/release/parser.go`:

```go
var (
	// Title extraction stops at these markers
	titleMarkerRegex = regexp.MustCompile(`(?i)\b(19|20)\d{2}\b|\b\d{3,4}p\b|\bS\d{1,2}E\d{1,2}\b|\bS\d{1,2}\b|\b\d{1,2}x\d{1,2}\b|\bSeason[\s.]\d{1,2}\b|\b4K\b|\bUHD\b`)
)
```

**Step 4: Run test to verify it passes**

Run: `task test -- -run TestParse_TitleWithNewFormats -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "$(cat <<'EOF'
feat(release): update title marker regex for TV formats

Add 1x05, S01 (without episode), and Season patterns to title marker
regex so title extraction stops correctly for these TV-specific formats.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Add Golden Tests for New TV Patterns

**Files:**
- Modify: `pkg/release/golden_test.go`

**Step 1: Add new golden test cases**

Add TV series test cases to `goldenCases` in `pkg/release/golden_test.go`:

```go
	// === TV Series Multi-Episode ===
	{
		name:       "Multi-episode range S01E05-06",
		input:      "Breaking.Bad.S01E05-06.720p.BluRay.x264-DEMAND",
		resolution: Resolution720p,
		source:     SourceBluRay,
		codec:      CodecX264,
		title:      "Breaking Bad",
		group:      "DEMAND",
	},
	{
		name:       "Multi-episode sequential S01E05E06",
		input:      "Game.of.Thrones.S01E05E06.1080p.BluRay.x264-ROVERS",
		resolution: Resolution1080p,
		source:     SourceBluRay,
		codec:      CodecX264,
		title:      "Game of Thrones",
		group:      "ROVERS",
	},
	{
		name:       "Alternate format 1x05",
		input:      "The.Simpsons.12x05.Episode.Title.720p.HDTV.x264-LOL",
		resolution: Resolution720p,
		source:     SourceHDTV,
		codec:      CodecX264,
		title:      "The Simpsons",
		group:      "LOL",
	},

	// === Season Packs ===
	{
		name:       "Full season pack",
		input:      "Stranger.Things.Season.01.1080p.NF.WEB-DL.DDP5.1.x264-NTb",
		resolution: Resolution1080p,
		source:     SourceWEBDL,
		codec:      CodecX264,
		audio:      AudioEAC3,
		title:      "Stranger Things",
		group:      "NTb",
		service:    "Netflix",
	},
	{
		name:       "Complete season pack",
		input:      "The.Office.US.Complete.Season.3.720p.BluRay.x264-DEMAND",
		resolution: Resolution720p,
		source:     SourceBluRay,
		codec:      CodecX264,
		title:      "The Office US",
		group:      "DEMAND",
	},
	{
		name:       "S01 pack no episodes",
		input:      "House.of.the.Dragon.S01.2160p.HMAX.WEB-DL.DDP5.1.Atmos.DV.H.265-FLUX",
		resolution: Resolution2160p,
		source:     SourceWEBDL,
		codec:      CodecX265,
		hdr:        DolbyVision,
		audio:      AudioAtmos,
		title:      "House of the Dragon",
		group:      "FLUX",
		service:    "HBO Max",
	},

	// === Daily Shows ===
	{
		name:       "Daily show European date",
		input:      "Late.Night.16.01.2026.720p.HDTV.x264-SORNY",
		resolution: Resolution720p,
		source:     SourceHDTV,
		codec:      CodecX264,
		title:      "Late Night",
		group:      "SORNY",
	},
	{
		name:       "Daily show compact date",
		input:      "Tonight.Show.20260116.720p.HULU.WEB-DL.AAC2.0.H.264-TEPES",
		resolution: Resolution720p,
		source:     SourceWEBDL,
		codec:      CodecX264,
		audio:      AudioAAC,
		title:      "Tonight Show",
		group:      "TEPES",
		service:    "Hulu",
	},

	// === Fixed Audio Patterns ===
	{
		name:       "DD.5.1 with dots (fixed)",
		input:      "The.Matrix.1999.1080p.BluRay.DD.5.1.x264-GRP",
		resolution: Resolution1080p,
		source:     SourceBluRay,
		codec:      CodecX264,
		audio:      AudioAC3,
		title:      "The Matrix",
		year:       1999,
		group:      "GRP",
	},

	// === Fixed HDR Patterns ===
	{
		name:       "DoVi variant (fixed)",
		input:      "Crouching.Tiger.Hidden.Dragon.2000.2160p.UHD.BluRay.DoVi.x265-PTer",
		resolution: Resolution2160p,
		source:     SourceBluRay,
		codec:      CodecX265,
		hdr:        DolbyVision,
		title:      "Crouching Tiger Hidden Dragon",
		year:       2000,
		group:      "PTer",
	},
```

**Step 2: Run golden tests**

Run: `task test -- -run TestParse_Golden -v`
Expected: All tests PASS (including new cases)

**Step 3: Commit**

```bash
git add pkg/release/golden_test.go
git commit -m "$(cat <<'EOF'
test(release): add golden tests for TV series patterns

Add golden test cases for:
- Multi-episode ranges (S01E05-06, S01E05E06)
- Alternate episode formats (1x05)
- Season packs (Season 01, S01, Complete Season)
- Daily show date formats (European, compact)
- Fixed audio patterns (DD.5.1)
- Fixed HDR patterns (DoVi)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Update CLI Parse Command for New Fields

**Files:**
- Modify: `cmd/arrgo/parse.go`
- Test: Manual testing

**Step 1: Read current parse.go**

Examine `cmd/arrgo/parse.go` to understand current output format.

**Step 2: Update output to show new fields**

Add display of new fields in the text output:

```go
// After existing episode display, add:
if len(info.Episodes) > 1 {
    fmt.Printf("  Episodes: %v\n", info.Episodes)
}
if info.IsCompleteSeason {
    fmt.Printf("  Complete Season: yes\n")
}
if info.IsSplitSeason {
    fmt.Printf("  Split Season: Part %d\n", info.SplitPart)
}
```

**Step 3: Test manually**

Run: `./arrgo parse "Show.S01E05-06.720p.HDTV.x264-GRP"`
Expected: Output shows Episodes: [5 6]

Run: `./arrgo parse "Show.Season.01.1080p.BluRay.x264-GRP"`
Expected: Output shows Complete Season: yes

**Step 4: Commit**

```bash
git add cmd/arrgo/parse.go
git commit -m "$(cat <<'EOF'
feat(cli): show multi-episode and season pack info in parse output

Update parse command to display Episodes slice when multiple episodes
detected, and IsCompleteSeason/IsSplitSeason flags for season packs.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Run Full Test Suite and Fix Any Regressions

**Files:**
- Various test files

**Step 1: Run full test suite**

Run: `task check`
Expected: All checks pass (fmt, lint, test)

**Step 2: Fix any failing tests**

If any golden tests or corpus tests fail, update expectations or fix parsing logic.

**Step 3: Run corpus stats test**

Run: `task test -- -run TestCorpus -v`
Expected: Detection thresholds still met

**Step 4: Update snapshots if needed**

Run: `task test -- -run TestSnapshot -update` (if snapshot flag exists)

**Step 5: Final commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
test(release): fix regressions and update test expectations

Ensure all tests pass after TV series parsing enhancements.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

This plan adds the following TV series parsing capabilities:

1. **Multi-episode support**: `S01E05-06`, `S01E05E06E07` → `Episodes: [5,6,7]`
2. **Alternate formats**: `1x05`, `s01.05` episode patterns
3. **Season packs**: `Season 01`, `S01`, `Complete Season` detection via `IsCompleteSeason`
4. **Split seasons**: `Season 1 Part 2` → `IsSplitSeason: true, SplitPart: 2`
5. **Daily show dates**: European, compact, word-month formats
6. **Bug fixes**: DD.5.1 audio detection, DoVi HDR detection

All changes are backward-compatible with existing movie parsing.

**Clean-Room Derivation:** All patterns are derived from publicly documented scene naming conventions and analysis of actual release names from public indexer APIs. No code was copied from GPL-licensed software.
