package main

import (
	"testing"

	"github.com/arrgo/arrgo/pkg/release"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"small bytes", 500, "500 B"},
		{"exactly 1KB", 1024, "1.0 KB"},
		{"1.5KB", 1536, "1.5 KB"},
		{"exactly 1MB", 1024 * 1024, "1.0 MB"},
		{"1GB", 1073741824, "1.0 GB"},
		{"11.2GB", 12000000000, "11.2 GB"},
		{"exactly 1TB", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"1.5TB", 1024 * 1024 * 1024 * 1024 * 3 / 2, "1.5 TB"},
		{"exactly 1PB", 1024 * 1024 * 1024 * 1024 * 1024, "1.0 PB"},
		{"2.5PB", 1024 * 1024 * 1024 * 1024 * 1024 * 5 / 2, "2.5 PB"},
		{"large EB range", 1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1.0 EB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSize(tt.bytes); got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatSizeNegative(t *testing.T) {
	// Negative numbers are technically invalid but the function handles them
	// by returning them in bytes format since they're less than 1024
	got := formatSize(-100)
	want := "-100 B"
	if got != want {
		t.Errorf("formatSize(-100) = %q, want %q", got, want)
	}
}

func TestBuildBadges(t *testing.T) {
	tests := []struct {
		name string
		info *release.Info
		want string
	}{
		{
			name: "empty info returns empty string",
			info: &release.Info{},
			want: "",
		},
		{
			name: "resolution only",
			info: &release.Info{
				Resolution: release.Resolution1080p,
			},
			want: "[1080p]",
		},
		{
			name: "720p resolution",
			info: &release.Info{
				Resolution: release.Resolution720p,
			},
			want: "[720p]",
		},
		{
			name: "2160p resolution",
			info: &release.Info{
				Resolution: release.Resolution2160p,
			},
			want: "[2160p]",
		},
		{
			name: "source BluRay formats nicely",
			info: &release.Info{
				Source: release.SourceBluRay,
			},
			want: "[BluRay]",
		},
		{
			name: "source WEB-DL formats nicely",
			info: &release.Info{
				Source: release.SourceWEBDL,
			},
			want: "[WEB-DL]",
		},
		{
			name: "source WEBRip formats nicely",
			info: &release.Info{
				Source: release.SourceWEBRip,
			},
			want: "[WEBRip]",
		},
		{
			name: "source HDTV formats nicely",
			info: &release.Info{
				Source: release.SourceHDTV,
			},
			want: "[HDTV]",
		},
		{
			name: "codec x264",
			info: &release.Info{
				Codec: release.CodecX264,
			},
			want: "[x264]",
		},
		{
			name: "codec x265",
			info: &release.Info{
				Codec: release.CodecX265,
			},
			want: "[x265]",
		},
		{
			name: "HDR DolbyVision shows as DV",
			info: &release.Info{
				HDR: release.DolbyVision,
			},
			want: "[DV]",
		},
		{
			name: "HDR10",
			info: &release.Info{
				HDR: release.HDR10,
			},
			want: "[HDR10]",
		},
		{
			name: "HDR10+",
			info: &release.Info{
				HDR: release.HDR10Plus,
			},
			want: "[HDR10+]",
		},
		{
			name: "audio Atmos",
			info: &release.Info{
				Audio: release.AudioAtmos,
			},
			want: "[Atmos]",
		},
		{
			name: "audio TrueHD",
			info: &release.Info{
				Audio: release.AudioTrueHD,
			},
			want: "[TrueHD]",
		},
		{
			name: "audio DTS-HD MA",
			info: &release.Info{
				Audio: release.AudioDTSHD,
			},
			want: "[DTS-HD MA]",
		},
		{
			name: "remux only",
			info: &release.Info{
				IsRemux: true,
			},
			want: "[Remux]",
		},
		{
			name: "edition only",
			info: &release.Info{
				Edition: "Directors Cut",
			},
			want: "[Directors Cut]",
		},
		{
			name: "resolution and source",
			info: &release.Info{
				Resolution: release.Resolution1080p,
				Source:     release.SourceBluRay,
			},
			want: "[1080p] [BluRay]",
		},
		{
			name: "full quality release",
			info: &release.Info{
				Resolution: release.Resolution2160p,
				Source:     release.SourceBluRay,
				Codec:      release.CodecX265,
				HDR:        release.DolbyVision,
				Audio:      release.AudioAtmos,
				IsRemux:    true,
			},
			want: "[2160p] [BluRay] [x265] [DV] [Atmos] [Remux]",
		},
		{
			name: "full release with edition",
			info: &release.Info{
				Resolution: release.Resolution1080p,
				Source:     release.SourceBluRay,
				Codec:      release.CodecX264,
				Edition:    "Extended",
			},
			want: "[1080p] [BluRay] [x264] [Extended]",
		},
		{
			name: "WEB-DL with HDR10 and DD+",
			info: &release.Info{
				Resolution: release.Resolution2160p,
				Source:     release.SourceWEBDL,
				Codec:      release.CodecX265,
				HDR:        release.HDR10,
				Audio:      release.AudioEAC3,
			},
			want: "[2160p] [WEB-DL] [x265] [HDR10] [DD+]",
		},
		{
			name: "HDTV with basic audio",
			info: &release.Info{
				Resolution: release.Resolution720p,
				Source:     release.SourceHDTV,
				Audio:      release.AudioAC3,
			},
			want: "[720p] [HDTV] [DD]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildBadges(tt.info); got != tt.want {
				t.Errorf("buildBadges() = %q, want %q", got, tt.want)
			}
		})
	}
}
