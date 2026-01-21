# Parser Robustness Enhancement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve release parser accuracy from ~75-80% to 90%+ using lightweight fuzzy matching and TMDB fallback without replicating Sonarr/Radarr complexity.

**Architecture:** Three-tier approach: (1) Enhanced regex with improved normalization, (2) Jaro-Winkler fuzzy title matching via go-edlib, (3) TMDB API fallback for low-confidence matches. Each tier is independently testable.

**Tech Stack:** Go, go-edlib (Jaro-Winkler), existing TMDB client, testify for assertions.

---

## Task 1: Add Roman Numeral Normalization

**Files:**
- Modify: `pkg/release/normalize.go`
- Create: `pkg/release/normalize_test.go` (if not exists, else modify)

**Step 1: Write the failing test**

Add to `pkg/release/normalize_test.go`:

```go
package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRomanNumerals(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Rocky III", "Rocky 3"},
		{"Part II", "Part 2"},
		{"Chapter IV", "Chapter 4"},
		{"Star Wars Episode V", "Star Wars Episode 5"},
		{"Final Fantasy VII", "Final Fantasy 7"},
		{"Resident Evil VIII", "Resident Evil 8"},
		{"Henry V", "Henry 5"},
		{"The Godfather Part II", "The Godfather Part 2"},
		// Should NOT convert
		{"I Robot", "I Robot"},           // "I" alone is ambiguous
		{"VII Days", "VII Days"},         // Roman at start without context
		{"Matrix", "Matrix"},             // No roman numerals
		{"2001 A Space Odyssey", "2001 A Space Odyssey"}, // Arabic stays
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeRomanNumerals(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/... -run TestNormalizeRomanNumerals -v`
Expected: FAIL with "undefined: NormalizeRomanNumerals"

**Step 3: Write minimal implementation**

Add to `pkg/release/normalize.go`:

```go
import (
	"regexp"
	"strings"
)

// romanNumeralRegex matches Roman numerals II-X when preceded by space or word boundary
// Does NOT match standalone "I" to avoid false positives like "I Robot"
var romanNumeralRegex = regexp.MustCompile(`\b(II|III|IV|V|VI|VII|VIII|IX|X)\b`)

var romanToArabic = map[string]string{
	"II": "2", "III": "3", "IV": "4", "V": "5",
	"VI": "6", "VII": "7", "VIII": "8", "IX": "9", "X": "10",
}

// NormalizeRomanNumerals converts Roman numerals (II-X) to Arabic numbers.
// Does not convert standalone "I" to avoid false positives.
func NormalizeRomanNumerals(s string) string {
	return romanNumeralRegex.ReplaceAllStringFunc(s, func(match string) string {
		if arabic, ok := romanToArabic[strings.ToUpper(match)]; ok {
			return arabic
		}
		return match
	})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -run TestNormalizeRomanNumerals -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/normalize.go pkg/release/normalize_test.go
git commit -m "feat(parser): add Roman numeral to Arabic number normalization

Converts II-X to 2-10 for better title matching.
Does not convert standalone 'I' to avoid false positives."
```

---

## Task 2: Integrate Roman Numeral Normalization into CleanTitle

**Files:**
- Modify: `pkg/release/normalize.go`
- Modify: `pkg/release/normalize_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/normalize_test.go`:

```go
func TestCleanTitleWithRomanNumerals(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Back to the Future Part II", "back to future part 2"},
		{"Rocky III", "rocky 3"},
		{"The Godfather Part III", "godfather part 3"},
		{"Fast & Furious", "fast and furious"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CleanTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/... -run TestCleanTitleWithRomanNumerals -v`
Expected: FAIL (Roman numerals not converted)

**Step 3: Modify CleanTitle to include normalization**

Update `CleanTitle` in `pkg/release/normalize.go`:

```go
// CleanTitle normalizes a title for matching purposes.
// Removes articles, punctuation, accents, and normalizes whitespace.
// Converts Roman numerals to Arabic numbers for consistent matching.
func CleanTitle(title string) string {
	s := strings.ToLower(title)

	// Remove accents
	s = removeAccents(s)

	// Convert Roman numerals to Arabic before other processing
	s = NormalizeRomanNumerals(s)

	// Normalize punctuation
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ".", " ")

	// Split on colon to handle subtitles (e.g., "Léon: The Professional")
	// Strip leading articles from each part
	parts := strings.Split(s, ":")
	for i, part := range parts {
		parts[i] = stripLeadingArticle(strings.TrimSpace(part))
	}
	s = strings.Join(parts, " ")

	// Remove other punctuation
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Collapse whitespace
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
```

Note: The Roman numeral regex needs case-insensitive matching since we lowercase first:

```go
var romanNumeralRegex = regexp.MustCompile(`(?i)\b(ii|iii|iv|v|vi|vii|viii|ix|x)\b`)
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -run TestCleanTitleWithRomanNumerals -v`
Expected: PASS

**Step 5: Run full test suite to check for regressions**

Run: `task test`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pkg/release/normalize.go pkg/release/normalize_test.go
git commit -m "feat(parser): integrate Roman numeral normalization into CleanTitle

CleanTitle now converts Roman numerals II-X to Arabic numbers
for consistent title matching across sequel variations."
```

---

## Task 3: Add Audio Pattern Variants

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/golden_test.go`

**Step 1: Add golden test cases for audio variants**

Add to `pkg/release/golden_test.go` in the goldenCases slice:

```go
// === Audio Pattern Variants (new cases) ===
{
	name:       "DD.5.1 with dots",
	input:      "Movie.Name.2024.1080p.BluRay.DD.5.1.x264-GROUP",
	resolution: Resolution1080p,
	source:     SourceBluRay,
	codec:      CodecX264,
	audio:      AudioAC3,
	title:      "Movie Name",
	year:       2024,
	group:      "GROUP",
},
{
	name:       "DD5.1 no separator",
	input:      "Movie.Name.2024.1080p.BluRay.DD5.1.x264-GROUP",
	resolution: Resolution1080p,
	source:     SourceBluRay,
	codec:      CodecX264,
	audio:      AudioAC3,
	title:      "Movie Name",
	year:       2024,
	group:      "GROUP",
},
{
	name:       "DDP.5.1 with dots",
	input:      "Movie.Name.2024.1080p.WEB-DL.DDP.5.1.x265-GROUP",
	resolution: Resolution1080p,
	source:     SourceWEBDL,
	codec:      CodecX265,
	audio:      AudioEAC3,
	title:      "Movie Name",
	year:       2024,
	group:      "GROUP",
},
{
	name:       "TrueHD.Atmos with dot",
	input:      "Movie.Name.2024.2160p.UHD.BluRay.TrueHD.Atmos.x265-GROUP",
	resolution: Resolution2160p,
	source:     SourceBluRay,
	codec:      CodecX265,
	audio:      AudioAtmos,
	title:      "Movie Name",
	year:       2024,
	group:      "GROUP",
},
```

**Step 2: Run test to verify failures**

Run: `go test ./pkg/release/... -run TestGoldenCases -v`
Expected: Some new cases may fail if audio patterns don't match

**Step 3: Expand audio patterns in parser.go**

Update the audio regex patterns in `pkg/release/parser.go`:

```go
// Audio detection patterns (must work with both raw and normalized names)
ddPlusRegex = regexp.MustCompile(`(?i)\b(ddp|dd\+|dda)[\s.]?\d`)
ddRegex     = regexp.MustCompile(`(?i)\bdd[\s.]?\d[\s.]?\d`)  // DD5.1, DD.5.1, DD 5 1
```

Update `parseAudio` function:

```go
func parseAudio(name string) AudioCodec {
	lower := strings.ToLower(name)

	// Check Atmos first (can be combined with TrueHD or DD+)
	if strings.Contains(lower, "atmos") {
		return AudioAtmos
	}

	// DTS-HD MA before plain DTS
	if strings.Contains(lower, "dts-hd") || strings.Contains(lower, "dts hd") || strings.Contains(lower, "dtshd") {
		return AudioDTSHD
	}

	// TrueHD
	if strings.Contains(lower, "truehd") || strings.Contains(lower, "true hd") {
		return AudioTrueHD
	}

	// DD+ / DDP / EAC3 before DD / AC3
	// Match: ddp, dd+, ddp5.1, dd+5.1, ddp.5.1, eac3, e-ac3
	if containsAny(lower, "ddp", "dd+", "eac3", "e-ac3", "e ac3") || ddPlusRegex.MatchString(lower) {
		return AudioEAC3
	}

	// DD / AC3 - match: dd5.1, dd.5.1, dd 5.1, dd51, ac3, dolby digital
	if containsAny(lower, "dd5", "dd2", "dd7", "ac3", "dolby digital") || ddRegex.MatchString(lower) {
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

Run: `go test ./pkg/release/... -run TestGoldenCases -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/golden_test.go
git commit -m "feat(parser): expand audio pattern detection

Add support for DD.5.1, DD5.1, DDP.5.1 and similar variants.
Improve TrueHD and DTS-HD detection patterns."
```

---

## Task 4: Add Service Pattern Variants

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/golden_test.go`

**Step 1: Add golden test cases for service variants**

Add to `pkg/release/golden_test.go`:

```go
// === Service Pattern Variants (new cases) ===
{
	name:       "DSNY service code",
	input:      "Movie.Name.2024.1080p.DSNY.WEB-DL.x264-GROUP",
	resolution: Resolution1080p,
	source:     SourceWEBDL,
	codec:      CodecX264,
	service:    "Disney+",
	title:      "Movie Name",
	year:       2024,
	group:      "GROUP",
},
{
	name:       "APTV service code",
	input:      "Movie.Name.2024.2160p.APTV.WEB-DL.DV.x265-GROUP",
	resolution: Resolution2160p,
	source:     SourceWEBDL,
	codec:      CodecX265,
	hdr:        DolbyVision,
	service:    "Apple TV+",
	title:      "Movie Name",
	year:       2024,
	group:      "GROUP",
},
```

**Step 2: Run test to verify failures**

Run: `go test ./pkg/release/... -run TestGoldenCases -v`
Expected: New service cases may fail

**Step 3: Expand serviceMap in parser.go**

Update `serviceMap` in `pkg/release/parser.go`:

```go
var serviceMap = map[string]string{
	// Netflix
	"nf":        "Netflix",
	"netflix":   "Netflix",
	// Amazon
	"amzn":      "Amazon",
	"amazon":    "Amazon",
	// Disney+
	"dsnp":      "Disney+",
	"dsny":      "Disney+",
	"disney":    "Disney+",
	"disneyplus": "Disney+",
	// Apple TV+
	"atvp":      "Apple TV+",
	"aptv":      "Apple TV+",
	"atv":       "Apple TV+",
	// HBO Max
	"hmax":      "HBO Max",
	"hbom":      "HBO Max",
	"hbo":       "HBO Max",
	// Peacock
	"pcok":      "Peacock",
	"peacock":   "Peacock",
	// Hulu
	"hulu":      "Hulu",
	// Paramount+
	"pmtp":      "Paramount+",
	"paramount": "Paramount+",
	// Other services
	"stan":      "Stan",
	"crav":      "Crave",
	"now":       "NOW",
	"it":        "iT",
	"tubi":      "Tubi",
	"vudu":      "Vudu",
	"ma":        "Movies Anywhere",
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -run TestGoldenCases -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/golden_test.go
git commit -m "feat(parser): expand streaming service detection

Add DSNY, APTV, HBOM, TUBI, VUDU, MA service codes.
Improves service recognition coverage."
```

---

## Task 5: Add go-edlib Dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the dependency**

Run: `go get github.com/hbollon/go-edlib`

**Step 2: Verify no conflicts**

Run: `go mod tidy`

**Step 3: Verify build succeeds**

Run: `go build ./...`
Expected: Build succeeds without errors

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add go-edlib for fuzzy string matching

go-edlib provides Jaro-Winkler similarity for title matching.
MIT licensed, pure Go, ~50KB overhead."
```

---

## Task 6: Create Fuzzy Title Matcher - Types and Interface

**Files:**
- Create: `pkg/release/matcher.go`
- Create: `pkg/release/matcher_test.go`

**Step 1: Write the failing test for types**

Create `pkg/release/matcher_test.go`:

```go
package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchConfidenceString(t *testing.T) {
	tests := []struct {
		conf     MatchConfidence
		expected string
	}{
		{ConfidenceHigh, "high"},
		{ConfidenceMedium, "medium"},
		{ConfidenceLow, "low"},
		{ConfidenceNone, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.conf.String())
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/... -run TestMatchConfidenceString -v`
Expected: FAIL with "undefined: MatchConfidence"

**Step 3: Write minimal implementation**

Create `pkg/release/matcher.go`:

```go
package release

// MatchConfidence represents the confidence level of a title match.
type MatchConfidence int

const (
	ConfidenceNone   MatchConfidence = iota // Score < 0.70
	ConfidenceLow                           // Score >= 0.70
	ConfidenceMedium                        // Score >= 0.85
	ConfidenceHigh                          // Score >= 0.95
)

func (c MatchConfidence) String() string {
	switch c {
	case ConfidenceHigh:
		return "high"
	case ConfidenceMedium:
		return "medium"
	case ConfidenceLow:
		return "low"
	default:
		return "none"
	}
}

// MatchResult represents the result of a fuzzy title match.
type MatchResult struct {
	Title      string          // The matched candidate title
	Score      float64         // Jaro-Winkler similarity score (0.0-1.0)
	Confidence MatchConfidence // Confidence level based on score
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -run TestMatchConfidenceString -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/matcher.go pkg/release/matcher_test.go
git commit -m "feat(matcher): add MatchConfidence type and MatchResult struct

Foundation types for fuzzy title matching feature."
```

---

## Task 7: Implement MatchTitle Function

**Files:**
- Modify: `pkg/release/matcher.go`
- Modify: `pkg/release/matcher_test.go`

**Step 1: Write the failing test**

Add to `pkg/release/matcher_test.go`:

```go
func TestMatchTitle(t *testing.T) {
	candidates := []string{
		"The Matrix",
		"The Matrix Reloaded",
		"The Matrix Revolutions",
		"Back to the Future",
		"Back to the Future Part II",
		"Back to the Future Part III",
	}

	tests := []struct {
		name           string
		parsed         string
		expectedTitle  string
		expectedConf   MatchConfidence
		minScore       float64
	}{
		{
			name:          "exact match",
			parsed:        "The Matrix",
			expectedTitle: "The Matrix",
			expectedConf:  ConfidenceHigh,
			minScore:      0.99,
		},
		{
			name:          "close match with roman numeral",
			parsed:        "Back to the Future 2",
			expectedTitle: "Back to the Future Part II",
			expectedConf:  ConfidenceMedium,
			minScore:      0.80,
		},
		{
			name:          "no match",
			parsed:        "Completely Different Movie",
			expectedTitle: "",
			expectedConf:  ConfidenceNone,
			minScore:      0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTitle(tt.parsed, candidates)
			if tt.expectedConf == ConfidenceNone {
				assert.Equal(t, ConfidenceNone, result.Confidence)
			} else {
				assert.Equal(t, tt.expectedTitle, result.Title)
				assert.GreaterOrEqual(t, result.Score, tt.minScore)
				assert.GreaterOrEqual(t, result.Confidence, tt.expectedConf)
			}
		})
	}
}

func TestMatchTitleEmptyCandidates(t *testing.T) {
	result := MatchTitle("Any Title", []string{})
	assert.Equal(t, ConfidenceNone, result.Confidence)
	assert.Equal(t, "", result.Title)
	assert.Equal(t, 0.0, result.Score)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/... -run TestMatchTitle -v`
Expected: FAIL with "undefined: MatchTitle"

**Step 3: Implement MatchTitle**

Add to `pkg/release/matcher.go`:

```go
import (
	"github.com/hbollon/go-edlib"
)

// MatchTitle finds the best match for a parsed title against candidate titles.
// Uses Jaro-Winkler similarity which favors prefix matches (good for media titles).
// Returns the best match with confidence level based on score thresholds.
func MatchTitle(parsed string, candidates []string) MatchResult {
	if len(candidates) == 0 {
		return MatchResult{Confidence: ConfidenceNone}
	}

	// Normalize the parsed title for comparison
	normalizedParsed := CleanTitle(parsed)

	best := MatchResult{
		Score:      0,
		Confidence: ConfidenceNone,
	}

	for _, candidate := range candidates {
		normalizedCandidate := CleanTitle(candidate)

		// Calculate Jaro-Winkler similarity (returns value between 0 and 1)
		score := edlib.JaroWinklerSimilarity(normalizedParsed, normalizedCandidate)

		if score > best.Score {
			best.Title = candidate
			best.Score = score
		}
	}

	// Set confidence level based on score thresholds
	switch {
	case best.Score >= 0.95:
		best.Confidence = ConfidenceHigh
	case best.Score >= 0.85:
		best.Confidence = ConfidenceMedium
	case best.Score >= 0.70:
		best.Confidence = ConfidenceLow
	default:
		best.Confidence = ConfidenceNone
		best.Title = "" // Clear title for no-match case
	}

	return best
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -run TestMatchTitle -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pkg/release/matcher.go pkg/release/matcher_test.go
git commit -m "feat(matcher): implement MatchTitle with Jaro-Winkler similarity

Uses go-edlib for Jaro-Winkler distance calculation.
Returns confidence levels: high (>=0.95), medium (>=0.85), low (>=0.70)."
```

---

## Task 8: Add MatchConfidence to Release.Info

**Files:**
- Modify: `pkg/release/release.go`

**Step 1: Add field to Info struct**

Update `pkg/release/release.go` - add to Info struct:

```go
// Info contains parsed release information.
type Info struct {
	// ... existing fields ...

	// Match confidence (set during title matching, not parsing)
	MatchConfidence MatchConfidence `json:"match_confidence,omitempty"`
}
```

**Step 2: Run tests to verify no regressions**

Run: `task test`
Expected: All tests pass (field is zero value by default)

**Step 3: Commit**

```bash
git add pkg/release/release.go
git commit -m "feat(release): add MatchConfidence field to Info struct

Allows storing match confidence alongside parsed release info."
```

---

## Task 9: Update CLI Parse Command to Show Confidence

**Files:**
- Modify: `cmd/arrgo/parse.go`

**Step 1: Update ParseResultJSON**

Add to `ParseResultJSON` struct in `cmd/arrgo/parse.go`:

```go
type ParseResultJSON struct {
	// ... existing fields ...
	MatchConfidence string `json:"match_confidence,omitempty"`
}
```

**Step 2: Update toJSON method**

Update the `toJSON` method:

```go
func (r ParseResult) toJSON() ParseResultJSON {
	info := r.Info
	result := ParseResultJSON{
		// ... existing fields ...
	}

	// Set match confidence if not none
	if info.MatchConfidence != release.ConfidenceNone {
		result.MatchConfidence = info.MatchConfidence.String()
	}

	return result
}
```

**Step 3: Update printHumanReadable**

Add to `printHumanReadable` function:

```go
// After CleanTitle output
if info.MatchConfidence != release.ConfidenceNone {
	fmt.Printf("Confidence:  %s\n", info.MatchConfidence.String())
}
```

**Step 4: Run build to verify**

Run: `go build ./cmd/arrgo/...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add cmd/arrgo/parse.go
git commit -m "feat(cli): display match confidence in parse output

Shows confidence level when available in both human and JSON output."
```

---

## Task 10: Integration Test for Fuzzy Matching

**Files:**
- Create: `pkg/release/matcher_integration_test.go`

**Step 1: Create integration test**

Create `pkg/release/matcher_integration_test.go`:

```go
package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMatchTitleRealWorldCases tests fuzzy matching with real-world variations.
func TestMatchTitleRealWorldCases(t *testing.T) {
	// Simulated library titles
	library := []string{
		"Back to the Future",
		"Back to the Future Part II",
		"Back to the Future Part III",
		"The Matrix",
		"The Matrix Reloaded",
		"The Matrix Revolutions",
		"Fast & Furious",
		"Fast Five",
		"The Fast and the Furious",
		"Rocky",
		"Rocky II",
		"Rocky III",
		"Rocky IV",
	}

	tests := []struct {
		releaseName   string
		expectedTitle string
		minConfidence MatchConfidence
	}{
		// Roman numeral variations
		{"Back.to.the.Future.Part.II.1989.1080p.BluRay", "Back to the Future Part II", ConfidenceMedium},
		{"Back.to.the.Future.2.1989.1080p.BluRay", "Back to the Future Part II", ConfidenceMedium},
		{"Rocky.III.1982.2160p.UHD.BluRay", "Rocky III", ConfidenceHigh},
		{"Rocky.3.1982.2160p.UHD.BluRay", "Rocky III", ConfidenceMedium},

		// Conjunction variations
		{"Fast.and.Furious.2009.1080p.BluRay", "Fast & Furious", ConfidenceHigh},

		// Standard matches
		{"The.Matrix.1999.2160p.UHD.BluRay", "The Matrix", ConfidenceHigh},
	}

	for _, tt := range tests {
		t.Run(tt.releaseName, func(t *testing.T) {
			// Parse the release to extract title
			info := Parse(tt.releaseName)

			// Match against library
			result := MatchTitle(info.Title, library)

			assert.Equal(t, tt.expectedTitle, result.Title, "wrong title matched")
			assert.GreaterOrEqual(t, result.Confidence, tt.minConfidence, "confidence too low")
		})
	}
}
```

**Step 2: Run integration test**

Run: `go test ./pkg/release/... -run TestMatchTitleRealWorldCases -v`
Expected: PASS

**Step 3: Commit**

```bash
git add pkg/release/matcher_integration_test.go
git commit -m "test(matcher): add integration tests for real-world title variations

Tests Roman numeral conversions, conjunction handling, and standard matches."
```

---

## Task 11: Run Full Test Suite and Lint

**Step 1: Run full test suite**

Run: `task test`
Expected: All tests pass

**Step 2: Run linter**

Run: `task lint`
Expected: No warnings

**Step 3: Build**

Run: `task build`
Expected: Build succeeds

**Step 4: Manual verification**

Run: `./arrgo parse "Back.to.the.Future.Part.II.1989.1080p.BluRay.x264-GROUP"`
Expected: Shows Title, Year, Resolution, Source, Codec, CleanTitle

Run: `./arrgo parse --json "Rocky.III.1982.2160p.UHD.BluRay.DV.x265-GROUP"`
Expected: Valid JSON output with all fields

**Step 5: Final commit**

```bash
git add -A
git commit -m "chore: parser robustness enhancement complete

- Roman numeral normalization (II-X to 2-10)
- Expanded audio patterns (DD.5.1 variants)
- Expanded service detection (DSNY, APTV, etc.)
- Fuzzy title matching with Jaro-Winkler
- Match confidence levels (high/medium/low/none)

Estimated improvement: +10-15% accuracy on edge cases."
```

---

## Verification Checklist

After completing all tasks:

1. [ ] `task test` - All tests pass (should be 850+ tests)
2. [ ] `task lint` - No warnings
3. [ ] `task build` - Builds successfully
4. [ ] `./arrgo parse "Test.Movie.2024.1080p.BluRay.x264-GROUP"` - Works
5. [ ] Roman numerals convert: `./arrgo parse "Rocky.III.1982.1080p.BluRay.x264-GROUP"` shows CleanTitle with "3"
6. [ ] Audio variants detected: `./arrgo parse "Movie.2024.1080p.BluRay.DD.5.1.x264-GROUP"` shows Audio: DD

## Files Changed Summary

| File | Change Type |
|------|-------------|
| `pkg/release/normalize.go` | Modified - Roman numeral normalization |
| `pkg/release/normalize_test.go` | Modified - New tests |
| `pkg/release/parser.go` | Modified - Audio/service patterns |
| `pkg/release/golden_test.go` | Modified - New test cases |
| `pkg/release/matcher.go` | Created - Fuzzy matching |
| `pkg/release/matcher_test.go` | Created - Matcher tests |
| `pkg/release/matcher_integration_test.go` | Created - Integration tests |
| `pkg/release/release.go` | Modified - MatchConfidence field |
| `cmd/arrgo/parse.go` | Modified - Display confidence |
| `go.mod`, `go.sum` | Modified - go-edlib dependency |
