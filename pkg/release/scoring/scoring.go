// Package scoring provides shared scoring constants and matching functions
// for quality profile evaluation.
package scoring

import (
	"strings"

	"github.com/vmunix/arrgo/pkg/release"
)

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
