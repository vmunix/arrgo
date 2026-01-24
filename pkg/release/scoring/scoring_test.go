// pkg/release/scoring/scoring_test.go
package scoring

import (
	"testing"

	"github.com/vmunix/arrgo/pkg/release"
)

func TestScoreConstants(t *testing.T) {
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
