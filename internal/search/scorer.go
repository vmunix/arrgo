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
