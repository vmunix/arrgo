package release

import (
	"testing"
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
			if got := tt.r.String(); got != tt.want {
				t.Errorf("Resolution.String() = %v, want %v", got, tt.want)
			}
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
			if got := tt.s.String(); got != tt.want {
				t.Errorf("Source.String() = %v, want %v", got, tt.want)
			}
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
			if got := tt.c.String(); got != tt.want {
				t.Errorf("Codec.String() = %v, want %v", got, tt.want)
			}
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

			if got.Resolution != tt.wantRes {
				t.Errorf("Resolution = %v, want %v", got.Resolution, tt.wantRes)
			}
			if got.Source != tt.wantSource {
				t.Errorf("Source = %v, want %v", got.Source, tt.wantSource)
			}
			if got.Codec != tt.wantCodec {
				t.Errorf("Codec = %v, want %v", got.Codec, tt.wantCodec)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %v, want %v", got.Year, tt.wantYear)
			}
			if got.Group != tt.wantGroup {
				t.Errorf("Group = %v, want %v", got.Group, tt.wantGroup)
			}
			if got.Proper != tt.wantProper {
				t.Errorf("Proper = %v, want %v", got.Proper, tt.wantProper)
			}
			if got.Repack != tt.wantRepack {
				t.Errorf("Repack = %v, want %v", got.Repack, tt.wantRepack)
			}
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

			if got.Season != tt.wantSeason {
				t.Errorf("Season = %v, want %v", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEpisode {
				t.Errorf("Episode = %v, want %v", got.Episode, tt.wantEpisode)
			}
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

			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
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
			if got.HDR != tt.wantHDR {
				t.Errorf("HDR = %v, want %v", got.HDR, tt.wantHDR)
			}
		})
	}
}
