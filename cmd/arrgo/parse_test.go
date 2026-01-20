package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/release"
)

func TestReadReleaseFile(t *testing.T) {
	// Create a temporary file with test data
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "releases.txt")

	content := `Movie.2024.1080p.BluRay.x264-GROUP
# This is a comment
Another.Movie.2023.720p.WEB-DL.x265-TEAM

  Spaced.Movie.2022.2160p.UHD.BluRay.x265-RELEASE
`
	err := os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err, "failed to write test file")

	names, err := readReleaseFile(testFile)
	require.NoError(t, err)

	want := []string{
		"Movie.2024.1080p.BluRay.x264-GROUP",
		"Another.Movie.2023.720p.WEB-DL.x265-TEAM",
		"Spaced.Movie.2022.2160p.UHD.BluRay.x265-RELEASE",
	}

	require.Len(t, names, len(want))

	for i, got := range names {
		assert.Equal(t, want[i], got, "names[%d]", i)
	}
}

func TestReadReleaseFile_NotFound(t *testing.T) {
	_, err := readReleaseFile("/nonexistent/file.txt")
	assert.Error(t, err, "expected error for nonexistent file")
}

func TestReadReleaseFile_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")

	err := os.WriteFile(testFile, []byte(""), 0644)
	require.NoError(t, err, "failed to write test file")

	names, err := readReleaseFile(testFile)
	require.NoError(t, err)

	assert.Empty(t, names)
}

func TestParseResult_ToJSON(t *testing.T) {
	result := ParseResult{
		Info: &release.Info{
			Title:      "The Matrix",
			Year:       1999,
			Resolution: release.Resolution1080p,
			Source:     release.SourceBluRay,
			Codec:      release.CodecX264,
			HDR:        release.HDRNone,
			Audio:      release.AudioDTSHD,
			IsRemux:    true,
			Group:      "FraMeSToR",
			CleanTitle: "the matrix",
		},
		Score:   100,
		Profile: "hd",
		Breakdown: []ScoreBonus{
			{Attribute: "Resolution", Value: "1080p", Position: 0, Bonus: 80},
		},
	}

	jsonResult := result.toJSON()

	assert.Equal(t, "The Matrix", jsonResult.Title)
	assert.Equal(t, 1999, jsonResult.Year)
	assert.Equal(t, "1080p", jsonResult.Resolution)
	assert.Equal(t, "bluray", jsonResult.Source)
	assert.True(t, jsonResult.IsRemux)
	assert.Empty(t, jsonResult.HDR, "HDR should be empty for HDRNone")
	assert.Equal(t, "DTS-HD MA", jsonResult.Audio)
	assert.Equal(t, 100, jsonResult.Score)
	assert.Equal(t, "hd", jsonResult.Profile)
}

func TestParseResult_ToJSON_WithHDR(t *testing.T) {
	result := ParseResult{
		Info: &release.Info{
			Title:      "Movie",
			Resolution: release.Resolution2160p,
			HDR:        release.DolbyVision,
		},
	}

	jsonResult := result.toJSON()

	assert.Equal(t, "DV", jsonResult.HDR)
}

func TestParseResult_ToJSON_SeriesInfo(t *testing.T) {
	result := ParseResult{
		Info: &release.Info{
			Title:      "Show Name",
			Season:     1,
			Episode:    5,
			Resolution: release.Resolution1080p,
		},
	}

	jsonResult := result.toJSON()

	assert.Equal(t, 1, jsonResult.Season)
	assert.Equal(t, 5, jsonResult.Episode)
}

func TestParseResultJSON_Marshal(t *testing.T) {
	result := ParseResultJSON{
		Title:      "Test Movie",
		Year:       2024,
		Resolution: "1080p",
		Source:     "bluray",
		Codec:      "x264",
		IsRemux:    false,
		CleanTitle: "test movie",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Verify key fields are in the JSON
	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"title":"Test Movie"`)
	assert.Contains(t, jsonStr, `"year":2024`)
	assert.Contains(t, jsonStr, `"resolution":"1080p"`)
}

func TestScoreWithBreakdown_BasicScoring(t *testing.T) {
	info := release.Info{
		Resolution: release.Resolution1080p,
		Source:     release.SourceBluRay,
		Codec:      release.CodecX265,
	}

	profile := config.QualityProfile{
		Resolution: []string{"1080p"},
		Sources:    []string{"bluray", "webdl"},
		Codecs:     []string{"x265", "x264"},
	}

	score, breakdown := scoreWithBreakdown(info, profile)

	// Expected: 80 (resolution) + 10 (source @ pos 0) + 10 (codec @ pos 0) = 100
	assert.Equal(t, 100, score)

	require.GreaterOrEqual(t, len(breakdown), 3, "breakdown should have at least 3 entries")
}

func TestScoreWithBreakdown_RejectList(t *testing.T) {
	info := release.Info{
		Resolution: release.Resolution1080p,
		Source:     release.SourceHDTV,
	}

	profile := config.QualityProfile{
		Resolution: []string{"1080p"},
		Reject:     []string{"hdtv"},
	}

	score, breakdown := scoreWithBreakdown(info, profile)

	assert.Equal(t, 0, score, "score should be 0 (rejected)")

	require.Len(t, breakdown, 1)

	assert.Equal(t, "Reject", breakdown[0].Attribute)
}

func TestScoreWithBreakdown_ResolutionNotAllowed(t *testing.T) {
	info := release.Info{
		Resolution: release.Resolution720p,
	}

	profile := config.QualityProfile{
		Resolution: []string{"1080p", "2160p"}, // 720p not allowed
	}

	score, breakdown := scoreWithBreakdown(info, profile)

	assert.Equal(t, 0, score, "score should be 0 (resolution not allowed)")

	require.Len(t, breakdown, 1)

	assert.Equal(t, "not in allowed list", breakdown[0].Note)
}

func TestScoreWithBreakdown_HDRBonus(t *testing.T) {
	info := release.Info{
		Resolution: release.Resolution2160p,
		HDR:        release.DolbyVision,
	}

	profile := config.QualityProfile{
		Resolution: []string{"2160p"},
		HDR:        []string{"dolby-vision", "hdr10"},
	}

	score, breakdown := scoreWithBreakdown(info, profile)

	// Expected: 100 (resolution) + 15 (HDR @ pos 0) = 115
	assert.Equal(t, 115, score)

	// Find HDR in breakdown
	var hdrBonus *ScoreBonus
	for i := range breakdown {
		if breakdown[i].Attribute == "HDR" {
			hdrBonus = &breakdown[i]
			break
		}
	}

	require.NotNil(t, hdrBonus, "HDR not found in breakdown")

	assert.Equal(t, 15, hdrBonus.Bonus)
}

func TestScoreWithBreakdown_RemuxBonus(t *testing.T) {
	info := release.Info{
		Resolution: release.Resolution1080p,
		IsRemux:    true,
	}

	profile := config.QualityProfile{
		Resolution:  []string{"1080p"},
		PreferRemux: true,
	}

	score, breakdown := scoreWithBreakdown(info, profile)

	// Expected score: 80 for resolution, 20 for remux, totaling 100.
	assert.Equal(t, 100, score)

	// Find Remux in breakdown
	var remuxBonus *ScoreBonus
	for i := range breakdown {
		if breakdown[i].Attribute == "Remux" {
			remuxBonus = &breakdown[i]
			break
		}
	}

	require.NotNil(t, remuxBonus, "Remux not found in breakdown")

	assert.Equal(t, 20, remuxBonus.Bonus)
}

func TestScoreResolution(t *testing.T) {
	tests := []struct {
		name        string
		resolution  release.Resolution
		preferences []string
		wantScore   int
		wantNote    string
	}{
		{
			name:        "2160p with no preferences",
			resolution:  release.Resolution2160p,
			preferences: nil,
			wantScore:   100,
			wantNote:    "no restrictions",
		},
		{
			name:        "1080p first choice",
			resolution:  release.Resolution1080p,
			preferences: []string{"1080p", "720p"},
			wantScore:   80,
			wantNote:    "#1 choice",
		},
		{
			name:        "720p second choice",
			resolution:  release.Resolution720p,
			preferences: []string{"1080p", "720p"},
			wantScore:   60,
			wantNote:    "#2 choice",
		},
		{
			name:        "resolution not in list",
			resolution:  release.Resolution720p,
			preferences: []string{"1080p", "2160p"},
			wantScore:   0,
			wantNote:    "not in allowed list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, bonus := scoreResolution(tt.resolution, tt.preferences)
			assert.Equal(t, tt.wantScore, score)
			assert.Equal(t, tt.wantNote, bonus.Note)
		})
	}
}

func TestScoreAttribute(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		preferences []string
		baseBonus   int
		wantBonus   int
		wantNote    string
	}{
		{
			name:        "first position gets full bonus",
			value:       "bluray",
			preferences: []string{"bluray", "webdl"},
			baseBonus:   10,
			wantBonus:   10,
			wantNote:    "#1 choice",
		},
		{
			name:        "second position gets 80%",
			value:       "webdl",
			preferences: []string{"bluray", "webdl"},
			baseBonus:   10,
			wantBonus:   8,
			wantNote:    "#2 choice",
		},
		{
			name:        "unknown value gets 0",
			value:       "unknown",
			preferences: []string{"bluray"},
			baseBonus:   10,
			wantBonus:   0,
			wantNote:    "",
		},
		{
			name:        "empty preferences gets 0",
			value:       "bluray",
			preferences: nil,
			baseBonus:   10,
			wantBonus:   0,
			wantNote:    "",
		},
		{
			name:        "not in list",
			value:       "hdtv",
			preferences: []string{"bluray", "webdl"},
			baseBonus:   10,
			wantBonus:   0,
			wantNote:    "not in preference list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bonus := scoreAttribute(tt.value, tt.preferences, tt.baseBonus, "Test")
			assert.Equal(t, tt.wantBonus, bonus.Bonus)
			assert.Equal(t, tt.wantNote, bonus.Note)
		})
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
		{release.HDR10Plus, "hdr10+", true},
		{release.HDR10Plus, "hdr10plus", true},
		{release.HDR10, "hdr10", true},
		{release.HDRGeneric, "hdr", true},
		{release.HLG, "hlg", true},
		{release.HDRNone, "hdr", false},
		{release.DolbyVision, "hdr10", false},
	}

	for _, tt := range tests {
		name := tt.hdr.String() + "_" + tt.pref
		t.Run(name, func(t *testing.T) {
			got := hdrMatches(tt.hdr, tt.pref)
			assert.Equal(t, tt.match, got, "hdrMatches(%v, %q)", tt.hdr, tt.pref)
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
		{release.AudioTrueHD, "truehd", true},
		{release.AudioDTSHD, "dtshd", true},
		{release.AudioDTSHD, "dts-hd", true},
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
		name := tt.audio.String() + "_" + tt.pref
		t.Run(name, func(t *testing.T) {
			got := audioMatches(tt.audio, tt.pref)
			assert.Equal(t, tt.match, got, "audioMatches(%v, %q)", tt.audio, tt.pref)
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
			name:       "source in reject list",
			info:       release.Info{Source: release.SourceHDTV},
			rejectList: []string{"hdtv"},
			want:       true,
		},
		{
			name:       "codec in reject list",
			info:       release.Info{Codec: release.CodecX264},
			rejectList: []string{"x264"},
			want:       true,
		},
		{
			name:       "resolution in reject list",
			info:       release.Info{Resolution: release.Resolution720p},
			rejectList: []string{"720p"},
			want:       true,
		},
		{
			name:       "remux in reject list",
			info:       release.Info{IsRemux: true},
			rejectList: []string{"remux"},
			want:       true,
		},
		{
			name:       "CAM in reject list",
			info:       release.Info{Source: release.SourceCAM},
			rejectList: []string{"cam"},
			want:       true,
		},
		{
			name:       "telesync in reject list",
			info:       release.Info{Source: release.SourceTelesync},
			rejectList: []string{"ts"},
			want:       true,
		},
		{
			name:       "no match",
			info:       release.Info{Source: release.SourceBluRay, Codec: release.CodecX265},
			rejectList: []string{"hdtv", "x264"},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRejectList(tt.info, tt.rejectList)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSourceDisplayName(t *testing.T) {
	tests := []struct {
		source release.Source
		want   string
	}{
		{release.SourceBluRay, "BluRay"},
		{release.SourceWEBDL, "WEB-DL"},
		{release.SourceWEBRip, "WEBRip"},
		{release.SourceHDTV, "HDTV"},
		{release.SourceUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := sourceDisplayName(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHdrDisplayName(t *testing.T) {
	tests := []struct {
		hdr  release.HDRFormat
		want string
	}{
		{release.DolbyVision, "Dolby Vision"},
		{release.HDR10Plus, "HDR10+"},
		{release.HDR10, "HDR10"},
		{release.HDRGeneric, "HDR"},
		{release.HLG, "HLG"},
		{release.HDRNone, ""},
	}

	for _, tt := range tests {
		name := tt.hdr.String()
		if name == "" {
			name = "none"
		}
		t.Run(name, func(t *testing.T) {
			got := hdrDisplayName(tt.hdr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValueOrEmpty(t *testing.T) {
	assert.Equal(t, "(none)", valueOrEmpty(""))
	assert.Equal(t, "test", valueOrEmpty("test"))
}

func TestBoolToYesNo(t *testing.T) {
	assert.Equal(t, "yes", boolToYesNo(true))
	assert.Equal(t, "no", boolToYesNo(false))
}

func TestGetProfileNames(t *testing.T) {
	cfg := &config.Config{
		Quality: config.QualityConfig{
			Profiles: map[string]config.QualityProfile{
				"hd":  {},
				"uhd": {},
				"any": {},
			},
		},
	}

	names := getProfileNames(cfg)

	assert.Len(t, names, 3)

	// Check all expected names are present (order may vary due to map iteration)
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, expected := range []string{"hd", "uhd", "any"} {
		assert.True(t, nameSet[expected], "missing profile name %q", expected)
	}
}
