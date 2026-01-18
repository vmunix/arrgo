package release

import "testing"

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
