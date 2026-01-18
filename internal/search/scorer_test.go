package search

import (
	"testing"

	"github.com/arrgo/arrgo/pkg/release"
)

func TestParseQualitySpec(t *testing.T) {
	tests := []struct {
		input   string
		wantRes release.Resolution
		wantSrc release.Source
	}{
		{"1080p bluray", release.Resolution1080p, release.SourceBluRay},
		{"1080p webdl", release.Resolution1080p, release.SourceWEBDL},
		{"720p", release.Resolution720p, release.SourceUnknown},
		{"2160p bluray", release.Resolution2160p, release.SourceBluRay},
		{"720p hdtv", release.Resolution720p, release.SourceHDTV},
		{"1080p webrip", release.Resolution1080p, release.SourceWEBRip},
		{"2160p", release.Resolution2160p, release.SourceUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			spec := ParseQualitySpec(tt.input)
			if spec.Resolution != tt.wantRes {
				t.Errorf("ParseQualitySpec(%q).Resolution = %v, want %v", tt.input, spec.Resolution, tt.wantRes)
			}
			if spec.Source != tt.wantSrc {
				t.Errorf("ParseQualitySpec(%q).Source = %v, want %v", tt.input, spec.Source, tt.wantSrc)
			}
		})
	}
}

func TestQualitySpec_Matches(t *testing.T) {
	tests := []struct {
		name string
		spec QualitySpec
		info release.Info
		want bool
	}{
		{
			name: "exact match 1080p bluray",
			spec: QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			info: release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			want: true,
		},
		{
			name: "resolution only matches any source",
			spec: QualitySpec{Resolution: release.Resolution720p, Source: release.SourceUnknown},
			info: release.Info{Resolution: release.Resolution720p, Source: release.SourceBluRay},
			want: true,
		},
		{
			name: "resolution only matches unknown source",
			spec: QualitySpec{Resolution: release.Resolution720p, Source: release.SourceUnknown},
			info: release.Info{Resolution: release.Resolution720p, Source: release.SourceUnknown},
			want: true,
		},
		{
			name: "resolution mismatch",
			spec: QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			info: release.Info{Resolution: release.Resolution720p, Source: release.SourceBluRay},
			want: false,
		},
		{
			name: "source mismatch",
			spec: QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			info: release.Info{Resolution: release.Resolution1080p, Source: release.SourceWEBDL},
			want: false,
		},
		{
			name: "both mismatch",
			spec: QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			info: release.Info{Resolution: release.Resolution720p, Source: release.SourceHDTV},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.Matches(tt.info)
			if got != tt.want {
				t.Errorf("QualitySpec.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScorer_Score(t *testing.T) {
	profiles := map[string][]string{
		"hd": {"1080p bluray", "1080p webdl", "1080p hdtv", "720p bluray"},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name    string
		info    release.Info
		profile string
		want    int
	}{
		{
			name:    "first choice gets highest score",
			info:    release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			profile: "hd",
			want:    4, // len(specs) - 0 = 4
		},
		{
			name:    "second choice gets second highest",
			info:    release.Info{Resolution: release.Resolution1080p, Source: release.SourceWEBDL},
			profile: "hd",
			want:    3, // len(specs) - 1 = 3
		},
		{
			name:    "third choice",
			info:    release.Info{Resolution: release.Resolution1080p, Source: release.SourceHDTV},
			profile: "hd",
			want:    2, // len(specs) - 2 = 2
		},
		{
			name:    "fourth choice (last)",
			info:    release.Info{Resolution: release.Resolution720p, Source: release.SourceBluRay},
			profile: "hd",
			want:    1, // len(specs) - 3 = 1
		},
		{
			name:    "no match returns 0",
			info:    release.Info{Resolution: release.Resolution720p, Source: release.SourceHDTV},
			profile: "hd",
			want:    0,
		},
		{
			name:    "unknown profile returns 0",
			info:    release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			profile: "nonexistent",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scorer.Score(tt.info, tt.profile)
			if got != tt.want {
				t.Errorf("Scorer.Score() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScorer_Score_ResolutionOnly(t *testing.T) {
	// Test profile with resolution-only specs
	profiles := map[string][]string{
		"any-source": {"1080p", "720p"},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name string
		info release.Info
		want int
	}{
		{
			name: "1080p bluray matches first",
			info: release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			want: 2,
		},
		{
			name: "1080p webdl also matches first",
			info: release.Info{Resolution: release.Resolution1080p, Source: release.SourceWEBDL},
			want: 2,
		},
		{
			name: "720p anything matches second",
			info: release.Info{Resolution: release.Resolution720p, Source: release.SourceHDTV},
			want: 1,
		},
		{
			name: "2160p no match",
			info: release.Info{Resolution: release.Resolution2160p, Source: release.SourceBluRay},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scorer.Score(tt.info, "any-source")
			if got != tt.want {
				t.Errorf("Scorer.Score() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewScorer_MultipleProfiles(t *testing.T) {
	profiles := map[string][]string{
		"uhd":    {"2160p bluray", "2160p webdl"},
		"hd":     {"1080p bluray", "1080p webdl"},
		"any-hd": {"1080p"},
	}
	scorer := NewScorer(profiles)

	// Same release should score differently in different profiles
	info := release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay}

	uhdScore := scorer.Score(info, "uhd")
	hdScore := scorer.Score(info, "hd")
	anyHdScore := scorer.Score(info, "any-hd")

	if uhdScore != 0 {
		t.Errorf("1080p bluray in uhd profile should be 0, got %d", uhdScore)
	}
	if hdScore != 2 { // len(2) - 0 = 2
		t.Errorf("1080p bluray in hd profile should be 2, got %d", hdScore)
	}
	if anyHdScore != 1 { // len(1) - 0 = 1
		t.Errorf("1080p bluray in any-hd profile should be 1, got %d", anyHdScore)
	}
}
