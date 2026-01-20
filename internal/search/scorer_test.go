package search

import (
	"testing"

	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/release"
)

func TestScorer_Score_BaseResolution(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"any": {
			Resolution: []string{"2160p", "1080p", "720p"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name       string
		resolution release.Resolution
		wantScore  int
	}{
		{"2160p gets 100", release.Resolution2160p, 100},
		{"1080p gets 80", release.Resolution1080p, 80},
		{"720p gets 60", release.Resolution720p, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{Resolution: tt.resolution}
			got := scorer.Score(info, "any")
			if got != tt.wantScore {
				t.Errorf("Score() = %v, want %v", got, tt.wantScore)
			}
		})
	}
}

func TestScorer_Score_ResolutionMustMatch(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd-only": {
			Resolution: []string{"1080p"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name       string
		resolution release.Resolution
		wantScore  int
	}{
		{"1080p matches profile", release.Resolution1080p, 80},
		{"720p not in profile", release.Resolution720p, 0},
		{"2160p not in profile", release.Resolution2160p, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{Resolution: tt.resolution}
			got := scorer.Score(info, "hd-only")
			if got != tt.wantScore {
				t.Errorf("Score() = %v, want %v", got, tt.wantScore)
			}
		})
	}
}

func TestScorer_Score_SourceBonus(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Sources:    []string{"bluray", "webdl", "webrip"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name      string
		source    release.Source
		wantBonus int
	}{
		// Position 0: 10 * (1 - 0.2*0) = 10
		{"bluray at position 0", release.SourceBluRay, 10},
		// Position 1: 10 * (1 - 0.2*1) = 8
		{"webdl at position 1", release.SourceWEBDL, 8},
		// Position 2: 10 * (1 - 0.2*2) = 6
		{"webrip at position 2", release.SourceWEBRip, 6},
		// Not in list: 0
		{"hdtv not in list", release.SourceHDTV, 0},
		{"unknown source", release.SourceUnknown, 0},
	}

	baseScore := 80 // 1080p base score

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{
				Resolution: release.Resolution1080p,
				Source:     tt.source,
			}
			got := scorer.Score(info, "hd")
			want := baseScore + tt.wantBonus
			if got != want {
				t.Errorf("Score() = %v, want %v (base=%d + bonus=%d)", got, want, baseScore, tt.wantBonus)
			}
		})
	}
}

func TestScorer_Score_CodecBonus(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Codecs:     []string{"x265", "x264"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name      string
		codec     release.Codec
		wantBonus int
	}{
		// Position 0: 10 * (1 - 0.2*0) = 10
		{"x265 at position 0", release.CodecX265, 10},
		// Position 1: 10 * (1 - 0.2*1) = 8
		{"x264 at position 1", release.CodecX264, 8},
		// Not in list: 0
		{"unknown codec", release.CodecUnknown, 0},
	}

	baseScore := 80 // 1080p base score

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{
				Resolution: release.Resolution1080p,
				Codec:      tt.codec,
			}
			got := scorer.Score(info, "hd")
			want := baseScore + tt.wantBonus
			if got != want {
				t.Errorf("Score() = %v, want %v (base=%d + bonus=%d)", got, want, baseScore, tt.wantBonus)
			}
		})
	}
}

func TestScorer_Score_HDRBonus(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"uhd": {
			Resolution: []string{"2160p"},
			HDR:        []string{"dolby-vision", "hdr10+", "hdr10"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name      string
		hdr       release.HDRFormat
		wantBonus int
	}{
		// Position 0: 15 * (1 - 0.2*0) = 15
		{"dolby-vision at position 0", release.DolbyVision, 15},
		// Position 1: 15 * (1 - 0.2*1) = 12
		{"hdr10+ at position 1", release.HDR10Plus, 12},
		// Position 2: 15 * (1 - 0.2*2) = 9
		{"hdr10 at position 2", release.HDR10, 9},
		// Not in list: 0
		{"hdr generic not in list", release.HDRGeneric, 0},
		{"no hdr", release.HDRNone, 0},
	}

	baseScore := 100 // 2160p base score

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{
				Resolution: release.Resolution2160p,
				HDR:        tt.hdr,
			}
			got := scorer.Score(info, "uhd")
			want := baseScore + tt.wantBonus
			if got != want {
				t.Errorf("Score() = %v, want %v (base=%d + bonus=%d)", got, want, baseScore, tt.wantBonus)
			}
		})
	}
}

func TestScorer_Score_AudioBonus(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Audio:      []string{"atmos", "truehd", "dtshd"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name      string
		audio     release.AudioCodec
		wantBonus int
	}{
		// Position 0: 15 * (1 - 0.2*0) = 15
		{"atmos at position 0", release.AudioAtmos, 15},
		// Position 1: 15 * (1 - 0.2*1) = 12
		{"truehd at position 1", release.AudioTrueHD, 12},
		// Position 2: 15 * (1 - 0.2*2) = 9
		{"dtshd at position 2", release.AudioDTSHD, 9},
		// Not in list: 0
		{"dts not in list", release.AudioDTS, 0},
		{"unknown audio", release.AudioUnknown, 0},
	}

	baseScore := 80 // 1080p base score

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{
				Resolution: release.Resolution1080p,
				Audio:      tt.audio,
			}
			got := scorer.Score(info, "hd")
			want := baseScore + tt.wantBonus
			if got != want {
				t.Errorf("Score() = %v, want %v (base=%d + bonus=%d)", got, want, baseScore, tt.wantBonus)
			}
		})
	}
}

func TestScorer_Score_RemuxBonus(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"remux-preferred": {
			Resolution:  []string{"1080p"},
			PreferRemux: true,
		},
		"no-remux-pref": {
			Resolution:  []string{"1080p"},
			PreferRemux: false,
		},
	}
	scorer := NewScorer(profiles)

	baseScore := 80 // 1080p

	t.Run("remux with prefer_remux=true gets bonus", func(t *testing.T) {
		info := release.Info{
			Resolution: release.Resolution1080p,
			IsRemux:    true,
		}
		got := scorer.Score(info, "remux-preferred")
		want := baseScore + 20 // remux bonus
		if got != want {
			t.Errorf("Score() = %v, want %v", got, want)
		}
	})

	t.Run("non-remux with prefer_remux=true gets no bonus", func(t *testing.T) {
		info := release.Info{
			Resolution: release.Resolution1080p,
			IsRemux:    false,
		}
		got := scorer.Score(info, "remux-preferred")
		if got != baseScore {
			t.Errorf("Score() = %v, want %v", got, baseScore)
		}
	})

	t.Run("remux with prefer_remux=false gets no bonus", func(t *testing.T) {
		info := release.Info{
			Resolution: release.Resolution1080p,
			IsRemux:    true,
		}
		got := scorer.Score(info, "no-remux-pref")
		if got != baseScore {
			t.Errorf("Score() = %v, want %v", got, baseScore)
		}
	})
}

func TestScorer_Score_RejectList(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p", "720p"},
			Reject:     []string{"hdtv", "x264"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name   string
		info   release.Info
		reject bool
	}{
		{
			name: "hdtv source rejected",
			info: release.Info{
				Resolution: release.Resolution1080p,
				Source:     release.SourceHDTV,
			},
			reject: true,
		},
		{
			name: "x264 codec rejected",
			info: release.Info{
				Resolution: release.Resolution1080p,
				Codec:      release.CodecX264,
			},
			reject: true,
		},
		{
			name: "bluray x265 not rejected",
			info: release.Info{
				Resolution: release.Resolution1080p,
				Source:     release.SourceBluRay,
				Codec:      release.CodecX265,
			},
			reject: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scorer.Score(tt.info, "hd")
			if tt.reject && got != 0 {
				t.Errorf("Score() = %v, want 0 (rejected)", got)
			}
			if !tt.reject && got == 0 {
				t.Errorf("Score() = 0, want > 0 (not rejected)")
			}
		})
	}
}

func TestScorer_Score_CombinedBonuses(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"uhd": {
			Resolution:  []string{"2160p"},
			Sources:     []string{"bluray", "webdl"},
			Codecs:      []string{"x265"},
			HDR:         []string{"dolby-vision", "hdr10"},
			Audio:       []string{"atmos", "truehd"},
			PreferRemux: true,
		},
	}
	scorer := NewScorer(profiles)

	// Premium release: 2160p bluray x265 dolby-vision atmos remux
	info := release.Info{
		Resolution: release.Resolution2160p,
		Source:     release.SourceBluRay,
		Codec:      release.CodecX265,
		HDR:        release.DolbyVision,
		Audio:      release.AudioAtmos,
		IsRemux:    true,
	}

	got := scorer.Score(info, "uhd")

	// Calculate expected:
	// Base: 100 (2160p)
	// Source: +10 (bluray at position 0)
	// Codec: +10 (x265 at position 0)
	// HDR: +15 (dolby-vision at position 0)
	// Audio: +15 (atmos at position 0)
	// Remux: +20
	// Total: 170
	want := 100 + 10 + 10 + 15 + 15 + 20

	if got != want {
		t.Errorf("Score() = %v, want %v", got, want)
	}
}

func TestScorer_Score_PositionAdjustment(t *testing.T) {
	// Test the position adjustment formula: bonus * (1 - 0.2 * position)
	profiles := map[string]config.QualityProfile{
		"test": {
			Resolution: []string{"1080p"},
			HDR:        []string{"dolby-vision", "hdr10+", "hdr10", "hdr", "hlg"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		hdr       release.HDRFormat
		position  int
		wantBonus int
	}{
		// Position 0: 15 * (1 - 0.2*0) = 15 * 1.0 = 15
		{release.DolbyVision, 0, 15},
		// Position 1: 15 * (1 - 0.2*1) = 15 * 0.8 = 12
		{release.HDR10Plus, 1, 12},
		// Position 2: 15 * (1 - 0.2*2) = 15 * 0.6 = 9
		{release.HDR10, 2, 9},
		// Position 3: 15 * (1 - 0.2*3) = 15 * 0.4 = 5 (due to float truncation)
		{release.HDRGeneric, 3, 5},
		// Position 4: 15 * (1 - 0.2*4) = 15 * 0.2 = 2 (due to float truncation)
		{release.HLG, 4, 2},
	}

	baseScore := 80 // 1080p

	for _, tt := range tests {
		t.Run(tt.hdr.String(), func(t *testing.T) {
			info := release.Info{
				Resolution: release.Resolution1080p,
				HDR:        tt.hdr,
			}
			got := scorer.Score(info, "test")
			want := baseScore + tt.wantBonus
			if got != want {
				t.Errorf("Score() = %v, want %v (bonus=%d at position %d)", got, want, tt.wantBonus, tt.position)
			}
		})
	}
}

func TestScorer_Score_UnknownProfile(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}},
	}
	scorer := NewScorer(profiles)

	info := release.Info{Resolution: release.Resolution1080p}
	got := scorer.Score(info, "nonexistent")

	if got != 0 {
		t.Errorf("Score() = %v, want 0 for unknown profile", got)
	}
}

func TestScorer_Score_EmptyResolutionMeansAny(t *testing.T) {
	// Empty resolution list means any resolution is accepted
	profiles := map[string]config.QualityProfile{
		"any": {
			// No resolution specified
			Sources: []string{"bluray"},
		},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name       string
		resolution release.Resolution
		wantBase   int
	}{
		{"2160p accepted", release.Resolution2160p, 100},
		{"1080p accepted", release.Resolution1080p, 80},
		{"720p accepted", release.Resolution720p, 60},
		{"unknown accepted", release.ResolutionUnknown, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := release.Info{
				Resolution: tt.resolution,
				Source:     release.SourceBluRay,
			}
			got := scorer.Score(info, "any")
			want := tt.wantBase + 10 // bluray bonus
			if got != want {
				t.Errorf("Score() = %v, want %v", got, want)
			}
		})
	}
}

func TestScorer_Score_EmptySourceMeansNoBonus(t *testing.T) {
	// Empty sources list means no source bonus is given
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			// No sources specified
		},
	}
	scorer := NewScorer(profiles)

	// BluRay should get no bonus since no sources are preferred
	info := release.Info{
		Resolution: release.Resolution1080p,
		Source:     release.SourceBluRay,
	}
	got := scorer.Score(info, "hd")
	want := 80 // Just base score, no source bonus

	if got != want {
		t.Errorf("Score() = %v, want %v (no source bonus)", got, want)
	}
}

func TestScorer_Score_MultipleProfiles(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"uhd": {
			Resolution: []string{"2160p"},
		},
		"hd": {
			Resolution: []string{"1080p"},
		},
	}
	scorer := NewScorer(profiles)

	info := release.Info{Resolution: release.Resolution1080p}

	// Same release should score differently in different profiles
	uhdScore := scorer.Score(info, "uhd")
	hdScore := scorer.Score(info, "hd")

	if uhdScore != 0 {
		t.Errorf("1080p in uhd profile should be 0, got %d", uhdScore)
	}
	if hdScore != 80 {
		t.Errorf("1080p in hd profile should be 80, got %d", hdScore)
	}
}

func TestHdrMatches(t *testing.T) {
	tests := []struct {
		hdr   release.HDRFormat
		pref  string
		match bool
	}{
		{release.DolbyVision, "dolby-vision", true},
		{release.DolbyVision, "dv", true},
		{release.DolbyVision, "dolbyvision", true},
		{release.DolbyVision, "hdr10", false},
		{release.HDR10Plus, "hdr10+", true},
		{release.HDR10Plus, "hdr10plus", true},
		{release.HDR10Plus, "hdr10", false},
		{release.HDR10, "hdr10", true},
		{release.HDR10, "hdr", false},
		{release.HDRGeneric, "hdr", true},
		{release.HLG, "hlg", true},
		{release.HDRNone, "hdr", false},
	}

	for _, tt := range tests {
		name := tt.hdr.String() + " matches " + tt.pref
		t.Run(name, func(t *testing.T) {
			got := hdrMatches(tt.hdr, tt.pref)
			if got != tt.match {
				t.Errorf("hdrMatches(%v, %q) = %v, want %v", tt.hdr, tt.pref, got, tt.match)
			}
		})
	}
}

func TestAudioMatches(t *testing.T) {
	tests := []struct {
		audio release.AudioCodec
		pref  string
		match bool
	}{
		{release.AudioAtmos, "atmos", true},
		{release.AudioAtmos, "truehd", false},
		{release.AudioTrueHD, "truehd", true},
		{release.AudioDTSHD, "dtshd", true},
		{release.AudioDTSHD, "dts-hd", true},
		{release.AudioDTSHD, "dts", false},
		{release.AudioDTS, "dts", true},
		{release.AudioEAC3, "dd+", true},
		{release.AudioEAC3, "ddp", true},
		{release.AudioEAC3, "eac3", true},
		{release.AudioAC3, "dd", true},
		{release.AudioAC3, "ac3", true},
		{release.AudioAAC, "aac", true},
		{release.AudioFLAC, "flac", true},
		{release.AudioOpus, "opus", true},
		{release.AudioUnknown, "aac", false},
	}

	for _, tt := range tests {
		name := tt.audio.String() + " matches " + tt.pref
		t.Run(name, func(t *testing.T) {
			got := audioMatches(tt.audio, tt.pref)
			if got != tt.match {
				t.Errorf("audioMatches(%v, %q) = %v, want %v", tt.audio, tt.pref, got, tt.match)
			}
		})
	}
}

func TestMatchesRejectList(t *testing.T) {
	tests := []struct {
		name       string
		info       release.Info
		rejectList []string
		want       bool
	}{
		{
			name:       "empty reject list",
			info:       release.Info{Source: release.SourceHDTV},
			rejectList: nil,
			want:       false,
		},
		{
			name:       "hdtv in reject list",
			info:       release.Info{Source: release.SourceHDTV},
			rejectList: []string{"hdtv"},
			want:       true,
		},
		{
			name:       "x264 in reject list",
			info:       release.Info{Codec: release.CodecX264},
			rejectList: []string{"x264"},
			want:       true,
		},
		{
			name:       "x265 alias h265 in reject list",
			info:       release.Info{Codec: release.CodecX265},
			rejectList: []string{"h265"},
			want:       true,
		},
		{
			name:       "remux in reject list",
			info:       release.Info{IsRemux: true},
			rejectList: []string{"remux"},
			want:       true,
		},
		{
			name:       "resolution in reject list",
			info:       release.Info{Resolution: release.Resolution720p},
			rejectList: []string{"720p"},
			want:       true,
		},
		{
			name:       "no match in reject list",
			info:       release.Info{Source: release.SourceBluRay, Codec: release.CodecX265},
			rejectList: []string{"hdtv", "x264"},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRejectList(tt.info, tt.rejectList)
			if got != tt.want {
				t.Errorf("matchesRejectList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculatePositionBonus(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		preferences []string
		baseBonus   int
		want        int
	}{
		{
			name:        "position 0",
			value:       "bluray",
			preferences: []string{"bluray", "webdl"},
			baseBonus:   10,
			want:        10,
		},
		{
			name:        "position 1",
			value:       "webdl",
			preferences: []string{"bluray", "webdl"},
			baseBonus:   10,
			want:        8,
		},
		{
			name:        "position 2",
			value:       "webrip",
			preferences: []string{"bluray", "webdl", "webrip"},
			baseBonus:   10,
			want:        6,
		},
		{
			name:        "position 3",
			value:       "hdtv",
			preferences: []string{"bluray", "webdl", "webrip", "hdtv"},
			baseBonus:   10,
			want:        3, // 10 * 0.4 = 3 (due to float truncation)
		},
		{
			name:        "position 4",
			value:       "other",
			preferences: []string{"a", "b", "c", "d", "other"},
			baseBonus:   10,
			want:        1, // 10 * 0.2 = 1 (due to float truncation)
		},
		{
			name:        "position 5 (multiplier would be 0)",
			value:       "last",
			preferences: []string{"a", "b", "c", "d", "e", "last"},
			baseBonus:   10,
			want:        0,
		},
		{
			name:        "not in list",
			value:       "notfound",
			preferences: []string{"bluray", "webdl"},
			baseBonus:   10,
			want:        0,
		},
		{
			name:        "empty preferences",
			value:       "bluray",
			preferences: []string{},
			baseBonus:   10,
			want:        0,
		},
		{
			name:        "empty value",
			value:       "",
			preferences: []string{"bluray"},
			baseBonus:   10,
			want:        0,
		},
		{
			name:        "unknown value",
			value:       "unknown",
			preferences: []string{"bluray"},
			baseBonus:   10,
			want:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculatePositionBonus(tt.value, tt.preferences, tt.baseBonus)
			if got != tt.want {
				t.Errorf("calculatePositionBonus() = %v, want %v", got, tt.want)
			}
		})
	}
}
