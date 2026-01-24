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
