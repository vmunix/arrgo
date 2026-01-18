// Package release provides types for parsing and representing media release information.
package release

// Resolution represents the video resolution of a release.
type Resolution int

const (
	ResolutionUnknown Resolution = iota
	Resolution720p
	Resolution1080p
	Resolution2160p
)

func (r Resolution) String() string {
	switch r {
	case Resolution720p:
		return "720p"
	case Resolution1080p:
		return "1080p"
	case Resolution2160p:
		return "2160p"
	default:
		return "unknown"
	}
}

// Source represents the media source type of a release.
type Source int

const (
	SourceUnknown Source = iota
	SourceBluRay
	SourceWEBDL
	SourceWEBRip
	SourceHDTV
)

func (s Source) String() string {
	switch s {
	case SourceBluRay:
		return "bluray"
	case SourceWEBDL:
		return "webdl"
	case SourceWEBRip:
		return "webrip"
	case SourceHDTV:
		return "hdtv"
	default:
		return "unknown"
	}
}

// Codec represents the video codec used in a release.
type Codec int

const (
	CodecUnknown Codec = iota
	CodecX264
	CodecX265
)

func (c Codec) String() string {
	switch c {
	case CodecX264:
		return "x264"
	case CodecX265:
		return "x265"
	default:
		return "unknown"
	}
}

// Info contains parsed release information.
type Info struct {
	Title      string
	Year       int
	Season     int
	Episode    int
	Resolution Resolution
	Source     Source
	Codec      Codec
	Group      string
	Proper     bool
	Repack     bool
}
