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

// unknownStr is the string representation for unknown values.
const unknownStr = "unknown"

func (r Resolution) String() string {
	switch r {
	case Resolution720p:
		return "720p"
	case Resolution1080p:
		return "1080p"
	case Resolution2160p:
		return "2160p"
	default:
		return unknownStr
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
	SourceCAM
	SourceTelesync
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
	case SourceCAM:
		return "cam"
	case SourceTelesync:
		return "telesync"
	default:
		return unknownStr
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
		return unknownStr
	}
}

// HDRFormat represents HDR/Dolby Vision formats.
type HDRFormat int

const (
	HDRNone    HDRFormat = iota
	HDRGeneric           // "HDR" without specific version
	HDR10
	HDR10Plus
	DolbyVision
	HLG
)

func (h HDRFormat) String() string {
	switch h {
	case HDRGeneric:
		return "HDR"
	case HDR10:
		return "HDR10"
	case HDR10Plus:
		return "HDR10+"
	case DolbyVision:
		return "DV"
	case HLG:
		return "HLG"
	default:
		return ""
	}
}

// AudioCodec represents the audio format of a release.
type AudioCodec int

const (
	AudioUnknown AudioCodec = iota
	AudioAAC
	AudioAC3  // Dolby Digital
	AudioEAC3 // DD+, DDP
	AudioDTS
	AudioDTSHD // DTS-HD MA
	AudioTrueHD
	AudioAtmos // TrueHD Atmos or DD+ Atmos
	AudioFLAC
	AudioOpus
)

func (a AudioCodec) String() string {
	switch a {
	case AudioAAC:
		return "AAC"
	case AudioAC3:
		return "DD"
	case AudioEAC3:
		return "DD+"
	case AudioDTS:
		return "DTS"
	case AudioDTSHD:
		return "DTS-HD MA"
	case AudioTrueHD:
		return "TrueHD"
	case AudioAtmos:
		return "Atmos"
	case AudioFLAC:
		return "FLAC"
	case AudioOpus:
		return "Opus"
	default:
		return ""
	}
}

// Info contains parsed release information.
type Info struct {
	Title      string
	Year       int
	Season     int
	Episode    int    // Primary episode (first in range), kept for backward compatibility
	Episodes   []int  // All episodes in release (e.g., [5,6,7] for S01E05-E07)
	DailyDate  string // Daily show date in YYYY-MM-DD format (e.g., "2026-01-16")
	Resolution Resolution
	Source     Source
	Codec      Codec
	Group      string
	Proper     bool
	Repack     bool

	// Extended metadata
	HDR     HDRFormat
	Audio   AudioCodec
	IsRemux bool
	Edition string // "Directors Cut", "Extended", "IMAX", etc.
	Service string // Streaming service: NF, AMZN, DSNP, etc.

	// Season pack detection
	IsCompleteSeason bool // Complete season release (e.g., "Season 01", "S01")
	IsSplitSeason    bool // Split/partial season (e.g., "Season 1 Part 2")
	SplitPart        int  // Part number for split seasons

	// Normalized title for matching
	CleanTitle string
}
