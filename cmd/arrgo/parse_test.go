package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	names, err := readReleaseFile(testFile)
	if err != nil {
		t.Fatalf("readReleaseFile() error = %v", err)
	}

	want := []string{
		"Movie.2024.1080p.BluRay.x264-GROUP",
		"Another.Movie.2023.720p.WEB-DL.x265-TEAM",
		"Spaced.Movie.2022.2160p.UHD.BluRay.x265-RELEASE",
	}

	if len(names) != len(want) {
		t.Fatalf("got %d names, want %d", len(names), len(want))
	}

	for i, got := range names {
		if got != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestReadReleaseFile_NotFound(t *testing.T) {
	_, err := readReleaseFile("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestReadReleaseFile_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")

	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	names, err := readReleaseFile(testFile)
	if err != nil {
		t.Fatalf("readReleaseFile() error = %v", err)
	}

	if len(names) != 0 {
		t.Errorf("got %d names, want 0", len(names))
	}
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

	if jsonResult.Title != "The Matrix" {
		t.Errorf("Title = %q, want %q", jsonResult.Title, "The Matrix")
	}
	if jsonResult.Year != 1999 {
		t.Errorf("Year = %d, want %d", jsonResult.Year, 1999)
	}
	if jsonResult.Resolution != "1080p" {
		t.Errorf("Resolution = %q, want %q", jsonResult.Resolution, "1080p")
	}
	if jsonResult.Source != "bluray" {
		t.Errorf("Source = %q, want %q", jsonResult.Source, "bluray")
	}
	if jsonResult.IsRemux != true {
		t.Errorf("IsRemux = %v, want true", jsonResult.IsRemux)
	}
	if jsonResult.HDR != "" {
		t.Errorf("HDR = %q, want empty (HDRNone)", jsonResult.HDR)
	}
	if jsonResult.Audio != "DTS-HD MA" {
		t.Errorf("Audio = %q, want %q", jsonResult.Audio, "DTS-HD MA")
	}
	if jsonResult.Score != 100 {
		t.Errorf("Score = %d, want %d", jsonResult.Score, 100)
	}
	if jsonResult.Profile != "hd" {
		t.Errorf("Profile = %q, want %q", jsonResult.Profile, "hd")
	}
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

	if jsonResult.HDR != "DV" {
		t.Errorf("HDR = %q, want %q", jsonResult.HDR, "DV")
	}
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

	if jsonResult.Season != 1 {
		t.Errorf("Season = %d, want %d", jsonResult.Season, 1)
	}
	if jsonResult.Episode != 5 {
		t.Errorf("Episode = %d, want %d", jsonResult.Episode, 5)
	}
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
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Verify key fields are in the JSON
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"title":"Test Movie"`) {
		t.Errorf("JSON missing title field")
	}
	if !strings.Contains(jsonStr, `"year":2024`) {
		t.Errorf("JSON missing year field")
	}
	if !strings.Contains(jsonStr, `"resolution":"1080p"`) {
		t.Errorf("JSON missing resolution field")
	}
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
	if score != 100 {
		t.Errorf("score = %d, want 100", score)
	}

	if len(breakdown) < 3 {
		t.Fatalf("breakdown has %d entries, want at least 3", len(breakdown))
	}
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

	if score != 0 {
		t.Errorf("score = %d, want 0 (rejected)", score)
	}

	if len(breakdown) != 1 {
		t.Fatalf("breakdown has %d entries, want 1", len(breakdown))
	}

	if breakdown[0].Attribute != "Reject" {
		t.Errorf("breakdown[0].Attribute = %q, want %q", breakdown[0].Attribute, "Reject")
	}
}

func TestScoreWithBreakdown_ResolutionNotAllowed(t *testing.T) {
	info := release.Info{
		Resolution: release.Resolution720p,
	}

	profile := config.QualityProfile{
		Resolution: []string{"1080p", "2160p"}, // 720p not allowed
	}

	score, breakdown := scoreWithBreakdown(info, profile)

	if score != 0 {
		t.Errorf("score = %d, want 0 (resolution not allowed)", score)
	}

	if len(breakdown) != 1 {
		t.Fatalf("breakdown has %d entries, want 1", len(breakdown))
	}

	if breakdown[0].Note != "not in allowed list" {
		t.Errorf("breakdown[0].Note = %q, want %q", breakdown[0].Note, "not in allowed list")
	}
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
	if score != 115 {
		t.Errorf("score = %d, want 115", score)
	}

	// Find HDR in breakdown
	var hdrBonus *ScoreBonus
	for i := range breakdown {
		if breakdown[i].Attribute == "HDR" {
			hdrBonus = &breakdown[i]
			break
		}
	}

	if hdrBonus == nil {
		t.Fatal("HDR not found in breakdown")
	}

	if hdrBonus.Bonus != 15 {
		t.Errorf("HDR bonus = %d, want 15", hdrBonus.Bonus)
	}
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

	// Expected: 80 (resolution) + 20 (remux) = 100
	if score != 100 {
		t.Errorf("score = %d, want 100", score)
	}

	// Find Remux in breakdown
	var remuxBonus *ScoreBonus
	for i := range breakdown {
		if breakdown[i].Attribute == "Remux" {
			remuxBonus = &breakdown[i]
			break
		}
	}

	if remuxBonus == nil {
		t.Fatal("Remux not found in breakdown")
	}

	if remuxBonus.Bonus != 20 {
		t.Errorf("Remux bonus = %d, want 20", remuxBonus.Bonus)
	}
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
			if score != tt.wantScore {
				t.Errorf("score = %d, want %d", score, tt.wantScore)
			}
			if bonus.Note != tt.wantNote {
				t.Errorf("note = %q, want %q", bonus.Note, tt.wantNote)
			}
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
			if bonus.Bonus != tt.wantBonus {
				t.Errorf("bonus = %d, want %d", bonus.Bonus, tt.wantBonus)
			}
			if bonus.Note != tt.wantNote {
				t.Errorf("note = %q, want %q", bonus.Note, tt.wantNote)
			}
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
			if got := hdrMatches(tt.hdr, tt.pref); got != tt.match {
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
			if got := audioMatches(tt.audio, tt.pref); got != tt.match {
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
			if got := matchesRejectList(tt.info, tt.rejectList); got != tt.want {
				t.Errorf("matchesRejectList() = %v, want %v", got, tt.want)
			}
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
			if got := sourceDisplayName(tt.source); got != tt.want {
				t.Errorf("sourceDisplayName(%v) = %q, want %q", tt.source, got, tt.want)
			}
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
			if got := hdrDisplayName(tt.hdr); got != tt.want {
				t.Errorf("hdrDisplayName(%v) = %q, want %q", tt.hdr, got, tt.want)
			}
		})
	}
}

func TestValueOrEmpty(t *testing.T) {
	if got := valueOrEmpty(""); got != "(none)" {
		t.Errorf("valueOrEmpty(\"\") = %q, want %q", got, "(none)")
	}
	if got := valueOrEmpty("test"); got != "test" {
		t.Errorf("valueOrEmpty(\"test\") = %q, want %q", got, "test")
	}
}

func TestBoolToYesNo(t *testing.T) {
	if got := boolToYesNo(true); got != "yes" {
		t.Errorf("boolToYesNo(true) = %q, want %q", got, "yes")
	}
	if got := boolToYesNo(false); got != "no" {
		t.Errorf("boolToYesNo(false) = %q, want %q", got, "no")
	}
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

	if len(names) != 3 {
		t.Errorf("got %d names, want 3", len(names))
	}

	// Check all expected names are present (order may vary due to map iteration)
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, expected := range []string{"hd", "uhd", "any"} {
		if !nameSet[expected] {
			t.Errorf("missing profile name %q", expected)
		}
	}
}
