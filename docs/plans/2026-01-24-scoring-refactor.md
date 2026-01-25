# Scoring Logic Refactor Implementation Plan

> **Status:** ✅ COMPLETED (2026-01-24)

**Goal:** Extract duplicated scoring logic from `cmd/arrgo/parse.go` and `internal/search/scorer.go` into a shared `pkg/release/scoring` package.

**Architecture:** Create a new `pkg/release/scoring` package containing score constants, matching functions (`hdrMatches`, `audioMatches`, `matchesRejectList`, `rejectMatchesSpecial`), and base score calculation. Both consumers import from the shared package. The CLI's breakdown-enabled scoring and search's simple scoring remain in their respective locations but use shared primitives.

**Tech Stack:** Go, standard library only

---

## Summary of Duplication

**Identical code in both files:**
- Score constants (resolution scores, bonus values)
- `hdrMatches()` - HDR format to preference string matching
- `audioMatches()` - Audio codec to preference string matching
- `matchesRejectList()` - Reject list checking
- `rejectMatchesSpecial()` - Special reject patterns (cam, ts, remux, etc.)
- `resolutionBaseScore()` - Resolution to base score mapping

---

### Task 1: Create scoring package with constants

**Files:**
- Create: `pkg/release/scoring/scoring.go`
- Test: `pkg/release/scoring/scoring_test.go`

**Step 1: Write the failing test for constants**

```go
// pkg/release/scoring/scoring_test.go
package scoring

import "testing"

func TestScoreConstants(t *testing.T) {
	// Verify constants exist and have expected values
	if ScoreResolution2160p != 100 {
		t.Errorf("ScoreResolution2160p = %d, want 100", ScoreResolution2160p)
	}
	if ScoreResolution1080p != 80 {
		t.Errorf("ScoreResolution1080p = %d, want 80", ScoreResolution1080p)
	}
	if ScoreResolution720p != 60 {
		t.Errorf("ScoreResolution720p = %d, want 60", ScoreResolution720p)
	}
	if ScoreResolutionOther != 40 {
		t.Errorf("ScoreResolutionOther = %d, want 40", ScoreResolutionOther)
	}
	if BonusSource != 10 {
		t.Errorf("BonusSource = %d, want 10", BonusSource)
	}
	if BonusCodec != 10 {
		t.Errorf("BonusCodec = %d, want 10", BonusCodec)
	}
	if BonusHDR != 15 {
		t.Errorf("BonusHDR = %d, want 15", BonusHDR)
	}
	if BonusAudio != 15 {
		t.Errorf("BonusAudio = %d, want 15", BonusAudio)
	}
	if BonusRemux != 20 {
		t.Errorf("BonusRemux = %d, want 20", BonusRemux)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/scoring/... -v`
Expected: FAIL - package does not exist

**Step 3: Write minimal implementation**

```go
// pkg/release/scoring/scoring.go
// Package scoring provides shared scoring constants and matching functions
// for quality profile evaluation.
package scoring

// Base scores for resolutions.
const (
	ScoreResolution2160p = 100
	ScoreResolution1080p = 80
	ScoreResolution720p  = 60
	ScoreResolutionOther = 40
)

// Bonus values for matching attributes.
const (
	BonusSource = 10
	BonusCodec  = 10
	BonusHDR    = 15
	BonusAudio  = 15
	BonusRemux  = 20
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/scoring/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/scoring/scoring.go pkg/release/scoring/scoring_test.go
git commit -m "feat(scoring): add shared scoring constants package"
```

---

### Task 2: Add ResolutionBaseScore function

**Files:**
- Modify: `pkg/release/scoring/scoring.go`
- Modify: `pkg/release/scoring/scoring_test.go`

**Step 1: Write the failing test**

```go
// Add to pkg/release/scoring/scoring_test.go

func TestResolutionBaseScore(t *testing.T) {
	tests := []struct {
		resolution release.Resolution
		want       int
	}{
		{release.Resolution2160p, 100},
		{release.Resolution1080p, 80},
		{release.Resolution720p, 60},
		{release.ResolutionUnknown, 40},
	}

	for _, tt := range tests {
		t.Run(tt.resolution.String(), func(t *testing.T) {
			got := ResolutionBaseScore(tt.resolution)
			if got != tt.want {
				t.Errorf("ResolutionBaseScore(%v) = %d, want %d", tt.resolution, got, tt.want)
			}
		})
	}
}
```

Also add import at top of test file:
```go
import (
	"testing"

	"github.com/vmunix/arrgo/pkg/release"
)
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/scoring/... -v`
Expected: FAIL - undefined: ResolutionBaseScore

**Step 3: Write minimal implementation**

Add to `pkg/release/scoring/scoring.go`:

```go
import "github.com/vmunix/arrgo/pkg/release"

// ResolutionBaseScore returns the base score for a given resolution.
func ResolutionBaseScore(r release.Resolution) int {
	switch r {
	case release.Resolution2160p:
		return ScoreResolution2160p
	case release.Resolution1080p:
		return ScoreResolution1080p
	case release.Resolution720p:
		return ScoreResolution720p
	default:
		return ScoreResolutionOther
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/scoring/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/scoring/scoring.go pkg/release/scoring/scoring_test.go
git commit -m "feat(scoring): add ResolutionBaseScore function"
```

---

### Task 3: Add HDRMatches function

**Files:**
- Modify: `pkg/release/scoring/scoring.go`
- Modify: `pkg/release/scoring/scoring_test.go`

**Step 1: Write the failing test**

```go
// Add to pkg/release/scoring/scoring_test.go

func TestHDRMatches(t *testing.T) {
	tests := []struct {
		hdr  release.HDRFormat
		pref string
		want bool
	}{
		// Dolby Vision variations
		{release.DolbyVision, "dolby-vision", true},
		{release.DolbyVision, "dv", true},
		{release.DolbyVision, "dolbyvision", true},
		{release.DolbyVision, "DV", true}, // case insensitive
		{release.DolbyVision, "hdr10", false},
		// HDR10+
		{release.HDR10Plus, "hdr10+", true},
		{release.HDR10Plus, "hdr10plus", true},
		{release.HDR10Plus, "hdr10", false},
		// HDR10
		{release.HDR10, "hdr10", true},
		{release.HDR10, "HDR10", true},
		{release.HDR10, "hdr", false},
		// Generic HDR
		{release.HDRGeneric, "hdr", true},
		{release.HDRGeneric, "hdr10", false},
		// HLG
		{release.HLG, "hlg", true},
		{release.HLG, "HLG", true},
		// None
		{release.HDRNone, "hdr", false},
	}

	for _, tt := range tests {
		name := tt.hdr.String() + "_" + tt.pref
		t.Run(name, func(t *testing.T) {
			got := HDRMatches(tt.hdr, tt.pref)
			if got != tt.want {
				t.Errorf("HDRMatches(%v, %q) = %v, want %v", tt.hdr, tt.pref, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/scoring/... -v`
Expected: FAIL - undefined: HDRMatches

**Step 3: Write minimal implementation**

Add to `pkg/release/scoring/scoring.go`:

```go
import "strings"

// HDRMatches checks if an HDR format matches a preference string.
func HDRMatches(hdr release.HDRFormat, pref string) bool {
	prefLower := strings.ToLower(pref)
	switch hdr {
	case release.DolbyVision:
		return prefLower == "dolby-vision" || prefLower == "dv" || prefLower == "dolbyvision"
	case release.HDR10Plus:
		return prefLower == "hdr10+" || prefLower == "hdr10plus"
	case release.HDR10:
		return prefLower == "hdr10"
	case release.HDRGeneric:
		return prefLower == "hdr"
	case release.HLG:
		return prefLower == "hlg"
	default:
		return false
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/scoring/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/scoring/scoring.go pkg/release/scoring/scoring_test.go
git commit -m "feat(scoring): add HDRMatches function"
```

---

### Task 4: Add AudioMatches function

**Files:**
- Modify: `pkg/release/scoring/scoring.go`
- Modify: `pkg/release/scoring/scoring_test.go`

**Step 1: Write the failing test**

```go
// Add to pkg/release/scoring/scoring_test.go

func TestAudioMatches(t *testing.T) {
	tests := []struct {
		audio release.AudioCodec
		pref  string
		want  bool
	}{
		// Atmos
		{release.AudioAtmos, "atmos", true},
		{release.AudioAtmos, "ATMOS", true},
		{release.AudioAtmos, "truehd", false},
		// TrueHD
		{release.AudioTrueHD, "truehd", true},
		{release.AudioTrueHD, "TrueHD", true},
		// DTS-HD
		{release.AudioDTSHD, "dtshd", true},
		{release.AudioDTSHD, "dts-hd", true},
		{release.AudioDTSHD, "dts-hd ma", true},
		{release.AudioDTSHD, "dts", false},
		// DTS
		{release.AudioDTS, "dts", true},
		{release.AudioDTS, "dtshd", false},
		// EAC3 (DD+)
		{release.AudioEAC3, "dd+", true},
		{release.AudioEAC3, "ddp", true},
		{release.AudioEAC3, "eac3", true},
		// AC3 (DD)
		{release.AudioAC3, "dd", true},
		{release.AudioAC3, "ac3", true},
		// AAC
		{release.AudioAAC, "aac", true},
		// FLAC
		{release.AudioFLAC, "flac", true},
		// Opus
		{release.AudioOpus, "opus", true},
		// Unknown
		{release.AudioUnknown, "aac", false},
	}

	for _, tt := range tests {
		name := tt.audio.String() + "_" + tt.pref
		t.Run(name, func(t *testing.T) {
			got := AudioMatches(tt.audio, tt.pref)
			if got != tt.want {
				t.Errorf("AudioMatches(%v, %q) = %v, want %v", tt.audio, tt.pref, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/scoring/... -v`
Expected: FAIL - undefined: AudioMatches

**Step 3: Write minimal implementation**

Add to `pkg/release/scoring/scoring.go`:

```go
// AudioMatches checks if an audio codec matches a preference string.
func AudioMatches(audio release.AudioCodec, pref string) bool {
	prefLower := strings.ToLower(pref)
	switch audio {
	case release.AudioAtmos:
		return prefLower == "atmos"
	case release.AudioTrueHD:
		return prefLower == "truehd"
	case release.AudioDTSHD:
		return prefLower == "dtshd" || prefLower == "dts-hd" || prefLower == "dts-hd ma"
	case release.AudioDTS:
		return prefLower == "dts"
	case release.AudioEAC3:
		return prefLower == "dd+" || prefLower == "ddp" || prefLower == "eac3"
	case release.AudioAC3:
		return prefLower == "dd" || prefLower == "ac3"
	case release.AudioAAC:
		return prefLower == "aac"
	case release.AudioFLAC:
		return prefLower == "flac"
	case release.AudioOpus:
		return prefLower == "opus"
	default:
		return false
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/scoring/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/scoring/scoring.go pkg/release/scoring/scoring_test.go
git commit -m "feat(scoring): add AudioMatches function"
```

---

### Task 5: Add MatchesRejectList function

**Files:**
- Modify: `pkg/release/scoring/scoring.go`
- Modify: `pkg/release/scoring/scoring_test.go`

**Step 1: Write the failing test**

```go
// Add to pkg/release/scoring/scoring_test.go

func TestMatchesRejectList(t *testing.T) {
	tests := []struct {
		name       string
		info       release.Info
		rejectList []string
		want       bool
	}{
		{
			name:       "empty reject list",
			info:       release.Info{Resolution: release.Resolution1080p},
			rejectList: nil,
			want:       false,
		},
		{
			name:       "resolution rejected",
			info:       release.Info{Resolution: release.Resolution720p},
			rejectList: []string{"720p"},
			want:       true,
		},
		{
			name:       "source rejected",
			info:       release.Info{Source: release.SourceCAM},
			rejectList: []string{"cam"},
			want:       true,
		},
		{
			name:       "codec rejected",
			info:       release.Info{Codec: release.CodecX264},
			rejectList: []string{"x264"},
			want:       true,
		},
		{
			name:       "hdr rejected",
			info:       release.Info{HDR: release.DolbyVision},
			rejectList: []string{"dv"},
			want:       true,
		},
		{
			name:       "audio rejected",
			info:       release.Info{Audio: release.AudioAAC},
			rejectList: []string{"aac"},
			want:       true,
		},
		{
			name:       "remux special case",
			info:       release.Info{IsRemux: true},
			rejectList: []string{"remux"},
			want:       true,
		},
		{
			name:       "cam alias",
			info:       release.Info{Source: release.SourceCAM},
			rejectList: []string{"camrip"},
			want:       true,
		},
		{
			name:       "telesync alias",
			info:       release.Info{Source: release.SourceTelesync},
			rejectList: []string{"ts"},
			want:       true,
		},
		{
			name:       "no match",
			info:       release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			rejectList: []string{"720p", "cam"},
			want:       false,
		},
		{
			name:       "case insensitive",
			info:       release.Info{Resolution: release.Resolution720p},
			rejectList: []string{"720P"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesRejectList(tt.info, tt.rejectList)
			if got != tt.want {
				t.Errorf("MatchesRejectList() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/scoring/... -v`
Expected: FAIL - undefined: MatchesRejectList

**Step 3: Write minimal implementation**

Add to `pkg/release/scoring/scoring.go`:

```go
// MatchesRejectList checks if a release matches any reject criteria.
func MatchesRejectList(info release.Info, rejectList []string) bool {
	if len(rejectList) == 0 {
		return false
	}

	// Build lowercase set of release attributes
	attrs := []string{
		strings.ToLower(info.Resolution.String()),
		strings.ToLower(info.Source.String()),
		strings.ToLower(info.Codec.String()),
	}

	// Add HDR format if present
	if info.HDR != release.HDRNone {
		attrs = append(attrs, strings.ToLower(info.HDR.String()))
	}

	// Add audio codec if present
	if info.Audio != release.AudioUnknown {
		attrs = append(attrs, strings.ToLower(info.Audio.String()))
	}

	// Check each reject term
	for _, reject := range rejectList {
		rejectLower := strings.ToLower(reject)
		for _, attr := range attrs {
			if attr == rejectLower {
				return true
			}
		}
		// Also check special cases for reject list
		if rejectMatchesSpecial(info, rejectLower) {
			return true
		}
	}

	return false
}

// rejectMatchesSpecial handles special reject list matching.
func rejectMatchesSpecial(info release.Info, reject string) bool {
	switch reject {
	case "cam", "camrip", "hdcam":
		return info.Source == release.SourceCAM
	case "ts", "telesync", "hdts":
		return info.Source == release.SourceTelesync
	case "hdtv":
		return info.Source == release.SourceHDTV
	case "webrip":
		return info.Source == release.SourceWEBRip
	case "remux":
		return info.IsRemux
	case "x264", "h264":
		return info.Codec == release.CodecX264
	case "x265", "h265", "hevc":
		return info.Codec == release.CodecX265
	}
	return false
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/scoring/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/scoring/scoring.go pkg/release/scoring/scoring_test.go
git commit -m "feat(scoring): add MatchesRejectList function"
```

---

### Task 6: Update internal/search/scorer.go to use shared package

**Files:**
- Modify: `internal/search/scorer.go`

**Step 1: Run existing tests to verify they pass before refactoring**

Run: `go test ./internal/search/... -v`
Expected: PASS (establish baseline)

**Step 2: Update imports and replace duplicated code**

Replace the entire `internal/search/scorer.go` with:

```go
package search

import (
	"strings"

	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/release"
	"github.com/vmunix/arrgo/pkg/release/scoring"
)

// Scorer scores releases against quality profiles.
type Scorer struct {
	profiles map[string]config.QualityProfile
}

// NewScorer creates a new Scorer from config profiles.
func NewScorer(profiles map[string]config.QualityProfile) *Scorer {
	return &Scorer{
		profiles: profiles,
	}
}

// Score returns the quality score for a release in the given profile.
func (s *Scorer) Score(info release.Info, profile string) int {
	p, ok := s.profiles[profile]
	if !ok {
		return 0
	}

	// Check reject list first
	if scoring.MatchesRejectList(info, p.Reject) {
		return 0
	}

	// Check resolution requirement
	baseScore := calculateBaseScore(info, p.Resolution)
	if baseScore == 0 {
		return 0
	}

	score := baseScore

	// Add bonuses for matching attributes
	score += calculatePositionBonus(info.Source.String(), p.Sources, scoring.BonusSource)
	score += calculatePositionBonus(info.Codec.String(), p.Codecs, scoring.BonusCodec)
	score += calculateHDRBonus(info.HDR, p.HDR)
	score += calculateAudioBonus(info.Audio, p.Audio)

	// Remux bonus
	if p.PreferRemux && info.IsRemux {
		score += scoring.BonusRemux
	}

	return score
}

// calculateBaseScore returns the base score for a resolution.
func calculateBaseScore(info release.Info, profileResolutions []string) int {
	if len(profileResolutions) == 0 {
		return scoring.ResolutionBaseScore(info.Resolution)
	}

	releaseRes := info.Resolution.String()
	for _, res := range profileResolutions {
		if strings.EqualFold(releaseRes, res) {
			return scoring.ResolutionBaseScore(info.Resolution)
		}
	}

	return 0
}

// calculatePositionBonus calculates bonus points based on position in preference list.
func calculatePositionBonus(value string, preferences []string, baseBonus int) int {
	if len(preferences) == 0 || value == "" || value == "unknown" {
		return 0
	}

	valueLower := strings.ToLower(value)
	for i, pref := range preferences {
		if strings.EqualFold(valueLower, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			return int(float64(baseBonus) * multiplier)
		}
	}

	return 0
}

// calculateHDRBonus calculates bonus for HDR format matching.
func calculateHDRBonus(hdr release.HDRFormat, preferences []string) int {
	if len(preferences) == 0 || hdr == release.HDRNone {
		return 0
	}

	for i, pref := range preferences {
		if scoring.HDRMatches(hdr, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			return int(float64(scoring.BonusHDR) * multiplier)
		}
	}

	return 0
}

// calculateAudioBonus calculates bonus for audio codec matching.
func calculateAudioBonus(audio release.AudioCodec, preferences []string) int {
	if len(preferences) == 0 || audio == release.AudioUnknown {
		return 0
	}

	for i, pref := range preferences {
		if scoring.AudioMatches(audio, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			return int(float64(scoring.BonusAudio) * multiplier)
		}
	}

	return 0
}
```

**Step 3: Run tests to verify refactoring preserved behavior**

Run: `go test ./internal/search/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/search/scorer.go
git commit -m "refactor(search): use shared scoring package"
```

---

### Task 7: Update cmd/arrgo/parse.go to use shared package

**Files:**
- Modify: `cmd/arrgo/parse.go`

**Step 1: Run existing tests to verify they pass before refactoring**

Run: `go test ./cmd/arrgo/... -v`
Expected: PASS (establish baseline)

**Step 2: Update imports**

Replace imports section:

```go
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/release"
	"github.com/vmunix/arrgo/pkg/release/scoring"
)
```

**Step 3: Remove duplicated constants**

Delete these lines (approximately lines 221-238):

```go
// Score constants (matching internal/search/scorer.go)
const (
	scoreResolution2160p = 100
	scoreResolution1080p = 80
	scoreResolution720p  = 60
	scoreResolutionOther = 40
	bonusSource          = 10
	bonusCodec           = 10
	bonusHDR             = 15
	bonusAudio           = 15
	bonusRemux           = 20
)
```

**Step 4: Update scoreWithBreakdown to use shared constants**

Replace `scoreWithBreakdown` function:

```go
// scoreWithBreakdown calculates the score and returns a detailed breakdown.
func scoreWithBreakdown(info release.Info, profile config.QualityProfile) (int, []ScoreBonus) {
	// Check reject list first
	if scoring.MatchesRejectList(info, profile.Reject) {
		return 0, []ScoreBonus{{
			Attribute: "Reject",
			Value:     "matched reject list",
			Position:  -1,
			Bonus:     0,
			Note:      "release rejected",
		}}
	}

	var breakdown []ScoreBonus
	totalScore := 0

	// Check resolution
	resScore, resBonus := scoreResolution(info.Resolution, profile.Resolution)
	if resBonus.Bonus > 0 || resScore > 0 {
		breakdown = append(breakdown, resBonus)
		totalScore += resScore
	}
	if resScore == 0 && len(profile.Resolution) > 0 {
		return 0, []ScoreBonus{{
			Attribute: "Resolution",
			Value:     info.Resolution.String(),
			Position:  -1,
			Bonus:     0,
			Note:      "not in allowed list",
		}}
	}

	// Source bonus
	if bonus := scoreAttribute(info.Source.String(), profile.Sources, scoring.BonusSource, "Source"); bonus.Bonus > 0 || info.Source != release.SourceUnknown {
		if bonus.Value != valueUnknown {
			breakdown = append(breakdown, bonus)
			totalScore += bonus.Bonus
		}
	}

	// Codec bonus
	if bonus := scoreAttribute(info.Codec.String(), profile.Codecs, scoring.BonusCodec, "Codec"); bonus.Bonus > 0 || info.Codec != release.CodecUnknown {
		if bonus.Value != valueUnknown {
			breakdown = append(breakdown, bonus)
			totalScore += bonus.Bonus
		}
	}

	// HDR bonus
	if info.HDR != release.HDRNone {
		bonus := scoreHDR(info.HDR, profile.HDR)
		breakdown = append(breakdown, bonus)
		totalScore += bonus.Bonus
	}

	// Audio bonus
	if info.Audio != release.AudioUnknown {
		bonus := scoreAudioCodec(info.Audio, profile.Audio)
		breakdown = append(breakdown, bonus)
		totalScore += bonus.Bonus
	}

	// Remux bonus
	if info.IsRemux && profile.PreferRemux {
		breakdown = append(breakdown, ScoreBonus{
			Attribute: "Remux",
			Value:     "yes",
			Position:  -1,
			Bonus:     scoring.BonusRemux,
			Note:      "preferred",
		})
		totalScore += scoring.BonusRemux
	}

	return totalScore, breakdown
}
```

**Step 5: Update scoreResolution to use shared function**

Replace `scoreResolution` function:

```go
// scoreResolution returns the base resolution score and breakdown entry.
func scoreResolution(res release.Resolution, preferences []string) (int, ScoreBonus) {
	resStr := res.String()
	baseScore := scoring.ResolutionBaseScore(res)

	bonus := ScoreBonus{
		Attribute: "Resolution",
		Value:     resStr,
		Position:  -1,
		Bonus:     baseScore,
	}

	if len(preferences) == 0 {
		bonus.Note = "no restrictions"
		return baseScore, bonus
	}

	for i, pref := range preferences {
		if strings.EqualFold(resStr, pref) {
			bonus.Position = i
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return baseScore, bonus
		}
	}

	bonus.Bonus = 0
	bonus.Note = "not in allowed list"
	return 0, bonus
}
```

**Step 6: Delete resolutionBaseScore function**

Remove the entire `resolutionBaseScore` function (it's now in the shared package).

**Step 7: Update scoreAttribute to use shared constant**

No changes needed - it already receives the bonus as a parameter.

**Step 8: Update scoreHDR to use shared function**

Replace `scoreHDR` function:

```go
// scoreHDR calculates HDR bonus with position awareness.
func scoreHDR(hdr release.HDRFormat, preferences []string) ScoreBonus {
	bonus := ScoreBonus{
		Attribute: "HDR",
		Value:     hdr.String(),
		Position:  -1,
		Bonus:     0,
	}

	if len(preferences) == 0 {
		return bonus
	}

	for i, pref := range preferences {
		if scoring.HDRMatches(hdr, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			bonus.Position = i
			bonus.Bonus = int(float64(scoring.BonusHDR) * multiplier)
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return bonus
		}
	}

	bonus.Note = noteNotInPrefList
	return bonus
}
```

**Step 9: Delete hdrMatches function**

Remove the entire `hdrMatches` function (it's now in the shared package).

**Step 10: Update scoreAudioCodec to use shared function**

Replace `scoreAudioCodec` function:

```go
// scoreAudioCodec calculates audio bonus with position awareness.
func scoreAudioCodec(audio release.AudioCodec, preferences []string) ScoreBonus {
	bonus := ScoreBonus{
		Attribute: "Audio",
		Value:     audio.String(),
		Position:  -1,
		Bonus:     0,
	}

	if len(preferences) == 0 {
		return bonus
	}

	for i, pref := range preferences {
		if scoring.AudioMatches(audio, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			bonus.Position = i
			bonus.Bonus = int(float64(scoring.BonusAudio) * multiplier)
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return bonus
		}
	}

	bonus.Note = noteNotInPrefList
	return bonus
}
```

**Step 11: Delete audioMatches function**

Remove the entire `audioMatches` function (it's now in the shared package).

**Step 12: Delete matchesRejectList function**

Remove the entire `matchesRejectList` function (it's now in the shared package).

**Step 13: Delete rejectMatchesSpecial function**

Remove the entire `rejectMatchesSpecial` function (it's now in the shared package).

**Step 14: Run tests to verify refactoring preserved behavior**

Run: `go test ./cmd/arrgo/... -v`
Expected: PASS

**Step 15: Commit**

```bash
git add cmd/arrgo/parse.go
git commit -m "refactor(cli): use shared scoring package"
```

---

### Task 8: Run full test suite and verify

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 2: Run linter**

Run: `task lint` or `golangci-lint run`
Expected: No errors

**Step 3: Manual verification**

Run: `go build ./cmd/arrgo && ./arrgo parse --score hd "Movie.2024.1080p.BluRay.x264-GROUP"`
Expected: Should show score breakdown with correct values

**Step 4: Final commit if any cleanup needed**

If any issues found, fix and commit.

---

### Task 9: Close the issue

**Step 1: Close GitHub issue**

Run: `gh issue close 13 --comment "Resolved in commits. Scoring logic extracted to pkg/release/scoring package."`

---

## Summary

After completing all tasks:

1. New package `pkg/release/scoring` contains:
   - Score constants (exported)
   - `ResolutionBaseScore()`
   - `HDRMatches()`
   - `AudioMatches()`
   - `MatchesRejectList()` (with internal `rejectMatchesSpecial`)

2. `internal/search/scorer.go` imports and uses shared package

3. `cmd/arrgo/parse.go` imports and uses shared package

4. No duplicated code remains
