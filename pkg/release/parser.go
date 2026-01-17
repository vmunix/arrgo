// Package release parses release names to extract quality, source, codec, etc.
package release

import (
	"regexp"
	"strings"
)

// Info contains parsed information from a release name.
type Info struct {
	Title      string
	Year       int
	Season     int
	Episode    int
	Quality    string // 2160p, 1080p, 720p, 480p
	Source     string // bluray, webdl, webrip, hdtv, dvd
	Codec      string // x264, x265, hevc, xvid
	Audio      string // dts, truehd, atmos, aac
	Group      string // release group
	Proper     bool
	Repack     bool
	Extended   bool
	Directors  bool // director's cut
}

// Parse extracts information from a release name.
func Parse(name string) *Info {
	info := &Info{}

	// Normalize
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")

	// Quality
	info.Quality = parseQuality(name)

	// Source
	info.Source = parseSource(name)

	// Codec
	info.Codec = parseCodec(name)

	// Flags
	info.Proper = containsAny(name, "proper")
	info.Repack = containsAny(name, "repack", "rerip")
	info.Extended = containsAny(name, "extended")
	info.Directors = containsAny(name, "directors cut", "director's cut")

	// Year
	yearRe := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	if match := yearRe.FindString(name); match != "" {
		// TODO: parse year
	}

	// Season/Episode
	seRe := regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,2})`)
	if matches := seRe.FindStringSubmatch(name); len(matches) == 3 {
		// TODO: parse season/episode
	}

	// Group (usually last, after hyphen)
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		group := strings.TrimSpace(name[idx+1:])
		// Remove file extension if present
		if dotIdx := strings.LastIndex(group, " "); dotIdx > 0 {
			group = group[:dotIdx]
		}
		info.Group = group
	}

	return info
}

func parseQuality(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "2160p"), strings.Contains(name, "4k"), strings.Contains(name, "uhd"):
		return "2160p"
	case strings.Contains(name, "1080p"):
		return "1080p"
	case strings.Contains(name, "720p"):
		return "720p"
	case strings.Contains(name, "480p"), strings.Contains(name, "sd"):
		return "480p"
	default:
		return ""
	}
}

func parseSource(name string) string {
	name = strings.ToLower(name)
	switch {
	case containsAny(name, "bluray", "blu-ray", "bdrip", "brrip"):
		return "bluray"
	case containsAny(name, "web-dl", "webdl"):
		return "webdl"
	case containsAny(name, "webrip", "web-rip"):
		return "webrip"
	case containsAny(name, "hdtv"):
		return "hdtv"
	case containsAny(name, "dvdrip", "dvd"):
		return "dvd"
	default:
		return ""
	}
}

func parseCodec(name string) string {
	name = strings.ToLower(name)
	switch {
	case containsAny(name, "x265", "h265", "hevc"):
		return "hevc"
	case containsAny(name, "x264", "h264", "avc"):
		return "x264"
	case containsAny(name, "xvid", "divx"):
		return "xvid"
	default:
		return ""
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

// Score calculates a quality score for ranking releases.
func (i *Info) Score() int {
	score := 0

	// Quality
	switch i.Quality {
	case "2160p":
		score += 100
	case "1080p":
		score += 80
	case "720p":
		score += 60
	case "480p":
		score += 40
	}

	// Source
	switch i.Source {
	case "bluray":
		score += 30
	case "webdl":
		score += 25
	case "webrip":
		score += 20
	case "hdtv":
		score += 15
	case "dvd":
		score += 10
	}

	// Codec (HEVC preferred for efficiency)
	switch i.Codec {
	case "hevc":
		score += 10
	case "x264":
		score += 8
	}

	// Bonuses
	if i.Proper || i.Repack {
		score += 5
	}

	return score
}
