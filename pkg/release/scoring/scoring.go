// Package scoring provides shared scoring constants and matching functions
// for quality profile evaluation.
package scoring

import "github.com/vmunix/arrgo/pkg/release"

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
