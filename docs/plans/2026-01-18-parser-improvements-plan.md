# Parser Improvements Implementation Plan

**Status:** ✅ Complete (2026-01-19)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve `pkg/release` parser to handle real-world scene releases more robustly, achieving 95%+ title extraction and better metadata detection.

**Architecture:** Token-based parsing with anchor detection (year/SxxExx). Pre-compiled regex patterns with map-based token classification. Title normalization for matching. Backward compatible - existing tests must pass.

**Tech Stack:** Go stdlib (regexp, strings, unicode), no external dependencies.

---

### Task 1: Pre-compile Regex Patterns

**Files:**
- Modify: `pkg/release/parser.go`

**Step 1: Create package-level compiled patterns**

Add at top of `parser.go`, after imports:

```go
// Pre-compiled regex patterns (compiled once at package init)
var (
	yearRegex     = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	seasonEpRegex = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,2})`)
	resolutionRegex = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4K|UHD)\b`)
	titleMarkerRegex = regexp.MustCompile(`(?i)\b(19|20)\d{2}\b|\b\d{3,4}p\b|\bS\d{1,2}E\d{1,2}\b|\b4K\b|\bUHD\b`)
)
```

**Step 2: Update Parse function to use pre-compiled patterns**

Replace the inline regex compilations in `Parse()`:

```go
// Year - use pre-compiled
if match := yearRegex.FindString(normalized); match != "" {
	if year, err := strconv.Atoi(match); err == nil {
		info.Year = year
	}
}

// Season/Episode - use pre-compiled
if matches := seasonEpRegex.FindStringSubmatch(normalized); len(matches) == 3 {
```

**Step 3: Update parseTitle to use pre-compiled pattern**

```go
func parseTitle(name string) string {
	loc := titleMarkerRegex.FindStringIndex(name)
	if loc != nil {
		title := strings.TrimSpace(name[:loc[0]])
		return title
	}
	return ""
}
```

**Step 4: Run tests to verify no regression**

Run: `go test ./pkg/release -v`
Expected: All tests PASS

**Step 5: Run benchmark to verify improvement**

Run: `go test -bench=BenchmarkParse_Corpus -benchmem ./pkg/release`
Expected: Fewer allocations per operation

**Step 6: Commit**

```bash
git add pkg/release/parser.go
git commit -m "perf(release): pre-compile regex patterns"
```

---

### Task 2: Add New Types for Extended Metadata

**Files:**
- Modify: `pkg/release/release.go`

**Step 1: Add HDR format enum**

Add after Codec type definition:

```go
// HDRFormat represents HDR/Dolby Vision formats.
type HDRFormat int

const (
	HDRNone HDRFormat = iota
	HDRGeneric  // "HDR" without specific version
	HDR10
	HDR10Plus
	DolbyVision
	HLG
)

func (h HDRFormat) String() string {
	switch h {
	case HDRGeneric:
		return "HDR"
	case HDR10:
		return "HDR10"
	case HDR10Plus:
		return "HDR10+"
	case DolbyVision:
		return "DV"
	case HLG:
		return "HLG"
	default:
		return ""
	}
}
```

**Step 2: Add Audio codec enum**

```go
// AudioCodec represents the audio format of a release.
type AudioCodec int

const (
	AudioUnknown AudioCodec = iota
	AudioAAC
	AudioAC3      // Dolby Digital
	AudioEAC3     // DD+, DDP
	AudioDTS
	AudioDTSHD    // DTS-HD MA
	AudioTrueHD
	AudioAtmos    // TrueHD Atmos or DD+ Atmos
	AudioFLAC
	AudioOpus
)

func (a AudioCodec) String() string {
	switch a {
	case AudioAAC:
		return "AAC"
	case AudioAC3:
		return "DD"
	case AudioEAC3:
		return "DD+"
	case AudioDTS:
		return "DTS"
	case AudioDTSHD:
		return "DTS-HD MA"
	case AudioTrueHD:
		return "TrueHD"
	case AudioAtmos:
		return "Atmos"
	case AudioFLAC:
		return "FLAC"
	case AudioOpus:
		return "Opus"
	default:
		return ""
	}
}
```

**Step 3: Extend Info struct with new fields**

Update the Info struct:

```go
// Info contains parsed release information.
type Info struct {
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

	// Extended metadata
	HDR     HDRFormat
	Audio   AudioCodec
	IsRemux bool
	Edition string // "Directors Cut", "Extended", "IMAX", etc.
	Service string // Streaming service: NF, AMZN, DSNP, etc.

	// Normalized title for matching
	CleanTitle string
}
```

**Step 4: Run tests to verify no regression**

Run: `go test ./pkg/release -v`
Expected: All tests PASS (new fields default to zero values)

**Step 5: Commit**

```bash
git add pkg/release/release.go
git commit -m "feat(release): add HDR, Audio, Remux, Edition, Service fields"
```

---

### Task 3: Add HDR Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write failing test for HDR detection**

Add to `release_test.go`:

```go
func TestParse_HDR(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantHDR HDRFormat
	}{
		{"DV only", "Movie.2024.2160p.WEB-DL.DV.H265-GRP", DolbyVision},
		{"HDR10", "Movie.2024.2160p.BluRay.HDR10.x265-GRP", HDR10},
		{"HDR10+", "Movie.2024.2160p.UHD.BluRay.HDR10+.x265-GRP", HDR10Plus},
		{"DV HDR combo", "Movie.2024.2160p.WEB-DL.DV.HDR.H265-GRP", DolbyVision},
		{"Generic HDR", "Movie.2024.2160p.BluRay.HDR.x265-GRP", HDRGeneric},
		{"HLG", "Movie.2024.2160p.BluRay.HLG.x265-GRP", HLG},
		{"No HDR", "Movie.2024.1080p.BluRay.x264-GRP", HDRNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.HDR != tt.wantHDR {
				t.Errorf("HDR = %v, want %v", got.HDR, tt.wantHDR)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestParse_HDR -v`
Expected: FAIL (HDR always HDRNone)

**Step 3: Add HDR parsing function**

Add to `parser.go`:

```go
var hdrRegex = regexp.MustCompile(`(?i)\b(HDR10\+|HDR10|HDR|DV|Dolby\.?Vision|HLG)\b`)

func parseHDR(name string) HDRFormat {
	matches := hdrRegex.FindAllString(name, -1)
	if len(matches) == 0 {
		return HDRNone
	}

	// Check in priority order (most specific first)
	for _, m := range matches {
		lower := strings.ToLower(m)
		switch {
		case lower == "dv" || strings.Contains(lower, "dolby"):
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

**Step 4: Wire into Parse function**

Add after codec parsing in `Parse()`:

```go
// HDR
info.HDR = parseHDR(normalized)
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/release -run TestParse_HDR -v`
Expected: All PASS

**Step 6: Run full test suite**

Run: `go test ./pkg/release -v`
Expected: All PASS

**Step 7: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): add HDR/DV detection"
```

---

### Task 4: Add Audio Codec Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write failing test for audio detection**

Add to `release_test.go`:

```go
func TestParse_Audio(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantAudio AudioCodec
	}{
		{"DTS-HD MA", "Movie.2024.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP", AudioDTSHD},
		{"TrueHD Atmos", "Movie.2024.2160p.BluRay.TrueHD.Atmos.7.1.x265-GRP", AudioAtmos},
		{"DD+ Atmos", "Movie.2024.2160p.WEB-DL.DDP5.1.Atmos.H265-GRP", AudioAtmos},
		{"DDP", "Movie.2024.1080p.WEB-DL.DDP5.1.x264-GRP", AudioEAC3},
		{"DD5.1", "Movie.2024.1080p.WEB-DL.DD5.1.x264-GRP", AudioAC3},
		{"FLAC", "Movie.2024.1080p.BluRay.FLAC.2.0.x264-GRP", AudioFLAC},
		{"AAC", "Movie.2024.1080p.WEB-DL.AAC2.0.x264-GRP", AudioAAC},
		{"TrueHD no Atmos", "Movie.2024.1080p.BluRay.TrueHD.5.1.x264-GRP", AudioTrueHD},
		{"Plain DTS", "Movie.2024.1080p.BluRay.DTS.5.1.x264-GRP", AudioDTS},
		{"Opus", "Movie.2024.1080p.WEB.Opus.2.0.x264-GRP", AudioOpus},
		{"No audio info", "Movie.2024.1080p.BluRay.x264-GRP", AudioUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Audio != tt.wantAudio {
				t.Errorf("Audio = %v, want %v", got.Audio, tt.wantAudio)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestParse_Audio -v`
Expected: FAIL

**Step 3: Add audio parsing function**

Add to `parser.go`:

```go
func parseAudio(name string) AudioCodec {
	lower := strings.ToLower(name)

	// Check Atmos first (can be combined with TrueHD or DD+)
	if strings.Contains(lower, "atmos") {
		return AudioAtmos
	}

	// DTS-HD MA before plain DTS
	if strings.Contains(lower, "dts-hd") || strings.Contains(lower, "dts hd") {
		return AudioDTSHD
	}

	// TrueHD
	if strings.Contains(lower, "truehd") {
		return AudioTrueHD
	}

	// DD+ / DDP / EAC3 before DD / AC3
	if containsAny(lower, "ddp", "dd+", "eac3", "e-ac3") {
		return AudioEAC3
	}

	// DD / AC3
	if containsAny(lower, "dd5", "dd2", "dd7", "ac3", "dolby digital") {
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

**Step 4: Wire into Parse function**

Add after HDR parsing in `Parse()`:

```go
// Audio
info.Audio = parseAudio(normalized)
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/release -run TestParse_Audio -v`
Expected: All PASS

**Step 6: Run full test suite**

Run: `go test ./pkg/release -v`
Expected: All PASS

**Step 7: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): add audio codec detection"
```

---

### Task 5: Add Remux Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write failing test**

Add to `release_test.go`:

```go
func TestParse_Remux(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRemux  bool
		wantSource Source
	}{
		{"AVC REMUX", "Movie.2024.1080p.BluRay.AVC.REMUX-GRP", true, SourceBluRay},
		{"REMUX standalone", "Movie.2024.2160p.UHD.BluRay.REMUX.HEVC-GRP", true, SourceBluRay},
		{"Not remux", "Movie.2024.1080p.BluRay.x264-GRP", false, SourceBluRay},
		{"BDRemux variant", "Movie.2024.1080p.BDRemux.x264-GRP", true, SourceBluRay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.IsRemux != tt.wantRemux {
				t.Errorf("IsRemux = %v, want %v", got.IsRemux, tt.wantRemux)
			}
			if got.Source != tt.wantSource {
				t.Errorf("Source = %v, want %v", got.Source, tt.wantSource)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestParse_Remux -v`
Expected: FAIL

**Step 3: Add remux detection**

Add to `parser.go`:

```go
func parseRemux(name string) bool {
	return containsAny(strings.ToLower(name), "remux", "bdremux")
}
```

**Step 4: Wire into Parse function**

Add after Audio parsing:

```go
// Remux
info.IsRemux = parseRemux(normalized)
```

**Step 5: Run tests**

Run: `go test ./pkg/release -run TestParse_Remux -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): add remux detection"
```

---

### Task 6: Add Edition Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write failing test**

```go
func TestParse_Edition(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantEdition string
	}{
		{"Directors Cut", "Movie.2024.Directors.Cut.1080p.BluRay.x264-GRP", "Directors Cut"},
		{"Extended", "Movie.2024.EXTENDED.1080p.BluRay.x264-GRP", "Extended"},
		{"IMAX", "Movie.2024.IMAX.2160p.WEB-DL.x265-GRP", "IMAX"},
		{"Theatrical", "Movie.2024.Theatrical.Cut.1080p.BluRay.x264-GRP", "Theatrical"},
		{"Unrated", "Movie.2024.UNRATED.1080p.BluRay.x264-GRP", "Unrated"},
		{"No edition", "Movie.2024.1080p.BluRay.x264-GRP", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Edition != tt.wantEdition {
				t.Errorf("Edition = %q, want %q", got.Edition, tt.wantEdition)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestParse_Edition -v`
Expected: FAIL

**Step 3: Add edition parsing**

```go
var editionRegex = regexp.MustCompile(`(?i)\b(Directors?\.?Cut|Extended|IMAX|Theatrical\.?Cut?|Unrated|Uncut|Remastered|Anniversary|Criterion|Special\.?Edition)\b`)

func parseEdition(name string) string {
	match := editionRegex.FindString(name)
	if match == "" {
		return ""
	}

	// Normalize
	lower := strings.ToLower(match)
	switch {
	case strings.Contains(lower, "director"):
		return "Directors Cut"
	case strings.Contains(lower, "extended"):
		return "Extended"
	case strings.Contains(lower, "imax"):
		return "IMAX"
	case strings.Contains(lower, "theatrical"):
		return "Theatrical"
	case strings.Contains(lower, "unrated"):
		return "Unrated"
	case strings.Contains(lower, "uncut"):
		return "Uncut"
	case strings.Contains(lower, "remaster"):
		return "Remastered"
	case strings.Contains(lower, "anniversary"):
		return "Anniversary"
	case strings.Contains(lower, "criterion"):
		return "Criterion"
	case strings.Contains(lower, "special"):
		return "Special Edition"
	default:
		return match
	}
}
```

**Step 4: Wire into Parse function**

```go
// Edition
info.Edition = parseEdition(normalized)
```

**Step 5: Run tests**

Run: `go test ./pkg/release -run TestParse_Edition -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): add edition detection"
```

---

### Task 7: Add Streaming Service Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write failing test**

```go
func TestParse_Service(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantService string
	}{
		{"Netflix", "Movie.2024.1080p.NF.WEB-DL.x264-GRP", "Netflix"},
		{"Amazon", "Movie.2024.1080p.AMZN.WEB-DL.x264-GRP", "Amazon"},
		{"Disney+", "Movie.2024.2160p.DSNP.WEB-DL.x265-GRP", "Disney+"},
		{"AppleTV+", "Movie.2024.2160p.ATVP.WEB-DL.x265-GRP", "Apple TV+"},
		{"HBO Max", "Movie.2024.1080p.HMAX.WEB-DL.x264-GRP", "HBO Max"},
		{"Peacock", "Movie.2024.1080p.PCOK.WEB-DL.x264-GRP", "Peacock"},
		{"Hulu", "Movie.2024.1080p.HULU.WEB-DL.x264-GRP", "Hulu"},
		{"No service", "Movie.2024.1080p.WEB-DL.x264-GRP", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Service != tt.wantService {
				t.Errorf("Service = %q, want %q", got.Service, tt.wantService)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestParse_Service -v`
Expected: FAIL

**Step 3: Add service detection**

```go
var serviceMap = map[string]string{
	"nf":     "Netflix",
	"netflix": "Netflix",
	"amzn":   "Amazon",
	"amazon": "Amazon",
	"dsnp":   "Disney+",
	"disney": "Disney+",
	"atvp":   "Apple TV+",
	"aptv":   "Apple TV+",
	"hmax":   "HBO Max",
	"hbo":    "HBO Max",
	"pcok":   "Peacock",
	"peacock": "Peacock",
	"hulu":   "Hulu",
	"pmtp":   "Paramount+",
	"paramount": "Paramount+",
	"stan":   "Stan",
	"crav":   "Crave",
	"now":    "NOW",
	"it":     "iT",
}

func parseService(name string) string {
	lower := strings.ToLower(name)
	for code, service := range serviceMap {
		// Match as whole word
		if strings.Contains(lower, "."+code+".") ||
		   strings.Contains(lower, " "+code+" ") ||
		   strings.HasPrefix(lower, code+".") {
			return service
		}
	}
	return ""
}
```

**Step 4: Wire into Parse function**

```go
// Service
info.Service = parseService(name) // Use original name, not normalized
```

**Step 5: Run tests**

Run: `go test ./pkg/release -run TestParse_Service -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): add streaming service detection"
```

---

### Task 8: Improve Codec Detection

**Files:**
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write test for improved codec detection**

Add to existing tests or create new:

```go
func TestParse_ImprovedCodec(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCodec Codec
	}{
		{"H.264 with dot", "Movie.2024.1080p.WEB-DL.H.264-GRP", CodecX264},
		{"H.265 with dot", "Movie.2024.2160p.WEB-DL.H.265-GRP", CodecX265},
		{"AVC", "Movie.2024.1080p.BluRay.AVC-GRP", CodecX264},
		{"HEVC uppercase", "Movie.2024.2160p.BluRay.HEVC-GRP", CodecX265},
		{"XviD", "Movie.2024.DVDRip.XviD-GRP", CodecUnknown}, // We don't track XviD
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Codec != tt.wantCodec {
				t.Errorf("Codec = %v, want %v", got.Codec, tt.wantCodec)
			}
		})
	}
}
```

**Step 2: Run test to see which fail**

Run: `go test ./pkg/release -run TestParse_ImprovedCodec -v`
Expected: Some may FAIL (H.264 with dot)

**Step 3: Update parseCodec function**

Replace existing `parseCodec`:

```go
func parseCodec(name string) Codec {
	lower := strings.ToLower(name)

	// Normalize H.264 -> h264, H.265 -> h265
	lower = strings.ReplaceAll(lower, "h.264", "h264")
	lower = strings.ReplaceAll(lower, "h.265", "h265")

	switch {
	case containsAny(lower, "x265", "h265", "hevc"):
		return CodecX265
	case containsAny(lower, "x264", "h264", "avc"):
		return CodecX264
	default:
		return CodecUnknown
	}
}
```

**Step 4: Run tests**

Run: `go test ./pkg/release -run TestParse_ImprovedCodec -v`
Expected: All PASS

**Step 5: Run full suite and corpus**

Run: `go test ./pkg/release -v`
Run: `go test ./pkg/release -run TestParse_Corpus -v`
Expected: All PASS, codec detection should improve

**Step 6: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): improve codec detection for H.264/H.265 variants"
```

---

### Task 9: Add Title Normalization

**Files:**
- Create: `pkg/release/normalize.go`
- Create: `pkg/release/normalize_test.go`
- Modify: `pkg/release/parser.go`

**Step 1: Write test for normalization**

Create `pkg/release/normalize_test.go`:

```go
package release

import "testing"

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The Matrix", "matrix"},
		{"A Beautiful Mind", "beautiful mind"},
		{"An American Werewolf", "american werewolf"},
		{"Fast & Furious", "fast and furious"},
		{"Léon: The Professional", "leon professional"},
		{"Spider-Man: No Way Home", "spider man no way home"},
		{"  Extra   Spaces  ", "extra spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CleanTitle(tt.input)
			if got != tt.want {
				t.Errorf("CleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release -run TestCleanTitle -v`
Expected: FAIL (CleanTitle not defined)

**Step 3: Create normalize.go**

Create `pkg/release/normalize.go`:

```go
package release

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// CleanTitle normalizes a title for matching purposes.
// Removes articles, punctuation, accents, and normalizes whitespace.
func CleanTitle(title string) string {
	s := strings.ToLower(title)

	// Remove accents
	s = removeAccents(s)

	// Normalize punctuation
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, ":", " ")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ".", " ")

	// Remove other punctuation
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Remove articles from start
	s = stripLeadingArticle(s)

	// Collapse whitespace
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func removeAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

func stripLeadingArticle(s string) string {
	s = strings.TrimSpace(s)
	articles := []string{"the ", "a ", "an "}
	for _, art := range articles {
		if strings.HasPrefix(s, art) {
			return strings.TrimPrefix(s, art)
		}
	}
	return s
}
```

**Step 4: Add golang.org/x/text dependency**

Run: `go get golang.org/x/text`

**Step 5: Run test**

Run: `go test ./pkg/release -run TestCleanTitle -v`
Expected: All PASS

**Step 6: Wire CleanTitle into Parse**

Add to end of `Parse()` in `parser.go`:

```go
// Clean title for matching
info.CleanTitle = CleanTitle(info.Title)
```

**Step 7: Run full test suite**

Run: `go test ./pkg/release -v`
Expected: All PASS

**Step 8: Commit**

```bash
git add pkg/release/normalize.go pkg/release/normalize_test.go pkg/release/parser.go go.mod go.sum
git commit -m "feat(release): add title normalization for matching"
```

---

### Task 10: Add Daily Show Date Detection

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/parser.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Add DailyDate field to Info**

Already added in Task 2. Verify it exists:

```go
DailyDate  string     // For daily shows: "2026-01-16"
```

If not present, add it to the Info struct.

**Step 2: Write failing test**

```go
func TestParse_DailyShow(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantDailyDate string
		wantYear      int
	}{
		{"Daily show", "Show.2026.01.16.Episode.Title.720p.HDTV.x264-GRP", "2026-01-16", 0},
		{"Not daily", "Show.S01E05.720p.HDTV.x264-GRP", "", 0},
		{"Movie with year", "Movie.2024.1080p.BluRay.x264-GRP", "", 2024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.DailyDate != tt.wantDailyDate {
				t.Errorf("DailyDate = %q, want %q", got.DailyDate, tt.wantDailyDate)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %v, want %v", got.Year, tt.wantYear)
			}
		})
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./pkg/release -run TestParse_DailyShow -v`
Expected: FAIL

**Step 4: Add daily date detection**

Add to `parser.go`:

```go
var dailyRegex = regexp.MustCompile(`\b(20\d{2})\.(\d{2})\.(\d{2})\b`)

func parseDailyDate(name string) string {
	matches := dailyRegex.FindStringSubmatch(name)
	if len(matches) == 4 {
		// Validate it's a reasonable date (month 01-12, day 01-31)
		month := matches[2]
		day := matches[3]
		if month >= "01" && month <= "12" && day >= "01" && day <= "31" {
			return matches[1] + "-" + month + "-" + day
		}
	}
	return ""
}
```

**Step 5: Update Parse to check daily format before year**

In `Parse()`, reorder the year/daily detection:

```go
// Daily show date (check before year extraction)
info.DailyDate = parseDailyDate(name)

// Year (only if not a daily show)
if info.DailyDate == "" {
	if match := yearRegex.FindString(normalized); match != "" {
		if year, err := strconv.Atoi(match); err == nil {
			info.Year = year
		}
	}
}
```

**Step 6: Run tests**

Run: `go test ./pkg/release -run TestParse_DailyShow -v`
Expected: All PASS

**Step 7: Run full suite**

Run: `go test ./pkg/release -v`
Expected: All PASS

**Step 8: Commit**

```bash
git add pkg/release/parser.go pkg/release/release_test.go
git commit -m "feat(release): add daily show date detection"
```

---

### Task 11: Final Corpus Validation

**Files:**
- Modify: `pkg/release/corpus_test.go`

**Step 1: Update corpus test with stricter thresholds**

Update the thresholds in `corpus_test.go`:

```go
// Updated thresholds based on improvements
emptyPct := pct(stats.emptyTitle, stats.total)
if emptyPct > 5 {
	t.Errorf("Too many empty titles: %.1f%% (want < 5%%)", emptyPct)
}

resPct := pct(stats.hasResolution, stats.total)
if resPct < 90 {
	t.Errorf("Resolution detection too low: %.1f%% (want > 90%%)", resPct)
}

codecPct := pct(stats.hasCodec, stats.total)
if codecPct < 60 {
	t.Errorf("Codec detection too low: %.1f%% (want > 60%%)", codecPct)
}
```

**Step 2: Add new field tracking to corpus test**

Add tracking for new fields:

```go
var stats struct {
	total         int
	emptyTitle    int
	hasResolution int
	hasSource     int
	hasCodec      int
	hasYear       int
	hasGroup      int
	// New
	hasHDR        int
	hasAudio      int
	hasRemux      int
	hasEdition    int
	hasService    int
	hasDailyDate  int
}

// In the loop:
if info.HDR != HDRNone {
	stats.hasHDR++
}
if info.Audio != AudioUnknown {
	stats.hasAudio++
}
if info.IsRemux {
	stats.hasRemux++
}
if info.Edition != "" {
	stats.hasEdition++
}
if info.Service != "" {
	stats.hasService++
}
if info.DailyDate != "" {
	stats.hasDailyDate++
}

// In logging:
t.Logf("  Has HDR:        %d (%.1f%%)", stats.hasHDR, pct(stats.hasHDR, stats.total))
t.Logf("  Has Audio:      %d (%.1f%%)", stats.hasAudio, pct(stats.hasAudio, stats.total))
t.Logf("  Has Remux:      %d (%.1f%%)", stats.hasRemux, pct(stats.hasRemux, stats.total))
t.Logf("  Has Edition:    %d (%.1f%%)", stats.hasEdition, pct(stats.hasEdition, stats.total))
t.Logf("  Has Service:    %d (%.1f%%)", stats.hasService, pct(stats.hasService, stats.total))
t.Logf("  Has DailyDate:  %d (%.1f%%)", stats.hasDailyDate, pct(stats.hasDailyDate, stats.total))
```

**Step 3: Run corpus test**

Run: `go test ./pkg/release -run TestParse_Corpus -v`
Expected: PASS with improved stats

**Step 4: Run full test suite**

Run: `go test ./pkg/release -v`
Expected: All PASS

**Step 5: Run linter**

Run: `golangci-lint run ./pkg/release`
Expected: No issues

**Step 6: Run benchmark comparison**

Run: `go test -bench=BenchmarkParse_Corpus -benchmem ./pkg/release`
Expected: Fewer allocations than baseline (~170/parse)

**Step 7: Commit**

```bash
git add pkg/release/corpus_test.go
git commit -m "test(release): update corpus test with new field tracking"
```

---

### Task 12: Documentation Update

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Update module description in CLAUDE.md**

Find the pkg/release entry in CLAUDE.md and update:

```markdown
| pkg/release | Release name parsing (resolution, source, codec, HDR, audio, edition, service) |
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update release package description"
```

---

## Summary

After completing all tasks:

1. Pre-compiled regex patterns for performance
2. New fields: HDR, Audio, IsRemux, Edition, Service, DailyDate, CleanTitle
3. Improved codec detection (H.264/H.265 variants)
4. Title normalization for matching
5. Daily show date format support
6. Comprehensive test coverage against 1824-title corpus

Expected improvements:
- Codec detection: 40% → 60%+
- New metadata extraction for ~50% of releases (HDR, Audio, Service)
- Better performance (fewer allocations)
- Clean titles for future matching features
