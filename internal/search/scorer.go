package search

import (
	"strings"

	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/release"
)

// Base scores for resolutions.
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

// Scorer scores releases against quality profiles.
type Scorer struct {
	profiles map[string]config.QualityProfile
}

// NewScorer creates a new Scorer from config profiles.
// Profiles is a map where key is profile name and value is a QualityProfile.
func NewScorer(profiles map[string]config.QualityProfile) *Scorer {
	return &Scorer{
		profiles: profiles,
	}
}

// Score returns the quality score for a release in the given profile.
// Returns 0 if no match (release should be filtered out).
// Scoring algorithm:
//   - Base score from resolution match (2160p=100, 1080p=80, 720p=60, unknown=40)
//   - Bonuses for source, codec, HDR, audio matches (position-adjusted)
//   - Remux bonus if prefer_remux is enabled
//   - Reject list causes score of 0 (filtered out)
func (s *Scorer) Score(info release.Info, profile string) int {
	p, ok := s.profiles[profile]
	if !ok {
		return 0
	}

	// Check reject list first
	if matchesRejectList(info, p.Reject) {
		return 0
	}

	// Check resolution requirement
	baseScore := calculateBaseScore(info, p.Resolution)
	if baseScore == 0 {
		return 0
	}

	score := baseScore

	// Add bonuses for matching attributes
	score += calculatePositionBonus(info.Source.String(), p.Sources, bonusSource)
	score += calculatePositionBonus(info.Codec.String(), p.Codecs, bonusCodec)
	score += calculateHDRBonus(info.HDR, p.HDR)
	score += calculateAudioBonus(info.Audio, p.Audio)

	// Remux bonus
	if p.PreferRemux && info.IsRemux {
		score += bonusRemux
	}

	return score
}

// calculateBaseScore returns the base score for a resolution.
// If the profile specifies resolutions, the release must match one of them.
// If no resolutions specified, any resolution gets its natural base score.
func calculateBaseScore(info release.Info, profileResolutions []string) int {
	// If no resolution requirement, give natural base score
	if len(profileResolutions) == 0 {
		return resolutionBaseScore(info.Resolution)
	}

	// Check if release resolution matches any profile resolution
	releaseRes := info.Resolution.String()
	for _, res := range profileResolutions {
		if strings.EqualFold(releaseRes, res) {
			return resolutionBaseScore(info.Resolution)
		}
	}

	// Resolution not in allowed list
	return 0
}

// resolutionBaseScore returns the base score for a given resolution.
func resolutionBaseScore(r release.Resolution) int {
	switch r {
	case release.Resolution2160p:
		return scoreResolution2160p
	case release.Resolution1080p:
		return scoreResolution1080p
	case release.Resolution720p:
		return scoreResolution720p
	default:
		return scoreResolutionOther
	}
}

// calculatePositionBonus calculates bonus points based on position in preference list.
// Formula: bonus * (1 - 0.2 * position) where position is 0-indexed.
// Returns 0 if no match or empty preference list.
func calculatePositionBonus(value string, preferences []string, baseBonus int) int {
	if len(preferences) == 0 || value == "" || value == "unknown" {
		return 0
	}

	valueLower := strings.ToLower(value)
	for i, pref := range preferences {
		if matchesPreference(valueLower, pref) {
			// bonus * (1 - 0.2 * position)
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			return int(float64(baseBonus) * multiplier)
		}
	}

	return 0
}

// matchesPreference checks if a release value matches a profile preference.
func matchesPreference(value, pref string) bool {
	prefLower := strings.ToLower(pref)
	return value == prefLower
}

// calculateHDRBonus calculates bonus for HDR format matching.
func calculateHDRBonus(hdr release.HDRFormat, preferences []string) int {
	if len(preferences) == 0 || hdr == release.HDRNone {
		return 0
	}

	for i, pref := range preferences {
		if hdrMatches(hdr, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			return int(float64(bonusHDR) * multiplier)
		}
	}

	return 0
}

// hdrMatches checks if an HDR format matches a preference string.
func hdrMatches(hdr release.HDRFormat, pref string) bool {
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

// calculateAudioBonus calculates bonus for audio codec matching.
func calculateAudioBonus(audio release.AudioCodec, preferences []string) int {
	if len(preferences) == 0 || audio == release.AudioUnknown {
		return 0
	}

	for i, pref := range preferences {
		if audioMatches(audio, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			return int(float64(bonusAudio) * multiplier)
		}
	}

	return 0
}

// audioMatches checks if an audio codec matches a preference string.
func audioMatches(audio release.AudioCodec, pref string) bool {
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

// matchesRejectList checks if a release matches any reject criteria.
func matchesRejectList(info release.Info, rejectList []string) bool {
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
// Note: cam, camrip, ts, telesync, hdcam are not currently tracked by the parser.
// These would require extending pkg/release to detect these low-quality sources.
func rejectMatchesSpecial(info release.Info, reject string) bool {
	// Handle common reject patterns
	switch reject {
	case "cam", "camrip", "ts", "telesync", "hdcam":
		// Low-quality sources not currently tracked by parser - future enhancement
		return false
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
