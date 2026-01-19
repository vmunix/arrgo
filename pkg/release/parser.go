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
)

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
	case containsAny(name, "bluray", "blu-ray", "bdrip", "brrip"):
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
	name = strings.ToLower(name)
	switch {
	case containsAny(name, "x265", "h265", "hevc"):
		return CodecX265
	case containsAny(name, "x264", "h264", "avc"):
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
