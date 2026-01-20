package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolution_String(t *testing.T) {
	tests := []struct {
		name string
		r    Resolution
		want string
	}{
		{"unknown", ResolutionUnknown, "unknown"},
		{"720p", Resolution720p, "720p"},
		{"1080p", Resolution1080p, "1080p"},
		{"2160p", Resolution2160p, "2160p"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.String())
		})
	}
}

func TestSource_String(t *testing.T) {
	tests := []struct {
		name string
		s    Source
		want string
	}{
		{"unknown", SourceUnknown, "unknown"},
		{"bluray", SourceBluRay, "bluray"},
		{"webdl", SourceWEBDL, "webdl"},
		{"webrip", SourceWEBRip, "webrip"},
		{"hdtv", SourceHDTV, "hdtv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.String())
		})
	}
}

func TestCodec_String(t *testing.T) {
	tests := []struct {
		name string
		c    Codec
		want string
	}{
		{"unknown", CodecUnknown, "unknown"},
		{"x264", CodecX264, "x264"},
		{"x265", CodecX265, "x265"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.c.String())
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRes    Resolution
		wantSource Source
		wantCodec  Codec
		wantYear   int
		wantGroup  string
		wantProper bool
		wantRepack bool
	}{
		{
			name:       "2160p UHD BluRay x265",
			input:      "Movie.2024.2160p.UHD.BluRay.x265-GROUP",
			wantRes:    Resolution2160p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX265,
			wantYear:   2024,
			wantGroup:  "GROUP",
		},
		{
			name:       "1080p BluRay x264",
			input:      "Movie.2024.1080p.BluRay.x264-GROUP",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GROUP",
		},
		{
			name:       "720p HDTV",
			input:      "Show.S01E05.720p.HDTV.x264-TEAM",
			wantRes:    Resolution720p,
			wantSource: SourceHDTV,
			wantCodec:  CodecX264,
			wantGroup:  "TEAM",
		},
		{
			name:       "4K marker",
			input:      "Movie.2023.4K.WEB-DL.x265-RLS",
			wantRes:    Resolution2160p,
			wantSource: SourceWEBDL,
			wantCodec:  CodecX265,
			wantYear:   2023,
			wantGroup:  "RLS",
		},
		{
			name:       "WEBRip source",
			input:      "Show.2024.S02E10.1080p.WEBRip.x264-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceWEBRip,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
		},
		{
			name:       "HEVC codec",
			input:      "Movie.2022.1080p.BluRay.HEVC-TEAM",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX265,
			wantYear:   2022,
			wantGroup:  "TEAM",
		},
		{
			name:       "H264 codec variant",
			input:      "Movie.2021.720p.WEB-DL.H264-GRP",
			wantRes:    Resolution720p,
			wantSource: SourceWEBDL,
			wantCodec:  CodecX264,
			wantYear:   2021,
			wantGroup:  "GRP",
		},
		{
			name:       "H265 codec variant",
			input:      "Movie.2020.2160p.BluRay.H265-RLS",
			wantRes:    Resolution2160p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX265,
			wantYear:   2020,
			wantGroup:  "RLS",
		},
		{
			name:       "PROPER release",
			input:      "Movie.2024.1080p.BluRay.x264.PROPER-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
			wantProper: true,
		},
		{
			name:       "REPACK release",
			input:      "Movie.2024.1080p.BluRay.x264.REPACK-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
			wantRepack: true,
		},
		{
			name:       "Unknown resolution",
			input:      "Movie.2024.BluRay.x264-GRP",
			wantRes:    ResolutionUnknown,
			wantSource: SourceBluRay,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
		},
		{
			name:       "Unknown source",
			input:      "Movie.2024.1080p.x264-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceUnknown,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
		},
		{
			name:       "Unknown codec",
			input:      "Movie.2024.1080p.BluRay-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecUnknown,
			wantYear:   2024,
			wantGroup:  "GRP",
		},
		{
			name:       "BDRip source variant",
			input:      "Movie.2024.1080p.BDRip.x264-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
		},
		{
			name:       "Blu-ray hyphenated",
			input:      "Movie.2024.1080p.Blu-ray.x264-GRP",
			wantRes:    Resolution1080p,
			wantSource: SourceBluRay,
			wantCodec:  CodecX264,
			wantYear:   2024,
			wantGroup:  "GRP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			assert.Equal(t, tt.wantRes, got.Resolution, "Resolution")
			assert.Equal(t, tt.wantSource, got.Source, "Source")
			assert.Equal(t, tt.wantCodec, got.Codec, "Codec")
			assert.Equal(t, tt.wantYear, got.Year, "Year")
			assert.Equal(t, tt.wantGroup, got.Group, "Group")
			assert.Equal(t, tt.wantProper, got.Proper, "Proper")
			assert.Equal(t, tt.wantRepack, got.Repack, "Repack")
		})
	}
}

func TestParse_SeasonEpisode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSeason  int
		wantEpisode int
	}{
		{
			name:        "Standard S01E01",
			input:       "Show.S01E01.1080p.WEB-DL.x264-GRP",
			wantSeason:  1,
			wantEpisode: 1,
		},
		{
			name:        "Double digit season and episode",
			input:       "Show.S12E24.720p.HDTV.x264-GRP",
			wantSeason:  12,
			wantEpisode: 24,
		},
		{
			name:        "No season/episode (movie)",
			input:       "Movie.2024.1080p.BluRay.x264-GRP",
			wantSeason:  0,
			wantEpisode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantEpisode, got.Episode, "Episode")
		})
	}
}

func TestParse_Title(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
	}{
		{
			name:      "Movie with year",
			input:     "The.Movie.Title.2024.1080p.BluRay.x264-GRP",
			wantTitle: "The Movie Title",
		},
		{
			name:      "TV show with season",
			input:     "Some.Show.S01E05.720p.HDTV.x264-GRP",
			wantTitle: "Some Show",
		},
		{
			name:      "Movie with 4K marker",
			input:     "Cool.Film.4K.WEB-DL.x265-GRP",
			wantTitle: "Cool Film",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			assert.Equal(t, tt.wantTitle, got.Title, "Title")
		})
	}
}

func TestParse_HDR(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantHDR HDRFormat
	}{
		{"DV only", "Movie.2024.2160p.WEB-DL.DV.H265-GRP", DolbyVision},
		{"HDR10", "Movie.2024.2160p.BluRay.HDR10.x265-GRP", HDR10},
		{"HDR10+", "Movie.2024.2160p.UHD.BluRay.HDR10+.x265-GRP", HDR10Plus},
		{"DV HDR combo", "Movie.2024.2160p.WEB-DL.DV.HDR.H265-GRP", DolbyVision},
		{"Generic HDR", "Movie.2024.2160p.BluRay.HDR.x265-GRP", HDRGeneric},
		{"HLG", "Movie.2024.2160p.BluRay.HLG.x265-GRP", HLG},
		{"No HDR", "Movie.2024.1080p.BluRay.x264-GRP", HDRNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantHDR, got.HDR, "HDR")
		})
	}
}

func TestParse_Audio(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantAudio AudioCodec
	}{
		{"DTS-HD MA", "Movie.2024.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP", AudioDTSHD},
		{"TrueHD Atmos", "Movie.2024.2160p.BluRay.TrueHD.Atmos.7.1.x265-GRP", AudioAtmos},
		{"DD+ Atmos", "Movie.2024.2160p.WEB-DL.DDP5.1.Atmos.H265-GRP", AudioAtmos},
		{"DDP", "Movie.2024.1080p.WEB-DL.DDP5.1.x264-GRP", AudioEAC3},
		{"DD5.1", "Movie.2024.1080p.WEB-DL.DD5.1.x264-GRP", AudioAC3},
		{"FLAC", "Movie.2024.1080p.BluRay.FLAC.2.0.x264-GRP", AudioFLAC},
		{"AAC", "Movie.2024.1080p.WEB-DL.AAC2.0.x264-GRP", AudioAAC},
		{"TrueHD no Atmos", "Movie.2024.1080p.BluRay.TrueHD.5.1.x264-GRP", AudioTrueHD},
		{"Plain DTS", "Movie.2024.1080p.BluRay.DTS.5.1.x264-GRP", AudioDTS},
		{"Opus", "Movie.2024.1080p.WEB.Opus.2.0.x264-GRP", AudioOpus},
		{"No audio info", "Movie.2024.1080p.BluRay.x264-GRP", AudioUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantAudio, got.Audio, "Audio")
		})
	}
}

func TestParse_Remux(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRemux  bool
		wantSource Source
	}{
		{"AVC REMUX", "Movie.2024.1080p.BluRay.AVC.REMUX-GRP", true, SourceBluRay},
		{"REMUX standalone", "Movie.2024.2160p.UHD.BluRay.REMUX.HEVC-GRP", true, SourceBluRay},
		{"Not remux", "Movie.2024.1080p.BluRay.x264-GRP", false, SourceBluRay},
		{"BDRemux variant", "Movie.2024.1080p.BDRemux.x264-GRP", true, SourceBluRay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantRemux, got.IsRemux, "IsRemux")
			assert.Equal(t, tt.wantSource, got.Source, "Source")
		})
	}
}

func TestParse_Edition(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantEdition string
	}{
		{"Directors Cut", "Movie.2024.Directors.Cut.1080p.BluRay.x264-GRP", "Directors Cut"},
		{"Extended", "Movie.2024.EXTENDED.1080p.BluRay.x264-GRP", "Extended"},
		{"IMAX", "Movie.2024.IMAX.2160p.WEB-DL.x265-GRP", "IMAX"},
		{"Theatrical", "Movie.2024.Theatrical.Cut.1080p.BluRay.x264-GRP", "Theatrical"},
		{"Unrated", "Movie.2024.UNRATED.1080p.BluRay.x264-GRP", "Unrated"},
		{"No edition", "Movie.2024.1080p.BluRay.x264-GRP", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantEdition, got.Edition, "Edition")
		})
	}
}

func TestParse_Service(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantService string
	}{
		{"Netflix", "Movie.2024.1080p.NF.WEB-DL.x264-GRP", "Netflix"},
		{"Amazon", "Movie.2024.1080p.AMZN.WEB-DL.x264-GRP", "Amazon"},
		{"Disney+", "Movie.2024.2160p.DSNP.WEB-DL.x265-GRP", "Disney+"},
		{"AppleTV+", "Movie.2024.2160p.ATVP.WEB-DL.x265-GRP", "Apple TV+"},
		{"HBO Max", "Movie.2024.1080p.HMAX.WEB-DL.x264-GRP", "HBO Max"},
		{"Peacock", "Movie.2024.1080p.PCOK.WEB-DL.x264-GRP", "Peacock"},
		{"Hulu", "Movie.2024.1080p.HULU.WEB-DL.x264-GRP", "Hulu"},
		{"No service", "Movie.2024.1080p.WEB-DL.x264-GRP", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantService, got.Service, "Service")
		})
	}
}

func TestParse_ImprovedCodec(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCodec Codec
	}{
		{"H.264 with dot", "Movie.2024.1080p.WEB-DL.H.264-GRP", CodecX264},
		{"H.265 with dot", "Movie.2024.2160p.WEB-DL.H.265-GRP", CodecX265},
		{"AVC", "Movie.2024.1080p.BluRay.AVC-GRP", CodecX264},
		{"HEVC uppercase", "Movie.2024.2160p.BluRay.HEVC-GRP", CodecX265},
		{"XviD", "Movie.2024.DVDRip.XviD-GRP", CodecUnknown}, // We don't track XviD
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantCodec, got.Codec, "Codec")
		})
	}
}

func TestParse_DailyShow(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantDailyDate string
		wantYear      int
	}{
		{"Daily show", "Show.2026.01.16.Episode.Title.720p.HDTV.x264-GRP", "2026-01-16", 0},
		{"Not daily", "Show.S01E05.720p.HDTV.x264-GRP", "", 0},
		{"Movie with year", "Movie.2024.1080p.BluRay.x264-GRP", "", 2024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantDailyDate, got.DailyDate, "DailyDate")
			assert.Equal(t, tt.wantYear, got.Year, "Year")
		})
	}
}

func TestInfo_Episodes_Slice(t *testing.T) {
	info := &Info{
		Episodes: []int{5, 6, 7},
	}
	assert.Len(t, info.Episodes, 3)
	assert.Equal(t, 5, info.Episodes[0])
}

func TestParse_AlternateEpisodeFormats(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSeason  int
		wantEpisode int
	}{
		{
			name:        "1x05 format",
			input:       "Show.1x05.720p.HDTV.x264-GRP",
			wantSeason:  1,
			wantEpisode: 5,
		},
		{
			name:        "12x24 format double digit",
			input:       "Show.12x24.Episode.Title.1080p.WEB-DL.x264-GRP",
			wantSeason:  12,
			wantEpisode: 24,
		},
		{
			name:        "s01.05 format with dot",
			input:       "Show.s01.05.720p.HDTV.x264-GRP",
			wantSeason:  1,
			wantEpisode: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantEpisode, got.Episode, "Episode")
		})
	}
}

func TestParse_MultiEpisode(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantSeason   int
		wantEpisode  int // First episode
		wantEpisodes []int
	}{
		{
			name:         "S01E05-06 range with hyphen",
			input:        "Show.S01E05-06.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6},
		},
		{
			name:         "S01E05-E06 range with E prefix",
			input:        "Show.S01E05-E06.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6},
		},
		{
			name:         "S01E05E06 sequential",
			input:        "Show.S01E05E06.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6},
		},
		{
			name:         "S01E05E06E07 triple episode",
			input:        "Show.S01E05E06E07.1080p.WEB-DL.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5, 6, 7},
		},
		{
			name:         "S01E01-03 range spanning 3",
			input:        "Show.S01E01-03.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  1,
			wantEpisodes: []int{1, 2, 3},
		},
		{
			name:         "Single episode still works",
			input:        "Show.S01E05.720p.HDTV.x264-GRP",
			wantSeason:   1,
			wantEpisode:  5,
			wantEpisodes: []int{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantEpisode, got.Episode, "Episode")
			assert.Equal(t, tt.wantEpisodes, got.Episodes, "Episodes")
		})
	}
}

func TestParse_AudioGaps(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantAudio AudioCodec
	}{
		{
			name:      "DD.5.1 with dots",
			input:     "Movie.2024.1080p.BluRay.DD.5.1.x264-GRP",
			wantAudio: AudioAC3,
		},
		{
			name:      "DD.2.0 stereo",
			input:     "Movie.2024.1080p.BluRay.DD.2.0.x264-GRP",
			wantAudio: AudioAC3,
		},
		{
			name:      "DD 5.1 with space",
			input:     "Movie.2024.1080p.BluRay.DD 5.1.x264-GRP",
			wantAudio: AudioAC3,
		},
		{
			name:      "DD+ 5.1 (should be EAC3)",
			input:     "Movie.2024.1080p.WEB-DL.DD+.5.1.x264-GRP",
			wantAudio: AudioEAC3,
		},
		{
			name:      "Dolby Digital explicit",
			input:     "Movie.2024.1080p.BluRay.Dolby.Digital.5.1.x264-GRP",
			wantAudio: AudioAC3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantAudio, got.Audio, "Audio")
		})
	}
}

func TestParse_DailyShowFormats(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantDailyDate string
		wantYear      int
	}{
		{
			name:          "YYYY.MM.DD standard",
			input:         "Show.2026.01.16.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "YYYY-MM-DD with hyphens",
			input:         "Show.2026-01-16.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "YYYYMMDD compact",
			input:         "Show.20260116.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "DD.MM.YYYY European",
			input:         "Show.16.01.2026.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "16 Jan 2026 word month",
			input:         "Show.16.Jan.2026.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "Jan 16 2026 US word format",
			input:         "Show.Jan.16.2026.Episode.720p.HDTV.x264-GRP",
			wantDailyDate: "2026-01-16",
			wantYear:      0,
		},
		{
			name:          "Movie with year (not daily)",
			input:         "Movie.2024.1080p.BluRay.x264-GRP",
			wantDailyDate: "",
			wantYear:      2024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantDailyDate, got.DailyDate, "DailyDate")
			assert.Equal(t, tt.wantYear, got.Year, "Year")
		})
	}
}

func TestParse_SeasonPack(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		wantSeason         int
		wantCompleteSeason bool
		wantSplitSeason    bool
		wantSplitPart      int
	}{
		{
			name:               "Season 01 pack",
			input:              "Show.Season.01.1080p.BluRay.x264-GRP",
			wantSeason:         1,
			wantCompleteSeason: true,
		},
		{
			name:               "S01 pack no episodes",
			input:              "Show.S01.1080p.BluRay.x264-GRP",
			wantSeason:         1,
			wantCompleteSeason: true,
		},
		{
			name:               "Complete Season",
			input:              "Show.Complete.Season.2.720p.WEB-DL.x264-GRP",
			wantSeason:         2,
			wantCompleteSeason: true,
		},
		{
			name:            "Season 1 Part 2",
			input:           "Show.Season.1.Part.2.1080p.WEB-DL.x264-GRP",
			wantSeason:      1,
			wantSplitSeason: true,
			wantSplitPart:   2,
		},
		{
			name:            "S01 Vol 1",
			input:           "Show.S01.Vol.1.1080p.WEB-DL.x264-GRP",
			wantSeason:      1,
			wantSplitSeason: true,
			wantSplitPart:   1,
		},
		{
			name:               "Regular episode not a pack",
			input:              "Show.S01E05.720p.HDTV.x264-GRP",
			wantSeason:         1,
			wantCompleteSeason: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			assert.Equal(t, tt.wantSeason, got.Season, "Season")
			assert.Equal(t, tt.wantCompleteSeason, got.IsCompleteSeason, "IsCompleteSeason")
			assert.Equal(t, tt.wantSplitSeason, got.IsSplitSeason, "IsSplitSeason")
			assert.Equal(t, tt.wantSplitPart, got.SplitPart, "SplitPart")
		})
	}
}
