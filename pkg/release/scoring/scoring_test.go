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
