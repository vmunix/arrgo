// Package release parses release names to extract quality, source, codec, etc.
package release

import (
	"regexp"
	"strconv"
	"strings"
)

// Pre-compiled regex patterns (compiled once at package init)
var (
	yearRegex        = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	seasonEpRegex    = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,2})`)
	titleMarkerRegex = regexp.MustCompile(`(?i)\b(19|20)\d{2}\b|\b\d{3,4}p\b|\bS\d{1,2}E\d{1,2}\b|\b4K\b|\bUHD\b`)
	hdrRegex         = regexp.MustCompile(`(?i)\bHDR10\+|\b(HDR10Plus|HDR10|HDR|DV|Dolby\.?Vision|HLG)\b`)
	editionRegex     = regexp.MustCompile(`(?i)\b(Directors?[\s.]?Cut|Extended|IMAX|Theatrical[\s.]?Cut?|Unrated|Uncut|Remastered|Anniversary|Criterion|Special[\s.]?Edition)\b`)
)

// serviceMap maps streaming service codes to their full names.
var serviceMap = map[string]string{
	"nf":        "Netflix",
	"netflix":   "Netflix",
	"amzn":      "Amazon",
	"amazon":    "Amazon",
	"dsnp":      "Disney+",
	"disney":    "Disney+",
	"atvp":      "Apple TV+",
	"aptv":      "Apple TV+",
	"hmax":      "HBO Max",
	"hbo":       "HBO Max",
	"pcok":      "Peacock",
	"peacock":   "Peacock",
	"hulu":      "Hulu",
	"pmtp":      "Paramount+",
	"paramount": "Paramount+",
	"stan":      "Stan",
	"crav":      "Crave",
	"now":       "NOW",
	"it":        "iT",
}

// Parse extracts information from a release name.
func Parse(name string) *Info {
	info := &Info{}

	// Normalize
	normalized := strings.ReplaceAll(name, ".", " ")
	normalized = strings.ReplaceAll(normalized, "_", " ")

	// Resolution
	info.Resolution = parseResolution(normalized)

	// Source
	info.Source = parseSource(normalized)

	// Codec
	info.Codec = parseCodec(normalized)

	// HDR
	info.HDR = parseHDR(normalized)

	// Audio
	info.Audio = parseAudio(normalized)

	// Remux
	info.IsRemux = parseRemux(normalized)

	// Edition
	info.Edition = parseEdition(normalized)

	// Service - use original name (not normalized) for exact delimiter matching
	info.Service = parseService(name)

	// Flags
	info.Proper = containsAny(normalized, "proper")
	info.Repack = containsAny(normalized, "repack", "rerip")

	// Year - use pre-compiled
	if match := yearRegex.FindString(normalized); match != "" {
		if year, err := strconv.Atoi(match); err == nil {
			info.Year = year
		}
	}

	// Season/Episode - use pre-compiled
	if matches := seasonEpRegex.FindStringSubmatch(normalized); len(matches) == 3 {
		if season, err := strconv.Atoi(matches[1]); err == nil {
			info.Season = season
		}
		if episode, err := strconv.Atoi(matches[2]); err == nil {
			info.Episode = episode
		}
	}

	// Group (usually last, after hyphen)
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		group := strings.TrimSpace(name[idx+1:])
		// Remove file extension if present
		if dotIdx := strings.LastIndex(group, "."); dotIdx > 0 {
			group = group[:dotIdx]
		}
		info.Group = group
	}

	// Title - extract from start up to year or quality marker
	info.Title = parseTitle(normalized)

	return info
}

func parseResolution(name string) Resolution {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "2160p"), strings.Contains(name, "4k"), strings.Contains(name, "uhd"):
		return Resolution2160p
	case strings.Contains(name, "1080p"):
		return Resolution1080p
	case strings.Contains(name, "720p"):
		return Resolution720p
	default:
		return ResolutionUnknown
	}
}

func parseSource(name string) Source {
	name = strings.ToLower(name)
	switch {
	case containsAny(name, "bluray", "blu-ray", "bdrip", "brrip", "bdremux"):
		return SourceBluRay
	case containsAny(name, "web-dl", "webdl"):
		return SourceWEBDL
	case containsAny(name, "webrip", "web-rip"):
		return SourceWEBRip
	case containsAny(name, "hdtv"):
		return SourceHDTV
	default:
		return SourceUnknown
	}
}

func parseCodec(name string) Codec {
	lower := strings.ToLower(name)

	// Normalize H.264 -> h264, H.265 -> h265
	// Handle both dot-separated (h.264) and space-separated (h 264) forms
	lower = strings.ReplaceAll(lower, "h.264", "h264")
	lower = strings.ReplaceAll(lower, "h.265", "h265")
	lower = strings.ReplaceAll(lower, "h 264", "h264")
	lower = strings.ReplaceAll(lower, "h 265", "h265")

	switch {
	case containsAny(lower, "x265", "h265", "hevc"):
		return CodecX265
	case containsAny(lower, "x264", "h264", "avc"):
		return CodecX264
	default:
		return CodecUnknown
	}
}

func containsAny(s string, substrs ...string) bool {
	s = strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func parseTitle(name string) string {
	// Find the first marker that indicates end of title
	// Common markers: year (4 digits), resolution, S01E01, etc.
	loc := titleMarkerRegex.FindStringIndex(name)
	if loc != nil {
		title := strings.TrimSpace(name[:loc[0]])
		return title
	}
	return ""
}

func parseHDR(name string) HDRFormat {
	matches := hdrRegex.FindAllString(name, -1)
	if len(matches) == 0 {
		return HDRNone
	}

	// Check in priority order (most specific first)
	// DolbyVision takes priority as DV releases often also have HDR metadata
	for _, m := range matches {
		lower := strings.ToLower(m)
		switch {
		case lower == "dv" || strings.Contains(lower, "dolby"):
			return DolbyVision
		}
	}
	for _, m := range matches {
		lower := strings.ToLower(m)
		switch {
		case lower == "hdr10+" || lower == "hdr10plus":
			return HDR10Plus
		case lower == "hdr10":
			return HDR10
		case lower == "hlg":
			return HLG
		case lower == "hdr":
			return HDRGeneric
		}
	}
	return HDRNone
}

func parseAudio(name string) AudioCodec {
	lower := strings.ToLower(name)

	// Check Atmos first (can be combined with TrueHD or DD+)
	if strings.Contains(lower, "atmos") {
		return AudioAtmos
	}

	// DTS-HD MA before plain DTS
	if strings.Contains(lower, "dts-hd") || strings.Contains(lower, "dts hd") {
		return AudioDTSHD
	}

	// TrueHD
	if strings.Contains(lower, "truehd") {
		return AudioTrueHD
	}

	// DD+ / DDP / EAC3 before DD / AC3
	if containsAny(lower, "ddp", "dd+", "eac3", "e-ac3") {
		return AudioEAC3
	}

	// DD / AC3
	if containsAny(lower, "dd5", "dd2", "dd7", "ac3", "dolby digital") {
		return AudioAC3
	}

	// Plain DTS
	if strings.Contains(lower, "dts") {
		return AudioDTS
	}

	// Lossless
	if strings.Contains(lower, "flac") {
		return AudioFLAC
	}

	// Others
	if strings.Contains(lower, "opus") {
		return AudioOpus
	}
	if strings.Contains(lower, "aac") {
		return AudioAAC
	}

	return AudioUnknown
}

func parseRemux(name string) bool {
	return containsAny(strings.ToLower(name), "remux", "bdremux")
}

func parseEdition(name string) string {
	match := editionRegex.FindString(name)
	if match == "" {
		return ""
	}

	// Normalize
	lower := strings.ToLower(match)
	switch {
	case strings.Contains(lower, "director"):
		return "Directors Cut"
	case strings.Contains(lower, "extended"):
		return "Extended"
	case strings.Contains(lower, "imax"):
		return "IMAX"
	case strings.Contains(lower, "theatrical"):
		return "Theatrical"
	case strings.Contains(lower, "unrated"):
		return "Unrated"
	case strings.Contains(lower, "uncut"):
		return "Uncut"
	case strings.Contains(lower, "remaster"):
		return "Remastered"
	case strings.Contains(lower, "anniversary"):
		return "Anniversary"
	case strings.Contains(lower, "criterion"):
		return "Criterion"
	case strings.Contains(lower, "special"):
		return "Special Edition"
	default:
		return match
	}
}

// parseService detects streaming service from release name.
// Uses original name (not normalized) to match exact delimiters like ".NF." to avoid false positives.
func parseService(name string) string {
	lower := strings.ToLower(name)
	for code, service := range serviceMap {
		// Match as whole word with delimiters
		if strings.Contains(lower, "."+code+".") ||
			strings.Contains(lower, " "+code+" ") ||
			strings.HasPrefix(lower, code+".") {
			return service
		}
	}
	return ""
}
