# Parser Year Extraction Fix

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix parser to extract actual release year when title contains year-like numbers (e.g., "Blade Runner 2049" released 2017).

**Architecture:** Validate year candidates against plausible range (1900 to current+1). Skip implausible years and include them in title.

**Tech Stack:** Go, regexp, time package

---

### Task 1: Add failing tests for year extraction edge cases

**Files:**
- Modify: `pkg/release/parser_test.go`

**Step 1: Write failing tests**

Add test cases for:
- "Blade.Runner.2049.2017.1080p.BluRay" → Title: "Blade Runner 2049", Year: 2017
- "2001.A.Space.Odyssey.1968.1080p.BluRay" → Title: "2001 A Space Odyssey", Year: 1968
- "1917.2019.1080p.BluRay" → Title: "1917", Year: 2019

```go
func TestParse_YearInTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTitle string
		wantYear int
	}{
		{
			name:      "future year in title - Blade Runner 2049",
			input:     "Blade.Runner.2049.2017.1080p.BluRay.x264-GROUP",
			wantTitle: "Blade Runner 2049",
			wantYear:  2017,
		},
		{
			name:      "past year in title - 2001 A Space Odyssey",
			input:     "2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GROUP",
			wantTitle: "2001 A Space Odyssey",
			wantYear:  1968,
		},
		{
			name:      "year-only title - 1917",
			input:     "1917.2019.1080p.BluRay.x264-GROUP",
			wantTitle: "1917",
			wantYear:  2019,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := Parse(tt.input)
			assert.Equal(t, tt.wantTitle, info.Title)
			assert.Equal(t, tt.wantYear, info.Year)
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/release -run TestParse_YearInTitle -v`
Expected: FAIL - wrong title and year extracted

**Step 3: Commit failing tests**

```bash
git add pkg/release/parser_test.go
git commit -m "test: add failing tests for year-in-title parsing (#60)"
```

---

### Task 2: Add isValidReleaseYear helper function

**Files:**
- Modify: `pkg/release/parser.go`

**Step 1: Write test for helper**

```go
func TestIsValidReleaseYear(t *testing.T) {
	currentYear := time.Now().Year()

	tests := []struct {
		year  int
		valid bool
	}{
		{1968, true},   // 2001 A Space Odyssey release
		{2017, true},   // Blade Runner 2049 release
		{2049, false},  // Future - in title, not release year
		{2001, true},   // Valid year (also title of film)
		{1899, false},  // Too old
		{currentYear + 1, true},  // Next year OK (pre-releases)
		{currentYear + 2, false}, // Too far future
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("year_%d", tt.year), func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidReleaseYear(tt.year))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestIsValidReleaseYear -v`
Expected: FAIL - function doesn't exist

**Step 3: Implement helper**

```go
// isValidReleaseYear checks if a year is a plausible release year.
// Valid range: 1900 to current year + 1 (for pre-releases).
func isValidReleaseYear(year int) bool {
	currentYear := time.Now().Year()
	return year >= 1900 && year <= currentYear+1
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release -run TestIsValidReleaseYear -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/parser.go pkg/release/parser_test.go
git commit -m "feat: add isValidReleaseYear helper (#60)"
```

---

### Task 3: Update year extraction to skip invalid years

**Files:**
- Modify: `pkg/release/parser.go`

**Step 1: Update Parse() year extraction**

Replace lines 107-113:

```go
// Year (only if not a daily show)
if info.DailyDate == "" {
	if match := yearRegex.FindString(normalized); match != "" {
		if year, err := strconv.Atoi(match); err == nil {
			info.Year = year
		}
	}
}
```

With:

```go
// Year (only if not a daily show)
// Find first valid release year (skip future years that are part of title)
if info.DailyDate == "" {
	matches := yearRegex.FindAllString(normalized, -1)
	for _, match := range matches {
		if year, err := strconv.Atoi(match); err == nil {
			if isValidReleaseYear(year) {
				info.Year = year
				break
			}
		}
	}
}
```

**Step 2: Run tests**

Run: `go test ./pkg/release -run TestParse_YearInTitle -v`
Expected: Year tests pass, but Title tests still fail (titleMarkerRegex stops at first year)

**Step 3: Commit partial fix**

```bash
git add pkg/release/parser.go
git commit -m "feat: skip invalid years during extraction (#60)"
```

---

### Task 4: Update title extraction to stop at valid year only

**Files:**
- Modify: `pkg/release/parser.go`

**Step 1: Create findFirstValidYearIndex helper**

```go
// findFirstValidYearIndex returns the index of the first valid release year in the string.
// Returns -1 if no valid year found.
func findFirstValidYearIndex(s string) int {
	matches := yearRegex.FindAllStringIndex(s, -1)
	for _, loc := range matches {
		yearStr := s[loc[0]:loc[1]]
		if year, err := strconv.Atoi(yearStr); err == nil {
			if isValidReleaseYear(year) {
				return loc[0]
			}
		}
	}
	return -1
}
```

**Step 2: Update parseTitle to use valid year boundary**

The current `titleMarkerRegex` matches any year. We need a two-pass approach:
1. First check for valid year index
2. Then check other markers (resolution, S01E01, etc.)
3. Use whichever comes first

```go
func parseTitle(name string) string {
	// Find first valid year
	yearIdx := findFirstValidYearIndex(name)

	// Find first non-year marker (resolution, S01E01, etc.)
	// Create a regex without year patterns
	nonYearMarkerRegex := regexp.MustCompile(`(?i)\b\d{3,4}p\b|\bS\d{1,2}E\d{1,2}(?:E\d{1,2}|-E?\d{1,2})*\b|\bS\d{1,2}\b|\b\d{1,2}x\d{1,2}\b|\bComplete[\s.]+Season[\s.]\d{1,2}\b|\bSeason[\s.]\d{1,2}\b|\b4K\b|\bUHD\b`)
	markerLoc := nonYearMarkerRegex.FindStringIndex(name)

	// Use whichever comes first
	endIdx := -1
	if yearIdx >= 0 && (markerLoc == nil || yearIdx < markerLoc[0]) {
		endIdx = yearIdx
	} else if markerLoc != nil {
		endIdx = markerLoc[0]
	}

	if endIdx > 0 {
		return strings.TrimSpace(name[:endIdx])
	}
	return ""
}
```

**Step 3: Run all tests**

Run: `go test ./pkg/release -v`
Expected: All tests pass including TestParse_YearInTitle

**Step 4: Commit**

```bash
git add pkg/release/parser.go
git commit -m "feat: title extraction stops at valid year only (#60)"
```

---

### Task 5: Update golden tests for fixed behavior

**Files:**
- Modify: `pkg/release/golden_test.go`

**Step 1: Update 2001 A Space Odyssey test case**

Find the test case around line 1306 and update expected values:

```go
{
	name:       "Year in title",
	input:      "2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GROUP",
	resolution: Resolution1080p,
	source:     SourceBluRay,
	codec:      CodecX264,
	title:      "2001 A Space Odyssey",  // Was: ""
	year:       1968,                     // Was: 2001
	group:      "GROUP",
},
```

**Step 2: Run golden tests**

Run: `go test ./pkg/release -run TestGolden -v`
Expected: PASS

**Step 3: Run full test suite**

Run: `go test ./pkg/release -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/release/golden_test.go
git commit -m "test: update golden tests for year-in-title fix (#60)"
```

---

### Task 6: Final verification and cleanup

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Run linter**

Run: `golangci-lint run`
Expected: No issues

**Step 3: Manual verification with arrgo parse**

```bash
go build ./cmd/arrgo
./arrgo parse "Blade.Runner.2049.2017.1080p.BluRay.x264-GROUP"
./arrgo parse "2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GROUP"
./arrgo parse "1917.2019.1080p.BluRay.x264-GROUP"
```

Expected: Correct title and year for each

**Step 4: Squash commits and finalize**

```bash
git rebase -i HEAD~5  # Squash into single commit if desired
```

Or keep granular commits - user preference.
