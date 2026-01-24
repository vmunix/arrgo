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
